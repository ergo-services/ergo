---
description: The shared assertion vocabulary that unit and stage are written in
---

# Check

You cannot test an actor the way you test a function. There is no `result := actor.Handle(msg)` to inspect: an actor runs on its own goroutine, keeps private state, and speaks only in messages. What you *can* see is what it does - the messages it sends, the children it spawns, the way it terminates. So the testing tools watch the thing under test and record every such action, and a test then asks questions of that recording: did it send this, how many times, to whom, and after what.

`check` is the language those questions are written in. It is not a harness you run on its own; it is the shared vocabulary - the record types and the assertion grammar - that both [unit](unit.md) and [stage](stage.md) hand you. You will rarely import it directly. You call its assertions on a unit `Subject` or a stage `Node`, and because both layers expose the very same grammar, learning it once here lets you assert anything in either of them. The examples below use a handle - `sub` for a unit subject, `node` for a stage node - which you can read simply as "the thing under test."

## The Journal of Records

Begin with what is being queried, because every assertion is a query against it. As the thing under test runs, the harness appends each action it observes to an ordered journal, and each entry is a typed *record*. You never build a record - the harness does - and you never read the journal line by line. But picturing it is what makes the grammar make sense.

A journal captured while one actor handled a single message might read like this:

```
Spawn(parent=svc child=worker register=worker err=<nil>)
Send(from=svc to=worker msg=Job{ID:"42"} err=<nil>)
Log(from=svc level=info msg="dispatched job 42")
Call(from=svc to=registry req=Lookup{Name:"db"} err=<nil>)
```

The entries are different types because they describe different kinds of happening, and which kinds turn up depends on what the actor did and what produced them. They fall into three groups:

- **Egress** - what the actor *does*: `Send`, `Call`, `Spawn`, `Link`, `Monitor`, `Log`, and its other outgoing actions.
- **Lifecycle** - `Terminated`, the actor's own end.
- **Ingress** - what *reaches* the actor: `Delivered`, `Down`, `Exit`, `Event`. There is something to record on the way in only where delivery is real, so these appear in [stage](stage.md), not in unit.

Each record carries fields that describe the action - a `Send` has `From`, `To`, `Message`, `Options`, `Error`; a `Spawn` has `Parent`, `Child`, `Register`, `Factory`, `Error`. You match on those fields rather than scanning text. The complete list of record types and their fields is in the package godoc; in practice you meet each one through the assertion that selects it, which is the next thing.

## The Assertion Chain

An assertion is a single chain with four parts: choose a record type, narrow it with filters, say how many should match, and run it.

```go
sub.ShouldSend().To("db").Message(SaveUser{ID: 7}).Once().Assert()
```

Read left to right: of the recorded `Send` actions (`ShouldSend`), the ones addressed to `"db"` and carrying that message (the two filters), there should be exactly one (`Once`); evaluate it now and report a failure if not (`Assert`). There is one `Should...` builder per record type - `ShouldSend`, `ShouldSpawn`, `ShouldCall`, `ShouldLog`, and so on - and everything that follows on this page is a variation of one of those four parts. Learn the chain and you can read any assertion in the framework.

## How Many: Cardinalities

The third part answers "how many," and there are four ways to answer:

```go
sub.ShouldSpawn().Once().Assert()                  // exactly one
sub.ShouldSpawn().Times(3).Assert()                // exactly three
sub.ShouldSend().To("metrics").AtLeast(1).Assert() // one or more
sub.ShouldSend().To("audit").None().Assert()       // never
```

`None` is how you assert a negative - that an action did *not* happen - so there is no separate "should not" builder; you expect a count of zero.

## Narrowing: Filters

The filters are named after the record's fields, and they reach well past `To` and `Message`. You narrow on whatever distinguishes the action you mean:

```go
sub.ShouldSend().To("worker").Priority(gen.MessagePriorityHigh).Once().Assert()
sub.ShouldSpawn().Factory(factoryWorker).Times(3).Assert()
sub.ShouldLog().Level(gen.LogLevelError).Containing("timeout").Once().Assert()
```

Where an action can fail, `Error` matches an exact error and `ErrorIs` matches a wrapped one:

```go
sub.ShouldCall().To("db").ErrorIs(gen.ErrTimeout).Once().Assert()
```

And when no named filter fits - you need one field of a struct, a range, a computed condition - `Where` takes a typed predicate over the record itself:

```go
sub.ShouldSend().Where(func(r check.Send) bool {
    n, ok := r.Message.(Notification)
    return ok && n.Urgent
}).AtLeast(1).Assert()
```

Filters compose: chain as many as you need, and a record must satisfy all of them to match.

## The Same Chain for Every Record

Because the chain is uniform, the assertions you have not met yet read exactly like the ones you have. Termination is its own record, asserted with `ShouldTerminate`, which adds a small vocabulary for the reason:

```go
sub.ShouldTerminate().Abnormally().Once().Assert() // crashed, panicked, killed, or errored
sub.ShouldTerminate().None().Assert()              // still running
```

On a live node the ingress records become assertions too - `ShouldDeliver` for a message that arrived, `ShouldReceiveDown` and `ShouldReceiveExit` for the notifications a monitor or link delivers, `ShouldReceiveEvent` for a subscribed event. They read with the same chain, with filters for their own fields (`About` and `Reason` on a down, for instance), and they belong to [stage](stage.md), where delivery is real:

```go
node.ShouldReceiveDown().To(watcher).About(worker).Reason(gen.TerminateReasonKill).Once().Within(time.Second).Assert()
```

That `Within` is new; it is the next idea.

## Reading a Snapshot, or Waiting: Within

Whether an assertion waits depends on the layer, and this is the one real difference between using `check` in unit and in stage. In [unit](unit.md) the actor has already run to completion by the time you assert, so the journal is final - the assertion reads a snapshot and returns at once. In [stage](stage.md) real actors run concurrently, so the action you expect may not be recorded yet. `Within` turns the terminal into a bounded wait that polls until the assertion holds or the deadline passes:

```go
node.ShouldDeliver().To(worker).Within(time.Second).Once().Assert()
```

A positive assertion is satisfied the moment its count is met. A negative is the case to think about: to claim something did not happen, you have to watch for a while, so `None().Within(...)` passes only if nothing matched for the whole window. One sharp edge with an exact count: `Within` is met at the first poll where the count equals n, so if a still-growing count overshoots n between two polls it never reads as n again and the assertion fails at the deadline - for a count that only grows, use `AtLeast` rather than `Times`.

## Scoping to a Phase: Mark and Since

A test often runs in stages, and an earlier stage may have produced the same kind of record you now want to count. `Mark` records the current position in the journal; `Since` restricts the next assertion to what came after it:

```go
sub.SendMessage(client, "first")
mark := sub.Mark()

sub.SendMessage(client, "second")
sub.ShouldSend().To(client).Since(mark).Once().Assert() // only the reply to "second"
```

This is also how you express "no *second* occurrence after a legitimate first one": mark past the first, then assert `None().Since(mark)`.

## Pulling Values Out: Capture and Collect

Actors produce values you cannot know in advance - a spawned child's PID, an allocated alias. `Capture` returns the first matching record so you can read those values and use them later in the test:

```go
spawn, ok := sub.ShouldSpawn().Once().Capture()
childPID := spawn.Child
```

`Collect` returns every matching record in the order observed, which is what you want when the order itself is under test - a round-robin distribution, say:

```go
sends := sub.ShouldSend().To("worker").Collect() // []check.Send, in order
```

(`Records` returns the whole journal as a slice, for poking at it while you debug a test; the assertions themselves should use the grammar.)

## Assert or Must

Both `Assert` and `Must` evaluate the same way; they differ in what a failure does. `Assert` reports it and lets the test continue. `Must` stops the test immediately - reach for it when later steps cannot run meaningfully without this one, so the log shows the real cause instead of a cascade of follow-on failures.

## Asserting the Rest of a Test

Not everything in a test is a record. You still inspect returned errors and compare captured values, and `check` carries plain helpers for that, so a test needs no second assertion library:

```go
check.NoError(t, err)
check.ErrorIs(t, err, gen.ErrProcessUnknown)
check.Equal(t, JobQueued{ID: "42"}, got)
check.True(t, behavior.started)
```

The set also includes `False`, `NotEqual`, `Nil`, `NotNil`, `Error`, `Contains`, and `ErrorContains`.

## Matching Stub Calls

One last piece of vocabulary shows up not in assertions but in the stubbing APIs of [mock](mock.md) and [unit](unit.md), where you tell a dependency how to answer a particular call. A small set of matchers narrows a stub to the calls it should handle: `Anything`, `Equals`, `MatchedBy`, and `IsType`.

```go
sub.OnCall("db").Where(check.IsType[Query]()).Respond(rows)
```

`IsType[V]` matches a value assignable to `V`: a concrete type matches its exact dynamic type, an interface matches any value that implements it. You will see these in context on the next pages.

That is the whole language. What produces the journals it queries - and the inputs you drive to fill them - are [mock](mock.md), [unit](unit.md), and [stage](stage.md).
