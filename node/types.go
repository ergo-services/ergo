package node

import (
	"context"
	"crypto/cipher"
	"crypto/tls"
	"fmt"
	"io"
	"time"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/lib"
)

var (
	ErrAppAlreadyLoaded     = fmt.Errorf("application is already loaded")
	ErrAppAlreadyStarted    = fmt.Errorf("application is already started")
	ErrAppUnknown           = fmt.Errorf("unknown application name")
	ErrAppIsNotRunning      = fmt.Errorf("application is not running")
	ErrNameUnknown          = fmt.Errorf("unknown name")
	ErrNameOwner            = fmt.Errorf("not an owner")
	ErrProcessBusy          = fmt.Errorf("process is busy")
	ErrProcessUnknown       = fmt.Errorf("unknown process")
	ErrProcessIncarnation   = fmt.Errorf("process ID belongs to the previous incarnation")
	ErrProcessTerminated    = fmt.Errorf("process terminated")
	ErrMonitorUnknown       = fmt.Errorf("unknown monitor reference")
	ErrSenderUnknown        = fmt.Errorf("unknown sender")
	ErrBehaviorUnknown      = fmt.Errorf("unknown behavior")
	ErrBehaviorGroupUnknown = fmt.Errorf("unknown behavior group")
	ErrAliasUnknown         = fmt.Errorf("unknown alias")
	ErrAliasOwner           = fmt.Errorf("not an owner")
	ErrNoRoute              = fmt.Errorf("no route to node")
	ErrTaken                = fmt.Errorf("resource is taken")
	ErrTimeout              = fmt.Errorf("timed out")
	ErrFragmented           = fmt.Errorf("fragmented data")

	ErrUnsupported     = fmt.Errorf("not supported")
	ErrPeerUnsupported = fmt.Errorf("peer does not support this feature")

	ErrProxyUnknownRequest   = fmt.Errorf("unknown proxy request")
	ErrProxyTransitDisabled  = fmt.Errorf("proxy feature disabled")
	ErrProxyNoRoute          = fmt.Errorf("no proxy route to node")
	ErrProxyConnect          = fmt.Errorf("can't establish proxy connection")
	ErrProxyHopExceeded      = fmt.Errorf("proxy hop is exceeded")
	ErrProxyLoopDetected     = fmt.Errorf("proxy loop detected")
	ErrProxyPathTooLong      = fmt.Errorf("proxy path too long")
	ErrProxySessionUnknown   = fmt.Errorf("unknown session id")
	ErrProxySessionDuplicate = fmt.Errorf("session is already exist")
)

const (
	// node options
	defaultListenBegin     uint16        = 15000
	defaultListenEnd       uint16        = 65000
	defaultKeepAlivePeriod time.Duration = 15
	defaultProxyPathLimit  int           = 32

	EnvKeyVersion     gen.EnvKey = "ergo:Version"
	EnvKeyNode        gen.EnvKey = "ergo:Node"
	EnvKeyRemoteSpawn gen.EnvKey = "ergo:RemoteSpawn"

	DefaultProtoRecvQueueLength   int = 100
	DefaultProtoSendQueueLength   int = 100
	DefaultProroFragmentationUnit int = 65000

	DefaultCompressionLevel     int = -1
	DefaultCompressionThreshold int = 1024

	DefaultProxyMaxHop int = 8
)

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
	// ListEnv returns a map of configured Node environment variables.
	ListEnv() map[gen.EnvKey]interface{}
	// SetEnv set node environment variable with given name. Use nil value to remove variable with given name. Ignores names with "ergo:" as a prefix.
	SetEnv(name gen.EnvKey, value interface{})
	// Env returns value associated with given environment name.
	Env(name gen.EnvKey) interface{}

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

	// AddStaticRoute adds static route for the given name
	AddStaticRoute(node string, host string, port uint16, options RouteOptions) error
	// AddStaticRoutePort adds static route for the given node name which makes node skip resolving port process
	AddStaticRoutePort(node string, port uint16, options RouteOptions) error
	// AddStaticRouteOptions adds static route options for the given node name which does regular port resolving but applies static options
	AddStaticRouteOptions(node string, options RouteOptions) error
	// Remove static route removes static route with given name
	RemoveStaticRoute(name string) bool
	// StaticRoutes returns list of routes added using AddStaticRoute
	StaticRoutes() []Route
	// StaticRoute returns Route for the given name. Returns false if it doesn't exist.
	StaticRoute(name string) (Route, bool)

	AddProxyRoute(name string, proxy ProxyRoute) error
	RemoveProxyRoute(name string) bool
	ProxyRoutes() []ProxyRoute
	ProxyRoute(name string) (ProxyRoute, bool)

	// Resolve
	Resolve(peername string) (Route, error)

	// Connect sets up a connection to node
	Connect(nodename string) error
	// Disconnect close connection to the node
	Disconnect(nodename string) error
	// Nodes returns the list of connected nodes
	Nodes() []string
	// NodesIndirect returns the list of nodes connected via proxies
	NodesIndirect() []string

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

	RouteSpawnRequest(node string, behaviorName string, request gen.RemoteSpawnRequest, args ...etf.Term) error
	RouteSpawnReply(to etf.Pid, ref etf.Ref, result etf.Term) error

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
	RouteMonitorExitReg(terminated gen.ProcessID, reason string, ref etf.Ref) error
	RouteMonitorExit(terminated etf.Pid, reason string, ref etf.Ref) error
	// RouteNodeDown
	RouteNodeDown(name string, disconnect *ProxyDisconnect)

	//
	// implemented by network
	//

	// RouteProxyConnectRequest
	RouteProxyConnectRequest(from ConnectionInterface, request ProxyConnectRequest) error
	// RouteProxyConnectReply
	RouteProxyConnectReply(from ConnectionInterface, reply ProxyConnectReply) error
	// RouteProxyConnectCancel
	RouteProxyConnectCancel(from ConnectionInterface, cancel ProxyConnectCancel) error
	// RouteProxyDisconnect
	RouteProxyDisconnect(from ConnectionInterface, disconnect ProxyDisconnect) error
	// RouteProxy returns ErrProxySessionEndpoint if this node is the endpoint of the
	// proxy session. In this case, the packet must be handled on this node with
	// provided ProxySession parameters.
	RouteProxy(from ConnectionInterface, sessionID string, packet *lib.Buffer) error
}

// Options defines bootstrapping options for the node
type Options struct {
	// Applications application list that must be started
	Applications []gen.ApplicationBehavior

	// Env node environment
	Env map[gen.EnvKey]interface{}

	// Creation. Default value: uint32(time.Now().Unix())
	Creation uint32

	// Flags defines enabled options for the running node
	Flags Flags

	// Listen defines a listening port number for accepting incoming connections.
	Listen uint16
	// ListenBegin and ListenEnd define a range of the port numbers where
	// the node looking for available free port number for the listening.
	// Default values 15000 and 65000 accordingly
	ListenBegin uint16
	ListenEnd   uint16

	// TLS settings
	TLS TLS

	// StaticRoutesOnly disables resolving service (default is EPMD client) and
	// makes resolving localy only for nodes added using gen.AddStaticRoute
	StaticRoutesOnly bool

	// Resolver defines a resolving service (default is EPMD service, client and server)
	Resolver Resolver

	// Compression enables compression for outgoing messages (if peer node has this feature enabled)
	Compression Compression

	// Handshake defines a handshake handler. By default is using
	// DIST handshake created with dist.CreateHandshake(...)
	Handshake HandshakeInterface

	// Proto defines a proto handler. By default is using
	// DIST proto created with dist.CreateProto(...)
	Proto ProtoInterface

	// Cloud enable Ergo Cloud support
	Cloud Cloud

	// Proxy enable proxy feature on this node. Disabling this option makes
	// this node to reject any proxy request.
	Proxy Proxy
}

type TLS struct {
	Enable     bool
	Server     tls.Certificate
	Client     tls.Certificate
	SkipVerify bool
}

type Cloud struct {
	Enable bool
	ID     string
	Cookie string
}

type Proxy struct {
	Transit bool
	Flags   ProxyFlags
	Cookie  string // set cookie for incoming connection
	Routes  map[string]ProxyRoute
}

type Compression struct {
	// Enable enables compression for all outgoing messages having size
	// greater than the defined threshold.
	Enable bool
	// Level defines compression level. Value must be in range 1..9 or -1 for the default level
	Level int
	// Threshold defines the minimal message size for the compression.
	// Messages less of this threshold will not be compressed.
	Threshold int
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

	Link(local etf.Pid, remote etf.Pid) error
	Unlink(local etf.Pid, remote etf.Pid) error
	LinkExit(to etf.Pid, terminated etf.Pid, reason string) error

	Monitor(local etf.Pid, remote etf.Pid, ref etf.Ref) error
	Demonitor(local etf.Pid, remote etf.Pid, ref etf.Ref) error
	MonitorExit(to etf.Pid, terminated etf.Pid, reason string, ref etf.Ref) error

	MonitorReg(local etf.Pid, remote gen.ProcessID, ref etf.Ref) error
	DemonitorReg(local etf.Pid, remote gen.ProcessID, ref etf.Ref) error
	MonitorExitReg(to etf.Pid, terminated gen.ProcessID, reason string, ref etf.Ref) error

	SpawnRequest(nodeName string, behaviorName string, request gen.RemoteSpawnRequest, args ...etf.Term) error
	SpawnReply(to etf.Pid, ref etf.Ref, spawned etf.Pid) error
	SpawnReplyError(to etf.Pid, ref etf.Ref, err error) error

	ProxyConnectRequest(connect ProxyConnectRequest) error
	ProxyConnectReply(reply ProxyConnectReply) error
	ProxyConnectCancel(cancel ProxyConnectCancel) error
	ProxyDisconnect(disconnect ProxyDisconnect) error
	ProxyRegisterSession(session ProxySession) error
	ProxyUnregisterSession(id string) error
	ProxyPacket(packet *lib.Buffer) error
}

// Handshake template struct for the custom Handshake implementation
type Handshake struct {
	HandshakeInterface
}

// Handshake defines handshake interface
type HandshakeInterface interface {
	// Init initialize handshake.
	Init(nodename string, creation uint32, flags Flags) error
	// Start initiates handshake process. Argument tls means the connection is wrapped by TLS
	// Returns Flags received from the peer during handshake
	Start(conn io.ReadWriter, tls bool, cookie string) (Flags, error)
	// Accept accepts handshake process initiated by another side of this connection.
	// Returns the name of connected peer and Flags received from the peer.
	Accept(conn io.ReadWriter, tls bool, cookie string) (string, Flags, error)
	// Version handshake version. Must be implemented if this handshake is going to be used
	// for the accepting connections (this method is used in registration on the Resolver)
	Version() HandshakeVersion
}

type HandshakeVersion int

// Proto template struct for the custom Proto implementation
type Proto struct {
	ProtoInterface
}

// Proto defines proto interface for the custom Proto implementation
type ProtoInterface interface {
	// Init initialize connection handler
	Init(ctx context.Context, conn io.ReadWriter, nodename string, peername string, flags Flags) (ConnectionInterface, error)
	// Serve connection
	Serve(connection ConnectionInterface, router CoreRouter)
	// Terminate invoked once Serve callback is finished
	Terminate(connection ConnectionInterface)
}

// ProtoOptions
type ProtoOptions struct {
	// NumHandlers defines the number of readers/writers per connection. Default is the number of CPU
	NumHandlers int
	// MaxMessageSize limit the message size. Default 0 (no limit)
	MaxMessageSize int
	// SendQueueLength defines queue size of handler for the outgoing messages. Default 100.
	SendQueueLength int
	// RecvQueueLength defines queue size of handler for the incoming messages. Default 100.
	RecvQueueLength int
	// FragmentationUnit defines unit size for the fragmentation feature. Default 65000
	FragmentationUnit int
	// Custom brings a custom set of options to the ProtoInterface.Serve handler
	Custom CustomProtoOptions
}

// CustomProtoOptions a custom set of proto options
type CustomProtoOptions interface{}

// Flags
type Flags struct {
	// Enable enable flags customization
	Enable bool
	// EnableHeaderAtomCache enables header atom cache feature
	EnableHeaderAtomCache bool
	// EnableBigCreation
	EnableBigCreation bool
	// EnableBigPidRef accepts a larger amount of data in pids and references
	EnableBigPidRef bool
	// EnableFragmentation enables fragmentation feature for the sending data
	EnableFragmentation bool
	// EnableAlias accepts process aliases
	EnableAlias bool
	// EnableRemoteSpawn accepts remote spawn request
	EnableRemoteSpawn bool
	// Compression compression support
	EnableCompression bool
	// Proxy enables support for incoming proxy connection
	EnableProxy bool
	// Software keepalive enables sending keep alive messages if node doesn't support
	// TCPConn.SetKeepAlive(true). For erlang peers this flag is mandatory.
	EnableSoftwareKeepAlive bool
}

// Resolver defines resolving interface
type Resolver interface {
	Register(ctx context.Context, nodename string, port uint16, options ResolverOptions) error
	Resolve(peername string) (Route, error)
}

// ResolverOptions defines resolving options
type ResolverOptions struct {
	NodeVersion       Version
	HandshakeVersion  HandshakeVersion
	EnableTLS         bool
	EnableProxy       bool
	EnableCompression bool
}

// Route
type Route struct {
	Node    string
	Host    string
	Port    uint16
	Options RouteOptions
}

// RouteOptions
type RouteOptions struct {
	Cookie    string
	EnableTLS bool
	IsErgo    bool
	Cert      tls.Certificate
	Handshake HandshakeInterface
	Proto     ProtoInterface
	Custom    CustomRouteOptions
}

// ProxyRoute
type ProxyRoute struct {
	Proxy  string
	Cookie string
	Flags  ProxyFlags
	MaxHop int // DefaultProxyMaxHop == 8
}

// ProxyFlags
type ProxyFlags struct {
	Enable            bool
	EnableLink        bool
	EnableMonitor     bool
	EnableRemoteSpawn bool
	EnableEncryption  bool
}

// ProxyConnectRequest
type ProxyConnectRequest struct {
	ID        etf.Ref
	To        string // To node
	Digest    []byte // md5(md5(md5(md5(Node)+Cookie)+To)+PublicKey)
	PublicKey []byte
	Flags     ProxyFlags
	Hop       int
	Path      []string
}

// ProxyConnectReply
type ProxyConnectReply struct {
	ID        etf.Ref
	To        string
	Digest    []byte // md5(md5(md5(md5(Node)+Cookie)+To)+symmetric key)
	Cipher    []byte // encrypted symmetric key using PublicKey from the ProxyConnectRequest
	Flags     ProxyFlags
	SessionID string // proxy session ID
	Path      []string
}

// ProxyConnectCancel
type ProxyConnectCancel struct {
	ID     etf.Ref
	From   string
	Reason string
	Path   []string
}

// ProxyDisconnect
type ProxyDisconnect struct {
	Node      string
	Proxy     string
	SessionID string
	Reason    string
}

// Proxy session
type ProxySession struct {
	ID        string
	NodeFlags ProxyFlags
	PeerFlags ProxyFlags
	PeerName  string
	Block     cipher.Block // made from symmetric key
}

// CustomRouteOptions a custom set of route options
type CustomRouteOptions interface{}
