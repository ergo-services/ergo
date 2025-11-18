---
description: Understanding the network stack for distributed communication
---

# Network Stack

The network stack makes remote messaging work like local messaging. When you send to a process on another node, the framework discovers where that node is, establishes a connection if needed, encodes the message, sends it over TCP, and delivers it to the recipient's mailbox. From your perspective, it's just `Send(pid, message)` - whether the PID is local or remote.

This transparency requires three systems working together: service discovery to find nodes, connection management to establish reliable links, and message encoding to serialize data for transmission. Each system handles a specific problem, and together they create the illusion that remote communication is just local communication.

## The Big Picture

When you send a message to a remote process:

1. **Routing decision** - The framework examines the node portion of the PID. Local node? Direct mailbox delivery. Remote node? Continue to step 2.

2. **Connection lookup** - Check if a connection to that node already exists. If yes, use it. If no, continue to step 3.

3. **Discovery** - Query the registrar (or check static routes) to find where the remote node is listening: hostname, port, TLS requirements, protocol versions.

4. **Connection establishment** - Open TCP connections to the remote node, perform mutual authentication via handshake, negotiate capabilities, exchange caching dictionaries, create a connection pool.

5. **Message transmission** - Encode the message into bytes (EDF), optionally compress it, wrap it in a protocol frame (ENP), send it over one of the TCP connections in the pool.

6. **Remote delivery** - The receiving node reads the frame, decompresses if needed, decodes back to Go values, routes to the recipient's mailbox.

This entire pipeline is invisible to your code. You call `Send`, and the framework does the rest.

## Service Discovery

Before connecting to a remote node, the framework needs to know where that node is. Service discovery translates logical node names (`worker@example.com`) into connection parameters (IP, port, TLS, protocol versions).

The embedded registrar provides basic discovery:
- One node per host runs a registrar server (whoever started first)
- Other nodes connect as clients
- Same-host discovery is direct (no network)
- Cross-host discovery uses UDP queries
- Automatic failover if the server node dies

For production clusters, external registrars provide more features:
- **etcd** - Centralized discovery, application routing, configuration storage, HTTP polling for registration
- **Saturn** - Purpose-built for Ergo, immediate event propagation, efficient at scale

The embedded registrar works for development and small deployments. For larger clusters or dynamic topologies, use etcd or Saturn. The choice is transparent to your code - you specify the registrar at node startup, and everything else works identically.

For details, see [Service Discovery](service-discovering.md).

## Static Routes

Discovery is dynamic - nodes register themselves, and others query to find them. But sometimes you want explicit control. Maybe nodes have fixed addresses. Maybe you're behind a firewall that blocks discovery. Maybe you're connecting to external systems.

Static routes let you hardcode connection parameters:

```go
route := gen.NetworkRoute{
    Route: gen.Route{
        Host: "10.0.1.50",
        Port: 4370,
        TLS:  true,
    },
}
network.AddRoute("prod-db@example.com", route, 100)
```

Now when connecting to `prod-db@example.com`, the framework uses your route directly. No discovery query. No registrar involvement. You've taken control.

Static routes support pattern matching (`"prod-.*"`), multiple routes with failover weights, and hybrid approaches (use patterns for selection, resolvers for address lookup). You can configure per-route cookies, certificates, network flags, and atom mappings.

The framework checks static routes first, always. If a static route exists, discovery is bypassed. If static routes fail or don't exist, the framework falls back to discovery.

For details, see [Static Routes](static-routes.md).

## Connection Establishment

Once the framework knows where to connect (from discovery or static routes), it establishes a connection pool.

### Handshake

The handshake performs mutual authentication using challenge-response. Node A connects to node B:

1. A sends hello with random salt and digest (computed from salt + cookie)
2. B verifies digest - if cookies match, digest is correct
3. B sends its own challenge
4. A verifies B's response
5. Both sides authenticated

If TLS is enabled, certificate fingerprints are exchanged and verified too.

After authentication, nodes exchange introduction messages:
- Node names and version information
- Network flags (capabilities: remote spawn? important delivery? fragmentation?)
- Caching dictionaries (atoms, types, errors that will be used frequently)

The flags negotiation ensures nodes with different feature sets can work together. Features not supported by both sides are disabled for that connection.

The caching dictionaries enable efficiency. Instead of encoding `"mynode@localhost"` repeatedly (19 bytes), it gets a cache ID and subsequent uses encode as 2 bytes.

### Connection Pool

After handshake, the accepting node tells the dialing node to create a connection pool:
- Pool size (default 3 TCP connections)
- Acceptor addresses to connect to

The dialing node opens additional TCP connections using a shortened join handshake (skips full authentication since the first connection already authenticated). These connections join the pool, forming a single logical connection with multiple physical TCP links.

Multiple connections enable parallel message delivery. Each message goes to a connection based on the sender's identity (derived from sender PID). Messages from the same sender always use the same connection, preserving order. Messages from different senders use different connections, enabling parallelism.

The receiving side creates 4 receive queues per TCP connection. A 3-connection pool has 12 receive queues processing messages concurrently. This parallel processing improves throughput while preserving per-sender message ordering.

## Message Encoding and Transmission

Once a connection exists, messages flow through encoding and framing.

### EDF (Ergo Data Format)

EDF is a binary encoding specifically designed for the framework's communication patterns. It's type-aware - each value is prefixed with a type tag (e.g., `0x95` for int64, `0xaa` for PID, `0x9d` for slice). The decoder reads the tag and knows what follows.

Framework types like `gen.PID` and `gen.Ref` have optimized encodings. Structs are encoded field-by-field in declaration order (no field names on the wire). Custom types must be registered on both sides - registration happens during `init()`, and during handshake nodes exchange their type lists to agree on encoding.

Compression is automatic. If a message exceeds the compression threshold (default 1024 bytes), it's compressed using GZIP, ZLIB, or LZW. The protocol frame indicates compression, so the receiver decompresses before decoding.

For details on EDF - type tags, struct encoding, registration requirements, compression, caching - see [Network Transparency](network-transparency.md).

### ENP (Ergo Network Protocol)

ENP wraps encoded messages in frames for transmission. Each frame has an 8-byte header with magic byte, protocol version, frame length, order byte, and message type. The frame body contains sender/recipient identifiers and the EDF-encoded payload.

The order byte preserves message ordering per sender. Messages from the same sender have the same order value and route to the same receive queue, guaranteeing sequential processing. Messages from different senders have different order values and route to different queues, enabling parallel processing.

For details on protocol framing, order bytes, receive queue distribution, and the exact byte layout, see [Network Transparency](network-transparency.md).

## Network Transparency in Practice

Network transparency means remote operations look like local operations. You send to a PID without checking if it's local or remote. You establish links and monitors the same way regardless of location. The framework handles discovery, encoding, and transmission automatically.

But transparency has limits:
- **Latency** - Remote sends take milliseconds vs microseconds for local
- **Bandwidth** - Network links have finite capacity, local operations don't
- **Failures** - Networks fail in ways local memory doesn't (packets lost, connections drop, nodes unreachable)
- **Partial failures** - Some nodes work while others fail (local systems fail entirely or work entirely)

The framework makes distributed programming feel local, but you still need to design for network realities: use timeouts, handle connection failures, prefer async over sync, batch messages, keep payloads small.

For deep understanding of how transparency works - EDF encoding, struct serialization, type registration, important delivery, failure semantics - see [Network Transparency](network-transparency.md).

## Network Configuration

Configure the network stack in `gen.NodeOptions.Network`:

```go
node, err := ergo.StartNode("myapp@localhost", gen.NodeOptions{
    Network: gen.NetworkOptions{
        Mode:           gen.NetworkModeEnabled,
        Cookie:         "secret-cluster-cookie",
        MaxMessageSize: 10 * 1024 * 1024, // 10MB
        Flags: gen.NetworkFlags{
            Enable:                       true,
            EnableRemoteSpawn:            true,
            EnableRemoteApplicationStart: true,
            EnableImportantDelivery:      true,
        },
        Acceptors: []gen.AcceptorOptions{
            {
                Port:       15000,
                PortRange:  10,
                BufferSize: 64 * 1024,
            },
        },
    },
})
```

**Mode** - `NetworkModeEnabled` enables full networking with acceptors. `NetworkModeHidden` allows outgoing connections only (no acceptors). `NetworkModeDisabled` disables networking entirely.

**Cookie** - Shared secret for authentication. All nodes must use the same cookie to communicate. Set explicitly for distributed deployments.

**MaxMessageSize** - Maximum incoming message size. Protects against memory exhaustion. Default unlimited (fine for trusted clusters).

**Flags** - Control capabilities. Remote nodes learn your flags during handshake and can only use features you've enabled. `EnableRemoteSpawn` allows spawning (with explicit permission per process). `EnableImportantDelivery` enables delivery confirmation.

**Acceptors** - Define listeners for incoming connections. Multiple acceptors on different ports are supported. Each can have its own cookie, TLS, and protocol.

## Custom Network Stacks

The framework provides three extension points:

**gen.NetworkHandshake** - Control connection establishment and authentication. Implement this to change how nodes authenticate or how connection pools are created.

**gen.NetworkProto** - Control message encoding and transmission. The Erlang distribution protocol is implemented as a custom proto, allowing Ergo nodes to join Erlang clusters.

**gen.Connection** - The actual connection handling. Implement this for custom framing, routing, or error handling.

You can register multiple handshakes and protos, allowing one node to support multiple protocol stacks simultaneously:

```go
node, err := ergo.StartNode("myapp@localhost", gen.NodeOptions{
    Network: gen.NetworkOptions{
        Handshake: customHandshake,
        Proto:     customProto,
        Acceptors: []gen.AcceptorOptions{
            {Port: 15000, Proto: ergoProto},   // Ergo protocol
            {Port: 16000, Proto: erlangProto}, // Erlang protocol
        },
    },
})
```

This enables migration scenarios (gradually migrate from Erlang to Ergo) and integration scenarios (connect to systems using different protocols).

## Remote Operations

Once connections exist, you can spawn processes and start applications on remote nodes:

```go
remote, err := node.Network().GetNode("worker@otherhost")
if err != nil {
    return err
}

pid, err := remote.Spawn("worker_name", gen.ProcessOptions{})
```

Remote spawning requires the remote node to explicitly enable it:

```go
// On the remote node
node.Network().EnableSpawn("worker_name", createWorker)
```

Without explicit permission, remote spawn requests fail. This prevents arbitrary code execution.

The same pattern applies to starting applications:

```go
remote.ApplicationStart("myapp", gen.ApplicationOptions{})
```

Requires:

```go
node.Network().EnableApplicationStart("myapp")
```

This security model ensures you control exactly what remote nodes can do on your node.

## Where to Go Next

This chapter provided an overview of how the network stack operates. For deeper understanding:

- **[Service Discovery](service-discovering.md)** - How nodes find each other, application routing, configuration management, embedded vs external registrars
- **[Network Transparency](network-transparency.md)** - How messages are encoded, EDF details, protocol framing, compression, caching, important delivery
- **[Static Routes](static-routes.md)** - Explicit routing configuration, pattern matching, failover, proxy routes

Each of these chapters dives deep into its specific topic, giving you the details needed for production deployments.
