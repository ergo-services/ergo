---
description: Standalone fakes of the gen interfaces, for testing code that consumes them
---

# Mock

Not all code that touches the framework is an actor. A helper that takes a `gen.Process` to make a call, a custom resolver that implements `gen.Resolver`, a constructor that reads configuration from a `gen.Node` - these are ordinary functions, and you test them the ordinary way: give them a dependency you control, run them, and check what happened. `mock` provides that dependency. For each of the framework's interfaces it offers a standalone fake you can hand to the code under test in place of the real thing.

These fakes are deliberately dumb, and it helps to say plainly what they are not: a mock is not the [unit](unit.md) harness in disguise. It runs no actor, starts no goroutine, and never fails your test on its own. It implements an interface, lets you override the methods you care about, and returns safe defaults for the rest. Each example names the type doing the work, so it is always clear which mock you are looking at.

## A Dumb Mock

`mock.NewProcess` returns a value that satisfies `gen.Process` in full. You override the methods the code under test will actually call, then pass it wherever that interface is expected:

```go
func saveUser(db gen.Process, u User) error {
    _, err := db.Call("users", Insert{Name: u.Name})
    return err
}

func TestSaveUser(t *testing.T) {
    db := mock.NewProcess()
    db.OnCall(func(to, request any) (any, error) { return Row{ID: 7}, nil })

    err := saveUser(db, User{Name: "ann"})
    check.NoError(t, err)
}
```

`saveUser` holds `db` as a `gen.Process` and cannot tell it is a fake. Every method has a matching `On<Method>` setter - `OnCall`, `OnSend`, `OnSpawn`, `OnLink`, across the whole interface - so you configure exactly the surface your test exercises and leave the rest alone.

## Safe Defaults, Never a Failure

A method you do not override still works; it just returns a safe default. A query returns a zero value, an action reports success, and anything that must produce an identifier returns a synthesized one - a stable `gen.PID`, `gen.Alias`, or `gen.Ref`. Nothing panics, and nothing fails the test.

```go
db := mock.NewProcess()
db.OnCall(func(to, request any) (any, error) { return Row{ID: 7}, nil })

check.NoError(t, db.Send("audit", "saved")) // Send was never overridden; it just succeeds
```

This is the deliberate difference from [unit](unit.md), and it is worth understanding because it tells you which tool you are holding. The unit harness fails loudly when an actor takes an action you did not set up, because there the unexpected action is the bug under test. A mock makes no such judgment: it is a dependency you are injecting, not the subject of the test, so an unconfigured call is simply a no-op with a sensible result.

## When You Want to Assert What the Code Did

Sometimes the return value is not what you care about - you want to verify what the code *did* with the dependency: that it spawned three workers, that it logged the failure. For that, every mock type has a second constructor whose name ends in `T` and which takes the test's `testing.T`. It behaves exactly like the dumb one, and on top of that records every action and exposes the `check` assertion grammar on the mock.

```go
func TestBootstrap(t *testing.T) {
    node := mock.NewNodeT(t)
    bootstrap(node) // the code under test calls node.Spawn three times

    node.ShouldSpawn().Times(3).Assert()
}
```

So each type comes as a pair: `mock.NewNode` is the dumb form, `mock.NewNodeT(t)` the recording one; likewise `NewProcess` / `NewProcessT`, `NewLog` / `NewLogT`, and the rest. Because the recording mock carries the grammar from [check](check.md), the whole vocabulary - `ShouldSend`, filters, cardinalities, `Capture` - is available on it.

## Overrides and Recording Together

The two features compose, and in the order you would want. On a recording mock an override decides the return value while the action is still recorded - the override shapes what the call returns, the recorder simply notes that it happened:

```go
p := mock.NewProcessT(t)
p.OnSend(func(to, message any) error { return gen.ErrProcessUnknown })

err := p.Send("dead", "hi")
check.ErrorIs(t, err, gen.ErrProcessUnknown) // the override decided this

p.ShouldSend().To("dead").Once().Assert()    // and it was still recorded
```

The override runs first, because it is the behavior; the record is taken afterward. This mirrors stubbing in [unit](unit.md): setting a return value never hides the action from assertions.

## Composed Mocks

Some interfaces hand back others. A `gen.Node` exposes a `gen.Log`, a `gen.Network`, and a `gen.Cron`; a `gen.Network` exposes a `gen.Registrar`; a `gen.Registrar` exposes a `gen.Resolver`. A recording mock creates these sub-mocks sharing its own recorder, so everything the node and its logger do collates into one ordered journal you assert over as a whole:

```go
n := mock.NewNodeT(t)
n.Send("peer", "ping")
n.Log().Info("started %d workers", 3)

n.ShouldSend().To("peer").Message("ping").Once().Assert()
n.ShouldLog().Containing("started 3 workers").Once().Assert()
```

The sub-mocks are exported types, so when you need to shape a value deeper in the tree you reach it by type assertion and override its method - here, making the node's resolver fail:

```go
net := mock.NewNetworkT(t)
reg, _ := net.Registrar()
reg.(*mock.Registrar).Resolver().(*mock.Resolver).
    OnResolve(func(node gen.Atom) ([]gen.Route, error) { return nil, gen.ErrNoRoute })
```

## The Mock Types

There is one mock per interface, each with the dumb and recording constructor pair:

| Constructor | Interface |
|-------------|-----------|
| `NewNode` / `NewNodeT` | `gen.Node` |
| `NewProcess` / `NewProcessT` | `gen.Process` |
| `NewMeta` / `NewMetaT` | `gen.MetaProcess` |
| `NewLog` / `NewLogT` | `gen.Log` |
| `NewCron` / `NewCronT` | `gen.Cron` |
| `NewNetwork` / `NewNetworkT` | `gen.Network` |
| `NewRemoteNode` / `NewRemoteNodeT` | `gen.RemoteNode` |
| `NewRegistrar` / `NewRegistrarT` | `gen.Registrar` |
| `NewResolver` / `NewResolverT` | `gen.Resolver` |

## When to Reach for Mock

Use a mock when the thing under test is *not* an actor but consumes one of the framework interfaces: a function that takes a `gen.Process` or `gen.Node`, a custom `gen.Resolver` or `gen.Registrar`, a constructor that reads from a node. When the thing under test *is* an actor - a behavior whose `Init`, `HandleMessage`, and `HandleCall` you want to drive - use [unit](unit.md), which builds its own controllable node around the actor and adds typed input drivers on top. The two share the `check` grammar, so what you learn asserting on a recording mock transfers directly.
