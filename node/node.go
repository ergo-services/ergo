package node

import (
	"context"

	//"crypto/rsa"

	"fmt"

	"github.com/halturin/ergo/erlang"
	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/gen"
	"github.com/halturin/ergo/lib"

	//	"net/http"

	"strings"
	"time"
)

const (
	appBehaviorGroup = "$application"
)

type nodeInternal interface {
	Node
	registrarInternal
}

// Node instance of created node using CreateNode
type node struct {
	registrarInternal
	networkInternal

	name     string
	cookie   string
	creation uint32
	opts     Options
	context  context.Context
	stop     context.CancelFunc
}

// StartWithContext create new node with specified context, name and cookie string
func StartWithContext(ctx context.Context, name string, cookie string, opts Options) (nodeInternal, error) {

	lib.Log("Start with name '%s' and cookie '%s'", name, cookie)
	nodectx, nodestop := context.WithCancel(ctx)

	// Creation must be > 0 so make 'or 0x1'
	creation := uint32(time.Now().Unix()) | 1

	node := &node{
		cookie:   cookie,
		context:  nodectx,
		stop:     nodestop,
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

	registrar := newRegistrar(ctx, name, creation, node)
	network, err := newNetwork(ctx, name, opts, registrar)
	if err != nil {
		return nil, err
	}

	node.registrarInternal = registrar
	node.networkInternal = network

	//	netKernelSup := &netKernelSup{}
	node.Spawn("erlang", gen.ProcessOptions{}, &erlang.Erlang{})

	return node, nil
}

// IsAlive returns true if node is running
func (n *node) IsAlive() bool {
	return n.context.Err() == nil
}

// Wait waits until node stopped
func (n *node) Wait() {
	<-n.context.Done()
}

// WaitWithTimeout waits until node stopped. Return ErrTimeout
// if given timeout is exceeded
func (n *node) WaitWithTimeout(d time.Duration) error {

	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-timer.C:
		return ErrTimeout
	case <-n.context.Done():
		return nil
	}
}
func (n *node) Spawn(name string, opts gen.ProcessOptions, object gen.ProcessBehavior, args ...etf.Term) (gen.Process, error) {
	// process started by node has no parent
	options := processOptions{
		ProcessOptions: opts,
	}
	return n.spawn(name, options, object, args...)
}

func (n *node) Stop() {
	n.stop()
}

func (n *node) Name() string {
	return n.name
}

// LoadedApplications returns a list of loaded applications (including running applications)
func (n *node) LoadedApplications() []gen.ApplicationInfo {
	return n.listApplications(false)
}

// WhichApplications returns a list of running applications
func (n *node) WhichApplications() []gen.ApplicationInfo {
	return n.listApplications(true)
}

// WhichApplications returns a list of running applications
func (n *node) listApplications(onlyRunning bool) []gen.ApplicationInfo {
	info := []gen.ApplicationInfo{}
	for _, rb := range n.GetRegisteredBehaviorGroup(appBehaviorGroup) {
		spec, ok := rb.Data.(*gen.ApplicationSpec)
		if !ok {
			continue
		}

		if onlyRunning && spec.Process == nil {
			// list only started apps
			continue
		}

		appInfo := gen.ApplicationInfo{
			Name:        spec.Name,
			Description: spec.Description,
			Version:     spec.Version,
		}
		if spec.Process != nil {
			appInfo.PID = spec.Process.Self()
		}
		info = append(info, appInfo)
	}
	return info
}

// GetApplicationInfo returns information about application
func (n *node) GetApplicationInfo(name string) (gen.ApplicationInfo, error) {
	rb, err := n.GetRegisteredBehavior(appBehaviorGroup, name)
	if err != nil {
		return gen.ApplicationInfo{}, ErrAppUnknown
	}
	spec, ok := rb.Data.(*gen.ApplicationSpec)
	if !ok {
		return gen.ApplicationInfo{}, ErrAppUnknown
	}

	pid := etf.Pid{}
	if spec.Process != nil {
		pid = spec.Process.Self()
	}

	appInfo := gen.ApplicationInfo{
		Name:        spec.Name,
		Description: spec.Description,
		Version:     spec.Version,
		PID:         pid,
	}
	return appInfo, nil
}

// ApplicationLoad loads the application specification for an application
// into the node.
func (n *node) ApplicationLoad(app gen.ApplicationBehavior, args ...etf.Term) error {

	spec, err := app.Load(args...)
	if err != nil {
		return err
	}
	return n.RegisterBehavior(appBehaviorGroup, spec.Name, app, &spec)
}

// ApplicationUnload unloads given application
func (n *node) ApplicationUnload(appName string) error {
	rb, err := n.GetRegisteredBehavior(appBehaviorGroup, appName)
	if err != nil {
		return ErrAppUnknown
	}

	spec, ok := rb.Data.(*gen.ApplicationSpec)
	if !ok {
		return ErrAppUnknown
	}
	if spec.Process != nil {
		return ErrAppAlreadyStarted
	}

	return n.UnregisterBehavior(appBehaviorGroup, appName)
}

// ApplicationStartPermanent start Application with start type ApplicationStartPermanent
// If this application terminates, all other applications and the entire node are also
// terminated
func (n *node) ApplicationStartPermanent(appName string, args ...etf.Term) (gen.Process, error) {
	return n.applicationStart(gen.ApplicationStartPermanent, appName, args...)
}

// ApplicationStartTransient start Application with start type ApplicationStartTransient
// If transient application terminates with reason 'normal', this is reported and no
// other applications are terminated. Otherwise, all other applications and node
// are terminated
func (n *node) ApplicationStartTransient(appName string, args ...etf.Term) (gen.Process, error) {
	return n.applicationStart(gen.ApplicationStartTransient, appName, args...)
}

// ApplicationStart start Application with start type ApplicationStartTemporary
// If an application terminates, this is reported but no other applications
// are terminated
func (n *node) ApplicationStart(appName string, args ...etf.Term) (gen.Process, error) {
	return n.applicationStart(gen.ApplicationStartTemporary, appName, args...)
}

func (n *node) applicationStart(startType, appName string, args ...etf.Term) (gen.Process, error) {
	rb, err := n.GetRegisteredBehavior(appBehaviorGroup, appName)
	if err != nil {
		return nil, ErrAppUnknown
	}

	spec, ok := rb.Data.(*gen.ApplicationSpec)
	if !ok {
		return nil, ErrAppUnknown
	}

	spec.StartType = startType

	// to prevent race condition on starting application we should
	// make sure that nobodyelse starting it
	spec.Lock()
	defer spec.Unlock()

	if spec.Process != nil {
		return nil, ErrAppAlreadyStarted
	}

	// start dependencies
	for _, depAppName := range spec.Applications {
		if _, e := n.ApplicationStart(depAppName); e != nil && e != ErrAppAlreadyStarted {
			return nil, e
		}
	}

	env := map[string]interface{}{
		"spec": spec,
	}
	options := gen.ProcessOptions{
		Env: env,
	}
	process, e := n.Spawn("", options, rb.Behavior, args...)
	if e != nil {
		return nil, e
	}

	return process, nil
}

// ApplicationStop stop running application
func (n *node) ApplicationStop(name string) error {
	rb, err := n.GetRegisteredBehavior(appBehaviorGroup, name)
	if err != nil {
		return ErrAppUnknown
	}

	spec, ok := rb.Data.(*gen.ApplicationSpec)
	if !ok {
		return ErrAppUnknown
	}

	spec.Lock()
	defer func() { spec.Unlock() }()
	if spec.Process == nil {
		return ErrAppIsNotRunning
	}

	fmt.Println("QQQ", spec.Process.Self())

	spec.Process.Exit("normal")
	return nil
}

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