package node

import (
	"context"

	//"crypto/rsa"

	"fmt"

	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/lib"

	//	"net/http"

	"strings"
	"time"
)

// Node instance of created node using CreateNode
type Node struct {
	Registrar
	Network

	name     string
	Cookie   string
	creation uint32
	opts     Options
	context  context.Context
	Stop     context.CancelFunc
}

// Options struct with bootstrapping options for CreateNode
type Options struct {
	ListenRangeBegin       uint16
	ListenRangeEnd         uint16
	Hidden                 bool
	EPMDPort               uint16
	DisableEPMDServer      bool
	DisableEPMD            bool // use static routes only
	SendQueueLength        int
	RecvQueueLength        int
	FragmentationUnit      int
	DisableHeaderAtomCache bool
	TLSMode                TLSModeType
	TLScrtServer           string
	TLSkeyServer           string
	TLScrtClient           string
	TLSkeyClient           string
	// HandshakeVersion. Allowed values 5 or 6. Default version is 5
	HandshakeVersion int
	// ConnectionHandlers defines the number of readers/writers per connection. Default is the number of CPU.
	ConnectionHandlers int

	cookie   string
	creation uint32
}

// TLSmodeType should be one of TLSmodeDisabled (default), TLSmodeAuto or TLSmodeStrict
type TLSmodeType string

// StartNodeWithContext create new node with specified context, name and cookie string
func startNodeWithContext(ctx context.Context, name string, cookie string, opts Options) (*Node, error) {

	lib.Log("Start with name '%s' and cookie '%s'", name, cookie)
	nodectx, nodestop := context.WithCancel(ctx)

	// Creation must be > 0 so make 'or 0x1'
	creation := uint32(time.Now().Unix()) | 1

	node := &Node{
		Cookie:   cookie,
		context:  nodectx,
		Stop:     nodestop,
		creation: creation,
	}

	if name == "" {
		return nil, fmt.Errorf("Node name must be defined")
	}
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

	if len(strings.Split(name, "@")) != 2 {
		return nil, fmt.Errorf("incorrect FQDN node name (example: node@localhost)")
	}

	opts.cookie = cookie
	opts.creation = creation
	node.opts = opts
	node.name = name

	registrar := NewRegistrar(ctx, name, creation, node)
	network, err := NewNetwork(ctx, name, opts, registrar)
	if err != nil {
		return nil, err
	}

	node.Registrar = registrar
	node.Network = network

	//	netKernelSup := &netKernelSup{}
	//	node.Spawn("net_kernel_sup", ProcessOptions{}, netKernelSup)

	return node, nil
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
func (n *node) Spawn(name string, opts ProcessOptions, object ProcessBehavior, args ...etf.Term) (*Process, error) {
	return n.spawn(name, opts, object, args...)
}

// LoadedApplications returns a list with information about the
// applications, which are loaded using ApplicatoinLoad
//func (n *Node) LoadedApplications() []ApplicationInfo {
//	info := []ApplicationInfo{}
//	for _, a := range n.registrar.ApplicationList() {
//		appInfo := ApplicationInfo{
//			Name:        a.Name,
//			Description: a.Description,
//			Version:     a.Version,
//		}
//		info = append(info, appInfo)
//	}
//	return info
//}
//
//// WhichApplications returns a list with information about the applications that are currently running.
//func (n *Node) WhichApplications() []ApplicationInfo {
//	info := []ApplicationInfo{}
//	for _, a := range n.registrar.ApplicationList() {
//		if a.process == nil {
//			// list only started apps
//			continue
//		}
//		appInfo := ApplicationInfo{
//			Name:        a.Name,
//			Description: a.Description,
//			Version:     a.Version,
//			PID:         a.process.self,
//		}
//		info = append(info, appInfo)
//	}
//	return info
//}
//
//// GetApplicationInfo returns information about application
//func (n *Node) GetApplicationInfo(name string) (ApplicationInfo, error) {
//	spec := n.registrar.GetApplicationSpecByName(name)
//	if spec == nil {
//		return ApplicationInfo{}, ErrAppUnknown
//	}
//
//	pid := etf.Pid{}
//	if spec.process != nil {
//		pid = spec.process.self
//	}
//
//	return ApplicationInfo{
//		Name:        name,
//		Description: spec.Description,
//		Version:     spec.Version,
//		PID:         pid,
//	}, nil
//}
//
//// ApplicationLoad loads the application specification for an application
//// into the node. It also loads the application specifications for any included applications
//func (n *Node) ApplicationLoad(app ApplicationBehavior, args ...etf.Term) error {
//
//	spec, err := app.Load(args...)
//	if err != nil {
//		return err
//	}
//	spec.app = app
//	return n.registrar.RegisterApp(spec.Name, &spec)
//}
//
//// ApplicationUnload unloads the application specification for Application from the
//// node. It also unloads the application specifications for any included applications.
//func (n *Node) ApplicationUnload(appName string) error {
//	spec := n.registrar.GetApplicationSpecByName(appName)
//	if spec == nil {
//		return ErrAppUnknown
//	}
//	if spec.process != nil {
//		return ErrAppAlreadyStarted
//	}
//
//	n.registrar.UnregisterApp(appName)
//	return nil
//}
//
//// ApplicationStartPermanent start Application with start type ApplicationStartPermanent
//// If this application terminates, all other applications and the entire node are also
//// terminated
//func (n *Node) ApplicationStartPermanent(appName string, args ...etf.Term) (*Process, error) {
//	return n.applicationStart(ApplicationStartPermanent, appName, args...)
//}
//
//// ApplicationStartTransient start Application with start type ApplicationStartTransient
//// If transient application terminates with reason 'normal', this is reported and no
//// other applications are terminated. Otherwise, all other applications and node
//// are terminated
//func (n *Node) ApplicationStartTransient(appName string, args ...etf.Term) (*Process, error) {
//	return n.applicationStart(ApplicationStartTransient, appName, args...)
//}
//
//// ApplicationStart start Application with start type ApplicationStartTemporary
//// If an application terminates, this is reported but no other applications
//// are terminated
//func (n *Node) ApplicationStart(appName string, args ...etf.Term) (*Process, error) {
//	return n.applicationStart(ApplicationStartTemporary, appName, args...)
//}
//
//func (n *Node) applicationStart(startType, appName string, args ...etf.Term) (*Process, error) {
//
//	spec := n.registrar.GetApplicationSpecByName(appName)
//	if spec == nil {
//		return nil, ErrAppUnknown
//	}
//
//	spec.startType = startType
//
//	// to prevent race condition on starting application we should
//	// make sure that nobodyelse starting it
//	spec.mutex.Lock()
//	defer spec.mutex.Unlock()
//
//	if spec.process != nil {
//		return nil, ErrAppAlreadyStarted
//	}
//
//	// start dependencies
//	for _, depAppName := range spec.Applications {
//		if _, e := n.ApplicationStart(depAppName); e != nil && e != ErrAppAlreadyStarted {
//			return nil, e
//		}
//	}
//
//	// passing 'spec' to the process loop in order to handle children's startup.
//	args = append([]etf.Term{spec}, args)
//	appProcess, e := n.Spawn("", ProcessOptions{}, spec.app, args...)
//	if e != nil {
//		return nil, e
//	}
//
//	spec.process = appProcess
//	return appProcess, nil
//}
//
//// ApplicationStop stop running application
//func (n *Node) ApplicationStop(name string) error {
//	spec := n.registrar.GetApplicationSpecByName(name)
//	if spec == nil {
//		return ErrAppUnknown
//	}
//
//	if spec.process == nil {
//		return ErrAppIsNotRunning
//	}
//
//	spec.process.Exit(spec.process.Self(), "normal")
//	// we should wait until children process stopped.
//	if e := spec.process.WaitWithTimeout(5 * time.Second); e != nil {
//		return ErrProcessBusy
//	}
//	return nil
//}

// ProvideRPC register given module/function as RPC method
//func (n *Node) ProvideRPC(module string, function string, fun rpcFunction) error {
//	lib.Log("RPC provide: %s:%s %#v", module, function, fun)
//	message := etf.Tuple{
//		etf.Atom("$provide"),
//		etf.Atom(module),
//		etf.Atom(function),
//		fun,
//	}
//	rex := n.GetProcessByName("rex")
//	if rex == nil {
//		return fmt.Errorf("RPC module is disabled")
//	}
//
//	if v, err := rex.Call(rex.Self(), message); v != etf.Atom("ok") || err != nil {
//		return fmt.Errorf("value: %s err: %s", v, err)
//	}
//
//	return nil
//}
//
//// RevokeRPC unregister given module/function
//func (n *Node) RevokeRPC(module, function string) error {
//	lib.Log("RPC revoke: %s:%s", module, function)
//
//	rex := n.GetProcessByName("rex")
//	if rex == nil {
//		return fmt.Errorf("RPC module is disabled")
//	}
//
//	message := etf.Tuple{
//		etf.Atom("$revoke"),
//		etf.Atom(module),
//		etf.Atom(function),
//	}
//
//	if v, err := rex.Call(rex.Self(), message); v != etf.Atom("ok") || err != nil {
//		return fmt.Errorf("value: %s err: %s", v, err)
//	}
//
//	return nil
//}
