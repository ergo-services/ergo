package ergonode

import (
	"context"
	"errors"
	"fmt"

	"github.com/halturin/ergonode/etf"
)

type ProcessType = string

const (
	DefaultProcessMailboxSize = 100
	// Process type
	ProcessGenServer  = "gen_server"
	ProcessSupervisor = "supervisor"
)

type Process struct {
	local   chan etf.Term
	remote  chan etf.Tuple
	ready   chan bool
	self    etf.Pid
	context context.Context
	Stop    ProcessStopFunc
	name    string
	Node    *Node

	object interface{}
	state  interface{}
}

type ProcessStopFunc func(reason string)

// Behaviour interface contains methods you should implement to make own process behaviour
type ProcessBehaviour interface {
	loop(Process, interface{}, ...interface{}) // method which implements control flow of process
}

// Self returns self Pid
func (p *Process) Self() etf.Pid {
	return p.self
}

// Send making outgoing message
func (p *Process) Send(to interface{}, message etf.Term) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New(fmt.Sprint(r))
		}
	}()

	switch tto := to.(type) {
	case etf.Pid:
		p.sendbyPid(tto, message)
	case etf.Tuple:
		if len(tto) == 2 {
			// causes panic if casting to etf.Atom goes wrong
			if tto[0].(etf.Atom) == tto[1].(etf.Atom) {
				// just stub.
			}
			p.sendbyTuple(tto, message)
		}
	}

	return nil
}

func (p *Process) sendbyPid(to etf.Pid, message etf.Term) {
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

func (p *Process) sendbyTuple(to etf.Tuple, message etf.Term) {
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

	// msg := []etf.Term{etf.Tuple{REG_SEND, p.self, etf.Atom(""), to[0]}, *message}
	// conn.wchan <- msg
}

func (p *Process) MonitorProcess(to etf.Pid) etf.Ref {
	return p.Node.monitor.MonitorProcess(p.self, to)
}

func (p *Process) MonitorNode(name string) etf.Ref {
	return p.Node.monitor.MonitorNode(p.self, name)
}

func (p *Process) DemonitorProcess(ref etf.Ref) {
	p.Node.monitor.DemonitorProcess(ref)
}

func (p *Process) DemonitorNode(ref etf.Ref) {
	p.Node.monitor.DemonitorNode(ref)
}
