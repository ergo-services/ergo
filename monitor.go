package ergo

// http://erlang.org/doc/reference_manual/processes.html

import (
	"fmt"
	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/lib"
	"sync"
)

type monitorItem struct {
	pid  etf.Pid
	ref  etf.Ref
	refs string
}

type linkProcessRequest struct {
	pidA etf.Pid
	pidB etf.Pid
}

type monitor struct {
	processes      map[etf.Pid][]monitorItem
	ref2pid        map[string]etf.Pid
	mutexProcesses sync.Mutex
	links          map[etf.Pid][]etf.Pid
	mutexLinks     sync.Mutex
	nodes          map[string][]monitorItem
	ref2node       map[string]string
	mutexNodes     sync.Mutex

	node *Node
}

func createMonitor(node *Node) *monitor {
	m := &monitor{
		processes: make(map[etf.Pid][]monitorItem),
		links:     make(map[etf.Pid][]etf.Pid),
		nodes:     make(map[string][]monitorItem),

		ref2pid:  make(map[string]etf.Pid),
		ref2node: make(map[string]string),

		node: node,
	}

	return m
}

func (m *monitor) MonitorProcess(by etf.Pid, process interface{}) etf.Ref {
	ref := m.node.MakeRef()
	m.MonitorProcessWithRef(by, process, ref)
	return ref
}

func (m *monitor) MonitorProcessWithRef(by etf.Pid, process interface{}, ref etf.Ref) {
next:
	switch t := process.(type) {
	case etf.Pid:
		lib.Log("[%s] MONITOR process: %v => %v", m.node.FullName, by, t)
		// http://erlang.org/doc/reference_manual/processes.html#monitors
		// FIXME: If Pid does not exist, the 'DOWN' message should be
		// send immediately with Reason set to noproc.

		m.mutexProcesses.Lock()
		l := m.processes[t]
		key := ref2key(ref)
		item := monitorItem{
			pid:  by,
			ref:  ref,
			refs: key,
		}
		m.processes[t] = append(l, item)
		m.ref2pid[key] = t
		m.mutexProcesses.Unlock()

		if !isFakePid(t) && string(t.Node) != m.node.FullName { // request monitor remote process
			message := etf.Tuple{distProtoMONITOR, by, t, ref}
			m.node.registrar.routeRaw(t.Node, message)
		}

	case etf.Atom: // requesting monitor of local process
		process = fakeMonitorPidFromName(string(t))
		goto next

	case etf.Tuple:
		// requesting monitor of remote process by the local one using registered process name
		nodeName := t.Element(2).(etf.Atom)
		if nodeName != etf.Atom(m.node.FullName) {
			message := etf.Tuple{distProtoMONITOR, by, t, ref}
			m.node.registrar.routeRaw(nodeName, message)
			// FIXME:
			// make fake pid with remote nodename and keep it
			// in order to handle 'nodedown' event with
			// fakePid := fakeMonitorPidFromName(string(nodeName))

		}

		// registering monitor of local process
		local := t.Element(1).(etf.Atom)
		message := etf.Tuple{distProtoMONITOR, by, local, ref}
		m.node.registrar.route(by, local, message)
	}
}

func (m *monitor) DemonitorProcess(ref etf.Ref) {
	var pid etf.Pid
	var ok bool

	key := ref2key(ref)

	m.mutexProcesses.Lock()
	defer m.mutexProcesses.Unlock()

	if pid, ok = m.ref2pid[key]; !ok {
		// unknown monitor reference
		return
	}

	l := m.processes[pid]

	// remove PID from monitoring processes list
	for i := range l {
		if l[i].refs == key {
			if !isFakePid(pid) && string(pid.Node) != m.node.FullName { // request demonitor remote process
				message := etf.Tuple{distProtoDEMONITOR, l[i].pid, pid, ref}
				m.node.registrar.routeRaw(pid.Node, message)
			}

			l[i] = l[0]
			l = l[1:]
			delete(m.ref2pid, key)
			break
		}
	}

	if len(l) == 0 {
		delete(m.processes, pid)
	} else {
		m.processes[pid] = l
	}
}

func (m *monitor) Link(pidA, pidB etf.Pid) {
	lib.Log("[%s] LINK process: %v => %v", m.node.FullName, pidA, pidB)

	// http://erlang.org/doc/reference_manual/processes.html#links
	// Links are bidirectional and there can only be one link between
	// two processes. Repeated calls to link(Pid) have no effect.

	var linksA, linksB []etf.Pid

	m.mutexLinks.Lock()
	defer m.mutexLinks.Unlock()

	// remote makes link to local
	if pidA.Node != etf.Atom(m.node.FullName) {
		goto skip
	}

	linksA = m.links[pidA]
	for i := range linksA {
		if linksA[i] == pidB {
			goto skip
		}
	}

	linksA = append(linksA, pidB)
	m.links[pidA] = linksA

skip:
	// local makes link to remote
	if pidB.Node != etf.Atom(m.node.FullName) {
		message := etf.Tuple{distProtoLINK, pidA, pidB}
		m.node.registrar.routeRaw(pidB.Node, message)

		// goto doneBl
		// we do not jump to doneBl in order to be able to handle
		// 'nodedown' event and notify that kind of links
		// with 'EXIT' messages and 'noconnection' as a reason
	}

	linksB = m.links[pidB]
	for i := range linksB {
		if linksB[i] == pidA {
			return
		}
	}

	linksB = append(linksB, pidA)
	m.links[pidB] = linksB

}

func (m *monitor) Unlink(pidA, pidB etf.Pid) {
	m.mutexLinks.Lock()
	defer m.mutexLinks.Unlock()

	if pidB.Node != etf.Atom(m.node.FullName) {
		message := etf.Tuple{distProtoUNLINK, pidA, pidB}
		m.node.registrar.routeRaw(pidB.Node, message)
	}

	linksA := m.links[pidA]
	for i := range linksA {
		if linksA[i] == pidB {
			linksA[i] = linksA[0]
			linksA = linksA[1:]
			m.links[pidA] = linksA
			break
		}
	}

	linksB := m.links[pidB]
	for i := range linksB {
		if linksB[i] == pidA {
			linksB[i] = linksB[0]
			linksB = linksB[1:]
			m.links[pidB] = linksB
			break
		}
	}
}

func (m *monitor) MonitorNode(by etf.Pid, node string) etf.Ref {
	lib.Log("[%s] MONITOR NODE : %v => %s", m.node.FullName, by, node)

	ref := m.node.MakeRef()
	m.mutexNodes.Lock()
	defer m.mutexNodes.Unlock()

	l := m.nodes[node]
	key := ref2key(ref)
	item := monitorItem{
		pid:  by,
		ref:  ref,
		refs: key,
	}
	m.nodes[node] = append(l, item)
	m.ref2node[key] = node

	return ref
}

func (m *monitor) DemonitorNode(ref etf.Ref) {
	var name string
	var ok bool

	m.mutexNodes.Lock()
	defer m.mutexNodes.Unlock()

	key := ref2key(ref)
	if name, ok = m.ref2node[key]; !ok {
		return
	}

	l := m.nodes[name]

	// remove PID from monitoring processes list
	for i := range l {
		if l[i].refs == key {
			l[i] = l[0]
			l = l[1:]
			delete(m.ref2pid, key)
			break
		}
	}
	m.nodes[name] = l
}

func (m *monitor) NodeDown(name string) {
	lib.Log("[%s] MONITOR NODE  down: %v", m.node.FullName, name)

	m.mutexNodes.Lock()
	if pids, ok := m.nodes[name]; ok {
		for i := range pids {
			lib.Log("[%s] MONITOR node down: %v. send notify to: %v", m.node.FullName, name, pids[i])
			m.notifyNodeDown(pids[i].pid, name)
			delete(m.nodes, name)
		}
	}
	m.mutexNodes.Unlock()

	// notify process monitors
	m.mutexProcesses.Lock()
	for pid, ps := range m.processes {
		if pid.Node == etf.Atom(name) {
			for i := range ps {
				m.notifyProcessTerminated(ps[i].ref, ps[i].pid, pid, "noconnection")
			}
			delete(m.processes, pid)
		}
	}
	m.mutexProcesses.Unlock()

	// notify linked processes
	m.mutexLinks.Lock()
	for link, pids := range m.links {
		if link.Node == etf.Atom(name) {
			for i := range pids {
				m.notifyProcessExit(pids[i], link, "noconnection")
			}
			delete(m.links, link)
		}
	}
	m.mutexLinks.Unlock()
}

func (m *monitor) ProcessTerminated(process etf.Pid, name etf.Atom, reason string) {
	lib.Log("[%s] MONITOR process terminated: %v", m.node.FullName, process)

	m.mutexProcesses.Lock()
	if pids, ok := m.processes[process]; ok {
		for i := range pids {
			lib.Log("[%s] MONITOR process terminated: %v send notify to: %v", m.node.FullName, process, pids[i].pid)
			m.notifyProcessTerminated(pids[i].ref, pids[i].pid, process, reason)
		}
		delete(m.processes, process)
	}
	m.mutexProcesses.Unlock()

	m.mutexLinks.Lock()
	if pidLinks, ok := m.links[process]; ok {
		for i := range pidLinks {
			lib.Log("[%s] LINK process exited: %v send notify to: %v", m.node.FullName, process, pidLinks[i])
			m.notifyProcessExit(pidLinks[i], process, reason)

			// remove A link
			if pids, ok := m.links[pidLinks[i]]; ok {
				for k := range pids {
					if pids[k] == process {
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
		// remove link
		delete(m.links, process)
	}
	m.mutexLinks.Unlock()

	// handling termination monitors that have setted up by name.
	if name != "" {
		fakePid := fakeMonitorPidFromName(string(name))
		m.ProcessTerminated(fakePid, "", reason)
	}
}

func (m *monitor) GetLinks(process etf.Pid) []etf.Pid {
	m.mutexLinks.Lock()
	defer m.mutexLinks.Unlock()

	if links, ok := m.links[process]; ok {
		return links
	}
	return nil
}

func (m *monitor) GetMonitors(process etf.Pid) []etf.Pid {
	monitors := []etf.Pid{}
	m.mutexProcesses.Lock()
	defer m.mutexProcesses.Unlock()

	for p, by := range m.processes {
		for b := range by {
			if by[b].pid == process {
				monitors = append(monitors, p)
			}
		}
	}
	return monitors
}

func (m *monitor) GetMonitoredBy(process etf.Pid) []etf.Pid {
	monitors := []etf.Pid{}
	m.mutexProcesses.Lock()
	defer m.mutexProcesses.Unlock()

	if m, ok := m.processes[process]; ok {
		monitors := []etf.Pid{}
		for i := range m {
			monitors = append(monitors, m[i].pid)
		}

	}
	return monitors
}

func (m *monitor) notifyNodeDown(to etf.Pid, node string) {
	message := etf.Term(etf.Tuple{etf.Atom("nodedown"), node})
	m.node.registrar.route(etf.Pid{}, to, message)
}

func (m *monitor) notifyProcessTerminated(ref etf.Ref, to etf.Pid, terminated etf.Pid, reason string) {

	// for remote {21, FromProc, ToPid, Ref, Reason}, where FromProc = monitored process
	if to.Node != etf.Atom(m.node.FullName) {
		message := etf.Tuple{distProtoMONITOR_EXIT, terminated, to, ref, etf.Atom(reason)}
		m.node.registrar.routeRaw(to.Node, message)
		return
	}

	// {'DOWN', Ref, process, Pid, Reason}
	// {'DOWN',#Ref<0.0.13893633.237772>,process,<26194.4.1>,reason}
	fakePid := fakeMonitorPidFromName(string(terminated.Node))
	if terminated == fakePid {
		p := etf.Tuple{terminated.Node, m.node.FullName}
		message := etf.Term(etf.Tuple{etf.Atom("DOWN"), ref, etf.Atom("process"), p, etf.Atom(reason)})
		m.node.registrar.route(terminated, to, message)
		return
	}

	message := etf.Term(etf.Tuple{etf.Atom("DOWN"), ref, etf.Atom("process"), terminated, etf.Atom(reason)})
	m.node.registrar.route(terminated, to, message)
}

func (m *monitor) notifyProcessExit(to etf.Pid, terminated etf.Pid, reason string) {
	// for remote: {3, FromPid, ToPid, Reason}
	if to.Node != etf.Atom(m.node.FullName) {
		message := etf.Tuple{distProtoEXIT, terminated, to, etf.Atom(reason)}
		m.node.registrar.routeRaw(to.Node, message)
		return
	}
	message := etf.Term(etf.Tuple{etf.Atom("EXIT"), terminated, etf.Atom(reason)})
	m.node.registrar.route(terminated, to, message)
}

func ref2key(ref etf.Ref) string {
	return fmt.Sprintf("%v", ref)
}

func fakeMonitorPidFromName(name string) etf.Pid {
	fakePid := etf.Pid{}
	fakePid.Node = etf.Atom(name) // registered process name
	fakePid.ID = 4294967295       // 2^32 - 1
	fakePid.Serial = 4294967295   // 2^32 - 1
	fakePid.Creation = 255        // 2^8 - 1
	return fakePid
}

func isFakePid(pid etf.Pid) bool {
	if pid.ID == 4294967295 && pid.Serial == 4294967295 && pid.Creation == 255 {
		return true
	}
	return false
}
