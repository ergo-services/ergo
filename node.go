// Copyright 2012-2013 Metachord Ltd.
// All rights reserved.
// Use of this source code is governed by a MIT license
// that can be found in the LICENSE file.

/*
Package node provides interface for creation and publishing node using
Erlang distribution protocol: http://www.erlang.org/doc/apps/erts/erl_dist_protocol.html

The Publish function allows incoming connection to the current node on 5858 port.
EPMD will reply with this port on name request for this node name

	enode := node.NewNode(name, cookie)
	err := enode.Publish(5858)
	if err != nil {
		log.Printf("Cannot publish: %s", err)
		enode = nil
	}


Function Spawn creates new process from struct which implements Process interface:

	eSrv := new(eclusSrv)
	pid := enode.Spawn(eSrv)

Now you can call Register function to store this pid with arbitrary name:

	enode.Register(etf.Atom("eclus"), pid)

*/
package node

import (
	"errors"
	"flag"
	"fmt"
	"github.com/halturin/node/dist"
	"github.com/halturin/node/etf"
	"log"
	"net"
	"strconv"
	"strings"
)

var nTrace bool

func init() {
	flag.BoolVar(&nTrace, "erlang.node.trace", false, "trace erlang node")
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
}

type procChannels struct {
	in     chan etf.Term
	inFrom chan etf.Tuple
	ctl    chan etf.Term
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
	}

	l, err := net.Listen("tcp", net.JoinHostPort("", strconv.Itoa(int(port))))
	if err != nil {
		panic(err.Error())
	}

	go func() {
		for {
			conn, err := l.Accept()
			nLog("Accept new at ENode")
			if err != nil {
				nLog(err.Error())
			} else {
				wchan := make(chan []etf.Term, 10)
				ndchan := make(chan *dist.NodeDesc)
				go node.mLoopReader(conn, wchan, ndchan)
				go node.mLoopWriter(conn, wchan, ndchan)
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
	ctlChanSize, ok := options["chan-size"].(int)
	if !ok {
		chanSize = 100
	}
	in := make(chan etf.Term, chanSize)
	inFrom := make(chan etf.Tuple, chanSize)
	ctl := make(chan etf.Term, ctlChanSize)
	initCh := make(chan bool)
	pcs := procChannels{
		in:     in,
		inFrom: inFrom,
		ctl:    ctl,
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
			// FIXME: make proper allocation, now it just stub
			var id uint32 = 0
			for k, _ := range n.channels {
				if k.Id >= id {
					id = k.Id + 1
				}
			}
			var pid etf.Pid
			pid.Node = etf.Atom(n.FullName)
			pid.Id = id
			pid.Serial = 0 // FIXME
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

func (n *Node) mLoopReader(c net.Conn, wchan chan []etf.Term, ndchan chan *dist.NodeDesc) {

	currNd := dist.NewNodeDesc(n.FullName, n.Cookie, false)
	ndchan <- currNd
	for {
		terms, err := currNd.ReadMessage(c)
		if err != nil {
			nLog("Enode error (reading): %s", err.Error())
			break
		}
		n.handleTerms(c, wchan, terms)
	}
	c.Close()
}

func (n *Node) mLoopWriter(c net.Conn, wchan chan []etf.Term, ndchan chan *dist.NodeDesc) {

	currNd := <-ndchan

	for {
		terms := <-wchan
		err := currNd.WriteMessage(c, terms)
		if err != nil {
			nLog("Enode error (writing): %s", err.Error())
			break
		}
	}
	c.Close()
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
						n.RegSend(t.Element(2), t.Element(4), terms[1])
					} else {
						nLog("*** ERROR: bad REG_SEND: %#v", terms)
					}
				default:
					nLog("Unhandled node message (act %d): %#v", act, t)
				}
			case etf.Atom:
				switch act {
				case etf.Atom("$connection"):
					nLog("SET NODE %#v", t)
					n.connections[t[1].(etf.Atom)] = nodeConn{conn: c, wchan: wchan}
				}
			default:
				nLog("UNHANDLED ACT: %#v", t.Element(1))
			}
		}
	}
}

// RegSend sends message from one process to registered
func (n *Node) RegSend(from, to etf.Term, message etf.Term) {
	nLog("REG_SEND: From: %#v, To: %#v, Message: %#v", from, to, message)
	var toPid etf.Pid
	switch tp := to.(type) {
	case etf.Pid:
		toPid = tp
	case etf.Atom:
		toPid = n.Whereis(tp)
	}
	n.SendFrom(from, toPid, message)
}

// Whereis returns pid of registered process
func (n *Node) Whereis(who etf.Atom) (pid etf.Pid) {
	pid, _ = n.registered[who]
	return
}

// SendFrom sends message from source to destination
func (n *Node) SendFrom(from etf.Term, to etf.Pid, message etf.Term) {
	nLog("SendFrom: %#v, %#v, %#v", from, to, message)
	pcs := n.channels[to]
	pcs.inFrom <- etf.Tuple{from, message}
}

func (n *Node) Send(to interface{}, message etf.Term) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New(fmt.Sprint(r))
		}
	}()

	switch tto := to.(type) {
	case etf.Pid:
		sendPid(n, tto, message)
	case etf.Tuple:
		if len(tto) == 2 {
			// causes panic in case of
			if tto[0].(etf.Atom) == tto[1].(etf.Atom) {
				// just stub.
			}
			sendTuple(n, tto, message)
		}
	}

	return nil
}

// Send sends message to destination process withoud source
func sendPid(n *Node, to etf.Pid, message etf.Term) {
	nLog("Send (via PID): %#v, %#v", to, message)
	if string(to.Node) == n.FullName {
		nLog("Send to local node")
		pcs := n.channels[to]
		pcs.in <- message
	} else {
		nLog("Send to remote node: %#v, %#v", to, n.connections[to.Node])

		msg := []etf.Term{etf.Tuple{SEND, etf.Atom(""), to}, message}
		n.connections[to.Node].wchan <- msg
	}
}

func sendTuple(n *Node, to etf.Tuple, message etf.Term) {
	nLog("Send (via NAME): %#v, %#v", to, message)

	// to = {processname, 'nodename@hostname'}
	if conn, exists := n.connections[to[1].(etf.Atom)]; !exists {
		nLog("Send (via NAME): create new connection (%s)", to[1])
		if err := connect(n, to[1].(etf.Atom)); err != nil {
			panic(err.Error())
		}
	} else {
		nLog("Send (via NAME): use existing connection (%s)", to[1])
		msg := []etf.Term{etf.Tuple{SEND, etf.Atom(""), to}, message}
		conn.wchan <- msg
	}
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

	wchan := make(chan []etf.Term, 10)
	ndchan := make(chan *dist.NodeDesc)
	go n.mLoopReader(c, wchan, ndchan)
	go n.mLoopWriter(c, wchan, ndchan)

	n.connections[to] = nodeConn{conn: c, wchan: wchan}
	return nil
}
