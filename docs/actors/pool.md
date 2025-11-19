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

The pool spawns workers during initialization. Each worker is linked to the pool (via `LinkParent: true`). If a worker crashes, the pool receives an exit signal and can restart it.

Workers are created using the `WorkerFactory`. This is the same factory pattern as regular `Spawn` - it returns a `gen.ProcessBehavior` instance. The workers can be `act.Actor`, `act.Pool` (nested pools), or custom behaviors.

### Rate Limiting Through Pool Configuration

The combination of `PoolSize` and `WorkerMailboxSize` provides a natural rate limiting mechanism. The pool can buffer at most `PoolSize × WorkerMailboxSize` messages. If all workers are busy and their mailboxes are full, new messages are rejected:

```go
// Rate limit: 5 workers × 20 messages = 100 requests max in flight
return act.PoolOptions{
    PoolSize:          5,
    WorkerMailboxSize: 20,
    WorkerFactory:     createAPIWorker,
}, nil
```

When a sender tries to send beyond this limit, they receive `ErrProcessMailboxFull` (if using important delivery) or the message is dropped with a log entry. This backpressure prevents the system from accepting more work than it can handle.

For external APIs (HTTP, gRPC), this translates to returning "503 Service Unavailable" when the pool is saturated. The pool size controls maximum concurrency, and the mailbox size controls burst capacity. Tune both based on your worker processing speed and acceptable latency.

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
        return PoolStats{
            WorkerCount: p.pool.Len(),
            Forwarded:   p.forwarded,
        }, nil
    
    default:
        p.Log().Warning("unhandled request: %T", request)
        return nil, nil  // Caller will timeout
    }
}
```

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

`RemoveWorkers` takes workers from the queue and sends them `gen.TerminateReasonNormal` via `SendExit`. The workers terminate gracefully, finishing any in-progress work before shutting down.

Both methods return the new pool size after the operation. They fail if called from outside Running state.

## Worker Restarts

Workers are linked to the pool with `LinkParent: true`. When a worker crashes, the pool receives an exit signal. The `forward` mechanism detects this (`ErrProcessUnknown` / `ErrProcessTerminated`), spawns a replacement with the same factory and arguments, and forwards the message to the new worker.

This is automatic restart, not supervision. The pool doesn't track worker history or apply restart strategies. It just replaces dead workers immediately when detected during forwarding. If you need sophisticated restart strategies, use a Supervisor to manage the pool and its workers.

## Pool Statistics

Pools expose internal metrics via `Inspect`:

```go
stats, err := node.Inspect(poolPID)
// stats contains:
// - "pool_size": configured number of workers
// - "worker_behavior": type name of worker behavior
// - "worker_mailbox_size": mailbox limit per worker
// - "worker_restarts": count of workers restarted
// - "messages_forwarded": total messages forwarded to workers
// - "messages_unhandled": messages dropped (all workers full)
```

Use this for monitoring pool health. High `messages_unhandled` indicates workers are overwhelmed. High `worker_restarts` suggests worker stability issues.

## When to Use Pools

**Use a pool when**:
- One actor is a bottleneck (mailbox growing, latency increasing)
- Work items are independent (no ordering dependencies)
- Workers are stateless or can reconstruct state cheaply

**Don't use a pool when**:
- Work items depend on previous items (pools don't guarantee ordering)
- Workers maintain critical state that can't be lost on restart
- Concurrency isn't the bottleneck (single actor is fast enough)

Pools are for horizontal scaling of stateless work. If workers need state coordination, use multiple independent actors with explicit routing instead.

## Patterns and Pitfalls

**Set WorkerMailboxSize** to limit backpressure propagation. Unbounded mailboxes let workers accumulate huge queues, hiding the overload until memory exhausts. Bounded mailboxes cause forwarding to try next worker, eventually reaching the sender with backpressure.

**Don't forward Exit signals intentionally**. The pool doesn't forward Exit messages to workers. If you need to broadcast shutdown to all workers, iterate manually and send to each worker PID.

**Monitor forwarding metrics**. If `messages_unhandled` increases, your pool is undersized or workers are too slow. Scale up with `AddWorkers` or optimize worker processing.

**Use priority for pool management**. Send management commands with `MessagePriorityHigh` to ensure they go to the pool, not forwarded to workers.

**Nested pools are possible** but rarely useful. A pool of pools adds latency without much benefit. Prefer one pool with more workers over nested layers.
