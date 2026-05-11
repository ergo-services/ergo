# Supervisor

Actors fail. They panic, encounter errors, or lose external resources. In traditional systems, you add defensive code: catch exceptions, retry operations, validate state. This spreads failure handling throughout your codebase, mixing recovery logic with business logic.

The actor model takes a different approach: let it crash. When an actor fails, terminate it cleanly and restart it in a known-good state. This requires something watching the actor and managing its lifecycle - a supervisor.

`act.Supervisor` is an actor that manages child processes. It starts them during initialization, monitors them for failures, and applies restart strategies when they terminate. Supervisors can manage other supervisors, creating hierarchical fault tolerance trees where failures are isolated and recovered automatically.

Like `act.Actor`, the `act.Supervisor` struct implements the low-level `gen.ProcessBehavior` interface and has the embedded `gen.Process` interface. To create a supervisor, you embed `act.Supervisor` in your struct and implement the `act.SupervisorBehavior` interface.

The only mandatory method is `Init`, which returns a `SupervisorSpec` describing the children and the restart policy:

```go
Init(args ...any) (SupervisorSpec, error)
```

All other behavior methods are optional. `act.Supervisor` provides default implementations:
- `HandleChildStart` and `HandleChildTerminate` for child lifecycle hooks (only when `EnableHandleChild: true`).
- `HandleMessage`, `HandleCall`, `HandleEvent` for receiving regular messages, synchronous calls, and events while running.
- `HandleInspect` for diagnostic queries.
- `Terminate` for cleanup on supervisor exit.

These optional methods are described in detail in the sections below. The full interface is defined in `act/supervisor.go` if you want to look at the source.

## Creating a Supervisor

Embed `act.Supervisor` and implement `Init` to define the supervision spec:

```go
type AppSupervisor struct {
    act.Supervisor
}

func (s *AppSupervisor) Init(args ...any) (act.SupervisorSpec, error) {
    return act.SupervisorSpec{
        Type: act.SupervisorTypeOneForOne,
        Children: []act.SupervisorChildSpec{
            {
                Name:    "database",
                Factory: createDBWorker,
                Args:    []any{"postgres://..."},
            },
            {
                Name:    "api",
                Factory: createAPIServer,
                Args:    []any{8080},
            },
        },
        Restart: act.SupervisorRestart{
            Strategy:  act.SupervisorStrategyTransient,
            Intensity: 5,
            Period:    5,
        },
    }, nil
}

func createSupervisorFactory() gen.ProcessBehavior {
    return &AppSupervisor{}
}

// Spawn the supervisor
pid, err := node.Spawn(createSupervisorFactory, gen.ProcessOptions{})
```

The supervisor spawns all children during `Init` (except Simple One For One, which starts with zero children). Each child is connected to the supervisor with a pair of unidirectional links (`LinkChild` and `LinkParent` set automatically). If a child terminates, the supervisor receives an exit signal and applies the restart strategy.

Children are started sequentially in declaration order. If any child's spawn fails (the factory's `ProcessInit` returns an error), the supervisor terminates immediately with that error. This ensures the supervision tree is fully initialized or not at all - no partial states.

## Supervision Types

The `Type` field in `SupervisorSpec` determines what happens when a child fails.

### One For One

Each child is independent. When one child terminates, only that child is restarted. Other children continue running unaffected.

```go
Type: act.SupervisorTypeOneForOne,
Children: []act.SupervisorChildSpec{
    {Name: "worker1", Factory: createWorker},
    {Name: "worker2", Factory: createWorker},
    {Name: "worker3", Factory: createWorker},
},
```

If `worker2` crashes, the supervisor restarts only `worker2`. `worker1` and `worker3` keep running. Use this when children are independent - databases, caches, API handlers that don't depend on each other.

Each child runs with a registered name (the `Name` from the spec). This means only one instance per child spec. To run multiple instances of the same worker, use Simple One For One instead.

### All For One

Children are tightly coupled. When any child terminates, all children are stopped and restarted together.

```go
Type: act.SupervisorTypeAllForOne,
Children: []act.SupervisorChildSpec{
    {Name: "cache", Factory: createCache},
    {Name: "processor", Factory: createProcessor},  // Depends on cache
    {Name: "api", Factory: createAPI},              // Depends on both
},
```

If `cache` crashes, the supervisor stops `processor` and `api` (in reverse order if `KeepOrder` is true, simultaneously otherwise), then restarts all three in declaration order. Use this when children share state or dependencies that can't survive partial failures.

### Rest For One

When a child terminates, only children started *after* it are affected. Children started *before* it continue running.

```go
Type: act.SupervisorTypeRestForOne,
Children: []act.SupervisorChildSpec{
    {Name: "database", Factory: createDB},       // Independent
    {Name: "cache", Factory: createCache},       // Depends on database
    {Name: "api", Factory: createAPI},           // Depends on cache
},
```

If `cache` crashes, the supervisor stops `api`, then restarts `cache` and `api` in order. `database` is unaffected. Use this for dependency chains where later children depend on earlier ones, but earlier ones don't depend on later ones.

With `KeepOrder: true`, children are stopped sequentially (last to first). With `KeepOrder: false`, they stop simultaneously. Either way, restart happens in declaration order after all affected children have stopped.

### Simple One For One

All children run the same code, spawned dynamically instead of at supervisor startup.

```go
Type: act.SupervisorTypeSimpleOneForOne,
Children: []act.SupervisorChildSpec{
    {
        Name:    "worker",
        Factory: createWorker,
        Args:    []any{"default-config"},
    },
},
```

The supervisor starts with zero children. Call `supervisor.StartChild("worker", "custom-args")` to spawn instances:

```go
// Start 5 worker instances
for i := 0; i < 5; i++ {
    supervisor.StartChild("worker", fmt.Sprintf("worker-%d", i))
}
```

Each instance is independent. They're not registered by name (no `SpawnRegister`), so you track them by PID. When an instance terminates, only that instance is restarted (if the restart strategy allows). Other instances continue running.

Use Simple One For One for worker pools where you dynamically scale the number of identical workers based on load. The child spec is a template - each `StartChild` creates a new instance from that template.

## Choosing a Type

A quick decision matrix when you only need to pick the supervisor `Type`. The next sections explain restart strategies, intensity, per-child overrides, and other knobs that compose with the type you pick.

| If your children are... | Pick |
|---|---|
| Independent of each other (failure of one is unrelated to others) | `SupervisorTypeOneForOne` |
| Tightly coupled (any failure means restart everyone) | `SupervisorTypeAllForOne` |
| Arranged in a dependency chain (later children depend on earlier ones) | `SupervisorTypeRestForOne` |
| Dynamically created identical workers (one template, many instances) | `SupervisorTypeSimpleOneForOne` |

A larger reference table covering all combinations of type, strategy, per-child overrides, and lifecycle flags is at the end of this document: see [Behavior Cookbook](#behavior-cookbook).

## Restart Strategies

The `Restart.Strategy` field on `SupervisorRestart` sets the default rule for all children. Each child can override it via `SupervisorChildRestart.Strategy`. See [Per-Child Restart Control](#per-child-restart-control).

### Transient (Default)

Restart only on abnormal termination. If a child returns `gen.TerminateReasonNormal` or `gen.TerminateReasonShutdown`, it's not restarted:

```go
Restart: act.SupervisorRestart{
    Strategy: act.SupervisorStrategyTransient,  // Default
}
```

Use this for workers that can gracefully stop - maybe they finished their work, or received a shutdown command. Crashes (panics, errors, kills) trigger restarts. Normal termination doesn't.

### Temporary

Never restart, regardless of termination reason:

```go
Restart: act.SupervisorRestart{
    Strategy: act.SupervisorStrategyTemporary,
}
```

The child runs once. If it terminates (normal or crash), it stays terminated. Use this for initialization tasks or processes that shouldn't be restarted automatically.

### Permanent

Always restart, regardless of termination reason:

```go
Restart: act.SupervisorRestart{
    Strategy: act.SupervisorStrategyPermanent,
}
```

Even `gen.TerminateReasonNormal` triggers restart. Use this for critical processes that must always be running - maybe a health monitor or connection manager that should never stop.

With Permanent strategy, `DisableAutoShutdown` is ignored, and the `Significant` flag has no effect - every child termination triggers restart.

## Restart Intensity

Restarts aren't free. If a child crashes repeatedly, restarting it repeatedly just wastes resources. The `Intensity` and `Period` options limit restart frequency:

```go
Restart: act.SupervisorRestart{
    Strategy:  act.SupervisorStrategyTransient,
    Intensity: 5,   // Maximum 5 restarts
    Period:    10,  // Within 10 seconds
}
```

The supervisor tracks restart timestamps (in milliseconds). When a child terminates and needs restart, the supervisor checks: have there been more than `Intensity` restarts in the last `Period` seconds? If yes, the restart intensity is exceeded.

When the intensity is exceeded the supervisor stops all running children and terminates itself:
- Each child receives `act.ErrSupervisorRestartsExceeded` as its exit reason.
- The supervisor itself exits with `*gen.Error{Msg: "restart intensity exceeded", Inner: <original child reason>}`.
- A parent supervisor or monitor can call `errors.Unwrap(reason)` to recover the original child cause.

Old restarts outside the period window are discarded from tracking. This is a sliding window: if your child crashes 5 times in 10 seconds, then runs stable for 11 seconds, then crashes again, the counter resets. It is 1 restart in the window, not 6 total.

Default values are `Intensity: 5` and `Period: 5` if you don't specify them.

The supervisor-level counter is shared across all children that don't opt in to a per-child counter. To give an individual child its own restart budget, see [Per-Child Restart Control](#per-child-restart-control).

## Per-Child Restart Control

Most supervisors only need the supervisor-level `Restart`. But when you need fine-grained control (one child should be allowed to fail without taking down the rest, different children need different restart semantics, or you want a Simple One For One pool where a misbehaving instance doesn't kill the pool), each child can override the defaults via the optional `Restart` field on `SupervisorChildSpec`:

```go
type SupervisorChildRestart struct {
    Strategy   SupervisorStrategy
    Intensity  uint16
    Period     uint16
    OnExceed   OnExceed
}
```

Three independent axes, all opt-in. A zero-value `SupervisorChildRestart` means "inherit everything from the supervisor", so existing specs continue to work unchanged.

### Per-Child Strategy Override

`SupervisorStrategyInherit` is the zero-value sentinel for the `Strategy` field. At the child level it means "use the supervisor's Strategy", which is exactly the behavior of any child spec without an explicit per-child Restart. (At the supervisor level, `Inherit` is normalized to `SupervisorStrategyTransient` on init.)

To override, mix and match restart strategies in one supervisor:

```go
SupervisorSpec{
    Type:    act.SupervisorTypeOneForOne,
    Restart: act.SupervisorRestart{Strategy: act.SupervisorStrategyPermanent},
    Children: []act.SupervisorChildSpec{
        {Name: "core", Factory: createCore},
        {
            Name:    "diagnostics",
            Factory: createDiagnostics,
            Restart: act.SupervisorChildRestart{
                Strategy: act.SupervisorStrategyTemporary,
            },
        },
        {
            Name:    "logger",
            Factory: createLogger,
            Restart: act.SupervisorChildRestart{
                Strategy: act.SupervisorStrategyTransient,
            },
        },
    },
}
```

`core` inherits Permanent and is always restarted. `diagnostics` is Temporary and runs once. `logger` is Transient and stops only on a clean exit.

For All For One and Rest For One supervisors, the per-child Strategy controls whether *this child's* termination is treated as a trigger for the group restart:
- A Permanent child terminating (any reason) triggers the group restart.
- A Transient child terminating abnormally triggers it. A normal exit removes the child without triggering anything.
- A Temporary child terminating just removes the child. Siblings keep running.

This matches OTP semantics: in a coupled group, you can mark some children as "coupled" (Permanent / Transient) and others as "best-effort" (Temporary).

### Per-Child Restart Counter

Setting `Intensity > 0` gives a child its own restart counter, separate from the supervisor's global counter:

```go
Restart: act.SupervisorChildRestart{
    Intensity: 5,
    Period:    60,
}
```

For One For One the counter is per-spec: every restart of this child (regardless of how many times other children flap) counts only against this child's budget. For Simple One For One the counter is per-instance, where a *logical instance* is the lifetime of one `StartChild` call: its `args`, its restart history, and the chain of PIDs across restarts. Each `StartChild` invocation creates a new logical instance with its own counter; the counter survives across restarts of that same logical instance, even though the PID changes on each restart.

For All For One and Rest For One a per-child `Intensity` is rejected at supervisor init with `act.ErrSupervisorInvalidSpec`. Group-restart semantics make per-child thresholds meaningless: when one child fails, the supervisor restarts the whole group, so charging a per-child counter is undefined.

Children with `Intensity == 0` (the default) keep using the supervisor's global counter, exactly as before. You can mix freely: some children with their own counters, others sharing the global one, in the same supervisor.

### OnExceed

When a per-child counter overflows, the default reaction is the same as for the global counter: terminate the supervisor. Sometimes you want the opposite. A noisy non-critical child should be quietly disabled while its siblings keep running.

Set `OnExceed: act.OnExceedDisable`:

```go
Restart: act.SupervisorChildRestart{
    Intensity: 5,
    Period:    60,
    OnExceed:  act.OnExceedDisable,
}
```

Behavior on overflow:
- For One For One: the child spec is marked `disabled` and the supervisor stays alive. Other children are unaffected. Re-enable later with `EnableChild`, which clears the child's local counter.
- For Simple One For One: the offending instance is dropped from the supervisor. The spec stays available for new `StartChild` calls. Other instances of the same spec are unaffected.

`OnExceedDisable` requires `Intensity > 0`. Setting `OnExceedDisable` without a per-child counter is rejected at init (there is no counter to overflow).

The default value `OnExceedTerminateSupervisor` mirrors the supervisor-level behavior. When a per-child counter with this setting overflows, the supervisor terminates with `*gen.Error{Msg: "restart intensity exceeded", Inner: <original child reason>}`, the same wrap as the global-counter overflow.

### Validation Rules

The supervisor rejects the following at `Init` with `act.ErrSupervisorInvalidSpec`:
- `Intensity > 0` for All For One or Rest For One.
- `OnExceed: OnExceedDisable` without `Intensity > 0`.
- `Period > 0` without `Intensity > 0`.
- Unknown Strategy value.

Errors are wrapped, so `errors.Is(err, act.ErrSupervisorInvalidSpec)` matches.

### Default Behavior is Preserved

Adding `SupervisorChildRestart` is purely additive. A zero-value `Restart` field means "inherit everything", which is exactly the behavior every existing spec relies on:
- Strategy inherits from the supervisor.
- The supervisor's global counter is used.
- On global overflow, the supervisor terminates as it always did.

Setting per-child Restart on one child does not change behavior for any other child.

### Important Caveat: Global Counter Always Wins

A per-child counter does not protect that child from a global overflow. If one child without a per-child counter floods the supervisor's global counter past `Intensity`, the supervisor terminates the whole subtree. Children configured with `OnExceedDisable` are also terminated as part of that shutdown.

For full isolation, give every child its own `Intensity`, or set the supervisor-level `Intensity` high enough to absorb any expected noise.

## Failure Isolation Patterns

Three patterns cover most cases where one child's failure should not bring down the supervisor.

### Pattern 1: One Dispensable Child, Several Critical Ones

You have a supervisor where most children are critical, but one is allowed to fail. Telemetry, optional caches, background metrics collectors are typical examples.

```go
SupervisorSpec{
    Type: act.SupervisorTypeOneForOne,
    Restart: act.SupervisorRestart{
        Strategy:  act.SupervisorStrategyPermanent,
        Intensity: 5,
        Period:    5,
    },
    Children: []act.SupervisorChildSpec{
        {Name: "database", Factory: createDB},
        {Name: "api", Factory: createAPI},
        {
            Name:    "telemetry",
            Factory: createTelemetry,
            Restart: act.SupervisorChildRestart{
                Intensity: 100,
                Period:    60,
                OnExceed:  act.OnExceedDisable,
            },
        },
    },
}
```

`database` and `api` share the supervisor's global counter. Five failures of either one in 5 seconds kills the subtree, and a parent supervisor will rebuild it.

`telemetry` runs on its own counter (100 restarts in 60 seconds is a high tolerance, on purpose). When the telemetry pipeline degrades and starts crashing repeatedly, only `telemetry` is dropped. The rest of the application keeps serving requests.

To bring telemetry back later (after fixing the underlying issue, or after a config flag change):

```go
sup.EnableChild("telemetry")  // clears the local counter, spawns a fresh instance
```

### Pattern 2: Worker Pool Where One Bad Task Doesn't Kill the Pool

You have a Simple One For One pool where each instance handles a different task. A poison-pill input that crashes one worker should not take down all the others.

```go
SupervisorSpec{
    Type: act.SupervisorTypeSimpleOneForOne,
    Restart: act.SupervisorRestart{
        Strategy: act.SupervisorStrategyTransient,
    },
    Children: []act.SupervisorChildSpec{{
        Name:    "worker",
        Factory: createWorker,
        Restart: act.SupervisorChildRestart{
            Intensity: 5,
            Period:    10,
            OnExceed:  act.OnExceedDisable,
        },
    }},
}
```

```go
sup.StartChild("worker", taskA)  // first instance, args = taskA
sup.StartChild("worker", taskB)  // second instance, args = taskB
```

Each instance keeps its own counter, linked to its `args`. The counter survives across restarts of the same logical instance: if `taskA` panics once and is restarted, the counter is at 1; if it panics again, the counter is at 2; and so on.

If `taskA` crashes 5 times in 10 seconds, only that instance is dropped. `taskB` is untouched, and `StartChild("worker", taskC)` will spawn a fresh instance with a fresh counter at any time.

This is the canonical pattern for per-request actors, per-connection handlers, per-task workers. One bad input should never cascade into a full pool wipeout.

### Pattern 3: Mixed Restart Semantics

You have a supervisor where different children deserve different rules. Some are critical and must always run, some can finish their work cleanly, some are one-shot.

```go
SupervisorSpec{
    Type: act.SupervisorTypeOneForOne,
    Restart: act.SupervisorRestart{
        Strategy: act.SupervisorStrategyPermanent,
    },
    Children: []act.SupervisorChildSpec{
        {Name: "watchdog", Factory: createWatchdog},
        {
            Name:    "batch",
            Factory: createBatch,
            Restart: act.SupervisorChildRestart{
                Strategy: act.SupervisorStrategyTransient,
            },
        },
        {
            Name:    "init_task",
            Factory: createInitTask,
            Restart: act.SupervisorChildRestart{
                Strategy: act.SupervisorStrategyTemporary,
            },
        },
    },
}
```

`watchdog` inherits Permanent, so it is always restarted. `batch` is Transient, so a normal exit removes it (the work is done) but a crash restarts it. `init_task` is Temporary, so it runs once at supervisor startup and then stays gone.

For All For One or Rest For One supervisors the same per-child Strategy override controls *triggering*: a Temporary child can fail without triggering a group restart, while a Permanent child's failure always does.

## Mailbox Preservation Across Restart

By default, when a process restarts after a failure, its mailbox is reset to empty. Any messages that arrived before the failure but were not yet processed are lost. For some children this is fine. For others (workers mid-task, request handlers with queued work) losing those messages means losing work.

The framework provides an opt-in mechanism to carry a dying process's mailbox over to its restarted incarnation, so the new instance picks up exactly where the old one left off (minus the one message that triggered the failure, which is treated as already consumed).

### Enabling Mailbox Preservation

Set `Options.PreserveMailbox: true` on the child spec:

```go
SupervisorChildSpec{
    Name:    "worker",
    Factory: createWorker,
    Options: gen.ProcessOptions{
        PreserveMailbox: true,
    },
}
```

That single flag is the entire user-facing knob. The framework handles the rest: when the worker terminates abnormally, the runtime captures its mailbox into a `*gen.Error` exit reason. The supervisor extracts the mailbox from that reason and hands it to the next spawn. The new incarnation begins life with the surviving messages already in its queues, in their original priority order.

### What Triggers Preservation

The runtime captures the mailbox on any abnormal termination:

- panic in a callback,
- callback returning an arbitrary error,
- forced `Kill`,
- exit signal cascade from a linked process that died abnormally.

Normal exits (`gen.TerminateReasonNormal` and `gen.TerminateReasonShutdown`) do not trigger capture. A clean shutdown means the actor explicitly decided to stop, and reusing its mailbox on the next incarnation would contradict that decision.

### Restrictions

- **Only One For One and Simple One For One.** All For One and Rest For One use group-restart semantics: when one child fails, every sibling is torn down and rebuilt together. Preserving the mailbox of one specific child while wiping the siblings' state is contradictory. The supervisor rejects `PreserveMailbox: true` on All For One / Rest For One children at init with `act.ErrSupervisorInvalidSpec`.

- **The triggering message is not replayed.** The message the actor was processing when it failed has already been popped from the queue. It is considered consumed (with an error). Replaying it would create restart loops on poison-pill messages: each retry would fail again until restart intensity intervenes. The framework deliberately drops the triggering message and resumes from the next one in queue.

- **In-process state is not preserved.** Only the mailbox queues survive. Any state held in the actor's struct fields is gone. The new incarnation runs `Init` from scratch and then starts pulling surviving messages.

### Example: SOFO Worker Pool with Per-Task Mailbox Preservation

```go
SupervisorSpec{
    Type: act.SupervisorTypeSimpleOneForOne,
    Restart: act.SupervisorRestart{
        Strategy: act.SupervisorStrategyTransient,
    },
    Children: []act.SupervisorChildSpec{{
        Name:    "task_worker",
        Factory: createTaskWorker,
        Options: gen.ProcessOptions{
            PreserveMailbox: true,
        },
        Restart: act.SupervisorChildRestart{
            Intensity: 5,
            Period:    10,
            OnExceed:  act.OnExceedDisable,
        },
    }},
}
```

Each `StartChild` invocation creates a worker instance with its own restart counter. When a worker panics, its mailbox is captured and handed to the restart. The new incarnation continues processing the queued tasks. If the same logical instance keeps panicking past its budget (5 failures in 10 seconds), `OnExceedDisable` drops just that instance while the rest of the pool keeps serving. Without `OnExceedDisable`, the supervisor itself would terminate.

### Network Boundary

A live mailbox cannot cross the network. The `Mailbox` fields on `gen.Error` and `gen.ProcessOptions` are excluded from EDF wire encoding via the `edf:"-"` tag. On remote spawn or remote exit signals, those fields are zero-valued (nil) on the receiving side. Mailbox preservation is a same-node feature.

## Significant Children

In All For One and Rest For One supervisors, the `Significant` flag marks children whose termination can trigger supervisor shutdown:

```go
Children: []act.SupervisorChildSpec{
    {
        Name:        "critical_service",
        Factory:     createCriticalService,
        Significant: true,  // If this stops cleanly, supervisor stops
    },
    {
        Name:    "helper",
        Factory: createHelper,
        // Significant: false (default)
    },
},
```

With `SupervisorStrategyTransient`:
- Significant child terminates **normally** → supervisor stops all children and terminates
- Significant child **crashes** → restart strategy applies
- Non-significant child → restart strategy applies regardless of termination reason

With `SupervisorStrategyTemporary`:
- Significant child terminates (any reason) → supervisor stops all children and terminates
- Non-significant child → no restart, child stays terminated

With `SupervisorStrategyPermanent`:
- `Significant` flag is ignored
- All terminations trigger restart

For One For One and Simple One For One, `Significant` is always ignored.

Use significant children when a specific child's clean termination means "mission accomplished, shut down the subtree." Example: a batch processor that finishes its work and terminates normally should stop the entire supervision tree, not get restarted.

## Auto Shutdown

By default, if all children terminate normally (not crashes) and none are significant, the supervisor stops itself with `gen.TerminateReasonNormal`. This is auto shutdown.

```go
DisableAutoShutdown: false,  // Default - supervisor stops when children stop
```

Enable `DisableAutoShutdown` to keep the supervisor running even with zero children:

```go
DisableAutoShutdown: true,  // Supervisor stays alive with zero children
```

Auto shutdown is ignored for Simple One For One supervisors (they're designed for dynamic children) and ignored when using Permanent strategy.

Use auto shutdown when your supervisor's purpose is managing those specific children. When they're all gone, the supervisor has no purpose. Disable it when the supervisor manages dynamically added children or should stay alive to accept management commands.

## Keep Order

For All For One and Rest For One, the `KeepOrder` flag controls how children are stopped:

```go
Restart: act.SupervisorRestart{
    KeepOrder: true,  // Stop sequentially in reverse order
}
```

With `KeepOrder: true`:
- Children stop one at a time, last to first
- Supervisor waits for each child to fully terminate before stopping the next
- Slow but orderly - useful when children have shutdown dependencies

With `KeepOrder: false` (default):
- All affected children receive `SendExit` simultaneously
- They terminate in parallel
- Fast but unordered - use when children can shut down independently

After stopping (either way), children restart sequentially in declaration order. `KeepOrder` only affects stopping, not starting.

For One For One and Simple One For One, `KeepOrder` is ignored (only one child is affected).

## Dynamic Management

Supervisors provide methods for runtime adjustments:

```go
// Start a child from the spec (if not already running)
err := supervisor.StartChild("worker")

// Start with different args (overrides spec)
err := supervisor.StartChild("worker", "new-config")

// Add a new child spec and start it
err := supervisor.AddChild(act.SupervisorChildSpec{
    Name:    "new_worker",
    Factory: createWorker,
})

// Disable a child (stops it, won't restart on crash)
err := supervisor.DisableChild("worker")

// Re-enable a disabled child (starts it again)
err := supervisor.EnableChild("worker")

// Get list of children
children := supervisor.Children()
for _, child := range children {
    fmt.Printf("Spec: %s, PID: %s, Disabled: %v\n", 
        child.Spec, child.PID, child.Disabled)
}
```

**Critical**: These methods fail with `act.ErrSupervisorStrategyActive` if called while the supervisor is executing a restart strategy (stopping children, waiting for their exit signals, or starting replacements). You must wait for the strategy to finish before issuing management calls.

While a restart strategy is running, the supervisor processes only the Urgent queue (where exit signals arrive) and ignores System and Main queues. This guarantees exit signals are handled promptly without interference from management commands or regular messages.

For Simple One For One supervisors, `StartChild` with args stores those args for that specific child instance. When that instance restarts (due to crash, kill, etc.), it uses the stored args, not the template args from the spec. For other supervisor types (One For One, All For One, Rest For One), `StartChild` with args updates the spec's args for future restarts.

## Child Callbacks

Enable `EnableHandleChild: true` to receive notifications when children start or stop:

```go
func (s *AppSupervisor) Init(args ...any) (act.SupervisorSpec, error) {
    return act.SupervisorSpec{
        EnableHandleChild: true,
        // ... rest of spec
    }, nil
}

func (s *AppSupervisor) HandleChildStart(name gen.Atom, pid gen.PID) error {
    s.Log().Info("child %s started with PID %s", name, pid)
    // Maybe register in service discovery, send init message
    return nil
}

func (s *AppSupervisor) HandleChildTerminate(name gen.Atom, pid gen.PID, reason error) error {
    s.Log().Info("child %s (PID %s) terminated: %s", name, pid, reason)
    // Maybe deregister from service discovery, clean up resources
    return nil
}
```

These callbacks run **after** the restart strategy completes. For example:
1. Child crashes
2. Supervisor applies restart strategy (stops affected children if needed)
3. Supervisor starts replacement children
4. **Then** `HandleChildTerminate` is called for the terminated child
5. **Then** `HandleChildStart` is called for the replacement

The callbacks are invoked as regular messages sent by the supervisor to itself. They arrive in the Main queue, so they're processed after the restart logic (which happens in the exit signal handler).

If `HandleChildStart` or `HandleChildTerminate` returns an error, the supervisor terminates with that error. Use these callbacks for integration with external systems, not for restart decisions - restart logic is handled by the supervisor type and strategy.

## Supervisor as a Regular Actor

Supervisors are actors. They have mailboxes, handle messages, and can communicate with other processes:

```go
func (s *AppSupervisor) HandleMessage(from gen.PID, message any) error {
    switch msg := message.(type) {
    case ScaleCommand:
        if msg.Up {
            s.AddWorkers(msg.Count)
        } else {
            s.RemoveWorkers(msg.Count)
        }
    
    case HealthCheckRequest:
        children := s.Children()
        s.Send(from, HealthResponse{
            Running: len(children),
            Healthy: s.countHealthy(children),
        })
    }
    return nil
}

func (s *AppSupervisor) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
    switch request.(type) {
    case GetChildrenRequest:
        return s.Children(), nil
    }
    return nil, nil
}
```

This lets you build management APIs: query supervisor state, scale children dynamically, reconfigure at runtime. The supervisor processes these messages between handling exit signals.

## Observer Integration

Supervisors provide runtime inspection via the `HandleInspect` method, which is automatically integrated with the Observer monitoring tool. When you call `gen.Process.Inspect()` on a supervisor, it returns detailed metrics about its current state:

**One For One / All For One / Rest For One:**
- `type`: Supervisor type ("One For One", "All For One", "Rest For One")
- `strategy`: Restart strategy (Transient, Temporary, Permanent)
- `intensity`: Maximum restart count within period
- `period`: Time window in seconds for restart intensity
- `keep_order`: Whether children stop sequentially (All/Rest For One only)
- `auto_shutdown`: Whether supervisor stops when all children terminate
- `restarts_count`: Number of supervisor-level restart timestamps currently tracked
- `children_total`: Total child specs defined
- `children_running`: Currently running children
- `children_disabled`: Disabled children that won't restart
- `child:<name>:restarts`: Per-child restart count, only present for children with `Intensity > 0` in their `SupervisorChildRestart`

**Simple One For One:**
- `type`: "Simple One For One"
- `strategy`: Restart strategy
- `intensity`: Maximum restart count within period
- `period`: Time window in seconds
- `restarts_count`: Number of supervisor-level restart timestamps tracked
- `specs_total`: Total child spec templates
- `specs_disabled`: Disabled specs
- `instances_total`: Total running instances across all specs
- `child:<name>`: Number of running instances for that child spec
- `child:<name>:args`: Number of instances with custom args for that child spec
- `child:<name>:restarts`: Aggregated per-instance restart count for that spec, only present when the spec has `Intensity > 0` in its `SupervisorChildRestart`

**Restart history (all supervisor types):**

- `history:count`: Number of restart events currently kept in the ring buffer
- `history:<N>:time`: RFC3339Nano timestamp of restart event N (oldest at index 0)
- `history:<N>:child`: Spec name of the child that triggered this restart
- `history:<N>:reason`: `Error()` string of the termination reason

The history captures up to 50 recent restart decisions and is the fastest path to diagnose "why is this subtree flapping" without parsing logs. For All For One / Rest For One supervisors only the triggering child is recorded, not the cascading sibling kills.

The Observer UI displays this information in real-time, letting you monitor supervision trees, track restart patterns, and identify failing components. You can also query this data programmatically:

```go
// From within a process context
info, err := process.Inspect(supervisorPID)

// Directly from the node
info, err := node.Inspect(supervisorPID)

// Returns map[string]string with metrics above
```

Both methods only work for local supervisors (same node). This integration makes it easy to diagnose issues in production: check restart counts to identify unstable processes, verify child counts match expected scaling, monitor which instances have custom configurations.

## Restart Intensity Behavior

Understanding restart intensity is critical for reliable systems. Here's exactly how it works:

The supervisor maintains a list of restart timestamps in milliseconds. When a child terminates and restart is needed:

1. Append current timestamp to the list.
2. Remove timestamps older than `Period` seconds.
3. If list length > `Intensity`, intensity is exceeded.
4. If exceeded: stop all running children with `act.ErrSupervisorRestartsExceeded` as their exit reason. The supervisor itself terminates with `*gen.Error{Msg: "restart intensity exceeded", Inner: <original child reason>}`. The original failure cause is preserved via `errors.Unwrap` so a parent supervisor can introspect it.
5. If not exceeded: proceed with restart.

When a per-child counter is configured, the same algorithm runs against the child's own restart history using the child's own `Intensity` and `Period`. With `OnExceed: OnExceedDisable`, step 4 changes: instead of terminating the supervisor, the child is disabled (One For One) or the offending instance is dropped (Simple One For One), and the supervisor stays alive. With `OnExceedTerminateSupervisor` (the default), step 4 produces the same `*gen.Error` wrap as the global path.

Example with `Intensity: 3, Period: 5`:

```
Time 0s:  Child crashes → restart (count: 1)
Time 1s:  Child crashes → restart (count: 2)
Time 2s:  Child crashes → restart (count: 3)
Time 3s:  Child crashes → EXCEEDED (count: 4 within 5s window)
          → Stop all children, supervisor terminates
```

But if the child runs stable between crashes:

```
Time 0s:  Child crashes → restart (count: 1)
Time 6s:  Child crashes → restart (count: 1, previous outside window)
Time 12s: Child crashes → restart (count: 1, previous outside window)
```

The sliding window means intermittent failures don't accumulate. Only rapid repeated failures exceed intensity.

## Shutdown Behavior

When a supervisor terminates (receives exit signal, calls terminate from `HandleMessage`, or crashes), it stops all children first:

1. Send `gen.TerminateReasonShutdown` via `SendExit` to all running children
2. Wait for all children to terminate
3. Call `Terminate` callback
4. Remove supervisor from node

With `KeepOrder: true` (All For One / Rest For One), children stop sequentially. With `KeepOrder: false`, they stop in parallel. Either way, the supervisor waits for all to finish before terminating itself.

If a non-child process sends the supervisor an exit signal (via `Link` or `SendExit`), the supervisor initiates shutdown. This is how parent supervisors stop child supervisors - send an exit signal, and the entire subtree shuts down cleanly.

## Dynamic Children (Simple One For One)

Simple One For One supervisors start with empty children and spawn them on demand:

```go
Type: act.SupervisorTypeSimpleOneForOne,
Children: []act.SupervisorChildSpec{
    {
        Name:    "worker",  // Template name
        Factory: createWorker,
        Args:    []any{"default-config"},
    },
},
```

Start instances with `StartChild`:

```go
// Start 10 workers with different args
for i := 0; i < 10; i++ {
    supervisor.StartChild("worker", fmt.Sprintf("worker-%d", i))
}
```

Each call spawns a new worker. The `args` passed to `StartChild` are stored for that specific instance. When the restart strategy triggers (child crashes, exceeds intensity, etc.), the child restarts with the same args it was originally started with, not the template args from the spec. This ensures each worker instance maintains its configuration across restarts.

Workers are not registered by name (no `SpawnRegister`). You track them by PID from the return value or via `supervisor.Children()`.

Disabling a child spec stops **all** running instances with that spec name:

```go
// Stops all "worker" instances
supervisor.DisableChild("worker")
```

Simple One For One ignores `DisableAutoShutdown` - the supervisor never auto-shuts down, even with zero children. It's designed for dynamic workloads where zero children is a valid state.

## Patterns and Pitfalls

**Default Strategy is Transient**. The supervisor-level `Strategy` zero value is `SupervisorStrategyInherit`, which is normalized to `SupervisorStrategyTransient` on init. Children with no explicit `Restart` field inherit Transient. To change the default for the whole supervisor, set `Strategy` explicitly on `SupervisorRestart`.

**Set restart intensity carefully**. Too low and transient failures kill your supervisor. Too high and crash loops consume resources. Start with defaults (`Intensity: 5, Period: 5`) and tune based on observed behavior.

**Use Significant sparingly**. Marking a child significant couples its lifecycle to the entire supervision tree. This is powerful but reduces isolation. Prefer non-significant children and handle critical failures at a higher supervision level.

**Don't call management methods during restart**. `StartChild`, `AddChild`, `EnableChild`, `DisableChild` fail with `ErrSupervisorStrategyActive` if the supervisor is mid-restart. Wait for the restart to complete (check via `Inspect` or wait for `HandleChildStart` callback).

**Disable auto shutdown for dynamic supervisors**. If your supervisor uses `AddChild` to add children at runtime, enable `DisableAutoShutdown`. Otherwise, it terminates when it starts with zero children or when all dynamically added children eventually stop.

**Use HandleChildStart for integration, not validation**. By the time `HandleChildStart` is called, the child is already spawned and linked. Returning an error terminates the supervisor, but doesn't prevent the child from running. Use child's `Init` for validation instead.

**KeepOrder is only for stopping**. Children always start sequentially in declaration order. `KeepOrder` controls only the stopping phase of All For One and Rest For One restarts.

**Simple One For One args are persistent per instance**. Args passed to `StartChild` are stored and used for that specific instance across all restarts. If you start a worker with `StartChild("worker", "config-A")` and it crashes, the restarted instance receives "config-A" again, not the template args from the child spec. This persistence ensures each worker maintains its identity and configuration through failures. If you need different args for a restart, you must manually stop the old instance and start a new one with different args.

**Per-child counter does not protect from a global overflow**. A child with `OnExceedDisable` is still terminated as a side effect when another child overflows the supervisor's global counter. If you need a child to truly survive other children's failures, give every child a per-child `Intensity`, or raise the supervisor-level `Intensity` enough to absorb the noise.

**`OnExceedDisable` requires `Intensity > 0`**. Setting `OnExceed` without a per-child counter is rejected at init. The reasoning: there is no per-child counter to overflow, and applying Disable on the global counter would be ambiguous (which child should be disabled?).

**Per-child `Intensity` is rejected for All For One and Rest For One**. Group-restart strategies have no use for per-child thresholds: when one child fails, the supervisor restarts the whole group, so charging a per-child counter has no defined meaning.

**Use `errors.Is` and `errors.Unwrap` to inspect failures**. When a supervisor terminates due to a restart-intensity overflow, its exit reason is `*gen.Error{Msg: "restart intensity exceeded", Inner: <original child reason>}`. A parent supervisor or monitor can match the structural cause with `errors.Is(reason, act.ErrSupervisorRestartsExceeded)` (where applicable) and recover the underlying child failure with `errors.Unwrap(reason)`.

## Behavior Cookbook

By the time you reach this section every term in the table below has been introduced. Use it as a quick reference: pick the row that matches the behavior you want and apply the combination on the right.

| If you want... | Combination |
|---|---|
| Independent children. Supervisor dies if any one flaps too much. | `Type: SupervisorTypeOneForOne` and supervisor-level `Restart`. No per-child override. |
| All children coupled. Any failure restarts the whole group. | `Type: SupervisorTypeAllForOne` and supervisor-level `Restart`. |
| Dependency chain. Failure restarts this child and every child after it. | `Type: SupervisorTypeRestForOne` and supervisor-level `Restart`. |
| Dynamic pool with one shared restart budget. | `Type: SupervisorTypeSimpleOneForOne`. |
| Dynamic pool of one-shot workers (fire-and-forget). | `Type: SupervisorTypeSimpleOneForOne` and supervisor `Strategy: SupervisorStrategyTemporary`. |
| One child is allowed to degrade and stay disabled while siblings keep running. | `Type: SupervisorTypeOneForOne` plus per-child `Restart: SupervisorChildRestart{Intensity, Period, OnExceed: OnExceedDisable}`. Re-enable later with `EnableChild`. |
| Pool where one bad instance is dropped while the pool keeps serving. | `Type: SupervisorTypeSimpleOneForOne` plus per-child `Restart: SupervisorChildRestart{Intensity, Period, OnExceed: OnExceedDisable}`. |
| One child has its own restart budget but overflow still terminates the supervisor. | `Type: SupervisorTypeOneForOne` or `SupervisorTypeSimpleOneForOne` plus per-child `Restart: SupervisorChildRestart{Intensity, Period}`. Default `OnExceed` terminates the supervisor. |
| Child that runs once and stays gone. | Per-child `Restart: SupervisorChildRestart{Strategy: SupervisorStrategyTemporary}`. |
| Child that always restarts, even on Normal exit. | Per-child `Restart: SupervisorChildRestart{Strategy: SupervisorStrategyPermanent}`. |
| All For One or Rest For One child whose abnormal exit must trigger a group restart. | Per-child `Strategy: SupervisorStrategyTransient` (default) or `SupervisorStrategyPermanent`. |
| All For One or Rest For One child whose death must not trigger a group restart. | Per-child `Restart: SupervisorChildRestart{Strategy: SupervisorStrategyTemporary}`. |
| Clean exit of one child ends the whole subtree. | `Type: SupervisorTypeAllForOne` or `SupervisorTypeRestForOne`, supervisor `Strategy: SupervisorStrategyTransient`, per-child `Significant: true`. |
| Supervisor stays alive with zero children (used to manage children added at runtime). | `DisableAutoShutdown: true`. |
| Worker resumes its queued messages after a panic restart. | `Type: SupervisorTypeOneForOne` or `SupervisorTypeSimpleOneForOne`, child `Options: gen.ProcessOptions{PreserveMailbox: true}`. |

Most rows are composable in a single supervisor: different children can have different per-child Restart, per-child Restart can combine with `Significant`, and `DisableAutoShutdown` is orthogonal to everything above. Only the per-child `Intensity` field is exclusive to `SupervisorTypeOneForOne` and `SupervisorTypeSimpleOneForOne`.
