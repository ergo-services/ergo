package ergo

import (
	"context"
	"fmt"
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
	peer peer
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

	names     map[string]etf.Pid
	processes map[etf.Pid]*Process
	peers     map[string]peer
	apps      map[string]*ApplicationSpec
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
		peers:     make(map[string]peer),
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
		case p := <-r.channels.process:
			if p.name != "" {
				if _, exist := r.names[p.name]; exist {
					p.err <- ErrNameIsTaken
					continue
				}
				r.names[p.name] = p.process.self
			}

			r.processes[p.process.self] = p.process
			p.err <- nil

		case up := <-r.channels.unregisterProcess:
			if p, ok := r.processes[up]; ok {
				lib.Log("[%s] REGISTRAR unregistering process: %v", r.node.FullName, p.self)
				delete(r.processes, up)
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

				// delete associated process with this app
				for _, spec := range r.apps {
					if spec.process != nil && spec.process.self == p.self {
						spec.process = nil
					}
				}
			}

		case n := <-r.channels.name:
			lib.Log("[%s] registering name %v", r.node.FullName, n)
			if _, ok := r.names[n.name]; ok {
				// already registered
				n.err <- ErrNameIsTaken
				continue
			}
			r.names[n.name] = n.pid
			n.err <- nil

		case un := <-r.channels.unregisterName:
			lib.Log("[%s] unregistering name %v", r.node.FullName, un)
			delete(r.names, un)

		case p := <-r.channels.peer:
			lib.Log("[%s] registering peer %v", r.node.FullName, p)
			if _, ok := r.peers[p.name]; ok {
				// already registered
				p.err <- ErrNameIsTaken
				continue
			}
			r.peers[p.name] = p.peer
			p.err <- nil

		case up := <-r.channels.unregisterPeer:
			lib.Log("[%s] unregistering peer %v", r.node.FullName, up)
			if _, ok := r.peers[up]; ok {
				r.node.monitor.NodeDown(up)
				delete(r.peers, up)
			}

		case a := <-r.channels.app:
			lib.Log("[%s] registering app %v", r.node.FullName, a)
			if _, ok := r.apps[a.name]; ok {
				// already loaded
				a.err <- ErrAppAlreadyLoaded
				continue
			}
			r.apps[a.name] = a.spec
			a.err <- nil

		case ua := <-r.channels.unregisterApp:
			lib.Log("[%s] unregistering app %v", r.node.FullName, ua)
			delete(r.apps, ua)

		case <-r.node.context.Done():
			lib.Log("[%s] Finalizing (KILL) registrar (total number of processes: %d)", r.node.FullName, len(r.processes))
			for _, p := range r.processes {
				p.Kill()
			}
			return

		case bp := <-r.channels.routeByPid:
			lib.Log("[%s] sending message by pid %v", r.node.FullName, bp.pid)
			if bp.retries > 2 {
				// drop this message after 3 attempts to deliver this message
				continue
			}

			if string(bp.pid.Node) == r.nodeName {
				// local route
				if p, ok := r.processes[bp.pid]; ok {
					p.mailBox <- etf.Tuple{bp.from, bp.message}
				}
				continue
			}
			peer, ok := r.peers[string(bp.pid.Node)]
			if !ok {
				// initiate connection and make yet another attempt to deliver this message
				go func() {
					if err := r.node.connect(bp.pid.Node); err != nil {
						lib.Log("[%s] can't connect to %v: %s", r.node.FullName, bp.pid.Node, err)
					}

					bp.retries++
					r.channels.routeByPid <- bp
				}()
				continue
			}
			select {
			case peer.send <- []etf.Term{etf.Tuple{distProtoSEND, etf.Atom(""), bp.pid}, bp.message}:
			default:
				fmt.Printf("Congession detected on link with %s. Packet dropped\n", peer.name)
			}

		case bn := <-r.channels.routeByName:
			lib.Log("[%s] sending message by name %v", r.node.FullName, bn.name)
			if pid, ok := r.names[bn.name]; ok {
				r.route(bn.from, pid, bn.message)
			}

		case bt := <-r.channels.routeByTuple:
			lib.Log("[%s] sending message by tuple %v", r.node.FullName, bt.tuple)
			if bt.retries > 2 {
				// drop this message after 3 attempts to deliver this message
				continue
			}

			toNode := etf.Atom("")
			switch x := bt.tuple.Element(2).(type) {
			case etf.Atom:
				toNode = x
			default:
				toNode = etf.Atom(bt.tuple.Element(2).(string))
			}

			toProcessName := bt.tuple.Element(1)
			if toNode == etf.Atom(r.nodeName) {
				r.route(bt.from, toProcessName, bt.message)
				continue
			}

			peer, ok := r.peers[string(toNode)]
			if !ok {
				// initiate connection and make yet another attempt to deliver this message
				go func() {
					r.node.connect(toNode)
					bt.retries++
					r.channels.routeByTuple <- bt
				}()

				continue
			}
			select {
			case peer.send <- []etf.Term{etf.Tuple{distProtoREG_SEND, bt.from, etf.Atom(""), toProcessName}, bt.message}:
			default:
				fmt.Printf("Congession detected on link with %s. Packet dropped\n", peer.name)
			}

		case rw := <-r.channels.routeRaw:
			if rw.retries > 2 {
				// drop this message after 3 attempts of delivering
				continue
			}
			peer, ok := r.peers[rw.nodename]
			if !ok {
				// initiate connection and make yet another attempt to deliver this message
				go func() {
					if err := r.node.connect(etf.Atom(rw.nodename)); err != nil {
						lib.Log("[%s] can't connect to %v: %s", r.node.FullName, rw.nodename, err)
					}

					rw.retries++
					r.channels.routeRaw <- rw
				}()

				continue
			}

			peer.send <- []etf.Term{rw.message}

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

	req := registerProcessRequest{
		name:    name,
		process: process,
		err:     make(chan error),
	}

	r.channels.process <- req
	if err := <-req.err; err != nil {
		return nil, err
	}

	return process, nil
}

// UnregisterProcess unregister process by Pid
func (r *registrar) UnregisterProcess(pid etf.Pid) {
	r.channels.unregisterProcess <- pid
}

// RegisterName register associates the name with pid
func (r *registrar) RegisterName(name string, pid etf.Pid) error {
	req := registerNameRequest{
		name: name,
		pid:  pid,
		err:  make(chan error),
	}
	defer close(req.err)
	r.channels.name <- req
	return <-req.err
}

// UnregisterName unregister named process
func (r *registrar) UnregisterName(name string) {
	r.channels.unregisterName <- name
}

func (r *registrar) RegisterPeer(p peer) error {
	req := registerPeerRequest{
		name: p.name,
		peer: p,
		err:  make(chan error),
	}
	defer close(req.err)
	r.channels.peer <- req
	return <-req.err
}

func (r *registrar) UnregisterPeer(name string) {
	r.channels.unregisterPeer <- name
}

func (r *registrar) RegisterApp(name string, spec *ApplicationSpec) error {
	req := registerAppRequest{
		name: name,
		spec: spec,
		err:  make(chan error),
	}
	defer close(req.err)
	r.channels.app <- req
	return <-req.err
}

func (r *registrar) UnregisterApp(name string) {
	r.channels.unregisterApp <- name
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

func (r registrar) ProcessList() []*Process {
	req := requestProcessList{
		reply: make(chan []*Process),
	}
	r.channels.commands <- req
	return <-req.reply
}

func (r registrar) ApplicationList() []*ApplicationSpec {
	req := requestApplicationList{
		reply: make(chan []*ApplicationSpec),
	}
	r.channels.commands <- req
	return <-req.reply
}

// route routes message to a local/remote process
func (r *registrar) route(from etf.Pid, to etf.Term, message etf.Term) {
	switch tto := to.(type) {
	case etf.Pid:
		req := routeByPidRequest{
			from:    from,
			pid:     tto,
			message: message,
		}
		r.channels.routeByPid <- req

	case etf.Tuple:
		if len(tto) == 2 {
			req := routeByTupleRequest{
				from:    from,
				tuple:   tto,
				message: message,
			}
			r.channels.routeByTuple <- req
		}

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
		list := []*Process{}
		for _, p := range r.processes {
			list = append(list, p)
		}
		c.reply <- list

	case requestApplicationSpec:
		if spec, ok := r.apps[c.name]; ok {
			c.reply <- spec
			return
		}
		c.reply <- nil

	case requestApplicationList:
		list := []*ApplicationSpec{}
		for _, a := range r.apps {
			list = append(list, a)
		}
		c.reply <- list
	}

}
