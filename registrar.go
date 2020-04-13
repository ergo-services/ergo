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

type registerProcessRequest struct {
	name    string
	process *Process
	err     chan error
}

type registerNameRequest struct {
	name string
	pid  etf.Pid
	err  chan error
}

type registerPeerRequest struct {
	name string
	peer *peer
	err  chan error
}

type registerAppRequest struct {
	name string
	spec *ApplicationSpec
	err  chan error
}

type routeByPidRequest struct {
	from    etf.Pid
	pid     etf.Pid
	message etf.Term
	retries int
}

type routeByNameRequest struct {
	from    etf.Pid
	name    string
	message etf.Term
	retries int
}

type routeByTupleRequest struct {
	from    etf.Pid
	tuple   etf.Tuple
	message etf.Term
	retries int
}

type routeRawRequest struct {
	nodename string
	message  etf.Term
	retries  int
}

type requestProcessDetails struct {
	name  string
	pid   etf.Pid
	reply chan *Process
}

type requestApplicationSpec struct {
	name  string
	reply chan *ApplicationSpec
}

type requestProcessList struct {
	reply chan []*Process
}

type requestApplicationList struct {
	reply chan []*ApplicationSpec
}

type registrarChannels struct {
	process           chan registerProcessRequest
	unregisterProcess chan etf.Pid
	name              chan registerNameRequest
	unregisterName    chan string
	peer              chan registerPeerRequest
	unregisterPeer    chan string
	app               chan registerAppRequest
	unregisterApp     chan string

	routeByPid   chan routeByPidRequest
	routeByName  chan routeByNameRequest
	routeByTuple chan routeByTupleRequest
	routeRaw     chan routeRawRequest

	commands chan interface{}
}

type registrar struct {
	nextPID  uint32
	nodeName string
	creation byte

	node *Node

	channels registrarChannels

	names          map[string]etf.Pid
	mutexNames     sync.Mutex
	processes      map[etf.Pid]*Process
	mutexProcesses sync.Mutex
	peers          map[string]*peer
	mutexPeers     sync.Mutex
	apps           map[string]*ApplicationSpec
	mutexApps      sync.Mutex
}

func createRegistrar(node *Node) *registrar {
	r := registrar{
		nextPID:  startPID,
		nodeName: node.FullName,
		creation: byte(1),
		node:     node,
		channels: registrarChannels{
			process:           make(chan registerProcessRequest, 10),
			unregisterProcess: make(chan etf.Pid, 10),
			name:              make(chan registerNameRequest, 10),
			unregisterName:    make(chan string, 10),
			peer:              make(chan registerPeerRequest, 10),
			unregisterPeer:    make(chan string, 10),
			app:               make(chan registerAppRequest, 10),
			unregisterApp:     make(chan string, 10),

			routeByPid:   make(chan routeByPidRequest, 100),
			routeByName:  make(chan routeByNameRequest, 100),
			routeByTuple: make(chan routeByTupleRequest, 100),
			routeRaw:     make(chan routeRawRequest, 100),

			commands: make(chan interface{}, 100),
		},

		names:     make(map[string]etf.Pid),
		processes: make(map[etf.Pid]*Process),
		peers:     make(map[string]*peer),
		apps:      make(map[string]*ApplicationSpec),
	}
	go r.run()
	return &r
}

func (r *registrar) createNewPID() etf.Pid {
	// http://erlang.org/doc/apps/erts/erl_ext_dist.html#pid_ext
	// https://stackoverflow.com/questions/243363/can-someone-explain-the-structure-of-a-pid-in-erlang
	i := atomic.AddUint32(&r.nextPID, 1)
	return etf.Pid{
		Node:     etf.Atom(r.nodeName),
		ID:       i,
		Serial:   1,
		Creation: byte(r.creation),
	}

}

func (r *registrar) run() {
	for {
		select {

		case bn := <-r.channels.routeByName:
			lib.Log("[%s] sending message by name %v", r.node.FullName, bn.name)
			if pid, ok := r.names[bn.name]; ok {
				r.route(bn.from, pid, bn.message)
			}

		case cmd := <-r.channels.commands:
			r.handleCommand(cmd)
		}
	}
}

func (r *registrar) RegisterProcess(object interface{}) (*Process, error) {
	opts := ProcessOptions{
		MailboxSize: DefaultProcessMailboxSize, // size of channel for regular messages
	}
	return r.RegisterProcessExt("", object, opts)
}

func (r *registrar) RegisterProcessExt(name string, object interface{}, opts ProcessOptions) (*Process, error) {

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
	exit := func(from etf.Pid, reason string) {
		lib.Log("[%s] EXIT: %#v with reason: %s", r.node.FullName, pid, reason)
		ex := gracefulExitRequest{
			from:   from,
			reason: reason,
		}
		exitChannel <- ex
	}

	process := &Process{
		mailBox:      make(chan etf.Tuple, mailboxSize),
		ready:        make(chan bool),
		gracefulExit: exitChannel,
		direct:       make(chan directMessage),
		self:         pid,
		groupLeader:  opts.GroupLeader,
		Context:      ctx,
		Kill:         kill,
		Exit:         exit,
		name:         name,
		Node:         r.node,
		reply:        make(chan etf.Tuple, 2),
		object:       object,
	}

	if name != "" {
		r.mutexNames.Lock()
		if _, exist := r.names[name]; exist {
			r.mutexNames.Unlock()
			return nil, ErrNameIsTaken
		}
		r.names[name] = process.self
		r.mutexNames.Unlock()
	}

	r.mutexProcesses.Lock()
	r.processes[process.self] = process
	r.mutexProcesses.Unlock()

	return process, nil
}

// UnregisterProcess unregister process by Pid
func (r *registrar) UnregisterProcess(pid etf.Pid) {
	r.mutexProcesses.Lock()
	if p, ok := r.processes[pid]; ok {
		lib.Log("[%s] REGISTRAR unregistering process: %v", r.node.FullName, p.self)
		delete(r.processes, pid)
		r.mutexProcesses.Unlock()

		r.mutexNames.Lock()
		if (p.name) != "" {
			lib.Log("[%s] REGISTRAR unregistering name (%v): %s", r.node.FullName, p.self, p.name)
			delete(r.names, p.name)
		}

		// delete names registered with this pid
		for name, pid := range r.names {
			if p.self == pid {
				delete(r.names, name)
			}
		}
		r.mutexNames.Unlock()

		// delete associated process with this app
		r.mutexProcesses.Lock()
		for _, spec := range r.apps {
			if spec.process != nil && spec.process.self == p.self {
				spec.process = nil
			}
		}
		r.mutexProcesses.Unlock()
		return
	}
	r.mutexProcesses.Unlock()
}

// RegisterName register associates the name with pid
func (r *registrar) RegisterName(name string, pid etf.Pid) error {
	lib.Log("[%s] registering name %v", r.node.FullName, name)
	r.mutexNames.Lock()
	if _, ok := r.names[name]; ok {
		// already registered
		r.mutexNames.Unlock()
		return ErrNameIsTaken
	}
	r.names[name] = pid
	r.mutexNames.Unlock()
	return nil
}

// UnregisterName unregister named process
func (r *registrar) UnregisterName(name string) {
	lib.Log("[%s] unregistering name %v", r.node.FullName, name)
	r.mutexNames.Lock()
	delete(r.names, name)
	r.mutexNames.Unlock()
}

func (r *registrar) RegisterPeer(p *peer) error {
	lib.Log("[%s] registering peer %v", r.node.FullName, p)
	r.mutexPeers.Lock()
	if _, ok := r.peers[p.name]; ok {
		// already registered
		r.mutexPeers.Unlock()
		return ErrNameIsTaken
	}
	r.peers[p.name] = p
	r.mutexPeers.Unlock()
	return nil
}

func (r *registrar) UnregisterPeer(name string) {
	lib.Log("[%s] unregistering peer %v", r.node.FullName, name)
	r.mutexPeers.Lock()
	if _, ok := r.peers[name]; ok {
		r.node.monitor.NodeDown(name)
		delete(r.peers, name)
	}
	r.mutexPeers.Unlock()
}

func (r *registrar) RegisterApp(name string, spec *ApplicationSpec) error {
	lib.Log("[%s] registering app %v", r.node.FullName, name)
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
	lib.Log("[%s] unregistering app %v", r.node.FullName, name)
	r.mutexApps.Lock()
	delete(r.apps, name)
	r.mutexApps.Unlock()
}

func (r *registrar) GetApplicationSpecByName(name string) *ApplicationSpec {
	reply := make(chan *ApplicationSpec)
	req := requestApplicationSpec{
		name:  name,
		reply: reply,
	}
	r.channels.commands <- req
	return <-reply
}

// GetProcessByPid returns Process struct for the given Pid. Returns nil if it doesn't exist (not found)
func (r *registrar) GetProcessByPid(pid etf.Pid) *Process {
	reply := make(chan *Process)
	req := requestProcessDetails{
		pid:   pid,
		reply: reply,
	}
	r.channels.commands <- req
	if p := <-reply; p != nil {
		return p
	}
	// unknown process
	return nil
}

// GetProcessByPid returns Process struct for the given name. Returns nil if it doesn't exist (not found)
func (r *registrar) GetProcessByName(name string) *Process {
	reply := make(chan *Process)
	req := requestProcessDetails{
		name:  name,
		reply: reply,
	}
	r.channels.commands <- req
	if p := <-reply; p != nil {
		return p
	}
	// unknown process
	return nil
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
	switch tto := to.(type) {
	case etf.Pid:
		lib.Log("[%s] sending message by pid %v", r.node.FullName, tto)
		if string(tto.Node) == r.nodeName {
			// local route
			r.mutexProcesses.Lock()
			if p, ok := r.processes[tto]; ok {
				p.mailBox <- etf.Tuple{from, message}
			}
			r.mutexProcesses.Unlock()
			return
		}

		r.mutexPeers.Lock()
		peer, ok := r.peers[string(tto.Node)]
		r.mutexPeers.Unlock()
		if !ok {
			if err := r.node.connect(tto.Node); err != nil {
				fmt.Println("ERRR", err)
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
		if len(tto) == 2 {
			req := routeByTupleRequest{
				from:    from,
				tuple:   tto,
				message: message,
			}
			r.channels.routeByTuple <- req
		}

		lib.Log("[%s] sending message by tuple %v", r.node.FullName, tto)

		toNode := etf.Atom("")
		switch x := tto.Element(2).(type) {
		case etf.Atom:
			toNode = x
		default:
			toNode = etf.Atom(tto.Element(2).(string))
		}

		toProcessName := tto.Element(1)
		if toNode == etf.Atom(r.nodeName) {
			// local route
			r.route(from, toProcessName, message)
			return
		}

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
		req := routeByNameRequest{
			from:    from,
			name:    tto,
			message: message,
		}
		r.channels.routeByName <- req

	case etf.Atom:
		req := routeByNameRequest{
			from:    from,
			name:    string(tto),
			message: message,
		}
		r.channels.routeByName <- req
	default:
		lib.Log("[%s] unknow sender type %#v", r.node.FullName, tto)
	}
}

func (r *registrar) routeRaw(nodename etf.Atom, message etf.Term) {
	req := routeRawRequest{
		nodename: string(nodename),
		message:  message,
	}
	r.channels.routeRaw <- req

	r.mutexPeers.Lock()
	peer, ok := r.peers[string(nodename)]
	r.mutexPeers.Unlock()

	if !ok {
		// initiate connection and make yet another attempt to deliver this message
		if err := r.node.connect(nodename); err != nil {
			lib.Log("[%s] can't connect to %v: %s", r.node.FullName, nodename, err)
			return
		}

		r.mutexPeers.Lock()
		peer, _ = r.peers[string(nodename)]
		r.mutexPeers.Unlock()
	}

	send := peer.GetChannel()
	send <- []etf.Term{message}
}

func (r *registrar) handleCommand(cmd interface{}) {
	switch c := cmd.(type) {
	case requestProcessDetails:
		pid := c.pid
		if c.name != "" {
			// requesting Process by name
			if p, ok := r.names[c.name]; ok {
				pid = p
			}
		}

		if p, ok := r.processes[pid]; ok {
			c.reply <- p
		} else {
			c.reply <- nil
		}

	case requestProcessList:

	case requestApplicationSpec:
		if spec, ok := r.apps[c.name]; ok {
			c.reply <- spec
			return
		}
		c.reply <- nil

	case requestApplicationList:
	}

}
