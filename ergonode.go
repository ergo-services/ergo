// Copyright 2012-2013 Metachord Ltd.
// All rights reserved.
// Use of this source code is governed by a MIT license
// that can be found in the LICENSE file.

package ergonode

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/halturin/ergonode/dist"
	"github.com/halturin/ergonode/etf"
)

var nTrace bool

func init() {
	flag.BoolVar(&nTrace, "trace.node", false, "trace node")
}

func nLog(f string, a ...interface{}) {
	if nTrace {
		log.Printf(f, a...)
	}
}

type regReq struct {
	replyTo  chan etf.Pid
	channels procChannels
}

type regNameReq struct {
	name etf.Atom
	pid  etf.Pid
}

type unregNameReq struct {
	name etf.Atom
}

type registryChan struct {
	storeChan     chan regReq
	regNameChan   chan regNameReq
	unregNameChan chan unregNameReq
}

type nodeConn struct {
	conn  net.Conn
	wchan chan []etf.Term
}

type systemProcs struct {
	netKernel        *netKernel
	globalNameServer *globalNameServer
	rpcRex           *rpcRex
}

type Node struct {
	EPMD
	epmdreply   chan interface{}
	Cookie      string
	registry    *registryChan
	channels    map[etf.Pid]procChannels
	registered  map[etf.Atom]etf.Pid
	connections map[etf.Atom]nodeConn
	sysProcs    systemProcs
	monitors    map[etf.Atom][]etf.Pid // node monitors
	monitorsP   map[etf.Pid][]etf.Pid  // process monitors
	procID      uint32
	lock        sync.Mutex
}

type procChannels struct {
	in     chan etf.Term
	inFrom chan etf.Tuple
	init   chan bool
}

// Behaviour interface contains methods you should implement to make own process behaviour
type Behaviour interface {
	ProcessLoop(pcs procChannels, pd Process, args ...interface{}) // method which implements control flow of process
}

// Process interface contains methods which should be implemented in each process
type Process interface {
	Options() (options map[string]interface{}) // method returns process-related options
	setNode(node *Node)                        // method set pointer to Node structure
	setPid(pid etf.Pid)                        // method set pid of started process
}

// NodeArgs must be implemented by types that can constitute create-time parameters
// for a *Node.
type NodeArgs interface {
	Type() string
}

// EPMDArgs allows passing custom host (or ip address) and port number when trying to
// connect to the EPMD daemon. Please note, that the Erlang specification requires
// the daemon to reside on the same machine as the actual node.
type EPMDArgs struct {
	Host string
	Port uint16
}

func (*EPMDArgs) Type() string { return "EPMDArgs" }

func NewEPMDArgs(host string, port uint16) *EPMDArgs {
	return &EPMDArgs{Host: host, Port: port}
}

// Create creates a new node context with specified name, cookie string, and
// additional (optional) connectivity args. The optional args are useful for cases
// such as at least one local EPMD daemon listening on a non-standard local ip and port,
// to allow multiple, discrete node clusters (e.g. one on 4369, another on 4370).
// When no optional nodeArgs are provided, the empd connectivity defaults to
// 127.0.0.1 / 4369
//
// Example (create a gen_server listening on port 15515, relying on an epmd daemon at 4370):
//	epmdArgs := ergonode.NewEPMDArgs("192.168.1.10", 4370)
//	n := ergonode.Create("n1@192.168.1.10, 15515, "123", epmdArgs)
//
func Create(name string, port uint16, cookie string, nodeArgs ...NodeArgs) (node *Node) {
	nLog("Start with name '%s' and cookie '%s'", name, cookie)

	registry := &registryChan{
		storeChan:     make(chan regReq),
		regNameChan:   make(chan regNameReq),
		unregNameChan: make(chan unregNameReq),
	}

	epmd := EPMD{}

	epmd.Init(name, port, nodeArgs...)

	node = &Node{
		EPMD:        epmd,
		Cookie:      cookie,
		registry:    registry,
		channels:    make(map[etf.Pid]procChannels),
		registered:  make(map[etf.Atom]etf.Pid),
		connections: make(map[etf.Atom]nodeConn),
		monitors:    make(map[etf.Atom][]etf.Pid),
		monitorsP:   make(map[etf.Pid][]etf.Pid),
		procID:      1,
	}

	l, err := net.Listen("tcp", net.JoinHostPort("", strconv.Itoa(int(port))))
	if err != nil {
		panic(err.Error())
	}

	go func() {
		for {
			c, err := l.Accept()
			nLog("Accepted new connection from %s", c.RemoteAddr().String())
			if err != nil {
				nLog(err.Error())
			} else {
				wchan := make(chan []etf.Term, 10)
				node.run(c, wchan, false)
			}
		}
	}()

	go node.registrator()

	node.sysProcs.netKernel = new(netKernel)
	node.Spawn(node.sysProcs.netKernel)

	node.sysProcs.globalNameServer = new(globalNameServer)
	node.Spawn(node.sysProcs.globalNameServer)

	node.sysProcs.rpcRex = new(rpcRex)
	node.Spawn(node.sysProcs.rpcRex)

	return node
}

// Spawn create new process and store its identificator in table at current node
func (n *Node) Spawn(pd Process, args ...interface{}) (pid etf.Pid) {
	options := pd.Options()
	chanSize, ok := options["chan-size"].(int)
	if !ok {
		chanSize = 100
	}

	in := make(chan etf.Term, chanSize)
	inFrom := make(chan etf.Tuple, chanSize)
	initCh := make(chan bool)
	pcs := procChannels{
		in:     in,
		inFrom: inFrom,
		init:   initCh,
	}
	pid = n.storeProcess(pcs)
	pd.setNode(n)
	pd.setPid(pid)
	go pd.(Behaviour).ProcessLoop(pcs, pd, args...)
	<-initCh
	return
}

// Register associates the name with pid
func (n *Node) Register(name etf.Atom, pid etf.Pid) {
	r := regNameReq{name: name, pid: pid}
	n.registry.regNameChan <- r
}

// Unregister removes the registered name
func (n *Node) Unregister(name etf.Atom) {
	r := unregNameReq{name: name}
	n.registry.unregNameChan <- r
}

// Registered returns a list of names which have been registered using Register
func (n *Node) Registered() (pids []etf.Atom) {
	pids = make([]etf.Atom, len(n.registered))
	i := 0
	for p, _ := range n.registered {
		pids[i] = p
		i++
	}
	return
}

func (n *Node) registrator() {
	for {
		select {
		case req := <-n.registry.storeChan:
			var pid etf.Pid
			pid.Node = etf.Atom(n.FullName)
			pid.Id = n.getProcID()
			pid.Serial = 1
			pid.Creation = byte(n.Creation)

			n.channels[pid] = req.channels
			req.replyTo <- pid
		case req := <-n.registry.regNameChan:
			n.registered[req.name] = req.pid
		case req := <-n.registry.unregNameChan:
			delete(n.registered, req.name)
		}
	}
}

func (n *Node) storeProcess(chs procChannels) (pid etf.Pid) {
	myChan := make(chan etf.Pid)
	n.registry.storeChan <- regReq{replyTo: myChan, channels: chs}
	pid = <-myChan
	return pid
}

func (n *Node) getProcID() (s uint32) {

	n.lock.Lock()
	defer n.lock.Unlock()

	s = n.procID
	n.procID += 1
	return
}

func (n *Node) run(c net.Conn, wchan chan []etf.Term, negotiate bool) {

	var currNd *dist.NodeDesc

	if negotiate {
		currNd = dist.NewNodeDesc(n.FullName, n.Cookie, false, c)
	} else {
		currNd = dist.NewNodeDesc(n.FullName, n.Cookie, false, nil)
	}

	// run writer routine
	go func() {
		for {
			terms := <-wchan
			err := currNd.WriteMessage(c, terms)
			if err != nil {
				nLog("Enode error (writing): %s", err.Error())
				break
			}
		}
		c.Close()
		n.lock.Lock()
		n.handle_monitors_node(currNd.GetRemoteName())
		delete(n.connections, currNd.GetRemoteName())
		n.lock.Unlock()
	}()

	go func() {
		for {
			terms, err := currNd.ReadMessage(c)
			if err != nil {
				nLog("Enode error (reading): %s", err.Error())
				break
			}
			n.handleTerms(c, wchan, terms)
		}
		c.Close()
		n.lock.Lock()
		n.handle_monitors_node(currNd.GetRemoteName())
		delete(n.connections, currNd.GetRemoteName())
		n.lock.Unlock()
	}()

	<-currNd.Ready

	return
}

func (n *Node) handleTerms(c net.Conn, wchan chan []etf.Term, terms []etf.Term) {
	nLog("Node terms: %#v", terms)

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
						nLog("*** ERROR: bad REG_SEND: %#v", terms)
					}
				case SEND:
					n.route(nil, t.Element(3), terms[1])

				// Not implemented yet, just stubs. TODO.
				case LINK:
					nLog("LINK message (act %d): %#v", act, t)
				case UNLINK:
					nLog("UNLINK message (act %d): %#v", act, t)
				case NODE_LINK:
					nLog("NODE_LINK message (act %d): %#v", act, t)
				case EXIT:
					nLog("EXIT message (act %d): %#v", act, t)
				case EXIT2:
					nLog("EXIT2 message (act %d): %#v", act, t)
				case MONITOR:
					nLog("MONITOR message (act %d): %#v", act, t)
				case DEMONITOR:
					nLog("DEMONITOR message (act %d): %#v", act, t)
				case MONITOR_EXIT:
					nLog("MONITOR_EXIT message (act %d): %#v", act, t)

					// {'DOWN',#Ref<0.0.13893633.237772>,process,<26194.4.1>,reason}
					M := etf.Term(etf.Tuple{etf.Atom("DOWN"),
						t.Element(3), etf.Atom("process"),
						t.Element(2), t.Element(5)})

					n.route(t.Element(2), t.Element(3), M)

				default:
					nLog("Unhandled node message (act %d): %#v", act, t)
				}
			case etf.Atom:
				switch act {
				case etf.Atom("$connection"):
					nLog("SET NODE %#v", t)
					n.lock.Lock()
					n.connections[t[1].(etf.Atom)] = nodeConn{conn: c, wchan: wchan}
					n.lock.Unlock()

					// currNd.Ready channel waiting for registration of this connection
					ready := (t[2]).(chan bool)
					ready <- true
				}
			default:
				nLog("UNHANDLED ACT: %#v", t.Element(1))
			}
		}
	}
}

// route incomming message to registered (with sender 'from' value)
func (n *Node) route(from, to etf.Term, message etf.Term) {
	var toPid etf.Pid
	switch tp := to.(type) {
	case etf.Pid:
		toPid = tp
	case etf.Atom:
		toPid, _ = n.registered[tp]
	}
	pcs := n.channels[toPid]
	if from == nil {
		nLog("SEND: To: %#v, Message: %#v", to, message)
		pcs.in <- message
	} else {
		nLog("REG_SEND: (%#v )From: %#v, To: %#v, Message: %#v", pcs.inFrom, from, to, message)
		pcs.inFrom <- etf.Tuple{from, message}
	}
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
	var conn nodeConn
	var exists bool
	nLog("Send (via PID): %#v, %#v", to, message)
	if string(to.Node) == n.FullName {
		nLog("Send to local node")
		pcs := n.channels[to]
		pcs.in <- message
	} else {

		nLog("Send to remote node: %#v, %#v", to, n.connections[to.Node])

		if conn, exists = n.connections[to.Node]; !exists {
			nLog("Send (via PID): create new connection (%s)", to.Node)
			if err := connect(n, to.Node); err != nil {
				panic(err.Error())
			}
			conn, _ = n.connections[to.Node]
		}

		msg := []etf.Term{etf.Tuple{SEND, etf.Atom(""), to}, *message}
		conn.wchan <- msg
	}
}

func (n *Node) sendbyTuple(from etf.Pid, to etf.Tuple, message *etf.Term) {
	var conn nodeConn
	var exists bool
	nLog("Send (via NAME): %#v, %#v", to, message)

	// to = {processname, 'nodename@hostname'}

	if conn, exists = n.connections[to[1].(etf.Atom)]; !exists {
		nLog("Send (via NAME): create new connection (%s)", to[1])
		if err := connect(n, to[1].(etf.Atom)); err != nil {
			panic(err.Error())
		}
		conn, _ = n.connections[to[1].(etf.Atom)]
	}

	msg := []etf.Term{etf.Tuple{REG_SEND, from, etf.Atom(""), to[0]}, *message}
	conn.wchan <- msg

	return
}

func (n *Node) Monitor(by etf.Pid, to etf.Pid) {
	var conn nodeConn
	var exists bool

	if string(to.Node) == n.FullName {
		nLog("Monitor local PID: %#v by %#v", to, by)

		pcs := n.channels[to]
		msg := []etf.Term{etf.Tuple{MONITOR, by, to, n.MakeRef()}}
		pcs.in <- msg

		return
	}

	nLog("Monitor remote PID: %#v by %#v", to, by)

	if conn, exists = n.connections[to.Node]; !exists {
		nLog("Send (via PID): create new connection (%s)", to.Node)
		if err := connect(n, to.Node); err != nil {
			panic(err.Error())
		}
		conn, _ = n.connections[to.Node]
	}

	msg := []etf.Term{etf.Tuple{MONITOR, by, to, n.MakeRef()}}
	conn.wchan <- msg
}

func (n *Node) MonitorNode(by etf.Pid, node etf.Atom, flag bool) {
	var exists bool
	var monitors []etf.Pid

	nLog("Monitor node: %#v by %#v", node, by)
	if _, exists = n.connections[node]; !exists {
		nLog("... connecting to %#v", node)
		if err := connect(n, node); err != nil {
			panic(err.Error())
		}
	}

	monitors = n.monitors[node]

	if !flag {
		nLog("... removing monitor: %#v by %#v", node, by)
		monitors = removePid(monitors, by)
	} else {
		nLog("... setting up monitor: %#v by %#v", node, by)
		// DUE TO...

		// http://erlang.org/doc/man/erlang.html#monitor_node-2
		// Making several calls to monitor_node(Node, true) for the same Node is not an error;
		// it results in as many independent monitoring instances.

		// DO NOT CHECK for existing this pid in the list, just add one more
		monitors = append(monitors, by)
	}

	n.monitors[node] = monitors
	nLog("Monitors for node (%#v): %#v", node, monitors)

}

func (n *Node) handle_monitors_node(node etf.Atom) {
	nLog("Node (%#v) is down. Send it to %#v", node, n.monitors[node])
	for _, pid := range n.monitors[node] {
		pcs := n.channels[pid]
		msg := etf.Term(etf.Tuple{etf.Atom("nodedown"), node})
		pcs.in <- msg
	}
}

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

	if port = n.ResolvePort(string(to)); port < 0 {
		return errors.New("Unknown node name")
	}
	ns := strings.Split(string(to), "@")

	nLog("CONNECT TO: %s %d\n", to, port)
	c, err := net.Dial("tcp", net.JoinHostPort(ns[1], strconv.Itoa(int(port))))
	if err != nil {
		nLog("Error calling net.Dial : %s", err.Error())
		return err
	}

	if tcp, ok := c.(*net.TCPConn); ok {
		tcp.SetKeepAlive(true)
	}

	wchan := make(chan []etf.Term, 10)
	n.run(c, wchan, true)

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
