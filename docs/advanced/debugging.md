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

### The `verbose` Tag

The `verbose` tag enables verbose logging of framework internals:

```bash
go run --tags verbose ./cmd
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

### The `latency` Tag

The `latency` tag enables mailbox latency measurement for all processes:

```bash
go run --tags latency ./cmd
```

This activates:

- **Monotonic timestamp** on every message pushed into the MPSC queue
- **`QueueMPSC.Latency()`** returns the age (in nanoseconds) of the oldest unprocessed message in the queue
- **`ProcessMailbox.Latency()`** returns the maximum latency across all four mailbox queues (Main, System, Urgent, Log)
- **`MailboxLatency` field** in `ProcessShortInfo` for per-process latency snapshots
- **`Node.ProcessRangeShortInfo()`** for efficient iteration over all processes with their latency data

Without the tag, `Latency()` returns -1 (disabled) and there is zero runtime overhead: no timestamps are recorded, no atomic operations are added to the message path.

The overhead with the tag enabled is approximately 10-25% on micro-benchmarks (LOCAL 1-1 scenario with a single producer and consumer exchanging messages). In real applications with many processes, the overhead is lower because the cost is amortized across concurrent operations.

Latency measurement answers the question "how long has the oldest message been sitting in this process's mailbox?" A high value means the process is not keeping up with incoming messages: it is either overloaded, stuck in a long-running callback, or blocked. This is particularly useful for:

- Identifying backpressure in actor pipelines
- Detecting stuck processes before they cause cascading failures
- Finding hotspot processes in large clusters

For cluster-wide observability with Prometheus and Grafana, see the [Metrics actor](../extra-library/actors/metrics.md) which integrates latency data into distribution, top-N, and per-node panels when built with the `latency` tag.

### The `typestats` Tag

The `typestats` tag enables per-type encode/decode statistics:

```bash
go run --tags typestats ./cmd
```

This activates:

- **Encoded/Decoded counts** per registered EDF type for root-level operations (calls at the message boundary, not nested fields)
- **EncodedBytes/DecodedBytes** measured as decompressed wire size, pre-compression on encode and post-decompression on decode, including the type-prefix header
- **`Stats.Enabled` flag** in `gen.RegisteredTypeInfo` set to `true` to signal counters are active
- Counters visible via **`Network().RegisteredTypes()`** API and the **Observer Types panel**

Without the tag, counters remain zero, `Stats.Enabled` is `false`, and there is zero runtime overhead. Encode and decode go through pass-through wrappers that the Go inliner reduces to direct calls.

The overhead with the tag enabled is approximately 2-3% on encode/decode throughput, from two `atomic.AddInt64` operations per root call.

A counter increments only when a value of that type is the message itself, the top of an `Encode` or `Decode` call. Built-in primitives like `gen.PID`, `gen.Atom`, `gen.Ref` typically appear as fields inside other messages, so their bytes contribute to the parent message's byte total, not to their own counters. Encoded and Decoded are independent: a node may receive some types only and send others only.

Use case: identify message types that dominate network traffic. The average byte size per operation (`EncodedBytes / Encoded`) indicates whether a type is a candidate for compression at the producer process. Types with a high average are strong candidates for compressing at the source; types with a low average are not worth the framing overhead.

### Combining Tags

Tags can be combined for comprehensive debugging:

```bash
go run --tags "pprof,norecover,verbose" ./cmd
```

or with latency measurement:

```bash
go run --tags "pprof,latency" ./cmd
```

or with type statistics:

```bash
go run --tags "pprof,latency,typestats" ./cmd
```

This enables all specified features simultaneously. Use combinations when investigating complex issues that span multiple subsystems.

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

### Labels In A Plain Goroutine Dump

The `?debug=1` profile above groups goroutines by stack and prints the labels as `# labels:`
lines. A plain dump - `?debug=2`, `runtime.Stack`, or the traceback of an unrecovered panic -
is a different format, and until Go 1.27 it carried no labels at all.

Since Go 1.27 the labels are printed in the header line of every goroutine, after the state:

```
goroutine 38669 [chan receive] {pid: "<ABC123.0.1041>"}:
ergo.services/ergo/act.(*Actor).ProcessRun(0x140003c2000)
	/path/act/actor.go:259 +0x758
...

goroutine 24812 [IO wait] {meta: "Alias#<ABC123.107118.6819740677833.0>", role: reader}:
internal/poll.runtime_pollWait(0x112aec600, 0x72)
	/usr/local/go/src/runtime/netpoll.go:351 +0xa0
...
```

The runtime gates this on the `tracebacklabels` setting, whose default follows the `go` version
your module declares: a module on `go 1.21` gets the pre-1.27 behaviour and no labels. Ask for
them either with a directive in the main package:

```go
//go:debug tracebacklabels=1

package main
```

or with `GODEBUG=tracebacklabels=1` in the environment. The build tag is still required - it is
what attaches the labels in the first place; the setting only decides whether a plain dump
prints them.

This is what makes the next section work: a dump taken with `?debug=2` can be searched by PID
only when the labels are in it.

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

In production, `norecover` is not appropriate: the framework's panic recovery is what keeps the node running after a faulty callback. To surface the same panic origin without crashing the node, register the [Sentry logger](../extra-library/loggers/sentry.md) - it captures every recovered panic with its origin stack trace and forwards it to a centralized issue tracker. Recurring panics group by root cause automatically.

## Observer Integration

The [Observer](../extra-library/applications/observer.md) application embeds into a node and provides a web interface for inspecting it and the rest of the cluster. While not strictly a debugging tool, it complements profiler-based debugging by providing:

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

- **Build tags** that enable profiling, diagnostics, and latency measurement without production overhead
- **Goroutine labels** that link runtime goroutines to their actor (PID) and meta process (Alias) identities
- **Shutdown diagnostics** that identify processes preventing clean termination
- **Observer integration** for visual inspection of running systems

Combined with Go's standard profiling tools, these capabilities enable effective debugging of even complex distributed systems.

