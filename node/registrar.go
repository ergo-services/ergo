package node

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/gen"
	"github.com/halturin/ergo/lib"
)

const (
	startPID = 1000
)

type registrar struct {
	monitor
	ctx context.Context

	nextPID  uint64
	uniqID   uint64
	nodename string
	creation uint32

	net  networkInternal
	node nodeInternal

	names          map[string]etf.Pid
	mutexNames     sync.Mutex
	aliases        map[etf.Alias]*process
	mutexAliases   sync.Mutex
	processes      map[uint64]*process
	mutexProcesses sync.Mutex
	peers          map[string]*peer
	mutexPeers     sync.Mutex

	behaviors      map[string]map[string]gen.RegisteredBehavior
	mutexBehaviors sync.Mutex
}

type registrarInternal interface {
	gen.Registrar
	monitorInternal

	spawn(name string, opts processOptions, behavior gen.ProcessBehavior, args ...etf.Term) (gen.Process, error)
	registerPeer(peer *peer) error
	unregisterPeer(name string)
	newAlias(p *process) (etf.Alias, error)
	deleteAlias(owner *process, alias etf.Alias) error
	getProcessByPid(etf.Pid) *process
}

func newRegistrar(ctx context.Context, nodename string, creation uint32, node nodeInternal) registrarInternal {
	r := &registrar{
		ctx:       ctx,
		nextPID:   startPID,
		uniqID:    uint64(time.Now().UnixNano()),
		net:       node.(networkInternal),
		node:      node.(nodeInternal),
		nodename:  nodename,
		creation:  creation,
		names:     make(map[string]etf.Pid),
		aliases:   make(map[etf.Alias]*process),
		processes: make(map[uint64]*process),
		peers:     make(map[string]*peer),
		behaviors: make(map[string]map[string]gen.RegisteredBehavior),
	}
	r.monitor = newMonitor(r)
	return r
}

func (r *registrar) NodeName() string {
	return r.node.Name()
}

func (r *registrar) NodeStop() {
	r.node.Stop()
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

func (r *registrar) newAlias(p *process) (etf.Alias, error) {
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

func (r *registrar) deleteAlias(owner *process, alias etf.Alias) error {
	lib.Log("[%s] REGISTRAR delete process alias %v for %v", r.nodename, alias, owner.self)
	r.mutexProcesses.Lock()
	defer r.mutexProcesses.Unlock()
	r.mutexAliases.Lock()
	defer r.mutexAliases.Unlock()

	p, alias_exist := r.aliases[alias]
	if !alias_exist {
		return ErrAliasUnknown
	}

	process, process_exist := r.processes[owner.self.ID]
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

func (r *registrar) newProcess(name string, behavior gen.ProcessBehavior, opts processOptions) (*process, error) {

	var parentContext context.Context

	mailboxSize := DefaultProcessMailboxSize
	if opts.MailboxSize > 0 {
		mailboxSize = int(opts.MailboxSize)
	}

	switch {
	case opts.parent != nil:
		parentContext = opts.parent.context
	case opts.GroupLeader != nil:
		parentContext = opts.GroupLeader.Context()
	default:
		parentContext = r.ctx
	}

	processContext, kill := context.WithCancel(parentContext)

	pid := r.newPID()

	process := &process{
		registrarInternal: r,

		self:     pid,
		name:     name,
		behavior: behavior,
		env:      opts.Env,

		parent:      opts.parent,
		groupLeader: opts.GroupLeader,

		mailBox:      make(chan gen.ProcessMailboxMessage, mailboxSize),
		gracefulExit: make(chan gen.ProcessGracefulExitRequest),
		direct:       make(chan gen.ProcessDirectMessage),

		context: processContext,
		kill:    kill,

		reply: make(map[etf.Ref]chan etf.Term),
	}

	process.exit = func(from etf.Pid, reason string) error {
		lib.Log("[%s] EXIT from %s to %s with reason: %s", r.nodename, from, pid, reason)
		if processContext.Err() != nil {
			// process is already died
			return ErrProcessUnknown
		}

		ex := gen.ProcessGracefulExitRequest{
			From:   from,
			Reason: reason,
		}
		select {
		case process.gracefulExit <- ex:
		default:
			return ErrProcessBusy
		}

		// let the process decide whether to stop itself, otherwise its going to be killed
		if !process.trapExit {
			process.kill()
		}
		return nil
	}

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
		lib.Log("[%s] REGISTRAR unregistering process: %s", r.nodename, p.self)
		delete(r.processes, pid.ID)

		r.mutexNames.Lock()
		if (p.name) != "" {
			lib.Log("[%s] REGISTRAR unregistering name (%s): %s", r.nodename, p.self, p.name)
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

		// invoke cancel context to prevent memory leaks
		p.Kill()
		return
	}
}

// Spawn create new process
func (r *registrar) spawn(name string, opts processOptions, behavior gen.ProcessBehavior, args ...etf.Term) (gen.Process, error) {

	process, err := r.newProcess(name, behavior, opts)
	if err != nil {
		return nil, err
	}
	processState, err := behavior.ProcessInit(process, args...)
	if err != nil {
		return nil, err
	}

	started := make(chan bool)
	defer close(started)

	go func(ps gen.ProcessState) {
		pid := process.Self()
		//cleanProcess := func() {
		//	r.deleteProcess(pid)
		//	r.processTerminated(pid, name, "panic")
		//}

		//defer func(clean func()) {
		//	if r := recover(); r != nil {
		//		pc, fn, line, _ := runtime.Caller(2)
		//		fmt.Printf("Warning: process recovered (name: %s) %v %#v at %s[%s:%d]\n",
		//			name, process.self, r, runtime.FuncForPC(pc).Name(), fn, line)

		//		clean()

		//		process.ready <- fmt.Errorf("Can't start process: %s\n", r)
		//		close(process.stopped)
		//	}

		//	// we should close this channel otherwise if we try
		//	// immediately call process.Exit it blocks this call forewer
		//	// since there is nobody to read a message from this channel
		//	close(process.gracefulExit)
		//}(cleanProcess)

		// start process loop
		reason := behavior.ProcessLoop(ps, started)

		// process stopped. unregister it and let everybody (who set up
		// link/monitor) to know about it
		r.deleteProcess(pid)
		r.processTerminated(pid, name, reason)

	}(processState)

	// wait for the starting process loop
	<-started
	return process, nil
}

// RegisterName register associates the name with pid
func (r *registrar) RegisterName(name string, pid etf.Pid) error {
	lib.Log("[%s] REGISTRAR registering name %s", r.nodename, name)
	r.mutexNames.Lock()
	defer r.mutexNames.Unlock()
	if _, ok := r.names[name]; ok {
		// already registered
		return ErrTaken
	}
	r.names[name] = pid
	return nil
}

// UnregisterName unregister named process
func (r *registrar) UnregisterName(name string) {
	lib.Log("[%s] REGISTRAR unregistering name %s", r.nodename, name)
	r.mutexNames.Lock()
	defer r.mutexNames.Unlock()
	delete(r.names, name)
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
	defer r.mutexPeers.Unlock()
	if _, ok := r.peers[name]; ok {
		delete(r.peers, name)
		r.nodeDown(name)
	}
}

func (r *registrar) RegisterBehavior(group, name string, behavior gen.ProcessBehavior, data interface{}) error {
	lib.Log("[%s] REGISTRAR registering behavior %q in group %q ", r.nodename, name, group)
	var groupBehaviors map[string]gen.RegisteredBehavior
	var exist bool

	r.mutexBehaviors.Lock()
	defer r.mutexBehaviors.Unlock()

	groupBehaviors, exist = r.behaviors[group]
	if !exist {
		groupBehaviors = make(map[string]gen.RegisteredBehavior)
		r.behaviors[group] = groupBehaviors
	}

	_, exist = groupBehaviors[name]
	if exist {
		return ErrTaken
	}

	rb := gen.RegisteredBehavior{
		Behavior: behavior,
		Data:     data,
	}
	groupBehaviors[name] = rb
	return nil
}

func (r *registrar) GetRegisteredBehavior(group, name string) (gen.RegisteredBehavior, error) {
	var groupBehaviors map[string]gen.RegisteredBehavior
	var rb gen.RegisteredBehavior
	var exist bool

	r.mutexBehaviors.Lock()
	defer r.mutexBehaviors.Unlock()

	groupBehaviors, exist = r.behaviors[group]
	if !exist {
		return rb, ErrBehaviorGroupUnknown
	}

	rb, exist = groupBehaviors[name]
	if !exist {
		return rb, ErrBehaviorUnknown
	}
	return rb, nil
}

func (r *registrar) GetRegisteredBehaviorGroup(group string) []gen.RegisteredBehavior {
	var groupBehaviors map[string]gen.RegisteredBehavior
	var exist bool
	var listrb []gen.RegisteredBehavior

	r.mutexBehaviors.Lock()
	defer r.mutexBehaviors.Unlock()

	groupBehaviors, exist = r.behaviors[group]
	if !exist {
		return listrb
	}

	for _, v := range groupBehaviors {
		listrb = append(listrb, v)
	}
	return listrb
}

func (r *registrar) UnregisterBehavior(group, name string) error {
	lib.Log("[%s] REGISTRAR unregistering behavior %s in group %s ", r.nodename, name, group)
	var groupBehaviors map[string]gen.RegisteredBehavior
	var exist bool

	r.mutexBehaviors.Lock()
	defer r.mutexBehaviors.Unlock()

	groupBehaviors, exist = r.behaviors[group]
	if !exist {
		return ErrBehaviorUnknown
	}
	delete(groupBehaviors, name)

	// remove group if its empty
	if len(groupBehaviors) == 0 {
		delete(r.behaviors, group)
	}
	return nil
}

// IsProcessAlive returns true if the process with given pid is alive
func (r *registrar) IsProcessAlive(process gen.Process) bool {
	pid := process.Self()
	p := r.GetProcessByPid(pid)
	if p == nil {
		return false
	}

	return p.IsAlive()
}

// ProcessInfo returns the details about given Pid
func (r *registrar) ProcessInfo(pid etf.Pid) (gen.ProcessInfo, error) {
	p := r.GetProcessByPid(pid)
	if p == nil {
		return gen.ProcessInfo{}, fmt.Errorf("undefined")
	}

	return p.Info(), nil
}

// GetProcessByPid returns Process struct for the given Pid. Returns nil if it doesn't exist (not found)
func (r *registrar) GetProcessByPid(pid etf.Pid) gen.Process {
	if p := r.getProcessByPid(pid); p != nil {
		return p
	}
	// we must return nil explicitly, otherwise returning value is not nil
	// even for the nil(*process) due to the nature of interface type
	return nil
}

func (r *registrar) getProcessByPid(pid etf.Pid) *process {
	r.mutexProcesses.Lock()
	defer r.mutexProcesses.Unlock()
	if p, ok := r.processes[pid.ID]; ok {
		return p
	}
	// unknown process
	return nil
}

// GetProcessByAlias returns Process struct for the given alias. Returns nil if it doesn't exist (not found)
func (r *registrar) GetProcessByAlias(alias etf.Alias) gen.Process {
	r.mutexAliases.Lock()
	defer r.mutexAliases.Unlock()
	if p, ok := r.aliases[alias]; ok {
		return p
	}
	// unknown process
	return nil
}

// GetProcessByPid returns Process struct for the given name. Returns nil if it doesn't exist (not found)
func (r *registrar) GetProcessByName(name string) gen.Process {
	var pid etf.Pid
	if name != "" {
		// requesting Process by name
		r.mutexNames.Lock()
		defer r.mutexNames.Unlock()

		if p, ok := r.names[name]; ok {
			pid = p
		} else {
			return nil
		}
	}

	return r.GetProcessByPid(pid)
}

func (r *registrar) ProcessList() []gen.Process {
	list := []gen.Process{}
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
				case p.mailBox <- gen.ProcessMailboxMessage{from, message}:

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
				lib.Log("[%s] Can't connect to %v: %s", r.nodename, tto.Node, err)
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
			lib.Log("[%s] Can't send message. wrong type. must be etf.Tuple{string, string} or etf.Tuple{etf.Atom, etf.Atom}", r.nodename)
			return
		}

		toNode := etf.Atom("")
		switch x := tto.Element(2).(type) {
		case etf.Atom:
			toNode = x
		case string:
			toNode = etf.Atom(tto.Element(2).(string))
		default:
			lib.Log("[%s] Can't send message. wrong type of node name. must be etf.Atom or string", r.nodename)
			return
		}

		toProcessName := etf.Atom("")
		switch x := tto.Element(1).(type) {
		case etf.Atom:
			toProcessName = x
		case string:
			toProcessName = etf.Atom(tto.Element(1).(string))
		default:
			lib.Log("[%s] Can't send message. wrong type of process name. must be etf.Atom or string", r.nodename)
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
				lib.Log("[%s] Can't connect to %v: %s", r.nodename, toNode, err)
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
				lib.Log("[%s] Can't connect to %v: %s", r.nodename, tto.Node, err)
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
			lib.Log("[%s] Can't connect to %v: %s", r.nodename, nodename, err)
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
