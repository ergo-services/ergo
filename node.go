package ergonode

import (
	"context"
	"errors"
	"fmt"
	"log"
	"runtime"
	"sync/atomic"
	"syscall"

	"github.com/halturin/ergonode/dist"
	"github.com/halturin/ergonode/etf"
	"github.com/halturin/ergonode/lib"

	"net"
	"strconv"
	"strings"
	"time"
)

type Node struct {
	dist.EPMD
	listener net.Listener
	Cookie   string

	registrar *registrar
	monitor   *monitor
	context   context.Context
	Stop      context.CancelFunc

	StartedAt time.Time
	uniqID    int64
}

type NodeOptions struct {
	ListenRangeBegin uint16
	ListenRangeEnd   uint16
	Hidden           bool
	EPMDPort         uint16
}

const (
	defaultListenRangeBegin uint16 = 15000
	defaultListenRangeEnd   uint16 = 65000
	defaultEPMDPort         uint16 = 4369
	versionOTP              int    = 21
	versionERTSprefix              = "ergo"
)

// CreateNode create new node with name and cookie string
func CreateNode(name string, cookie string, opts NodeOptions) *Node {
	return CreateNodeWithContext(context.Background(), name, cookie, opts)
}

// CreateNodeWithContext create new node with specified context, name and cookie string
func CreateNodeWithContext(ctx context.Context, name string, cookie string, opts NodeOptions) *Node {

	lib.Log("Start with name '%s' and cookie '%s'", name, cookie)
	nodectx, nodestop := context.WithCancel(ctx)

	node := Node{
		Cookie:    cookie,
		context:   nodectx,
		Stop:      nodestop,
		StartedAt: time.Now(),
		uniqID:    time.Now().UnixNano(),
	}

	// start networking if name is defined
	if name != "" {
		// set defaults
		if opts.ListenRangeBegin == 0 {
			opts.ListenRangeBegin = defaultListenRangeBegin
		}
		if opts.ListenRangeEnd == 0 {
			opts.ListenRangeEnd = defaultListenRangeEnd
		}
		lib.Log("Listening range: %d...%d", opts.ListenRangeBegin, opts.ListenRangeEnd)

		if opts.EPMDPort == 0 {
			opts.EPMDPort = defaultEPMDPort
		}
		if opts.EPMDPort != 4369 {
			lib.Log("Using custom EPMD port: %d", opts.EPMDPort)
		}

		if opts.Hidden {
			lib.Log("Running as hidden node")
		}
		ns := strings.Split(name, "@")
		if len(ns) != 2 {
			panic("FQDN for node name is required (example: node@hostname)")
		}

		if listenPort := node.listen(ns[1], opts.ListenRangeBegin, opts.ListenRangeEnd); listenPort == 0 {
			panic("Can't listen port")
		} else {
			// start EPMD
			node.EPMD.Init(nodectx, name, listenPort, opts.EPMDPort, opts.Hidden)
		}

	}

	node.registrar = createRegistrar(&node)
	node.monitor = createMonitor(&node)

	netKernelSup := &netKernelSup{}
	node.Spawn("net_kernel_sup", ProcessOptions{}, netKernelSup)

	return &node
}

// Spawn create new process
func (n *Node) Spawn(name string, opts ProcessOptions, object interface{}, args ...interface{}) (*Process, error) {

	process, err := n.registrar.RegisterProcessExt(name, object, opts)
	if err != nil {
		return nil, err
	}

	go func() {
		pid := process.Self()

		// defer func() {
		// 	if r := recover(); r != nil {
		// 		lib.Log("Recovered process: %v %#v ", process.self, r)
		// 		n.registrar.UnregisterProcess(pid)
		// 		n.monitor.ProcessTerminated(pid, etf.Atom(name), "panic")
		// 		process.Kill()
		// 	}
		// 	close(process.ready)
		// }()

		reason := object.(ProcessBehaviour).loop(process, object, args...)
		n.registrar.UnregisterProcess(pid)
		n.monitor.ProcessTerminated(pid, etf.Atom(name), reason)
		if reason != "kill" {
			process.Kill()
		}

	}()

	<-process.ready

	return process, nil
}

// Register register associates the name with pid
func (n *Node) Register(name string, pid etf.Pid) error {
	return n.registrar.RegisterName(name, pid)
}

func (n *Node) Unregister(name string) {
	n.registrar.UnregisterName(name)
}

func (n *Node) IsProcessAlive(pid etf.Pid) bool {
	if pid.Node != etf.Atom(n.FullName) {
		return false
	}
	if n.registrar.GetProcessByPid(pid) == nil {
		return false
	}
	return true
}

func (n *Node) ProcessInfo(pid etf.Pid) (ProcessInfo, error) {
	p := n.registrar.GetProcessByPid(pid)
	if p == nil {
		return ProcessInfo{}, errors.New("undefined")
	}
	info := ProcessInfo{
		CurrentFunction: p.currentFunction,
		Status:          "running",
		MessageQueueLen: len(p.mailBox),
		// Links:
		// Dictionary
		TrapExit:    p.trapExit,
		GroupLeader: p.groupLeader.Self(),
		Reductions:  p.reductions,
	}
	return info, nil
}

func (n *Node) serve(c net.Conn, negotiate bool) {

	var node *dist.NodeDesc

	if negotiate {
		node = dist.NewNodeDesc(n.FullName, n.Cookie, false, c)
	} else {
		node = dist.NewNodeDesc(n.FullName, n.Cookie, false, nil)
	}

	send := make(chan []etf.Term, 10)
	// run writer routine
	go func() {
		defer c.Close()
		defer func() { n.registrar.UnregisterPeer(node.GetRemoteName()) }()

		for {
			select {
			case terms := <-send:
				err := node.WriteMessage(c, terms)
				if err != nil {
					lib.Log("Enode error (writing): %s", err.Error())
					return
				}
			case <-n.context.Done():
				return
			}

		}
	}()

	go func() {
		defer c.Close()
		defer func() { n.registrar.UnregisterPeer(node.GetRemoteName()) }()
		for {
			terms, err := node.ReadMessage(c)
			if err != nil {
				lib.Log("Enode error (reading): %s", err.Error())
				break
			}
			n.handleTerms(terms)
		}
	}()

	p := peer{
		conn: c,
		send: send,
	}

	<-node.Ready
	n.registrar.RegisterPeer(node.GetRemoteName(), p)
}

// LoadedApplications returns a list with information about the
// applications, which are loaded using ApplicatoinLoad
func (n *Node) LoadedApplications() []ApplicationInfo {
	info := []ApplicationInfo{}
	for _, a := range n.registrar.ApplicationList() {
		if a.process != nil {
			// list only loaded and not started apps
			continue
		}
		appInfo := ApplicationInfo{
			Name:        a.Name,
			Description: a.Description,
			Version:     a.Version,
		}
		info = append(info, appInfo)
	}
	return info
}

// WhichApplications returns a list with information about the applications that are currently running.
func (n *Node) WhichApplications() []ApplicationInfo {
	info := []ApplicationInfo{}
	for _, a := range n.registrar.ApplicationList() {
		if a.process == nil {
			// list only started apps
			continue
		}
		appInfo := ApplicationInfo{
			Name:        a.Name,
			Description: a.Description,
			Version:     a.Version,
		}
		info = append(info, appInfo)
	}
	return info
}

// ApplicationLoad loads the application specification for an application
// into the node. It also loads the application specifications for any included applications
func (n *Node) ApplicationLoad(app interface{}, args ...interface{}) error {

	spec, err := app.(ApplicationBehavior).Load(args...)
	if err != nil {
		return err
	}
	spec.app = app.(ApplicationBehavior)
	for i := range spec.Applications {
		if e := n.ApplicationLoad(spec.Applications[i], args...); e != nil && e != ErrAppAlreadyLoaded {
			return e
		}
	}

	return n.registrar.RegisterApp(spec.Name, &spec)
}

// ApplicationUnload unloads the application specification for Application from the
// node. It also unloads the application specifications for any included applications.
func (n *Node) ApplicationUnload(appName string) (bool, error) {
	spec := n.registrar.GetApplicationSpecByName(appName)
	if spec == nil {
		return false, fmt.Errorf("Unknown application name")
	}
	if spec.process != nil {
		if spec == nil {
			return false, fmt.Errorf("Application is running")
		}
	}

	n.registrar.UnregisterApp(appName)
	return true, nil
}

// ApplicationStart starts Application
func (n *Node) ApplicationStart(appName string, args ...interface{}) (*Process, error) {

	spec := n.registrar.GetApplicationSpecByName(appName)
	if spec == nil {
		return nil, fmt.Errorf("Unknown application name: %s", appName)
	}

	if spec.process != nil {
		return nil, ErrAppAlreadyStarted
	}

	for _, depAppName := range spec.Applications {
		if _, e := n.ApplicationStart(depAppName); e != nil && e != ErrAppAlreadyStarted {
			return nil, e
		}
	}

	unrefSpec := *spec
	args = append([]interface{}{unrefSpec}, args)
	appProcess, e := n.Spawn("", ProcessOptions{}, spec.app, args...)
	if e != nil {
		return nil, e
	}

	spec.process = appProcess
	return appProcess, nil
}

func (n *Node) handleTerms(terms []etf.Term) {
	// defer func() {
	// 	if r := recover(); r != nil {
	// 		lib.Log("Recovered node.handleTerms: %s", r)
	// 	}
	// }()

	if len(terms) == 0 {
		// keep alive
		return
	}

	lib.Log("Node terms: %#v", terms)

	switch t := terms[0].(type) {
	case etf.Tuple:
		switch act := t.Element(1).(type) {
		case int:
			switch act {
			case REG_SEND:
				// {6, FromPid, Unused, ToName}
				if len(terms) == 2 {
					n.registrar.route(t.Element(2).(etf.Pid), t.Element(4), terms[1])
				} else {
					lib.Log("*** ERROR: bad REG_SEND: %#v", terms)
				}

			case SEND:
				// {2, Unused, ToPid}
				// SEND has no sender pid
				n.registrar.route(etf.Pid{}, t.Element(3), terms[1])

			case LINK:
				// {1, FromPid, ToPid}
				lib.Log("LINK message (act %d): %#v", act, t)
				n.monitor.Link(t.Element(2).(etf.Pid), t.Element(3).(etf.Pid))

			case UNLINK:
				// {4, FromPid, ToPid}
				lib.Log("UNLINK message (act %d): %#v", act, t)
				n.monitor.Unink(t.Element(2).(etf.Pid), t.Element(3).(etf.Pid))

			case NODE_LINK:
				lib.Log("NODE_LINK message (act %d): %#v", act, t)

			case EXIT:
				// {3, FromPid, ToPid, Reason}
				lib.Log("EXIT message (act %d): %#v", act, t)
				terminated := t.Element(2).(etf.Pid)
				reason := t.Element(4).(etf.Atom)
				n.monitor.ProcessTerminated(terminated, etf.Atom(""), string(reason))

			case EXIT2:
				lib.Log("EXIT2 message (act %d): %#v", act, t)

			case MONITOR:
				// {19, FromPid, ToProc, Ref}, where FromPid = monitoring process
				// and ToProc = monitored process pid or name (atom)
				lib.Log("MONITOR message (act %d): %#v", act, t)
				n.monitor.MonitorProcessWithRef(t.Element(2).(etf.Pid), t.Element(3), t.Element(4).(etf.Ref))

			case DEMONITOR:
				// {20, FromPid, ToProc, Ref}, where FromPid = monitoring process
				// and ToProc = monitored process pid or name (atom)
				lib.Log("DEMONITOR message (act %d): %#v", act, t)
				n.monitor.DemonitorProcess(t.Element(4).(etf.Ref))

			case MONITOR_EXIT:
				// {21, FromProc, ToPid, Ref, Reason}, where FromProc = monitored process
				// pid or name (atom), ToPid = monitoring process, and Reason = exit reason for the monitored process
				lib.Log("MONITOR_EXIT message (act %d): %#v", act, t)
				terminated := t.Element(2).(etf.Pid)
				reason := t.Element(5).(etf.Atom)
				// FIXME: we must handle case when 'terminated' is atom
				n.monitor.ProcessTerminated(terminated, etf.Atom(""), string(reason))

			// Not implemented yet, just stubs. TODO.
			case SEND_SENDER:
				lib.Log("SEND_SENDER message (act %d): %#v", act, t)
			case SEND_SENDER_TT:
				lib.Log("SEND_SENDER_TT message (act %d): %#v", act, t)
			case PAYLOAD_EXIT:
				lib.Log("PAYLOAD_EXIT message (act %d): %#v", act, t)
			case PAYLOAD_EXIT_TT:
				lib.Log("PAYLOAD_EXIT_TT message (act %d): %#v", act, t)
			case PAYLOAD_EXIT2:
				lib.Log("PAYLOAD_EXIT2 message (act %d): %#v", act, t)
			case PAYLOAD_EXIT2_TT:
				lib.Log("PAYLOAD_EXIT2_TT message (act %d): %#v", act, t)
			case PAYLOAD_MONITOR_P_EXIT:
				lib.Log("PAYLOAD_MONITOR_P_EXIT message (act %d): %#v", act, t)

			default:
				lib.Log("Unhandled node message (act %d): %#v", act, t)
			}
		case etf.Atom:
			switch act {
			case etf.Atom("$connection"):
				// Ready channel waiting for registration of this connection
				ready := (t[2]).(chan bool)
				ready <- true
			}
		default:
			lib.Log("UNHANDLED ACT: %#v", t.Element(1))
		}
	}
}

// ProvideRPC register given module/function as RPC method
func (n *Node) ProvideRPC(module string, function string, fun rpcFunction) error {
	lib.Log("RPC provide: %s:%s %#v", module, function, fun)
	message := etf.Tuple{
		etf.Atom("$provide"),
		etf.Atom(module),
		etf.Atom(function),
		fun,
	}
	rex := n.registrar.GetProcessByName("rex")
	if rex == nil {
		return fmt.Errorf("RPC module is disabled")
	}

	if v, err := rex.Call(rex.Self(), message); v != etf.Atom("ok") || err != nil {
		return fmt.Errorf("value: %s err: %s", v, err)
	}

	return nil
}

// RevokeRPC unregister given module/function
func (n *Node) RevokeRPC(module, function string) error {
	lib.Log("RPC revoke: %s:%s", module, function)

	rex := n.registrar.GetProcessByName("rex")
	if rex == nil {
		return fmt.Errorf("RPC module is disabled")
	}

	message := etf.Tuple{
		etf.Atom("$revoke"),
		etf.Atom(module),
		etf.Atom(function),
	}

	if v, err := rex.Call(rex.Self(), message); v != etf.Atom("ok") || err != nil {
		return fmt.Errorf("value: %s err: %s", v, err)
	}

	return nil
}

// MakeRef returns atomic reference etf.Ref within this node
func (n *Node) MakeRef() (ref etf.Ref) {
	ref.Node = etf.Atom(n.FullName)
	ref.Creation = 1
	nt := atomic.AddInt64(&n.uniqID, 1)
	id1 := uint32(uint64(nt) & ((2 << 17) - 1))
	id2 := uint32(uint64(nt) >> 46)
	ref.Id = []uint32{id1, id2, 0}

	return
}

func (n *Node) VersionERTS() string {
	return fmt.Sprintf("%s-%s", versionERTSprefix, runtime.Version())
}

func (n *Node) VersionOTP() int {
	return versionOTP
}

func (n *Node) connect(to etf.Atom) error {
	var port int
	var err error
	var dialer = net.Dialer{
		Control: setSocketOptions,
	}
	if port, err = n.ResolvePort(string(to)); port < 0 {
		return fmt.Errorf("Can't resolve port: %s", err)
	}
	ns := strings.Split(string(to), "@")

	c, err := dialer.DialContext(n.context, "tcp", net.JoinHostPort(ns[1], strconv.Itoa(int(port))))
	if err != nil {
		lib.Log("Error calling net.Dialer.DialerContext : %s", err.Error())
		return err
	}

	n.serve(c, true)
	return nil
}

func (n *Node) listen(name string, listenRangeBegin, listenRangeEnd uint16) uint16 {

	lc := net.ListenConfig{Control: setSocketOptions}

	for p := listenRangeBegin; p <= listenRangeEnd; p++ {
		l, err := lc.Listen(n.context, "tcp", net.JoinHostPort(name, strconv.Itoa(int(p))))
		if err != nil {
			continue
		}
		go func() {
			for {
				c, err := l.Accept()

				lib.Log("Accepted new connection from %s", c.RemoteAddr().String())
				if err != nil {
					lib.Log(err.Error())
				} else {
					n.serve(c, false)
				}
			}
		}()
		return p
	}

	return 0
}

func setSocketOptions(network string, address string, c syscall.RawConn) error {
	var fn = func(s uintptr) {
		var setErr error
		setErr = syscall.SetsockoptInt(int(s), syscall.IPPROTO_TCP, syscall.TCP_KEEPINTVL, 5)
		if setErr != nil {
			log.Fatal(setErr)
		}
	}
	if err := c.Control(fn); err != nil {
		return err
	}

	return nil

}
