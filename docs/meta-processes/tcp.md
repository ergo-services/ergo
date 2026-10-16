# TCP

A TCP server meta process accepts sockets and creates a connection meta process for each. Each connection reads bytes into `MessageTCP` messages and writes the bytes sent back to its alias.

We will build an echo server, route connections to workers and then add framing and buffer reuse. The connection reads a byte stream: application message boundaries need a framing rule.

## TCP Server: Accepting Connections

Create a TCP server with `meta.CreateTCPServer`:

```go
type EchoServer struct {
    act.Actor
}

func (e *EchoServer) Init(args ...any) error {
    options := meta.TCPServerOptions{
        Host: "0.0.0.0",  // Listen on all interfaces
        Port: 8080,
    }
    
    server, err := meta.CreateTCPServer(options)
    if err != nil {
        return fmt.Errorf("failed to create TCP server: %w", err)
    }
    
    // Start the server meta-process
    serverID, err := e.SpawnMeta(server, gen.MetaOptions{})
    if err != nil {
        // Failed to spawn - close the listening socket
        server.Terminate(err)
        return fmt.Errorf("failed to spawn TCP server: %w", err)
    }
    
    e.Log().Info("TCP server listening on %s:%d (id: %s)", 
        options.Host, options.Port, serverID)
    return nil
}
```

The server opens a TCP socket and enters an accept loop. When a connection arrives, the server spawns a new `TCPConnection` meta-process to handle it. Each connection runs in its own meta-process, isolated from other connections.

If `SpawnMeta` fails, you must call `server.Terminate(err)` to close the listening socket. Without this, the port remains bound and unusable until the process exits.

The server runs forever, accepting connections and spawning handlers. When the parent actor terminates, the server terminates too (cascading termination), closing the listening socket and stopping all connection handlers.

## TCP Connection: Handling I/O

When the server accepts a connection, it automatically spawns a `TCPConnection` meta-process. This meta-process reads data from the socket and sends it to your actor. To write data, you send messages to the connection's meta-process.

```go
func (e *EchoServer) HandleMessage(from gen.PID, message any) error {
    switch m := message.(type) {
    case meta.MessageTCPConnect:
        // New connection established
        e.Log().Info("client connected: %s -> %s (id: %s)", 
            m.RemoteAddr, m.LocalAddr, m.ID)
        
        // Send welcome message
        e.Send(m.ID, meta.MessageTCP{
            Data: []byte("Welcome to echo server!\n"),
        })
        
    case meta.MessageTCP:
        // Received data from client
        e.Log().Info("received %d bytes from %s", len(m.Data), m.ID)
        
        // Echo it back
        e.Send(m.ID, meta.MessageTCP{
            Data: m.Data,
        })
        
    case meta.MessageTCPDisconnect:
        // Connection closed
        e.Log().Info("client disconnected: %s", m.ID)
    }
    return nil
}
```

**MessageTCPConnect** arrives when the connection is established. It contains the connection's meta-process ID (`m.ID`), remote address, and local address. Save the ID if you need to track connections or send data later.

Both addresses are `netip.AddrPort` values from the standard `net/netip` package: comparable, usable as map keys, and cheap to copy. An IPv4 client is always reported as a 4-byte address, even when the server listens on the dual-stack wildcard.

**MessageTCP** arrives when data is read from the socket. `m.Data` contains the bytes read (up to `ReadBufferSize` at a time). To send data, send a `MessageTCP` back to the connection's ID. The meta-process writes it to the socket.

**MessageTCPDisconnect** arrives when the connection closes (client disconnected, network error, or you terminated the connection). After this, the connection meta-process is dead - sending to its ID returns an error.

All three messages are registered for the network, so the actor that serves a connection does not have to run on the node that accepted it. A process on another node writes to the socket by sending `MessageTCP` to the connection ID:

```go
// anywhere in the cluster, given the connection ID
process.Send(connID, meta.MessageTCP{ID: connID, Data: response})
```

If the connection meta-process cannot send messages to your actor (actor crashed, mailbox full), it terminates the connection and stops. This ensures failed actors don't leak connections.

## Routing to Workers

By default, all connections send messages to the parent actor - the one that spawned the server. For a server handling many connections, this creates a bottleneck. All connections compete for the parent's mailbox, and messages are processed sequentially.

Use `ProcessPool` to distribute connections across multiple workers:

```go
type TCPDispatcher struct {
    act.Actor
}

func (d *TCPDispatcher) Init(args ...any) error {
    // Start worker pool
    for i := 0; i < 10; i++ {
        workerName := gen.Atom(fmt.Sprintf("tcp_worker_%d", i))
        _, err := d.SpawnRegister(workerName, createWorker, gen.ProcessOptions{})
        if err != nil {
            return err
        }
    }
    
    // Configure server with worker pool
    options := meta.TCPServerOptions{
        Port: 8080,
        ProcessPool: []gen.Atom{
            "tcp_worker_0",
            "tcp_worker_1",
            "tcp_worker_2",
            "tcp_worker_3",
            "tcp_worker_4",
            "tcp_worker_5",
            "tcp_worker_6",
            "tcp_worker_7",
            "tcp_worker_8",
            "tcp_worker_9",
        },
    }
    
    server, err := meta.CreateTCPServer(options)
    if err != nil {
        return err
    }
    
    _, err = d.SpawnMeta(server, gen.MetaOptions{})
    if err != nil {
        server.Terminate(err)
        return err
    }
    
    return nil
}
```

The server distributes connections round-robin across the pool. Connection 1 goes to `tcp_worker_0`, connection 2 goes to `tcp_worker_1`, and so on. After `tcp_worker_9`, it wraps back to `tcp_worker_0`.

Each worker handles its connections independently. If a worker crashes, its connections terminate (they can't send messages anymore). The supervisor restarts the worker, which begins handling new connections. The distribution is stateless - the server doesn't track which worker handles which connection.

**Do not use `act.Pool` in ProcessPool**. `act.Pool` forwards messages to any available worker, breaking the connection-to-worker binding. If connection A sends message 1 to worker X and message 2 to worker Y, the protocol state becomes corrupted. Use a list of individual process names instead.

Workers are typically actors that maintain per-connection state:

```go
type TCPWorker struct {
    act.Actor
    connections map[gen.Alias]*ConnectionState
}

type ConnectionState struct {
    remoteAddr netip.AddrPort
    buffer     []byte
    // ... protocol state
}

func (w *TCPWorker) HandleMessage(from gen.PID, message any) error {
    switch m := message.(type) {
    case meta.MessageTCPConnect:
        w.connections[m.ID] = &ConnectionState{
            remoteAddr: m.RemoteAddr,
        }
        
    case meta.MessageTCP:
        state := w.connections[m.ID]
        w.processData(m.ID, state, m.Data)
        
    case meta.MessageTCPDisconnect:
        delete(w.connections, m.ID)
    }
    return nil
}
```

## Client Connections

To initiate outgoing TCP connections, use `meta.CreateTCPConnection`:

```go
type HTTPClient struct {
    act.Actor
    connID gen.Alias
}

func (c *HTTPClient) Init(args ...any) error {
    options := meta.TCPConnectionOptions{
        Host: "example.com",
        Port: 80,
    }
    
    connection, err := meta.CreateTCPConnection(options)
    if err != nil {
        return fmt.Errorf("failed to connect: %w", err)
    }
    
    connID, err := c.SpawnMeta(connection, gen.MetaOptions{})
    if err != nil {
        connection.Terminate(err)
        return fmt.Errorf("failed to spawn connection: %w", err)
    }
    
    c.connID = connID
    return nil
}

func (c *HTTPClient) HandleMessage(from gen.PID, message any) error {
    switch m := message.(type) {
    case meta.MessageTCPConnect:
        // Connection established, send HTTP request
        request := "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"
        c.Send(m.ID, meta.MessageTCP{
            Data: []byte(request),
        })
        
    case meta.MessageTCP:
        // Received HTTP response
        c.Log().Info("response: %s", string(m.Data))
        
    case meta.MessageTCPDisconnect:
        // Server closed connection
        c.Log().Info("connection closed by server")
    }
    return nil
}
```

`CreateTCPConnection` connects to the remote host immediately. If the connection fails (host unreachable, connection refused), it returns an error. If successful, it returns a meta-process behavior ready to spawn.

The spawned meta-process sends `MessageTCPConnect` when ready, then streams received data as `MessageTCP` messages. To send data, send `MessageTCP` to the connection's ID.

Client connections use the same `TCPConnection` meta-process as server-side connections. The only difference is how they're created: `CreateTCPConnection` initiates a connection, while the server spawns connections automatically on accept.

## Chunking: Message Framing

Raw TCP is a byte stream, not a message stream. If you send two 100-byte messages, they might arrive as one 200-byte read, or three reads (150 bytes, 40 bytes, 10 bytes). You must frame messages to detect boundaries.

Enable chunking for automatic framing:

**Fixed-length messages:**

```go
options := meta.TCPServerOptions{
    Port: 8080,
    ReadChunk: meta.ChunkOptions{
        Enable:      true,
        FixedLength: 256,  // Every message is exactly 256 bytes
    },
}
```

Every `MessageTCP` contains exactly 256 bytes. The meta-process buffers reads until 256 bytes accumulate, then sends them. If a socket read returns 512 bytes, you receive two `MessageTCP` messages.

**Header-based messages:**

```go
options := meta.TCPServerOptions{
    Port: 8080,
    ReadBufferSize: 8192,
    ReadChunk: meta.ChunkOptions{
        Enable: true,
        
        // Protocol: [4-byte length][payload]
        HeaderSize:                 4,
        HeaderLengthPosition:       0,
        HeaderLengthSize:           4,
        HeaderLengthIncludesHeader: false,  // Length is payload only
        
        MaxLength: 1048576,  // Max 1MB per message
    },
}
```

The meta-process reads the 4-byte header, extracts the length as a big-endian integer, waits for the full payload, then sends the complete message (header + payload) as one `MessageTCP`.

Protocol example:

```
Message 1: [0x00 0x00 0x00 0x0A] [10 bytes payload]
Message 2: [0x00 0x00 0x01 0x00] [256 bytes payload]
```

You receive:
- First `MessageTCP`: 14 bytes (4 + 10)
- Second `MessageTCP`: 260 bytes (4 + 256)

If both messages arrive in one socket read (274 bytes total), the meta-process splits them automatically. If the header arrives first and the payload arrives later (slow connection), the meta-process waits for the complete message.

`MaxLength` protects against malformed or malicious messages. If the header claims a message is 4GB, the meta-process terminates with `gen.ErrTooLarge` instead of allocating 4GB of memory.

`HeaderLengthSize` can be 1, 2, or 4 bytes (big-endian). `HeaderLengthPosition` specifies the offset within the header. Example for a protocol with type + flags + length:

```go
// Protocol: [type][flags][length-MSB][length-LSB][payload]
ReadChunk: meta.ChunkOptions{
    Enable:               true,
    HeaderSize:           4,
    HeaderLengthPosition: 2,  // Length starts at byte 2
    HeaderLengthSize:     2,  // 2-byte length
}
```

Without chunking, you receive raw bytes as the meta-process reads them. You must buffer and frame messages yourself - typically by accumulating data in your actor's state and detecting message boundaries manually.

## Buffer Management

The meta-process allocates buffers for reading socket data. By default, each read allocates a new buffer, which becomes garbage after you process it. For high-throughput servers, this causes GC pressure.

Use a buffer pool:

```go
bufferPool := &sync.Pool{
    New: func() any {
        return make([]byte, 8192)
    },
}

options := meta.TCPServerOptions{
    Port:           8080,
    ReadBufferSize: 8192,
    ReadBufferPool: bufferPool,
}
```

The meta-process gets buffers from the pool when reading. When you receive `MessageTCP`, the `Data` field is a buffer from the pool. **Return it to the pool after processing**:

```go
func (w *Worker) HandleMessage(from gen.PID, message any) error {
    switch m := message.(type) {
    case meta.MessageTCP:
        // Process data
        result := w.processPacket(m.Data)
        
        // Send response
        w.Send(m.ID, meta.MessageTCP{Data: result})
        
        // Return read buffer to pool
        bufferPool.Put(m.Data)
    }
    return nil
}
```

When you send `MessageTCP` to write data, the meta-process automatically returns the buffer to the pool after writing (if a pool is configured). Don't use the buffer after sending.

If you need to store data beyond the current message, copy it:

```go
case meta.MessageTCP:
    state := w.connections[m.ID]
    
    // Store in connection state - must copy
    state.buffer = append(state.buffer, m.Data...)
    
    // Return original buffer
    bufferPool.Put(m.Data)
```

Buffer pools are essential for servers handling thousands of connections or high throughput. For low-volume clients, the GC overhead is negligible - skip the pool for simplicity.

## Write Keepalive

Some protocols require periodic writes to keep connections alive. If no data is sent for a timeout period, the peer disconnects. You could send keepalive messages with timers, but this is tedious and error-prone.

Enable automatic keepalive:

```go
options := meta.TCPServerOptions{
    Port: 8080,
    WriteBufferKeepAlive:       []byte{0x00},  // Send null byte
    WriteBufferKeepAlivePeriod: 30 * time.Second,
}
```

The meta-process wraps the socket with a keepalive writer. If nothing is written for 30 seconds, it automatically sends a null byte. The peer receives it as normal data. Design your protocol to ignore keepalive messages.

Keepalive bytes can be anything: a ping message, a heartbeat packet, or a protocol-specific keepalive. The peer sees them as regular socket data.

This is application-level keepalive (layer 7), not TCP keepalive (layer 4). Both can be used simultaneously.

## TCP Keepalive (OS-Level)

TCP has built-in keepalive at the protocol level. Enable it with `KeepAlivePeriod`:

```go
options := meta.TCPServerOptions{
    Port: 8080,
    Advanced: meta.TCPAdvancedOptions{
        KeepAlivePeriod: 60 * time.Second,
    },
}
```

The OS sends TCP keepalive probes every 60 seconds when the connection is idle. If the peer doesn't respond, the connection is closed. This detects dead connections (network partition, crashed peer) without application involvement.

The value is handed straight to `net.ListenConfig.KeepAlive` (and to `net.Dialer.KeepAlive` for a client connection), so it carries Go's meaning rather than an inverted one. Leaving `KeepAlivePeriod` unset is the **zero** value, and zero means keepalive is **enabled** with the operating system's own period - typically two hours on Linux, varying by platform. A positive duration sets the probe period explicitly. To turn keepalive off, set it **negative**.

TCP keepalive (OS-level) and write buffer keepalive (application-level) serve different purposes:
- **TCP keepalive**: Detects dead connections
- **Write keepalive**: Satisfies application protocols that require periodic data

Most servers need TCP keepalive to clean up dead connections. Some protocols also need write keepalive to satisfy their requirements.

## TLS Encryption

Enable TLS with a certificate manager:

```go
certManager := createCertManager()  // Your certificate manager

options := meta.TCPServerOptions{
    Port:        8443,
    CertManager: certManager,
}
```

The server wraps accepted connections with TLS. The certificate manager provides certificates dynamically (for SNI, certificate rotation, etc.). See [CertManager](../basics/certmanager.md) for details.

For client connections:

```go
options := meta.TCPConnectionOptions{
    Host:        "example.com",
    Port:        443,
    CertManager: certManager,
}
```

The client establishes a TLS connection during `CreateTCPConnection`. By default, the client verifies the server's certificate. To skip verification (testing only):

```go
options := meta.TCPConnectionOptions{
    Host:               "self-signed-server.local",
    Port:               443,
    CertManager:        certManager,
    InsecureSkipVerify: true,  // Don't verify server certificate
}
```

**Never use `InsecureSkipVerify` in production**. It disables certificate validation, making you vulnerable to man-in-the-middle attacks.

With TLS enabled, data is encrypted automatically. Your actor sends and receives plaintext `MessageTCP` - the meta-process handles encryption/decryption transparently.

## Process Routing

For both server and client connections, you can route messages to a specific process:

```go
// Server: all connections send messages to "connection_manager"
serverOpts := meta.TCPServerOptions{
    Port:        8080,
    ProcessPool: []gen.Atom{"connection_manager"},
}

// Client: this connection sends messages to "http_handler"
clientOpts := meta.TCPConnectionOptions{
    Host:    "example.com",
    Port:    80,
    Process: "http_handler",
}
```

If `Process` is not set (client) or `ProcessPool` is empty (server), messages go to the parent actor.

For servers, `ProcessPool` enables load distribution. For clients, `Process` enables separation of concerns - the actor that initiates connections doesn't need to handle the protocol.

### Handler on another node

`Process` and `ProcessPool` name processes on the node that owns the socket. The
messages themselves are not confined to it: `MessageTCPConnect`, `MessageTCP` and
`MessageTCPDisconnect` are registered for the network, so a process anywhere in the
cluster can write to a connection by its ID, and the bytes leave from the node that
holds the socket.

What you cannot do is point a connection at a remote process directly, and that is
deliberate. The connection meta-process delivers every read from the same loop that
reads the socket. Sending to another node is a heavier operation than a local
delivery and depends on the state of the link between the two nodes, so a socket
pointed at a remote process would read at the speed of that link.

Put an ordinary actor in front instead. It lives on the node that owns the socket,
receives the messages at local speed, and carries them across:

```go
type forwarder struct {
    act.Actor
    worker gen.ProcessID
}

func (f *forwarder) Init(args ...any) error {
    f.worker = gen.ProcessID{Name: "worker", Node: "workers@127.0.0.1"}
    return nil
}

func (f *forwarder) HandleMessage(from gen.PID, message any) error {
    return f.Send(f.worker, message)
}
```

```go
options := meta.TCPServerOptions{
    Port:        8080,
    ProcessPool: []gen.Atom{"fwd_0", "fwd_1", "fwd_2"},
}
```

The distribution is unchanged: each connection is bound to one forwarder for its
whole lifetime, and a slow hop to the other node fills that forwarder's mailbox
instead of stalling the sockets.

The worker on the other node replies the usual way, by sending `MessageTCP` to the
connection's ID. It does not need a route back through the forwarder.

Two things become yours to decide with this layout.

**Buffers.** With `ReadBufferPool` configured, the buffer in `MessageTCP.Data` has to
come back to the pool, and a process on another node can never do that. Return it in
the forwarder, after the send to the other node has returned:

```go
func (f *forwarder) HandleMessage(from gen.PID, message any) error {
    err := f.Send(f.worker, message)
    if m, ok := message.(meta.MessageTCP); ok {
        bufferPool.Put(m.Data)
    }
    return err
}
```

**An unreachable node.** `Send` returns an error when the other node is down or was
never connected. The forwarder decides what that means: drop the message, close the
connection with `SendExitMeta`, or keep a bounded backlog and retry. Until it decides
otherwise the connection stays open and the client keeps sending.

## Inspection

TCP meta-processes support inspection for debugging:

```go
// Inspect server
serverInfo, _ := process.InspectMeta(serverID)
// Returns: map[string]string{"listener": "0.0.0.0:8080"}

// Inspect connection
connInfo, _ := process.InspectMeta(connID)
// Returns: map[string]string{
//     "local":     "192.168.1.10:8080",
//     "remote":    "192.168.1.20:54321",
//     "process":   "tcp_worker_3",
//     "bytes in":  "1048576",
//     "bytes out": "524288",
// }
```

A meta process is inspected by its alias through `InspectMeta`, not by a `Call`. The request travels the meta's system queue and is answered by its `HandleInspect`, which a `Call` never reaches. `gen.Node` carries the same method for callers that are not processes.

Use this for monitoring, debugging, or displaying connection status in management interfaces.

## Patterns and Pitfalls

**Pattern: Connection registry**

```go
type ConnectionManager struct {
    act.Actor
    connections map[gen.Alias]*ConnectionInfo
}

type ConnectionInfo struct {
    remoteAddr netip.AddrPort
    startTime  time.Time
}

func (m *ConnectionManager) HandleMessage(from gen.PID, message any) error {
    switch msg := message.(type) {
    case meta.MessageTCPConnect:
        m.connections[msg.ID] = &ConnectionInfo{
            remoteAddr: msg.RemoteAddr,
            startTime:  time.Now(),
        }
        m.Log().Info("connection #%d: %s", len(m.connections), msg.RemoteAddr)
        
    case meta.MessageTCPDisconnect:
        info := m.connections[msg.ID]
        duration := time.Since(info.startTime)
        m.Log().Info("connection closed: %s (duration: %s)", 
            info.remoteAddr, duration)
        delete(m.connections, msg.ID)
    }
    return nil
}
```

Track all active connections. Useful for monitoring, rate limiting, or forced disconnection.

**Pattern: Protocol state machine**

```go
type ProtocolHandler struct {
    act.Actor
    connections map[gen.Alias]*ProtocolState
}

type ProtocolState struct {
    state  int  // Current state in protocol state machine
    buffer []byte
}

func (h *ProtocolHandler) HandleMessage(from gen.PID, message any) error {
    switch m := message.(type) {
    case meta.MessageTCPConnect:
        h.connections[m.ID] = &ProtocolState{state: STATE_INITIAL}
        
    case meta.MessageTCP:
        state := h.connections[m.ID]
        state.buffer = append(state.buffer, m.Data...)
        
        // Process buffered data according to current state
        for {
            complete, nextState := h.processState(m.ID, state)
            if !complete {
                break
            }
            state.state = nextState
        }
        
        bufferPool.Put(m.Data)
    }
    return nil
}
```

Maintain per-connection protocol state for complex protocols with multiple stages (handshake, authentication, data transfer).

**Pattern: Broadcast to all connections**

```go
func (m *ConnectionManager) broadcastMessage(data []byte) {
    for connID := range m.connections {
        m.Send(connID, meta.MessageTCP{Data: data})
    }
}
```

Send the same data to all active connections. Useful for chat servers, pub/sub systems, or monitoring dashboards.

**Pitfall: Not handling MessageTCPDisconnect**

```go
// WRONG: Connection state leaked
func (w *Worker) HandleMessage(from gen.PID, message any) error {
    switch m := message.(type) {
    case meta.MessageTCPConnect:
        w.connections[m.ID] = &State{}
        
    case meta.MessageTCP:
        w.connections[m.ID].process(m.Data)
        
    // No MessageTCPDisconnect handler!
    }
    return nil
}
```

After disconnect, the connection state remains in memory forever. Always clean up on disconnect.

**Pitfall: act.Pool in ProcessPool**

```go
// WRONG: Protocol state corrupted
options := meta.TCPServerOptions{
    ProcessPool: []gen.Atom{"worker_pool"},  // Don't use act.Pool!
}
```

If `worker_pool` is an `act.Pool`, messages from one connection are distributed across multiple workers. Connection A's messages might go to worker 1, then worker 2, then worker 1 again. Protocol state is split across workers, causing corruption.

Use individual process names, not pools.

**Pitfall: Blocking in message handler**

```go
// WRONG: Blocks actor, stalls other connections
func (w *Worker) HandleMessage(from gen.PID, message any) error {
    switch m := message.(type) {
    case meta.MessageTCP:
        // Slow database query
        result := w.db.Query("SELECT * FROM large_table")
        w.Send(m.ID, meta.MessageTCP{Data: result})
    }
    return nil
}
```

If the worker handles multiple connections, one slow operation blocks all of them. The worker can't process messages from other connections while blocked.

Solution: Spawn a goroutine for slow operations, or use a worker pool (one worker per connection).

**Pitfall: Forgetting to return buffers**

```go
// WRONG: Buffer leaked
func (w *Worker) HandleMessage(from gen.PID, message any) error {
    switch m := message.(type) {
    case meta.MessageTCP:
        // Store data, never return buffer
        w.dataQueue = append(w.dataQueue, m.Data)
    }
    return nil
}

// CORRECT: Copy if storing
func (w *Worker) HandleMessage(from gen.PID, message any) error {
    switch m := message.(type) {
    case meta.MessageTCP:
        copied := make([]byte, len(m.Data))
        copy(copied, m.Data)
        w.dataQueue = append(w.dataQueue, copied)
        bufferPool.Put(m.Data)
    }
    return nil
}
```

Pool buffers are reused. Storing them directly leads to data corruption. Always copy, then return.
