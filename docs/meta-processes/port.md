# Port

Actors communicate through message passing within the framework. But what if you need to integrate with an external program written in Python, C, or any other language? You could spawn goroutines to manage stdin/stdout, handle protocol framing, deal with buffer management - but this breaks the actor model and spreads I/O complexity throughout your code.

Port meta-process solves this by wrapping external programs as actors. The external program runs as a child process. You send messages to the Port, and it writes them to the program's stdin. The Port reads from stdout and sends you messages. From your actor's perspective, you're just exchanging messages with another actor - the external program's details are abstracted away.

This enables clean integration with legacy systems, specialized libraries in other languages, or any tool that uses stdin/stdout for communication. The actor model stays intact while bridging to external processes.

## Creating a Port

Create a Port with `meta.CreatePort` and spawn it as a meta-process:

```go
type Controller struct {
    act.Actor
    portID gen.Alias
}

func (c *Controller) Init(args ...any) error {
    // Define port options
    options := meta.PortOptions{
        Cmd:  "python3",
        Args: []string{"processor.py", "--mode=batch"},
        Env: map[gen.Env]string{
            "WORKER_ID": "worker-1",
        },
    }
    
    // Create port behavior
    portBehavior, err := meta.CreatePort(options)
    if err != nil {
        return fmt.Errorf("failed to create port: %w", err)
    }
    
    // Spawn as meta-process
    portID, err := c.SpawnMeta(portBehavior, gen.MetaOptions{})
    if err != nil {
        return fmt.Errorf("failed to spawn port: %w", err)
    }
    
    c.portID = portID
    c.Log().Info("spawned port for %s (id: %s)", options.Cmd, portID)
    return nil
}
```

The Port starts the external program and establishes three pipes: stdin (for writing), stdout (for reading), and stderr (for errors). The program runs as a child process managed by the Port meta-process.

When the Port starts, it sends `MessagePortStart` to your actor. When the external program terminates (or the Port is stopped), it sends `MessagePortTerminate`. Between these, you exchange data messages.

## Text Mode: Line-Based Communication

By default, Port operates in text mode. It reads stdout line by line and sends each line as `MessagePortText`. It reads stderr the same way and sends errors as `MessagePortError`.

```go
func (c *Controller) HandleMessage(from gen.PID, message any) error {
    switch m := message.(type) {
    case meta.MessagePortStart:
        c.Log().Info("port started: %s", m.ID)
        // Send initial command
        c.Send(m.ID, meta.MessagePortText{Text: "INIT worker-1\n"})
        
    case meta.MessagePortText:
        // Received line from stdout
        c.Log().Info("port output: %s", m.Text)
        c.processOutput(m.Text)
        
    case meta.MessagePortError:
        // Received line from stderr
        c.Log().Warning("port error: %s", m.Error)
        
    case meta.MessagePortTerminate:
        c.Log().Info("port terminated: %s", m.ID)
        // Restart or cleanup
    }
    return nil
}

func (c *Controller) processCommand(cmd string) {
    // Send command to external program
    c.Send(c.portID, meta.MessagePortText{
        Text: cmd + "\n",
    })
}
```

Text mode uses `bufio.Scanner` internally, which splits input by lines (newline delimiter). You can customize the splitting logic:

```go
options := meta.PortOptions{
    Cmd: "processor",
    
    // Custom split function for stdout
    SplitFuncStdout: func(data []byte, atEOF bool) (advance int, token []byte, err error) {
        // Find null-terminated strings instead of newlines
        if i := bytes.IndexByte(data, 0); i >= 0 {
            return i + 1, data[:i], nil
        }
        if atEOF && len(data) > 0 {
            return len(data), data, nil
        }
        return 0, nil, nil
    },
    
    // Custom split function for stderr (optional)
    SplitFuncStderr: bufio.ScanWords, // Split stderr by words
}
```

Text mode is simple and works well for line-oriented protocols: command-response pairs, JSON-per-line, log output, or any text-based format. But it's not suitable for binary protocols.

## Binary Mode: Raw Bytes

For binary protocols (Protobuf, MessagePack, custom framing), enable binary mode:

```go
options := meta.PortOptions{
    Cmd: "binary-processor",
    Binary: meta.PortBinaryOptions{
        Enable:         true,
        ReadBufferSize: 16384,  // 16KB read buffer
    },
}
```

In binary mode, the Port reads raw bytes from stdout and sends them as `MessagePortData`. You send binary data using `MessagePortData` messages:

```go
func (c *Controller) HandleMessage(from gen.PID, message any) error {
    switch m := message.(type) {
    case meta.MessagePortStart:
        // Send binary request
        request := encodeRequest("GET", "/data")
        c.Send(m.ID, meta.MessagePortData{Data: request})
        
    case meta.MessagePortData:
        // Received binary data from stdout
        response := decodeResponse(m.Data)
        c.handleResponse(response)
    }
    return nil
}
```

The Port reads up to `ReadBufferSize` bytes at a time from stdout and sends each chunk as `MessagePortData`. There's no framing or splitting - you receive raw bytes as the Port reads them. If your protocol has message boundaries, you must track them yourself.

Stderr is always processed in text mode, even when binary mode is enabled. Stderr messages arrive as `MessagePortError`.

## Chunking: Automatic Message Framing

Reading raw bytes means dealing with partial messages. A 1KB message might arrive as three separate `MessagePortData` messages (512 bytes, 400 bytes, 88 bytes), or multiple messages might arrive together in one chunk. You need to buffer, reassemble, and detect message boundaries.

Chunking solves this by automatically framing messages. Instead of receiving raw bytes, you receive complete chunks - one `MessagePortData` per message, properly framed.

### Fixed-Length Chunks

If every message is the same size, use fixed-length chunking:

```go
options := meta.PortOptions{
    Cmd: "fixed-protocol",
    Binary: meta.PortBinaryOptions{
        Enable: true,
        ReadChunk: meta.ChunkOptions{
            Enable:      true,
            FixedLength: 256,  // Every message is exactly 256 bytes
        },
    },
}
```

The Port buffers stdout until it has 256 bytes, then sends them as one `MessagePortData`. If a read returns 512 bytes, you receive two `MessagePortData` messages (256 bytes each). If a read returns 100 bytes, the Port waits for more data before sending.

This is efficient for fixed-size protocols: binary structs, fixed-width encodings, or any format where every message has the same length.

### Header-Based Chunking

Most binary protocols use variable-length messages with a header that specifies the length. Chunking can parse these headers automatically:

```go
options := meta.PortOptions{
    Cmd: "length-prefix-protocol",
    Binary: meta.PortBinaryOptions{
        Enable: true,
        ReadChunk: meta.ChunkOptions{
            Enable: true,
            
            // Header structure
            HeaderSize:           4,  // 4-byte header
            HeaderLengthPosition: 0,  // Length starts at byte 0
            HeaderLengthSize:     4,  // Length is a 4-byte integer
            
            // Does length include the header?
            HeaderLengthIncludesHeader: false,  // Length is payload only
            
            // Safety limit
            MaxLength: 1048576,  // Max 1MB per message
        },
    },
}
```

This configuration matches a protocol where:
- Every message starts with a 4-byte header
- The header contains a 4-byte big-endian integer (bytes 0-3)
- The integer specifies the payload length (header not included)
- Messages are: `[4-byte length][payload]`

The Port reads the header, extracts the length, waits for the full payload to arrive, then sends the complete message (header + payload) as `MessagePortData`.

**Example protocol:**

```
Message 1: [0x00 0x00 0x00 0x0A] [10 bytes of payload]
Message 2: [0x00 0x00 0x01 0x00] [256 bytes of payload]
```

With the configuration above, you receive two `MessagePortData` messages:
- First: 14 bytes (4-byte header + 10-byte payload)
- Second: 260 bytes (4-byte header + 256-byte payload)

If the external program writes both messages at once (274 bytes total), the Port automatically splits them. If the program writes slowly (header arrives, then payload arrives later), the Port waits for the complete message before sending.

**Header length options:**

```go
// Length is in bytes 2-3 (2-byte length at offset 2)
HeaderLengthPosition: 2,
HeaderLengthSize:     2,

// Length includes the header (length = total message size)
HeaderLengthIncludesHeader: true,

// Protocol: [type][flags][length-MSB][length-LSB][payload]
//            byte0  byte1  byte2       byte3      bytes 4+
```

`HeaderLengthSize` can be 1, 2, or 4 bytes. All lengths are big-endian. The Port reads the header, extracts the length value, computes the total message size (adding header size if `HeaderLengthIncludesHeader` is false), and buffers until the complete message arrives.

**MaxLength protection:**

```go
MaxLength: 65536,  // Reject messages larger than 64KB
```

If the header specifies a length exceeding `MaxLength`, the Port terminates with `gen.ErrTooLarge`. This protects against malformed messages or malicious programs that claim a message is 4GB (causing memory exhaustion).

Set `MaxLength` based on your protocol's reasonable maximum. Leave it zero for no limit (use cautiously).

## Buffer Management

The Port allocates buffers for reading stdout. By default, each read allocates a new buffer, which is sent in `MessagePortData` and becomes garbage when you're done with it. For high-throughput ports, this causes GC pressure.

Use a buffer pool to reuse buffers:

```go
bufferPool := &sync.Pool{
    New: func() any {
        return make([]byte, 16384)
    },
}

options := meta.PortOptions{
    Cmd: "high-throughput",
    Binary: meta.PortBinaryOptions{
        Enable:         true,
        ReadBufferSize: 16384,
        ReadBufferPool: bufferPool,
    },
}
```

The Port gets buffers from the pool when reading stdout. When you receive `MessagePortData`, the `Data` field is a buffer from the pool. **You must return it to the pool when done**:

```go
func (c *Controller) HandleMessage(from gen.PID, message any) error {
    switch m := message.(type) {
    case meta.MessagePortData:
        // Process the data
        c.processData(m.Data)
        
        // Return buffer to pool
        bufferPool.Put(m.Data)
    }
    return nil
}
```

If you forget to return buffers, the pool will allocate new ones, defeating the purpose. If you return a buffer and then access it later, you'll get corrupted data (the buffer is reused by the Port for the next read).

When you send `MessagePortData` to write to stdin, the Port automatically returns the buffer to the pool after writing (if a pool is configured). You don't need to do anything:

```go
buf := bufferPool.Get().([]byte)
// Fill buf with data
c.Send(portID, meta.MessagePortData{Data: buf})
// Port returns buf to pool after writing
```

Buffer pools are critical for high-throughput scenarios. For low-volume ports (a few messages per second), the GC overhead is negligible - skip the pool for simplicity.

## Write Keepalive

Some external programs expect periodic input to stay alive. If stdin goes silent for too long, they timeout or disconnect. You could send keepalive messages from your actor (with timers), but that's tedious and error-prone.

Enable automatic keepalive:

```go
options := meta.PortOptions{
    Cmd: "keepalive-required",
    Binary: meta.PortBinaryOptions{
        Enable:                     true,
        WriteBufferKeepAlive:       []byte{0x00}, // Send null byte
        WriteBufferKeepAlivePeriod: 5 * time.Second,
    },
}
```

The Port wraps stdin with a keepalive flusher. If nothing is written for `WriteBufferKeepAlivePeriod`, it automatically sends `WriteBufferKeepAlive` bytes. This keeps the connection alive without any action from your actor.

The keepalive message can be anything: a null byte, a specific protocol message, a ping command. The external program receives it as normal stdin input. Design your protocol to ignore or handle keepalive messages.

Keepalive is only available in binary mode. In text mode, you need to send keepalive messages manually.

## Environment Variables

The external program inherits environment variables based on your configuration:

```go
options := meta.PortOptions{
    Cmd: "processor",
    
    // Enable OS environment variables (PATH, HOME, etc)
    EnableEnvOS: true,
    
    // Enable meta-process environment variables
    EnableEnvMeta: true,
    
    // Custom environment variables
    Env: map[gen.Env]string{
        "WORKER_ID": "worker-1",
        "LOG_LEVEL": "debug",
    },
}
```

**EnableEnvOS**: Includes the operating system's environment. This gives the program access to `PATH`, `HOME`, `USER`, and other system variables. Useful when the program needs to find other executables or access user-specific paths.

**EnableEnvMeta**: Includes environment variables from the meta-process (inherited from its parent actor). Meta-processes share their parent's environment. If the parent has `MY_VAR=value`, the Port's external program sees `MY_VAR=value` too.

**Env**: Custom variables specific to this Port. These are always included regardless of the other flags.

Order of precedence (if duplicate names):
1. Custom `Env` (highest priority)
2. Meta-process environment
3. OS environment (lowest priority)

## Routing Messages

By default, all Port messages (start, terminate, data, errors) go to the parent process - the actor that spawned the Port. For single-port scenarios, this is fine. For multiple ports or advanced architectures, you want routing:

```go
options := meta.PortOptions{
    Cmd:     "worker",
    Process: "data_handler",  // Send all messages to this registered process
}
```

All Port messages are sent to the process registered as `data_handler`. This enables:

**Worker pools:**

```go
options := meta.PortOptions{
    Cmd:     "processor",
    Process: "worker_pool",  // act.Pool actor
}
```

The Port sends all messages to a pool, which distributes them across workers. Multiple ports can share the same pool for load balancing.

**Centralized handlers:**

```go
options := meta.PortOptions{
    Cmd:     "python3",
    Args:    []string{"script1.py"},
    Process: "python_manager",
}

options2 := meta.PortOptions{
    Cmd:     "python3",
    Args:    []string{"script2.py"},
    Process: "python_manager",
}
```

Both ports send messages to `python_manager`, which coordinates multiple Python scripts.

**Distinguishing ports with tags:**

```go
options1 := meta.PortOptions{
    Cmd:     "worker",
    Tag:     "input-processor",
    Process: "manager",
}

options2 := meta.PortOptions{
    Cmd:     "worker",
    Tag:     "output-formatter",
    Process: "manager",
}
```

The `Tag` field appears in all Port messages. The manager uses it to distinguish which port sent the message:

```go
func (m *Manager) HandleMessage(from gen.PID, message any) error {
    switch msg := message.(type) {
    case meta.MessagePortData:
        switch msg.Tag {
        case "input-processor":
            m.handleInput(msg.Data)
        case "output-formatter":
            m.handleOutput(msg.Data)
        }
    }
    return nil
}
```

If `Process` is empty or not registered, messages go to the parent process.

## Port Messages

Messages you receive from the Port:

**MessagePortStart** - Port started successfully, external program is running:

```go
type MessagePortStart struct {
    ID  gen.Alias  // Port's meta-process ID
    Tag string     // Tag from PortOptions
}
```

Sent once after the external program starts. Use this to send initialization commands.

**MessagePortTerminate** - Port stopped, external program exited:

```go
type MessagePortTerminate struct {
    ID  gen.Alias
    Tag string
}
```

Sent when the external program terminates (exit, crash, killed) or when you terminate the Port. After this, the Port is dead - you cannot send it more messages.

**MessagePortText** - Line from stdout (text mode only):

```go
type MessagePortText struct {
    ID   gen.Alias
    Tag  string
    Text string  // One line (delimiter removed)
}
```

Sent for each line read from stdout in text mode. The delimiter (newline or custom) is stripped from `Text`.

**MessagePortData** - Binary data from stdout (binary mode only):

```go
type MessagePortData struct {
    ID   gen.Alias
    Tag  string
    Data []byte  // Raw bytes or complete chunk
}
```

In binary mode without chunking, `Data` contains whatever bytes the Port read (up to `ReadBufferSize`). With chunking, `Data` contains one complete chunk.

If `ReadBufferPool` is configured, `Data` is from the pool - return it when done.

**MessagePortError** - Line from stderr (always text mode):

```go
type MessagePortError struct {
    ID    gen.Alias
    Tag   string
    Error error  // Line from stderr as an error
}
```

Sent for each line read from stderr. Stderr is always processed in text mode, even when binary mode is enabled for stdout.

Messages you send to the Port:

**MessagePortText** - Send text to stdin (text mode):

```go
c.Send(portID, meta.MessagePortText{
    Text: "COMMAND arg1 arg2\n",
})
```

Writes `Text` to stdin. Newlines are not added automatically - include them if your protocol needs them.

**MessagePortData** - Send binary data to stdin (binary mode):

```go
c.Send(portID, meta.MessagePortData{
    Data: encodedMessage,
})
```

Writes `Data` to stdin. If `ReadBufferPool` is configured, the Port returns the buffer to the pool after writing. Don't use the buffer after sending.

## Termination and Cleanup

When the external program exits (normally or crash), the Port sends `MessagePortTerminate` and terminates itself. The Port also kills the external program if:

- The Port is terminated (you call `process.SendExit` to the Port's ID)
- The Port's parent terminates (cascading termination)
- An error occurs reading stdout (broken pipe, I/O error)

The Port calls `Kill()` on the child process and waits for it to exit. This ensures cleanup happens even if the program is misbehaving.

Stderr is read in a separate goroutine. This means stderr messages can arrive after `MessagePortTerminate` if the program wrote to stderr just before exiting. Design your actor to handle this ordering.

## Inspection

Port supports inspection for debugging:

```go
result, err := process.Call(portID, gen.Inspect{})
```

Returns a map with Port status:

```go
map[string]string{
    "tag":               "worker-1",
    "cmd":               "/usr/bin/python3",
    "args":              "[script.py --mode=batch]",
    "pid":               "12345",       // OS process ID
    "binary":            "true",        // Binary mode enabled
    "binary.read_chunk": "true",        // Chunking enabled
    "env":               "[WORKER_ID=worker-1]",
    "pwd":               "/path/to/working/dir",
    "bytesIn":           "1048576",     // Bytes read from stdout
    "bytesOut":          "524288",      // Bytes written to stdin
}
```

Use this for monitoring, debugging, or displaying Port status in management UIs.

## Patterns and Pitfalls

**Pattern: Request-response wrapper**

```go
type PortWrapper struct {
    act.Actor
    portID   gen.Alias
    pending  map[gen.Ref]gen.PID
    sequence uint64
}

func (w *PortWrapper) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
    // Generate unique request ID
    reqID := atomic.AddUint64(&w.sequence, 1)
    
    // Store caller
    w.pending[ref] = from
    
    // Send to port with ID
    w.Send(w.portID, meta.MessagePortText{
        Text: fmt.Sprintf("%d:%s\n", reqID, request),
    })
    
    // Will respond asynchronously
    return nil, nil
}

func (w *PortWrapper) HandleMessage(from gen.PID, message any) error {
    switch m := message.(type) {
    case meta.MessagePortText:
        // Parse response: "reqID:result"
        parts := strings.SplitN(m.Text, ":", 2)
        reqID, result := parts[0], parts[1]
        
        // Find pending caller
        for ref, caller := range w.pending {
            if matchesRequestID(ref, reqID) {
                w.SendResponse(caller, ref, result)
                delete(w.pending, ref)
                break
            }
        }
    }
    return nil
}
```

Wrap a Port to provide synchronous Call semantics. Useful for RPC-style protocols.

**Pattern: Supervised restart**

```go
type PortSupervisor struct {
    act.Supervisor
}

func (s *PortSupervisor) Init(args ...any) (act.SupervisorSpec, error) {
    return act.SupervisorSpec{
        Children: []act.SupervisorChildSpec{
            {
                Name:    "port_manager",
                Factory: createPortManager,
            },
        },
        Restart: act.SupervisorRestart{
            Strategy:  act.SupervisorStrategyPermanent,
            Intensity: 5,
            Period:    10,
        },
    }, nil
}
```

Supervise the actor that spawns ports. If the actor crashes, the supervisor restarts it, which re-spawns ports. Ports inherit parent lifecycle - when the actor terminates, all its ports terminate.

**Pattern: Backpressure with buffer pool**

```go
bufferPool := &sync.Pool{
    New: func() any {
        return make([]byte, 8192)
    },
}

// Limit concurrent buffers
sem := make(chan struct{}, 100) // Max 100 buffers in flight

func (c *Controller) HandleMessage(from gen.PID, message any) error {
    switch m := message.(type) {
    case meta.MessagePortData:
        // Acquire semaphore (blocks if 100 buffers in use)
        sem <- struct{}{}
        
        go func() {
            defer func() {
                <-sem             // Release semaphore
                bufferPool.Put(m.Data) // Return buffer
            }()
            
            // Process data (can be slow)
            c.processData(m.Data)
        }()
    }
    return nil
}
```

Limit memory usage by capping concurrent buffers. If processing is slow, the semaphore blocks, which blocks the actor's message loop, which applies backpressure to the Port.

**Pitfall: Forgetting to return buffers**

```go
// WRONG: Buffer leaked
func (c *Controller) HandleMessage(from gen.PID, message any) error {
    switch m := message.(type) {
    case meta.MessagePortData:
        c.dataQueue = append(c.dataQueue, m.Data) // Stored, never returned!
    }
    return nil
}

// CORRECT: Copy if you need to store
func (c *Controller) HandleMessage(from gen.PID, message any) error {
    switch m := message.(type) {
    case meta.MessagePortData:
        copied := make([]byte, len(m.Data))
        copy(copied, m.Data)
        c.dataQueue = append(c.dataQueue, copied)
        bufferPool.Put(m.Data) // Return original
    }
    return nil
}
```

Pool buffers are reused. If you store them, they'll be overwritten by future reads. Copy data if you need to keep it.

**Pitfall: Blocking on stdin writes**

```go
// Port writes are blocking
c.Send(portID, meta.MessagePortData{Data: largeBuffer})
// ^ This Send doesn't block, but the Port's write to stdin might
```

If the external program stops reading stdin (buffer full, process blocked), the Port blocks writing. The Port's `HandleMessage` is blocked, so it can't send you more stdout data. Deadlock.

Solution: Design your protocol so the external program never stops reading stdin. Use flow control or chunking to prevent overflows.

**Pitfall: Ignoring MessagePortError**

```go
// WRONG: Stderr ignored
func (c *Controller) HandleMessage(from gen.PID, message any) error {
    switch m := message.(type) {
    case meta.MessagePortData:
        c.process(m.Data)
    // No case for MessagePortError!
    }
    return nil
}
```

Stderr messages arrive as `MessagePortError`. If you don't handle them, warnings and errors from the external program are lost. Always handle stderr or explicitly decide to ignore it.

**Pitfall: Not handling MessagePortTerminate**

```go
// WRONG: Port terminated, but actor keeps trying to use it
func (c *Controller) processData(data []byte) {
    c.Send(c.portID, meta.MessagePortData{Data: data})
    // ^ Fails if port terminated
}
```

After `MessagePortTerminate`, the Port is dead. Sending messages returns errors. Handle termination: restart the Port, fail gracefully, or terminate your actor.

Port meta-processes enable clean integration with external programs. They handle process management, I/O buffering, protocol framing, and lifecycle coordination - letting you focus on the protocol logic while maintaining the actor model's isolation and simplicity.
