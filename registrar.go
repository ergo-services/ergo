package ergonode

import (
	"context"

	"github.com/halturin/ergonode/etf"
)

const (
	startPID = 1000
)

type registerProcessRequest struct {
	replyTo chan etf.Pid
	context context.Context
}

type registerNameRequest struct {
	name etf.Atom
	pid  etf.Pid
}

type registrarChannels struct {
	Process           chan registerProcessRequest
	UnregisterProcess chan etf.Pid
	Name              chan registerNameRequest
	UnregisterName    chan etf.Atom
}

type Registrar struct {
	nextPID     uint64
	nodeContext context.Context
}

func createRegistrar(ctx context.Context) *Registrar {
	r := Registrar{
		nextPID:     startPID,
		nodeContext: ctx,
	}
	return &r
}

func (r *Registrar) RegisterProcess(channels processChannels) {

}

func (n *Node) storeProcess(chs processChannels) (pid etf.Pid) {
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

// Registered returns a list of names which have been registered using Register
func (n *Node) RegisteredNames() (pids []etf.Atom) {
	pids = make([]etf.Atom, len(n.registered))
	i := 0
	for p, _ := range n.registered {
		pids[i] = p
		i++
	}
	return
}

func createRegistrar() {

}
