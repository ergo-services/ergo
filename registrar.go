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
	name    string
	object  interface{}
	context context.Context
}

type registerNameRequest struct {
	name string
	pid  etf.Pid
}

type registrarChannels struct {
	process           chan registerProcessRequest
	unregisterProcess chan etf.Pid
	name              chan registerNameRequest
	unregisterName    chan string
	reply             chan Process
}

type registrar struct {
	nextPID  uint32
	nodeName string
	creation byte

	nodeContext context.Context
	nodeStop    context.CancelFunc

	channels registrarChannels

	names     map[string]etf.Pid
	processes map[etf.Pid]Process
}

func createRegistrar(ctx context.Context, nodename string) *registrar {
	nodeCtx, nodeStop := context.WithCancel(ctx)
	r := registrar{
		nextPID:     startPID,
		nodeName:    nodename,
		creation:    byte(1),
		nodeContext: nodeCtx,
		nodeStop:    nodeStop,
		channels: registrarChannels{
			process:           make(chan registerProcessRequest),
			unregisterProcess: make(chan etf.Pid),
			name:              make(chan registerNameRequest),
			unregisterName:    make(chan string),
			reply:             make(chan Process),
		},

		names:     make(map[string]etf.Pid),
		processes: make(map[etf.Pid]Process),
	}
	go r.run()
	return &r
}

func (r *registrar) createNewPID(name string) etf.Pid {
	r.nextPID++
	return etf.Pid{
		Node:     etf.Atom(r.nodeName),
		Id:       r.nextPID,
		Serial:   1,
		Creation: byte(r.creation),
	}

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

			opts := p.object.(ProcessBehaviour).Options()
			mailbox_size := DefaultProcessMailboxSize
			if size, ok := opts["mailbox-size"]; ok {
				mailbox_size = size.(int)
			}
			ctx, stop := context.WithCancel(r.nodeContext)
			process := Process{
				local:   make(chan etf.Term, mailbox_size),
				remote:  make(chan etf.Tuple, mailbox_size),
				ready:   make(chan bool),
				self:    r.createNewPID(r.nodeName),
				context: ctx,
				stop:    stop,
				name:    p.name,
			}

			r.processes[process.self] = process
			if p.name != "" {
				r.names[p.name] = process.self
			}
			r.channels.reply <- process

			r.nextPID++

		case up := <-r.channels.unregisterProcess:
			lib.Log("unregistering process %v", up)

		case n := <-r.channels.name:
			lib.Log("registering name %v", n)

		case un := <-r.channels.unregisterName:
			lib.Log("unregistering name %v", un)

		case <-r.nodeContext.Done():
			lib.Log("Finalizing registrar for %s", r.nodeName)
			// FIXME: finalize all registered proceses
			return
		}
	}
}

func (r *registrar) RegisterProcess(object interface{}) Process {
	return r.RegisterProcessWithName("", object)
}

func (r *registrar) RegisterProcessWithName(name string, object interface{}) Process {
	req := registerProcessRequest{
		name:   name,
		object: object,
	}
	r.channels.process <- req

	select {
	case p := <-r.channels.reply:
		go func() {
			select {
			case <-p.context.Done():
				r.UnregisterProcess(p.self)
			}
		}()
		return p
	}

}

func (r *registrar) UnregisterProcess(pid etf.Pid) {
	if p, ok := r.processes[pid]; ok {
		lib.Log("unregistering process: %#v", p.self)
		close(p.local)
		close(p.remote)
		close(p.ready)
		delete(r.processes, pid)
		if (p.name) != "" {
			lib.Log("unregistering name (%#v): %s", p.self, p.name)
			delete(r.names, p.name)
		}
	}
}

// Register associates the name with pid
func (r *registrar) RegisterName(name string, pid etf.Pid) {
	req := registerNameRequest{name: name, pid: pid}
	r.channels.name <- req
}

func (r *registrar) UnregisterName(name string) {
	r.channels.unregisterName <- name
}

// // Unregister removes the registered name
// func (n *Node) Unregister(name etf.Atom) {
// 	r := unregNameReq{name: name}
// 	n.registry.unregNameChan <- r
// }

// // Registered returns a list of names which have been registered using Register
func (r *registrar) Registered() []Process {
	p := make([]Process, len(r.processes))
	i := 0
	for _, process := range r.processes {
		p[i] = process
		i++
	}
	return p
}

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
