package ergo

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/lib"
)

const (
	startPID = 1000
)

type registrar struct {
	nextPID  uint64
	nodeName string
	creation byte

	node *Node

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

func createRegistrar(node *Node) *registrar {
	r := registrar{
		nextPID:   startPID,
		nodeName:  node.FullName,
		creation:  byte(1),
		node:      node,
		names:     make(map[string]etf.Pid),
		aliases:   make(map[etf.Alias]*Process),
		processes: make(map[uint64]*Process),
		peers:     make(map[string]*peer),
		apps:      make(map[string]*ApplicationSpec),
	}
	return &r
}

func (r *registrar) createNewPID() etf.Pid {
	// http://erlang.org/doc/apps/erts/erl_ext_dist.html#pid_ext
	// https://stackoverflow.com/questions/243363/can-someone-explain-the-structure-of-a-pid-in-erlang
	i := atomic.AddUint64(&r.nextPID, 1)
	return etf.Pid{
		Node:     etf.Atom(r.nodeName),
		ID:       i,
		Creation: r.node.creation,
	}

}

func (r *registrar) createNewAlias(p *Process) (etf.Alias, error) {
	var alias etf.Alias
	lib.Log("[%s] REGISTRAR create process alias for %v", r.node.FullName, p.self)
	r.mutexProcesses.Lock()
	defer r.mutexProcesses.Unlock()

	// chech if its alive
	_, exist := r.processes[p.self.ID]
	if !exist {
		return alias, ErrProcessUnknown
	}

	alias = etf.Alias(r.node.MakeRef())

	r.mutexAliases.Lock()
	r.aliases[alias] = p
	r.mutexAliases.Unlock()

	p.aliases = append(p.aliases, alias)
	return alias, nil
}

func (r *registrar) deleteAlias(owner *Process, alias etf.Alias) error {
	lib.Log("[%s] REGISTRAR delete process alias %v for %v", r.node.FullName, alias, owner.self)
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

func (r *registrar) RegisterProcess(object ProcessBehavior) (*Process, error) {
	opts := ProcessOptions{
		MailboxSize: DefaultProcessMailboxSize, // size of channel for regular messages
	}
	return r.RegisterProcessExt("", object, opts)
}

func (r *registrar) RegisterProcessExt(name string, object ProcessBehavior, opts ProcessOptions) (*Process, error) {

	mailboxSize := DefaultProcessMailboxSize
	if opts.MailboxSize > 0 {
		mailboxSize = int(opts.MailboxSize)
	}

	parentContext := r.node.context
	if opts.parent != nil {
		parentContext = opts.parent.Context
	}
	ctx, kill := context.WithCancel(parentContext)

	pid := r.createNewPID()

	exitChannel := make(chan gracefulExitRequest)

	process := &Process{
		mailBox:      make(chan mailboxMessage, mailboxSize),
		ready:        make(chan error),
		stopped:      make(chan bool),
		gracefulExit: exitChannel,
		direct:       make(chan directMessage),
		self:         pid,
		groupLeader:  opts.GroupLeader,
		Context:      ctx,
		Kill:         kill,
		name:         name,
		Node:         r.node,
		reply:        make(map[etf.Ref]chan etf.Term),
		object:       object,
	}

	exit := func(from etf.Pid, reason string) {
		lib.Log("[%s] EXIT: %#v with reason: %s", r.node.FullName, pid, reason)
		ex := gracefulExitRequest{
			from:   from,
			reason: reason,
		}
		if ctx.Err() != nil {
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
		case exitChannel <- ex:
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

// UnregisterProcess unregister process by Pid
func (r *registrar) UnregisterProcess(pid etf.Pid) {
	r.mutexProcesses.Lock()
	defer r.mutexProcesses.Unlock()
	if p, ok := r.processes[pid.ID]; ok {
		lib.Log("[%s] REGISTRAR unregistering process: %#v", r.node.FullName, p.self)
		delete(r.processes, pid.ID)

		r.mutexNames.Lock()
		if (p.name) != "" {
			lib.Log("[%s] REGISTRAR unregistering name (%#v): %s", r.node.FullName, p.self, p.name)
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

// RegisterName register associates the name with pid
func (r *registrar) RegisterName(name string, pid etf.Pid) error {
	lib.Log("[%s] REGISTRAR registering name %#v", r.node.FullName, name)
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
	lib.Log("[%s] REGISTRAR unregistering name %#v", r.node.FullName, name)
	r.mutexNames.Lock()
	delete(r.names, name)
	r.mutexNames.Unlock()
}

func (r *registrar) RegisterPeer(peer *peer) error {
	lib.Log("[%s] REGISTRAR registering peer %#v", r.node.FullName, peer.name)
	r.mutexPeers.Lock()
	defer r.mutexPeers.Unlock()

	if _, ok := r.peers[peer.name]; ok {
		// already registered
		return ErrTaken
	}
	r.peers[peer.name] = peer
	return nil
}

func (r *registrar) UnregisterPeer(name string) {
	lib.Log("[%s] REGISTRAR unregistering peer %v", r.node.FullName, name)
	r.mutexPeers.Lock()
	if _, ok := r.peers[name]; ok {
		delete(r.peers, name)
		r.node.monitor.NodeDown(name)
	}
	r.mutexPeers.Unlock()
}

func (r *registrar) RegisterApp(name string, spec *ApplicationSpec) error {
	lib.Log("[%s] REGISTRAR registering app %v", r.node.FullName, name)
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
	lib.Log("[%s] REGISTRAR unregistering app %v", r.node.FullName, name)
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
func (r *registrar) route(from etf.Pid, to etf.Term, message etf.Term) {
next:
	switch tto := to.(type) {
	case etf.Pid:
		lib.Log("[%s] REGISTRAR sending message by pid %#v", r.node.FullName, tto)
		if string(tto.Node) == r.nodeName {
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
			if err := r.node.connect(tto.Node); err != nil {
				lib.Log("[%s] can't connect to %v: %s", r.node.FullName, tto.Node, err)
				return
			}

			r.mutexPeers.Lock()
			peer, _ = r.peers[string(tto.Node)]
			r.mutexPeers.Unlock()
		}

		send := peer.GetChannel()
		send <- []etf.Term{etf.Tuple{distProtoSEND, etf.Atom(""), tto}, message}

	case etf.Tuple:
		lib.Log("[%s] REGISTRAR sending message by tuple %#v", r.node.FullName, tto)

		if len(tto) != 2 {
			lib.Log("[%s] can't send message. wrong type. must be etf.Tuple{string, string} or etf.Tuple{etf.Atom, etf.Atom}", r.node.FullName)
			return
		}

		toNode := etf.Atom("")
		switch x := tto.Element(2).(type) {
		case etf.Atom:
			toNode = x
		case string:
			toNode = etf.Atom(tto.Element(2).(string))
		default:
			lib.Log("[%s] can't send message. wrong type of node name. must be etf.Atom or string", r.node.FullName)
			return
		}

		toProcessName := etf.Atom("")
		switch x := tto.Element(1).(type) {
		case etf.Atom:
			toProcessName = x
		case string:
			toProcessName = etf.Atom(tto.Element(1).(string))
		default:
			lib.Log("[%s] can't send message. wrong type of process name. must be etf.Atom or string", r.node.FullName)
			return
		}

		if toNode == etf.Atom(r.nodeName) {
			// local route
			r.route(from, toProcessName, message)
			return
		}

		// sending to remote node
		r.mutexPeers.Lock()
		peer, ok := r.peers[string(toNode)]
		r.mutexPeers.Unlock()
		if !ok {
			// initiate connection and make yet another attempt to deliver this message
			if err := r.node.connect(toNode); err != nil {
				lib.Log("[%s] can't connect to %v: %s", r.node.FullName, toNode, err)
				return
			}

			r.mutexPeers.Lock()
			peer, _ = r.peers[string(toNode)]
			r.mutexPeers.Unlock()
		}

		send := peer.GetChannel()
		send <- []etf.Term{etf.Tuple{distProtoREG_SEND, from, etf.Atom(""), toProcessName}, message}

	case string:
		lib.Log("[%s] REGISTRAR sending message by name %#v", r.node.FullName, tto)
		r.mutexNames.Lock()
		if pid, ok := r.names[tto]; ok {
			to = pid
			r.mutexNames.Unlock()
			goto next
		}
		r.mutexNames.Unlock()

	case etf.Atom:
		lib.Log("[%s] REGISTRAR sending message by name %#v", r.node.FullName, tto)
		r.mutexNames.Lock()
		if pid, ok := r.names[string(tto)]; ok {
			to = pid
			r.mutexNames.Unlock()
			goto next
		}
		r.mutexNames.Unlock()

	case etf.Alias:
		lib.Log("[%s] REGISTRAR sending message by alias %#v", r.node.FullName, tto)
		r.mutexAliases.Lock()
		if string(tto.Node) == r.nodeName {
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
			if err := r.node.connect(tto.Node); err != nil {
				lib.Log("[%s] can't connect to %v: %s", r.node.FullName, tto.Node, err)
				return
			}

			r.mutexPeers.Lock()
			peer, _ = r.peers[string(tto.Node)]
			r.mutexPeers.Unlock()
		}

		send := peer.GetChannel()
		send <- []etf.Term{etf.Tuple{distProtoALIAS_SEND, from, tto}, message}

	default:
		lib.Log("[%s] unknow receiver type %#v", r.node.FullName, tto)
	}
}

func (r *registrar) routeRaw(nodename etf.Atom, messages ...etf.Term) error {
	r.mutexPeers.Lock()
	peer, ok := r.peers[string(nodename)]
	r.mutexPeers.Unlock()
	if len(messages) == 0 {
		return fmt.Errorf("nothing to send")
	}
	if !ok {
		// initiate connection and make yet another attempt to deliver this message
		if err := r.node.connect(nodename); err != nil {
			lib.Log("[%s] can't connect to %v: %s", r.node.FullName, nodename, err)
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
