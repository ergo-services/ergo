# Pool

A single actor processes messages sequentially. This is fundamental to the actor model - it eliminates race conditions and makes reasoning about state straightforward. But it also means one actor can become a bottleneck. If messages arrive faster than the actor can process them, the mailbox grows, latency increases, and eventually the system stalls.

The standard solution is to run multiple workers. Instead of sending requests to one actor, distribute them across several identical actors processing in parallel. This works, but now you need routing logic: pick a worker, check if it's alive, handle mailbox overflow, restart dead workers. This boilerplate appears in every pool implementation.

`act.Pool` solves this. It's an actor that manages a pool of worker actors and automatically distributes incoming messages and requests across them. You send to the pool's PID, the pool forwards to an available worker. The pool handles worker lifecycle, automatic restarts, and load balancing. From the sender's perspective, it's just one actor. Under the hood, it's N workers processing in parallel.

## Creating a Pool

Like `act.Actor` provides callbacks for regular actors, `act.Pool` uses the `act.PoolBehavior` interface:

```go
type PoolBehavior interface {
    gen.ProcessBehavior
    
    Init(args ...any) (PoolOptions, error)
    
    HandleMessage(from gen.PID, message any) error
    HandleCall(from gen.PID, ref gen.Ref, request any) (any, error)
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

The combination of `PoolSize` and `WorkerMailboxSize` bounds how much work the pool holds: `PoolSize` messages being handled, plus `PoolSize × WorkerMailboxSize` waiting in the workers' mailboxes. A message being handled has already left its mailbox, so the two add up. There is no buffer at the pool itself. Once every mailbox is full, further messages are **dropped** rather than rejected - the sender is not told, as the next section explains:

```go
// Rate limit: 5 workers × 20 messages = 100 requests max in flight
return act.PoolOptions{
    PoolSize:          5,
    WorkerMailboxSize: 20,
    WorkerFactory:     createAPIWorker,
}, nil
```

That product is the work in flight the pool can hold. Past it the message is **dropped**: the pool logs an error, increments `ergo:messages_unhandled` and releases the message. The sender is not told. `ErrProcessMailboxFull` comes from a target's own queue on the ordinary send path, and the pool's forwarding is not that path - a `Send` to a saturated pool returns `nil`, and a `Call` ends in the caller's own timeout with no indication of the cause.

So this is a limit, not backpressure. If an external API is to answer "503 Service Unavailable" when the pool is saturated, that decision has to be made before the pool: check `ergo:messages_unhandled` from the inspect callback, or gate admission in the handler. The pool size controls maximum concurrency and the mailbox size controls burst capacity - tune both against worker processing speed and acceptable latency, and treat a rising drop counter as the signal that the sizing is wrong.

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

If all workers have full mailboxes, the message is dropped and logged. The pool doesn't have its own buffer beyond the workers' mailboxes. This is intentional - backpressure should propagate to senders.

The pool forwards Regular messages, Requests, and Events. Exit signals and Inspect requests are handled by the pool itself (they're not forwarded to workers).

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

This is why forwarding is transparent. The worker doesn't know it's part of a pool. It processes messages as if they were sent directly to it.

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
            Size:      info["ergo:pool_size"],
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

`RemoveWorkers` takes workers from the queue and sends them `gen.TerminateReasonNormal` via `SendExit`. That exit goes to the **Urgent** queue, and every run loop drains Urgent before System before Main, so a removed worker stops at its next dispatch: the message it is handling now finishes, and everything already queued behind it in Main does not. `SetTrapExit` does not soften this either - the trap only applies to an exit from the worker's parent, and here the parent is the pool that sent it.

If in-flight work must not be lost, drain before removing: stop feeding the pool, wait for the workers' mailboxes to empty, then call `RemoveWorkers`.

Both methods return the new pool size after the operation. They fail if called from outside Running state.

## Worker Restarts

Workers are spawned with `LinkParent: true`, which links them to the pool and not the pool to them - a crashing worker sends the pool no signal at all. Detection happens in the `forward` path instead: the pool pops a worker, forwards, and if the answer is `ErrProcessUnknown` or `ErrProcessTerminated` it spawns a replacement with the same factory and arguments and forwards the message to the new worker.

This is automatic restart, not supervision. The pool doesn't track worker history or apply restart strategies, and it does not learn of a death until it next tries to use that worker - a pool sitting idle keeps a dead PID in its queue until the next message. If you need sophisticated restart strategies, use a Supervisor to manage the pool and its workers.

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
// - "ergo:messages_unhandled": messages dropped (all workers full)
```

All of these keys use the reserved `ergo:` prefix. A `HandleInspect` you implement is merged on top of them, so your fields are added beside these rather than replacing the set - and one of these is overridden only if you name it with the prefix.

Use this for monitoring pool health. High `ergo:messages_unhandled` indicates workers are overwhelmed. High `ergo:worker_restarts` suggests worker stability issues.

`ProcessOptions.Fallback` does not help here, though it is the natural thing to reach for. Two reasons, either of which is enough. `PoolOptions` carries only `PoolSize`, `WorkerMailboxSize`, `WorkerFactory` and `WorkerArgs` - the pool builds its workers' `ProcessOptions` itself and sets no fallback, and there is no runtime setter for one. And the pool delivers with `Forward`, which pushes onto the worker's queue directly and answers `gen.ErrProcessMailboxFull`; the fallback is consulted only on the ordinary routing path that `Send` takes. A message the pool cannot place is dropped and counted, never diverted. The remedies are `AddWorkers`, a larger `WorkerMailboxSize`, or shedding load before the pool.

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

**Set WorkerMailboxSize** to limit backpressure propagation. Unbounded mailboxes let workers accumulate huge queues, hiding the overload until memory exhausts. Bounded mailboxes cause forwarding to try next worker, eventually reaching the sender with backpressure.

**Don't forward Exit signals intentionally**. The pool doesn't forward Exit messages to workers. If you need to broadcast shutdown to all workers, iterate manually and send to each worker PID.

**Monitor forwarding metrics**. If `ergo:messages_unhandled` increases, your pool is undersized or workers are too slow. Scale up with `AddWorkers` or optimize worker processing.

**Use priority for pool management**. Send management commands with `MessagePriorityHigh` to ensure they go to the pool, not forwarded to workers.

**Nested pools are possible** but rarely useful. A pool of pools adds latency without much benefit. Prefer one pool with more workers over nested layers.
