---
description: How to test actor systems in Ergo, and which layer to use
---

# Overview

An actor is not a function. You cannot call it and inspect a return value. It runs on its own goroutine, communicates only through messages, keeps private state that evolves from one message to the next, spawns children, and can be terminated or restarted by a supervisor. A test that reaches into an actor's fields tests the wrong thing and breaks the isolation the model depends on.

So the testing tools observe an actor the way the rest of the system does: through what it *does*. Every outward action - a message sent, a process spawned, a log line written, an exit signal, a timer scheduled - is captured as a record. You drive the actor with inputs - deliver a message, fire a timer, deliver an exit - and assert on the records it produced. You test behavior, not state. That single idea runs through all four packages this section covers.

## The Observation Model

The shape of every test is the same: drive an input, then assert on an output.

```go
sub.SendMessage(client, "ping")
sub.ShouldSend().To(client).Message("pong").Once().Assert()
```

The actor received a message and sent one back. The harness recorded the send, and the assertion checks that it happened exactly once, to the right target, with the right payload. The same fluent grammar - a `Should...` builder, filters, a count, a terminal - describes every kind of action, and it reads the same whether the actor runs in a mock or on a live node. There is no `result := actor.Process(msg)` to inspect, because actors do not work that way; instead you verify the messages an actor sends, the children it spawns, the events it emits, and how it terminates.

## The Layers

Four packages cover the span from a single function up to a cluster of real nodes. They share one assertion grammar and line up along one axis: how much of the real system is present.

**check** is the language, and nothing more. It defines the record types and the fluent `Should...` grammar every test is written in. You rarely import it directly - `unit` and `stage` expose its assertions on their own handles - but reading it once teaches the vocabulary the other layers reuse: cardinalities, filters, scoping, and value capture.

```go
sub.ShouldSend().To("db").Message(SaveUser{ID: 7}).Once().Assert() // exactly once
sub.ShouldSpawn().Times(3).Assert()                                // a count
sub.ShouldSend().To("audit").None().Assert()                       // never happened
child, _ := sub.ShouldSpawn().Once().Capture()                     // grab the result
```

**mock** adds a controllable dependency, for code that is not an actor. It provides a standalone fake of each `gen.*` interface - `Node`, `Process`, `MetaProcess`, `Log`, `Cron`, `Network`, `RemoteNode`, `Registrar`, `Resolver` - to inject into ordinary code that consumes one: a helper, a resolver, a constructor. Override the methods you care about; the rest return safe defaults.

```go
db := mock.NewProcess()
db.OnCall(func(to, request any) (any, error) { return Row{ID: 7}, nil })
saveUser(db) // the code under test holds db as a gen.Process
```

A second constructor with a `T` suffix also records every call, so you can assert on what the code did with the dependency:

```go
node := mock.NewNodeT(t)
bootstrap(node) // the code under test calls node.Spawn, node.Log, ...
node.ShouldSpawn().Times(3).Assert()
```

**unit** brings one actor to life against a mock node. It spawns a single behavior, lets you drive its callbacks - deliver messages, fire timers, deliver exits and downs - and asserts on what it does. It runs synchronously: no real goroutines, no clock, fully deterministic and fast. This is where most actor logic is tested.

```go
sub, _ := unit.Spawn(t, factoryWorker, gen.ProcessOptions{})
sub.SendMessage(client, StartJob{ID: "42"})
sub.ShouldSend().To("scheduler").Message(JobQueued{ID: "42"}).Once().Assert()
```

**stage** brings the whole runtime to life. It starts real nodes, runs real actors and applications, lets them talk over the real network, and observes what the live runtime does. Use it for what `unit` deliberately leaves out: the real scheduler, supervision and restarts, links and monitors across nodes, remote spawn, disconnects. Because everything runs for real and concurrently, assertions wait with `Within` instead of reading a snapshot.

```go
s := stage.New(t)
n := s.Node("n")
worker := n.Spawn(factoryWorker, gen.ProcessOptions{})

n.Send(worker, Job{ID: "42"})
n.ShouldDeliver().To(worker).Message(Job{ID: "42"}).Within(time.Second).Once().Assert()
```

## Choosing a Layer

- Testing one actor's message handling, spawning, logging, or lifecycle: use **unit**.
- Testing a helper or component that takes a `gen.Node` or `gen.Process` and you need to control what it returns: use **mock**.
- Testing behavior that needs the real scheduler, the real network, or more than one node: use **stage**.
- Understanding what `ShouldSend`, `Within`, `Once`, or `Capture` mean in any of the above: read **check**.

`unit` and `stage` are not two ways to write the same test; they answer different questions. A typical project tests the bulk of its actor logic with `unit`, where tests run in microseconds and never flake, and reserves `stage` for the cross-node and supervision scenarios that only the real runtime exhibits.

Read on in order: [Check](check.md) for the grammar every test is written in, then [Mock](mock.md), [Unit](unit.md), and [Stage](stage.md), each building on the one before.
