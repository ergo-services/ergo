---
description: The live multi-node harness for testing the real runtime end to end
---

# Stage

Some behavior only exists when everything is real. A supervisor restarts a crashed child on its own goroutine. A monitor fires a `Down` after the process it watched - on another node - dies. A message leaves one node, is serialized, crosses a TCP connection, and lands in a mailbox a few milliseconds later. None of this is decision logic you can step through callback by callback; it is the runtime doing its job, concurrently, in real time. That is the gap `stage` fills.

The [unit](unit.md) harness deliberately removes all of that: it gives one actor a mock node and runs its callbacks by hand, so a test is fast and perfectly deterministic. `stage` makes the opposite trade. It starts real nodes, runs real actors and applications on them, lets them talk over the real network, and watches what the live runtime actually does. You give up the frozen snapshot and the single-actor focus; in return you can test what only the real runtime exhibits - supervision and restarts, links and monitors across nodes, cross-node messaging, remote spawn, service discovery, disconnects.

Everything else about `stage` follows from one fact: because the system runs for real and concurrently, you do not inspect a result, you wait for it and observe it. The rest of this page builds that idea up from the smallest possible test.

## The Shape of a Test

Every stage test has the same skeleton: create a stage, start a node on it, put a process on the node, make something happen, and assert on it.

```go
s := stage.New(t)
n := s.Node("n")
ponger := n.Spawn(factoryPonger, gen.ProcessOptions{}) // a process that accepts messages

n.Send(ponger, ping{Seq: 1})
n.ShouldDeliver().To(ponger).Message(ping{Seq: 1}).Once().Within(time.Second).Assert()
```

`stage.New` returns a `Stage` - the owner of every node the test starts - and registers cleanup with the test, so the nodes are stopped automatically when it ends; you never tear them down by hand. `s.Node` starts one live node and returns a handle to it.

That handle, a `*stage.Node`, is worth a close look, because you work through it for the whole test. It is a thin wrapper around a real `gen.Node`, not the node itself. It surfaces the operations a test reaches for most - `Spawn`, `SpawnRegister`, `Send`, `Call`, `SendExit`, `Kill` - and it carries the assertion grammar from [check](check.md), which is why you write `n.ShouldDeliver(...)` straight on it. For anything the wrapper does not cover - any other method of the underlying node - `n.Native()` returns the real `gen.Node` with its full API. You will need it the moment a second node appears.

The last line of that test is the one new idea. In `unit`, an actor has already run by the time you assert, so an assertion reads a finished snapshot. Here the send and its delivery happen on the runtime's own goroutines, and the record of the delivery may not exist the instant you check for it. So `Within` makes the assertion wait: it polls until the assertion holds or the deadline passes. Almost every stage assertion carries a `Within`, and the next sections lean on it constantly.

## Where Things Are Recorded

On one node it hardly matters where a happening is recorded - it all lands in that node's journal. The moment there are two nodes it matters a great deal, and answering "which node do I assert on?" is the key to reading and writing stage tests.

The harness observes a node by wrapping it, and what it sees falls into two kinds:

- what the node's own processes **do** - send a message, make a call, spawn a child, set up a link or a monitor. This is *egress*, and it is recorded on the node the acting process runs on.
- what **arrives** at the node's processes - a message delivered into a mailbox, a `Down`, an `Exit`, a subscribed event. This is *ingress*, and it is recorded on the node that hosts the receiver.

Each node keeps its own journal of both. So one interaction that crosses the network leaves two traces, on two different nodes:

```go
s := stage.New(t)
a, b := s.Node("a"), s.Node("b")

// a value that crosses the wire must be registered for transport, on both nodes;
// that registration lives on the node's Network, reached through Native()
a.Native().Network().RegisterType(ping{})
b.Native().Network().RegisterType(ping{})

ponger := b.Spawn(factoryPonger, gen.ProcessOptions{})
pinger := a.Spawn(factoryPinger, gen.ProcessOptions{}) // on a sendPing trigger, sends a ping to the target

a.Send(pinger, sendPing{To: ponger, Seq: 1})

a.ShouldSend().From(pinger).Message(ping{Seq: 1}).Once().Within(time.Second).Must()  // egress, on a
b.ShouldDeliver().To(ponger).Message(ping{Seq: 1}).Once().Within(time.Second).Must()  // ingress, on b
```

The send is asserted on `a`, where the pinger runs; the delivery on `b`, where the ponger runs. That is the whole rule: assert egress on the actor's node, ingress on the recipient's node.

Two smaller things in that test are worth naming. The nodes were never explicitly connected - the first time `a` addressed a process on `b`, the runtime looked `b` up through the registrar and dialed it for you. And every value that crosses the wire must be registered with `RegisterType`, because the network serializes it; this is where the `Native()` escape hatch first earns its keep, since type registration is a node-level concern the wrapper does not surface.

This egress/ingress split is the half that [unit](unit.md) cannot show. A mock node has no real delivery to observe, so unit records only egress, and you assert an actor's reaction to an input you fed it. On a live node the delivery is real, so stage records the ingress directly - which is why the ingress assertions `ShouldDeliver`, `ShouldReceiveDown`, and `ShouldReceiveEvent` live here and not there.

## Waiting for Live Results

You have seen `Within` on every cross-node assertion, and the reason is the trade we started with: the runtime is concurrent, so a happening you expect may not be recorded yet when you assert. `Within` turns the assertion into a bounded wait. A positive assertion - `Once`, `Times`, `AtLeast` - succeeds the instant its condition is met, so the wait usually costs only the real latency of the action.

A negative is the case that needs care. To claim something did *not* happen, you must give it time to fail to happen: `None().Within(...)` watches for the whole window and passes only if nothing matched it.

```go
b.ShouldDeliver().To(ponger).Message(ping{Seq: 2}).None().Within(150 * time.Millisecond).Assert()
```

Because a live test runs in phases and the same action can recur, scope an assertion to one phase with `Mark` and `Since`: `Mark` records the current position in the journal, `Since` restricts the next assertion to what came after it. This is how you prove "no *second* event after the legitimate one" without the first occurrence spoiling the count:

```go
m := n.Mark()
n.ShouldReceiveDown().To(w).About(target).Since(m).None().Within(150 * time.Millisecond).Assert()
```

Two finishing choices. End with `Must` instead of `Assert` when the test cannot continue meaningfully without this step - a cross-node test that proceeds past a connection that never came up only produces noise; `Assert` reports the failure and lets the test go on. And note the one place you do *not* wait: a synchronous `Call` blocks for its reply and hands it back directly, so you check its return value with `check.Equal`, no `Within` involved.

```go
resp, err := a.Call(ponger, pingRequest{Seq: 7})
check.NoError(t, err)
check.Equal(t, pong{Seq: 7}, resp)
```

The grammar itself - `ShouldX`, the cardinalities, `Within`, `Mark`, `Since`, `Must` - is the shared vocabulary documented in [check](check.md); stage only supplies the live nodes it runs against.

## Observing a Process Stop

A natural thing to test is that a process stopped - and here stage works differently from unit in a way that reveals its whole philosophy. Stage does not hand you a "terminated" record. It records what the runtime really does at its seams, and a process ending is not a message on a wire; it is something other processes learn about through the mechanisms the framework already provides - a monitor's `Down` or a link's `Exit`. So you observe a stop the way the rest of the system does: watch the process, end it, and assert the notification.

```go
target := n.Spawn(factoryPonger, gen.ProcessOptions{})
w := n.Spawn(factoryWatcher, gen.ProcessOptions{}, target) // watcher monitors target in its Init

n.ShouldMonitor().From(w).Target(target).Once().Within(time.Second).Must() // egress: the monitor was set up

n.Kill(target)
n.ShouldReceiveDown().To(w).About(target).Reason(gen.TerminateReasonKill).
    Once().Within(time.Second).Must()                                      // ingress: the watcher's Down
```

The same principle draws the line between stage and unit for two more things. A `SendAfter` timer is not recorded as a scheduled action - it fires for real, and you observe the resulting send or delivery once it does. And the node logger is turned off in a stage, so there are no log records to assert on. A termination reason on its own, a scheduled-send record, a log line: those are facts a harness has to synthesize, and synthesizing is unit's job. Stage shows you only what genuinely happened.

## Connecting on Purpose

You saw that nodes connect themselves on first contact, so `s.Connect` is never required just to make traffic flow. You reach for it deliberately, in two cases.

The first is to test connectivity itself. `s.Connect(a, b)` dials immediately and waits, deterministically, until both sides have registered the link before returning - so the test asserts that two nodes *can* reach each other, by a direct call, instead of inferring it from an application message that happened to get through.

The second is remote operations. `Connect` returns the peer as a `gen.RemoteNode`, and stage gives back a wrapped one whose `Spawn`, `SpawnRegister`, and application-start calls are recorded on the initiating node's journal - which is exactly what the next section relies on.

```go
remote := s.Connect(a, b) // dials now, waits for both sides, returns a's view of b
```

## Remote Spawn and Application Start

A node will not let a stranger start processes on it: remote operations are denied by default. The target opens the door in two steps - it allows the specific factory with `EnableSpawn` and enables remote spawn in its network flags - and only then does a spawn issued across the connection succeed. It is recorded as remote egress on the node that initiated it:

```go
b := s.Node("b", stage.NodeOptions{NetworkFlags: gen.NetworkFlags{Enable: true, EnableRemoteSpawn: true}})
b.EnableSpawn("worker", factoryWorker)

remote := s.Connect(a, b)
remote.Spawn("worker", gen.ProcessOptions{})

a.ShouldRemoteSpawn().To(b.Name()).Name("worker").Once().Within(time.Second).Assert()
```

`EnableApplicationStart` is the application-level counterpart of `EnableSpawn`, and the same `remote` handle's application-start calls are recorded the same way.

## Service Discovery

Discovery is what let the two nodes find each other earlier, and you can configure it. By default a stage runs a private in-memory registrar: it needs no ports, is isolated to that one stage, and so any number of stages run in parallel without colliding. It serves node routes and enforces name uniqueness, matching the embedded registrar a bare node ships with.

Some applications do more than route between nodes - they discover *applications* and react to a registrar event stream. `RegistrarFull` upgrades the in-memory registrar to serve `ResolveApplication` and emit the canonical registrar events, the same contract etcd and Saturn implement:

```go
s := stage.New(t, stage.StageOptions{RegistrarFull: true})
n := s.Node("n")
sub := n.Spawn(factoryRegSub, gen.ProcessOptions{}) // subscribes to the registrar event in its Init
mk := n.Mark()

reg, _ := n.Native().Network().Registrar()
reg.RegisterApplicationRoute(gen.ApplicationRoute{Name: "myapp", Node: n.Name(), State: gen.ApplicationStateRunning})

n.ShouldReceiveEvent().To(sub).Where(func(e check.Event) bool {
    m, ok := e.Message.(gen.MessageRegistrarApplicationStarted)
    return ok && m.Route.Name == "myapp"
}).Since(mk).Once().Within(time.Second).Must()
```

To test against a real backend, set `StageOptions.Registrar` to a factory - etcd's, for instance. It is called once per node, so every node gets its own registrar instance over the one backend.

## Configuring Nodes and Clusters

`stage.NodeOptions` carries what a real node needs: `Applications` to load, `Env`, a `Cookie`, the network knobs (`MaxMessageSize`, `FragmentSize`, `NetworkFlags`), and `Security`. Loading an application is how you test framework-spawned, supervised, name-registered processes end to end - the very processes a bare `Spawn` cannot give you:

```go
a := s.Node("a", stage.NodeOptions{Applications: []gen.ApplicationBehavior{createApp1()}})
service1, err := a.ProcessPID("service1") // the application registered this process by name
```

By default a node starts bare, with no system processes, so a test can assert exact process and application counts; add the system services with `NodeOptions{EnableSystemApp: true}` when one is needed.

For more than two nodes, `s.ConnectMesh(nodes...)` connects every pair at once - exercising the simultaneous-connect collision handling a real cluster meets under a connect storm - and waits until every node sees every other before returning. `n.Kill` force-terminates a process, and, as the first test noted, the stage stops every node it started on cleanup, so a test never leaks a running node.

## Choosing Between Unit and Stage

You now have both halves of the testing story. [Unit](unit.md) freezes one actor against a mock node and reads a snapshot: it is fast, fully deterministic, and it models the things stage leaves to the real runtime - termination reasons, scheduled sends, log lines. Stage runs the real system and observes it live: it is the only way to test supervision and restarts, links and monitors across nodes, cross-node messaging, remote spawn, service discovery, and disconnects, and it pays for that with concurrency you wait on rather than control.

A healthy suite uses both, and the division is clean. Test an actor's decision logic - what it does with a message, how it reacts to a failure, what it spawns - in `unit`, where most of your tests should live. Reserve `stage` for behavior that only emerges when the runtime, the network, and more than one node are all real. Both speak the same assertion grammar, [check](check.md), so a test reads the same whichever layer it runs on.
