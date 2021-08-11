package ergo

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"sync"

	//"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"runtime"
	"sync/atomic"

	"github.com/halturin/ergo/dist"
	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/lib"

	"math/big"

	"log"
	"net"

	//	"net/http"
	"strconv"
	"strings"
	"time"
)

// Node instance of created node using CreateNode
type Node struct {
	epmd     *dist.EPMD
	listener net.Listener
	Cookie   string

	registrar *registrar
	monitor   *monitor
	context   context.Context
	Stop      context.CancelFunc

	StartedAt time.Time
	uniqID    int64

	tlscertServer tls.Certificate
	tlscertClient tls.Certificate

	FullName string

	creation uint32

	opts NodeOptions

	remoteSpawnMutex sync.Mutex
	remoteSpawn      map[string]ProcessBehavior
}

// NodeOptions struct with bootstrapping options for CreateNode
type NodeOptions struct {
	ListenRangeBegin       uint16
	ListenRangeEnd         uint16
	Hidden                 bool
	EPMDPort               uint16
	DisableEPMDServer      bool
	SendQueueLength        int
	RecvQueueLength        int
	FragmentationUnit      int
	DisableHeaderAtomCache bool
	TLSmode                TLSmodeType
	TLScrtServer           string
	TLSkeyServer           string
	TLScrtClient           string
	TLSkeyClient           string
	// HandshakeVersion. Allowed values 5 or 6. Default version is 5
	HandshakeVersion int
	// ConnectionHandlers defines the number of readers/writers per connection. Default is the number of CPU.
	ConnectionHandlers int
}

// TLSmodeType should be one of TLSmodeDisabled (default), TLSmodeAuto or TLSmodeStrict
type TLSmodeType string

const (
	defaultListenRangeBegin uint16 = 15000
	defaultListenRangeEnd   uint16 = 65000
	defaultEPMDPort         uint16 = 4369

	defaultSendQueueLength   int = 100
	defaultRecvQueueLength   int = 100
	defaultFragmentationUnit     = 65000

	defaultHandshakeVersion = 5

	// TLSmodeDisabled no TLS encryption
	TLSmodeDisabled TLSmodeType = ""
	// TLSmodeAuto generate self-signed certificate
	TLSmodeAuto TLSmodeType = "auto"
	// TLSmodeStrict with validation certificate
	TLSmodeStrict TLSmodeType = "strict"
)

// CreateNode create new node with name and cookie string
func CreateNode(name string, cookie string, opts NodeOptions) (*Node, error) {
	return CreateNodeWithContext(context.Background(), name, cookie, opts)
}

// CreateNodeWithContext create new node with specified context, name and cookie string
func CreateNodeWithContext(ctx context.Context, name string, cookie string, opts NodeOptions) (*Node, error) {

	lib.Log("Start with name '%s' and cookie '%s'", name, cookie)
	nodectx, nodestop := context.WithCancel(ctx)

	r := time.Now().Unix()

	node := &Node{
		epmd:      &dist.EPMD{},
		Cookie:    cookie,
		context:   nodectx,
		Stop:      nodestop,
		StartedAt: time.Now(),
		uniqID:    time.Now().UnixNano(),
		// Creation must be > 0 so make 'or 0x1'
		creation:    uint32(r) | 1,
		remoteSpawn: make(map[string]ProcessBehavior),
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

		if opts.SendQueueLength == 0 {
			opts.SendQueueLength = defaultSendQueueLength
		}

		if opts.RecvQueueLength == 0 {
			opts.RecvQueueLength = defaultRecvQueueLength
		}

		if opts.FragmentationUnit < 1500 {
			opts.FragmentationUnit = defaultFragmentationUnit
		}

		// must be 5 or 6
		if opts.HandshakeVersion != 5 && opts.HandshakeVersion != 6 {
			opts.HandshakeVersion = defaultHandshakeVersion
		}

		if opts.Hidden {
			lib.Log("Running as hidden node")
		}
		ns := strings.Split(name, "@")
		if len(ns) == 1 {
			ns = append(ns, "localhost")
			name = name + "@localhost"
		}
		if len(ns) != 2 {
			return nil, fmt.Errorf("incorrect FQDN node name (example: node@localhost)")
		}

		listenPort := node.listen(ns[1], opts)
		if listenPort == 0 {
			return nil, ErrTaken
		}
		// start EPMD
		node.epmd.Init(nodectx, name, listenPort, opts.EPMDPort, opts.Hidden, opts.DisableEPMDServer)

		node.FullName = name
	}

	node.opts = opts

	node.registrar = createRegistrar(node)
	node.monitor = createMonitor(node)

	netKernelSup := &netKernelSup{}
	node.Spawn("net_kernel_sup", ProcessOptions{}, netKernelSup)

	return node, nil
}

// Spawn create new process
func (n *Node) Spawn(name string, opts ProcessOptions, object ProcessBehavior, args ...etf.Term) (*Process, error) {

	process, err := n.registrar.RegisterProcessExt(name, object, opts)
	if err != nil {
		return nil, err
	}

	go func() {
		pid := process.Self()

		defer func() {
			if r := recover(); r != nil {
				pc, fn, line, _ := runtime.Caller(2)
				fmt.Printf("Warning: process recovered (name: %s) %v %#v at %s[%s:%d]\n",
					name, process.self, r, runtime.FuncForPC(pc).Name(), fn, line)

				n.registrar.UnregisterProcess(pid)
				n.monitor.ProcessTerminated(pid, name, "panic")
				process.Kill()

				process.ready <- fmt.Errorf("Can't start process: %s\n", r)
				close(process.stopped)
			}

			// we should close this channel otherwise if we try
			// immediately call process.Exit it blocks this call forewer
			// since there is nobody to read a message from this channel
			close(process.gracefulExit)
		}()

		// start process loop
		reason := object.(ProcessBehavior).Loop(process, args...)

		// process stopped. unregister it and let everybody (who set up
		// link/monitor) to know about it
		n.registrar.UnregisterProcess(pid)
		n.monitor.ProcessTerminated(pid, name, reason)

		// cancel the context if it was stopped by itself
		if reason != "kill" {
			process.Kill()
		}

		close(process.ready)
		close(process.stopped)
	}()

	if e := <-process.ready; e != nil {
		close(process.ready)
		return nil, e
	}

	return process, nil
}

func (n *Node) ProvideRemoteSpawn(name string, object ProcessBehavior) {
	n.remoteSpawnMutex.Lock()
	n.remoteSpawn[name] = object
	n.remoteSpawnMutex.Unlock()
	return
}

func (n *Node) RevokeRemoteSpawn(name string) bool {
	n.remoteSpawnMutex.Lock()
	defer n.remoteSpawnMutex.Unlock()
	if _, ok := n.remoteSpawn[name]; ok {
		delete(n.remoteSpawn, name)
		return true
	}
	return false
}

// Register register associates the name with pid
func (n *Node) Register(name string, pid etf.Pid) error {
	return n.registrar.RegisterName(name, pid)
}

func (n *Node) Unregister(name string) {
	n.registrar.UnregisterName(name)
}

// IsProcessAlive returns true if the process with given pid is alive
func (n *Node) IsProcessAlive(pid etf.Pid) bool {
	if pid.Node != etf.Atom(n.FullName) {
		return false
	}

	p := n.registrar.GetProcessByPid(pid)
	if p == nil {
		return false
	}

	return p.IsAlive()
}

// IsAlive returns true if node is running
func (n *Node) IsAlive() bool {
	return n.context.Err() == nil
}

// Wait waits until node stopped
func (n *Node) Wait() {
	<-n.context.Done()
}

// WaitWithTimeout waits until node stopped. Return ErrTimeout
// if given timeout is exceeded
func (n *Node) WaitWithTimeout(d time.Duration) error {

	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-timer.C:
		return ErrTimeout
	case <-n.context.Done():
		return nil
	}
}

// ProcessInfo returns the details about given Pid
func (n *Node) ProcessInfo(pid etf.Pid) (ProcessInfo, error) {
	p := n.registrar.GetProcessByPid(pid)
	if p == nil {
		return ProcessInfo{}, fmt.Errorf("undefined")
	}

	return p.Info(), nil
}

// AddStaticRoute adds static route record into the EPMD client
func (n *Node) AddStaticRoute(name string, port uint16) error {
	return n.epmd.AddStaticRoute(name, port)
}

// RemoveStaticRoute removes static route record from the EPMD client
func (n *Node) RemoveStaticRoute(name string) {
	n.epmd.RemoveStaticRoute(name)
}

// ResolvePort resolves port number for the given name. Returns -1 if not found
func (n *Node) ResolvePort(name string) int {
	if port, err := n.epmd.ResolvePort(name); err == nil {
		return port
	}

	return -1
}

func (n *Node) serve(link *dist.Link, opts NodeOptions) error {
	// define the total number of reader/writer goroutines
	numHandlers := runtime.GOMAXPROCS(opts.ConnectionHandlers)

	// do not use shared channels within intencive code parts, impacts on a performance
	receivers := struct {
		recv []chan *lib.Buffer
		n    int
		i    int
	}{
		recv: make([]chan *lib.Buffer, opts.RecvQueueLength),
		n:    numHandlers,
	}

	p := &peer{
		name: link.GetRemoteName(),
		send: make([]chan []etf.Term, numHandlers),
		n:    numHandlers,
	}

	if err := n.registrar.RegisterPeer(p); err != nil {
		// duplicate link?
		return err
	}

	// run readers for incoming messages
	for i := 0; i < numHandlers; i++ {
		// run packet reader/handler routines (decoder)
		recv := make(chan *lib.Buffer, opts.RecvQueueLength)
		receivers.recv[i] = recv
		go link.ReadHandlePacket(n.context, recv, n.handleMessage)
	}

	cacheIsReady := make(chan bool)

	// run link reader routine
	go func() {
		var err error
		var packetLength int
		var recv chan *lib.Buffer

		ctx, cancel := context.WithCancel(n.context)
		defer cancel()

		go func() {
			select {
			case <-ctx.Done():
				// if node's context is done
				link.Close()
			}
		}()

		// initializing atom cache if its enabled
		if !opts.DisableHeaderAtomCache {
			link.SetAtomCache(etf.NewAtomCache(ctx))
		}
		cacheIsReady <- true

		defer func() {
			link.Close()
			n.registrar.UnregisterPeer(link.GetRemoteName())

			// close handlers channel
			for i := 0; i < numHandlers; i++ {
				if p.send[i] != nil {
					close(p.send[i])
				}
				if receivers.recv[i] != nil {
					close(receivers.recv[i])
				}
			}
		}()

		b := lib.TakeBuffer()
		for {
			packetLength, err = link.Read(b)
			if err != nil || packetLength == 0 {
				// link was closed or got malformed data
				if err != nil {
					fmt.Println("link was closed", link.GetPeerName(), "error:", err)
				}
				lib.ReleaseBuffer(b)
				return
			}

			// take new buffer for the next reading and append the tail (part of the next packet)
			b1 := lib.TakeBuffer()
			b1.Set(b.B[packetLength:])
			// cut the tail and send it further for handling.
			// buffer b has to be released by the reader of
			// recv channel (link.ReadHandlePacket)
			b.B = b.B[:packetLength]
			recv = receivers.recv[receivers.i]
			recv <- b

			// set new buffer as a current for the next reading
			b = b1

			// round-robin switch to the next receiver
			receivers.i++
			if receivers.i < receivers.n {
				continue
			}
			receivers.i = 0

		}
	}()

	// we should make sure if the cache is ready before we start writers
	<-cacheIsReady

	// run readers/writers for incoming/outgoing messages
	for i := 0; i < numHandlers; i++ {
		// run writer routines (encoder)
		send := make(chan []etf.Term, opts.SendQueueLength)
		p.mutex.Lock()
		p.send[i] = send
		p.mutex.Unlock()
		go link.Writer(send, opts.FragmentationUnit)
	}

	return nil
}

// LoadedApplications returns a list with information about the
// applications, which are loaded using ApplicatoinLoad
func (n *Node) LoadedApplications() []ApplicationInfo {
	info := []ApplicationInfo{}
	for _, a := range n.registrar.ApplicationList() {
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
			PID:         a.process.self,
		}
		info = append(info, appInfo)
	}
	return info
}

// GetApplicationInfo returns information about application
func (n *Node) GetApplicationInfo(name string) (ApplicationInfo, error) {
	spec := n.registrar.GetApplicationSpecByName(name)
	if spec == nil {
		return ApplicationInfo{}, ErrAppUnknown
	}

	pid := etf.Pid{}
	if spec.process != nil {
		pid = spec.process.self
	}

	return ApplicationInfo{
		Name:        name,
		Description: spec.Description,
		Version:     spec.Version,
		PID:         pid,
	}, nil
}

// ApplicationLoad loads the application specification for an application
// into the node. It also loads the application specifications for any included applications
func (n *Node) ApplicationLoad(app interface{}, args ...etf.Term) error {

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
func (n *Node) ApplicationUnload(appName string) error {
	spec := n.registrar.GetApplicationSpecByName(appName)
	if spec == nil {
		return ErrAppUnknown
	}
	if spec.process != nil {
		return ErrAppAlreadyStarted
	}

	n.registrar.UnregisterApp(appName)
	return nil
}

// ApplicationStartPermanent start Application with start type ApplicationStartPermanent
// If this application terminates, all other applications and the entire node are also
// terminated
func (n *Node) ApplicationStartPermanent(appName string, args ...etf.Term) (*Process, error) {
	return n.applicationStart(ApplicationStartPermanent, appName, args...)
}

// ApplicationStartTransient start Application with start type ApplicationStartTransient
// If transient application terminates with reason 'normal', this is reported and no
// other applications are terminated. Otherwise, all other applications and node
// are terminated
func (n *Node) ApplicationStartTransient(appName string, args ...etf.Term) (*Process, error) {
	return n.applicationStart(ApplicationStartTransient, appName, args...)
}

// ApplicationStart start Application with start type ApplicationStartTemporary
// If an application terminates, this is reported but no other applications
// are terminated
func (n *Node) ApplicationStart(appName string, args ...etf.Term) (*Process, error) {
	return n.applicationStart(ApplicationStartTemporary, appName, args...)
}

func (n *Node) applicationStart(startType, appName string, args ...etf.Term) (*Process, error) {

	spec := n.registrar.GetApplicationSpecByName(appName)
	if spec == nil {
		return nil, ErrAppUnknown
	}

	spec.startType = startType

	// to prevent race condition on starting application we should
	// make sure that nobodyelse starting it
	spec.mutex.Lock()
	defer spec.mutex.Unlock()

	if spec.process != nil {
		return nil, ErrAppAlreadyStarted
	}

	// start dependencies
	for _, depAppName := range spec.Applications {
		if _, e := n.ApplicationStart(depAppName); e != nil && e != ErrAppAlreadyStarted {
			return nil, e
		}
	}

	// passing 'spec' to the process loop in order to handle children's startup.
	args = append([]etf.Term{spec}, args)
	appProcess, e := n.Spawn("", ProcessOptions{}, spec.app, args...)
	if e != nil {
		return nil, e
	}

	spec.process = appProcess
	return appProcess, nil
}

// ApplicationStop stop running application
func (n *Node) ApplicationStop(name string) error {
	spec := n.registrar.GetApplicationSpecByName(name)
	if spec == nil {
		return ErrAppUnknown
	}

	if spec.process == nil {
		return ErrAppIsNotRunning
	}

	spec.process.Exit(spec.process.Self(), "normal")
	// we should wait until children process stopped.
	if e := spec.process.WaitWithTimeout(5 * time.Second); e != nil {
		return ErrProcessBusy
	}
	return nil
}

func (n *Node) handleMessage(fromNode string, control, message etf.Term) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%s", r)
		}
	}()

	switch t := control.(type) {
	case etf.Tuple:
		switch act := t.Element(1).(type) {
		case int:
			switch act {
			case distProtoREG_SEND:
				// {6, FromPid, Unused, ToName}
				lib.Log("[%s] CONTROL REG_SEND [from %s]: %#v", n.FullName, fromNode, control)
				n.registrar.route(t.Element(2).(etf.Pid), t.Element(4), message)

			case distProtoSEND:
				// {2, Unused, ToPid}
				// SEND has no sender pid
				lib.Log("[%s] CONTROL SEND [from %s]: %#v", n.FullName, fromNode, control)
				n.registrar.route(etf.Pid{}, t.Element(3), message)

			case distProtoLINK:
				// {1, FromPid, ToPid}
				lib.Log("[%s] CONTROL LINK [from %s]: %#v", n.FullName, fromNode, control)
				n.monitor.Link(t.Element(2).(etf.Pid), t.Element(3).(etf.Pid))

			case distProtoUNLINK:
				// {4, FromPid, ToPid}
				lib.Log("[%s] CONTROL UNLINK [from %s]: %#v", n.FullName, fromNode, control)
				n.monitor.Unlink(t.Element(2).(etf.Pid), t.Element(3).(etf.Pid))

			case distProtoNODE_LINK:
				lib.Log("[%s] CONTROL NODE_LINK [from %s]: %#v", n.FullName, fromNode, control)

			case distProtoEXIT:
				// {3, FromPid, ToPid, Reason}
				lib.Log("[%s] CONTROL EXIT [from %s]: %#v", n.FullName, fromNode, control)
				terminated := t.Element(2).(etf.Pid)
				reason := fmt.Sprint(t.Element(4))
				n.monitor.ProcessTerminated(terminated, "", string(reason))

			case distProtoEXIT2:
				lib.Log("[%s] CONTROL EXIT2 [from %s]: %#v", n.FullName, fromNode, control)

			case distProtoMONITOR:
				// {19, FromPid, ToProc, Ref}, where FromPid = monitoring process
				// and ToProc = monitored process pid or name (atom)
				lib.Log("[%s] CONTROL MONITOR [from %s]: %#v", n.FullName, fromNode, control)
				n.monitor.MonitorProcessWithRef(t.Element(2).(etf.Pid), t.Element(3), t.Element(4).(etf.Ref))

			case distProtoDEMONITOR:
				// {20, FromPid, ToProc, Ref}, where FromPid = monitoring process
				// and ToProc = monitored process pid or name (atom)
				lib.Log("[%s] CONTROL DEMONITOR [from %s]: %#v", n.FullName, fromNode, control)
				n.monitor.DemonitorProcess(t.Element(4).(etf.Ref))

			case distProtoMONITOR_EXIT:
				// {21, FromProc, ToPid, Ref, Reason}, where FromProc = monitored process
				// pid or name (atom), ToPid = monitoring process, and Reason = exit reason for the monitored process
				lib.Log("[%s] CONTROL MONITOR_EXIT [from %s]: %#v", n.FullName, fromNode, control)
				reason := fmt.Sprint(t.Element(5))
				switch terminated := t.Element(2).(type) {
				case etf.Pid:
					n.monitor.ProcessTerminated(terminated, "", string(reason))
				case etf.Atom:
					pid := fakeMonitorPidFromName(string(terminated), fromNode)
					n.monitor.ProcessTerminated(pid, "", string(reason))
				}

			// Not implemented yet, just stubs. TODO.
			case distProtoSEND_SENDER:
				lib.Log("[%s] CONTROL SEND_SENDER unsupported [from %s]: %#v", n.FullName, fromNode, control)
			case distProtoPAYLOAD_EXIT:
				lib.Log("[%s] CONTROL PAYLOAD_EXIT unsupported [from %s]: %#v", n.FullName, fromNode, control)
			case distProtoPAYLOAD_EXIT2:
				lib.Log("[%s] CONTROL PAYLOAD_EXIT2 unsupported [from %s]: %#v", n.FullName, fromNode, control)
			case distProtoPAYLOAD_MONITOR_P_EXIT:
				lib.Log("[%s] CONTROL PAYLOAD_MONITOR_P_EXIT unsupported [from %s]: %#v", n.FullName, fromNode, control)
			case distProtoALIAS_SEND:
				// {33, FromPid, Alias}
				lib.Log("[%s] CONTROL ALIAS_SEND [from %s]: %#v", n.FullName, fromNode, control)
				alias := etf.Alias(t.Element(3).(etf.Ref))
				n.registrar.route(t.Element(2).(etf.Pid), alias, message)

			case distProtoSPAWN_REQUEST:
				// {29, ReqId, From, GroupLeader, {Module, Function, Arity}, OptList}
				lib.Log("[%s] CONTROL SPAWN_REQUEST [from %s]: %#v", n.FullName, fromNode, control)
				registerName := ""
				for _, option := range t.Element(6).(etf.List) {
					name, ok := option.(etf.Tuple)
					if !ok {
						break
					}
					if name.Element(1).(etf.Atom) == etf.Atom("name") {
						registerName = string(name.Element(2).(etf.Atom))
					}
				}

				from := t.Element(3).(etf.Pid)
				ref := t.Element(2).(etf.Ref)

				mfa := t.Element(5).(etf.Tuple)
				module := mfa.Element(1).(etf.Atom)
				var args etf.List
				if str, ok := message.(string); !ok {
					args = message.(etf.List)
				} else {
					// stupid Erlang's strings :). [1,2,3,4,5] sends as a string.
					// args can't be anything but etf.List.
					for i := range []byte(str) {
						args = append(args, str[i])
					}
				}

				n.remoteSpawnMutex.Lock()
				object, provided := n.remoteSpawn[string(module)]
				n.remoteSpawnMutex.Unlock()
				if !provided {
					message := etf.Tuple{distProtoSPAWN_REPLY, ref, from, 0, etf.Atom("not_provided")}
					n.registrar.routeRaw(from.Node, message)
					return
				}

				process, err_spawn := n.Spawn(registerName, ProcessOptions{}, object, args...)
				if err_spawn != nil {
					message := etf.Tuple{distProtoSPAWN_REPLY, ref, from, 0, etf.Atom(err_spawn.Error())}
					n.registrar.routeRaw(from.Node, message)
					return
				}
				message := etf.Tuple{distProtoSPAWN_REPLY, ref, from, 0, process.Self()}
				n.registrar.routeRaw(from.Node, message)

			case distProtoSPAWN_REPLY:
				// {31, ReqId, To, Flags, Result}
				lib.Log("[%s] CONTROL SPAWN_REPLY [from %s]: %#v", n.FullName, fromNode, control)

				to := t.Element(3).(etf.Pid)
				process := n.GetProcessByPid(to)
				if process == nil {
					return
				}
				ref := t.Element(2).(etf.Ref)
				//flags := t.Element(4)
				process.putSyncReply(ref, t.Element(5))

			default:
				lib.Log("[%s] CONTROL unknown command [from %s]: %#v", n.FullName, fromNode, control)
			}
		default:
			err = fmt.Errorf("unsupported message %#v", control)
		}
	}

	return
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

// GetProcessByName returns Process associated with given name
func (n *Node) GetProcessByName(name string) *Process {
	return n.registrar.GetProcessByName(name)
}

// GetProcessByPid returns Process by the given pid
func (n *Node) GetProcessByPid(pid etf.Pid) *Process {
	return n.registrar.GetProcessByPid(pid)
}

// GetProcessByAlias returns Process by the given alias
func (n *Node) GetProcessByAlias(alias etf.Alias) *Process {
	return n.registrar.GetProcessByAlias(alias)
}

// GetProcessList returns array of running process
func (n *Node) GetProcessList() []*Process {
	return n.registrar.ProcessList()
}

// GetPeerList returns list of connected nodes
func (n *Node) GetPeerList() []string {
	return n.registrar.PeerList()
}

// MakeRef returns atomic reference etf.Ref within this node
func (n *Node) MakeRef() (ref etf.Ref) {
	ref.Node = etf.Atom(n.FullName)
	ref.Creation = n.creation
	nt := atomic.AddInt64(&n.uniqID, 1)
	ref.ID[0] = uint32(uint64(nt) & ((2 << 17) - 1))
	ref.ID[1] = uint32(uint64(nt) >> 46)

	return
}

func (n *Node) VersionERTS() string {
	return fmt.Sprintf("%s-%s-%s", versionERTSprefix, version, runtime.Version())
}

func (n *Node) VersionOTP() int {
	return versionOTP
}

func (n *Node) connect(to etf.Atom) error {
	var port int
	var err error
	var c net.Conn
	if port, err = n.epmd.ResolvePort(string(to)); port < 0 {
		return fmt.Errorf("Can't resolve port for %s: %s", to, err)
	}
	ns := strings.Split(string(to), "@")

	TLSenabled := false

	switch n.opts.TLSmode {
	case TLSmodeAuto:
		tlsdialer := tls.Dialer{
			Config: &tls.Config{
				Certificates:       []tls.Certificate{n.tlscertClient},
				InsecureSkipVerify: true,
			},
		}
		c, err = tlsdialer.DialContext(n.context, "tcp", net.JoinHostPort(ns[1], strconv.Itoa(port)))
		TLSenabled = true

	case TLSmodeStrict:
		tlsdialer := tls.Dialer{
			Config: &tls.Config{
				Certificates: []tls.Certificate{n.tlscertClient},
			},
		}
		c, err = tlsdialer.DialContext(n.context, "tcp", net.JoinHostPort(ns[1], strconv.Itoa(port)))
		TLSenabled = true

	default:
		dialer := net.Dialer{}
		c, err = dialer.DialContext(n.context, "tcp", net.JoinHostPort(ns[1], strconv.Itoa(port)))
	}

	if err != nil {
		lib.Log("Error calling net.Dialer.DialerContext : %s", err.Error())
		return err
	}

	handshakeOptions := dist.HandshakeOptions{
		Name:     n.FullName,
		Cookie:   n.Cookie,
		TLS:      TLSenabled,
		Hidden:   false,
		Creation: n.creation,
		Version:  n.opts.HandshakeVersion,
	}
	link, e := dist.Handshake(c, handshakeOptions)
	if e != nil {
		return e
	}

	if err := n.serve(link, n.opts); err != nil {
		c.Close()
		return err
	}
	return nil
}

func (n *Node) listen(name string, opts NodeOptions) uint16 {
	var TLSenabled bool = true

	lc := net.ListenConfig{}
	for p := opts.ListenRangeBegin; p <= opts.ListenRangeEnd; p++ {
		l, err := lc.Listen(n.context, "tcp", net.JoinHostPort(name, strconv.Itoa(int(p))))
		if err != nil {
			continue
		}

		switch opts.TLSmode {
		case TLSmodeAuto:
			cert, err := generateSelfSignedCert()
			if err != nil {
				log.Fatalf("Can't generate certificate: %s\n", err)
			}

			n.tlscertServer = cert
			n.tlscertClient = cert

			TLSconfig := &tls.Config{
				Certificates:       []tls.Certificate{cert},
				InsecureSkipVerify: true,
			}
			l = tls.NewListener(l, TLSconfig)

		case TLSmodeStrict:
			certServer, err := tls.LoadX509KeyPair(opts.TLScrtServer, opts.TLSkeyServer)
			if err != nil {
				log.Fatalf("Can't load server certificate: %s\n", err)
			}
			certClient, err := tls.LoadX509KeyPair(opts.TLScrtServer, opts.TLSkeyServer)
			if err != nil {
				log.Fatalf("Can't load client certificate: %s\n", err)
			}

			n.tlscertServer = certServer
			n.tlscertClient = certClient

			TLSconfig := &tls.Config{
				Certificates: []tls.Certificate{certServer},
				ServerName:   "localhost",
			}
			l = tls.NewListener(l, TLSconfig)

		default:
			TLSenabled = false
		}

		go func() {
			for {
				c, err := l.Accept()
				lib.Log("Accepted new connection from %s", c.RemoteAddr().String())

				if n.IsAlive() == false {
					c.Close()
					return
				}

				if err != nil {
					lib.Log(err.Error())
					continue
				}
				handshakeOptions := dist.HandshakeOptions{
					Name:     n.FullName,
					Cookie:   n.Cookie,
					TLS:      TLSenabled,
					Hidden:   opts.Hidden,
					Creation: n.creation,
					Version:  n.opts.HandshakeVersion,
				}

				link, e := dist.HandshakeAccept(c, handshakeOptions)
				if e != nil {
					lib.Log("Can't handshake with %s: %s", c.RemoteAddr().String(), e)
					c.Close()
					continue
				}

				// start serving this link
				if err := n.serve(link, opts); err != nil {
					lib.Log("Can't serve connection link due to: %s", err)
					c.Close()
				}

			}
		}()

		// return port number this node listenig on for the incoming connections
		return p
	}

	// all the ports within a given range are taken
	return 0
}

func generateSelfSignedCert() (tls.Certificate, error) {
	var cert = tls.Certificate{}

	certPrivKey, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	if err != nil {
		return cert, err
	}
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return cert, err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{versionERTSprefix},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(time.Hour * 24 * 365),
		//IsCA:        true,

		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	template.IPAddresses = append(template.IPAddresses, net.ParseIP("127.0.0.1"))

	certBytes, err1 := x509.CreateCertificate(rand.Reader, &template, &template,
		&certPrivKey.PublicKey, certPrivKey)
	if err1 != nil {
		return cert, err1
	}

	certPEM := new(bytes.Buffer)
	pem.Encode(certPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})

	certPrivKeyPEM := new(bytes.Buffer)
	x509Encoded, _ := x509.MarshalECPrivateKey(certPrivKey)
	pem.Encode(certPrivKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509Encoded,
	})

	return tls.X509KeyPair(certPEM.Bytes(), certPrivKeyPEM.Bytes())
}
