package node

import (
	"context"
	"crypto/cipher"
	"crypto/tls"
	"net"
	"time"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/lib"
)

const (
	// node options
	defaultListenBegin     uint16        = 15000
	defaultListenEnd       uint16        = 65000
	defaultKeepAlivePeriod time.Duration = 15
	defaultProxyPathLimit  int           = 32

	DefaultProcessMailboxSize   int = 100
	DefaultProcessDirectboxSize int = 10

	EnvKeyVersion     gen.EnvKey = "ergo:Version"
	EnvKeyNode        gen.EnvKey = "ergo:Node"
	EnvKeyRemoteSpawn gen.EnvKey = "ergo:RemoteSpawn"

	DefaultProtoRecvQueueLength   int = 100
	DefaultProtoSendQueueLength   int = 100
	DefaultProtoFragmentationUnit int = 65000

	DefaultCompressionLevel     int = -1
	DefaultCompressionThreshold int = 1024

	DefaultProxyMaxHop int = 8

	EventNetwork gen.Event = "network"
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

	AddProxyRoute(proxy ProxyRoute) error
	RemoveProxyRoute(name string) bool
	// ProxyRoutes returns list of proxy routes added using AddProxyRoute
	ProxyRoutes() []ProxyRoute
	// ProxyRoute returns proxy route added using AddProxyRoute
	ProxyRoute(name string) (ProxyRoute, bool)

	// Resolve
	Resolve(node string) (Route, error)
	// ResolveProxy resolves proxy route. Checks for the proxy route added using AddProxyRoute.
	// If it wasn't found makes request to the registrar.
	ResolveProxy(node string) (ProxyRoute, error)

	// Connect sets up a connection to node
	Connect(node string) error
	// Disconnect close connection to the node
	Disconnect(node string) error
	// Nodes returns the list of connected nodes
	Nodes() []string
	// NodesIndirect returns the list of nodes connected via proxies
	NodesIndirect() []string
	// NetworkStats returns network statistics of the connection with the node. Returns error
	// ErrUnknown if connection with given node is not established.
	NetworkStats(name string) (NetworkStats, error)

	Links(process etf.Pid) []etf.Pid
	Monitors(process etf.Pid) []etf.Pid
	MonitorsByName(process etf.Pid) []gen.ProcessID
	MonitoredBy(process etf.Pid) []etf.Pid

	Stats() NodeStats

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

	// Listeners node can have multiple listening interface at once. If this list is empty
	// the default listener will be using. Only the first listener will be registered on
	// the Registrar
	Listeners []Listener

	// Flags defines option flags of this node for the outgoing connection
	Flags Flags

	// TLS settings
	TLS *tls.Config

	// StaticRoutesOnly disables resolving service (default is EPMD client) and
	// makes resolving localy only for nodes added using gen.AddStaticRoute
	StaticRoutesOnly bool

	// Registrar defines a registrar service (default is EPMD service, client and server)
	Registrar Registrar

	// Compression defines default compression options for the spawning processes.
	Compression Compression

	// Handshake defines a handshake handler. By default is using
	// DIST handshake created with dist.CreateHandshake(...)
	Handshake HandshakeInterface

	// Proto defines a proto handler. By default is using
	// DIST proto created with dist.CreateProto(...)
	Proto ProtoInterface

	// Cloud enable Ergo Cloud support
	Cloud Cloud

	// Proxy options
	Proxy Proxy

	// System options for the system application
	System System
}

type Listener struct {
	// Cookie cookie for the incoming connection to this listener. Leave it empty in
	// case of using the node's cookie.
	Cookie string
	// Listen defines a listening port number for accepting incoming connections.
	Listen uint16
	// ListenBegin and ListenEnd define a range of the port numbers where
	// the node looking for available free port number for the listening.
	// Default values 15000 and 65000 accordingly
	ListenBegin uint16
	ListenEnd   uint16
	// Handshake if its nil the default TLS (Options.TLS) will be using
	TLS *tls.Config
	// Handshake if its nil the default Handshake (Options.Handshake) will be using
	Handshake HandshakeInterface
	// Proto if its nil the default Proto (Options.Proto) will be using
	Proto ProtoInterface
	// Flags defines option flags of this node for the incoming connection
	// on this port. If its disabled the default Flags (Options.Flags) will be using
	Flags Flags
}

type Cloud struct {
	Enable  bool
	Cluster string
	Cookie  string
	Flags   CloudFlags
	Timeout time.Duration
}

type Proxy struct {
	// Transit allows to use this node as a proxy
	Transit bool
	// Accept incoming proxy connections
	Accept bool
	// Cookie sets cookie for incoming connections
	Cookie string
	// Flags sets options for incoming connections
	Flags ProxyFlags
	// Routes sets options for outgoing connections
	Routes map[string]ProxyRoute
}

type System struct {
	DisableAnonMetrics bool
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

	Creation() uint32
	Stats() NetworkStats
}

// Handshake template struct for the custom Handshake implementation
type Handshake struct {
	HandshakeInterface
}

// Handshake defines handshake interface
type HandshakeInterface interface {
	// Mandatory:

	// Init initialize handshake.
	Init(nodename string, creation uint32, flags Flags) error

	// Optional:

	// Start initiates handshake process. Argument tls means the connection is wrapped by TLS
	// Returns the name of connected peer, Flags and Creation wrapped into HandshakeDetails struct
	Start(remote net.Addr, conn lib.NetReadWriter, tls bool, cookie string) (HandshakeDetails, error)
	// Accept accepts handshake process initiated by another side of this connection.
	// Returns the name of connected peer, Flags and Creation wrapped into HandshakeDetails struct
	Accept(remote net.Addr, conn lib.NetReadWriter, tls bool, cookie string) (HandshakeDetails, error)
	// Version handshake version. Must be implemented if this handshake is going to be used
	// for the accepting connections (this method is used in registration on the Resolver)
	Version() HandshakeVersion
}

// HandshakeDetails
type HandshakeDetails struct {
	// Name node name
	Name string
	// Flags node flags
	Flags Flags
	// Creation
	Creation uint32
	// Version
	Version int
	// NumHandlers defines the number of readers/writers per connection. Default value is provided by ProtoOptions
	NumHandlers int
	// AtomMapping
	AtomMapping etf.AtomMapping
	// Buffer keeps data received along with the handshake
	Buffer *lib.Buffer
	// Custom allows passing the custom data to the ProtoInterface.Start
	Custom HandshakeCustomDetails
}

type HandshakeCustomDetails interface{}

type HandshakeVersion int

// Proto template struct for the custom Proto implementation
type Proto struct {
	ProtoInterface
}

// Proto defines proto interface for the custom Proto implementation
type ProtoInterface interface {
	// Init initialize connection handler
	Init(ctx context.Context, conn lib.NetReadWriter, nodename string, details HandshakeDetails) (ConnectionInterface, error)
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
}

// Registrar defines registrar interface
type Registrar interface {
	Register(ctx context.Context, nodename string, options RegisterOptions) error
	RegisterProxy(nodename string, maxhop int, flags ProxyFlags) error
	UnregisterProxy(peername string) error
	Resolve(peername string) (Route, error)
	ResolveProxy(peername string) (ProxyRoute, error)
	Config() RegistrarConfig
}

type RegistrarConfig struct {
	Version int
	Config  etf.Term
}

// RegisterOptions defines resolving options
type RegisterOptions struct {
	Port              uint16
	Creation          uint32
	NodeVersion       Version
	HandshakeVersion  HandshakeVersion
	EnableTLS         bool
	EnableProxy       bool
	EnableCompression bool
	Proxy             string
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
	TLS       *tls.Config
	IsErgo    bool
	Handshake HandshakeInterface
	Proto     ProtoInterface
}

// ProxyRoute
type ProxyRoute struct {
	// Name can be either nodename (example@domain) or domain (@domain)
	Name   string
	Proxy  string
	Cookie string
	Flags  ProxyFlags
	MaxHop int // DefaultProxyMaxHop == 8
}

// CloudFlags
type CloudFlags struct {
	Enable              bool
	EnableIntrospection bool
	EnableMetrics       bool
	EnableRemoteSpawn   bool
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
	Creation  uint32
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
	Creation  uint32
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
	Creation  uint32
	PeerName  string
	Block     cipher.Block // made from symmetric key
}

type NetworkStats struct {
	NodeName        string
	BytesIn         uint64
	BytesOut        uint64
	TransitBytesIn  uint64
	TransitBytesOut uint64
	MessagesIn      uint64
	MessagesOut     uint64
}

type NodeStats struct {
	TotalProcesses    uint64
	TotalReferences   uint64
	RunningProcesses  uint64
	RegisteredNames   uint64
	RegisteredAliases uint64

	MonitorsByPid  uint64
	MonitorsByName uint64
	MonitorsNodes  uint64
	Links          uint64

	LoadedApplications  uint64
	RunningApplications uint64

	NetworkConnections uint64
	ProxyConnections   uint64
	TransitConnections uint64
}

type MessageEventNetwork struct {
	PeerName string
	Online   bool
	Proxy    bool
}
