package ergonode

// http://erlang.org/doc/reference_manual/processes.html

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
	name    etf.Atom
	reason  string
}

type monitorChannels struct {
	process          chan monitorProcessRequest
	demonitorProcess chan monitorProcessRequest
	link             chan linkedProcessesRequest
	unlink           chan linkedProcessesRequest
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

type linkedProcessesRequest struct {
	pidA etf.Pid
	pidB etf.Pid
}

type monitor struct {
	processes map[etf.Pid][]monitorItem
	links     map[etf.Pid][]etf.Pid
	nodes     map[string][]monitorItem
	ref2pid   map[string]etf.Pid
	ref2node  map[string]string

	channels monitorChannels

	node *Node
}

func createMonitor(node *Node) *monitor {
	m := &monitor{
		processes: make(map[etf.Pid][]monitorItem),
		links:     make(map[etf.Pid][]etf.Pid),
		nodes:     make(map[string][]monitorItem),

		ref2pid:  make(map[string]etf.Pid),
		ref2node: make(map[string]string),

		channels: monitorChannels{
			process:          make(chan monitorProcessRequest, 10),
			demonitorProcess: make(chan monitorProcessRequest, 10),
			link:             make(chan linkedProcessesRequest, 10),
			unlink:           make(chan linkedProcessesRequest, 10),
			node:             make(chan monitorNodeRequest, 10),
			demonitorName:    make(chan monitorNodeRequest, 10),

			nodeDown:          make(chan string, 10),
			processTerminated: make(chan processTerminatedRequest, 10),
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
			// http://erlang.org/doc/reference_manual/processes.html#monitors
			// FIXME: If Pid does not exist, the 'DOWN' message is
			// sent immediately with Reason set to noproc.
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
				dp.process = pid
			} else {
				// unknown monitor reference
				continue
			}
			l := m.processes[dp.process]

			// remove PID from monitoring processes list
			for i := range l {
				if l[i].refs == key {
					l[i] = l[0]
					l = l[1:]
					delete(m.ref2pid, key)
					break
				}
			}

			if len(l) == 0 {
				delete(m.processes, dp.process)
			} else {
				m.processes[dp.process] = l
			}

		case l := <-m.channels.link:
			// http://erlang.org/doc/reference_manual/processes.html#links
			// Links are bidirectional and there can only be one link between
			// two processes. Repeated calls to link(Pid) have no effect.

			var linksA, linksB []etf.Pid

			// remote makes link to local
			if l.pidA.Node != etf.Atom(m.node.FullName) {
				goto doneAl
			}

			linksA = m.links[l.pidA]
			for i := range linksA {
				if linksA[i] == l.pidB {
					goto doneAl
				}
			}

			linksA = append(linksA, l.pidB)
			m.links[l.pidA] = linksA

		doneAl:
			// local makes link to remote
			if l.pidB.Node != etf.Atom(m.node.FullName) {
				message := etf.Tuple{LINK, l.pidA, l.pidB}
				m.node.registrar.routeRaw(l.pidB.Node, message)

				// goto doneBl
				// we do not jump to doneBl in order to be able to handle
				// 'nodedown' event and notify that kind of links
				// with 'EXIT' messages and 'noconnection' as a reason
			}

			linksB = m.links[l.pidB]
			for i := range linksB {
				if linksB[i] == l.pidA {
					goto doneBl
				}
			}

			linksB = append(linksB, l.pidA)
			m.links[l.pidB] = linksB

		doneBl:
			continue

		case ul := <-m.channels.unlink:

			linksA := m.links[ul.pidA]
			for i := range linksA {
				if linksA[i] == ul.pidB {
					linksA[i] = linksA[0]
					linksA = linksA[1:]
					m.links[ul.pidA] = linksA
					break
				}
			}

			linksB := m.links[ul.pidB]
			for i := range linksB {
				if linksB[i] == ul.pidA {
					linksB[i] = linksB[0]
					linksB = linksB[1:]
					m.links[ul.pidB] = linksB
					break
				}
			}

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
			fmt.Printf("\nHHHHHHHHHHPPPP %#v %#v\n", m.node.FullName, m.processes)

			// notify process monitors
			for pid, ps := range m.processes {
				if pid.Node == etf.Atom(nd) {
					for i := range ps {
						m.notifyProcessTerminated(ps[i].ref, ps[i].pid, pid, "noconnection")
					}
					delete(m.processes, pid)
				}
			}

			fmt.Printf("\nHHHHHHHHHLLLL %#v %#v\n", m.node.FullName, m.links)

			// notify linked processes
			for link, pids := range m.links {
				if link.Node == etf.Atom(nd) {
					for i := range pids {
						m.notifyProcessExit(pids[i], link, "noconnection")
					}
					delete(m.links, link)
				}
			}

		case pt := <-m.channels.processTerminated:
			lib.Log("MONITOR process terminated: %v", pt)
			if pids, ok := m.processes[pt.process]; ok {
				for i := range pids {
					lib.Log("MONITOR process terminated: %v send notify to: %v", pt, pids[i].pid)
					m.notifyProcessTerminated(pids[i].ref, pids[i].pid, pt.process, pt.reason)
					delete(m.processes, pt.process)
				}
			}

			if pidLinks, ok := m.links[pt.process]; ok {
				for i := range pidLinks {
					lib.Log("MONITOR (LINK) process exited: %v send notify to: %v", pt, pidLinks[i])
					m.notifyProcessExit(pidLinks[i], pt.process, pt.reason)

					// remove link
					if pids, ok := m.links[pidLinks[i]]; ok {
						for k := range pids {
							if pids[k] == pt.process {
								pids[k] = pids[0]
								pids = pids[1:]
								break
							}
						}

						if len(pids) > 0 {
							m.links[pidLinks[i]] = pids
						} else {
							delete(m.links, pidLinks[i])
						}
					}
				}
				delete(m.links, pt.process)
			}

			// handling termination monitors that have setted up by name.
			if pt.name != "" {
				fakePid := fakeMonitorPidFromName(string(pt.name))
				m.ProcessTerminated(fakePid, "", pt.reason)
			}

		case <-m.node.context.Done():
			return
		}
	}
}

func (m *monitor) MonitorProcess(by etf.Pid, process interface{}) etf.Ref {
	ref := m.node.MakeRef()
	m.MonitorProcessWithRef(by, process, ref)
	return ref
}

func (m *monitor) MonitorProcessWithRef(by etf.Pid, process interface{}, ref etf.Ref) {
	switch t := process.(type) {
	case etf.Atom: // requesting monitor of local process
		fakePid := fakeMonitorPidFromName(string(t))
		p := monitorProcessRequest{
			process: fakePid,
			by:      by,
			ref:     ref,
		}
		m.channels.process <- p

	case etf.Tuple:
		// requesting monitor of remote process by the local one using registered process name
		nodeName := t.Element(2).(etf.Atom)
		if nodeName != etf.Atom(m.node.FullName) {
			message := etf.Tuple{MONITOR, by, t, ref}
			m.node.registrar.routeRaw(nodeName, message)
			// FIXME:
			// make fake pid with remote nodename and keep it
			// in order to handle 'nodedown' event
			// fakePid := fakeMonitorPidFromName(string(nodeName))
			// p := monitorProcessRequest{
			// 	process: fakePid,
			// 	by:      by,
			// 	ref:     ref,
			// }
			// m.channels.process <- p
			// return
		}

		// registering monitor of local process
		local := t.Element(1).(etf.Atom)
		message := etf.Tuple{MONITOR, by, local, ref}
		m.node.registrar.route(by, local, message)

	case etf.Pid:
		if string(t.Node) != m.node.FullName { // request monitor remote process using Pid
			message := etf.Tuple{MONITOR, by, t, ref}
			m.node.registrar.routeRaw(t.Node, message)
		}
		p := monitorProcessRequest{
			process: t,
			by:      by,
			ref:     ref,
		}
		m.channels.process <- p
	}
}

func (m *monitor) DemonitorProcess(ref etf.Ref) {
	p := monitorProcessRequest{
		ref: ref,
	}
	m.channels.demonitorProcess <- p
}

func (m *monitor) Link(pidA, pidB etf.Pid) {
	p := linkedProcessesRequest{
		pidA: pidA,
		pidB: pidB,
	}
	m.channels.link <- p
}

func (m *monitor) Unink(pidA, pidB etf.Pid) {
	p := linkedProcessesRequest{
		pidA: pidA,
		pidB: pidB,
	}
	m.channels.unlink <- p
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

func (m *monitor) ProcessTerminated(process etf.Pid, name etf.Atom, reason string) {
	p := processTerminatedRequest{
		process: process,
		name:    name,
		reason:  reason,
	}
	m.channels.processTerminated <- p
}

func (m *monitor) notifyNodeDown(to etf.Pid, node string) {
	message := etf.Term(etf.Tuple{etf.Atom("nodedown"), node})
	m.node.registrar.route(etf.Pid{}, to, message)
}

func (m *monitor) notifyProcessTerminated(ref etf.Ref, to etf.Pid, terminated etf.Pid, reason string) {
	// {'DOWN', Ref, process, Pid, Reason}
	// {'DOWN',#Ref<0.0.13893633.237772>,process,<26194.4.1>,reason}
	fakePid := fakeMonitorPidFromName(string(terminated.Node))
	if terminated == fakePid {
		p := etf.Tuple{terminated.Node, m.node.FullName}
		message := etf.Term(etf.Tuple{etf.Atom("DOWN"), ref, etf.Atom("process"), p, reason})
		m.node.registrar.route(etf.Pid{}, to, message)
		return
	}

	message := etf.Term(etf.Tuple{etf.Atom("DOWN"), ref, etf.Atom("process"), terminated, reason})
	m.node.registrar.route(etf.Pid{}, to, message)
}

func (m *monitor) notifyProcessExit(to etf.Pid, terminated etf.Pid, reason string) {
	message := etf.Term(etf.Tuple{etf.Atom("EXIT"), terminated, reason})
	m.node.registrar.route(etf.Pid{}, to, message)
}

func ref2key(ref etf.Ref) string {
	return fmt.Sprintf("%v", ref)
}

func fakeMonitorPidFromName(name string) etf.Pid {
	fakePid := etf.Pid{}
	fakePid.Node = etf.Atom(name) // registered process name
	fakePid.Id = 4294967295       // 2^32 - 1
	fakePid.Serial = 4294967295   // 2^32 - 1
	fakePid.Creation = 255        // 2^8 - 1
	return fakePid
}
