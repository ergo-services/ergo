package ergo

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/lib"
)

const (
	startPID = 1000
)

type Registrar interface {
	Monitor
	NodeName() string
	GetProcessByName(name string) *Process
	GetProcessByPid(pid etf.Pid) *Process
	GetProcessByAlias(alias etf.Alias) *Process
	ProcessList() []*Process
	MakeRef() etf.Ref
	RegisterName(name string, pid etf.Pid) error
	UnregisterName(name string)
	Spawn(name string, opts ProcessOptions, object ProcessBehavior, args ...etf.Term) (*Process, error)
	IsProcessAlive(pid etf.Pid) bool

	Route(from etf.Pid, to etf.Term, message etf.Term)
	RouteRaw(nodename etf.Atom, messages ...etf.Term) error

	registerPeer(peer *peer) error
	unregisterPeer(name string)
	newAlias(p *Process) (etf.Alias, error)
	deleteAlias(owner *Process, alias etf.Alias) error
}

type registrar struct {
	monitor
	ctx context.Context

	nextPID  uint64
	uniqID   uint64
	nodename string
	creation uint32

	net network

	names          map[string]etf.Pid
	mutexNames     sync.Mutex
	aliases        map[etf.Alias]*Process
	mutexAliases   sync.Mutex
	processes      map[uint64]*Process
	mutexProcesses sync.Mutex
	peers          map[string]*peer
	mutexPeers     sync.Mutex
	apps           map[string]*ApplicationSpec
	mutexApps      sync.Mutex
}

func NewRegistrar(ctx context.Context, nodename string, creation uint32, net network) Registrar {
	r := &registrar{
		ctx:       ctx,
		nextPID:   startPID,
		uniqID:    uint64(time.Now().UnixNano()),
		net:       net,
		nodename:  nodename,
		creation:  creation,
		names:     make(map[string]etf.Pid),
		aliases:   make(map[etf.Alias]*Process),
		processes: make(map[uint64]*Process),
		peers:     make(map[string]*peer),
		apps:      make(map[string]*ApplicationSpec),
	}
	r.monitor = NewMonitor(r)
	return r
}

func (r *registrar) NodeName() string {
	return r.nodename
}

func (r *registrar) newPID() etf.Pid {
	// http://erlang.org/doc/apps/erts/erl_ext_dist.html#pid_ext
	// https://stackoverflow.com/questions/243363/can-someone-explain-the-structure-of-a-pid-in-erlang
	i := atomic.AddUint64(&r.nextPID, 1)
	return etf.Pid{
		Node:     etf.Atom(r.nodename),
		ID:       i,
		Creation: r.creation,
	}

}

// MakeRef returns atomic reference etf.Ref within this node
func (r *registrar) MakeRef() (ref etf.Ref) {
	ref.Node = etf.Atom(r.nodename)
	ref.Creation = r.creation
	nt := atomic.AddUint64(&r.uniqID, 1)
	ref.ID[0] = uint32(uint64(nt) & ((2 << 17) - 1))
	ref.ID[1] = uint32(uint64(nt) >> 46)

	return
}

func (r *registrar) newAlias(p *Process) (etf.Alias, error) {
	var alias etf.Alias
	lib.Log("[%s] REGISTRAR create process alias for %v", r.nodename, p.self)
	r.mutexProcesses.Lock()
	defer r.mutexProcesses.Unlock()

	// chech if its alive
	_, exist := r.processes[p.self.ID]
	if !exist {
		return alias, ErrProcessUnknown
	}

	alias = etf.Alias(r.MakeRef())

	r.mutexAliases.Lock()
	r.aliases[alias] = p
	r.mutexAliases.Unlock()

	p.aliases = append(p.aliases, alias)
	return alias, nil
}

func (r *registrar) deleteAlias(owner *Process, alias etf.Alias) error {
	lib.Log("[%s] REGISTRAR delete process alias %v for %v", r.nodename, alias, owner.self)
	r.mutexProcesses.Lock()
	defer r.mutexProcesses.Unlock()
	r.mutexAliases.Lock()
	defer r.mutexAliases.Unlock()

	p, alias_exist := r.aliases[alias]
	if !alias_exist {
		return ErrAliasUnknown
	}

	process, process_exist := r.processes[p.self.ID]
	if !process_exist {
		return ErrProcessUnknown
	}
	if process.self != owner.self {
		return ErrAliasOwner
	}

	for i := range p.aliases {
		if alias != p.aliases[i] {
			continue
		}
		delete(r.aliases, alias)
		p.aliases[i] = p.aliases[0]
		p.aliases = p.aliases[1:]
		return nil
	}

	fmt.Println("Bug: Process lost its alias. Please, report this issue")
	delete(r.aliases, alias)
	return ErrAliasUnknown
}

func (r *registrar) newProcess(name string, object ProcessBehavior, opts ProcessOptions) (*Process, error) {

	var parentContext context.Context

	mailboxSize := DefaultProcessMailboxSize
	if opts.MailboxSize > 0 {
		mailboxSize = int(opts.MailboxSize)
	}

	switch {
	case opts.Parent != nil:
		parentContext = opts.Parent.Context
	case opts.GroupLeader != nil:
		parentContext = opts.GroupLeader.Context
	default:
		parentContext = r.ctx
	}

	processContext, kill := context.WithCancel(parentContext)

	pid := r.newPID()

	process := &Process{
		Registrar: r,

		self:   pid,
		name:   name,
		object: object,
		env:    opts.Env,

		parent:      opts.Parent,
		groupLeader: opts.GroupLeader,

		mailBox:      make(chan mailboxMessage, mailboxSize),
		ready:        make(chan error),
		gracefulExit: make(chan gracefulExitRequest),
		direct:       make(chan directMessage),
		stopped:      make(chan bool),

		Context: processContext,
		Kill:    kill,

		reply: make(map[etf.Ref]chan etf.Term),
	}

	exit := func(from etf.Pid, reason string) {
		lib.Log("[%s] EXIT: %#v with reason: %s", r.nodename, pid, reason)
		ex := gracefulExitRequest{
			from:   from,
			reason: reason,
		}
		if processContext.Err() != nil {
			// process is already died
			return
		}
		if process.trapExit {
			message := mailboxMessage{
				from: from,
				message: etf.Tuple{
					etf.Atom("EXIT"),
					from,
					etf.Atom(reason),
				}}
			process.mailBox <- message
			return
		}
		// the reason why we use 'select':
		// if this process is on the way of exiting
		// there is nobody to read from the exitChannel and it locks the calling
		// process foreveer
		select {
		case process.gracefulExit <- ex:
		default:
		}
	}
	process.Exit = exit

	if name != "" {
		r.mutexNames.Lock()
		if _, exist := r.names[name]; exist {
			r.mutexNames.Unlock()
			return nil, ErrTaken
		}
		r.names[name] = process.self
		r.mutexNames.Unlock()
	}

	r.mutexProcesses.Lock()
	r.processes[process.self.ID] = process
	r.mutexProcesses.Unlock()

	return process, nil
}

func (r *registrar) deleteProcess(pid etf.Pid) {
	r.mutexProcesses.Lock()
	defer r.mutexProcesses.Unlock()
	if p, ok := r.processes[pid.ID]; ok {
		lib.Log("[%s] REGISTRAR unregistering process: %#v", r.nodename, p.self)
		delete(r.processes, pid.ID)

		r.mutexNames.Lock()
		if (p.name) != "" {
			lib.Log("[%s] REGISTRAR unregistering name (%#v): %s", r.nodename, p.self, p.name)
			delete(r.names, p.name)
		}

		// delete names registered with this pid
		for name, pid := range r.names {
			if p.self == pid {
				delete(r.names, name)
			}
		}
		r.mutexNames.Unlock()

		r.mutexAliases.Lock()
		for alias := range r.aliases {
			delete(r.aliases, alias)
		}
		r.mutexAliases.Unlock()

		// delete associated process with this app
		for _, spec := range r.apps {
			if spec.process != nil && spec.process.self == p.self {
				spec.process = nil
			}
		}
		// invoke cancel context to prevent memory leaks
		p.Kill()
		return
	}
}

// Spawn create new process
func (r *registrar) Spawn(name string, opts ProcessOptions, object ProcessBehavior, args ...etf.Term) (*Process, error) {

	process, err := r.newProcess(name, object, opts)
	if err != nil {
		return nil, err
	}

	go func() {
		pid := process.Self()
		cleanProcess := func() {
			r.deleteProcess(pid)
			r.processTerminated(pid, name, "panic")
		}

		defer func(clean func()) {
			if r := recover(); r != nil {
				pc, fn, line, _ := runtime.Caller(2)
				fmt.Printf("Warning: process recovered (name: %s) %v %#v at %s[%s:%d]\n",
					name, process.self, r, runtime.FuncForPC(pc).Name(), fn, line)

				clean()

				process.Kill()

				process.ready <- fmt.Errorf("Can't start process: %s\n", r)
				close(process.stopped)
			}

			// we should close this channel otherwise if we try
			// immediately call process.Exit it blocks this call forewer
			// since there is nobody to read a message from this channel
			close(process.gracefulExit)
		}(cleanProcess)

		// start process loop
		reason := object.(ProcessBehavior).Loop(process, args...)

		// process stopped. unregister it and let everybody (who set up
		// link/monitor) to know about it
		r.deleteProcess(pid)
		r.processTerminated(pid, name, reason)

		// cancel the context if it was stopped by itself
		if reason != "kill" {
			process.Kill()
		}

		close(process.ready)
		close(process.stopped)
	}()

	if e := <-process.ready; e != nil {
		close(process.ready)
		return nil, e
	}

	return process, nil
}

// RegisterName register associates the name with pid
func (r *registrar) RegisterName(name string, pid etf.Pid) error {
	lib.Log("[%s] REGISTRAR registering name %#v", r.nodename, name)
	r.mutexNames.Lock()
	if _, ok := r.names[name]; ok {
		// already registered
		r.mutexNames.Unlock()
		return ErrTaken
	}
	r.names[name] = pid
	r.mutexNames.Unlock()
	return nil
}

// UnregisterName unregister named process
func (r *registrar) UnregisterName(name string) {
	lib.Log("[%s] REGISTRAR unregistering name %#v", r.nodename, name)
	r.mutexNames.Lock()
	delete(r.names, name)
	r.mutexNames.Unlock()
}

func (r *registrar) registerPeer(peer *peer) error {
	lib.Log("[%s] REGISTRAR registering peer %#v", r.nodename, peer.name)
	r.mutexPeers.Lock()
	defer r.mutexPeers.Unlock()

	if _, ok := r.peers[peer.name]; ok {
		// already registered
		return ErrTaken
	}
	r.peers[peer.name] = peer
	return nil
}

func (r *registrar) unregisterPeer(name string) {
	lib.Log("[%s] REGISTRAR unregistering peer %v", r.nodename, name)
	r.mutexPeers.Lock()
	if _, ok := r.peers[name]; ok {
		delete(r.peers, name)
		r.nodeDown(name)
	}
	r.mutexPeers.Unlock()
}

func (r *registrar) RegisterApp(name string, spec *ApplicationSpec) error {
	lib.Log("[%s] REGISTRAR registering app %v", r.nodename, name)
	r.mutexApps.Lock()
	if _, ok := r.apps[name]; ok {
		// already loaded
		r.mutexApps.Unlock()
		return ErrAppAlreadyLoaded
	}
	r.apps[name] = spec
	r.mutexApps.Unlock()
	return nil
}

func (r *registrar) UnregisterApp(name string) {
	lib.Log("[%s] REGISTRAR unregistering app %v", r.nodename, name)
	r.mutexApps.Lock()
	delete(r.apps, name)
	r.mutexApps.Unlock()
}

func (r *registrar) GetApplicationSpecByName(name string) *ApplicationSpec {
	r.mutexApps.Lock()
	defer r.mutexApps.Unlock()
	if spec, ok := r.apps[name]; ok {
		return spec
	}
	return nil
}

// IsProcessAlive returns true if the process with given pid is alive
func (r *registrar) IsProcessAlive(pid etf.Pid) bool {
	if pid.Node != etf.Atom(r.nodename) {
		return false
	}

	p := r.GetProcessByPid(pid)
	if p == nil {
		return false
	}

	return p.IsAlive()
}

// ProcessInfo returns the details about given Pid
func (r *registrar) ProcessInfo(pid etf.Pid) (ProcessInfo, error) {
	p := r.GetProcessByPid(pid)
	if p == nil {
		return ProcessInfo{}, fmt.Errorf("undefined")
	}

	return p.Info(), nil
}

// GetProcessByPid returns Process struct for the given Pid. Returns nil if it doesn't exist (not found)
func (r *registrar) GetProcessByPid(pid etf.Pid) *Process {
	r.mutexProcesses.Lock()
	defer r.mutexProcesses.Unlock()
	if p, ok := r.processes[pid.ID]; ok {
		return p
	}
	// unknown process
	return nil
}

// GetProcessByAlias returns Process struct for the given alias. Returns nil if it doesn't exist (not found)
func (r *registrar) GetProcessByAlias(alias etf.Alias) *Process {
	r.mutexAliases.Lock()
	defer r.mutexAliases.Unlock()
	if p, ok := r.aliases[alias]; ok {
		return p
	}
	// unknown process
	return nil
}

// GetProcessByPid returns Process struct for the given name. Returns nil if it doesn't exist (not found)
func (r *registrar) GetProcessByName(name string) *Process {
	var pid etf.Pid
	if name != "" {
		// requesting Process by name
		r.mutexNames.Lock()
		if p, ok := r.names[name]; ok {
			r.mutexNames.Unlock()
			pid = p
		} else {
			r.mutexNames.Unlock()
			return nil
		}
	}

	return r.GetProcessByPid(pid)
}

func (r *registrar) ProcessList() []*Process {
	list := []*Process{}
	r.mutexProcesses.Lock()
	for _, p := range r.processes {
		list = append(list, p)
	}
	r.mutexProcesses.Unlock()
	return list
}

func (r *registrar) PeerList() []string {
	list := []string{}
	for n, _ := range r.peers {
		list = append(list, n)
	}
	return list
}

func (r *registrar) ApplicationList() []*ApplicationSpec {
	list := []*ApplicationSpec{}
	r.mutexApps.Lock()
	for _, a := range r.apps {
		list = append(list, a)
	}
	r.mutexApps.Unlock()
	return list
}

// route routes message to a local/remote process
func (r *registrar) Route(from etf.Pid, to etf.Term, message etf.Term) {
next:
	switch tto := to.(type) {
	case etf.Pid:
		lib.Log("[%s] REGISTRAR sending message by pid %#v", r.nodename, tto)
		if string(tto.Node) == r.nodename {
			// local route
			r.mutexProcesses.Lock()
			if p, ok := r.processes[tto.ID]; ok {
				select {
				case p.mailBox <- mailboxMessage{from, message}:

				default:
					fmt.Println("WARNING! mailbox of", p.Self(), "is full. dropped message from", from)
				}
			}
			r.mutexProcesses.Unlock()
			return
		}

		r.mutexPeers.Lock()
		peer, ok := r.peers[string(tto.Node)]
		r.mutexPeers.Unlock()
		if !ok {
			if err := r.net.connect(tto.Node); err != nil {
				lib.Log("[%s] can't connect to %v: %s", r.nodename, tto.Node, err)
				return
			}

			r.mutexPeers.Lock()
			peer, _ = r.peers[string(tto.Node)]
			r.mutexPeers.Unlock()
		}

		send := peer.GetChannel()
		send <- []etf.Term{etf.Tuple{distProtoSEND, etf.Atom(""), tto}, message}

	case etf.Tuple:
		lib.Log("[%s] REGISTRAR sending message by tuple %#v", r.nodename, tto)

		if len(tto) != 2 {
			lib.Log("[%s] can't send message. wrong type. must be etf.Tuple{string, string} or etf.Tuple{etf.Atom, etf.Atom}", r.nodename)
			return
		}

		toNode := etf.Atom("")
		switch x := tto.Element(2).(type) {
		case etf.Atom:
			toNode = x
		case string:
			toNode = etf.Atom(tto.Element(2).(string))
		default:
			lib.Log("[%s] can't send message. wrong type of node name. must be etf.Atom or string", r.nodename)
			return
		}

		toProcessName := etf.Atom("")
		switch x := tto.Element(1).(type) {
		case etf.Atom:
			toProcessName = x
		case string:
			toProcessName = etf.Atom(tto.Element(1).(string))
		default:
			lib.Log("[%s] can't send message. wrong type of process name. must be etf.Atom or string", r.nodename)
			return
		}

		if toNode == etf.Atom(r.nodename) {
			// local route
			r.Route(from, toProcessName, message)
			return
		}

		// sending to remote node
		r.mutexPeers.Lock()
		peer, ok := r.peers[string(toNode)]
		r.mutexPeers.Unlock()
		if !ok {
			// initiate connection and make yet another attempt to deliver this message
			if err := r.net.connect(toNode); err != nil {
				lib.Log("[%s] can't connect to %v: %s", r.nodename, toNode, err)
				return
			}

			r.mutexPeers.Lock()
			peer, _ = r.peers[string(toNode)]
			r.mutexPeers.Unlock()
		}

		send := peer.GetChannel()
		send <- []etf.Term{etf.Tuple{distProtoREG_SEND, from, etf.Atom(""), toProcessName}, message}

	case string:
		lib.Log("[%s] REGISTRAR sending message by name %#v", r.nodename, tto)
		r.mutexNames.Lock()
		if pid, ok := r.names[tto]; ok {
			to = pid
			r.mutexNames.Unlock()
			goto next
		}
		r.mutexNames.Unlock()

	case etf.Atom:
		lib.Log("[%s] REGISTRAR sending message by name %#v", r.nodename, tto)
		r.mutexNames.Lock()
		if pid, ok := r.names[string(tto)]; ok {
			to = pid
			r.mutexNames.Unlock()
			goto next
		}
		r.mutexNames.Unlock()

	case etf.Alias:
		lib.Log("[%s] REGISTRAR sending message by alias %#v", r.nodename, tto)
		r.mutexAliases.Lock()
		if string(tto.Node) == r.nodename {
			// local route by alias
			if p, ok := r.aliases[tto]; ok {
				to = p.self
				r.mutexAliases.Unlock()
				goto next
			}
		}
		r.mutexAliases.Unlock()

		r.mutexPeers.Lock()
		peer, ok := r.peers[string(tto.Node)]
		r.mutexPeers.Unlock()
		if !ok {
			if err := r.net.connect(tto.Node); err != nil {
				lib.Log("[%s] can't connect to %v: %s", r.nodename, tto.Node, err)
				return
			}

			r.mutexPeers.Lock()
			peer, _ = r.peers[string(tto.Node)]
			r.mutexPeers.Unlock()
		}

		send := peer.GetChannel()
		send <- []etf.Term{etf.Tuple{distProtoALIAS_SEND, from, tto}, message}

	default:
		lib.Log("[%s] unknow receiver type %#v", r.nodename, tto)
	}
}

func (r *registrar) RouteRaw(nodename etf.Atom, messages ...etf.Term) error {
	r.mutexPeers.Lock()
	peer, ok := r.peers[string(nodename)]
	r.mutexPeers.Unlock()
	if len(messages) == 0 {
		return fmt.Errorf("nothing to send")
	}
	if !ok {
		// initiate connection and make yet another attempt to deliver this message
		if err := r.net.connect(nodename); err != nil {
			lib.Log("[%s] can't connect to %v: %s", r.nodename, nodename, err)
			return err
		}

		r.mutexPeers.Lock()
		peer, _ = r.peers[string(nodename)]
		r.mutexPeers.Unlock()
	}

	send := peer.GetChannel()
	send <- messages
	return nil
}
