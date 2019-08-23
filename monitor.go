package ergonode

import (
	"context"

	"github.com/halturin/ergonode/etf"
	"github.com/halturin/ergonode/lib"
)

type monitorProcessRequest struct {
	process etf.Pid
	by      etf.Pid
}

type monitorNodeRequest struct {
	node string
	by   etf.Pid
}

type monitorChannels struct {
	process          chan monitorProcessRequest
	demonitorProcess chan monitorProcessRequest
	node             chan monitorNodeRequest
	demonitorName    chan monitorNodeRequest

	nodeDown          chan string
	processTerminated chan etf.Pid
}

type monitor struct {
	processes map[etf.Pid][]etf.Pid
	nodes     map[string][]etf.Pid

	channels monitorChannels

	node    *Node
	context context.Context
}

func createMonitor(ctx context.Context, node *Node) *monitor {
	m := &monitor{
		processes: make(map[etf.Pid][]etf.Pid),
		nodes:     make(map[string][]etf.Pid),
		channels: monitorChannels{
			process:          make(chan monitorProcessRequest),
			demonitorProcess: make(chan monitorProcessRequest),
			node:             make(chan monitorNodeRequest),
			demonitorName:    make(chan monitorNodeRequest),

			nodeDown:          make(chan string),
			processTerminated: make(chan etf.Pid),
		},
		node:    node,
		context: ctx,
	}

	go m.run()

	return m
}

func (m *monitor) run() {
	defer func() {
		close(m.channels.process)
		close(m.channels.demonitorProcess)
		close(m.channels.node)
		close(m.channels.demonitorName)
		close(m.channels.nodeDown)
		close(m.channels.processTerminated)
	}()

	for {
		select {
		case p := <-m.channels.process:
			lib.Log("MONITOR process: %#v by %#v", p.process, p.by)
			l := m.processes[p.process]
			l = append(l, p.by)
			m.processes[p.process] = l
		case dp := <-m.channels.demonitorProcess:
			l := m.processes[dp.by]
			// TODO: remove PID from monitoring processes list
			m.processes[dp.process] = l
		case n := <-m.channels.node:
			l := m.nodes[n.node]
			l = append(l, n.by)
			m.nodes[n.node] = l
		case dn := <-m.channels.demonitorName:
			l := m.nodes[dn.node]
			// TODO: remove PID from monitoring processes list
			m.nodes[dn.node] = l
		case nd := <-m.channels.nodeDown:
			if pids, ok := m.nodes[nd]; ok {
				for i := range pids {
					m.notifyNodeDown(nd, pids[i])
				}
			}
		case pt := <-m.channels.processTerminated:
			lib.Log("MONITOR process terminated: %#v", pt)
			if pids, ok := m.processes[pt]; ok {
				lib.Log("MONITOR process notif send to: %#v", pids)

				for i := range pids {
					m.notifyProcessTerminated(pt, pids[i])
				}
			}
		case <-m.context.Done():
			return
		}
	}
}

func (m *monitor) MonitorProcess(process, by etf.Pid) {
	p := monitorProcessRequest{
		process: process,
		by:      by,
	}
	m.channels.process <- p
}

func (m *monitor) DemonitorProcess(process, by etf.Pid) {
	p := monitorProcessRequest{
		process: process,
		by:      by,
	}
	m.channels.demonitorProcess <- p
}

func (m *monitor) MonitorNode(node string, by etf.Pid) {

	n := monitorNodeRequest{
		node: node,
		by:   by,
	}

	m.channels.node <- n
}

func (m *monitor) DemonitorNode(node string, by etf.Pid) {
	n := monitorNodeRequest{
		node: node,
		by:   by,
	}

	m.channels.node <- n
}

func (m *monitor) NodeDown(node string) {
	// TODO:
}

func (m *monitor) ProcessTerminated(process etf.Pid) {
	// TODO:
}

func (m *monitor) notifyNodeDown(node string, to etf.Pid) {
	// TODO:
}

func (m *monitor) notifyProcessTerminated(terminated etf.Pid, to etf.Pid) {
	// TODO:
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