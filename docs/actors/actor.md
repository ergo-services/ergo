# Actor

The actor model requires sequential message processing - each actor handles one message at a time in a dedicated goroutine. This eliminates data races within the actor but shifts complexity to the message handling loop: reading from multiple mailbox queues in priority order, dispatching to different handlers based on message type, managing state transitions, converting exit signals to regular messages when trapping is enabled.

You could implement this yourself with `gen.ProcessBehavior`, but you'd rewrite the same logic for every actor. `act.Actor` solves this. It implements the low-level `gen.ProcessBehavior` interface and provides a higher-level `act.ActorBehavior` interface with straightforward callbacks: `Init` for initialization, `HandleMessage` for asynchronous messages, `HandleCall` for synchronous requests, `Terminate` for cleanup. You write business logic, `act.Actor` handles the mailbox mechanics.

## Creating an Actor

Embed `act.Actor` in your struct and implement the `act.ActorBehavior` callbacks you need:

```go
type Worker struct {
    act.Actor
    counter int
}

func (w *Worker) Init(args ...any) error {
    w.counter = 0
    w.Log().Info("worker %s starting", w.PID())
    return nil
}

func (w *Worker) HandleMessage(from gen.PID, message any) error {
    switch msg := message.(type) {
    case IncrementRequest:
        w.counter += msg.Amount
        w.Send(from, IncrementResponse{Counter: w.counter})
    }
    return nil
}

func (w *Worker) Terminate(reason error) {
    w.Log().Info("worker stopped: %s", reason)
}

// Factory function for spawning
func createWorker() gen.ProcessBehavior {
    return &Worker{}
}
```

Spawn it like any process:

```go
pid, err := node.Spawn(createWorker, gen.ProcessOptions{})
```

The factory function is called each time you spawn. Each process gets a fresh instance with its own state. This isolation is fundamental to the actor model - actors share nothing except messages.

## Callback Interface

`act.ActorBehavior` defines the callbacks `act.Actor` will invoke:

```go
type ActorBehavior interface {
    gen.ProcessBehavior
    
    // Core lifecycle
    Init(args ...any) error
    HandleMessage(from gen.PID, message any) error
    HandleCall(from gen.PID, ref gen.Ref, request any) (any, error)
    Terminate(reason error)
    
    // Split handle callbacks (opt-in via SetSplitHandle)
    HandleMessageName(name gen.Atom, from gen.PID, message any) error
    HandleMessageAlias(alias gen.Alias, from gen.PID, message any) error
    HandleCallName(name gen.Atom, from gen.PID, ref gen.Ref, request any) (any, error)
    HandleCallAlias(alias gen.Alias, from gen.PID, ref gen.Ref, request any) (any, error)
    
    // Specialized callbacks
    HandleLog(message gen.MessageLog) error
    HandleEvent(message gen.MessageEvent) error
    HandleInspect(from gen.PID, item ...string) map[string]string
}
```

All callbacks are optional. `act.Actor` provides default implementations that log warnings for unhandled messages. Implement only what you need.

Since `act.Actor` embeds `gen.Process`, you have direct access to all process methods: `Send`, `Call`, `Spawn`, `Link`, `RegisterName`, etc. No need to store references - they're built in.

## Initialization

`Init` runs once when the process spawns. The `args` parameter contains whatever you passed to `Spawn`:

```go
pid, err := node.Spawn(createWorker, gen.ProcessOptions{}, "config", 42)

// In your actor:
func (w *Worker) Init(args ...any) error {
    if len(args) > 0 {
        w.config = args[0].(string)
    }
    if len(args) > 1 {
        w.maxCount = args[1].(int)
    }
    return nil
}
```

If `Init` returns an error, the process is cleaned up and removed. `Spawn` returns immediately with that error. Use this for validation: check arguments, verify resources, refuse to start if preconditions aren't met.

During `Init`, the process is in `ProcessStateInit`. All operations are available: `Spawn`, `Send`, `SetEnv`, `RegisterName`, `CreateAlias`, `RegisterEvent`, `Link*`, `Monitor*`, `Call*`, and property setters.

Any resources created during Init (names, aliases, events, links, monitors) are properly cleaned up if initialization fails.

## Message Handling

Messages arrive in the mailbox and sit in one of four queues: Urgent, System, Main, or Log. `act.Actor` processes them in priority order:

1. **Urgent** - Maximum priority messages (`MessagePriorityMax`)
2. **System** - High priority messages (`MessagePriorityHigh`)
3. **Main** - Normal priority messages (`MessagePriorityNormal`, default)
4. **Log** - Logging messages (lowest priority)

When a message arrives in Urgent, System, or Main, `act.Actor` calls `HandleMessage`:

```go
func (w *Worker) HandleMessage(from gen.PID, message any) error {
    switch msg := message.(type) {
    case WorkRequest:
        result := w.process(msg)
        w.Send(from, WorkResponse{Result: result})
    
    case StatusQuery:
        w.Send(from, StatusResponse{Status: w.status})
    
    case StopCommand:
        return gen.TerminateReasonNormal  // Terminate gracefully
    }
    
    return nil  // Continue running
}
```

The return value determines whether the actor continues or terminates:
- Return `nil` to keep running
- Return `gen.TerminateReasonNormal` for clean shutdown
- Return any other error to terminate (logged as error)

The `from` parameter tells you who sent the message. Use it for replies. If you don't need replies, ignore it.

## Synchronous Requests

When someone calls `process.Call(pid, request)`, `act.Actor` invokes your `HandleCall`:

```go
func (w *Worker) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
    switch req := request.(type) {
    case GetCounterRequest:
        return CounterResponse{Counter: w.counter}, nil
    
    case ResetCounterRequest:
        old := w.counter
        w.counter = 0
        return ResetResponse{OldValue: old}, nil
    
    default:
        w.Log().Warning("unknown request type: %T from %s", request, from)
        return nil, nil  // Don't respond to unknown requests
    }
}
```

The `error` return value controls process termination, not the caller's response:
- `(result, nil)` - Send `result` to caller, continue running
- `(result, gen.TerminateReasonNormal)` - Send `result`, then terminate cleanly
- `(nil, someError)` - Terminate immediately with `someError` (caller times out)

To send an application error to the caller, return it as the result value:

```go
func (w *Worker) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
    switch req := request.(type) {
    case DivideRequest:
        if req.Divisor == 0 {
            return fmt.Errorf("division by zero"), nil
        }
        return req.Dividend / req.Divisor, nil
    }
    
    w.Log().Warning("unknown request type: %T from %s", request, from)
    return nil, nil
}

// Caller side:
result, err := process.Call(workerPID, DivideRequest{10, 0})
if err != nil {
    // Framework error (timeout, process unknown, etc.)
    log.Printf("call failed: %s", err)
    return
}

if e, ok := result.(error); ok {
    // Application error returned by HandleCall
    log.Printf("operation failed: %s", e)
    return
}

// Success - use result
log.Printf("result: %v", result)
```

This separation between transport errors (`err` return from `Call`) and application errors (`result` as error) is fundamental to actor communication. See [Handle Sync](../advanced/handle-sync.md#sendresponse-vs-sendresponseerror-two-channels-for-results) for deeper discussion of error channels and when to use `SendResponseError`.

### Asynchronous Handling of Synchronous Requests

Sometimes you can't respond immediately. Maybe you need to query another service, or delegate work to a pool of workers. Return `(nil, nil)` from `HandleCall` to defer the response:

```go
func (w *Worker) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
    switch req := request.(type) {
    case ExpensiveQuery:
        // Send to worker pool
        w.Send(w.workerPool, PoolRequest{
            Query:  req,
            Caller: from,
            Ref:    ref,
        })
        // Return nil, nil to handle asynchronously
        return nil, nil
    }
    return nil, nil
}

// Later, when the worker pool replies:
func (w *Worker) HandleMessage(from gen.PID, message any) error {
    switch msg := message.(type) {
    case PoolResponse:
        // Send response to original caller
        w.SendResponse(msg.Caller, msg.Ref, msg.Result)
    }
    return nil
}
```

The `gen.Ref` identifies the request. The caller blocks waiting for a response with that ref. You can send the response from any process - the one that received the request, a worker, or even a remote process. Just call `SendResponse(callerPID, ref, result)`.

The ref has a deadline (from the caller's timeout). Check if it's still alive before doing expensive work:

```go
if !ref.IsAlive() {
    w.Log().Warning("caller timed out, discarding work")
    return nil
}
```

## Termination

To stop an actor, return a non-nil error from `HandleMessage` or `HandleCall`:

```go
func (w *Worker) HandleMessage(from gen.PID, message any) error {
    switch message.(type) {
    case ShutdownCommand:
        return gen.TerminateReasonNormal  // Clean shutdown
    
    case PanicCommand:
        return fmt.Errorf("intentional failure")  // Error shutdown
    }
    return nil
}
```

Termination reasons:
- `gen.TerminateReasonNormal` - Clean shutdown, not logged as error
- `gen.TerminateReasonKill` - Process was killed via `node.Kill(pid)`
- `gen.TerminateReasonPanic` - Panic occurred in callback (framework catches it)
- `gen.TerminateReasonShutdown` - Node is stopping (sent by parent or node)
- Any other error - Application-specific failure (logged as error)

After termination is triggered, `act.Actor` calls your `Terminate` callback:

```go
func (w *Worker) Terminate(reason error) {
    w.Log().Info("worker %s stopping: %s", w.PID(), reason)
    // Clean up resources
    w.closeConnections()
    w.sendFinalStats()
}
```

At this point, the process is in `ProcessStateTerminated` and has been removed from the node. Most `gen.Process` methods return `gen.ErrNotAllowed`. You can still send messages (fire-and-forget), but you can't make calls, create links, or spawn children.

If a panic occurs during `Init`, `HandleMessage`, or `HandleCall`, the framework catches it, logs the stack trace, and terminates the process with `gen.TerminateReasonPanic`. The `Terminate` callback still runs, giving you a chance to clean up.

## Trapping Exit Signals

By default, when an actor receives an exit signal (via `SendExit` or from a linked process), it terminates immediately. Enable `TrapExit` to convert exit signals into regular messages:

```go
func (w *Worker) Init(args ...any) error {
    w.SetTrapExit(true)
    return nil
}

func (w *Worker) HandleMessage(from gen.PID, message any) error {
    switch msg := message.(type) {
    case gen.MessageExitPID:
        w.Log().Info("linked process %s terminated: %s", msg.PID, msg.Reason)
        // Decide how to handle it
        if msg.Reason == gen.TerminateReasonPanic {
            // Linked worker panicked, maybe restart it
            w.restartWorker(msg.PID)
        }
        // Don't terminate - we're trapping
        return nil
    
    case gen.MessageExitNode:
        w.Log().Warning("node %s disconnected", msg.Name)
        // Handle network partition
        return nil
    }
    return nil
}
```

Exit signal messages:
- `gen.MessageExitPID` - From a process (`SendExit` or link)
- `gen.MessageExitProcessID` - From a named process link
- `gen.MessageExitAlias` - From an alias link
- `gen.MessageExitEvent` - From an event link
- `gen.MessageExitNode` - From a node link (network disconnect)

**Exception**: Exit signals from the parent process **cannot** be trapped. If your parent terminates (and you created a link with `LinkParent` option or via `Link`/`LinkPID`), you terminate regardless of `TrapExit`. This ensures supervision trees can forcefully terminate subtrees.

Use `TrapExit` when you want to handle failures gracefully - log them, restart workers, switch to fallback services. Don't use it if you want standard supervision behavior (child fails → parent restarts it).

## Split Handle

By default, `HandleMessage` and `HandleCall` are invoked regardless of how the process was addressed - by PID, by registered name, or by alias. Enable `SetSplitHandle(true)` to route based on address type:

```go
func (w *Worker) Init(args ...any) error {
    w.SetSplitHandle(true)
    w.RegisterName("worker_service")
    alias, _ := w.CreateAlias()
    w.publicAPI = alias
    return nil
}

func (w *Worker) HandleMessage(from gen.PID, message any) error {
    // Messages sent to PID directly (internal use)
    w.Log().Debug("internal message from %s", from)
    return nil
}

func (w *Worker) HandleMessageName(name gen.Atom, from gen.PID, message any) error {
    // Messages sent to registered name "worker_service" (public API)
    w.Log().Info("public API call via name %s", name)
    return nil
}

func (w *Worker) HandleMessageAlias(alias gen.Alias, from gen.PID, message any) error {
    // Messages sent to alias (temporary session)
    w.Log().Debug("session message via alias %s", alias)
    return nil
}
```

The same split applies to `HandleCall*` variants. Use this when you want different behavior for internal communication (PID) versus public API (registered name) versus temporary sessions (alias).

Most actors don't need this. Leave split handle disabled and use `HandleMessage`/`HandleCall` for everything.

## Specialized Callbacks

### Logging

If your actor is registered as a logger (via `node.AddLogger(pid, level)`), it receives log messages in the Log queue:

```go
func (w *Worker) HandleLog(message gen.MessageLog) error {
    // Format and write log message
    fmt.Printf("[%s] %s: %s\n", message.Level, message.PID, message.Message)
    return nil
}
```

Log messages have the lowest priority. They're processed after Urgent, System, and Main are empty. This prevents logging from starving regular message processing.

### Events

If your actor subscribed to an event (via `LinkEvent` or `MonitorEvent`), it receives event messages:

```go
func (w *Worker) HandleEvent(message gen.MessageEvent) error {
    switch message.Name {
    case "config_updated":
        w.reloadConfig()
    case "cache_invalidated":
        w.clearCache()
    }
    return nil
}
```

Events arrive in the System queue (high priority). Use them for cross-cutting concerns where multiple actors need to react to the same occurrence.

### Inspection

Actors can expose runtime state for monitoring and debugging via the `HandleInspect` callback:

```go
func (w *Worker) HandleInspect(from gen.PID, item ...string) map[string]string {
    return map[string]string{
        "counter":     fmt.Sprintf("%d", w.counter),
        "status":      w.status,
        "queue_depth": fmt.Sprintf("%d", w.queueDepth),
    }
}
```

Inspect the actor from within a process context or directly from the node:

```go
// From within another process
info, err := process.Inspect(workerPID)

// Directly from the node
info, err := node.Inspect(workerPID)
```

Both methods only work for local processes (same node). Inspection requests go to the Urgent queue and bypass normal message processing. Keep `HandleInspect` implementation fast - don't do expensive computations or I/O. Return only string values (serialization limitation). The optional `item` parameters allow filtering which fields to return, though most implementations ignore them and return all fields.

## Actor Pools

For workload distribution, use `act.Pool` instead of implementing manual worker management. See [Pool](pool.md) for details.

## Patterns and Pitfalls

**Don't spawn goroutines in callbacks**. The actor model is sequential - one message at a time. Spawning goroutines breaks this, introducing data races on actor state. If you need concurrency, spawn child actors and send them messages.

**Don't block on channels or mutexes**. Callbacks run in the actor's goroutine. Blocking it starves message processing. Use async message passing (`Send`) instead of sync primitives.

**Don't store `gen.Process` references**. The embedded `act.Actor` provides all process methods. Storing additional references wastes memory and can cause confusion about which instance is authoritative.

**Return errors for termination, not for caller responses**. `HandleCall`'s error return terminates the process. To send errors to callers, return them as the result value.

**Use `ref.IsAlive()` before expensive async work**. When handling calls asynchronously, check if the caller is still waiting before spending resources on the response.

**Enable `TrapExit` only when needed**. Default behavior (terminate on exit signal) works for most actors. Trap only when you have specific failure handling logic.
