package node

// http://erlang.org/doc/reference_manual/processes.html

import (
	"fmt"
	"math"
	"strings"
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
	link(pidA, pidB etf.Pid)
	unlink(pidA, pidB etf.Pid)

	monitorProcess(by etf.Pid, process interface{}, ref etf.Ref)
	demonitorProcess(ref etf.Ref) bool
	monitorNode(by etf.Pid, node string, ref etf.Ref)
	demonitorNode(ref etf.Ref) bool

	handleNodeDown(name string)
	handleTerminated(terminated etf.Pid, name, reason string)

	processLinks(process etf.Pid) []etf.Pid
	processMonitors(process etf.Pid) []etf.Pid
	processMonitorsByName(process etf.Pid) []gen.ProcessID
	processMonitoredBy(process etf.Pid) []etf.Pid
}

type monitor struct {
	processes      map[etf.Pid][]monitorItem
	ref2pid        map[etf.Ref]etf.Pid
	mutexProcesses sync.Mutex
	links          map[etf.Pid][]etf.Pid
	mutexLinks     sync.Mutex
	nodes          map[string][]monitorItem
	ref2node       map[etf.Ref]string
	mutexNodes     sync.Mutex

	nodename string
	router   Router
}

func newMonitor(nodename string, router Router) monitor {
	return monitor{
		processes: make(map[etf.Pid][]monitorItem),
		links:     make(map[etf.Pid][]etf.Pid),
		nodes:     make(map[string][]monitorItem),

		ref2pid:  make(map[etf.Ref]etf.Pid),
		ref2node: make(map[etf.Ref]string),

		nodename: nodename,
		router:   router,
	}
}

func (m *monitor) monitorProcess(by etf.Pid, process interface{}, ref etf.Ref) {
	if by.Node != ref.Node {
		lib.Log("[%s] Incorrect monitor request by Pid = %v and Ref = %v", m.nodename, by, ref)
		return
	}

next:
}

func (m *monitor) demonitorProcess(ref etf.Ref) bool {
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
		m.mutexProcesses.Lock()
		delete(m.ref2pid, ref)
		m.mutexProcesses.Unlock()
		break

	}
	m.nodes[name] = l
	delete(m.ref2node, ref)
	return true
}

func (m *monitor) handleNodeDown(name string) {
	lib.Log("[%s] MONITOR NODE  down: %v", m.nodename, name)

	// notify node monitors
	m.mutexNodes.Lock()
	if pids, ok := m.nodes[name]; ok {
		for i := range pids {
			lib.Log("[%s] MONITOR node down: %v. send notify to: %s", m.nodename, name, pids[i].pid)
			m.notifyNodeDown(pids[i].pid, name)
			delete(m.nodes, name)
		}
	}
	m.mutexNodes.Unlock()

	// notify process monitors
	m.mutexProcesses.Lock()
	for pid, ps := range m.processes {
		if isVirtualPid(pid) {
			processID := virtualPidToProcessID(pid)
			if processID.Node != name {
				continue
			}
			for i := range ps {
				// args: (to, terminated, reason, ref)
				m.RouteMonitorExitReg(ps[i].pid, processID, "noconnection", ps[i].ref)
			}
		} else {
			if string(pid.Node) != name {
				continue
			}
			for i := range ps {
				// args: (to, terminated, reason, ref)
				m.RouteMonitorExit(ps[i].pid, pid, "noconnection", ps[i].ref)
			}
		}
		delete(m.ref2pid, ps[i].ref)
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
			m.notifyLinkTerminated(pids[i], link, "noconnection")
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

func (m *monitor) handleTerminated(terminated etf.Pid, name, reason string) {
	lib.Log("[%s] MONITOR process terminated: %v", m.nodename, terminated)

	// just wrapper for the iterating through monitors list
	handleMonitors := func(terminatedPid etf.Pid, items []monitorItem) {
		for i := range items {
			lib.Log("[%s] MONITOR process terminated: %s. send notify to: %s", m.nodename, terminated, items[i].pid)
			m.notifyMonitorTerminated(items[i].ref, items[i].pid, terminatedPid, reason)
			delete(m.ref2pid, items[i].ref)
		}
		delete(m.processes, terminatedPid)
	}

	m.mutexProcesses.Lock()
	// if terminated process had a name we should make shure to clean up them all
	if name != "" {
		// monitor was created by the name so we should look up using virtual pid
		terminatedPid := virtualPid(gen.ProcessID{name, m.nodename})
		if items, ok := m.processes[terminatedPid]; ok {
			handleMonitors(terminatedPid, items)
		}
	}
	// check whether we have monitorItem on this process by Pid (terminated)
	if items, ok := m.processes[terminated]; ok {
		handleMonitors(terminated, items)
	}
	m.mutexProcesses.Unlock()

	m.mutexLinks.Lock()
	if pidLinks, ok := m.links[terminated]; ok {
		for i := range pidLinks {
			lib.Log("[%s] LINK process exited: %s. send notify to: %s", m.nodename, terminated, pidLinks[i])
			m.notifyLinkTerminated(pidLinks[i], terminated, reason)

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
		if isVirtualPid(p) {
			continue
		}
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

	for p, by := range m.processes {
		if !isVirtualPid(p) {
			continue
		}
		for b := range by {
			if by[b].pid == process {
				processID := virtualPidToProcessID(p)
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
		monitors := []etf.Pid{}
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
	return false
}

func (m *monitor) notifyNodeDown(to etf.Pid, node string) {
	message := gen.MessageNodeDown{node}
	m.router.RouteSend(etf.Pid{}, to, message)
}

func (m *monitor) RouteMonitorExit(to etf.Pid, terminated etf.Pid, reason string, ref etf.Ref) error {
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

func (m *monitor) notifyLinkTerminated(to etf.Pid, terminated etf.Pid, reason string) {
	// for remote: {3, FromPid, ToPid, Reason}
	if to.Node != etf.Atom(m.nodename) {
		if reason == "noconnection" {
			return
		}
		message := etf.Tuple{distProtoEXIT, terminated, to, etf.Atom(reason)}
		m.router.routeRaw(to.Node, message)
		return
	}

	// check if 'to' process is still alive. otherwise ignore this event
	if p := m.router.ProcessByPid(to); p != nil {
		p.exit(terminated, reason)
	}
}

func virtualPid(p gen.ProcessID) etf.Pid {
	pid := etf.Pid{}
	pid.Node = etf.Atom(p.Name + "|" + p.Node) // registered process name
	pid.ID = math.MaxUint64
	pid.Creation = math.MaxUint32
	return pid
}

func virtualPidToProcessID(pid etf.Pid) gen.ProcessID {
	s := strings.Split(string(pid.Node), "|")
	if len(s) != 2 {
		return gen.ProcessID{}
	}
	return gen.ProcessID{s[0], s[1]}
}

func isVirtualPid(pid etf.Pid) bool {
	if pid.ID == math.MaxUint64 && pid.Creation == math.MaxUint32 {
		return true
	}
	return false
}

//
// implementation of Router interface:
// RouteLink
// RouteUnlink
//
func (m *monitor) RouteLink(pidA etf.Pid, pidB etf.Pid) error {
	lib.Log("[%s] LINK process: %v => %v", m.nodename, pidA, pidB)

	// http://erlang.org/doc/reference_manual/processes.html#links
	// Links are bidirectional and there can only be one link between
	// two processes. Repeated calls to link(Pid) have no effect.

	// If the link already exists or a process attempts to create
	// a link to itself, nothing is done.
	if pidA == pidB {
		return fmt.Errorf("Can not link to itself")
	}

	m.mutexLinks.Lock()
	defer m.mutexLinks.Unlock()

	linksA := m.links[pidA]
	if pidA.Node == etf.Atom(m.nodename) {
		// check if these processes are linked already (source)
		for i := range linksA {
			if linksA[i] == pidB {
				return fmt.Errorf("Already linked")
			}
		}

		m.links[pidA] = append(linksA, pidB)
	}

	// check if these processes are linked already (destination)
	linksB := m.links[pidB]
	for i := range linksB {
		if linksB[i] == pidA {
			return fmt.Errorf("Already linked")
		}
	}

	if pidB.Node == etf.Atom(m.nodename) {
		// for the local process we should make sure if its alive
		// otherwise send 'EXIT' message with 'noproc' as a reason
		if p := m.router.ProcessByPid(pidB); p == nil {
			m.notifyLinkTerminated(pidA, pidB, "noproc")
			if len(linksA) > 0 {
				m.links[pidA] = linksA
			} else {
				delete(m.links, pidA)
			}
			return nil
		}
	} else {
		// linking with remote process
		m.router

		message := etf.Tuple{distProtoLINK, pidA, pidB}
		if err := m.router.routeRaw(pidB.Node, message); err != nil {
			// seems we have no connection with this node. notify the sender
			// with 'EXIT' message and 'noconnection' as a reason
			m.notifyLinkTerminated(pidA, pidB, "noconnection")
			if len(linksA) > 0 {
				m.links[pidA] = linksA
			} else {
				delete(m.links, pidA)
			}
			return nil
		}
	}

	m.links[pidB] = append(linksB, pidA)
	return nil
}

func (m *monitor) RouteUnlink(pidA etf.Pid, pidB etf.Pid) error {
	m.mutexLinks.Lock()
	defer m.mutexLinks.Unlock()

	if pidB.Node != etf.Atom(m.nodename) {
		message := etf.Tuple{distProtoUNLINK, pidA, pidB}
		m.router.routeRaw(pidB.Node, message)
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

func (m *monitor) RouteMonitor(by etf.Pid, pid etf.Pid, ref etf.Ref) error {
	lib.Log("[%s] MONITOR process: %s => %s", m.nodename, by, process)

	// If 'process' belongs to this node we should make sure if its alive.
	// http://erlang.org/doc/reference_manual/processes.html#monitors
	// If Pid does not exist a gen.MessageDown must be
	// send immediately with Reason set to noproc.
	if p := m.router.ProcessByPid(pid); string(pid.Node) == m.nodename && p == nil {
		m.notifyMonitorTerminated(ref, by, t, "noproc")
		return
	}

	m.mutexProcesses.Lock()
	l := m.processes[t]
	item := monitorItem{
		pid: by,
		ref: ref,
	}
	m.processes[t] = append(l, item)
	m.ref2pid[ref] = t
	m.mutexProcesses.Unlock()

	if isVirtualPid(t) {
		// this Pid was created as a virtual. we use virtual pids for the
		// monitoring process by the registered name.
		return
	}

	if string(t.Node) == m.nodename {
		// this is the local process so we have nothing to do
		return
	}

	// request monitoring the remote process
	message := etf.Tuple{distProtoMONITOR, by, t, ref}
	if err := m.router.routeRaw(t.Node, message); err != nil {
		m.notifyMonitorTerminated(ref, by, t, "noconnection")
		m.mutexProcesses.Lock()
		delete(m.ref2pid, ref)
		m.mutexProcesses.Unlock()
	}

	return nil
}

func (m *monitor) RouteMonitorReg(by etf.Pid, process gen.ProcessID, ref etf.Ref) error {
	vPid := virtualPid(process)
	if process.Node == m.nodename {
		// If process does not exist a gen.MessageDown must be
		// send immediately with Reason set to noproc.
		if p := m.router.ProcessByName(process.Name); p == nil {
			m.notifyMonitorTerminated(ref, by, vPid, "noproc")
			return
		}
	}

	message := etf.Tuple{distProtoMONITOR, by, etf.Atom(t.Name), ref}
	if err := m.router.routeRaw(etf.Atom(t.Node), message); err != nil {
		m.notifyMonitorTerminated(ref, by, vPid, "noconnection")
		return
	}

	return nil
}

func (m *monitor) RouterDemonitor(ref etf.Ref) error {
	var process interface{}
	var node etf.Atom

	m.mutexProcesses.Lock()
	defer m.mutexProcesses.Unlock()

	pid, knownRef := m.ref2pid[ref]
	if !knownRef {
		// unknown monitor reference
		return false
	}

	// cheching for monitorItem list
	items := m.processes[pid]

	// remove PID from monitoring processes list
	for i := range items {
		if items[i].ref != ref {
			continue
		}
		process = pid
		node = pid.Node
		if isVirtualPid(pid) {
			processID := virtualPidToProcessID(pid)
			process = etf.Atom(processID.Name)
			node = etf.Atom(processID.Node)
		}

		if string(node) != m.nodename {
			message := etf.Tuple{distProtoDEMONITOR, items[i].pid, process, ref}
			m.router.routeRaw(node, message)
		}

		items[i] = items[0]
		items = items[1:]
		delete(m.ref2pid, ref)
		break

	}

	if len(items) == 0 {
		delete(m.processes, pid)
	} else {
		m.processes[pid] = items
	}

	return true
}
