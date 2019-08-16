// Copyright 2012-2013 Metachord Ltd.
// All rights reserved.
// Use of this source code is governed by a MIT license
// that can be found in the LICENSE file.

package ergonode

import (
	"context"
	"errors"
	"fmt"

	"github.com/halturin/ergonode/dist"
	"github.com/halturin/ergonode/etf"
	"github.com/halturin/ergonode/lib"

	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Node struct {
	dist.EPMD
	listener net.Listener
	Cookie   string

	registrar *registrar
	peers     map[etf.Atom]nodepeer
	system    systemProcesses
	monitors  map[etf.Atom][]etf.Pid // node monitors
	monitorsP map[etf.Pid][]etf.Pid  // process monitors
	procID    uint64
	lock      sync.Mutex
	context   context.Context
	cancel    context.CancelFunc
}

type NodeOptions struct {
	ListenRangeBegin uint16
	ListenRangeEnd   uint16
	Hidden           bool
	EPMDPort         uint16
}

const (
	defaultListenRangeBegin uint16 = 15000
	defaultListenRangeEnd   uint16 = 65000
	defaultEPMDPort         uint16 = 4369
)

// CreateNode create new node with name and cookie string
func CreateNode(name string, cookie string, opts NodeOptions) *Node {
	return CreateNodeWithContext(context.Background(), name, cookie, opts)
}

// CreateNodeWithContext create new node with specified context, name and cookie string
func CreateNodeWithContext(ctx context.Context, name string, cookie string, opts NodeOptions) *Node {

	lib.Log("Start with name '%s' and cookie '%s'", name, cookie)

	node := Node{
		Cookie:    cookie,
		peers:     make(map[etf.Atom]nodepeer),
		monitors:  make(map[etf.Atom][]etf.Pid),
		monitorsP: make(map[etf.Pid][]etf.Pid),
		context:   ctx,
	}

	// start networking if name is defined
	if name != "" {
		// set defaults
		if opts.ListenRangeBegin == 0 {
			opts.ListenRangeBegin = defaultListenRangeBegin
		}
		if opts.ListenRangeEnd == 0 {
			opts.ListenRangeEnd = defaultListenRangeEnd
		}
		lib.Log("Listening range: %d...%d", opts.ListenRangeBegin, opts.ListenRangeEnd)

		if opts.EPMDPort == 0 {
			opts.EPMDPort = defaultEPMDPort
		}
		if opts.EPMDPort != 4369 {
			lib.Log("Using custom EPMD port: %d", opts.EPMDPort)
		}

		if opts.Hidden {
			lib.Log("Running as hidden node")
		}
		ns := strings.Split(name, "@")
		if len(ns) != 2 {
			panic("FQDN for node name is required (example: node@hostname)")
		}

		if listenPort := node.listen(name, opts.ListenRangeBegin, opts.ListenRangeEnd); listenPort == 0 {
			panic("Can't listen port")
		} else {
			// start EPMD
			node.EPMD.Init(name, listenPort, opts.EPMDPort, opts.Hidden)
		}

	}

	node.registrar = createRegistrar(node.context, node.FullName)

	// starting system processes
	node.system.netKernel = new(netKernel)
	node.Spawn(node.system.netKernel)

	node.system.globalNameServer = new(globalNameServer)
	node.Spawn(node.system.globalNameServer)

	node.system.rpc = new(rpc)
	node.Spawn(node.system.rpc)

	node.system.observer = new(observer)
	node.Spawn(node.system.observer)

	return &node
}

// Spawn create new process and store its identificator in table at current node
func (n *Node) Spawn(object interface{}, args ...interface{}) (pid etf.Pid) {
	n.registrar.RegisterProcess(context.Background(), object)
	go object.(ProcessBehaviour).ProcessLoop(object, args...)
	<-object.(Process).ready
	return
}

func (n *Node) Register(name etf.Atom, pid etf.Pid) {
	n.registrar.RegisterName(name, pid)
}

func (n *Node) serve(c net.Conn, negotiate bool) {

	var currNd *dist.NodeDesc

	if negotiate {
		currNd = dist.NewNodeDesc(n.FullName, n.Cookie, false, c)
	} else {
		currNd = dist.NewNodeDesc(n.FullName, n.Cookie, false, nil)
	}

	wchan := make(chan []etf.Term, 10)
	// run writer routine
	go func() {
		for {
			terms := <-wchan
			err := currNd.WriteMessage(c, terms)
			if err != nil {
				lib.Log("Enode error (writing): %s", err.Error())
				break
			}
		}
		c.Close()
		n.lock.Lock()
		// n.handle_monitors_node(currNd.GetRemoteName())
		delete(n.peers, currNd.GetRemoteName())
		n.lock.Unlock()
	}()

	go func() {
		for {
			terms, err := currNd.ReadMessage(c)
			if err != nil {
				lib.Log("Enode error (reading): %s", err.Error())
				break
			}
			n.handleTerms(c, wchan, terms)
		}
		c.Close()
		n.lock.Lock()
		// n.handle_monitors_node(currNd.GetRemoteName())
		delete(n.peers, currNd.GetRemoteName())
		n.lock.Unlock()
	}()

	<-currNd.Ready

	return
}

func (n *Node) handleTerms(c net.Conn, wchan chan []etf.Term, terms []etf.Term) {
	lib.Log("Node terms: %#v", terms)

	if len(terms) == 0 {
		return
	}
	switch t := terms[0].(type) {
	case etf.Tuple:
		if len(t) > 0 {
			switch act := t.Element(1).(type) {
			case int:
				switch act {
				case REG_SEND:
					if len(terms) == 2 {
						n.route(t.Element(2), t.Element(4), terms[1])
					} else {
						lib.Log("*** ERROR: bad REG_SEND: %#v", terms)
					}
				case SEND:
					n.route(nil, t.Element(3), terms[1])

				// Not implemented yet, just stubs. TODO.
				case LINK:
					lib.Log("LINK message (act %d): %#v", act, t)
				case UNLINK:
					lib.Log("UNLINK message (act %d): %#v", act, t)
				case NODE_LINK:
					lib.Log("NODE_LINK message (act %d): %#v", act, t)
				case EXIT:
					lib.Log("EXIT message (act %d): %#v", act, t)
				case EXIT2:
					lib.Log("EXIT2 message (act %d): %#v", act, t)
				case MONITOR:
					lib.Log("MONITOR message (act %d): %#v", act, t)
				case DEMONITOR:
					lib.Log("DEMONITOR message (act %d): %#v", act, t)
				case MONITOR_EXIT:
					lib.Log("MONITOR_EXIT message (act %d): %#v", act, t)

					// {'DOWN',#Ref<0.0.13893633.237772>,process,<26194.4.1>,reason}
					M := etf.Term(etf.Tuple{etf.Atom("DOWN"),
						t.Element(3), etf.Atom("process"),
						t.Element(2), t.Element(5)})

					n.route(t.Element(2), t.Element(3), M)

				// Not implemented yet, just stubs. TODO.
				case SEND_SENDER:
					lib.Log("SEND_SENDER message (act %d): %#v", act)
				case SEND_SENDER_TT:
					lib.Log("SEND_SENDER_TT message (act %d): %#v", act)
				case PAYLOAD_EXIT:
					lib.Log("PAYLOAD_EXIT message (act %d): %#v", act)
				case PAYLOAD_EXIT_TT:
					lib.Log("PAYLOAD_EXIT_TT message (act %d): %#v", act)
				case PAYLOAD_EXIT2:
					lib.Log("PAYLOAD_EXIT2 message (act %d): %#v", act)
				case PAYLOAD_EXIT2_TT:
					lib.Log("PAYLOAD_EXIT2_TT message (act %d): %#v", act)
				case PAYLOAD_MONITOR_P_EXIT:
					lib.Log("PAYLOAD_MONITOR_P_EXIT message (act %d): %#v", act)

				default:
					lib.Log("Unhandled node message (act %d): %#v", act, t)
				}
			case etf.Atom:
				switch act {
				case etf.Atom("$connection"):
					lib.Log("SET NODE %#v", t)
					n.lock.Lock()
					n.peers[t[1].(etf.Atom)] = nodepeer{conn: c, wchan: wchan}
					n.lock.Unlock()

					// currNd.Ready channel waiting for registration of this connection
					ready := (t[2]).(chan bool)
					ready <- true
				}
			default:
				lib.Log("UNHANDLED ACT: %#v", t.Element(1))
			}
		}
	}
}

// route incomming message to registered (with sender 'from' value)
func (n *Node) route(from, to etf.Term, message etf.Term) {
	// var toPid etf.Pid
	// switch tp := to.(type) {
	// case etf.Pid:
	// 	toPid = tp
	// case etf.Atom:
	// 	toPid, _ = n.registered[tp]
	// }
	// pcs := n.channels[toPid]
	// if from == nil {
	// 	lib.Log("SEND: To: %#v, Message: %#v", to, message)
	// 	pcs.in <- message
	// } else {
	// 	lib.Log("REG_SEND: (%#v )From: %#v, To: %#v, Message: %#v", pcs.inFrom, from, to, message)
	// 	pcs.inFrom <- etf.Tuple{from, message}
	// }
}

// Send making outgoing message
func (n *Node) Send(from interface{}, to interface{}, message *etf.Term) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New(fmt.Sprint(r))
		}
	}()

	switch tto := to.(type) {
	case etf.Pid:
		n.sendbyPid(tto, message)
	case etf.Tuple:
		if len(tto) == 2 {
			// causes panic if casting to etf.Atom goes wrong
			if tto[0].(etf.Atom) == tto[1].(etf.Atom) {
				// just stub.
			}
			n.sendbyTuple(from.(etf.Pid), tto, message)
		}
	}

	return nil
}

func (n *Node) sendbyPid(to etf.Pid, message *etf.Term) {
	// var conn nodepeer
	// var exists bool
	// lib.Log("Send (via PID): %#v, %#v", to, message)
	// if string(to.Node) == n.FullName {
	// 	lib.Log("Send to local node")
	// 	pcs := n.channels[to]
	// 	pcs.in <- *message
	// } else {

	// 	lib.Log("Send to remote node: %#v, %#v", to, n.peers[to.Node])

	// 	if conn, exists = n.peers[to.Node]; !exists {
	// 		lib.Log("Send (via PID): create new connection (%s)", to.Node)
	// 		if err := connect(n, to.Node); err != nil {
	// 			panic(err.Error())
	// 		}
	// 		conn, _ = n.peers[to.Node]
	// 	}

	// 	msg := []etf.Term{etf.Tuple{SEND, etf.Atom(""), to}, *message}
	// 	conn.wchan <- msg
	// }
}

func (n *Node) sendbyTuple(from etf.Pid, to etf.Tuple, message *etf.Term) {
	// var conn nodepeer
	// var exists bool
	// lib.Log("Send (via NAME): %#v, %#v", to, message)

	// // to = {processname, 'nodename@hostname'}

	// if conn, exists = n.peers[to[1].(etf.Atom)]; !exists {
	// 	lib.Log("Send (via NAME): create new connection (%s)", to[1])
	// 	if err := connect(n, to[1].(etf.Atom)); err != nil {
	// 		panic(err.Error())
	// 	}
	// 	conn, _ = n.peers[to[1].(etf.Atom)]
	// }

	// msg := []etf.Term{etf.Tuple{REG_SEND, from, etf.Atom(""), to[0]}, *message}
	// conn.wchan <- msg
}

// func (n *Node) Monitor(by etf.Pid, to etf.Pid) {
// 	var conn nodepeer
// 	var exists bool

// 	if string(to.Node) == n.FullName {
// 		lib.Log("Monitor local PID: %#v by %#v", to, by)

// 		pcs := n.channels[to]
// 		msg := []etf.Term{etf.Tuple{MONITOR, by, to, n.MakeRef()}}
// 		pcs.in <- msg

// 		return
// 	}

// 	lib.Log("Monitor remote PID: %#v by %#v", to, by)

// 	if conn, exists = n.peers[to.Node]; !exists {
// 		lib.Log("Send (via PID): create new connection (%s)", to.Node)
// 		if err := connect(n, to.Node); err != nil {
// 			panic(err.Error())
// 		}
// 		conn, _ = n.peers[to.Node]
// 	}

// 	msg := []etf.Term{etf.Tuple{MONITOR, by, to, n.MakeRef()}}
// 	conn.wchan <- msg
// }

// func (n *Node) MonitorNode(by etf.Pid, node etf.Atom, flag bool) {
// 	var exists bool
// 	var monitors []etf.Pid

// 	lib.Log("Monitor node: %#v by %#v", node, by)
// 	if _, exists = n.peers[node]; !exists {
// 		lib.Log("... connecting to %#v", node)
// 		if err := connect(n, node); err != nil {
// 			panic(err.Error())
// 		}
// 	}

// 	monitors = n.monitors[node]

// 	if !flag {
// 		lib.Log("... removing monitor: %#v by %#v", node, by)
// 		monitors = removePid(monitors, by)
// 	} else {
// 		lib.Log("... setting up monitor: %#v by %#v", node, by)
// 		// DUE TO...

// 		// http://erlang.org/doc/man/erlang.html#monitor_node-2
// 		// Making several calls to monitor_node(Node, true) for the same Node is not an error;
// 		// it results in as many independent monitoring instances.

// 		// DO NOT CHECK for existing this pid in the list, just add one more
// 		monitors = append(monitors, by)
// 	}

// 	n.monitors[node] = monitors
// 	lib.Log("Monitors for node (%#v): %#v", node, monitors)

// }

// func (n *Node) handle_monitors_node(node etf.Atom) {
// 	lib.Log("Node (%#v) is down. Send it to %#v", node, n.monitors[node])
// 	for _, pid := range n.monitors[node] {
// 		pcs := n.channels[pid]
// 		msg := etf.Term(etf.Tuple{etf.Atom("nodedown"), node})
// 		pcs.in <- msg
// 	}
// }

func (n *Node) MakeRef() (ref etf.Ref) {
	ref.Node = etf.Atom(n.FullName)
	ref.Creation = 1

	nt := time.Now().UnixNano()
	id1 := uint32(uint64(nt) & ((2 << 17) - 1))
	id2 := uint32(uint64(nt) >> 46)
	ref.Id = []uint32{id1, id2, 0}

	return
}

func connect(n *Node, to etf.Atom) error {

	var port int
	var err error

	if port, err = n.ResolvePort(string(to)); port < 0 {
		return fmt.Errorf("Can't resolve port: %s", err)
	}
	ns := strings.Split(string(to), "@")

	c, err := net.Dial("tcp", net.JoinHostPort(ns[1], strconv.Itoa(int(port))))
	if err != nil {
		lib.Log("Error calling net.Dial : %s", err.Error())
		return err
	}

	if tcp, ok := c.(*net.TCPConn); ok {
		tcp.SetKeepAlive(true)
	}

	n.serve(c, true)

	return nil
}

func removePid(pids []etf.Pid, pid etf.Pid) []etf.Pid {
	for i, p := range pids {
		if p == pid {
			return append(pids[:i], pids[i+1:]...)
		}
	}
	return pids
}

func (n *Node) listen(name string, listenRangeBegin, listenRangeEnd uint16) uint16 {
	for p := listenRangeBegin; p <= listenRangeEnd; p++ {
		l, err := net.Listen("tcp", net.JoinHostPort(name, strconv.Itoa(int(p))))
		if err != nil {
			continue
		}
		go func() {
			for {
				c, err := l.Accept()
				lib.Log("Accepted new connection from %s", c.RemoteAddr().String())
				if err != nil {
					lib.Log(err.Error())
				} else {
					n.serve(c, false)
				}
			}
		}()
		return p
	}

	return 0
}
