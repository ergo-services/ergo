package node

// http://erlang.org/doc/reference_manual/processes.html

import (
	"fmt"
	"sync"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/lib"
)

type monitorItem struct {
	pid etf.Pid // by
	ref etf.Ref
}

type monitorInternal interface {
	// RouteLink
	RouteLink(pidA etf.Pid, pidB etf.Pid) error
	// RouteUnlink
	RouteUnlink(pidA etf.Pid, pidB etf.Pid) error
	// RouteExit
	RouteExit(to etf.Pid, terminated etf.Pid, reason string) error
	// RouteMonitorReg
	RouteMonitorReg(by etf.Pid, process gen.ProcessID, ref etf.Ref) error
	// RouteMonitor
	RouteMonitor(by etf.Pid, process etf.Pid, ref etf.Ref) error
	// RouteDemonitor
	RouteDemonitor(by etf.Pid, ref etf.Ref) error
	// RouteMonitorExitReg
	RouteMonitorExitReg(to etf.Pid, terminated gen.ProcessID, reason string, ref etf.Ref) error
	// RouteMonitorExit
	RouteMonitorExit(to etf.Pid, terminated etf.Pid, reason string, ref etf.Ref) error
	// RouteNodeDown
	RouteNodeDown(name string)

	// IsMonitor
	IsMonitor(ref etf.Ref) bool

	monitorNode(by etf.Pid, node string, ref etf.Ref)
	demonitorNode(ref etf.Ref) bool

	handleTerminated(terminated etf.Pid, name, reason string)

	processLinks(process etf.Pid) []etf.Pid
	processMonitors(process etf.Pid) []etf.Pid
	processMonitorsByName(process etf.Pid) []gen.ProcessID
	processMonitoredBy(process etf.Pid) []etf.Pid
}

type monitor struct {
	// monitors by pid
	processes      map[etf.Pid][]monitorItem
	ref2pid        map[etf.Ref]etf.Pid
	mutexProcesses sync.Mutex
	// monitors by name
	names      map[gen.ProcessID]monitorItem
	ref2name   map[etf.Ref]gen.ProcessID
	mutexNames sync.Mutex

	// links
	links      map[etf.Pid][]etf.Pid
	mutexLinks sync.Mutex

	// monitors of nodes
	nodes      map[string][]monitorItem
	ref2node   map[etf.Ref]string
	mutexNodes sync.Mutex

	nodename string
	router   CoreRouter
}

func newMonitor(nodename string, router CoreRouter) monitorInternal {
	return monitor{
		processes: make(map[etf.Pid][]monitorItem),
		names:     make(map[gen.ProcessID]monitorItem),
		links:     make(map[etf.Pid][]etf.Pid),
		nodes:     make(map[string][]monitorItem),

		ref2pid:  make(map[etf.Ref]etf.Pid),
		ref2name: make(map[etf.Ref]gen.ProcessID),
		ref2node: make(map[etf.Ref]string),

		nodename: nodename,
		router:   router,
	}
}

func (m *monitor) monitorNode(by etf.Pid, node string, ref etf.Ref) {
	lib.Log("[%s] MONITOR NODE : %v => %s", m.nodename, by, node)

	m.mutexNodes.Lock()
	defer m.mutexNodes.Unlock()

	l := m.nodes[node]
	item := monitorItem{
		pid: by,
		ref: ref,
	}
	m.nodes[node] = append(l, item)
	m.ref2node[ref] = node

	_, err := m.router.GetConnection(string(pidB.Node))
	if err != nil {
		m.RouteNodeDown(node)
	}
	return ref
}

func (m *monitor) demonitorNode(ref etf.Ref) bool {
	var name string
	var ok bool

	m.mutexNodes.Lock()
	defer m.mutexNodes.Unlock()

	if name, ok = m.ref2node[ref]; !ok {
		return false
	}

	l := m.nodes[name]

	// remove PID from monitoring processes list
	for i := range l {
		if l[i].ref != ref {
			continue
		}

		l[i] = l[0]
		l = l[1:]
		break
	}
	delete(m.ref2node, ref)

	if len(l) == 0 {
		delete(m.nodes, name)
	} else {
		m.nodes[name] = l
	}

	return true
}

func (m *monitor) RouteNodeDown(name string) {
	lib.Log("[%s] MONITOR NODE  down: %v", m.nodename, name)

	// notify node monitors
	m.mutexNodes.Lock()
	if pids, ok := m.nodes[name]; ok {
		for i := range pids {
			lib.Log("[%s] MONITOR node down: %v. send notify to: %s", m.nodename, name, pids[i].pid)
			message := gen.MessageNodeDown{Name: name}
			m.router.RouteSend(etf.Pid{}, pids[i].pid, message)
		}
		delete(m.nodes, name)
	}
	m.mutexNodes.Unlock()

	// notify processes created monitors by pid
	m.mutexProcesses.Lock()
	for pid, ps := range m.processes {
		if string(pid.Node) != name {
			continue
		}
		for i := range ps {
			// args: (to, terminated, reason, ref)
			m.RouteMonitorExit(ps[i].pid, pid, "noconnection", ps[i].ref)
		}
		delete(m.ref2pid, ps[i].ref)
		delete(m.processes, pid)
	}
	m.mutexProcesses.Unlock()

	// notify processes created monitors by name
	m.mutexNames.Lock()
	for processID, ps := range m.names {
		if processID.Node != name {
			continue
		}
		for i := range ps {
			// args: (to, terminated, reason, ref)
			m.RouteMonitorExitReg(ps[i].pid, processID, "noconnection", ps[i].ref)
		}
		delete(m.ref2name, ps[i].ref)
		delete(m.names, processID)
	}
	m.mutexNames.Unlock()

	// notify linked processes
	m.mutexLinks.Lock()
	for link, pids := range m.links {
		if link.Node != etf.Atom(name) {
			continue
		}

		for i := range pids {
			m.RouteExit(pids[i], link, "noconnection")
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

func (m *monitor) handleTerminated(terminated etf.Pid, name string, reason string) {
	lib.Log("[%s] MONITOR process terminated: %v", m.nodename, terminated)

	// if terminated process had a name we should make shure to clean up them all
	m.mutexNames.Lock()
	if name != "" {
		terminatedProcessID := gen.ProcessID{name, m.nodename}
		if items, ok := m.names[terminatedProcessID]; ok {
			for i := range items {
				lib.Log("[%s] MONITOR process terminated: %s. send notify to: %s", m.nodename, terminatedProcessID, items[i].pid)
				m.RouteMonitorExit(items[i].pid, terminatedProcessID, reason, items[i].ref)
				delete(m.ref2name, items[i].ref)
			}
			delete(m.names, terminatedProcessID)
		}
	}
	m.mutexNames.Unlock()
	// check whether we have monitorItem on this process by Pid (terminated)
	m.mutexProcesses.Lock()
	if items, ok := m.processes[terminated]; ok {

		for i := range items {
			lib.Log("[%s] MONITOR process terminated: %s. send notify to: %s", m.nodename, terminated, items[i].pid)
			m.RouteMonitorExit(items[i].pid, terminated, reason, items[i].ref)
			delete(m.ref2pid, items[i].ref)
		}
		delete(m.processes, terminatedPid)
	}
	m.mutexProcesses.Unlock()

	m.mutexLinks.Lock()
	if pidLinks, ok := m.links[terminated]; ok {
		for i := range pidLinks {
			lib.Log("[%s] LINK process exited: %s. send notify to: %s", m.nodename, terminated, pidLinks[i])
			m.RouteExit(pidLinks[i], terminated, reason)

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
	defer m.mutexLinks.Unlock()

}

func (m *monitor) processLinks(process etf.Pid) []etf.Pid {
	m.mutexLinks.Lock()
	defer m.mutexLinks.Unlock()

	if l, ok := m.links[process]; ok {
		return l
	}
	return nil
}

func (m *monitor) processMonitors(process etf.Pid) []etf.Pid {
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

func (m *monitor) processMonitorsByName(process etf.Pid) []gen.ProcessID {
	monitors := []gen.ProcessID{}
	m.mutexProcesses.Lock()
	defer m.mutexProcesses.Unlock()

	for processID, by := range m.names {
		for b := range by {
			if by[b].pid == process {
				monitors = append(monitors, processID)
			}
		}
	}
	return monitors
}

func (m *monitor) processMonitoredBy(process etf.Pid) []etf.Pid {
	monitors := []etf.Pid{}
	m.mutexProcesses.Lock()
	defer m.mutexProcesses.Unlock()
	if m, ok := m.processes[process]; ok {
		for i := range m {
			monitors = append(monitors, m[i].pid)
		}

	}
	return monitors
}

func (m *monitor) IsMonitor(ref etf.Ref) bool {
	m.mutexProcesses.Lock()
	defer m.mutexProcesses.Unlock()
	if _, ok := m.ref2pid[ref]; ok {
		return true
	}
	if _, ok := m.ref2name[ref]; ok {
		return true
	}
	return false
}

//
// implementation of CoreRouter interface:
//
// RouteLink
// RouteUnlink
// RouteExit
// RouteMonitor
// RouteMonitorReg
// RouteDemonitor
// RouteMonitorExit
// RouteMonitorExitReg
//

func (m *monitor) RouteLink(pidA etf.Pid, pidB etf.Pid) error {
	lib.Log("[%s] LINK process: %v => %v", m.nodename, pidA, pidB)

	// http://erlang.org/doc/reference_manual/processes.html#links
	// Links are bidirectional and there can only be one link between
	// two processes. Repeated calls to link(Pid) have no effect.

	// Returns error if link is already exist or a process attempts to create
	// a link to itself

	if pidA == pidB {
		return fmt.Errorf("Can not link to itself")
	}

	m.mutexLinks.Lock()
	linksA := m.links[pidA]
	m.mutexLinks.Unlock()

	if pidA.Node == etf.Atom(m.nodename) {
		// check if these processes are linked already (source)
		for i := range linksA {
			if linksA[i] == pidB {
				return fmt.Errorf("Already linked")
			}
		}

	}

	// check if these processes are linked already (destination)
	m.mutexLinks.Lock()
	linksB := m.links[pidB]
	m.mutexLinks.Unlock()

	for i := range linksB {
		if linksB[i] == pidA {
			return fmt.Errorf("Already linked")
		}
	}

	if pidB.Node == etf.Atom(m.nodename) {
		// for the local process we should make sure if its alive
		// otherwise send 'EXIT' message with 'noproc' as a reason
		if p := m.router.ProcessByPid(pidB); p == nil {
			m.RouteExit(pidA, pidB, "noproc")
			return ErrProcessUnknown
		}
		m.links[pidA] = append(linksA, pidB)
		m.links[pidB] = append(linksB, pidA)
		return nil
	}

	// linking with remote process
	connection, err := m.router.GetConnection(string(pidB.Node))
	if err != nil {
		m.RouteExit(pidA, pidB, "noconnection")
		return err
	}

	if err := connection.Link(pidA, pidB); err != nil {
		m.RouteExit(pidA, pidB, err.Error())
		return err
	}

	m.links[pidA] = append(linksA, pidB)
	m.links[pidB] = append(linksB, pidA)
	return nil
}

func (m *monitor) RouteUnlink(pidA etf.Pid, pidB etf.Pid) error {
	m.mutexLinks.Lock()
	defer m.mutexLinks.Unlock()

	if pidB.Node != etf.Atom(m.nodename) {
		connection, err := m.router.GetConnection(string(pidB.Node))
		if err != nil {
			m.RouteExit(pidA, pidB, "noconnection")
			return err
		}
	}

	if pidA.Node == etf.Atom(m.nodename) {
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
	return nil
}

func (m *monitor) RouteExit(to etf.Pid, terminated etf.Pid, reason string) {
	// for remote: {3, FromPid, ToPid, Reason}
	if to.Node != etf.Atom(m.nodename) {
		if reason == "noconnection" {
			return
		}
		connection, err := m.router.GetConnection(string(to.Node))
		if err != nil {
			return err
		}

		return connection.LinkExit(to, terminated, reason)
	}

	// check if 'to' process is still alive. otherwise ignore this event
	if p := m.router.ProcessByPid(to); p != nil {
		p.exit(terminated, reason)
	}
}

func (m *monitor) RouteMonitor(by etf.Pid, pid etf.Pid, ref etf.Ref) error {
	lib.Log("[%s] MONITOR process: %s => %s", m.nodename, by, process)

	// If 'process' belongs to this node we should make sure if its alive.
	// http://erlang.org/doc/reference_manual/processes.html#monitors
	// If Pid does not exist a gen.MessageDown must be
	// send immediately with Reason set to noproc.
	if p := m.router.ProcessByPid(pid); string(pid.Node) == m.nodename && p == nil {
		m.RouteMonitorExit(by, pid, "noproc", ref)
		return
	}

	if string(pid.Node) != m.nodename {
		connection, err := m.router.GetConnection(string(pid.Node))
		if err != nil {
			return err
		}

		if err := connection.Monitor(by, pid, ref); err != nil {
			m.RouteMonitorExit(by, pid, "noconnection", ref)
			return err
		}
	}

	m.mutexProcesses.Lock()
	l := m.processes[pid]
	item := monitorItem{
		pid: by,
		ref: ref,
	}
	m.processes[t] = append(l, item)
	m.ref2pid[ref] = pid
	m.mutexProcesses.Unlock()

	return nil
}

func (m *monitor) RouteMonitorReg(by etf.Pid, process gen.ProcessID, ref etf.Ref) error {
	// If 'process' belongs to this node and does not exist a gen.MessageDown must be
	// send immediately with Reason set to noproc.
	if p := m.router.ProcessByName(process.Name); process.Node == m.nodename && p == nil {
		m.RouteMonitorExitReg(by, process, "noproc", ref)
		return
	}
	if process.Node != m.nodename {
		connection, err := m.router.GetConnection(process.Node)
		if err != nil {
			return err
		}

		if err := connection.MonitorReg(by, process, ref); err != nil {
			m.RouteMonitorExitReg(by, process, "noconnection", ref)
			return err
		}
	}

	m.mutexNames.Lock()
	l := m.names[vpid]
	item := monitorItem{
		pid: by,
		ref: ref,
	}
	m.names[t] = append(l, item)
	m.ref2name[ref] = process
	m.mutexNames.Unlock()

	return nil
}

func (m *monitor) RouteDemonitor(by etf.Pid, ref etf.Ref) error {
	var pid etf.Pid
	var processID gen.ProcessID
	var knownRefByname, knownRefByPid bool

	m.mutexProcesses.Lock()
	pid, knownRefByPid = m.ref2pid[ref]
	m.mutexProcesses.Unlock()

	if knownRefByPid == false {
		m.mutexNames.Lock()
		processID, knownRefByName := m.ref2name[ref]
		m.mutexNames.Unlock
		if knownRefByName == false {
			// unknown monitor reference
			return ErrMonitorUnknown
		}

		items := m.names[processID]
		for i := range items {
			if items[i].pid != by {
				continue
			}
			if items[i].ref != ref {
				continue
			}

			items[i] = items[0]
			items = items[1:]

			m.mutexNames.Lock()
			if len(items) == 0 {
				delete(m.names, processID)
			} else {
				m.names[processID] = items
			}
			delete(m.ref2name, ref)
			m.mutexNames.Unlock()

			if processID.Node != m.nodename {
				connection, err := m.router.GetConnection(processID.Nod)
				if err != nil {
					return err
				}
				return connection.DemonitorReg(by, processID, ref)
			}
			return nil
		}
		return nil
	}

	// cheching for monitorItem list
	items := m.processes[pid]

	// remove PID from monitoring processes list
	for i := range items {
		if items[i].pid != by {
			continue
		}
		if items[i].ref != ref {
			continue
		}

		items[i] = items[0]
		items = items[1:]

		m.mutexProcesses.Lock()
		if len(items) == 0 {
			delete(m.processes, pid)
		} else {
			m.processes[pid] = items
		}
		delete(m.ref2pid, ref)
		m.mutexProcesses.Unlock()

		if string(pid.Node) != m.nodename {
			connection, err := m.router.GetConnection(processID.Nod)
			if err != nil {
				return err
			}
			return connection.Demonitor(by, processID, ref)
		}

		return nil
	}
	return true
}

func (m *monitor) RouteMonitorExit(to etf.Pid, terminated etf.Pid, reason string, ref etf.Ref) error {
	if string(to.Node) != m.nodename {
		// remote
		if reason == "noconnection" {
			// do nothing. it was a monitor created by the remote node we lost connection to.
			return nil
		}

		connection, err := m.router.GetConnection(string(to.Node))
		if err != nil {
			return err
		}

		return connection.MonitorExit(to, terminated, reason, ref)
	}

	// local
	down := gen.MessageDown{
		Ref:    ref,
		Pid:    terminated,
		Reason: reason,
	}
	return m.router.RouteSend(terminated, to, down)
}

func (m *monitor) RouteMonitorExitReg(to etf.Pid, terminated gen.ProcessID, reason string, ref etf.Ref) error {
	if string(to.Node) != m.nodename {
		// remote
		if reason == "noconnection" {
			// do nothing
			return nil
		}

		connection, err := m.router.GetConnection(string(to.Node))
		if err != nil {
			return err
		}

		return connection.MonitorExitReg(to, processID, reason, ref)
	}

	// local
	down := gen.MessageDown{
		Ref:       ref,
		ProcessID: terminated,
		Reason:    reason,
	}
	return m.router.RouteSend(terminated, to, down)
}
