# Pool

Suppose an actor processes independent jobs, and each job takes longer than the interval between arrivals. Its mailbox starts to grow. Running several copies of that worker lets jobs proceed concurrently, provided they do not depend on one another's state or completion order.

`act.Pool` forwards normal-priority traffic to a group of workers created from the same factory. Workers receive the original sender and reply directly to it. A worker that has died is replaced when the pool next attempts to forward to it.

We will configure a pool, follow a request through it, and then examine what happens when workers stop or their mailboxes fill.

## Creating a Pool

Like `act.Actor` provides callbacks for regular actors, `act.Pool` uses the `act.PoolBehavior` interface:

```go
type PoolBehavior interface {
    gen.ProcessBehavior
    
    Init(args ...any) (PoolOptions, error)
    
    HandleMessage(from gen.PID, message any) error
    HandleCall(from gen.PID, ref gen.Ref, request any) (any, error)
    HandleForwardFailed(from gen.PID, message any, reason error) error
    Terminate(reason error)
    
    HandleEvent(message gen.MessageEvent) error
    HandleInspect(from gen.PID, item ...string) map[string]string
}
```

The key difference from `ActorBehavior`: `Init` returns `PoolOptions` that define the pool configuration. All callbacks are optional except `Init`.

Embed `act.Pool` in your struct and implement `Init` to configure workers:

```go
type WorkerPool struct {
    act.Pool
}

func (p *WorkerPool) Init(args ...any) (act.PoolOptions, error) {
    return act.PoolOptions{
        PoolSize:          5,                    // 5 workers
        WorkerFactory:     createWorker,         // Factory for workers
        WorkerMailboxSize: 100,                  // Limit each worker to 100 messages
        WorkerArgs:        []any{"config"},      // Args passed to worker Init
    }, nil
}

func createPoolFactory() gen.ProcessBehavior {
    return &WorkerPool{}
}

// Spawn the pool
poolPID, err := node.Spawn(createPoolFactory, gen.ProcessOptions{})
```

`PoolSize` below 1 is not an error and not "no workers": `ProcessInit` substitutes the default of **3**, and the substituted value is what `ergo:pool_size` then reports. `WorkerFactory` is the field with no default: leave it nil and the first worker spawn fails, `ProcessInit` returns that error, and the pool does not start.

The pool spawns workers during initialization with `LinkParent: true`. That link runs one way, from worker to pool: if the **pool** terminates, its workers get an exit signal and go with it. The reverse is not true - a crashing worker sends the pool nothing.

The pool notices a dead worker lazily instead. When it forwards a message and the target answers `gen.ErrProcessUnknown` or `gen.ErrProcessTerminated`, it spawns a replacement and forwards the message there. So a worker that dies while the pool is idle is replaced on the next message addressed to it, not at the moment of death.

Workers are created using the `WorkerFactory`. This is the same factory pattern as regular `Spawn` - it returns a `gen.ProcessBehavior` instance. The workers can be `act.Actor`, `act.Pool` (nested pools), or custom behaviors.

### Rate Limiting Through Pool Configuration

Workers can hold `PoolSize` active messages plus `PoolSize × WorkerMailboxSize` queued messages when the worker mailbox limit is set. The pool also has its own process mailbox, governed by its `gen.ProcessOptions`. The worker capacity is therefore not a bound on the entire entry point. Once every worker mailbox is full, forwarding fails and the pool drops the message:

```go
// Worker queues: up to 100 messages, plus active work and the pool mailbox
return act.PoolOptions{
    PoolSize:          5,
    WorkerMailboxSize: 20,
    WorkerFactory:     createAPIWorker,
}, nil
```

On a failed forward, the pool increments `ergo:messages_unhandled`, calls `HandleForwardFailed` and releases the message. The original `Send` may already have returned nil after accepting it into the pool mailbox. A waiting `Call` receives the forwarding error before the callback runs.

The worker limits do not provide admission feedback to an asynchronous sender. To return an HTTP 503 on overload, arrange a response from `HandleForwardFailed` or gate admission before sending to the pool. Watch the drop counter while tuning concurrency and queue capacity.

Consider five workers with mailbox limits of 20. A sender first queues a job in the pool mailbox. The pool later takes that job and tries the worker mailboxes. If all five are full at that moment, the forwarding attempt fails even though the sender's earlier `Send` succeeded. Limiting the pool mailbox as well bounds that first queue; it still does not turn submission into confirmation that a worker took the job.

For a request made with `Call`, the pool can return a forwarding error to the waiting caller. For a plain message, the sender needs an application acknowledgment or another admission mechanism if it must learn that error. This distinction is useful when choosing between fire-and-forget work and a request whose result controls an HTTP response.

## Automatic Message Distribution

When you send a message or make a call to the pool, `act.Pool` automatically forwards it to an available worker:

```go
// Send a message to the pool
process.Send(poolPID, WorkRequest{Data: "task1"})

// The pool forwards to a worker transparently
// The worker's HandleMessage receives it
```

Forwarding happens for messages in the Main queue (normal priority). The pool maintains a FIFO queue of worker PIDs. When a message arrives:

1. **Pop a worker** from the queue
2. **Forward the message** using `Forward` (preserves original sender and ref)
3. **Check result**:
   - Success → push worker back to queue
   - `ErrProcessUnknown` / `ErrProcessTerminated` → spawn replacement, forward to it
   - `ErrProcessMailboxFull` → push worker back, try next worker
4. **Repeat** until successful or all workers tried

The message has already left the pool's mailbox when this forwarding loop runs. If no worker accepts it, the pool does not put it back to wait for space: it calls the failure callback and releases it. Other messages may still be queued in the pool's own mailbox. [When No Worker Takes the Message](#when-no-worker-takes-the-message) explains how to report or retain the failed work.

The pool forwards Regular messages, Requests, and Events. Exit signals and Inspect requests are handled by the pool itself (they're not forwarded to workers).

## When No Worker Takes the Message

`HandleForwardFailed` receives the original sender, the message value and the forwarding error. When every worker mailbox is full, that error is `gen.ErrProcessMailboxFull`; when the worker queue is empty, it is `gen.ErrProcessUnknown`. A replacement worker can also fail to initialize, so handle other returned errors. The pool releases the mailbox envelope after the callback returns.

```go
func (p *WorkerPool) HandleForwardFailed(from gen.PID, message any, reason error) error {
    p.Log().Warning("dropped %T from %s: %s", message, from, reason)
    p.Send(p.deadletter, message)
    return nil
}
```

Returning a non-nil error from the callback terminates the pool, the same as from any other callback. The default implementation, which you get by not writing one, logs the drop as an error and keeps the pool running.

A failed forward of a `Call` request prompts the pool to send that forwarding error to the caller before invoking `HandleForwardFailed`. On a working connection this avoids waiting for the call deadline. That error response can itself fail to arrive, so the caller must still handle a timeout. The failure callback receives no request reference for a second response.

## Workers and the Original Sender

Workers receive the original sender's PID, not the pool's PID. When a worker processes a forwarded message, `from` points to whoever sent to the pool:

```go
// Sender
process.Send(poolPID, "hello")

// Worker's HandleMessage
func (w *Worker) HandleMessage(from gen.PID, message any) error {
    // 'from' is the original sender's PID, not the pool's PID
    w.Send(from, "reply")  // Reply goes to original sender
    return nil
}
```

The same applies to `Call` requests. Workers see the original caller's `from` and `ref`. When they return a result or call `SendResponse`, it goes directly to the original caller, bypassing the pool entirely.

The worker doesn't know it's part of a pool. It processes messages as if they were sent directly to it.

## Intercepting Pool Messages

Automatic forwarding applies only to the Main queue (normal priority). Urgent and System queues are handled by the pool itself through `HandleMessage` and `HandleCall` callbacks:

```go
// Normal priority - forwarded to worker automatically
process.Send(poolPID, WorkRequest{})

// High priority - handled by pool's HandleMessage
process.SendWithPriority(poolPID, ManagementCommand{}, gen.MessagePriorityHigh)

// Pool's HandleMessage - invoked for Urgent/System messages
func (p *WorkerPool) HandleMessage(from gen.PID, message any) error {
    switch msg := message.(type) {
    case ManagementCommand:
        count, _ := p.AddWorkers(msg.AdditionalWorkers)
        p.Log().Info("scaled to %d workers", count)
    
    default:
        p.Log().Warning("unhandled message: %T", message)
    }
    return nil
}
```

The same for synchronous requests:

```go
// Normal priority - forwarded to worker
result, err := process.Call(poolPID, WorkRequest{})

// High priority - handled by pool's HandleCall
stats, err := process.CallWithPriority(poolPID, GetPoolStatsRequest{}, gen.MessagePriorityHigh)

// Pool's HandleCall - invoked for Urgent/System requests
func (p *WorkerPool) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
    switch req := request.(type) {
    case GetPoolStatsRequest:
        // The pool's own counters are unexported. Read them through the
        // inspect callback, where they are published as strings.
        info := p.HandleInspect(from)
        return PoolStats{
            ConfiguredSize: info["ergo:pool_size"],
            Forwarded: info["ergo:messages_forwarded"],
        }, nil

    default:
        p.Log().Warning("unhandled request: %T", request)
        return nil, nil  // Caller will timeout
    }
}
```

`act.Pool` keeps its counters in unexported fields, so an embedding type cannot read `p.pool.Len()` or `p.forwarded` from its own package - that does not compile. The published route is the inspect callback, whose keys are listed below.

**Important**: High-priority requests that return `(nil, nil)` from `HandleCall` are **not** forwarded to workers. They're simply ignored, and the caller times out. Forwarding only happens for Main queue messages. If you want a request to be handled, either:
- Send it with normal priority (goes to workers)
- Handle it explicitly in pool's `HandleCall` and return a result

Use high priority only for pool management that should be handled by the pool itself, not for work that should go to workers.

## Dynamic Pool Management

Adjust the pool size at runtime with `AddWorkers` and `RemoveWorkers`:

```go
func (p *WorkerPool) HandleMessage(from gen.PID, message any) error {
    switch msg := message.(type) {
    case ScaleUpCommand:
        newSize, err := p.AddWorkers(msg.Count)
        if err != nil {
            p.Log().Error("failed to add workers: %s", err)
            return nil
        }
        p.Log().Info("scaled up to %d workers", newSize)
    
    case ScaleDownCommand:
        newSize, err := p.RemoveWorkers(msg.Count)
        if err != nil {
            p.Log().Error("failed to remove workers: %s", err)
            return nil
        }
        p.Log().Info("scaled down to %d workers", newSize)
    }
    return nil
}
```

`AddWorkers` spawns new workers with the same factory and options used during initialization. They're added to the FIFO queue and immediately available for work.

`RemoveWorkers` takes workers from the queue and sends them `gen.TerminateReasonNormal` via `SendExit`. That exit goes to the **Urgent** queue, and every run loop drains Urgent before System before Main, so a removed worker stops at its next dispatch: the message it is handling now finishes, and everything already queued behind it in Main does not. `SetTrapExit` does not soften this either - the trap does not apply to a PID exit from the worker's parent, and here the parent is the pool that sent it.

If in-flight work must not be lost, drain before removing: stop feeding the pool, wait for the workers' mailboxes to empty, then call `RemoveWorkers`.

Both methods return the new pool size after the operation. They fail if called from outside Running state.

## Worker Restarts

The worker-to-parent link stops a worker when its pool stops. The reverse link is absent: worker death does not notify the pool. Forwarding to a missing or terminated worker triggers a replacement with the same factory and arguments.

An idle pool can retain a dead worker PID until the next forward attempt. It has no restart budget or supervisor strategy. When those policies matter, use externally supervised workers with a [Router](router.md), or choose a supervision arrangement explicitly.

## Pool Statistics

Pools expose internal metrics via `Inspect`:

```go
stats, err := node.Inspect(poolPID)
// stats contains:
// - "ergo:pool_size": configured number of workers
// - "ergo:worker_behavior": type name of worker behavior
// - "ergo:worker_mailbox_size": mailbox limit per worker
// - "ergo:worker_restarts": count of workers restarted
// - "ergo:messages_forwarded": total messages forwarded to workers
// - "ergo:messages_unhandled": messages dropped (no worker took them)
```

`ergo:pool_size` reports the initial normalized configuration, not the current worker count after `AddWorkers` or `RemoveWorkers`. Use the counts returned by those methods when tracking dynamic size.

All of these keys use the reserved `ergo:` prefix. A `HandleInspect` you implement is merged on top of them, so your fields are added beside these rather than replacing the set - and one of these is overridden only if you name it with the prefix.

Use this for monitoring pool health. High `ergo:messages_unhandled` indicates workers are overwhelmed. High `ergo:worker_restarts` suggests worker stability issues.

`ProcessOptions.Fallback` applies on the ordinary send-routing path. Pool forwarding pushes directly to a worker queue and does not consult it. The pool also constructs worker options itself and exposes no fallback field in `PoolOptions`. Handle failed forwards, enlarge worker capacity or limit incoming work before the pool.

## When to Use Pools

**Use a pool when**:
- One actor is a bottleneck (mailbox growing, latency increasing)
- Work items are independent (no ordering dependencies)
- Workers are stateless or can reconstruct state cheaply

**Don't use a pool when**:
- Work items depend on previous items (pools don't guarantee ordering)
- Workers maintain critical state that can't be lost on restart
- Concurrency isn't the bottleneck (single actor is fast enough)

Pools are for horizontal scaling of stateless work. If workers need state coordination, message-type dispatching, or key affinity, use [Router](router.md) instead - it owns named slots and lets user code decide where each message goes.

## Patterns and Pitfalls

**Set WorkerMailboxSize**. Unbounded mailboxes let workers accumulate huge queues, hiding the overload until memory exhausts. A bounded mailbox makes forwarding try the next worker, and a message nobody takes is dropped and counted.

**Don't forward Exit signals intentionally**. The pool doesn't forward Exit messages to workers. If you need to broadcast shutdown to all workers, iterate manually and send to each worker PID.

**Monitor forwarding metrics**. If `ergo:messages_unhandled` increases, your pool is undersized or workers are too slow. Scale up with `AddWorkers` or optimize worker processing.

**Use priority for pool management**. Send management commands with `MessagePriorityHigh` to ensure they go to the pool, not forwarded to workers.

**Nested pools are possible** but rarely useful. A pool of pools adds latency without much benefit. Prefer one pool with more workers over nested layers.
