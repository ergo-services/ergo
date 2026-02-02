# Debugging

Debugging distributed actor systems presents unique challenges. Traditional debugging tools struggle with concurrent message passing, process isolation, and distributed state. This article covers the debugging capabilities built into Ergo Framework and demonstrates practical techniques for troubleshooting common issues.

## Build Tags

Ergo Framework uses Go build tags to enable debugging features without affecting production performance. These tags control compile-time behavior, ensuring zero overhead when disabled.

### The `pprof` Tag

The `pprof` tag enables the built-in profiler and goroutine labeling:

```bash
go run --tags pprof ./cmd
```

This activates:

- **pprof HTTP endpoint** at `http://localhost:9009/debug/pprof/`
- **PID labels** on actor goroutines and **Alias labels** on meta process goroutines for identification in profiler output

The endpoint address can be customized via environment variables:
- `PPROF_HOST` - host to bind (default: `localhost`)
- `PPROF_PORT` - port to listen on (default: `9009`)

The profiler endpoint exposes standard Go profiling data:

| Endpoint | Description |
|----------|-------------|
| `/debug/pprof/goroutine` | Stack traces of all goroutines |
| `/debug/pprof/heap` | Heap memory allocations |
| `/debug/pprof/profile` | CPU profile (30-second sample) |
| `/debug/pprof/block` | Goroutine blocking events |
| `/debug/pprof/mutex` | Mutex contention |

### The `norecover` Tag

By default, Ergo Framework recovers from panics in actor callbacks to prevent a single misbehaving actor from crashing the entire node. While this improves resilience in production, it can hide bugs during development.

```bash
go run --tags norecover ./cmd
```

With `norecover`, panics propagate normally, providing full stack traces and allowing debuggers to catch the exact failure point. This is particularly useful when:

- Investigating nil pointer dereferences in message handlers
- Tracking down type assertion failures
- Understanding the call sequence leading to a panic

### The `trace` Tag

The `trace` tag enables verbose logging of framework internals:

```bash
go run --tags trace ./cmd
```

This produces detailed output about:

- Process lifecycle events (spawn, terminate, state changes)
- Message routing decisions
- Network connection establishment and teardown
- Supervision tree operations

To see trace output, also set the node's log level:

```go
options := gen.NodeOptions{
    Log: gen.LogOptions{
        Level: gen.LogLevelTrace,
    },
}
```

### Combining Tags

Tags can be combined for comprehensive debugging:

```bash
go run --tags "pprof,norecover,trace" ./cmd
```

This enables all debugging features simultaneously. Use this combination when investigating complex issues that span multiple subsystems.

## Profiler Integration

The Go profiler is a powerful tool for understanding runtime behavior. Ergo Framework enhances its usefulness by labeling goroutines with their identifiers.

### Identifying Actor and Meta Process Goroutines

When built with the `pprof` tag, each actor's goroutine carries a label containing its PID, and each meta process goroutine carries a label with its Alias. This creates a direct link between the logical identity and the runtime goroutine.

To find labeled goroutines:

```bash
# Find actor goroutines by PID
curl -s "http://localhost:9009/debug/pprof/goroutine?debug=1" | grep -B5 'labels:.*pid'

# Find meta process goroutines by Alias
curl -s "http://localhost:9009/debug/pprof/goroutine?debug=1" | grep -B5 'labels:.*meta'
```

Example output for actors:

```
1 @ 0x100c17fa0 0x100c18abc 0x100c19def ...
# labels: {"pid":"<ABC123.0.1005>"}
#   main.(*Worker).HandleMessage+0x27  /path/worker.go:45
```

Example output for meta processes:

```
1 @ 0x100c17fa0 0x100c18abc 0x100c19def ...
# labels: {"meta":"Alias#<ABC123.0.1.2>", "role":"reader"}
#   main.(*TCPServer).Start+0x1bc  /path/tcp_server.go:52
```

Meta processes have two goroutines with different roles:
- `"role":"reader"` - External Reader goroutine running the `Start()` method (blocking I/O)
- `"role":"handler"` - Actor Handler goroutine processing messages (`HandleMessage`/`HandleCall`)

The output shows:
- The goroutine's stack trace
- The identifier label (PID for actors, Alias for meta processes)
- The exact location in your code where the goroutine is currently executing

### Debugging Stuck Processes

During graceful shutdown, Ergo Framework logs processes that are taking too long to terminate. These logs include PIDs that can be matched against profiler output.

Consider a shutdown scenario where the node reports:

```
[warning] shutdown: waiting for 3 processes
[warning]   <ABC123.0.1005> state=running queue=5
[warning]   <ABC123.0.1012> state=running queue=0
[warning]   <ABC123.0.1018> state=sleep queue=0
```

To investigate why `<ABC123.0.1005>` is stuck:

1. Capture the goroutine profile:
```bash
curl -s "http://localhost:9009/debug/pprof/goroutine?debug=2" > goroutines.txt
```

2. Search for the specific PID:
```bash
grep -A30 'pid.*ABC123.0.1005' goroutines.txt
```

3. Analyze the stack trace to understand what the actor is waiting on.

The `debug=2` parameter provides full stack traces with argument values, which is more verbose than `debug=1` but contains more diagnostic information.

### Common Patterns in Stack Traces

Different types of blocking have characteristic stack traces:

**Blocked on channel receive:**
```
runtime.chanrecv1
    /usr/local/go/src/runtime/chan.go:442
```

**Blocked on mutex:**
```
sync.(*Mutex).Lock
    /usr/local/go/src/sync/mutex.go:81
```

**Blocked on network I/O:**
```
internal/poll.(*FD).Read
    /usr/local/go/src/internal/poll/fd_unix.go:163
```

**Blocked on synchronous call (waiting for response):**
```
ergo.services/ergo/node.(*process).waitResponse
    /path/node/process.go:1961
```

Understanding these patterns helps quickly identify the root cause of stuck processes.

## Shutdown Diagnostics

Ergo Framework provides built-in diagnostics during graceful shutdown. When `ShutdownTimeout` is configured (default: 3 minutes), the framework logs pending processes every 5 seconds.

```go
options := gen.NodeOptions{
    ShutdownTimeout: 30 * time.Second, // shorter timeout for debugging
}
```

The shutdown log includes:

- **PID**: Process identifier for correlation with profiler
- **State**: Current process state (running, sleep, etc.)
- **Queue**: Number of messages waiting in the mailbox

A process with `state=running` and `queue=0` is actively processing something (likely stuck in a callback). A process with `state=running` and `queue>0` is stuck while new messages continue to arrive. A process with `state=sleep` and `queue=0` is idle - during shutdown this typically means the process is waiting for its children to terminate first (normal supervision tree behavior).

## Practical Debugging Scenarios

### Scenario: Message Handler Never Returns

Symptoms:
- Process stops responding to messages
- Other processes waiting on `Call` timeout
- Shutdown hangs on specific process

Investigation:

1. Note the PID from shutdown logs or observer
2. Capture goroutine profile with `debug=2`
3. Find the goroutine by PID label
4. Examine the stack trace

Common causes:
- Infinite loop in message handler
- Blocking channel operation
- Deadlock with another process via synchronous calls
- External service call without timeout

Solution approach:
- Never use blocking operations (channels, mutexes) in actor callbacks
- Always use timeouts for external calls
- Use asynchronous messaging patterns where possible

### Scenario: Memory Growth

Symptoms:
- Heap size increases over time
- Process eventually killed by OOM

Investigation:

1. Capture heap profile:
```bash
curl -s "http://localhost:9009/debug/pprof/heap" > heap.prof
go tool pprof heap.prof
```

2. In pprof, use `top` to see largest allocators:
```
(pprof) top 10
```

3. Use `list` to examine specific functions:
```
(pprof) list HandleMessage
```

Common causes:
- Messages accumulating in mailbox faster than processing
- Actor state holding references to large data
- Unbounded caches or buffers in actor state

### Scenario: Distributed Deadlock

Symptoms:
- Two or more processes stop responding
- Circular dependency in synchronous calls

Investigation:

1. Identify stuck processes from shutdown logs
2. For each process, capture its goroutine stack
3. Look for `waitResponse` in stack traces (indicates waiting for synchronous call response)
4. Map the call targets to build a dependency graph

Prevention:
- Prefer asynchronous messaging over synchronous calls
- Design clear hierarchies where calls flow in one direction
- Use timeouts on all synchronous operations
- Consider using request-response patterns with explicit message types

### Scenario: Process Crash Investigation

Symptoms:
- Process terminates unexpectedly
- `TerminateReasonPanic` in logs

Investigation:

1. Build with `--tags norecover` to get full panic stack
2. Run the scenario that triggers the crash
3. Examine the complete stack trace

With `norecover`, the panic propagates with full context:

```
panic: runtime error: invalid memory address or nil pointer dereference

goroutine 42 [running]:
main.(*MyActor).HandleMessage(0x140001a2000, {0x100d12345, 0x140001b0000})
    /path/myactor.go:45 +0x1bc
```

This shows exactly which line in your code triggered the panic.

## Observer Integration

The [Observer](https://docs.ergo.services/tools/observer) tool provides a web interface for inspecting running nodes. While not strictly a debugging tool, it complements profiler-based debugging by providing:

- Real-time process list with state and mailbox sizes
- Application and supervision tree visualization
- Network topology view
- Message inspection capabilities

Observer runs at `http://localhost:9911` by default when included in your node.

## Best Practices

1. **Always use build tags in development**: Run with `--tags pprof` during development to have profiler and goroutine labels available when needed.

2. **Configure reasonable shutdown timeout**: A shorter timeout (30-60 seconds) in development helps identify stuck processes quickly.

3. **Use framework logging**: The framework's `Log()` method automatically includes PID/Alias in log output, enabling correlation with profiler data.

4. **Use structured logging**: The framework's logging system supports log levels and structured fields. Add context with `AddFields()` for correlation:

   ```go
   func (a *MyActor) HandleMessage(from gen.PID, message any) error {
       log := a.Log()
       log.AddFields(
           gen.LogField{Name: "request_id", Value: requestID},
           gen.LogField{Name: "user_id", Value: userID},
       )
       defer log.DeleteFields("request_id", "user_id")

       log.Info("processing request")
       // all log messages now include request_id and user_id
       return nil
   }
   ```

   For scoped logging, use `PushFields()`/`PopFields()` to save and restore field sets.

5. **Profile regularly**: Periodic profiling during development helps catch performance regressions before production.

6. **Test shutdown paths**: Explicitly test graceful shutdown to verify all actors terminate cleanly.

## Summary

Debugging actor systems requires tools that bridge the gap between logical actors and runtime goroutines. Ergo Framework provides this bridge through:

- **Build tags** that enable profiling and diagnostics without production overhead
- **Goroutine labels** that link runtime goroutines to their actor (PID) and meta process (Alias) identities
- **Shutdown diagnostics** that identify processes preventing clean termination
- **Observer integration** for visual inspection of running systems

Combined with Go's standard profiling tools, these capabilities enable effective debugging of even complex distributed systems.
