# Supervisor

Actors fail. They panic, encounter errors, or lose external resources. In traditional systems, you add defensive code: catch exceptions, retry operations, validate state. This spreads failure handling throughout your codebase, mixing recovery logic with business logic.

The actor model takes a different approach: let it crash. When an actor fails, terminate it cleanly and restart it in a known-good state. This requires something watching the actor and managing its lifecycle - a supervisor.

`act.Supervisor` is an actor that manages child processes. It starts them during initialization, monitors them for failures, and applies restart strategies when they terminate. Supervisors can manage other supervisors, creating hierarchical fault tolerance trees where failures are isolated and recovered automatically.

Like `act.Actor`, the `act.Supervisor` struct implements the low-level `gen.ProcessBehavior` interface and has the embedded `gen.Process` interface. To create a supervisor, you embed `act.Supervisor` in your struct and implement the `act.SupervisorBehavior` interface:

```go
type SupervisorBehavior interface {
    gen.ProcessBehavior

    // Init invoked on supervisor spawn - MANDATORY
    Init(args ...any) (SupervisorSpec, error)

    // HandleChildStart invoked when a child starts (if EnableHandleChild is true)
    HandleChildStart(name gen.Atom, pid gen.PID) error

    // HandleChildTerminate invoked when a child terminates (if EnableHandleChild is true)
    HandleChildTerminate(name gen.Atom, pid gen.PID, reason error) error

    // HandleMessage invoked for regular messages
    HandleMessage(from gen.PID, message any) error

    // HandleCall invoked for synchronous requests
    HandleCall(from gen.PID, ref gen.Ref, request any) (any, error)

    // HandleEvent invoked for subscribed events
    HandleEvent(message gen.MessageEvent) error

    // HandleInspect invoked for inspection requests
    HandleInspect(from gen.PID, item ...string) map[string]string

    // Terminate invoked on supervisor termination
    Terminate(reason error)
}
```

Only `Init` is mandatory. All other methods are optional - `act.Supervisor` provides default implementations that log warnings. The `Init` method returns `SupervisorSpec` which defines the supervisor's behavior, children, and restart strategy.

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

The supervisor spawns all children during `Init` (except Simple One For One, which starts with zero children). Each child is linked bidirectionally to the supervisor (`LinkChild` and `LinkParent` set automatically). If a child terminates, the supervisor receives an exit signal and applies the restart strategy.

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

## Restart Strategies

The `Restart.Strategy` field determines when children are restarted.

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

The supervisor tracks restart timestamps (in milliseconds). When a child terminates and needs restart, the supervisor checks: have there been more than `Intensity` restarts in the last `Period` seconds? If yes, the restart intensity is exceeded. The supervisor stops all children and terminates itself with `act.ErrSupervisorRestartsExceeded`.

Old restarts outside the period window are discarded from tracking. This is a sliding window: if your child crashes 5 times in 10 seconds, then runs stable for 11 seconds, then crashes again - the counter resets. It's 1 restart in the window, not 6 total.

Default values are `Intensity: 5` and `Period: 5` if you don't specify them.

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

**Critical**: These methods fail with `act.ErrSupervisorStrategyActive` if called while the supervisor is executing a restart strategy. The supervisor is in `supStateStrategy` mode - it's stopping children, waiting for exit signals, or starting replacements. You must wait for it to return to `supStateNormal` before making management calls.

When the supervisor is applying a strategy, it processes only the Urgent queue (where exit signals arrive) and ignores System and Main queues. This ensures exit signals are handled promptly without interference from management commands or regular messages.

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
- `restarts_count`: Number of restart timestamps currently tracked
- `children_total`: Total child specs defined
- `children_running`: Currently running children
- `children_disabled`: Disabled children that won't restart

**Simple One For One:**
- `type`: "Simple One For One"
- `strategy`: Restart strategy
- `intensity`: Maximum restart count within period
- `period`: Time window in seconds
- `restarts_count`: Number of restart timestamps tracked
- `specs_total`: Total child spec templates
- `specs_disabled`: Disabled specs
- `instances_total`: Total running instances across all specs
- `child:<name>`: Number of running instances for specific child spec
- `child:<name>:args`: Number of instances with custom args for specific child spec

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

1. Append current timestamp to the list
2. Remove timestamps older than `Period` seconds
3. If list length > `Intensity`, intensity is exceeded
4. If exceeded: stop all children, terminate supervisor with `act.ErrSupervisorRestartsExceeded`
5. If not exceeded: proceed with restart

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

**Set restart intensity carefully**. Too low and transient failures kill your supervisor. Too high and crash loops consume resources. Start with defaults (`Intensity: 5, Period: 5`) and tune based on observed behavior.

**Use Significant sparingly**. Marking a child significant couples its lifecycle to the entire supervision tree. This is powerful but reduces isolation. Prefer non-significant children and handle critical failures at a higher supervision level.

**Don't call management methods during restart**. `StartChild`, `AddChild`, `EnableChild`, `DisableChild` fail with `ErrSupervisorStrategyActive` if the supervisor is mid-restart. Wait for the restart to complete (check via `Inspect` or wait for `HandleChildStart` callback).

**Disable auto shutdown for dynamic supervisors**. If your supervisor uses `AddChild` to add children at runtime, enable `DisableAutoShutdown`. Otherwise, it terminates when it starts with zero children or when all dynamically added children eventually stop.

**Use HandleChildStart for integration, not validation**. By the time `HandleChildStart` is called, the child is already spawned and linked. Returning an error terminates the supervisor, but doesn't prevent the child from running. Use child's `Init` for validation instead.

**KeepOrder is only for stopping**. Children always start sequentially in declaration order. `KeepOrder` controls only the stopping phase of All For One and Rest For One restarts.

**Simple One For One args are persistent per instance**. Args passed to `StartChild` are stored and used for that specific instance across all restarts. If you start a worker with `StartChild("worker", "config-A")` and it crashes, the restarted instance receives "config-A" again, not the template args from the child spec. This persistence ensures each worker maintains its identity and configuration through failures. If you need different args for a restart, you must manually stop the old instance and start a new one with different args.
