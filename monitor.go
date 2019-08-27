package ergonode

import (
	"fmt"

	"github.com/halturin/ergonode/etf"
	"github.com/halturin/ergonode/lib"
)

type monitorProcessRequest struct {
	process etf.Pid
	by      etf.Pid
	ref     etf.Ref
}

type monitorNodeRequest struct {
	node string
	by   etf.Pid
	ref  etf.Ref
}

type processTerminatedRequest struct {
	process etf.Pid
	reason  string
}

type monitorChannels struct {
	process          chan monitorProcessRequest
	demonitorProcess chan monitorProcessRequest
	node             chan monitorNodeRequest
	demonitorName    chan monitorNodeRequest

	nodeDown          chan string
	processTerminated chan processTerminatedRequest
}

type monitorItem struct {
	pid  etf.Pid
	ref  etf.Ref
	refs string
}

type monitor struct {
	processes map[etf.Pid][]monitorItem
	nodes     map[string][]monitorItem
	ref2pid   map[string]etf.Pid
	ref2node  map[string]string

	channels monitorChannels

	node *Node
}

func createMonitor(node *Node) *monitor {
	m := &monitor{
		processes: make(map[etf.Pid][]monitorItem),
		nodes:     make(map[string][]monitorItem),

		ref2pid:  make(map[string]etf.Pid),
		ref2node: make(map[string]string),

		channels: monitorChannels{
			process:          make(chan monitorProcessRequest),
			demonitorProcess: make(chan monitorProcessRequest),
			node:             make(chan monitorNodeRequest),
			demonitorName:    make(chan monitorNodeRequest),

			nodeDown:          make(chan string),
			processTerminated: make(chan processTerminatedRequest),
		},
		node: node,
	}

	go m.run()

	return m
}

func (m *monitor) run() {
	for {
		select {

		case p := <-m.channels.process:
			lib.Log("MONITOR process: %v => %v", p.by, p.process)
			l := m.processes[p.process]
			key := ref2key(p.ref)
			item := monitorItem{
				pid:  p.by,
				ref:  p.ref,
				refs: key,
			}
			m.processes[p.process] = append(l, item)
			m.ref2pid[key] = p.process

		case dp := <-m.channels.demonitorProcess:
			key := ref2key(dp.ref)
			if pid, ok := m.ref2pid[key]; ok {
				dp.by = pid
			} else {
				// unknown monitor reference
				continue
			}
			l := m.processes[dp.by]
			// remove PID from monitoring processes list
			for i := range l {
				if l[i].pid == dp.process && l[i].refs == key {
					l[i] = l[0]
					l = l[1:]
					delete(m.ref2pid, key)
					break
				}
			}
			m.processes[dp.process] = l

		case n := <-m.channels.node:
			l := m.nodes[n.node]
			key := ref2key(n.ref)
			item := monitorItem{
				pid:  n.by,
				ref:  n.ref,
				refs: key,
			}
			m.nodes[n.node] = append(l, item)
			m.ref2node[key] = n.node

		case dn := <-m.channels.demonitorName:
			key := ref2key(dn.ref)
			if name, ok := m.ref2node[key]; ok {
				dn.node = name
			} else {
				// unknown monitor reference
				continue
			}

			l := m.nodes[dn.node]

			// remove PID from monitoring processes list
			for i := range l {
				if l[i].pid == dn.by && l[i].refs == key {
					l[i] = l[0]
					l = l[1:]
					delete(m.ref2pid, key)
					break
				}
			}
			m.nodes[dn.node] = l

		case nd := <-m.channels.nodeDown:
			lib.Log("MONITOR node down: %v", nd)
			if pids, ok := m.nodes[nd]; ok {
				for i := range pids {
					lib.Log("MONITOR node down: %v. send notify to: %v", nd, pids[i])
					m.notifyNodeDown(pids[i].pid, nd)
					delete(m.nodes, nd)
				}
			}

		case pt := <-m.channels.processTerminated:
			lib.Log("MONITOR process terminated: %v", pt)
			if pids, ok := m.processes[pt.process]; ok {
				for i := range pids {
					lib.Log("MONITOR process terminated: %v send notify to: %v", pt, pids[i].pid)
					m.notifyProcessTerminated(pids[i].pid, pt.process, pt.reason)
					delete(m.processes, pt.process)
				}
			}

		case <-m.node.context.Done():
			return
		}
	}
}

func (m *monitor) MonitorProcess(by, process etf.Pid) etf.Ref {
	ref := m.node.MakeRef()
	p := monitorProcessRequest{
		process: process,
		by:      by,
		ref:     ref,
	}
	m.channels.process <- p
	return ref
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
}

func (m *monitor) DemonitorProcess(ref etf.Ref) {
	p := monitorProcessRequest{
		ref: ref,
	}
	m.channels.demonitorProcess <- p
}

func (m *monitor) MonitorNode(by etf.Pid, node string) etf.Ref {
	ref := m.node.MakeRef()
	n := monitorNodeRequest{
		node: node,
		by:   by,
		ref:  ref,
	}

	m.channels.node <- n
	return ref
}

func (m *monitor) DemonitorNode(ref etf.Ref) {
	n := monitorNodeRequest{
		ref: ref,
	}

	m.channels.node <- n
}

func (m *monitor) NodeDown(node string) {
	m.channels.nodeDown <- node
}

func (m *monitor) ProcessTerminated(process etf.Pid, reason string) {
	p := processTerminatedRequest{
		process: process,
		reason:  reason,
	}
	m.channels.processTerminated <- p
}

func (m *monitor) notifyNodeDown(to etf.Pid, node string) {
	// TODO: send event to the watchers

	// msg := etf.Term(etf.Tuple{etf.Atom("nodedown"), node})
	// m.node.Send
}

func (m *monitor) notifyProcessTerminated(to etf.Pid, terminated etf.Pid, reason string) {
	// TODO: send event to the watchers
	// {'DOWN', Ref, process, Pid2, Reason}

	// {'DOWN',#Ref<0.0.13893633.237772>,process,<26194.4.1>,reason}

	// M := etf.Term(etf.Tuple{etf.Atom("DOWN"),
	// t.Element(3), etf.Atom("process"),
	// t.Element(2), t.Element(5)})

	// n.route(t.Element(2), t.Element(3), M)
}

func ref2key(ref etf.Ref) string {
	return fmt.Sprintf("%v", ref)
}
