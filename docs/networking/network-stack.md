# Network Stack

The Ergo Framework's network stack implements [network transparency](network-transparency.md) using three components:&#x20;

&#x20;`Registrar` - Manages node registration, including applications and additional parameters to help nodes find each other and establish connections. See the [Service Discovering](service-discovering.md) section for more details.&#x20;

&#x20;`EDF` (Ergo Data Format) - A data format for network communication that automatically encodes and decodes messages when sent to remote nodes.&#x20;

&#x20;`ENP` (Ergo Network Protocol) - A protocol for network interaction, facilitating asynchronous message exchange, synchronous requests, and remote process/application initiation between nodes.

### Network Options

Network stack parameters can be specified at node startup in `gen.NodeOptions.Network`. This field, of type `gen.NetworkOptions`, allows you to configure the following:

* `Mode`: default is `gen.NetworkModeEnabled`. Use `gen.NetworkModeHidden` to disable incoming connections, while still allowing outgoing ones. Use `gen.NetworkModeDisabled` to completely disable networking.
* `Cookie`: sets a secret password for access control, used for both incoming and outgoing connections.
* `MaxMessageSize`: defines the maximum size of network messages.
* `Flags`: specify features available to remote nodes, like process or application launching.
* `Registrar`: by default, the node uses the built-in implementation (`ergo.services/ergo/net/registrar`). For [central registrar Saturn](../tools/saturn.md) or for [Erlang](../extra-library/network-protocols/erlang.md) clusters, use respective libraries ([Saturn](../extra-library/registrars/saturn-client.md) or [EPMD](../extra-library/network-protocols/erlang.md#epmd)).
* `Handshake`: connection setup starts with remote node authentication (cookie verification) and protocol parameter exchange. Default uses the built-in implementation (`ergo.services/ergo/net/handshake`).
* `Proto`: default protocol is Ergo Framework's ENP (`ergo.services/ergo/net/proto`), but other protocols like the [Erlang stack's DIST](../extra-library/network-protocols/erlang.md#dist-protocol) can be used.
* `Acceptors`: defines a set of acceptors for the node (`[]gen.AcceptorOptions`). Each acceptor can configure port, TLS encryption, message size limits, and a custom cookie different from `gen.NetworkOptions.Cookie`. You can also configure different `Registrar/Handshake/Proto` sets for each acceptor, enabling simultaneous use of multiple network stacks, such as Ergo Framework and Erlang.

### Establishing a connection with a remote node

To connect to a remote node, simply send a message to a process running on that node. The node will automatically attempt to establish a network connection before sending the message.&#x20;

You can also establish a connection explicitly by calling the `GetNode` method from the `gen.Network` interface. For example:

```go
func main() {
    ...
    node, err := ergo.StartNode("node1@localhost", gen.NodeOptions{})
    ...
    remote, err := node.Network().GetNode("node2@localhost")
    ...
}
```

The `GetNode` method returns the `gen.RemoteNode` interface, which provides information about the remote node. Additionally, it allows you to launch a process or application on that remote node:

```go
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

```

For more details about remote process and application spawning, refer to the [Remote Spawn Process](remote-spawn-process.md) and [Remote Start Application](remote-start-application.md) sections.

You can also use the `Node` method from the `gen.Network` interface. It returns a `gen.RemoteNode` if a connection to the remote node exists; otherwise, it returns `gen.ErrNoConnection`.

To establish a connection with a remote node using specific parameters, use the `GetNodeWithRoute` method from the `gen.Network` interface. This method allows for more customized network connection settings.&#x20;

### Network Stack Interfaces

This section explains the internal mechanisms by which a node interacts with the network stack. Understanding these can help you implement a custom network stack or components, such as a modified version of `gen.NetworkHandshake` for customized authentication.

For incoming connections, the node starts acceptors with specified parameters. For outgoing connections, it independently creates TCP connections using parameters from the registrar (see [Service Discovering](service-discovering.md)). After establishing the TCP connection, the node leverages several interfaces to operate the network stack:

#### gen.NetworkHandshake

At the first stage, the node works with the `gen.NetworkHandshake` interface. If a TCP connection was created by an acceptor, the `Accept` method is called, initiating the remote node's authentication and the exchange of parameters for the network protocol. If the TCP connection was initiated by the node itself, the `Start` method is used, which handles the authentication process and the exchange of network protocol parameters.&#x20;

The `Join` method is used to combine multiple TCP connections. In the ENP protocol, by default, 3 TCP connections are created and merged into one virtual connection between nodes, which improves network protocol performance. You can set the size of the TCP connection pool using the `PoolSize` parameter in `handshake.Options` (`ergo.services/ergo/net/handshake`). The Erlang network stack's DIST protocol does not support this feature.

```go
type NetworkHandshake interface {
	NetworkFlags() NetworkFlags
	// Start initiates handshake process.
	Start(NodeHandshake, net.Conn, HandshakeOptions) (HandshakeResult, error)
	// Join attempts to join new TCP-connection to the existing connection 
	// to remote node
	Join(NodeHandshake, net.Conn, id string, HandshakeOptions) ([]byte, error)
	// Accept initiate handshake process for the connection created by remote node.
	Accept(NodeHandshake, net.Conn, HandshakeOptions) (HandshakeResult, error)
	// Version
	Version() Version
}
```

#### gen.NetworkProto

After successfully completing the handshake procedure, the node transfers control of the TCP connection to the network protocol using the `gen.NetworkProto` interface. This occurs in two stages: first, the node calls the `NewConnection`method, which returns the `gen.Connection` interface for the created connection. The node then registers this connection in its internal routing mechanism with the remote node's name. If a connection with the same name already exists, the node automatically closes the TCP connection, as only one connection can exist between two nodes.&#x20;

Upon successful registration, the node calls the `Serve` method. The completion of this method indicates the termination of the connection with the remote node, effectively closing the connection.&#x20;

```go
type NetworkProto interface {
	// NewConnection
	NewConnection(core Core, result HandshakeResult, log Log) (Connection, error)
	// Serve connection. Argument dial is the closure to create TCP connection with invoking
	// NetworkHandshake.Join inside to shortcut the handshake process
	Serve(conn Connection, dial NetworkDial) error
	// Version
	Version() Version
}

```

#### gen.Connection

The `gen.Connection` interface is used by the node for routing messages and requests to the remote node with which a connection has been established. If you are developing your own protocol, you will need to implement all the methods of this interface to handle communication between nodes.&#x20;

```go
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

```

