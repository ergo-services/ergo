// Copyright 2012-2013 Metachord Ltd.
// All rights reserved.
// Use of this source code is governed by a MIT license
// that can be found in the LICENSE file.

package ergonode

import (
	"errors"
	"flag"
	"fmt"
	"github.com/halturin/ergonode/dist"
	"github.com/halturin/ergonode/etf"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
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

// Create create new node context with specified name and cookie string
func Create(name string, port uint16, cookie string) (node *Node) {
	nLog("Start with name '%s' and cookie '%s'", name, cookie)

	registry := &registryChan{
		storeChan:     make(chan regReq),
		regNameChan:   make(chan regNameReq),
		unregNameChan: make(chan unregNameReq),
	}

	epmd := EPMD{}
	epmd.Init(name, port)

	node = &Node{
		EPMD:        epmd,
		Cookie:      cookie,
		registry:    registry,
		channels:    make(map[etf.Pid]procChannels),
		registered:  make(map[etf.Atom]etf.Pid),
		connections: make(map[etf.Atom]nodeConn),
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

func connect(n *Node, to etf.Atom) error {

	var port int

	if port = n.ResolvePort(string(to)); port < 0 {
		return errors.New("Unknown node name")
	}
	ns := strings.Split(string(to), "@")

	fmt.Printf("CONNECT TO: %s %d\n", to, port)
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
