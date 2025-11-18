---
description: Handling synchronous requests in the asynchronous actor model
---

# Sync Request Handling

The actor model is fundamentally asynchronous. Processes send messages and continue immediately without waiting for responses. This asynchrony is core to the model - actors don't block, they process messages one at a time from their mailbox, and they scale because thousands of actors can run concurrently without threads blocking on I/O or responses.

But real systems often need synchronous patterns. A client makes a request and must wait for a response before continuing. An HTTP handler receives a request and can't return to the client until the response is ready. A database query needs to block until the data arrives. These synchronous requirements don't disappear just because your system uses actors.

The challenge is satisfying these synchronous requirements without actually blocking the actor. If an actor blocks waiting for a response, it can't process other messages in its mailbox. The actor becomes unresponsive to everything else. This defeats the purpose of the actor model - you want concurrent message processing, not sequential blocking.

This chapter explores how to handle synchronous-style requests while maintaining asynchronous actor behavior. You'll learn how the framework implements request-response, how to handle Call requests efficiently, and how to process them asynchronously even when the caller is blocked waiting.

## The Nature of Synchronous Calls in Actors

In traditional synchronous code, when you call a function, you wait for it to return:

```go
result := database.Query("SELECT * FROM users")
// blocked here until query completes
processResult(result)
```

The calling thread stops. The operating system schedules other threads. Eventually the query completes, the thread wakes up, and execution continues. This is fine when you have many threads - some block, others run. But it's wasteful, and it doesn't scale to tens of thousands of concurrent operations.

In the actor model, you send a message and continue:

```go
database.Send(QueryRequest{SQL: "SELECT * FROM users"})
// immediately continues, doesn't wait
doOtherWork()
```

The sender doesn't block. The message goes into the database actor's mailbox. When the database actor processes it, it sends a response message back. The original sender handles that response later in its own message loop. This is how actors achieve massive concurrency - no actor ever blocks waiting, so you can run thousands of actors with a small thread pool.

But what if the sender legitimately needs to wait? What if it's an HTTP handler that can't return to the client until the query completes?

The framework provides `Call` for this:

```go
result, err := process.Call(databasePID, QueryRequest{SQL: "SELECT * FROM users"})
// blocked here, but only this actor is blocked
// other actors continue running normally
```

From the caller's perspective, this looks synchronous - you call, you wait, you get a result. But from the system's perspective, it's asynchronous:

1. The caller sends a request message with a unique reference (`gen.Ref`)
2. The caller's goroutine blocks waiting for a response with that reference
3. The recipient receives the request as a `HandleCall` invocation
4. The recipient processes it and sends a response message with the same reference
5. The response arrives in the caller's mailbox, waking up the blocked goroutine
6. The caller's `Call` returns with the result

The caller blocks, but blocking is isolated to that one actor. The actor's goroutine is suspended (cheap), not spinning (expensive). Other actors run normally. The recipient processes the request whenever it gets to it in its mailbox, not immediately. The entire system remains asynchronous, but individual actors can use synchronous-style APIs when needed.

## Basic HandleCall Implementation

When a process receives a Call request, the framework invokes `HandleCall`:

```go
type Calculator struct {
    act.Actor
}

func (c *Calculator) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
    switch req := request.(type) {
    case AddRequest:
        result := req.A + req.B
        return result, nil
        
    case DivideRequest:
        if req.B == 0 {
            // Return error as the result value, not as termination reason
            return fmt.Errorf("division by zero"), nil
        }
        result := req.A / req.B
        return result, nil
        
    default:
        // Return error as the result value
        return fmt.Errorf("unknown request type"), nil
    }
}
```

**Critical distinction:** The error you return from `HandleCall` is **not** the response to the caller - it's the termination reason for your process!

- `return result, nil` - Send `result` to caller, continue running
- `return errorValue, nil` - Send `errorValue` to caller, continue running  
- `return result, gen.TerminateReasonNormal` - Send `result` to caller, then terminate normally
- `return nil, someError` - Terminate with `someError`, no response sent to caller

When you return a non-nil result from `HandleCall`, the framework automatically sends it as a response message to the caller. The caller's blocked `Call` unblocks and returns your result. Any value can be a result - integers, strings, structs, even errors.

If you need to send an error to the caller, return the error **as the result value**, not as the error return:

```go
// WRONG - terminates the process!
if invalid {
    return nil, fmt.Errorf("invalid request")
}

// CORRECT - sends error to caller
if invalid {
    return fmt.Errorf("invalid request"), nil
}
```

The second return value (error) is for terminating your process. Return `gen.TerminateReasonNormal` to gracefully stop, or any other error for abnormal termination. If you return both a result and `gen.TerminateReasonNormal`, the framework sends the result first, then terminates your process.

From the caller's side:

```go
// Somewhere in another actor
result, err := process.Call(calculatorPID, AddRequest{A: 10, B: 20})
if err != nil {
    // This is a framework error (timeout, connection lost, etc)
    process.Log().Error("call failed: %s", err)
    return err
}

// Check if the result itself is an error (application-level error)
if errResult, ok := result.(error); ok {
    process.Log().Error("calculator returned error: %s", errResult)
    return errResult
}

sum := result.(int)
process.Log().Info("10 + 20 = %d", sum)
```

The caller blocks at `Call` until your `HandleCall` returns. This can be milliseconds (local, fast computation) or seconds (remote, slow operation). The caller can specify a timeout - if no response arrives within the timeout, `Call` returns `nil, gen.ErrTimeout`.

Note the distinction: `err` from `Call` is a framework-level error (timeout, network failure, process terminated). The `result` itself might be an error value sent by your `HandleCall` - that's application-level.

## Why Not Just Use Channels?

You might wonder: why not just use Go channels for request-response?

```go
// Tempting but wrong in actor model
response := make(chan Result)
process.Send(workerPID, Request{Data: data, ResponseChan: response})
result := <-response  // block waiting
```

This breaks the actor model in subtle ways:

**Shared memory** - Channels are shared memory. Passing a channel in a message creates a direct communication path outside the actor system. If the worker is on a remote node, the channel doesn't work (channels don't serialize). Your code becomes non-portable between local and remote.

**Blocking semantics** - Blocking on a channel blocks the actor's goroutine, but the actor is still "running" from the framework's perspective. The actor can't process other messages while blocked. With `Call`, the framework knows the actor is waiting for a response and can properly account for it (the actor is in `ProcessStateWaitResponse`).

**Timeout coordination** - Channels don't have built-in timeouts. You'd wrap them in `select` with `time.After`, but timeout cleanup is tricky. With `Call`, timeouts are built-in, and references have deadlines that the receiver can check.

**No network transparency** - `Call` works identically for local and remote processes. Channels don't. If you use channels for local request-response, your code won't work when you move to a distributed deployment.

The framework's `Call` mechanism is designed specifically for request-response in the actor model, works across the network, and integrates properly with the actor lifecycle.

## Handling Requests with Worker Pools

A common pattern is a server process that receives many Call requests. If processing each request takes time (database query, HTTP call, complex computation), handling them sequentially in `HandleCall` creates a bottleneck. One slow request delays all subsequent requests.

The solution is `act.Pool` - a specialized actor that automatically distributes requests across a pool of worker actors:

```go
type Server struct {
    act.Pool
}

type Worker struct {
    act.Actor
}

func (s *Server) Init(args ...any) (act.PoolOptions, error) {
    return act.PoolOptions{
        PoolSize:      10,  // 10 worker actors
        WorkerFactory: func() gen.ProcessBehavior { return &Worker{} },
    }, nil
}

// No HandleCall needed for Server! Pool handles forwarding automatically.

func (w *Worker) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
    // Process the request
    switch req := request.(type) {
    case QueryRequest:
        // Simulate slow operation
        time.Sleep(100 * time.Millisecond)
        result := fmt.Sprintf("Result for: %s", req.Query)
        return result, nil
        
    default:
        // Return error as result value, not termination reason
        return fmt.Errorf("unknown request"), nil
    }
}
```

Notice what's **not** in this code - there's no `HandleCall` for the Server. You don't need one.

`act.Pool` automatically intercepts all incoming Call requests and forwards them to workers. When you send a Call to the Server PID, the Pool:

1. Receives the Call request in its mailbox
2. Pops an available worker from the pool
3. Forwards the entire request (from, ref, message) to the worker
4. Returns the worker to the pool (reusable for next request)

The worker receives the Call request with the **original caller's PID and ref**. When the worker returns a result from `HandleCall`, it goes directly to the original caller, bypassing the Pool entirely. The Pool is just a router.

From the caller's perspective:

```go
// Caller doesn't know about the pool
result, err := process.Call(serverPID, QueryRequest{Query: "data"})
// Result comes from whichever worker handled it
```

This gives you concurrent request processing:
- 10 Call requests arrive at the Server simultaneously  
- Pool forwards each to a different worker
- All 10 workers process concurrently
- Each worker responds directly to its caller
- Pool remains free to route more requests

The caller's experience is unchanged - they call, they block, they get a result. They don't know about the pool. The concurrency is entirely internal to the server.

**Worker resilience:**

If a worker crashes or becomes unresponsive, the Pool automatically spawns a replacement worker. Worker failures don't affect the Pool's availability - other workers continue processing requests while the Pool restarts failed workers in the background.

If all workers are busy (mailboxes full), incoming requests queue up in the Pool's mailbox until a worker becomes available.

For more details on Pool configuration and advanced patterns, see [Pool Actor](../actors/pool.md).

## Asynchronous Processing of Synchronous Requests

Sometimes you need to handle a Call request asynchronously within a single actor, without workers. Maybe you're waiting for a timer, or you need to make another Call before you can respond, or you want to batch multiple requests.

You can do this manually:

```go
type AsyncHandler struct {
    act.Actor
    pending map[gen.Ref]pendingRequest
}

type pendingRequest struct {
    from gen.PID
    data any
}

func (a *AsyncHandler) Init(args ...any) error {
    a.pending = make(map[gen.Ref]pendingRequest)
    return nil
}

func (a *AsyncHandler) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
    switch req := request.(type) {
    case BatchRequest:
        // Store the request for later
        a.pending[ref] = pendingRequest{from: from, data: req}
        
        // Maybe set a timer to process after accumulating more requests
        a.SendAfter(a.PID(), BatchTrigger{}, 100 * time.Millisecond)
        
        // Return nil to handle asynchronously
        return nil, nil
        
    case ImmediateRequest:
        // This one we can answer immediately
        return "immediate result", nil
    }
    
    // Return error as result value
    return fmt.Errorf("unknown request"), nil
}

func (a *AsyncHandler) HandleMessage(from gen.PID, message any) error {
    switch msg := message.(type) {
    case BatchTrigger:
        // Time to respond to all pending requests
        for ref, pr := range a.pending {
            result := a.processBatch(pr.data)
            a.SendResponse(pr.from, ref, result)
        }
        a.pending = make(map[gen.Ref]pendingRequest)  // clear
    }
    return nil
}
```

The pattern:
1. `HandleCall` stores `from` and `ref` for later
2. `HandleCall` returns `(nil, nil)` - async handling
3. Later (timer, another message, whatever), you process the request
4. Call `SendResponse(from, ref, result)` to send the result
5. Caller's blocked `Call` unblocks with your result

You must respond eventually, or the caller will timeout. If you lose track of the `ref` or forget to respond, the caller waits until timeout and gets `gen.ErrTimeout`.

The result you send with `SendResponse` can be any value - strings, numbers, structs, even errors. If you want to send an error to the caller, just send it as a normal result value:

```go
if invalid {
    a.SendResponse(pr.from, ref, fmt.Errorf("validation failed"))
}
```

The caller receives it as `result` (first return value from Call) and can check if it's an error.

## SendResponse vs SendResponseError: Two Channels for Results

When you handle Call requests asynchronously, you send responses later using `SendResponse`. But there's also `SendResponseError`. What's the difference, and when do you use each?

The difference is in which return value the caller receives from `Call`.

**SendResponse sends to the result channel:**

```go
// Handler
a.SendResponse(caller, ref, "success")

// Caller receives
result, err := process.Call(handler, request)
// result = "success"
// err = nil
```

Whatever you send appears as the **first return value** (result). The second return value (err) is `nil`, meaning no framework error occurred. The result can be anything - strings, numbers, structs, even errors:

```go
// Handler sends an error as a result
a.SendResponse(caller, ref, fmt.Errorf("user not found"))

// Caller receives
result, err := process.Call(handler, request)
// result = error("user not found")
// err = nil
```

The caller must check if the result is an error:

```go
result, err := process.Call(handler, request)
if err != nil {
    // Framework problem - timeout, network, process died
    return fmt.Errorf("call failed: %w", err)
}

if errResult, ok := result.(error); ok {
    // Application-level error
    return fmt.Errorf("operation failed: %w", errResult)
}

// Success - use result
processResult(result)
```

**SendResponseError sends to the error channel:**

```go
// Handler
a.SendResponseError(caller, ref, fmt.Errorf("database unavailable"))

// Caller receives
result, err := process.Call(handler, request)
// result = nil
// err = error("database unavailable")
```

The error appears as the **second return value** (err), exactly where framework errors like timeout and network failures appear. The first return value (result) is `nil`.

From the caller's perspective, there's no difference between an error from `SendResponseError` and a framework error:

```go
result, err := process.Call(handler, request)
if err != nil {
    // Could be:
    // - Timeout (gen.ErrTimeout)
    // - Network failure (gen.ErrNoConnection)
    // - Process crashed (gen.ErrProcessTerminated)
    // - OR: Handler sent via SendResponseError
    // Caller cannot distinguish!
    return fmt.Errorf("call failed: %w", err)
}
```

**The problem with mixing channels**

The framework uses the error channel for transport errors - problems with the messaging infrastructure. Your application uses it for business logic results. When you call `SendResponseError`, you're mixing these two concerns.

Consider a typical caller error handling:

```go
result, err := process.Call(databaseActor, query)
if err != nil {
    // Retry logic for transport errors
    time.Sleep(1 * time.Second)
    result, err = process.Call(databaseActor, query)
    if err != nil {
        return err  // Give up
    }
}
```

This makes sense for transport errors - network glitches, temporary overload. But if the database actor uses `SendResponseError` for "record not found", the caller retries unnecessarily. The record won't appear in one second.

The caller has no way to distinguish. Both arrive through the error channel.

**When mixing is justified**

Despite this issue, `SendResponseError` has legitimate uses. The key is: **use it for errors that should be handled like transport errors**.

Imagine a database query actor. It receives queries, executes them against a database, and returns results. What errors can occur?

**Application errors** - problems with the query itself:
- Bad SQL syntax
- Permission denied
- Constraint violation

These are **not** infrastructure problems. The actor is working fine, the database is up, the request was processed. The query just has issues. The caller should see these as results, not transport failures.

**Infrastructure errors** - problems with the database connection:
- Database server is down
- Network to database lost
- Connection pool exhausted
- Too many simultaneous connections

These **are** infrastructure problems. The actor couldn't process the request because a dependency is unavailable. From the caller's perspective, this is the same as if the actor itself were unreachable (timeout) or the node were down (network failure). The caller should handle all of these identically - retry, fallback, circuit breaking.

Here's how to implement this:

```go
type DatabaseActor struct {
    act.Actor
    db      *sql.DB
    pending map[gen.Ref]pendingRequest
}

type pendingRequest struct {
    from  gen.PID
    query string
}

func (d *DatabaseActor) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
    query := request.(string)
    
    // Store for async processing
    d.pending[ref] = pendingRequest{from: from, query: query}
    
    // Trigger async processing
    d.Send(d.PID(), executeQuery{ref: ref})
    
    return nil, nil  // Will respond asynchronously
}

func (d *DatabaseActor) HandleMessage(from gen.PID, message any) error {
    switch msg := message.(type) {
    case executeQuery:
        pr := d.pending[msg.ref]
        
        // Execute query
        rows, err := d.db.Query(pr.query)
        
        if err != nil {
            // Distinguish error types
            if isInfrastructureError(err) {
                // Database down, connection lost, etc
                // Send as transport error - caller should retry/fallback
                d.SendResponseError(pr.from, msg.ref, fmt.Errorf("database unavailable: %w", err))
            } else {
                // Bad SQL, permission denied, etc
                // Send as application result - caller should show to user
                d.SendResponse(pr.from, msg.ref, fmt.Errorf("query failed: %w", err))
            }
            delete(d.pending, msg.ref)
            return nil
        }
        
        // Success
        d.SendResponse(pr.from, msg.ref, rows)
        delete(d.pending, msg.ref)
    }
    return nil
}

func isInfrastructureError(err error) bool {
    // Check for connection-related errors
    if strings.Contains(err.Error(), "connection refused") {
        return true
    }
    if strings.Contains(err.Error(), "too many connections") {
        return true
    }
    // ... other infrastructure error checks
    return false
}
```

The caller handles both channels naturally:

```go
result, err := process.Call(databaseActor, "SELECT * FROM users")
if err != nil {
    // Infrastructure problem:
    // - Database is down (SendResponseError)
    // - Actor timed out (gen.ErrTimeout)
    // - Network failure (gen.ErrNoConnection)
    // All handled the same way - try fallback
    
    process.Log().Warning("database unavailable, using cache: %s", err)
    return useFallbackCache()
}

// Check if result is an error
if errResult, ok := result.(error); ok {
    // Application error - bad query, permission denied, etc
    // Don't retry, don't fallback - show to user
    return fmt.Errorf("query error: %w", errResult)
}

// Success
return result
```

This works because the caller **wants** to handle infrastructure failures identically, regardless of whether they originate from the framework (timeout, network) or from the application (database down). Both represent unavailable service, both trigger the same fallback logic.

**Guideline**

Use `SendResponse` for all normal cases, including expected errors (validation, not found, unauthorized). These are results - the request was processed, here's what happened.

Use `SendResponseError` only when the error represents an infrastructure failure that the caller should treat the same as transport errors - retry with backoff, circuit breaking, fallback to alternative services.

If in doubt, use `SendResponse`. It keeps transport and application concerns separate, giving the caller maximum clarity.

## Using Ref.IsAlive for Timeout Awareness

When you handle requests asynchronously, the caller might timeout before you respond. Imagine:

1. Caller makes a Call with 5 second timeout
2. Your HandleCall stores the request, returns nil (async)
3. 6 seconds pass
4. Caller's timeout fires, `Call` returns `gen.ErrTimeout`
5. Your actor finishes processing and calls `SendResponse`

Your response arrives after the caller stopped waiting. The caller won't receive it (it's not waiting on that `ref` anymore). Your work was wasted.

You can detect this with `ref.IsAlive()`:

```go
func (a *AsyncHandler) HandleMessage(from gen.PID, message any) error {
    switch msg := message.(type) {
    case BatchTrigger:
        for ref, pr := range a.pending {
            // Check if the caller is still waiting
            if !ref.IsAlive() {
                // Timeout expired, don't bother processing
                a.Log().Warning("request %s expired, skipping", ref)
                delete(a.pending, ref)
                continue
            }
            
            // Still waiting, process and respond
            result := a.processBatch(pr.data)
            a.SendResponse(pr.from, ref, result)
            delete(a.pending, ref)
        }
    }
    return nil
}
```

`ref.IsAlive()` checks the deadline embedded in the reference. When the caller made the Call with a timeout, the framework created a reference with `MakeRefWithDeadline(now + timeout)`. The deadline is stored in `ref.ID[2]` as a unix timestamp. `IsAlive()` compares it to the current time - if the deadline passed, it returns false.

This lets you skip processing expired requests. If a request took too long to reach the front of the queue, and the caller already gave up, don't waste resources computing a response nobody will receive.

But be careful: `IsAlive()` returning false doesn't mean the caller is definitely gone. It means the deadline passed. The caller might have disappeared for other reasons (crash, network disconnect), or they might still exist but already moved on. It's a hint for optimization, not a guarantee about caller state.

If you send a response after the deadline, nothing bad happens. The response message arrives, the receiver checks if anyone is waiting for that `ref`, finds nobody, and drops the message. It's just wasted work - harmless but inefficient.

## Common Patterns and Pitfalls

**Pattern: Immediate vs deferred**

```go
func (a *Handler) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
    switch req := request.(type) {
    case CachedRequest:
        // We have the answer immediately
        if result, found := a.cache[req.Key]; found {
            return result, nil
        }
        // Cache miss, fetch asynchronously
        a.pending[ref] = pendingRequest{from: from, data: req}
        a.fetchFromBackend(req.Key, ref)
        return nil, nil
        
    case WriteRequest:
        // Writes are fast, handle synchronously
        a.data[req.Key] = req.Value
        return "ok", nil
    }
    // Return error as result value
    return fmt.Errorf("unknown request"), nil
}
```

Some requests you can answer immediately, others need async processing. Mix both in the same `HandleCall` based on the situation.

**Pattern: Batch processing**

```go
type Batcher struct {
    act.Actor
    pending []pendingRequest
    timer   gen.CancelFunc
}

type pendingRequest struct {
    from gen.PID
    ref  gen.Ref
    data any
}

func (b *Batcher) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
    // Add to batch
    b.pending = append(b.pending, pendingRequest{from, ref, request})
    
    // Start timer if this is the first request
    if len(b.pending) == 1 {
        b.timer = b.SendAfter(b.PID(), Flush{}, 100 * time.Millisecond)
    }
    
    // If batch is full, flush immediately
    if len(b.pending) >= 100 {
        if b.timer != nil {
            b.timer()  // cancel timer
        }
        b.flush()
    }
    
    return nil, nil
}

func (b *Batcher) flush() {
    // Process all pending requests in one batch
    results := b.processBatch(b.pending)
    
    for i, pr := range b.pending {
        if pr.ref.IsAlive() {
            b.SendResponse(pr.from, pr.ref, results[i])
        }
    }
    
    b.pending = b.pending[:0]  // clear, keep capacity
}
```

Accumulate requests, process them together, respond to each individually. Efficient for operations with high setup cost (database connections, API requests with rate limits).

**Pitfall: Losing references**

```go
// WRONG: Storing only the reference
func (a *Handler) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
    a.pendingRefs = append(a.pendingRefs, ref)  // Lost the 'from'!
    return nil, nil
}

// Later - how do we respond?
func (a *Handler) respond() {
    for _, ref := range a.pendingRefs {
        a.SendResponse(???, ref, result)  // Who do we send to?
    }
}
```

You need both `from` and `ref` to send a response. Store them together.

**Pitfall: Confusing result errors with termination errors**

```go
// WRONG: This terminates your process!
func (a *Handler) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
    if !a.isAuthorized(from) {
        return nil, fmt.Errorf("unauthorized")  // OOPS! Process terminates
    }
    return a.process(request), nil
}

// CORRECT: Send error as result to caller
func (a *Handler) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
    if !a.isAuthorized(from) {
        return fmt.Errorf("unauthorized"), nil  // Caller gets error, process continues
    }
    return a.process(request), nil
}

// ALSO CORRECT: For async handling
func (a *Handler) HandleMessage(from gen.PID, message any) error {
    switch msg := message.(type) {
    case processedResult:
        // Send any result - value or error, doesn't matter
        a.SendResponse(msg.caller, msg.ref, msg.result)
    }
    return nil
}
```

This is the most common mistake. Remember: the error return from `HandleCall` terminates your process, it doesn't go to the caller (except the special case of `gen.TerminateReasonNormal` with a non-nil result).

**Pitfall: Blocking in HandleCall**

```go
// WRONG: Blocks the actor
func (a *Handler) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
    time.Sleep(5 * time.Second)  // Actor can't process other messages!
    return "done", nil
}
```

Even though the caller is blocked waiting, your actor shouldn't block. If you sleep for 5 seconds, you can't handle other messages during that time. Other callers will queue up waiting. If this is unavoidable (calling a blocking API you don't control), spawn a worker to handle it or use `act.Pool`.

## The Path to Important Delivery

Everything discussed so far assumes the response message arrives. But what if it doesn't? Networks drop packets. Remote processes crash. Connections fail.

When a response is lost, the caller blocks until timeout. Eventually `Call` returns `gen.ErrTimeout`, but you don't know if the request was processed or not. Did the receiver handle it and the response got lost? Or did the request itself get lost before reaching the receiver?

This uncertainty is a fundamental problem in distributed systems. The framework's `Call` mechanism gives you request-response semantics, but it doesn't guarantee the response arrives. It's "best effort" - works reliably for local calls and stable network connections, but no guarantees.

For many use cases, this is fine. Timeouts are acceptable. Callers can retry. Idempotent operations tolerate retries. But some operations can't tolerate uncertainty. A payment authorization must definitely succeed or definitely fail - timeout isn't acceptable.

The solution is **Important Delivery**. When you enable the Important flag, the framework changes from "best effort" to "confirmed delivery." Responses don't just get sent, they get acknowledged. If the response fails to deliver, you know immediately rather than waiting for timeout.

Important Delivery makes the network transparent for failures, not just successes. It turns request-response from "probably works" into "definitely works or definitely fails, no ambiguity."

We'll explore Important Delivery in depth in the next chapter. For now, understand that everything you've learned about `Call` and `HandleCall` still applies. Important Delivery is a layer on top, not a replacement. You'll still handle requests the same way - the framework just makes delivery more reliable.

For details on how messages and calls flow through the network, see [Network Transparency](../networking/network-transparency.md). For understanding delivery guarantees, continue to [Important Delivery](important-delivery.md).
