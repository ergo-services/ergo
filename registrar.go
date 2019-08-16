package ergonode

import (
	"context"

	"github.com/halturin/ergonode/etf"
	"github.com/halturin/ergonode/lib"
)

const (
	startPID = 1000
)

type registerProcessRequest struct {
	process interface{}
	context context.Context
}

type registerNameRequest struct {
	name etf.Atom
	pid  etf.Pid
}

type registrarChannels struct {
	process           chan registerProcessRequest
	unregisterProcess chan etf.Pid
	name              chan registerNameRequest
	unregisterName    chan etf.Atom
	reply             chan etf.Pid
}

type registrar struct {
	nextPID     uint32
	nodeName    string
	creation    byte
	nodeContext context.Context
	channels    registrarChannels

	names   map[etf.Atom]etf.Pid
	process map[etf.Pid]interface{}
}

func createRegistrar(ctx context.Context, nodename string) *registrar {
	r := registrar{
		nextPID:     startPID,
		nodeName:    nodename,
		creation:    byte(1),
		nodeContext: ctx,
		channels: registrarChannels{
			process:           make(chan registerProcessRequest),
			unregisterProcess: make(chan etf.Pid),
			name:              make(chan registerNameRequest),
			unregisterName:    make(chan etf.Atom),
			reply:             make(chan etf.Pid),
		},
	}
	go r.run()
	return &r
}

func (r *registrar) run() {
	defer func() {
		close(r.channels.process)
		close(r.channels.unregisterProcess)
		close(r.channels.name)
		close(r.channels.unregisterName)
		close(r.channels.reply)
	}()

	for {
		select {
		case p := <-r.channels.process:
			var pid etf.Pid
			pid.Node = etf.Atom(r.nodeName)
			pid.Id = r.nextPID
			pid.Serial = 1
			pid.Creation = byte(r.creation)
			opts := p.process.(ProcessBehaviour).Options()
			mailbox_size := DefaultProcessMailboxSize
			if size, ok := opts["mailbox-size"]; ok {
				mailbox_size = size.(int)
			}
			p.process.(Process).local = make(chan etf.Term, mailbox_size)
			p.process.(Process).remote = make(chan etf.Tuple, mailbox_size)
			p.process.(Process).ready = make(chan bool)
			p.process.(Process).self = pid
			r.channels.reply <- pid

			r.nextPID++

		case up := <-r.channels.unregisterProcess:
			lib.Log("unregistering process %v", up)

		case n := <-r.channels.name:
			lib.Log("registering name %v", n)

		case un := <-r.channels.unregisterName:
			lib.Log("unregistering name %v", un)

		case <-r.nodeContext.Done():
			lib.Log("Finalizing registrar for %s", r.nodeName)
			return
		}
	}
}

func (r *registrar) RegisterProcess(ctx context.Context, object interface{}) etf.Pid {
	req := registerProcessRequest{
		process: object,
		context: ctx,
	}
	r.channels.process <- req

	select {
	case pid := <-r.channels.reply:
		return pid
	}

}

// Register associates the name with pid
func (r *registrar) RegisterName(name etf.Atom, pid etf.Pid) {
	req := registerNameRequest{name: name, pid: pid}
	r.channels.name <- req
}

// // Unregister removes the registered name
// func (n *Node) Unregister(name etf.Atom) {
// 	r := unregNameReq{name: name}
// 	n.registry.unregNameChan <- r
// }

// // Registered returns a list of names which have been registered using Register
// func (n *Node) Registered() (pids []etf.Atom) {
// 	pids = make([]etf.Atom, len(n.registered))
// 	i := 0
// 	for p, _ := range n.registered {
// 		pids[i] = p
// 		i++
// 	}
// 	return
// }

// func (n *Node) registrator() {
// 	for {
// 		select {
// 		case req := <-n.registry.storeChan:
// 			var pid etf.Pid
// 			pid.Node = etf.Atom(n.FullName)
// 			pid.Id = n.getProcID()
// 			pid.Serial = 1
// 			pid.Creation = byte(n.Creation)

// 			n.channels[pid] = req.channels
// 			req.replyTo <- pid
// 		case req := <-n.registry.regNameChan:
// 			n.registered[req.name] = req.pid
// 		case req := <-n.registry.unregNameChan:
// 			delete(n.registered, req.name)
// 		}
// 	}
// }

// func (n *Node) storeProcess(chs procChannels) (pid etf.Pid) {
// 	myChan := make(chan etf.Pid)
// 	n.registry.storeChan <- regReq{replyTo: myChan, channels: chs}
// 	pid = <-myChan
// 	return pid
// }

// func (n *Node) getProcID() (s uint32) {

// 	n.lock.Lock()
// 	defer n.lock.Unlock()

// 	s = n.procID
// 	n.procID += 1
// 	return
// }

// // Registered returns a list of names which have been registered using Register
// func (n *Node) RegisteredNames() (pids []etf.Atom) {
// 	pids = make([]etf.Atom, len(n.registered))
// 	i := 0
// 	for p, _ := range n.registered {
// 		pids[i] = p
// 		i++
// 	}
// 	return
// }

// func createRegistrar() {

// }
