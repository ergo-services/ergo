---
description: What is a Process in Ergo Framework
---

# Process

A process is an actor - a lightweight entity that handles messages sequentially in its own goroutine. It's the fundamental building block of an Ergo application.

Every process has a mailbox where incoming messages wait to be processed. The mailbox contains four queues with different priorities: Urgent for critical system messages, System for framework control, Main for regular application messages, and Log for logging. When the process wakes up to handle messages, it processes them in priority order, taking from Urgent first, then System, then Main, and finally Log.

When built with `-tags=latency`, each queue tracks the age of its oldest unprocessed message. `ProcessMailbox.Latency()` returns the maximum latency across all four queues in nanoseconds, or -1 if the tag is not enabled. This helps identify processes that are falling behind on message processing. See [Debugging](../advanced/debugging.md) for details.

The process runs only when it has messages to handle. When the mailbox is empty, the process sleeps, consuming no CPU. When a message arrives, the process wakes, handles the message, and sleeps again if nothing else is waiting. This efficiency is why you can have thousands of processes in a single application.

## Identifying Processes

A process identifier (`gen.PID`) uniquely identifies a process across the entire distributed system. It contains three components: the node name where the process runs, a unique sequential number within that node, and a creation timestamp.

The creation timestamp is the node's startup time. If a node restarts, the creation value changes, which means PIDs from before the restart are distinguishable from PIDs after. If you try to send a message to a `gen.PID` with an old creation value, you get an error. This prevents messages from being delivered to the wrong process after a node restart.

Besides PIDs, processes can be identified by registered names. A process can register one name, making it addressable as `gen.ProcessID{Name: "worker", Node: "node@host"}`. This is useful for well-known processes that other parts of the system need to find without knowing their `gen.PID`.

Processes can also create aliases - temporary identifiers that provide additional addressing options. Unlike registered names (one per process), a process can create unlimited aliases using `gen.Alias`. They're useful when you need multiple ways to address the same process, such as in request-response patterns or when implementing services with multiple endpoints.

## Process Lifecycle

A process goes through several states during its lifetime.

It starts in Init, where the ProcessInit callback runs. In this state, the process can spawn children, send messages, register names, create aliases, register events, establish links and monitors, and make synchronous calls.

After initialization succeeds, the process enters Sleep and is ready to receive messages. When a message arrives, the process transitions to Running, handles the message, and returns to Sleep.

If the process makes a synchronous call, it enters WaitResponse while waiting for the reply. Once the response arrives, it returns to Running and continues processing.

Eventually the process terminates. This can happen in several ways: it returns an error from its message handler, it receives an exit signal, the node kills it, or a panic occurs. The ProcessTerminate callback runs, allowing cleanup. Then the process is removed from the node, and its resources are freed.

## Starting Processes

You spawn processes through a factory function that creates instances of your actor.

```go
type Worker struct {
    act.Actor
}

func createWorker() gen.ProcessBehavior {
    return &Worker{}
}

pid, err := node.Spawn(createWorker, gen.ProcessOptions{})
```

The factory is called each time you spawn - each process gets a fresh instance. This isolation is important for the actor model.

`gen.ProcessOptions` configures the new process: mailbox size, environment variables, compression settings, message priority, linking behavior, and initialization timeout. Most options have sensible defaults. The main ones you'll configure are `MailboxSize` (to limit memory) and `Env` (to pass configuration).

`InitTimeout` limits how long `ProcessInit` can take. Zero uses the default (5 seconds). If initialization exceeds this timeout, the process is terminated with `gen.ErrTimeout` and spawn returns an error. For remote spawn and application processes, the maximum allowed value is 15 seconds - exceeding this limit returns `gen.ErrNotAllowed`.

Two options deserve explanation: `LinkParent` and `LinkChild`. These options provide a convenient way to establish links automatically after initialization completes. If `LinkChild` is set, the parent links to the child. If `LinkParent` is set, the child links to the parent. These links only work for process-spawned children, not node-spawned processes. Note that you can also call `Link` methods directly during initialization if needed.

## Message Handling

Processes are defined by implementing the `gen.ProcessBehavior` interface. This is a low-level interface with three callbacks: `ProcessInit` for initialization, `ProcessRun` for the message processing loop, and `ProcessTerminate` for cleanup.

In practice, you rarely implement `gen.ProcessBehavior` directly. Instead, you use `act.Actor`, which implements `gen.ProcessBehavior` and provides a more convenient abstraction. `act.Actor` gives you `HandleMessage` and `HandleCall` callbacks - straightforward methods where you write your message handling logic without worrying about the mailbox mechanics.

```go
type Worker struct {
    act.Actor
}

func (w *Worker) HandleMessage(from gen.PID, message any) error {
    // Handle the message
    // Return nil to continue, return error to terminate
    return nil
}
```

The `ProcessInit` callback runs once during startup. Use it to initialize state, spawn children, configure properties. If it returns an error or exceeds the `InitTimeout`, the process is cleaned up and removed - it terminates immediately.

The `ProcessTerminate` callback runs during shutdown. Use it for cleanup: close files, send final messages, log termination. It receives the termination reason, so you can distinguish between normal shutdown and errors.

`act.Actor` handles the `ProcessRun` loop for you, calling your `HandleMessage` and `HandleCall` methods as messages arrive. This separation between the low-level interface (`gen.ProcessBehavior`) and the high-level abstraction (`act.Actor`) keeps the framework flexible while making common cases simple.

## Timed and Periodic Messages

Sometimes a process needs to act later, or on a schedule, rather than in response to an incoming message. The wrong way is to start a `time.Timer` or `time.Ticker` inside a callback: it fires on its own goroutine, outside the mailbox, and touching process state from there breaks the single-threaded guarantee.

Instead, let the process message itself. `SendAfter` delivers a message to a target once after a delay; `SendEvery` delivers it repeatedly on a fixed period, reusing a single timer so it does not allocate on each tick. Both return a cancel function, and both route the message through the mailbox - it is handled in `HandleMessage` like any other, on the process's own goroutine and in order. The delivery options (priority, compression, network order) are captured when you schedule, not re-read on each fire.

A `SendEvery` ticker lives as long as its owner: it stops when the process terminates or when you call its cancel function. A failed tick - the target is busy or already gone - does not stop it; transient failures are retried on the next period, and if you need to know that the target died, monitor it rather than infer it from a gap in ticks.

## Environment Variables

Processes inherit environment variables when they spawn. At that moment, variables are copied from multiple sources and merged with a priority order: node variables (lowest priority), then application, then leader, then parent, then variables specified in `gen.ProcessOptions` (highest priority). If the same variable exists in multiple sources, the higher priority value wins.

Once a process is running, its environment is independent. If the node changes an environment variable, running processes don't see the change. Only newly spawned processes inherit the updated values. This isolation is important - it means a process's configuration is stable for its lifetime.

When a process queries a variable with `Env` or `EnvList`, it looks only in its own environment - the merged copy created at spawn time. The hierarchy (Process > Parent > Leader > Application > Node) determines what was copied during spawning, not what's queried during lookup.

Variables are case-insensitive. "database_url", "DATABASE_URL", and "Database_Url" are all the same variable. This eliminates configuration mistakes from case mismatches.

Use `SetEnv` to modify variables during Init or Running states. Pass `nil` as the value to delete a variable. Changes affect only this process - they don't propagate to children, parents, or the node.

## Termination

Processes typically terminate themselves by returning an error from `ProcessRun`. In `act.Actor`, this manifests as returning an error from `HandleMessage`, `HandleCall`, or other handler callbacks. Return `gen.TerminateReasonNormal` for clean shutdown, or any other error to indicate why termination occurred. The process transitions to Terminated, runs its `ProcessTerminate` callback for cleanup, and is removed from the node.

If a panic occurs during message handling, the framework catches it, logs the stack trace, and terminates the process with `gen.TerminateReasonPanic`. The `ProcessTerminate` callback still runs, giving the process a chance to clean up despite the panic.

### Mailbox Preservation

When a process dies, its mailbox is discarded by default. Anything queued but unprocessed is lost. For short-lived or one-shot processes this is fine. For continuously working actors it can be very painful:

- A task worker pulling from an external queue panics on one bad task. The 50 other queued tasks in its mailbox evaporate. They have to be re-fetched from upstream if upstream still has them, or they are gone.
- A per-session actor (chat user, IoT device, game session) crashes during a code push. Messages buffered for it between crash and restart vanish.
- A batch aggregator collecting events for a periodic flush dies mid-batch. The unflushed accumulator is gone with no record of what was inside.
- A sequencer enforcing ordering across multiple sources crashes mid-stream. The reorder buffer is lost.

Mailbox preservation gives an "actor restarts but keeps its inbox" semantic. It turns the default behavior from at-most-once for queued messages to at-least-once. Enable it with one flag at spawn:

```go
node.Spawn(createWorker, gen.ProcessOptions{
    PreserveMailbox: true,
})
```

With the flag set, any abnormal termination (panic, callback error, forced `Kill`, exit cascade from a linked process) captures the mailbox into a `*gen.Error` exit reason. The original error is preserved as `Wrapped[0]`, so `errors.Is(reason, originalErr)` keeps working. A supervising parent automatically picks it up and hands it to the restart. The new incarnation runs `Init` on fresh struct state, then its `ProcessRun` loop pulls the surviving messages from the queues in priority order, exactly as if they had just arrived.

This is not a replacement for an external durable queue (Kafka, NATS, etc.). External queues give reliable delivery into the actor; mailbox preservation gives reliable handling within the actor across its own restarts. They complement each other.

**What is preserved.** All four priority queues (Urgent, System, Main, Log), in original FIFO order, with their tracing information.

**What is not preserved.**
- The triggering message (the one the actor was processing when it failed). Replaying it would create a panic-restart-panic loop on poison-pill inputs, so the framework deliberately drops it. Upstream must replay if needed.
- The actor's struct fields. In-process state lives only as long as the incarnation does; the new instance runs `Init` from scratch.
- Outgoing Calls in flight. The ref is tied to the dead PID; responses arriving after death are dropped.

**Normal exits skip capture.** `gen.TerminateReasonNormal` and `gen.TerminateReasonShutdown` mean the actor itself decided to stop, and re-feeding its queue would contradict that decision.

**At-least-once caveat.** Because the surviving messages are re-delivered to a new instance, partially completed side effects from the dead incarnation can be re-attempted by the new one. If your handler is not idempotent (or doesn't deduplicate by some message id), you can end up with duplicate side effects. This is the standard at-least-once trade-off: you trade duplicates for losses.

**Same-node only.** A live mailbox does not cross the network: the `Mailbox` field on `gen.Error` is excluded from EDF wire encoding. Remote spawns and remote exit signals see it as nil. Mailbox preservation is a local feature.

**When to enable.** Task workers, per-request/per-connection handlers, per-session actors, aggregators. Anywhere unprocessed messages in queue represent unfinished work that must not silently disappear.

**When not to enable.** Strongly stateful actors with non-idempotent side effects that don't deduplicate. Memory-constrained scenarios where a large preserved mailbox would pin too much memory between crash and restart. One-shot init scripts that must restart from a clean state.

A common production setup is `PreserveMailbox: true` together with a per-spec or per-instance restart budget and `OnExceedDisable`, so a poison-pill input cannot loop the worker through its budget and take the supervisor down. See [Mailbox Preservation Across Restart](../actors/supervisor.md#mailbox-preservation-across-restart) on the Supervisor page for the full supervisor-side contract, validation rules, and a worked example.

Processes can also be terminated externally. Sending an exit signal with `SendExit` delivers a high-priority termination request to the process's Urgent queue. Actors can trap these signals and handle them as regular messages, allowing graceful shutdown. This is how supervision trees restart workers - send an exit signal, wait for clean termination, then spawn a replacement.

The most forceful option is `Kill`. If the process is idle (Sleep state), it transitions directly to Terminated and `ProcessTerminate` is called. If the process is actively handling a message (Running or WaitResponse states), it's marked as Zombee. In Zombee state, all operations return `gen.ErrNotAllowed`. The process finishes its current message, then terminates and calls `ProcessTerminate`. Use `Kill` when you need to stop a process that isn't responding to exit signals.

Regardless of how termination happens, the node performs comprehensive cleanup. Events the process registered are unregistered. Its registered name becomes available for reuse. Aliases are deleted. Links and monitors are removed. If the process was acting as a logger, it's removed from the logging system. Meta processes spawned by this process are terminated. This ensures no dangling references remain after a process is gone.

## State-Based Access Control

Not all Process interface methods work in all states. This isn't arbitrary - it reflects what's actually possible.

During Init, the process can spawn children, send messages, register names, create aliases, register events, establish links and monitors, and make synchronous calls.

During Running, everything is available. The process is fully operational.

During Terminated, only sending messages works. You can't spawn new children or create new resources - the process is shutting down.

These restrictions are enforced by the framework. If you call a method in the wrong state, you get `gen.ErrNotAllowed`. This prevents subtle bugs where operations appear to succeed but silently fail because the process isn't in the right state.

The details of which methods work in which states are documented in the `gen.Process` godoc. In practice, you rarely hit these restrictions unless you're doing unusual things during initialization or shutdown.

For a deeper understanding of process operations and lifecycle management, refer to the `gen.Process` interface documentation in the code.
