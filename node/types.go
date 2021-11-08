package node

import (
	"crypto/tls"
	"fmt"
	"net"
	"time"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
)

var (
	ErrAppAlreadyLoaded     = fmt.Errorf("Application is already loaded")
	ErrAppAlreadyStarted    = fmt.Errorf("Application is already started")
	ErrAppUnknown           = fmt.Errorf("Unknown application name")
	ErrAppIsNotRunning      = fmt.Errorf("Application is not running")
	ErrNameUnknown          = fmt.Errorf("Unknown name")
	ErrNameOwner            = fmt.Errorf("Not an owner")
	ErrProcessBusy          = fmt.Errorf("Process is busy")
	ErrProcessUnknown       = fmt.Errorf("Unknown process")
	ErrProcessIncarnation   = fmt.Errorf("Process ID belongs to the previous incarnation")
	ErrProcessTerminated    = fmt.Errorf("Process terminated")
	ErrMonitorUnknown       = fmt.Errorf("Unknown monitor reference")
	ErrSenderUnknown        = fmt.Errorf("Unknown sender")
	ErrBehaviorUnknown      = fmt.Errorf("Unknown behavior")
	ErrBehaviorGroupUnknown = fmt.Errorf("Unknown behavior group")
	ErrAliasUnknown         = fmt.Errorf("Unknown alias")
	ErrAliasOwner           = fmt.Errorf("Not an owner")
	ErrNoRoute              = fmt.Errorf("No route to node")
	ErrTaken                = fmt.Errorf("Resource is taken")
	ErrTimeout              = fmt.Errorf("Timed out")
	ErrFragmented           = fmt.Errorf("Fragmented data")

	ErrUnsupported = fmt.Errorf("Not supported")
)

// Distributed operations codes (http://www.erlang.org/doc/apps/erts/erl_dist_protocol.html)
const (
	// node options
	defaultListenRangeBegin uint16 = 15000
	defaultListenRangeEnd   uint16 = 65000

	EnvKeyVersion EnvKey = "ergo:Version"
	EnvKeyNode    EnvKey = "ergo:Node"
)

// Node
type Node interface {
	gen.Core
	// Name returns node name
	Name() string
	// IsAlive returns true if node is still alive
	IsAlive() bool
	// Uptime returns node uptime in seconds
	Uptime() int64
	// Version return node version
	Version() Version
	// Spawn spawns a new process
	Spawn(name string, opts gen.ProcessOptions, object gen.ProcessBehavior, args ...etf.Term) (gen.Process, error)

	// RegisterName
	RegisterName(name string, pid etf.Pid) error
	// UnregisterName
	UnregisterName(name string) error

	LoadedApplications() []gen.ApplicationInfo
	WhichApplications() []gen.ApplicationInfo
	ApplicationInfo(name string) (gen.ApplicationInfo, error)
	ApplicationLoad(app gen.ApplicationBehavior, args ...etf.Term) (string, error)
	ApplicationUnload(appName string) error
	ApplicationStart(appName string, args ...etf.Term) (gen.Process, error)
	ApplicationStartPermanent(appName string, args ...etf.Term) (gen.Process, error)
	ApplicationStartTransient(appName string, args ...etf.Term) (gen.Process, error)
	ApplicationStop(appName string) error

	ProvideRPC(module string, function string, fun gen.RPC) error
	RevokeRPC(module, function string) error
	ProvideRemoteSpawn(name string, object gen.ProcessBehavior) error
	RevokeRemoteSpawn(name string) error

	// AddStaticRoute adds static route for the given node name which makes node skip resolving process
	AddStaticRoute(name string, port uint16, options RouteOptions) error
	// AddStaticRouteExt adds static route with extra options
	RemoveStaticRoute(name string) bool
	// StaticRoutes returns list of routes added using AddStaticRoute
	StaticRoutes() []Route

	// Connect sets up a connection to node
	Connect(node string) error

	Links(process etf.Pid) []etf.Pid
	Monitors(process etf.Pid) []etf.Pid
	MonitorsByName(process etf.Pid) []gen.ProcessID
	MonitoredBy(process etf.Pid) []etf.Pid

	Stop()
	Wait()
	WaitWithTimeout(d time.Duration) error
}

// Version
type Version struct {
	Release string
	Prefix  string
	OTP     int
}

// CoreRouter routes messages from/to remote node
type CoreRouter interface {

	//
	// implemented by core
	//

	// RouteSend routes message by Pid
	RouteSend(from etf.Pid, to etf.Pid, message etf.Term) error
	// RouteSendReg routes message by registered process name (gen.ProcessID)
	RouteSendReg(from etf.Pid, to gen.ProcessID, message etf.Term) error
	// RouteSendAlias routes message by process alias
	RouteSendAlias(from etf.Pid, to etf.Alias, message etf.Term) error

	ProcessByPid(pid etf.Pid) gen.Process
	ProcessByName(name string) gen.Process
	ProcessByAlias(alias etf.Alias) gen.Process

	GetConnection(nodename string) (ConnectionInterface, error)

	//
	// implemented by monitor
	//

	// RouteLink makes linking of the given two processes
	RouteLink(pidA etf.Pid, pidB etf.Pid) error
	// RouteUnlink makes unlinking of the given two processes
	RouteUnlink(pidA etf.Pid, pidB etf.Pid) error
	// RouteExit routes MessageExit to the linked process
	RouteExit(to etf.Pid, terminated etf.Pid, reason string) error
	// RouteMonitorReg makes monitor to the given registered process name (gen.ProcessID)
	RouteMonitorReg(by etf.Pid, process gen.ProcessID, ref etf.Ref) error
	// RouteMonitor makes monitor to the given Pid
	RouteMonitor(by etf.Pid, process etf.Pid, ref etf.Ref) error
	RouteDemonitor(by etf.Pid, ref etf.Ref) error
	RouteMonitorExitReg(to etf.Pid, terminated gen.ProcessID, reason string, ref etf.Ref) error
	RouteMonitorExit(to etf.Pid, terminated etf.Pid, reason string, ref etf.Ref) error

	RouteSpawnRequest() error
	RouteSpawnReply() error
	RouteProxy() error
}

// NetworkRoute
type NetworkRoute struct {
	Port   int
	Cookie string
	TLS    bool
}

// TLSmodeType should be one of TLSmodeDisabled (default), TLSmodeAuto or TLSmodeStrict
type TLSMode int

// ProxyMode
type ProxyMode int

const (
	// TLSModeDisabled no TLS encryption
	TLSModeDisabled TLSMode = 0
	// TLSModeAuto generate self-signed certificate
	TLSModeAuto TLSMode = 1
	// TLSModeStrict with validation certificate
	TLSModeStrict TLSMode = 2

	// ProxyModeDisabled
	ProxyModeDisabled ProxyMode = 0
	ProxyModeEnabled  ProxyMode = 1
)

// Options defines bootstrapping options for the node
type Options struct {
	// Applications application list that must be started
	Applications []gen.ApplicationBehavior
	// Env node environment
	Env map[gen.EnvKey]interface{}

	// Creation. Default value: uint32(time.Now().Unix())
	Creation uint32

	// network options

	// Listen defines a port number for accepting incoming connections
	Listen uint16
	// ListenBegin and ListenEnd define a range of the port numbers where
	// the node looking for available free port number for the listening.
	// Default values 15000 and 65000 accordingly
	ListenBegin uint16
	ListenEnd   uint16

	// StaticRoutesOnly disables port mapping service (default is EPMD client) and
	// makes resolving localy only for nodes added using gen.AddStaticRoute
	StaticRoutesOnly bool

	// ResolverListen defines port for the port mapping service (default is EPMD server)
	// Default 4369
	ResolverListen uint16
	// ResolverHost defines host for the listening.
	ResolverHost string
	// ResolverDisableServer disables embedded port mapping service (default is EPMD server)
	ResolverDisableServer bool
	// Resolver defines a resolving service (default is EPMD service, client and server)
	Resolver Resolver

	// ProxyMode enables/disables proxy mode for the node
	ProxyMode ProxyMode

	// TLS settings
	TLSMode      TLSMode
	TLSCrtServer string
	TLSKeyServer string
	TLSCrtClient string
	TLSKeyClient string

	// Handshake defines a handshake handler. By default is using
	// DIST handshake created with dist.CreateHandshake(...)
	Handshake Handshake
	// Proto defines a proto handler. By default is using
	// DIST proto created with dist.CreateProto(...)
	Proto Proto

	// enable Ergo Cloud support
	EnableCloud  bool
	CloudOptions CloudOptions
}

type CloudOptions struct {
	ID     string
	Cookie string
}

type TLS struct {
	Enabled bool
	Mode
	Server tls.Certificate
	Client tls.Certificate
	Config tls.Config
}

// Connection
type Connection struct {
	ConnectionInterface
}

// ConnectionInterface
type ConnectionInterface interface {
	Send(from gen.Process, to etf.Pid, message etf.Term) error
	SendReg(from gen.Process, to gen.ProcessID, message etf.Term) error
	SendAlias(from gen.Process, to etf.Alias, message etf.Term) error

	Link(local gen.Process, remote etf.Pid) error
	Unlink(local gen.Process, remote etf.Pid) error
	LinkExit(to etf.Pid, terminated etf.Pid, reason string) error

	Monitor(by etf.Pid, process etf.Pid, ref etf.Ref) error
	Demonitor(by etf.Pid, process etf.Pid, ref etf.Ref) error
	MonitorExit(to etf.Pid, terminated etf.Pid, reason string, ref etf.Ref) error

	MonitorReg(by etf.Pid, process gen.ProcessID, ref etf.Ref) error
	DemonitorReg(by etf.Pid, process gen.ProcessID, ref etf.Ref) error
	MonitorExitReg(to etf.Pid, terminated gen.Process, reason string, ref etf.Ref) error

	SpawnRequest()

	Proxy()
	ProxyReg()

	Options() ProtoOptions
}

// Handshake template struct for the custom Handshake implementation
type Handshake struct {
	HandshakeInterface
}

// Handshake defines handshake interface
type HandshakeInterface interface {
	// Init initialize handshake.
	Init(nodename string) (HandshakeVersion, error)
	// Start initiates handshake process. Returns proto options to override default ones.
	Start(c net.Conn, creation uint32, enabledTLS bool) (ConnectionInterface, error)
	// Accept accepts handshake process initiated by another side of this connection. Returns
	// proto options to override default ones and the peer name.
	Accept(c net.Conn, creation uint32, enabledTLS bool) (ConnectionInterface, string)
}

type HandshakeVersion int

// Proto template struct for the custom Proto implementation
type Proto struct {
	ProtoInterface
}

// Proto defines proto interface for the custom Proto implementation
type ProtoInterface interface {
	// Init initialize proto handler and set default options
	Init(router Router) error
	// Serve serves connection with options defined by handshake.
	Serve(c ConnectionInterface)
}

// CustomProtoOptions a custom set of proto options
type CustomProtoOptions interface{}

// ProtoOptions
type ProtoOptions struct {
	// MaxMessageSize limit the message size. Default 0 (no limit)
	MaxMessageSize int64
	// NumHandlers defines the number of readers/writers per connection. Default is the number of CPU.
	NumHandlers int
	// SendQueueLength defines queue size of handler for the outgoing messages. Default 100.
	SendQueueLength int
	// RecvQueueLength defines queue size of handler for the incoming messages. Default 100.
	RecvQueueLength int
	// FragmentationUnit defines unit size for the fragmentation feature. Default 65000
	FragmentationUnit int
	// DisableHeaderAtomCache makes proto handler disable header atom cache feature
	DisableHeaderAtomCache bool
	// Compression enable/disable compression
	Compression bool
	// Custom brings a custom set of options to the ProtoInterface.Serve handler
	Custom CustomProtoOptions
}

// ResolverOptions defines resolving options
type ResolverOptions struct {
	NodeVersion      Version
	HandshakeVersion HandshakeVersion
	EnabledTLS       bool
	EnabledProxy     bool
}

// Resolver defines resolving interface
type Resolver interface {
	Register(ctx contex.Context, options ResolverOptions) error
	Resolve(name string) (Route, error)
}

// CustomRouteOptions a custom set of route options
type CustomRouteOptions interface{}

// RouteOptions
type RouteOptions struct {
	Cookie       string
	EnabledTLS   bool
	EnabledProxy bool
	IsErgo       bool

	TLSConfig *tls.Config
	Handshake Handshake
	Proto     Proto
	Custom    CustomRouteOptions
}

// Route
type Route struct {
	NodeName string
	Name     string
	Host     string
	Port     uint16
	RouteOptions
}
