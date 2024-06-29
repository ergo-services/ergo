package gen

import (
	"fmt"
	"net"
)

type Network interface {
	Registrar() (Registrar, error)
	Cookie() string
	SetCookie(cookie string) error
	MaxMessageSize() int
	SetMaxMessageSize(size int)
	NetworkFlags() NetworkFlags
	SetNetworkFlags(flags NetworkFlags)
	Acceptors() ([]Acceptor, error)

	// Node returns existing connection with the given node
	Node(name Atom) (RemoteNode, error)
	// GetNode attempts to connect to the given node if the connection doesn't exist.
	// Otherwise, it returns the existing connection.
	GetNode(name Atom) (RemoteNode, error)
	// GetNodeWithRoute attempts to connect to the given node using provided route
	GetNodeWithRoute(name Atom, route NetworkRoute) (RemoteNode, error)

	AddRoute(match string, route NetworkRoute, weight int) error
	RemoveRoute(match string) error
	Route(name Atom) ([]NetworkRoute, error)

	AddProxyRoute(match string, proxy NetworkProxyRoute, weight int) error
	RemoveProxyRoute(match string) error
	ProxyRoute(name Atom) ([]NetworkProxyRoute, error)

	RegisterProto(proto NetworkProto)
	RegisterHandshake(handshake NetworkHandshake)

	// EnableSpawn allows the starting of the given process by the remote node(s)
	// Leaving argument "nodes" empty makes spawning this process by any remote node
	EnableSpawn(name Atom, factory ProcessFactory, nodes ...Atom) error
	DisableSpawn(name Atom, nodes ...Atom) error

	// EnableApplicationStart allows the starting of the given application by the remote node(s).
	// Leaving argument "nodes" empty makes starting this application by any remote node.
	EnableApplicationStart(name Atom, nodes ...Atom) error
	DisableApplicationStart(name Atom, nodes ...Atom) error

	Info() (NetworkInfo, error)
	Mode() NetworkMode
}

type RemoteNode interface {
	Name() Atom
	Uptime() int64
	ConnectionUptime() int64
	Version() Version
	Info() RemoteNodeInfo

	Spawn(name Atom, options ProcessOptions, args ...any) (PID, error)
	SpawnRegister(register Atom, name Atom, options ProcessOptions, args ...any) (PID, error)

	// ApplicationStart starts application on the remote node.
	// Starting mode is according to the defined in the gen.ApplicationSpec.Mode
	ApplicationStart(name Atom, options ApplicationOptions) error

	// ApplicationStartTemporary starts application on the remote node in temporary mode
	// overriding the value of gen.ApplicationSpec.Mode
	ApplicationStartTemporary(name Atom, options ApplicationOptions) error

	// ApplicationStartTransient starts application on the remote node in transient mode
	// overriding the value of gen.ApplicationSpec.Mode
	ApplicationStartTransient(name Atom, options ApplicationOptions) error

	// ApplicationStartPermanent starts application on the remote node in permanent mode
	// overriding the value of gen.ApplicationSpec.Mode
	ApplicationStartPermanent(name Atom, options ApplicationOptions) error

	Creation() int64

	Disconnect()
}

type Acceptor interface {
	Cookie() string
	SetCookie(cokie string)
	NetworkFlags() NetworkFlags
	SetNetworkFlags(flags NetworkFlags)
	MaxMessageSize() int
	SetMaxMessageSize(size int)
	Info() AcceptorInfo
}

type Connection interface {
	Node() RemoteNode

	// Methods for sending async message to the remote process
	SendPID(from PID, to PID, options MessageOptions, message any) error
	SendProcessID(from PID, to ProcessID, options MessageOptions, message any) error
	SendAlias(from PID, to Alias, options MessageOptions, message any) error

	SendEvent(from PID, options MessageOptions, message MessageEvent) error
	SendExit(from PID, to PID, reason error) error
	SendResponse(from PID, to PID, ref Ref, options MessageOptions, response any) error

	// target terminated
	SendTerminatePID(target PID, reason error) error
	SendTerminateProcessID(target ProcessID, reason error) error
	SendTerminateAlias(target Alias, reason error) error
	SendTerminateEvent(target Event, reason error) error

	// Methods for sending sync request to the remote process
	CallPID(ref Ref, from PID, to PID, options MessageOptions, message any) error
	CallProcessID(ref Ref, from PID, to ProcessID, options MessageOptions, message any) error
	CallAlias(ref Ref, from PID, to Alias, options MessageOptions, message any) error

	// Links
	LinkPID(pid PID, target PID) error
	UnlinkPID(pid PID, target PID) error

	LinkProcessID(pid PID, target ProcessID) error
	UnlinkProcessID(pid PID, target ProcessID) error

	LinkAlias(pid PID, target Alias) error
	UnlinkAlias(pid PID, target Alias) error

	LinkEvent(pid PID, target Event) ([]MessageEvent, error)
	UnlinkEvent(pid PID, targer Event) error

	// Monitors
	MonitorPID(pid PID, target PID) error
	DemonitorPID(pid PID, target PID) error

	MonitorProcessID(pid PID, target ProcessID) error
	DemonitorProcessID(pid PID, target ProcessID) error

	MonitorAlias(pid PID, target Alias) error
	DemonitorAlias(pid PID, target Alias) error

	MonitorEvent(pid PID, target Event) ([]MessageEvent, error)
	DemonitorEvent(pid PID, targer Event) error

	RemoteSpawn(name Atom, options ProcessOptionsExtra) (PID, error)

	Join(c net.Conn, id string, dial NetworkDial, tail []byte) error
	Terminate(reason error)
}

type NetworkMode int

const (
	// NetworkModeEnabled default network mode for the node. It makes node to
	// register on the registrar services providing the port number for the
	// incomming connections
	NetworkModeEnabled NetworkMode = 0

	// NerworkModeHidden makes node to start network with disabled listener(s) for the incomming connections.
	NetworkModeHidden NetworkMode = 1

	// NetworkModeDisabled disables networking for the node entirely.
	NetworkModeDisabled NetworkMode = -1
)

func (nm NetworkMode) String() string {
	switch nm {
	case NetworkModeEnabled:
		return "enabled"
	case NetworkModeHidden:
		return "hidden"
	case NetworkModeDisabled:
		return "disabled"
	}

	return fmt.Sprintf("unknown network mode %d", nm)
}

func (nm NetworkMode) MarshalJSON() ([]byte, error) {
	return []byte("\"" + nm.String() + "\""), nil
}

// NetworkOptions
type NetworkOptions struct {
	Mode NetworkMode
	// Cookie
	Cookie string
	// Flags
	Flags NetworkFlags
	// Registrar default registrar for outgoing connections
	Registrar Registrar
	// Handshake default handshake if registrar returned Route with no handshake version
	Handshake NetworkHandshake
	// Proto default proto if registrar returned Route with no proto version
	Proto NetworkProto

	// Acceptors node can have multiple acceptors at once
	Acceptors []AcceptorOptions
	// InsecureSkipVerify skips the certificate verification
	InsecureSkipVerify bool
	// MaxMessageSize limit the message size for the incoming messages.
	MaxMessageSize int
	// ProxyAccept options for incomming proxy connections
	ProxyAccept ProxyAccept
	// ProxyTransit options for the proxy connections through this node
	ProxyTransit ProxyTransit

	// TODO
	// FragmentationUnit chunck size in bytes
	//FragmentationUnit int
}

type ProxyAccept struct {
	// Cookie sets cookie for incoming connections
	Cookie string
	// Flags sets options for incoming connections
	Flags NetworkProxyFlags
}

type ProxyTransit struct {
	// TODO
	// proxy Routes
	// access control
	// etc
}

// NetworkFlags
type NetworkFlags struct {
	// Enable enable flags customization.
	Enable bool
	// EnableRemoteSpawn accepts remote spawn request
	EnableRemoteSpawn bool
	// EnableRemoteApplicationStart accepts remote request to start application
	EnableRemoteApplicationStart bool
	// EnableProxyTransit enables support for transit proxy connection
	EnableProxyTransit bool
	// EnableProxyAccept enables support for incoming proxy connection
	EnableProxyAccept bool
	// EnableImportantDelivery TODO will be implemented in the future release
	EnableImportantDelivery bool
}

// NetworkProxyFlags
type NetworkProxyFlags struct {
	Enable                       bool
	EnableRemoteSpawn            bool
	EnableRemoteApplicationStart bool
	EnableEncryption             bool
	EnableImportantDelivery      bool
}

type RemoteNodeInfo struct {
	Node             Atom
	Uptime           int64
	ConnectionUptime int64
	Version          Version

	HandshakeVersion Version
	ProtoVersion     Version

	NetworkFlags NetworkFlags

	PoolSize int
	PoolDSN  []string

	MaxMessageSize int
	MessagesIn     uint64
	MessagesOut    uint64

	BytesIn  uint64
	BytesOut uint64

	TransitBytesIn  uint64
	TransitBytesOut uint64
}

type AcceptorOptions struct {
	// Cookie cookie for the incoming connection to this listener. Leave it empty in
	// case of using the node's cookie.
	Cookie string
	// Hostname defines an interface for the listener. Default: takes from the node name.
	Host string
	// Port defines a listening port number for accepting incoming connections. Default 15000
	Port uint16
	// PortRange a range of the ports for the attempts to start listening:
	//   Starting from: <Port>
	//   Ending at: <Port> + <PortRange>
	PortRange uint16
	// TCP defines the TCP network. By default will be used IPv4 only.
	// For IPv6 use "tcp6". To listen on any available address use "tcp"
	TCP string
	// BufferSize defines buffer size for the TCP connection
	BufferSize int
	// MaxMessageSize allows overriding the gen.NetworkOptions.MaxMessageSize setting
	// for the connections accepted by this listener
	MaxMessageSize int

	Flags       NetworkFlags
	AtomMapping map[Atom]Atom

	CertManager        CertManager
	InsecureSkipVerify bool

	Registrar Registrar
	Handshake NetworkHandshake
	Proto     NetworkProto
}

// Handshake defines handshake interface
type NetworkHandshake interface {
	NetworkFlags() NetworkFlags
	// Start initiates handshake process.
	// Cert value has CertManager that was used to create this connection
	Start(NodeHandshake, net.Conn, HandshakeOptions) (HandshakeResult, error)
	// Join is invoking within the NetworkDial to shortcut the handshake process
	Join(NodeHandshake, net.Conn, string, HandshakeOptions) ([]byte, error)
	// Accept accepts handshake process initiated by another side of this connection.
	Accept(NodeHandshake, net.Conn, HandshakeOptions) (HandshakeResult, error)
	// Version
	Version() Version
}

type HandshakeOptions struct {
	Cookie         string
	Flags          NetworkFlags
	MaxMessageSize int
}

type HandshakeResult struct {
	HandshakeVersion Version

	ConnectionID       string
	Peer               Atom
	PeerCreation       int64
	PeerVersion        Version      // peer's version (gen.Node.Version())
	PeerFlags          NetworkFlags // peer's flags
	PeerMaxMessageSize int

	NodeFlags          NetworkFlags
	NodeMaxMessageSize int

	AtomMapping map[Atom]Atom

	// Tail if something is left in the buffer after the handshaking we should
	// pass it to the proto handler
	Tail []byte
	// Custom allows passing the custom data to the proto handler
	Custom any
}

type NetworkDial func(dsn, id string) (net.Conn, []byte, error)

type NetworkProto interface {
	// NewConnection
	NewConnection(core Core, result HandshakeResult, log Log) (Connection, error)
	// Serve connection. Argument dial is the closure to create TCP connection with invoking
	// NetworkHandshake.Join inside to shortcut the handshake process
	Serve(conn Connection, dial NetworkDial) error
	// Version
	Version() Version
}

type NetworkInfo struct {
	Mode NetworkMode

	Registrar        RegistrarInfo
	Acceptors        []AcceptorInfo
	MaxMessageSize   int
	HandshakeVersion Version
	ProtoVersion     Version

	Nodes []Atom

	Routes      []RouteInfo
	ProxyRoutes []ProxyRouteInfo

	Flags                   NetworkFlags
	EnabledSpawn            []NetworkSpawnInfo
	EnabledApplicationStart []NetworkApplicationStartInfo
}

type NetworkSpawnInfo struct {
	Name     Atom
	Behavior string
	Nodes    []Atom
}

type NetworkApplicationStartInfo struct {
	Name  Atom
	Nodes []Atom
}

type NetworkRoute struct {
	Resolver Resolver
	Route    Route

	Cookie             string
	Cert               CertManager
	InsecureSkipVerify bool
	Flags              NetworkFlags

	AtomMapping map[Atom]Atom

	LogLevel LogLevel
}

type NetworkProxyRoute struct {
	Resolver Resolver
	Route    ProxyRoute

	Cookie string
	Flags  NetworkProxyFlags
	MaxHop int // DefaultProxyMaxHop == 8
}
