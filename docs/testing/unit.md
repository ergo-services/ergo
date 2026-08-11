---
description: The in-process harness for testing a single actor's logic
---

# Unit

Most of what an actor does is decide. A message arrives; the actor looks at its state, perhaps asks a dependency something, and reacts - it replies, forwards, spawns a worker, logs a warning, or stops. That decision logic is the core of the actor, and it is what you most want under test: on its own, without a network, a scheduler, or the timing that makes concurrent tests flaky.

`unit` is built for exactly that. It spawns one behavior on a mock node, gives you a `Subject` to drive its callbacks by hand, and records everything the actor does so you can assert on it. The defining fact - the one everything else follows from - is that it is synchronous. There are no real goroutines and no clock: you deliver a message, the actor's handler runs to completion on the calling goroutine, and by the time the call returns the records are already there to read. Tests run in microseconds and give the same answer every time.

## The Shape of a Test

A unit test reads the same way every time: spawn the actor, drive an input, assert the reaction.

```go
sub, err := unit.Spawn(t, factoryWorker, gen.ProcessOptions{})
if err != nil {
    t.Fatal(err)
}

sub.SendMessage(client, "ping")
sub.ShouldSend().To(client).Message("pong").Once().Assert()
```

`unit.Spawn` runs the behavior's `Init` and returns a `Subject` - the actor under test. The `Subject` carries the assertion grammar from [check](check.md), which is why `sub.ShouldSend(...)` is a method on it. The options are the real `gen.ProcessOptions` (set the log level there, for instance, with `LogLevel`), and any trailing arguments are forwarded to `Init`, exactly as `gen.Node.Spawn` forwards them. When you need more than a default node - a node name, seeded environment, an injected dependency - build it first with `unit.StartNode(...)` and spawn on that; it mirrors how [stage](stage.md) reads.

Notice what did *not* happen after `SendMessage`: no wait. The handler ran inline, the send was recorded during that run, and the assertion read a finished result. Hold on to that - it is the whole reason unit tests are fast and never flake, and it is the one thing that changes when you move up to stage.

## Driving Inputs

`SendMessage` is one of a family of drivers, one for every way a message reaches an actor, so you can exercise each callback in isolation:

| Driver | Drives |
|--------|--------|
| `SendMessage` / `SendMessageName` / `SendMessageAlias` | `HandleMessage` and its name/alias split-handlers |
| `SendMessageWithPriority` | a message at a given queue priority |
| `Call` / `CallName` / `CallAlias` / `CallWithPriority` | `HandleCall`, returning the actor's reply |
| `DeliverExit` / `DeliverExitMessage` | an exit signal on the urgent queue |
| `DeliverDown` / `DeliverDownMessage` | a monitor's down notification |
| `DeliverEvent` / `DeliverRegistrarEvent` | `HandleEvent` |
| `DeliverLog` | `HandleLog` (actor registered as a logger) |
| `DeliverSpan` | `HandleSpan` (actor registered as a tracing exporter) |
| `Inspect` | `HandleInspect`, returning the reported map |
| `FireTimers` | scheduled `SendAfter` messages whose target is the actor |
| `FireCron` | a registered cron job's `gen.MessageCron` |

A request is the one driver that hands a value back: `Call` drives the actor's `HandleCall` and returns what it responded.

```go
resp, err := sub.Call(client, "status")
check.NoError(t, err)
check.Equal(t, "ready", resp)
```

Keep the direction straight here, because it is the most common source of confusion in unit tests: `Call` is you calling *into* the actor. Controlling what the actor's *own* outbound calls return is a different tool, `OnCall`, which comes up below. (One related subtlety: a message the actor sends to itself is recorded as an outgoing send, not looped back into its mailbox - to drive the reaction to it, deliver it yourself with `SendMessage`.)

## Setting Up the Actor's World

An actor never runs in a vacuum: it calls out to dependencies, and it reads things about itself and its node. To test it in isolation you control both sides of that world, and `unit` gives you a distinct tool for each. What the actor *does* outward - the calls and sends it makes - you shape with typed stubs. What the actor *reads* - its environment, its node, service discovery - you supply with overrides. Everything in the next two sections is one or the other; keeping that split in mind is most of what it takes to write a unit test confidently.

### Stubbing What the Actor Does

An actor that calls a dependency cannot be tested alone unless you decide what the call returns. `OnCall` does that - it intercepts the actor's outbound `Call` and answers it - which is what lets you drive both branches of the same handler:

```go
sub.OnCall("backend").Respond("OK")
sub.SendMessage(client, "ping")
sub.ShouldSend().To("client").Message("OK").Once().Assert()
```

Make the call fail, and the error branch runs instead:

```go
sub.OnCall("backend").Fail(gen.ErrTimeout)
sub.SendMessage(client, "ping")
sub.ShouldSend().To("logger").Message("backend failed").Once().Assert()
sub.ShouldSend().To("client").None().Assert() // the happy path did not run
```

Calls are not the only outward action with a return value. Spawning a child, allocating an alias, registering an event, spawning on a remote node - each has a typed stub for either outcome, and the error-only operations (`Send`, `Link`, `Monitor`, `SendExit`, and the like) take a `Fail`. `FailFunc` makes failure selective - a counter in the closure can fail only the second and fifth send while the rest succeed:

```go
sub.OnSpawn(factoryWorker).Fail(gen.ErrProcessTerminated)
sub.OnRemoteSpawn("peer@localhost", "svc").Return(remotePID)

i := 0
sub.OnSend("svc").FailFunc(func() error {
    i++
    if i == 2 || i == 5 {
        return gen.ErrProcessMailboxFull
    }
    return nil
})
```

Two things hold for every stub. Whatever it decides, the action is still recorded - the stub shapes the return value, it does not hide the send from `ShouldSend`. And a stub you never set is permissive: an unstubbed `Call` returns `(nil, nil)`, a value-producing operation returns a synthetic value, an error-only one succeeds. The single loud exception is an unstubbed *resolve* through the mock network, because a forgotten discovery stub is almost always a bug, not an intended default.

A stub only answers a call made after you set it. For a call from a handler that is enough - set the stub after spawn, before the input that triggers it, as above. But some actors call a dependency in `Init` itself: a supervisor that registers with a service as it starts, for one. By the time `Spawn` returns, `Init` has already run, so a stub set on the returned `Subject` is too late.

Split the spawn for that case. `Prepare` builds the actor and hands back its `Subject` without running `Init`; you set the stubs, then `Run` runs `Init` with them in place:

```go
sub := unit.Prepare(t, factoryRadarSup, gen.ProcessOptions{})
sub.OnCall("radar_health").Fail(gen.ErrProcessUnknown) // the service is not running
if err := sub.Run(); err != nil {
    t.Fatal(err)
}
```

`Spawn` is exactly `Prepare` followed by `Run`, so reach for the split only to stub before `Init`. Until `Run` the actor is not initialized: any driver fails the test loudly rather than run against a half-built actor, and calling `Run` twice fails the same way. The node has its own egress stubs (`unit.StartNode(t, ...).OnCall(...)`), a separate scope from the actor's: they shape the node's own outbound calls and do not reach the process under test, just as a meta's stubs are its own.

### Controlling What the Actor Reads

The mirror image is what the actor reads, and it comes from two sources: the actor reads things about itself, and it reads things from its node. Configure either after spawn, before the input that needs it.

What the actor reads about itself is an override on the `Subject`. Here the actor reads an environment value, and the test decides what it finds:

```go
sub.OnEnv(func(name gen.Env) (any, bool) { return "production", true })
```

Every non-egress method the actor calls on itself has such an override - `OnState`, `OnLog`, `OnUptime`, `OnInfo`, and the rest of its accessors; the godoc has the full set.

What the actor reads from its node is controlled the same way, through `sub.Node()`:

```go
sub.Node().OnIsAlive(func() bool { return false })
```

Service discovery is the node read that comes up most often. The node carries a built-in mock network; stub what the actor's resolver returns and you drive its routing decision with no registrar in sight:

```go
sub.Node().Network().Registrar().Resolver().
    OnResolveApplication("worker_app").
    Return(gen.ApplicationRoute{Node: "node1@localhost", State: gen.ApplicationStateRunning})
```

`FailRegistrar` drives the no-registrar branch, and `OnGetNode` returns a programmable remote node - reaching it is a read, but the `Spawn` the actor then issues on it is outward work, recorded and asserted as remote egress. Cron jobs the actor adds via `sub.Node().Cron().AddJob` are recorded and fire only when the test calls `FireCron`, so a scheduled action stays deterministic.

The node's type registry is modelled rather than stubbed away: `RegisterTypes` seeds it the way an application's `Load` does, `LookupType` and `RegisteredTypes` read it back, and `FailRegisterTypes` drives the rejected-registration branch. So "did this actor register the wire surface it needs" is assertable here instead of only in a system test. `LookupType` matches the canonical `#pkgpath/Name` key exactly, as a live node does - a short type name does not resolve.

## Reading What the Actor Produced

With the world set up and an input driven, you assert on the result. Beyond the record assertions you already know from [check](check.md), two things are worth calling out.

The PIDs `unit` hands back are honest. Every spawn gets a distinct, well-formed `gen.PID` under the node's name - spawn a hundred children and you get a hundred different PIDs, as a real node would. So you can capture a generated value and assert it flows correctly through later behavior:

```go
sub.SendMessage(client, "spawn-worker")
spawn, _ := sub.ShouldSpawn().Once().Capture()
sub.ShouldSend().To("manager").Message(spawn.Child).Once().Assert()
```

And termination is recorded. When a callback returns an error, panics, or returns a stop reason, the actor terminates; assert it with `ShouldTerminate`, or read it off the `Subject`:

```go
sub.SendMessage(client, "self-destruct")
sub.ShouldTerminate().Reason(errBoom).Once().Assert()
check.True(t, sub.Terminated())
```

A panic in a callback is recovered into `gen.TerminateReasonPanic`, exactly as the real runtime does, so a buggy actor fails its assertion instead of crashing the test. To check an actor's internal field directly, `Behavior` returns the live behavior for a white-box look, and node-level queries answer truthfully - `sub.Node().ProcessInfo(sub.PID())` returns its info, and an unknown PID yields `gen.ErrProcessUnknown`, just like a real node.

## Faithful Runtime Semantics

The mock node is not a loose stand-in; it enforces the rules a real process enforces, so a test catches the same misuse production would. Linking a process to itself is rejected with `gen.ErrNotAllowed`. `SetSendPriority` validates its argument, and the send priority is stateful - seeded from `ProcessOptions` and carried by later sends. The logger gates by level, so a line below the configured level is dropped, never recorded. A message addressed by registered name dispatches to the name split-handler. You do not opt into any of this; it is simply how the harness behaves, which is the point - the actor under test runs against the contract it will meet for real.

## Meta Processes

Meta processes - the I/O adapters behind TCP, UDP, web, and the like - have their own behavior contract, and `unit` drives them too. `SpawnMeta` instantiates a meta behavior under the actor, runs its `Init`, and returns a `MetaSubject`:

```go
m, err := sub.SpawnMeta(&echoMeta{}, gen.MetaOptions{})
m.DeliverMessage(sub.PID(), "hello")
m.ShouldSend().To("client").Message("got:hello").Once().Assert()
```

A meta shares its parent's journal and its egress is observed as coming from the parent PID, exactly how the runtime routes it. `DeliverMessage`, `Request`, `Inspect`, and `Terminate` drive the matching callbacks, and the state gates apply - `SendResponse` is rejected outside a running callback, just as in production.

The meta's outbound calls are stubbed on its own scope, not the parent's: `m.OnSend` and `m.OnSpawnMeta` configure only this meta, and the parent actor's stubs do not reach it. As with the actor, a stub must be set before the egress happens, so to shape what the meta does in its own `Init`, prepare it first: `sub.PrepareMeta(...)` builds the meta without running `Init`, you set its stubs, then `m.Run()` runs `Init`. `SpawnMeta` is `PrepareMeta` followed by `Run`.

## Choosing Between Unit and Stage

Use `unit` when the thing under test is one actor's decision logic - what it does with a message, how it handles a failure, what it spawns. It is fast, fully deterministic, and it models what stage leaves to the real runtime: termination reasons, scheduled timers, log lines. Most of a suite's tests belong here. When the behavior under test only emerges from the real runtime - supervision and restarts, links and monitors across nodes, cross-node messaging, remote spawn, disconnects - move up to [stage](stage.md). Both speak the same grammar, [check](check.md), so a test reads the same on either.
