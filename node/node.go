package node

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/lib"
)

const (
	appBehaviorGroup    = "ergo:applications"
	remoteBehaviorGroup = "ergo:remote"
)

// node instance of created node using CreateNode
type node struct {
	coreInternal

	name     string
	creation uint32
	context  context.Context
	stop     context.CancelFunc
	version  Version
	env      map[gen.EnvKey]interface{}
}

// StartWithContext create new node with specified context, name and cookie string
func StartWithContext(ctx context.Context, name string, cookie string, opts Options) (Node, error) {

	lib.Log("Start node with name %q and cookie %q", name, cookie)
	nodectx, nodestop := context.WithCancel(ctx)

	if len(strings.Split(name, "@")) != 2 {
		return nil, fmt.Errorf("incorrect FQDN node name (example: node@localhost)")
	}
	if opts.Creation == 0 {
		opts.Creation = uint32(time.Now().Unix())
	}

	// set defaults listening port range
	if opts.Listen > 0 {
		opts.ListenBegin = opts.Listen
		opts.ListenEnd = opts.Listen
		lib.Log("Node listening port: %d", opts.Listen)
	} else {
		if opts.ListenBegin == 0 {
			opts.ListenBegin = defaultListenBegin
		}
		if opts.ListenEnd == 0 {
			opts.ListenEnd = defaultListenEnd
		}
		lib.Log("Node listening range: %d...%d", opts.ListenBegin, opts.ListenEnd)
	}

	if opts.Handshake == nil {
		return nil, fmt.Errorf("Handshake must be defined")
	}
	if opts.Proto == nil {
		return nil, fmt.Errorf("Proto must be defined")
	}
	if opts.StaticRoutesOnly == false && opts.Resolver == nil {
		return nil, fmt.Errorf("Resolver must be defined if StaticRoutesOnly == false")
	}

	core, err := newCore(nodectx, name, opts)
	if err != nil {
		return nil, err
	}

	node := &node{
		name:         name,
		context:      nodectx,
		stop:         nodestop,
		creation:     opts.Creation,
		coreInternal: core,
	}

	for _, app := range opts.Applications {
		// load applications
		name, err := node.ApplicationLoad(app)
		if err != nil {
			nodestop()
			return nil, err
		}
		// start applications
		_, err = node.ApplicationStart(name)
		if err != nil {
			nodestop()
			return nil, err
		}
	}

	node.env = make(map[gen.EnvKey]interface{})
	for k, v := range opts.Env {
		node.env[k] = v
	}

	// set global variable 'ergo:Node'
	node.env[EnvKeyNode] = Node(node)

	return node, nil
}

// Version returns version of the node
func (n *node) Version() Version {
	return n.version
}

// Spawn
func (n *node) Spawn(name string, opts gen.ProcessOptions, object gen.ProcessBehavior, args ...etf.Term) (gen.Process, error) {
	// process started by node has no parent
	options := processOptions{
		ProcessOptions: opts,
	}
	return n.spawn(name, options, object, args...)
}

// RegisterName
func (n *node) RegisterName(name string, pid etf.Pid) error {
	return n.registerName(name, pid)
}

// UnregisterName
func (n *node) UnregisterName(name string) error {
	return n.unregisterName(name)
}

// Stop
func (n *node) Stop() {
	n.coreStop()
}

// Name
func (n *node) Name() string {
	return n.name
}

// IsAlive
func (n *node) IsAlive() bool {
	return n.coreIsAlive()
}

// Uptime
func (n *node) Uptime() int64 {
	return n.coreUptime()
}

// Wait
func (n *node) Wait() {
	n.coreWait()
}

// WaitWithTimeout
func (n *node) WaitWithTimeout(d time.Duration) error {
	return n.coreWaitWithTimeout(d)
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
	for _, rb := range n.RegisteredBehaviorGroup(appBehaviorGroup) {
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

// ApplicationInfo returns information about application
func (n *node) ApplicationInfo(name string) (gen.ApplicationInfo, error) {
	rb, err := n.RegisteredBehavior(appBehaviorGroup, name)
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

// ApplicationLoad loads the application specification for an application. Returns name of
// loaded application.
func (n *node) ApplicationLoad(app gen.ApplicationBehavior, args ...etf.Term) (string, error) {

	spec, err := app.Load(args...)
	if err != nil {
		return "", err
	}
	err = n.RegisterBehavior(appBehaviorGroup, spec.Name, app, &spec)
	if err != nil {
		return "", err
	}
	return spec.Name, nil
}

// ApplicationUnload unloads given application
func (n *node) ApplicationUnload(appName string) error {
	rb, err := n.RegisteredBehavior(appBehaviorGroup, appName)
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
	rb, err := n.RegisteredBehavior(appBehaviorGroup, appName)
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

	env := map[gen.EnvKey]interface{}{
		gen.EnvKeySpec: spec,
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
	rb, err := n.RegisteredBehavior(appBehaviorGroup, name)
	if err != nil {
		return ErrAppUnknown
	}

	spec, ok := rb.Data.(*gen.ApplicationSpec)
	if !ok {
		return ErrAppUnknown
	}

	spec.Lock()
	defer spec.Unlock()
	if spec.Process == nil {
		return ErrAppIsNotRunning
	}

	if e := spec.Process.Exit("normal"); e != nil {
		return e
	}
	// we should wait until children process stopped.
	if e := spec.Process.WaitWithTimeout(5 * time.Second); e != nil {
		return ErrProcessBusy
	}
	return nil
}

// Links
func (n *node) Links(process etf.Pid) []etf.Pid {
	return n.processLinks(process)
}

// Monitors
func (n *node) Monitors(process etf.Pid) []etf.Pid {
	return n.processMonitors(process)
}

// MonitorsByName
func (n *node) MonitorsByName(process etf.Pid) []gen.ProcessID {
	return n.processMonitorsByName(process)
}

// MonitoredBy
func (n *node) MonitoredBy(process etf.Pid) []etf.Pid {
	return n.processMonitoredBy(process)
}

// ProvideRPC register given module/function as RPC method
func (n *node) ProvideRPC(module string, function string, fun gen.RPC) error {
	lib.Log("[%s] RPC provide: %s:%s %#v", n.name, module, function, fun)
	rex := n.ProcessByName("rex")
	if rex == nil {
		return fmt.Errorf("RPC is disabled")
	}

	message := gen.MessageManageRPC{
		Provide:  true,
		Module:   module,
		Function: function,
		Fun:      fun,
	}
	if _, err := rex.Direct(message); err != nil {
		return err
	}

	return nil
}

// RevokeRPC unregister given module/function
func (n *node) RevokeRPC(module, function string) error {
	lib.Log("[%s] RPC revoke: %s:%s", n.name, module, function)

	rex := n.ProcessByName("rex")
	if rex == nil {
		return fmt.Errorf("RPC is disabled")
	}

	message := gen.MessageManageRPC{
		Provide:  false,
		Module:   module,
		Function: function,
	}

	if _, err := rex.Direct(message); err != nil {
		return err
	}

	return nil
}

// ProvideRemoteSpawn
func (n *node) ProvideRemoteSpawn(name string, behavior gen.ProcessBehavior) error {
	return n.RegisterBehavior(remoteBehaviorGroup, name, behavior, nil)
}

// RevokeRemoteSpawn
func (n *node) RevokeRemoteSpawn(name string) error {
	return n.UnregisterBehavior(remoteBehaviorGroup, name)
}

// DefaultProtoOptions
func DefaultProtoOptions(handlers int, compression, disableHeaderAtomCache bool) ProtoOptions {
	flags := ProtoFlags{
		DisableHeaderAtomCache: disableHeaderAtomCache,
		EnableBigCreation:      true,
	}
	return ProtoOptions{
		MaxMessageSize:    0, // no limit
		SendQueueLength:   DefaultProtoSendQueueLength,
		RecvQueueLength:   DefaultProtoRecvQueueLength,
		FragmentationUnit: DefaultProroFragmentationUnit,
		NumHandlers:       runtime.GOMAXPROCS(handlers),
		Flags:             flags,
	}
}
