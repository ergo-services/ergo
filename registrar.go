package ergonode

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/halturin/ergonode/etf"
	"github.com/halturin/ergonode/lib"
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
}

type registerPeer struct {
	name string
	peer peer
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

type registrarChannels struct {
	process           chan registerProcessRequest
	unregisterProcess chan etf.Pid
	name              chan registerNameRequest
	unregisterName    chan string
	peer              chan registerPeer
	unregisterPeer    chan string

	routeByPid   chan routeByPidRequest
	routeByName  chan routeByNameRequest
	routeByTuple chan routeByTupleRequest
	routeRaw     chan routeRawRequest
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
			peer:              make(chan registerPeer, 10),
			unregisterPeer:    make(chan string, 10),

			routeByPid:   make(chan routeByPidRequest, 100),
			routeByName:  make(chan routeByNameRequest, 100),
			routeByTuple: make(chan routeByTupleRequest, 100),
			routeRaw:     make(chan routeRawRequest, 100),
		},

		names:     make(map[string]etf.Pid),
		processes: make(map[etf.Pid]*Process),
		peers:     make(map[string]peer),
	}
	go r.run()
	return &r
}

func (r *registrar) createNewPID(name string) etf.Pid {
	i := atomic.AddUint32(&r.nextPID, 1)
	return etf.Pid{
		Node:     etf.Atom(r.nodeName),
		Id:       i,
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
					p.err <- fmt.Errorf("name is taken")
					continue
				}
				r.names[p.name] = p.process.self
			}

			r.processes[p.process.self] = p.process
			p.err <- nil

		case up := <-r.channels.unregisterProcess:
			if p, ok := r.processes[up]; ok {
				lib.Log("REGISTRAR unregistering process: %v", p.self)
				delete(r.processes, up)
				if (p.name) != "" {
					lib.Log("REGISTRAR unregistering name (%v): %s", p.self, p.name)
					delete(r.names, p.name)
				}
			}

		case n := <-r.channels.name:
			lib.Log("registering name %v", n)
			if _, ok := r.names[n.name]; ok {
				// already registered
				continue
			}
			r.names[n.name] = n.pid

		case un := <-r.channels.unregisterName:
			lib.Log("unregistering name %v", un)
			delete(r.names, un)

		case p := <-r.channels.peer:
			lib.Log("[%s] registering peer %v", r.node.FullName, p)
			if _, ok := r.peers[p.name]; ok {
				// already registered
				p.err <- fmt.Errorf("name is taken")
				continue
			}
			r.peers[p.name] = p.peer
			p.err <- nil

		case up := <-r.channels.unregisterPeer:
			lib.Log("unregistering peer %v", up)
			r.node.monitor.NodeDown(up)
			delete(r.peers, up)

		case <-r.node.context.Done():
			lib.Log("Finalizing registrar for %s (total number of processes: %d)", r.nodeName, len(r.processes))
			// FIXME: now its just call Stop function for
			// every single process. should we do that for the gen_servers
			// are running under supervisor?
			for _, p := range r.processes {
				lib.Log("FIN: %#v", p.name)
				p.Stop("normal")
			}
			return

		case bp := <-r.channels.routeByPid:
			lib.Log("sending message by pid %v", bp.pid)
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
						lib.Log("can't connect to %v: %s", bp.pid.Node, err)
					}

					bp.retries++
					r.channels.routeByPid <- bp
				}()
				continue
			}
			peer.send <- []etf.Term{etf.Tuple{REG_SEND, bp.from, etf.Atom(""), bp.pid}, bp.message}

		case bn := <-r.channels.routeByName:
			lib.Log("sending message by name %v", bn.name)
			if pid, ok := r.names[bn.name]; ok {
				r.route(bn.from, pid, bn.message)
			}

		case bt := <-r.channels.routeByTuple:
			lib.Log("sending message by tuple %v", bt.tuple)
			if bt.retries > 2 {
				// drop this message after 3 attempts to deliver this message
				continue
			}

			to_node := bt.tuple.Element(2).(string)
			to_process_name := bt.tuple.Element(1)
			if to_node == r.nodeName {
				r.route(bt.from, to_process_name, bt.message)
				continue
			}

			peer, ok := r.peers[to_node]
			if !ok {
				// initiate connection and make yet another attempt to deliver this message
				go func() {
					r.node.connect(etf.Atom(to_node))
					bt.retries++
					r.channels.routeByTuple <- bt
				}()

				continue
			}
			peer.send <- []etf.Term{etf.Tuple{REG_SEND, bt.from, etf.Atom(""), to_process_name}, bt.message}

		case rw := <-r.channels.routeRaw:
			if rw.retries > 2 {
				// drop this message after 3 attempts to deliver this message
				continue
			}

			peer, ok := r.peers[rw.nodename]
			if !ok {
				// initiate connection and make yet another attempt to deliver this message
				go func() {
					if err := r.node.connect(etf.Atom(rw.nodename)); err != nil {
						lib.Log("can't connect to %v: %s", rw.nodename, err)
					}

					rw.retries++
					r.channels.routeRaw <- rw
				}()

				continue
			}

			peer.send <- []etf.Term{rw.message}
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

	mailbox_size := DefaultProcessMailboxSize
	if opts.MailboxSize > 0 {
		mailbox_size = int(opts.MailboxSize)
	}

	ctx, stop := context.WithCancel(r.node.context)
	pid := r.createNewPID(r.nodeName)

	wrapped_stop := func(reason string) {
		lib.Log("STOPPING: %#v with reason: %s", pid, reason)
		stop()
		r.UnregisterProcess(pid)
		r.node.monitor.ProcessTerminated(pid, etf.Atom(name), reason)
	}

	process := &Process{
		mailBox: make(chan etf.Tuple, mailbox_size),
		ready:   make(chan bool),
		self:    pid,
		Context: ctx,
		Stop:    wrapped_stop,
		name:    name,
		Node:    r.node,
		reply:   make(chan etf.Tuple, 2),
		object:  object,
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
func (r *registrar) RegisterName(name string, pid etf.Pid) {
	req := registerNameRequest{name: name, pid: pid}
	r.channels.name <- req
}

// UnregisterName unregister named process
func (r *registrar) UnregisterName(name string) {
	r.channels.unregisterName <- name
}

func (r *registrar) RegisterPeer(name string, p peer) error {
	req := registerPeer{
		name: name,
		peer: p,
		err:  make(chan error),
	}
	r.channels.peer <- req
	return <-req.err
}

func (r *registrar) UnregisterPeer(name string) {
	r.channels.unregisterPeer <- name
}

func (r *registrar) GetProcessByPid(pid etf.Pid) *Process {
	// TODO: implement it
	return nil
}

// route incomming message to registered process
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
		lib.Log("unknow sender type %#v", tto)
	}
}

func (r *registrar) routeRaw(nodename etf.Atom, message etf.Term) {
	req := routeRawRequest{
		nodename: string(nodename),
		message:  message,
	}
	r.channels.routeRaw <- req
}
