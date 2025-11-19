# UDP

UDP is fundamentally different from TCP. There are no connections, no ordering guarantees, no reliability. Datagrams arrive independently, potentially out of order, possibly duplicated, or lost entirely. This makes UDP simpler than TCP, but also requires different handling patterns.

Traditional UDP servers use blocking `ReadFrom` calls in loops. This doesn't fit the actor model's one-message-at-a-time processing. You could spawn goroutines to read packets, but this breaks actor isolation and requires manual synchronization.

UDP meta-process wraps the socket in an actor. It runs a read loop in the `Start` goroutine, sending each received datagram as a message to your actor. To send datagrams, you send messages to the UDP server's meta-process. The actor model stays intact while integrating with blocking UDP operations.

Unlike TCP, UDP has no connections. One meta-process handles the entire socket - all incoming datagrams from all remote addresses. There's no per-connection state, no connection lifecycle, no connect/disconnect messages. Just datagrams in, datagrams out.

## Creating a UDP Server

Create a UDP server with `meta.CreateUDPServer`:

```go
type DNSServer struct {
    act.Actor
    udpID gen.Alias
}

func (d *DNSServer) Init(args ...any) error {
    options := meta.UDPServerOptions{
        Host:       "0.0.0.0",
        Port:       53,
        BufferSize: 512,  // DNS messages are typically small
    }
    
    server, err := meta.CreateUDPServer(options)
    if err != nil {
        return fmt.Errorf("failed to create UDP server: %w", err)
    }
    
    udpID, err := d.SpawnMeta(server, gen.MetaOptions{})
    if err != nil {
        // Failed to spawn - close the socket
        server.Terminate(err)
        return fmt.Errorf("failed to spawn UDP server: %w", err)
    }
    
    d.udpID = udpID
    d.Log().Info("DNS server listening on %s:%d (id: %s)", 
        options.Host, options.Port, udpID)
    return nil
}
```

The server opens a UDP socket and enters a read loop. For each received datagram, it sends `MessageUDP` to your actor. Your actor processes it and optionally sends a response by sending `MessageUDP` back to the server's meta-process ID.

If `SpawnMeta` fails, call `server.Terminate(err)` to close the socket. Without this, the port remains bound until the process exits.

The server runs forever, reading datagrams and forwarding them as messages. When the parent actor terminates, the server terminates too (cascading termination), closing the socket.

## Handling Datagrams

The UDP server sends `MessageUDP` for each received datagram:

```go
func (d *DNSServer) HandleMessage(from gen.PID, message any) error {
    switch m := message.(type) {
    case meta.MessageUDP:
        // Received UDP datagram
        d.Log().Info("received %d bytes from %s", len(m.Data), m.Addr)
        
        // Parse DNS query
        query, err := d.parseDNSQuery(m.Data)
        if err != nil {
            d.Log().Warning("invalid DNS query from %s: %s", m.Addr, err)
            return nil
        }
        
        // Build DNS response
        response := d.buildDNSResponse(query)
        
        // Send response back to the same address
        d.Send(d.udpID, meta.MessageUDP{
            Addr: m.Addr,
            Data: response,
        })
    }
    return nil
}
```

**MessageUDP** contains:
- **ID**: The UDP server's meta-process ID (same for all datagrams)
- **Addr**: Remote address that sent this datagram (`net.Addr` - typically `*net.UDPAddr`)
- **Data**: The datagram payload (up to `BufferSize` bytes)

To send a datagram, send `MessageUDP` to the server's ID with the destination address and payload. The server writes it to the socket with `WriteTo`. The `ID` field is ignored when sending (it's only used for incoming datagrams).

Unlike TCP:
- No connect/disconnect messages - datagrams are independent
- `Addr` changes for each datagram - track remote addresses yourself if needed
- No message framing - each UDP datagram is a complete message
- No ordering guarantees - process datagrams as they arrive

## Connectionless Nature

UDP has no connections. Each datagram is independent. The same remote address might send multiple datagrams, but there's no session state. If you need state per remote address, maintain it yourself:

```go
type GameServer struct {
    act.Actor
    udpID   gen.Alias
    players map[string]*PlayerState  // Key: remote address string
}

type PlayerState struct {
    addr       net.Addr
    lastSeen   time.Time
    position   Vector3
    health     int
}

func (g *GameServer) HandleMessage(from gen.PID, message any) error {
    switch m := message.(type) {
    case meta.MessageUDP:
        addrStr := m.Addr.String()
        
        // Get or create player state
        player, exists := g.players[addrStr]
        if !exists {
            player = &PlayerState{
                addr:   m.Addr,
                health: 100,
            }
            g.players[addrStr] = player
            g.Log().Info("new player: %s", addrStr)
        }
        
        // Update last seen
        player.lastSeen = time.Now()
        
        // Process game packet
        g.processGamePacket(player, m.Data)
        
    case CleanupTick:
        // Remove stale players
        now := time.Now()
        for addr, player := range g.players {
            if now.Sub(player.lastSeen) > 30*time.Second {
                delete(g.players, addr)
                g.Log().Info("player timeout: %s", addr)
            }
        }
    }
    return nil
}
```

Because UDP has no connection lifecycle, you need application-level timeout logic to clean up stale state. The server doesn't know when clients "disconnect" - they just stop sending datagrams.

## Routing to Workers

By default, all datagrams go to the parent actor. For servers handling high datagram rates, this creates a bottleneck. Use `Process` to route to a different handler:

```go
options := meta.UDPServerOptions{
    Port:    8125,
    Process: "metrics_collector",
}
```

All datagrams go to `metrics_collector` instead of the parent. This enables separation of concerns - the actor that creates the UDP server doesn't need to handle datagrams.

Unlike TCP's `ProcessPool`, UDP only has a single `Process` field. You can route to an `act.Pool`:

```go
// Start worker pool
poolPID, _ := process.SpawnRegister("udp_pool", createWorkerPool, gen.ProcessOptions{})

options := meta.UDPServerOptions{
    Port:    8125,
    Process: "udp_pool",  // act.Pool is OK for UDP
}
```

Each datagram is forwarded to the pool, which distributes them across workers. **This works for UDP because datagrams are independent** - there's no per-connection state to corrupt. For TCP, `ProcessPool` uses round-robin to maintain connection-to-worker binding. For UDP, the pool can distribute freely.

Use pools when datagram processing is CPU-intensive or slow (database writes, external API calls). Workers process datagrams in parallel, maximizing throughput.

## Buffer Management

The UDP server allocates a buffer for each datagram read. By default, it allocates a new buffer every time, which becomes garbage after you process it. For high datagram rates, this causes GC pressure.

Use a buffer pool:

```go
bufferPool := &sync.Pool{
    New: func() any {
        return make([]byte, 1500)  // MTU size
    },
}

options := meta.UDPServerOptions{
    Port:       8125,
    BufferSize: 1500,
    BufferPool: bufferPool,
}
```

The server gets buffers from the pool when reading. When you receive `MessageUDP`, the `Data` field is a buffer from the pool. **Return it to the pool after processing**:

```go
func (s *StatsServer) HandleMessage(from gen.PID, message any) error {
    switch m := message.(type) {
    case meta.MessageUDP:
        // Process datagram
        s.processMetric(m.Data)
        
        // Return buffer to pool
        bufferPool.Put(m.Data)
    }
    return nil
}
```

When you send `MessageUDP` to write a datagram, the server automatically returns the buffer to the pool after writing (if a pool is configured). Don't use the buffer after sending.

If you need to store data beyond the current message, copy it:

```go
case meta.MessageUDP:
    // Store in queue - must copy
    copied := make([]byte, len(m.Data))
    copy(copied, m.Data)
    s.queue = append(s.queue, copied)
    
    // Return original buffer
    bufferPool.Put(m.Data)
```

Buffer pools are essential for servers receiving thousands of datagrams per second. For low-volume servers (a few datagrams per second), the GC overhead is negligible - skip the pool for simplicity.

## Buffer Size

UDP datagrams are limited by the network's Maximum Transmission Unit (MTU). IPv4 networks typically have 1500-byte MTU, IPv6 has 1280-byte minimum. After subtracting IP and UDP headers (28 bytes for IPv4, 48 bytes for IPv6), you get:

- **IPv4 safe maximum**: 1472 bytes (1500 - 28)
- **IPv6 safe maximum**: 1232 bytes (1280 - 48)
- **Internet-safe maximum**: 512 bytes (DNS requirement)

Datagrams larger than MTU are fragmented at the IP layer. Fragmented datagrams are reassembled by the receiving OS before `ReadFrom` returns. However, if any fragment is lost, the entire datagram is discarded - UDP reliability degrades.

The default `BufferSize` is 65000 bytes (close to UDP's theoretical maximum of 65507 bytes). This handles any UDP datagram, but it's wasteful if your protocol uses smaller messages:

```go
// DNS server - queries rarely exceed 512 bytes
options := meta.UDPServerOptions{
    Port:       53,
    BufferSize: 512,
}

// Game server - small position updates
options := meta.UDPServerOptions{
    Port:       9999,
    BufferSize: 128,
}

// Media streaming - large packets OK
options := meta.UDPServerOptions{
    Port:       5004,
    BufferSize: 8192,
}
```

If a datagram is larger than `BufferSize`, it's truncated - you receive only the first `BufferSize` bytes. The rest is discarded. Set `BufferSize` to the maximum expected datagram size for your protocol.

Smaller buffers reduce memory usage (important with buffer pools). Larger buffers avoid truncation but waste memory if datagrams are typically small.

## No Chunking

Unlike TCP, UDP meta-process has no chunking support. UDP datagrams are atomic - each datagram is a complete message. There's no byte stream to split or reassemble. The protocol boundary is the datagram boundary.

If your protocol sends multi-datagram messages, you must handle reassembly yourself:

```go
type ReassemblyHandler struct {
    act.Actor
    fragments map[uint32]*FragmentSet  // Key: message ID
}

type FragmentSet struct {
    fragments []*Fragment
    received  map[int]bool
    total     int
}

func (r *ReassemblyHandler) HandleMessage(from gen.PID, message any) error {
    switch m := message.(type) {
    case meta.MessageUDP:
        // Parse fragment header
        msgID, fragNum, totalFrags := r.parseFragmentHeader(m.Data)
        
        // Get or create fragment set
        set, exists := r.fragments[msgID]
        if !exists {
            set = &FragmentSet{
                fragments: make([]*Fragment, totalFrags),
                received:  make(map[int]bool),
                total:     totalFrags,
            }
            r.fragments[msgID] = set
        }
        
        // Store fragment
        set.fragments[fragNum] = &Fragment{data: m.Data}
        set.received[fragNum] = true
        
        // Check if complete
        if len(set.received) == set.total {
            complete := r.reassemble(set.fragments)
            r.processMessage(complete)
            delete(r.fragments, msgID)
        }
        
        bufferPool.Put(m.Data)
    }
    return nil
}
```

UDP delivers datagrams out of order. Fragment 2 might arrive before fragment 1. Your reassembly logic must handle this. Use sequence numbers, timeouts for incomplete sets, and protection against memory exhaustion (limit maximum incomplete messages).

Most UDP protocols avoid multi-datagram messages entirely. Keep messages under MTU size for reliability and simplicity.

## Unreliability and Idempotence

UDP datagrams can be:
- **Lost**: Network congestion, router overload, buffer overflow
- **Duplicated**: Network retransmission, switch mirroring
- **Reordered**: Different paths through the network
- **Corrupted**: Rare, but possible despite checksums

Design your protocol to handle these:

**Loss tolerance**: Don't rely on every datagram arriving. Either accept loss (game state updates, sensor readings) or implement application-level acknowledgment and retransmission.

**Duplicate tolerance**: Process datagrams idempotently. If the same datagram arrives twice, the result is the same. Use sequence numbers to detect and discard duplicates:

```go
type Player struct {
    lastSequence uint32
}

func (g *GameServer) processGamePacket(player *Player, data []byte) {
    seq := binary.BigEndian.Uint32(data[0:4])
    
    // Discard old/duplicate packets
    if seq <= player.lastSequence {
        return
    }
    
    player.lastSequence = seq
    // Process packet
}
```

**Reordering tolerance**: Don't assume datagrams arrive in send order. Use timestamps or sequence numbers to handle reordering:

```go
type Measurement struct {
    timestamp time.Time
    value     float64
}

func (s *StatsCollector) processMeasurement(m Measurement) {
    // Store measurements in order by timestamp
    s.insertSorted(m)
}
```

**Corruption detection**: UDP has a 16-bit checksum, but it's weak. Critical data should have application-level integrity checks (CRC32, hash, signature).

Most importantly: **design your protocol so datagram loss doesn't break functionality**. UDP is for scenarios where loss is acceptable (real-time updates) or where you implement your own reliability layer (QUIC, custom protocols).

## Inspection

UDP server supports inspection for debugging:

```go
serverInfo, _ := process.Call(udpID, gen.Inspect{})
// Returns: map[string]string{
//     "listener":  "0.0.0.0:8125",
//     "process":   "metrics_collector",
//     "bytes in":  "10485760",
//     "bytes out": "1048576",
// }
```

Use this for monitoring datagram counts, bandwidth usage, or displaying server status.

## Patterns and Pitfalls

**Pattern: Metrics aggregation**

```go
type MetricsCollector struct {
    act.Actor
    metrics map[string]*Metric
}

func (m *MetricsCollector) HandleMessage(from gen.PID, message any) error {
    switch msg := message.(type) {
    case meta.MessageUDP:
        // Parse StatsD format: "metric.name:value|type"
        name, value, metricType := m.parseStatsD(msg.Data)
        
        metric := m.metrics[name]
        if metric == nil {
            metric = &Metric{}
            m.metrics[name] = metric
        }
        
        metric.update(value, metricType)
        bufferPool.Put(msg.Data)
        
    case FlushTick:
        // Periodically flush aggregated metrics
        m.flushMetrics()
        m.metrics = make(map[string]*Metric)
    }
    return nil
}
```

Aggregate many datagrams into periodic summaries. Lossy protocols (like StatsD) rely on volume - losing a few datagrams doesn't affect aggregate accuracy.

**Pattern: Request-response with timeout**

```go
type DNSClient struct {
    act.Actor
    udpID   gen.Alias
    pending map[uint16]*PendingQuery  // Key: DNS query ID
}

func (c *DNSClient) query(domain string) {
    queryID := c.nextQueryID()
    query := c.buildDNSQuery(queryID, domain)
    
    // Send query
    c.Send(c.udpID, meta.MessageUDP{
        Addr: c.dnsServerAddr,
        Data: query,
    })
    
    // Store pending query with timeout
    c.pending[queryID] = &PendingQuery{
        domain:  domain,
        sent:    time.Now(),
        timeout: c.SendAfter(c.PID(), QueryTimeout{queryID}, 5*time.Second),
    }
}

func (c *DNSClient) HandleMessage(from gen.PID, message any) error {
    switch m := message.(type) {
    case meta.MessageUDP:
        // Parse DNS response
        queryID := binary.BigEndian.Uint16(m.Data[0:2])
        pending := c.pending[queryID]
        if pending != nil {
            pending.timeout()  // Cancel timeout
            c.handleResponse(m.Data)
            delete(c.pending, queryID)
        }
        bufferPool.Put(m.Data)
        
    case QueryTimeout:
        // Query timed out - maybe retry
        pending := c.pending[m.queryID]
        if pending != nil {
            c.Log().Warning("DNS query timeout: %s", pending.domain)
            delete(c.pending, m.queryID)
        }
    }
    return nil
}
```

Implement application-level reliability with timeouts and retries. UDP doesn't guarantee delivery, so you must detect and handle failures.

**Pattern: Broadcast responder**

```go
type DiscoveryServer struct {
    act.Actor
    udpID gen.Alias
}

func (d *DiscoveryServer) HandleMessage(from gen.PID, message any) error {
    switch m := message.(type) {
    case meta.MessageUDP:
        if string(m.Data) == "DISCOVER" {
            response := d.buildDiscoveryResponse()
            
            // Reply to sender
            d.Send(d.udpID, meta.MessageUDP{
                Addr: m.Addr,
                Data: response,
            })
        }
        bufferPool.Put(m.Data)
    }
    return nil
}
```

Respond to broadcast discovery requests. Track sender address from `MessageUDP.Addr` and reply directly.

**Pitfall: Not returning buffers**

```go
// WRONG: Buffer leaked
func (s *Server) HandleMessage(from gen.PID, message any) error {
    switch m := message.(type) {
    case meta.MessageUDP:
        // Store in queue without copying
        s.queue = append(s.queue, m.Data)  // Buffer still referenced!
    }
    return nil
}

// CORRECT: Copy before storing
func (s *Server) HandleMessage(from gen.PID, message any) error {
    switch m := message.(type) {
    case meta.MessageUDP:
        copied := make([]byte, len(m.Data))
        copy(copied, m.Data)
        s.queue = append(s.queue, copied)
        bufferPool.Put(m.Data)  // Return original
    }
    return nil
}
```

Pool buffers are reused immediately. Storing them leads to data corruption when the pool reuses the buffer for the next datagram.

**Pitfall: Assuming reliability**

```go
// WRONG: Assumes all datagrams arrive
func (c *Client) sendTransaction(tx Transaction) {
    // Send 10 chunks
    for i := 0; i < 10; i++ {
        chunk := tx.getChunk(i)
        c.Send(c.udpID, meta.MessageUDP{
            Addr: c.serverAddr,
            Data: chunk,
        })
    }
    // Server will process when all 10 arrive... right? WRONG!
}
```

Some chunks will be lost. The server waits forever for missing chunks, or processes incomplete data. Either accept loss (send redundant data) or implement acknowledgment and retransmission.

**Pitfall: Large datagrams**

```go
// WRONG: Likely to be fragmented or lost
data := make([]byte, 8000)  // 8KB datagram
c.Send(c.udpID, meta.MessageUDP{
    Addr: serverAddr,
    Data: data,
})
```

IP-level fragmentation significantly increases loss probability. If any fragment is lost, the entire datagram is discarded. Keep datagrams under 1472 bytes for reliability, or 512 bytes for internet-wide compatibility.

**Pitfall: Not handling duplicates**

```go
// WRONG: Processes duplicate commands
func (g *GameServer) handleCommand(player *Player, cmd Command) {
    switch cmd.Type {
    case CmdFireWeapon:
        player.ammo--  // Duplicate datagram = fire twice!
        g.spawnProjectile(player)
    }
}

// CORRECT: Idempotent with sequence tracking
func (g *GameServer) handleCommand(player *Player, cmd Command) {
    if cmd.Sequence <= player.lastSequence {
        return  // Duplicate or old command
    }
    player.lastSequence = cmd.Sequence
    
    switch cmd.Type {
    case CmdFireWeapon:
        player.ammo--
        g.spawnProjectile(player)
    }
}
```

Network equipment can duplicate UDP datagrams (switch mirroring, retransmission logic). Process commands idempotently or track sequence numbers.

UDP meta-process handles the complexity of socket I/O and datagram delivery while maintaining actor isolation. Design your protocol for UDP's unreliable, unordered, connectionless nature - and leverage its simplicity and low latency where reliability isn't critical.