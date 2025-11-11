package gen

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
)

// Network interface provides distributed communication and node connectivity management.
// Retrieved via node.Network(). Handles connections, routing, and remote operations.
//
// Key responsibilities:
// - Managing connections to remote nodes
// - Static and dynamic routing configuration
// - Service registration and discovery via Registrar
// - Security and handshake protocol management
// - Remote spawn and application start permissions
type Network interface {
	// Registrar returns the registrar service for dynamic node discovery.
	// Returns error if no registrar is configured.
	Registrar() (Registrar, error)

	// Cookie returns the authentication cookie for network connections.
	Cookie() string

	// SetCookie sets the authentication cookie for new connections.
	// Existing connections are not affected.
	// Returns ErrIncorrect if cookie is invalid.
	SetCookie(cookie string) error

	// MaxMessageSize returns the maximum message size limit (in bytes).
	// Zero means no limit.
	MaxMessageSize() int

	// SetMaxMessageSize sets the maximum allowed message size (in bytes).
	// Messages exceeding this size are rejected.
	// Zero means unlimited.
	SetMaxMessageSize(size int)

	// NetworkFlags returns the current network capability flags.
	NetworkFlags() NetworkFlags

	// SetNetworkFlags updates network capability flags.
	// Controls features like remote spawn, fragmentation, proxy, important delivery.
	SetNetworkFlags(flags NetworkFlags)

	// Acceptors returns the list of active network acceptors (listeners).
	Acceptors() ([]Acceptor, error)

	// Node returns an existing connection to the given node.
	// Returns ErrNoConnection if no connection exists.
	// Does not attempt to establish a new connection.
	Node(name Atom) (RemoteNode, error)

	// GetNode returns a connection to the given node.
	// If connection doesn't exist, attempts to establish it using routes or registrar.
	// Returns existing connection if already connected.
	// Returns ErrNoConnection if node cannot be reached.
	GetNode(name Atom) (RemoteNode, error)

	// GetNodeWithRoute connects to the given node using the specified route.
	// Creates a new connection even if one already exists.
	// Useful for explicit routing or fallback connections.
	GetNodeWithRoute(name Atom, route NetworkRoute) (RemoteNode, error)

	// Nodes returns a list of currently connected node names.
	Nodes() []Atom

	// AddRoute adds a static route for connecting to nodes matching the pattern.
	// Pattern can use wildcards (e.g., "prod-*@example.com").
	// Weight determines route priority (higher = preferred).
	// Multiple routes for same pattern are tried in weight order.
	AddRoute(match string, route NetworkRoute, weight int) error

	// RemoveRoute removes all static routes matching the pattern.
	RemoveRoute(match string) error

	// Route returns all routes matching the given node name, ordered by weight.
	Route(name Atom) ([]NetworkRoute, error)

	// AddProxyRoute adds a proxy route for the matching pattern.
	// Proxy routes enable connection through intermediate proxy nodes.
	// Weight determines route priority.
	AddProxyRoute(match string, proxy NetworkProxyRoute, weight int) error

	// RemoveProxyRoute removes all proxy routes matching the pattern.
	RemoveProxyRoute(match string) error

	// ProxyRoute returns all proxy routes matching the given node name, ordered by weight.
	ProxyRoute(name Atom) ([]NetworkProxyRoute, error)

	// RegisterProto registers a custom network protocol implementation.
	// Replaces the default EDF protocol. Used for Erlang protocol compatibility.
	RegisterProto(proto NetworkProto)

	// RegisterHandshake registers a custom handshake implementation.
	// Replaces the default handshake protocol.
	RegisterHandshake(handshake NetworkHandshake)

	// EnableSpawn allows remote nodes to spawn the given process on this node.
	// Specify node names to restrict which nodes can spawn, or leave empty for any node.
	// Remote nodes must have EnableRemoteSpawn flag enabled.
	// Returns ErrTaken if spawn name already registered.
	EnableSpawn(name Atom, factory ProcessFactory, nodes ...Atom) error

	// DisableSpawn revokes remote spawn permission for the given process.
	// Specify node names to revoke for specific nodes, or leave empty to revoke for all.
	DisableSpawn(name Atom, nodes ...Atom) error

	// EnableApplicationStart allows remote nodes to start the given application on this node.
	// Specify node names to restrict which nodes can start, or leave empty for any node.
	// Remote nodes must have EnableRemoteApplicationStart flag enabled.
	// Returns ErrTaken if application name already registered.
	EnableApplicationStart(name Atom, nodes ...Atom) error

	// DisableApplicationStart revokes remote application start permission.
	// Specify node names to revoke for specific nodes, or leave empty to revoke for all.
	DisableApplicationStart(name Atom, nodes ...Atom) error

	// Info returns comprehensive network information including connections, routes, and stats.
	Info() (NetworkInfo, error)

	// Mode returns the current network mode (Enabled, Hidden, or Disabled).
	Mode() NetworkMode
}

// RemoteNode interface represents a connection to a remote Ergo node.
// Retrieved via network.Node() or network.GetNode().
// Provides operations for spawning processes and starting applications on the remote node.
//
// Remote operations require:
// - Active network connection
// - Remote node has enabled corresponding permissions (EnableSpawn, EnableApplicationStart)
// - Proper authentication (matching cookies)
type RemoteNode interface {
	// Name returns the remote node name.
	Name() Atom

	// Uptime returns the remote node uptime in seconds.
	// Reported by the remote node during handshake.
	Uptime() int64

	// ConnectionUptime returns the connection uptime in seconds.
	// Time since connection was established.
	ConnectionUptime() int64

	// Version returns the remote node version.
	// Reported during handshake.
	Version() Version

	// Info returns comprehensive information about the remote node and connection.
	// Includes network stats, flags, protocol versions, connection pool details.
	Info() RemoteNodeInfo

	// Spawn requests the remote node to spawn a new process.
	// The 'name' must be enabled on remote via network.EnableSpawn().
	// Process is created on the remote node and its PID is returned.
	// Returns error if remote node rejects the request or doesn't support the process.
	Spawn(name Atom, options ProcessOptions, args ...any) (PID, error)

	// SpawnRegister requests the remote node to spawn and register a process.
	// The spawned process will have the registered name on the remote node.
	// Returns error if spawn not enabled or name already taken on remote.
	SpawnRegister(register Atom, name Atom, options ProcessOptions, args ...any) (PID, error)

	// ApplicationStart starts an application on the remote node.
	// Uses the starting mode defined in ApplicationSpec.Mode.
	// Application must be enabled on remote via network.EnableApplicationStart().
	// Returns error if remote node rejects or application doesn't exist.
	ApplicationStart(name Atom, options ApplicationOptions) error

	// ApplicationStartTemporary starts an application on the remote node in temporary mode.
	// Overrides the ApplicationSpec.Mode setting.
	// Temporary: application stops when any child terminates abnormally.
	ApplicationStartTemporary(name Atom, options ApplicationOptions) error

	// ApplicationStartTransient starts an application on the remote node in transient mode.
	// Overrides the ApplicationSpec.Mode setting.
	// Transient: application stops only on abnormal termination of children.
	ApplicationStartTransient(name Atom, options ApplicationOptions) error

	// ApplicationStartPermanent starts an application on the remote node in permanent mode.
	// Overrides the ApplicationSpec.Mode setting.
	// Permanent: application never stops on child termination.
	ApplicationStartPermanent(name Atom, options ApplicationOptions) error

	// Creation returns the remote node creation timestamp.
	// Used to detect node restarts (creation time changes on restart).
	Creation() int64

	// Disconnect closes the connection to the remote node.
	// All processes with links/monitors to remote processes will receive down/exit messages.
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
	SendResponse(from PID, to PID, options MessageOptions, response any) error
	SendResponseError(from PID, to PID, options MessageOptions, err error) error

	// target terminated
	SendTerminatePID(target PID, reason error) error
	SendTerminateProcessID(target ProcessID, reason error) error
	SendTerminateAlias(target Alias, reason error) error
	SendTerminateEvent(target Event, reason error) error

	// Methods for sending sync request to the remote process
	CallPID(from PID, to PID, options MessageOptions, message any) error
	CallProcessID(from PID, to ProcessID, options MessageOptions, message any) error
	CallAlias(from PID, to Alias, options MessageOptions, message any) error

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

	// NerworkModeHidden makes node to start network with disabled acceptor(s) for the incomming connections.
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

// NetworkOptions configures network settings for the node.
// Part of NodeOptions. Defines how the node communicates with other nodes.
type NetworkOptions struct {
	// Mode sets the network mode.
	// NetworkModeEnabled (default) - full networking with acceptors
	// NetworkModeHidden - can connect out but no acceptors (no incoming connections)
	// NetworkModeDisabled - networking completely disabled
	Mode NetworkMode

	// Cookie is the authentication secret for network connections.
	// Both nodes must have matching cookies to establish connection.
	// If empty, a random cookie is generated (warning logged).
	Cookie string

	// Flags controls network features and capabilities.
	// Enables/disables remote spawn, application start, fragmentation, proxy, etc.
	Flags NetworkFlags

	// Registrar provides dynamic node discovery and service registration.
	// Optional. If set, node registers itself and discovers other nodes dynamically.
	// Examples: etcd registrar, Saturn registrar.
	Registrar Registrar

	// Handshake sets the handshake protocol for connection establishment.
	// If not set, uses default handshake implementation.
	// Custom handshake can implement different authentication schemes.
	Handshake NetworkHandshake

	// Proto sets the network protocol for message encoding/decoding.
	// If not set, uses default EDF (Ergo Data Format) protocol.
	// Can be replaced with Erlang protocol for Erlang/OTP compatibility.
	Proto NetworkProto

	// Acceptors configures listeners for incoming connections.
	// Node can have multiple acceptors on different ports/interfaces.
	// Empty means no acceptors (same as NetworkModeHidden).
	Acceptors []AcceptorOptions

	// InsecureSkipVerify disables TLS certificate verification.
	// Only use for testing or trusted networks. Security risk in production.
	InsecureSkipVerify bool

	// MaxMessageSize limits the size of incoming messages (in bytes).
	// Messages exceeding this limit are rejected.
	// Zero (default) means unlimited. Recommended: set to prevent DoS attacks.
	MaxMessageSize int

	// ProxyAccept configures settings for incoming proxy connections.
	// Allows other nodes to connect through this node as a proxy.
	ProxyAccept ProxyAcceptOptions

	// ProxyTransit configures settings for proxy connections through this node.
	// Controls how proxy connections are routed through this node.
	ProxyTransit ProxyTransitOptions

	// TODO
	// FragmentationUnit chunck size in bytes
	//FragmentationUnit int
}

type ProxyAcceptOptions struct {
	// Cookie sets cookie for incoming connections
	Cookie string
	// Flags sets options for incoming connections
	Flags NetworkProxyFlags
}

type ProxyTransitOptions struct {
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
	// EnableFragmentation enables support fragmentation messages
	EnableFragmentation bool
	// EnableProxyTransit enables support for transit proxy connection
	EnableProxyTransit bool
	// EnableProxyAccept enables support for incoming proxy connection
	EnableProxyAccept bool
	// EnableImportantDelivery enables support 'important' flag
	EnableImportantDelivery bool
}

// we must be able to extend this structure by introducing new features.
// it is using in the handshake process. to keep capability
// use the custom marshaling for this type.
func (nf NetworkFlags) MarshalEDF(w io.Writer) error {
	var flags uint64
	var buf [8]byte
	if nf.Enable == false {
		w.Write(buf[:])
		return nil
	}
	flags = 1 // nf.Enable = true
	if nf.EnableRemoteSpawn == true {
		flags |= 2
	}
	if nf.EnableRemoteApplicationStart == true {
		flags |= 4
	}
	if nf.EnableFragmentation == true {
		flags |= 8
	}
	if nf.EnableProxyTransit == true {
		flags |= 16
	}
	if nf.EnableProxyAccept == true {
		flags |= 32
	}
	if nf.EnableImportantDelivery == true {
		flags |= 64
	}
	binary.BigEndian.PutUint64(buf[:], flags)
	w.Write(buf[:])
	return nil
}

func (nf *NetworkFlags) UnmarshalEDF(buf []byte) error {
	if len(buf) < 8 {
		return fmt.Errorf("unable to unmarshal NetworkFlags")
	}
	flags := binary.BigEndian.Uint64(buf)
	nf.Enable = (flags & 1) > 0
	if nf.Enable == false {
		return nil
	}
	nf.EnableRemoteSpawn = (flags & 2) > 0
	nf.EnableRemoteApplicationStart = (flags & 4) > 0
	nf.EnableFragmentation = (flags & 8) > 0
	nf.EnableProxyTransit = (flags & 16) > 0
	nf.EnableProxyAccept = (flags & 32) > 0
	nf.EnableImportantDelivery = (flags & 64) > 0
	return nil
}

// NetworkProxyFlags
type NetworkProxyFlags struct {
	Enable                       bool
	EnableRemoteSpawn            bool
	EnableRemoteApplicationStart bool
	EnableEncryption             bool
	EnableImportantDelivery      bool
}

func (npf NetworkProxyFlags) MarshalEDF(w io.Writer) error {
	// TODO
	return nil
}

func (npf *NetworkProxyFlags) UnmarshalEDF(buf []byte) error {
	// TODO
	return nil
}

// RemoteNodeInfo contains detailed information about a remote node and its connection.
// Retrieved via remoteNode.Info() or as part of NetworkInfo.
// Includes node details, connection stats, protocol versions, and traffic metrics.
type RemoteNodeInfo struct {
	// Node is the remote node name.
	Node Atom

	// Uptime is the remote node uptime in seconds.
	// Reported by remote during handshake.
	Uptime int64

	// ConnectionUptime is the connection age in seconds.
	// Time since this connection was established.
	ConnectionUptime int64

	// Version is the remote node version information.
	// Includes Name, Release, License details.
	Version Version

	// HandshakeVersion is the handshake protocol version used.
	// Negotiated during connection establishment.
	HandshakeVersion Version

	// ProtoVersion is the network protocol version in use.
	// EDF protocol version or Erlang protocol version.
	ProtoVersion Version

	// NetworkFlags shows the remote node's network capabilities.
	// Indicates what features are supported (remote spawn, fragmentation, etc.).
	NetworkFlags NetworkFlags

	// PoolSize is the number of TCP connections in the connection pool.
	// Multiple connections used for load balancing and ordering.
	PoolSize int

	// PoolDSN lists the connection strings (host:port) for each pooled connection.
	PoolDSN []string

	// MaxMessageSize is the remote node's message size limit (in bytes).
	// Reported during handshake. Messages exceeding this are rejected.
	MaxMessageSize int

	// MessagesIn is the total number of messages received from this remote node.
	MessagesIn uint64

	// MessagesOut is the total number of messages sent to this remote node.
	MessagesOut uint64

	// BytesIn is the total bytes received from this remote node.
	BytesIn uint64

	// BytesOut is the total bytes sent to this remote node.
	BytesOut uint64

	// TransitBytesIn is the total proxy transit bytes received through this connection.
	// Only relevant if this connection is used as a proxy.
	TransitBytesIn uint64

	// TransitBytesOut is the total proxy transit bytes sent through this connection.
	// Only relevant if this connection is used as a proxy.
	TransitBytesOut uint64
}

// AcceptorOptions configures a network listener (acceptor) for incoming connections.
// Part of NetworkOptions.Acceptors. Node can have multiple acceptors on different ports/interfaces.
type AcceptorOptions struct {
	// Cookie is the authentication secret for incoming connections to this acceptor.
	// If empty, uses the node's default cookie.
	// Allows different acceptors to have different authentication.
	Cookie string

	// Host specifies the network interface to listen on.
	// Examples: "localhost", "0.0.0.0", "192.168.1.100"
	// If empty, extracts hostname from node name (e.g., "node@hostname" → "hostname").
	Host string

	// Port is the TCP port number for incoming connections.
	// Default: 15000 if not specified.
	Port uint16

	// PortRange defines the range of ports to try if Port is unavailable.
	// Attempts ports from Port to (Port + PortRange).
	// Example: Port=15000, PortRange=10 tries 15000-15010.
	// Useful for avoiding port conflicts.
	PortRange uint16

	// TCP specifies the TCP network type.
	// "tcp4" (default) - IPv4 only
	// "tcp6" - IPv6 only
	// "tcp" - both IPv4 and IPv6
	TCP string

	// BufferSize sets the TCP connection buffer size (in bytes).
	// Affects read/write performance. Zero uses system default.
	BufferSize int

	// MaxMessageSize limits incoming message size for this acceptor (in bytes).
	// Overrides NetworkOptions.MaxMessageSize for this specific acceptor.
	// Zero means use global MaxMessageSize setting.
	MaxMessageSize int

	// Flags controls network features for connections through this acceptor.
	// Can have different capabilities than other acceptors.
	Flags NetworkFlags

	// AtomMapping provides atom translation for incoming connections.
	// Maps remote atom names to local equivalents.
	// Useful for name compatibility between different clusters.
	AtomMapping map[Atom]Atom

	// CertManager provides TLS certificates for secure connections.
	// If set, acceptor uses TLS. If nil, uses plain TCP.
	CertManager CertManager

	// InsecureSkipVerify disables TLS certificate verification for this acceptor.
	// Only use for testing. Security risk in production.
	InsecureSkipVerify bool

	// Registrar overrides the default registrar for this acceptor.
	// If set, this acceptor registers with a different service registry.
	Registrar Registrar

	// Handshake overrides the default handshake protocol for this acceptor.
	// Allows different acceptors to use different authentication schemes.
	Handshake NetworkHandshake

	// Proto overrides the default network protocol for this acceptor.
	// Allows mixing EDF and Erlang protocols on different ports.
	Proto NetworkProto
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

// HandshakeOptions configures the handshake process for establishing connections.
// Used internally during connection establishment (Start, Accept, Join).
// Contains authentication, security, and capability negotiation settings.
type HandshakeOptions struct {
	// Cookie is the authentication secret for this connection.
	// Both sides must provide matching cookies to establish connection.
	// Used in challenge-response authentication (SHA256 digest).
	Cookie string

	// Flags declares this node's network capabilities to the peer.
	// Peer will know what features are supported (remote spawn, fragmentation, etc.).
	// Negotiated during handshake.
	Flags NetworkFlags

	// CertManager provides TLS certificates for secure connections.
	// If set, connection uses TLS encryption.
	// If nil, connection is unencrypted (plain TCP).
	CertManager CertManager

	// MaxMessageSize is this node's incoming message size limit (in bytes).
	// Communicated to peer during handshake so peer knows the limit.
	// Peer will reject sending messages exceeding this size.
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

// NetworkInfo contains comprehensive network status and configuration information.
// Retrieved via network.Info(). Provides a complete snapshot of networking state.
type NetworkInfo struct {
	// Mode is the current network mode (Enabled, Hidden, or Disabled).
	Mode NetworkMode

	// Registrar contains information about the registrar service.
	// Empty if no registrar is configured.
	Registrar RegistrarInfo

	// Acceptors lists all active network acceptors (listeners).
	// Includes port, host, and stats for each listener.
	Acceptors []AcceptorInfo

	// MaxMessageSize is the global message size limit (in bytes).
	// Zero means unlimited.
	MaxMessageSize int

	// HandshakeVersion is the default handshake protocol version.
	HandshakeVersion Version

	// ProtoVersion is the default network protocol version (EDF or Erlang).
	ProtoVersion Version

	// Nodes lists all currently connected remote node names.
	Nodes []Atom

	// Routes lists all configured static routes.
	// Includes pattern, resolver, weight, and connection details.
	Routes []RouteInfo

	// ProxyRoutes lists all configured proxy routes.
	// Includes pattern, proxy settings, and max hop count.
	ProxyRoutes []ProxyRouteInfo

	// Flags shows the node's network capabilities.
	// Indicates what features are enabled globally.
	Flags NetworkFlags

	// EnabledSpawn lists processes that remote nodes are allowed to spawn.
	// Includes process name, behavior, and which nodes can spawn it.
	EnabledSpawn []NetworkSpawnInfo

	// EnabledApplicationStart lists applications that remote nodes can start.
	// Includes application name and which nodes can start it.
	EnabledApplicationStart []NetworkApplicationStartInfo
}

// NetworkSpawnInfo describes a process enabled for remote spawning.
type NetworkSpawnInfo struct {
	// Name is the process name that remote nodes can spawn.
	Name Atom

	// Behavior is the type name of the ProcessBehavior.
	Behavior string

	// Nodes lists which remote nodes can spawn this process.
	// Empty means any node can spawn it.
	Nodes []Atom
}

// NetworkApplicationStartInfo describes an application enabled for remote starting.
type NetworkApplicationStartInfo struct {
	// Name is the application name that remote nodes can start.
	Name Atom

	// Nodes lists which remote nodes can start this application.
	// Empty means any node can start it.
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
