package ergo

// http://erlang.org/doc/reference_manual/processes.html

import (
	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/lib"
	"strings"
	"sync"
)

type monitorItem struct {
	pid     etf.Pid // by
	process etf.Pid
	ref     etf.Ref
	key     string
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
	if by.Node != ref.Node {
		lib.Log("[%s] Incorrect monitor request by Pid = %v and Ref = %v", m.node.FullName, by, ref)
		return
	}

next:
	switch t := process.(type) {
	case etf.Pid:
		lib.Log("[%s] MONITOR process: %v => %v", m.node.FullName, by, t)

		// If 'process' belongs this node we should make sure if its alive.
		// http://erlang.org/doc/reference_manual/processes.html#monitors
		// If Pid does not exist, the 'DOWN' message should be
		// send immediately with Reason set to noproc.
		if p := m.node.registrar.GetProcessByPid(t); string(t.Node) == m.node.FullName && p == nil {
			m.notifyProcessTerminated(ref, by, t, "noproc")
			return
		}

		m.mutexProcesses.Lock()
		l := m.processes[t]
		key := ref.String()
		item := monitorItem{
			pid:     by,
			ref:     ref,
			key:     key,
			process: t,
		}
		m.processes[t] = append(l, item)
		m.ref2pid[key] = t
		m.mutexProcesses.Unlock()

		if isFakePid(t) {
			// this Pid was created as a virtual. we use virtual (fake) pids for the
			// cases if monitoring process was requested using the registered name.
			return
		}

		if string(t.Node) == m.node.FullName {
			// this is the local process so we have nothing to do
			return
		}

		// request monitoring the remote process
		message := etf.Tuple{distProtoMONITOR, by, t, ref}
		if err := m.node.registrar.routeRaw(t.Node, message); err != nil {
			m.notifyProcessTerminated(ref, by, t, "noconnection")
			m.mutexProcesses.Lock()
			delete(m.ref2pid, key)
			m.mutexProcesses.Unlock()
		}

	case string:
		// requesting monitor of local process
		fakePid := fakeMonitorPidFromName(t, m.node.FullName)
		// If Pid does not exist, the 'DOWN' message should be
		// send immediately with Reason set to noproc.
		if p := m.node.registrar.GetProcessByName(t); p == nil {
			m.notifyProcessTerminated(ref, by, fakePid, "noproc")
			return
		}
		process = fakePid
		goto next
	case etf.Atom:
		// the same as 'string'
		fakePid := fakeMonitorPidFromName(string(t), m.node.FullName)
		if p := m.node.registrar.GetProcessByName(string(t)); p == nil {
			m.notifyProcessTerminated(ref, by, fakePid, "noproc")
			return
		}
		process = fakePid
		goto next

	case etf.Tuple:
		// requesting monitor of remote process by the local one using registered process name
		if len(t) != 2 {
			lib.Log("[%s] Incorrect monitor request by Pid = %v and process = %v", m.node.FullName, process, ref)
			return
		}

		name := t.Element(1).(string)
		nodeName := t.Element(2).(string)
		fakePid := fakeMonitorPidFromName(name, nodeName)
		process = fakePid

		if nodeName == m.node.FullName {
			// If Pid does not exist, the 'DOWN' message should be
			// send immediately with Reason set to noproc.
			if p := m.node.registrar.GetProcessByName(name); p == nil {
				m.notifyProcessTerminated(ref, by, fakePid, "noproc")
				return
			}
			goto next
		}

		message := etf.Tuple{distProtoMONITOR, by, name, ref}
		if err := m.node.registrar.routeRaw(etf.Atom(nodeName), message); err != nil {
			m.notifyProcessTerminated(ref, by, fakePid, "noconnection")
			return
		}

		// in order to handle 'nodedown' event we create a local monitor on a fake pid
		goto next
	}
}

func (m *monitor) DemonitorProcess(ref etf.Ref) bool {
	var pid etf.Pid
	var ok bool
	var process interface{}
	var nodeName etf.Atom

	key := ref.String()

	m.mutexProcesses.Lock()
	defer m.mutexProcesses.Unlock()

	if pid, ok = m.ref2pid[key]; !ok {
		// unknown monitor reference
		return false
	}

	// cheching for monitorItem list
	items := m.processes[pid]

	// remove PID from monitoring processes list
	for i := range items {
		if items[i].key != key {
			continue
		}
		process = items[i].process
		nodeName = pid.Node
		if isFakePid(pid) {
			p, n := fakePidExtractNames(pid)
			process = p
			nodeName = etf.Atom(n)
		}

		if string(nodeName) != m.node.FullName {
			message := etf.Tuple{distProtoDEMONITOR, items[i].pid, process, ref}
			m.node.registrar.routeRaw(nodeName, message)
		}

		items[i] = items[0]
		items = items[1:]
		delete(m.ref2pid, key)
		break

	}

	if len(items) == 0 {
		delete(m.processes, pid)
	} else {
		m.processes[pid] = items
	}

	return true
}

func (m *monitor) Link(pidA, pidB etf.Pid) {
	lib.Log("[%s] LINK process: %v => %v", m.node.FullName, pidA, pidB)

	// http://erlang.org/doc/reference_manual/processes.html#links
	// Links are bidirectional and there can only be one link between
	// two processes. Repeated calls to link(Pid) have no effect.

	// If the link already exists or a process attempts to create
	// a link to itself, nothing is done.
	if pidA == pidB {
		return
	}

	m.mutexLinks.Lock()
	defer m.mutexLinks.Unlock()

	linksA := m.links[pidA]
	if pidA.Node == etf.Atom(m.node.FullName) {
		// check if these processes are linked already (source)
		for i := range linksA {
			if linksA[i] == pidB {
				return
			}
		}

		m.links[pidA] = append(linksA, pidB)
	}

	// check if these processes are linked already (destination)
	linksB := m.links[pidB]
	for i := range linksB {
		if linksB[i] == pidA {
			return
		}
	}

	if pidB.Node == etf.Atom(m.node.FullName) {
		// for the local process we should make sure if its alive
		// otherwise send 'EXIT' message with 'noproc' as a reason
		if p := m.node.registrar.GetProcessByPid(pidB); p == nil {
			m.notifyProcessExit(pidA, pidB, "noproc")
			if len(linksA) > 0 {
				m.links[pidA] = linksA
			} else {
				delete(m.links, pidA)
			}
			return
		}
	} else {
		// linking with remote process
		message := etf.Tuple{distProtoLINK, pidA, pidB}
		if err := m.node.registrar.routeRaw(pidB.Node, message); err != nil {
			// seems we have no connection with this node. notify the sender
			// with 'EXIT' message and 'noconnection' as a reason
			m.notifyProcessExit(pidA, pidB, "noconnection")
			if len(linksA) > 0 {
				m.links[pidA] = linksA
			} else {
				delete(m.links, pidA)
			}
			return
		}
	}

	m.links[pidB] = append(linksB, pidA)
}

func (m *monitor) Unlink(pidA, pidB etf.Pid) {
	m.mutexLinks.Lock()
	defer m.mutexLinks.Unlock()

	if pidB.Node != etf.Atom(m.node.FullName) {
		message := etf.Tuple{distProtoUNLINK, pidA, pidB}
		m.node.registrar.routeRaw(pidB.Node, message)
	}

	if pidA.Node == etf.Atom(m.node.FullName) {
		linksA := m.links[pidA]
		for i := range linksA {
			if linksA[i] != pidB {
				continue
			}

			linksA[i] = linksA[0]
			linksA = linksA[1:]
			if len(linksA) > 0 {
				m.links[pidA] = linksA
			} else {
				delete(m.links, pidA)
			}
			break

		}
	}

	linksB := m.links[pidB]
	for i := range linksB {
		if linksB[i] != pidA {
			continue
		}
		linksB[i] = linksB[0]
		linksB = linksB[1:]
		if len(linksB) > 0 {
			m.links[pidB] = linksB
		} else {
			delete(m.links, pidB)
		}
		break

	}
}

func (m *monitor) MonitorNode(by etf.Pid, node string) etf.Ref {
	lib.Log("[%s] MONITOR NODE : %v => %s", m.node.FullName, by, node)

	ref := m.node.MakeRef()
	m.mutexNodes.Lock()
	defer m.mutexNodes.Unlock()

	l := m.nodes[node]
	key := ref.String()
	item := monitorItem{
		pid: by,
		ref: ref,
		key: key,
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

	key := ref.String()
	if name, ok = m.ref2node[key]; !ok {
		return
	}

	l := m.nodes[name]

	// remove PID from monitoring processes list
	for i := range l {
		if l[i].key != key {
			continue
		}

		l[i] = l[0]
		l = l[1:]
		m.mutexProcesses.Lock()
		delete(m.ref2pid, key)
		m.mutexProcesses.Unlock()
		break

	}
	m.nodes[name] = l
	delete(m.ref2node, key)
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
		nodeName := string(pid.Node)
		if isFakePid(pid) {
			nodeName = fakePidToNodeName(pid)
		}
		if nodeName != name {
			continue
		}
		for i := range ps {
			m.notifyProcessTerminated(ps[i].ref, ps[i].pid, pid, "noconnection")
			delete(m.ref2pid, ps[i].key)
		}
		delete(m.processes, pid)
	}
	m.mutexProcesses.Unlock()

	// notify linked processes
	m.mutexLinks.Lock()
	for link, pids := range m.links {
		if link.Node != etf.Atom(name) {
			continue
		}

		for i := range pids {
			m.notifyProcessExit(pids[i], link, "noconnection")
			p, ok := m.links[pids[i]]

			if !ok {
				continue
			}

			for k := range p {
				if p[k].Node != etf.Atom(name) {
					continue
				}

				p[k] = p[0]
				p = p[1:]

			}

			if len(p) > 0 {
				m.links[pids[i]] = p
				continue
			}

			delete(m.links, pids[i])
		}

		delete(m.links, link)
	}
	m.mutexLinks.Unlock()
}

func (m *monitor) ProcessTerminated(terminated etf.Pid, name, reason string) {
	lib.Log("[%s] MONITOR process terminated: %v", m.node.FullName, terminated)

	// just wrapper for the iterating through monitors list
	handleMonitors := func(terminatedPid etf.Pid, items []monitorItem) {
		for i := range items {
			lib.Log("[%s] MONITOR process terminated: %v send notify to: %v", m.node.FullName, terminated, items[i].pid)
			m.notifyProcessTerminated(items[i].ref, items[i].pid, items[i].process, reason)
			delete(m.ref2pid, items[i].key)
		}
		delete(m.processes, terminatedPid)
	}

	m.mutexProcesses.Lock()
	// if terminated process had a name we should make shure if we have
	// monitors had created using this name (fake Pid uses for this purpose)
	if name != "" {
		// monitor was created by the name so we should look up using fake pid
		terminatedPid := fakeMonitorPidFromName(name, m.node.FullName)
		if items, ok := m.processes[terminatedPid]; ok {
			handleMonitors(terminatedPid, items)
		}
	}
	// check whether we have monitorItem on this Pid (terminated)
	if items, ok := m.processes[terminated]; ok {
		handleMonitors(terminated, items)
	}
	m.mutexProcesses.Unlock()

	m.mutexLinks.Lock()
	if pidLinks, ok := m.links[terminated]; ok {
		for i := range pidLinks {
			lib.Log("[%s] LINK process exited: %v send notify to: %v", m.node.FullName, terminated, pidLinks[i])
			m.notifyProcessExit(pidLinks[i], terminated, reason)

			// remove A link
			pids, ok := m.links[pidLinks[i]]
			if !ok {
				continue
			}
			for k := range pids {
				if pids[k] != terminated {
					continue
				}
				pids[k] = pids[0]
				pids = pids[1:]
				break
			}

			if len(pids) > 0 {
				m.links[pidLinks[i]] = pids
			} else {
				delete(m.links, pidLinks[i])
			}
		}
		// remove link
		delete(m.links, terminated)
	}
	m.mutexLinks.Unlock()

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
		if isFakePid(terminated) {
			// it was monitored by name and this Pid was created using
			// fakeMonitorPidFromName. It means this Pid has
			// "processName|nodeName" in Node field
			terminatedName := fakePidToName(terminated)
			message := etf.Tuple{distProtoMONITOR_EXIT, etf.Atom(terminatedName), to, ref, etf.Atom(reason)}
			m.node.registrar.routeRaw(to.Node, message)
			return
		}
		// terminated is a real Pid. send it as it is.
		message := etf.Tuple{distProtoMONITOR_EXIT, terminated, to, ref, etf.Atom(reason)}
		m.node.registrar.routeRaw(to.Node, message)
		return
	}
	// {'DOWN', MonitorRef, Type, Object, Info}
	// {'DOWN',#Ref<0.0.13893633.237772>,process,<26194.4.1>,reason}
	// Type: We do not support port/time_offset so Type could be 'process' only.
	// Object: The monitored entity, which triggered the event. When monitoring a
	// local process, Object will be equal to the pid() that was being monitored.
	// When monitoring process by name, Object will have format {RegisteredName, Node}
	// where RegisteredName is the name which has been used with monitor/2 call and
	// Node is local or remote node name.
	if isFakePid(terminated) {
		// it was monitored by name
		p := fakePidToTuple(terminated)
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

	// check if 'to' process is still alive. otherwise ignore this event
	if p := m.node.GetProcessByPid(to); p != nil && p.IsAlive() {
		p.Exit(terminated, reason)
	}
}

func fakeMonitorPidFromName(name, node string) etf.Pid {
	fakePid := etf.Pid{}
	fakePid.Node = etf.Atom(name + "|" + node) // registered process name
	fakePid.ID = 4294967295                    // 2^32 - 1
	fakePid.Serial = 4294967295                // 2^32 - 1
	fakePid.Creation = 255                     // 2^8 - 1
	return fakePid
}

func fakePidToTuple(pid etf.Pid) etf.Tuple {
	s := strings.Split(string(pid.Node), "|")
	return etf.Tuple{s[0], s[1]}
}

func fakePidExtractNames(pid etf.Pid) (string, string) {
	s := strings.Split(string(pid.Node), "|")
	return s[0], s[1]
}

func fakePidToName(pid etf.Pid) string {
	s := strings.Split(string(pid.Node), "|")
	return s[0]
}

func fakePidToNodeName(pid etf.Pid) string {
	s := strings.Split(string(pid.Node), "|")
	return s[1]
}

func isFakePid(pid etf.Pid) bool {
	if pid.ID == 4294967295 && pid.Serial == 4294967295 && pid.Creation == 255 {
		return true
	}
	return false
}
