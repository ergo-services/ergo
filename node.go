package ergonode

import (
	"context"
	"fmt"
	"log"
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
	system    systemProcesses
	monitor   *monitor
	procID    uint64
	context   context.Context
	Stop      context.CancelFunc
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
		Cookie:  cookie,
		context: nodectx,
		Stop:    nodestop,
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

	// starting system processes
	process_opts := ProcessOptions{
		MailboxSize: DefaultProcessMailboxSize, // size of channel for regular messages
	}
	node.system.netKernel = new(netKernel)
	node.Spawn("net_kernel", process_opts, node.system.netKernel)

	node.system.globalNameServer = new(globalNameServer)
	node.Spawn("global_name_server", process_opts, node.system.globalNameServer)

	node.system.rpc = new(rpc)
	node.Spawn("rpc", process_opts, node.system.rpc)

	node.system.observer = new(observer)
	node.Spawn("observer", process_opts, node.system.observer)

	return &node
}

// Spawn create new process
func (n *Node) Spawn(name string, opts ProcessOptions, object interface{}, args ...interface{}) Process {
	process := n.registrar.RegisterProcessExt(name, object, opts)
	go func() {
		// FIXME: uncomment this 'defer' before release
		// defer func() {
		// 	if r := recover(); r != nil {
		// 		lib.Log("Recovered process: %#v with error: %s", object, r)
		// 		n.registrar.UnregisterProcess(process.self)
		// 	}
		// }()
		object.(ProcessBehaviour).loop(process, object, args...)
	}()
	<-process.ready

	return process
}

func (n *Node) Register(name string, pid etf.Pid) {
	n.registrar.RegisterName(name, pid)
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
		defer n.registrar.UnregisterPeer(node.GetRemoteName())

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
		defer n.registrar.UnregisterPeer(node.GetRemoteName())
		for {
			terms, err := node.ReadMessage(c)
			if err != nil {
				lib.Log("Enode error (reading): %s", err.Error())
				break
			}
			n.handleTerms(terms)
		}
	}()

	<-node.Ready

	p := peer{
		conn: c,
		send: send,
	}
	n.registrar.RegisterPeer(node.GetRemoteName(), p)
}

// FIXME: rework it using types like [2]etf.Term
func (n *Node) handleTerms(terms []etf.Term) {
	lib.Log("Node terms: %#v", terms)

	if len(terms) == 0 {
		return
	}

	switch t := terms[0].(type) {
	case etf.Tuple:
		if len(t) > 0 {
			switch act := t.Element(1).(type) {
			case int:
				switch act {
				case REG_SEND:
					if len(terms) == 2 {
						n.registrar.route(t.Element(2).(etf.Pid), t.Element(4), terms[1])
					} else {
						lib.Log("*** ERROR: bad REG_SEND: %#v", terms)
					}
				case SEND:
					n.registrar.route(t.Element(2).(etf.Pid), t.Element(3), terms[1])

				// Not implemented yet, just stubs. TODO.
				case LINK:
					lib.Log("LINK message (act %d): %#v", act, t)
				case UNLINK:
					lib.Log("UNLINK message (act %d): %#v", act, t)
				case NODE_LINK:
					lib.Log("NODE_LINK message (act %d): %#v", act, t)
				case EXIT:
					lib.Log("EXIT message (act %d): %#v", act, t)
				case EXIT2:
					lib.Log("EXIT2 message (act %d): %#v", act, t)
				case MONITOR:
					// {19, FromPid, ToProc, Ref}, where FromPid = monitoring process
					// and ToProc = monitored process pid or name (atom)
					lib.Log("MONITOR message (act %d): %#v", act, t)
					// TODO: send Ref to t.Element(2).(etf.Pid)
					ref := n.monitor.MonitorProcess(t.Element(2).(etf.Pid), t.Element(3).(etf.Pid))
					lib.Log("MONITOR ref for remote process (%v): %v", t.Element(2).(etf.Pid), ref)
				case DEMONITOR:
					// {20, FromPid, ToProc, Ref}, where FromPid = monitoring process
					// and ToProc = monitored process pid or name (atom)
					lib.Log("DEMONITOR message (act %d): %#v", act, t)
					n.monitor.DemonitorProcess(t.Element(4).(etf.Ref))
					// TODO: send ok to t.Element(1).(etf.Pid)

				case MONITOR_EXIT:
					// {21, FromProc, ToPid, Ref, Reason}, where FromProc = monitored process
					// pid or name (atom), ToPid = monitoring process, and Reason = exit reason for the monitored process
					lib.Log("MONITOR_EXIT message (act %d): %#v", act, t)
					n.monitor.ProcessTerminated(t.Element(2).(etf.Pid), t.Element(5).(string))

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
					// .Ready channel waiting for registration of this connection
					ready := (t[2]).(chan bool)
					ready <- true
				}
			default:
				lib.Log("UNHANDLED ACT: %#v", t.Element(1))
			}
		}
	}
}

func (n *Node) MakeRef() (ref etf.Ref) {
	ref.Node = etf.Atom(n.FullName)
	ref.Creation = 1

	nt := time.Now().UnixNano()
	id1 := uint32(uint64(nt) & ((2 << 17) - 1))
	id2 := uint32(uint64(nt) >> 46)
	ref.Id = []uint32{id1, id2, 0}

	return
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
			panic(err)
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
