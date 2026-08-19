package gen

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"reflect"
	"time"
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

	// ResolveApplication resolves deployment locations for the given application.
	// Shortcut for Registrar().Resolver().ResolveApplication(name); returns the
	// same error as Registrar() when no registrar is available.
	ResolveApplication(name Atom) (ApplicationRoutes, error)

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
	// Calling it again for the same name grants the additional nodes (an empty list
	// allows any node); use DisableSpawn to revoke.
	// Remote nodes must have EnableRemoteSpawn flag enabled.
	// Returns ErrTaken if the name is already registered with a different factory.
	EnableSpawn(name Atom, factory ProcessFactory, nodes ...Atom) error

	// DisableSpawn revokes remote spawn permission for the given process.
	// Specify node names to revoke for specific nodes, or leave empty to revoke for all.
	DisableSpawn(name Atom, nodes ...Atom) error

	// EnableApplicationStart allows remote nodes to start the given application on this node.
	// Specify node names to restrict which nodes can start, or leave empty for any node.
	// Calling it again for the same name grants the additional nodes (an empty list
	// allows any node); use DisableApplicationStart to revoke.
	// Remote nodes must have EnableRemoteApplicationStart flag enabled.
	EnableApplicationStart(name Atom, nodes ...Atom) error

	// DisableApplicationStart revokes remote application start permission.
	// Specify node names to revoke for specific nodes, or leave empty to revoke for all.
	DisableApplicationStart(name Atom, nodes ...Atom) error

	// Info returns comprehensive network information including connections, routes, and stats.
	Info() (NetworkInfo, error)

	// Mode returns the current network mode (Enabled, Hidden, or Disabled).
	Mode() NetworkMode

	// Protos returns all registered network protocols.
	Protos() []NetworkProto

	// RegisterType registers a Go type with every proto that implements TypeRegistry.
	// Strict: returns error if any TypeRegistry-capable proto fails.
	// Returns ErrUnsupported if no proto implements TypeRegistry.
	// The EDF proto supports up to 65535 registered types (each gets a compact
	// wire id); registering more returns an error.
	RegisterType(v any) error

	// RegisterTypes registers multiple types as a batch. Each proto resolves
	// inter-type dependencies internally; order of input is irrelevant.
	// Strict aggregation: any per-proto failure fails the call.
	RegisterTypes(types []any) error

	// RegisterError registers a sentinel error for wire transport.
	// Strict aggregation across TypeRegistry-capable protos.
	RegisterError(e error) error

	// RegisterErrors registers multiple sentinel errors as a batch.
	// Strict aggregation across TypeRegistry-capable protos.
	RegisterErrors(errs []error) error

	// RegisterAtom registers an atom for the wire-format atom cache.
	// Strict aggregation across TypeRegistry-capable protos.
	RegisterAtom(a Atom) error

	// RegisterAtoms registers multiple atoms as a batch.
	// Strict aggregation across TypeRegistry-capable protos.
	RegisterAtoms(atoms []Atom) error

	// RegisteredTypes aggregates entries from every TypeRegistry-capable proto.
	// One Go type may appear once per proto; entries carry Proto field set.
	RegisteredTypes() []RegisteredTypeInfo

	// LookupType resolves a registered type name to its reflect.Type via the
	// active wire-format protos. Returns the first match across protos.
	// Accepts either the canonical name ("#pkgpath/Type") or a short name ("Type").
	LookupType(name string) (reflect.Type, bool)
}

// TypeRegistry is implemented by NetworkProto implementations that have
// a wire-format type registry (e.g., EDF). Implementations using a
// schemaless / fixed wire format (e.g., Erlang external term) need not
// implement this interface.
type TypeRegistry interface {
	RegisterType(v any) error
	RegisterTypes(types []any) error
	RegisterError(e error) error
	RegisterErrors(errs []error) error
	RegisterAtom(a Atom) error
	RegisterAtoms(atoms []Atom) error
	RegisteredTypes() []RegisteredTypeInfo
	LookupType(name string) (reflect.Type, bool)
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

	// ApplicationInfo returns information about an application running on the remote node.
	// Queries the remote node for application details including state, mode, uptime, children, etc.
	// Returns ErrApplicationUnknown if application doesn't exist on remote node.
	// Returns ErrTimeout if remote doesn't respond within DefaultRequestTimeout.
	// Returns ErrNoConnection if connection to remote node is lost.
	ApplicationInfo(name Atom) (ApplicationInfo, error)

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

func (nm *NetworkMode) UnmarshalJSON(data []byte) error {
	s, err := unmarshalName(data)
	if err != nil {
		return err
	}
	switch s {
	case "enabled":
		*nm = NetworkModeEnabled
	case "hidden":
		*nm = NetworkModeHidden
	case "disabled":
		*nm = NetworkModeDisabled
	default:
		return fmt.Errorf("unknown network mode %q", s)
	}
	return nil
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

	// SoftwareKeepAliveMisses sets how many consecutive keepalives from a remote node can be missed
	// before the connection is considered dead. The remote node advertises its keepalive period
	// during handshake; this value controls how patient we are waiting for them.
	// Timeout = RemotePeriod * Misses. Zero uses DefaultSoftwareKeepAliveMisses.
	// Acceptors and routes inherit this unless overridden.
	SoftwareKeepAliveMisses int

	// HandshakeTimeout is the node-wide bound on the entire handshake, used when a route
	// or acceptor does not set its own. Zero uses DefaultHandshakeTimeout.
	HandshakeTimeout time.Duration

	// HandshakeMaxMessageSize is the node-wide upper bound (in bytes) on a single handshake
	// message the node will accept, used when a route or acceptor does not set its own. The
	// Introduce message carries the type-registry cache exchange and grows with the number of
	// registered types, so this must exceed the largest expected Introduce. Zero uses
	// DefaultHandshakeMaxMessageSize.
	HandshakeMaxMessageSize int

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

	// FragmentSize sets the maximum fragment packet size in bytes.
	// Messages larger than this are split into fragments for transmission.
	// Zero uses DefaultFragmentSize (65000). Sender-local, no negotiation needed.
	FragmentSize int

	// FragmentTimeout sets the maximum time in seconds to wait for all fragments of a message.
	// Incomplete assemblies are cleaned up after this duration.
	// Zero uses DefaultFragmentTimeout (30s).
	FragmentTimeout int

	// MaxFragmentAssemblies limits concurrent unordered fragment assemblies per connection.
	// Protects against memory exhaustion from many simultaneous large messages.
	// Zero uses DefaultMaxFragmentAssemblies (1000).
	MaxFragmentAssemblies int
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
	// EnableSimultaneousConnect enables simultaneous connect detection and resolution
	EnableSimultaneousConnect bool
	// EnableClockSkew enables clock skew measurement between connected nodes.
	// Both nodes must have it enabled for measurements to work.
	EnableClockSkew bool
	// EnableTracing enables distributed tracing support.
	// Both nodes must have it enabled for trace context propagation.
	EnableTracing bool
	// EnableSoftwareKeepAlive enables application-level keepalive with the given period in seconds.
	// Zero disables keepalive. Max 255.
	EnableSoftwareKeepAlive int
	// EnableWrappedErrors enables *gen.Error structured wire format with
	// preserved wrap chain. Both nodes must enable it; otherwise *gen.Error
	// is sent as a flat .Error() string.
	EnableWrappedErrors bool
	// EnableSchemaEvolution length-prefixes encoded structs so a peer with a
	// different field count tolerates the difference (extra trailing fields are
	// skipped, missing ones left zero-valued). Both nodes must enable it. With it
	// on, an encoded struct is capped at 2^32-1 bytes (4GB).
	EnableSchemaEvolution bool
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
	if nf.EnableSimultaneousConnect == true {
		flags |= 128
	}
	if nf.EnableSoftwareKeepAlive > 0 {
		period := nf.EnableSoftwareKeepAlive
		if period > 255 {
			period = 255
		}
		flags |= uint64(period) << 8
	}
	if nf.EnableClockSkew == true {
		flags |= 1 << 16
	}
	if nf.EnableTracing == true {
		flags |= 1 << 17
	}
	if nf.EnableWrappedErrors == true {
		flags |= 1 << 18
	}
	if nf.EnableSchemaEvolution == true {
		flags |= 1 << 19
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
	nf.EnableSimultaneousConnect = (flags & 128) > 0
	nf.EnableSoftwareKeepAlive = int((flags >> 8) & 0xFF)
	nf.EnableClockSkew = (flags & (1 << 16)) > 0
	nf.EnableTracing = (flags & (1 << 17)) > 0
	nf.EnableWrappedErrors = (flags & (1 << 18)) > 0
	nf.EnableSchemaEvolution = (flags & (1 << 19)) > 0
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
	var flags uint64
	var buf [8]byte
	if npf.Enable == false {
		w.Write(buf[:])
		return nil
	}
	flags = 1 // npf.Enable = true
	if npf.EnableRemoteSpawn == true {
		flags |= 2
	}
	if npf.EnableRemoteApplicationStart == true {
		flags |= 4
	}
	if npf.EnableEncryption == true {
		flags |= 8
	}
	if npf.EnableImportantDelivery == true {
		flags |= 16
	}
	binary.BigEndian.PutUint64(buf[:], flags)
	w.Write(buf[:])
	return nil
}

func (npf *NetworkProxyFlags) UnmarshalEDF(buf []byte) error {
	if len(buf) < 8 {
		return fmt.Errorf("unable to unmarshal NetworkProxyFlags")
	}
	flags := binary.BigEndian.Uint64(buf)
	npf.Enable = (flags & 1) > 0
	if npf.Enable == false {
		return nil
	}
	npf.EnableRemoteSpawn = (flags & 2) > 0
	npf.EnableRemoteApplicationStart = (flags & 4) > 0
	npf.EnableEncryption = (flags & 8) > 0
	npf.EnableImportantDelivery = (flags & 16) > 0
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

	// PoolSize is the configured target number of TCP connections in the pool.
	// Multiple connections used for load balancing and ordering.
	PoolSize int

	// PoolLen is the current number of TCP connections in the pool.
	// Reaches PoolSize once the pool has fully filled.
	PoolLen int

	// PoolDSN lists the connection strings (host:port) for each pooled connection.
	PoolDSN []string

	// MaxMessageSize is the remote node's message size limit (in bytes).
	// Reported during handshake. Messages exceeding this are rejected.
	MaxMessageSize int

	// TLS indicates whether this connection uses TLS encryption.
	TLS bool

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

	// Reconnections is the total number of pool item reconnections.
	// A non-zero value indicates connection instability.
	Reconnections uint64

	// FragmentsSent is the total number of individual fragments sent.
	FragmentsSent uint64
	// FragmentMessagesSent is the total number of messages that were fragmented for sending.
	FragmentMessagesSent uint64
	// FragmentsReceived is the total number of individual fragments received.
	FragmentsReceived uint64
	// FragmentMessagesRecv is the total number of fragmented messages successfully reassembled.
	FragmentMessagesRecv uint64
	// FragmentTimeouts is the total number of fragment assemblies that timed out.
	FragmentTimeouts uint64

	// TracedSent is the total number of messages sent with tracing wrapper.
	TracedSent uint64
	// TracedReceived is the total number of messages received with tracing wrapper.
	TracedReceived uint64

	// CompressedSent is the total number of messages compressed on send.
	CompressedSent uint64
	// CompressedBytesSent is the total bytes after compression (wire size).
	CompressedBytesSent uint64
	// CompressedOrigBytesSent is the total bytes before compression (original size).
	CompressedOrigBytesSent uint64
	// DecompressedRecv is the total number of messages decompressed on receive.
	DecompressedRecv uint64
	// DecompressedBytesRecv is the total bytes before decompression (wire size).
	DecompressedBytesRecv uint64
	// DecompressedOrigRecv is the total bytes after decompression (original size).
	DecompressedOrigRecv uint64

	// ClockSkew is the estimated clock offset relative to the remote node (nanoseconds).
	// Positive value means remote clock is ahead. Zero if not yet measured.
	ClockSkew int64
}

// RemoteNodeShortInfo is the essential information about a connection to a
// remote node. The short form of RemoteNodeInfo, carried by NodeShortInfo.Peers.
type RemoteNodeShortInfo struct {
	// Node is the remote node name.
	Node Atom

	// ConnectionUptime is the connection age in seconds.
	ConnectionUptime int64

	// MessagesIn is the total number of messages received from this remote node.
	MessagesIn uint64

	// MessagesOut is the total number of messages sent to this remote node.
	MessagesOut uint64

	// BytesIn is the total bytes received from this remote node.
	BytesIn uint64

	// BytesOut is the total bytes sent to this remote node.
	BytesOut uint64

	// Reconnections is the total number of pool item reconnections.
	// A non-zero value indicates connection instability.
	Reconnections uint64

	// TLS indicates whether this connection uses TLS encryption.
	TLS bool
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
	// If empty, extracts hostname from node name (e.g., "node@hostname" -> "hostname").
	Host string

	// Port is the TCP port number for incoming connections.
	// Default: 11144 if not specified.
	Port uint16

	// PortRange defines how many ports to try starting from Port.
	// Default (0): tries all ports from Port to 65535.
	// PortRange=1: tries only the Port itself.
	// PortRange=N (N>1): tries N ports starting from Port.
	// Example: Port=11144, PortRange=10 tries 11144-11153.
	PortRange uint16

	// RouteHost specifies the public/external host address to advertise in routes.
	// Used when node is behind NAT or load balancer.
	// If empty, Route.Host is not set (other nodes extract host from node name).
	// Examples: "203.0.113.50" (public IP), "api.example.com" (DNS name)
	RouteHost string

	// RoutePort specifies the public/external port to advertise in routes.
	// Used when NAT port mapping differs from listen port.
	// If zero, uses the actual listening port.
	// Example: listen on :15000, NAT forwards 32000 -> 15000, set RoutePort=32000
	RoutePort uint16

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

	// MaxHandshakes limits the number of simultaneous in-flight handshakes
	// on this acceptor. When the limit is reached, new connections are
	// rejected immediately with a "busy" reason.
	// Zero (default) means unlimited.
	MaxHandshakes int

	// SoftwareKeepAliveMisses sets how many consecutive keepalives from a remote node can be missed
	// before the connection is considered dead. Zero inherits from NetworkOptions or uses default.
	SoftwareKeepAliveMisses int

	// HandshakeTimeout bounds the entire handshake for incoming connections on this acceptor.
	// Zero inherits from NetworkOptions.HandshakeTimeout, then DefaultHandshakeTimeout.
	HandshakeTimeout time.Duration

	// HandshakeMaxMessageSize bounds a single handshake message accepted on this acceptor.
	// Zero inherits from NetworkOptions.HandshakeMaxMessageSize, then DefaultHandshakeMaxMessageSize.
	HandshakeMaxMessageSize int
}

// Handshake defines handshake interface
type NetworkHandshake interface {
	NetworkFlags() NetworkFlags
	// Start initiates handshake process.
	// Cert value has CertManager that was used to create this connection
	Start(NodeHandshake, net.Conn, HandshakeOptions) (HandshakeResult, error)
	// Join is invoking within the NetworkDial to shortcut the handshake process
	Join(NodeHandshake, net.Conn, string, HandshakeOptions) ([]byte, error)
	// Negotiate is the acceptor step 1: reads the greeting and the peer introduce,
	// computes the ConnectionID and the proto-ready Custom; does not send the accept
	// yet. A pool-join is resolved entirely here.
	Negotiate(NodeHandshake, net.Conn, HandshakeOptions) (HandshakeResult, error)
	// Accept is the acceptor step 2: sends our introduce and accept, reads the peer
	// final accept, fills Tail. The node calls it after registering the connection
	// by ConnectionID.
	Accept(NodeHandshake, net.Conn, HandshakeOptions, HandshakeResult) (HandshakeResult, error)
	// Reject sends a rejection message to the connecting side and is used
	// when the acceptor is too busy to handle a new handshake.
	Reject(net.Conn, string) error
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

	// HandshakeMaxMessageSize is the upper bound (in bytes) on a single handshake message
	// this node will accept. Already resolved by the node layer (route/acceptor override,
	// then node-wide, then DefaultHandshakeMaxMessageSize); zero falls back to the default.
	HandshakeMaxMessageSize int

	// CheckPending returns true if this node has a pending outgoing
	// connect to the given peer. Used for simultaneous connect detection.
	CheckPending func(peer Atom) bool
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

	PoolSize int
	PoolDSN  []string

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

	// ConnectionsEstablished is the cumulative number of connections established.
	ConnectionsEstablished uint64

	// ConnectionsLost is the cumulative number of connections lost.
	ConnectionsLost uint64

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

	// SoftwareKeepAliveMisses sets how many consecutive keepalives from a remote node can be missed
	// before the connection is considered dead. Zero inherits from NetworkOptions or uses default.
	SoftwareKeepAliveMisses int

	// HandshakeTimeout bounds the entire handshake for this outgoing connection.
	// Zero inherits from NetworkOptions.HandshakeTimeout, then DefaultHandshakeTimeout.
	HandshakeTimeout time.Duration

	// HandshakeMaxMessageSize bounds a single handshake message accepted on this outgoing connection.
	// Zero inherits from NetworkOptions.HandshakeMaxMessageSize, then DefaultHandshakeMaxMessageSize.
	HandshakeMaxMessageSize int
}

type NetworkProxyRoute struct {
	Resolver Resolver
	Route    ProxyRoute

	Cookie string
	Flags  NetworkProxyFlags
	MaxHop int // DefaultProxyMaxHop == 8
}
