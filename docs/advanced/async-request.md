---
description: Making a request without taking the caller out of service
---

# Requests Without Blocking

`Call` is the simplest way to ask another process for something, and for most code it is the right way. It is also the one place where an actor stops behaving like an actor. While the call is out, the process sits in the WaitResponse state and reads nothing from its mailbox. Messages pile up behind the answer it waits for. If the callee is slow, the caller is slow with it. If the callee never answers, the caller is out of service for the whole timeout.

For a worker that does one thing at a time, that is fine and even desirable - the blocked call is the back pressure. It stops being fine when the caller has other obligations while it waits. A manager that must keep reacting to the death of its children. An actor that drives a stream and cannot go quiet for five seconds. Anything that asks several nodes at once, where blocking makes the work sequential and the slowest peer sets the pace.

Until now the answer to that was one of three workarounds. Spawn a short-lived process whose only job is to block on `Call` and report back. Invent your own protocol on plain messages, where the server has to learn a second way of answering. Or block and live with it. The first costs a process and loses the caller's context, the second costs a protocol on both sides, and the third costs availability.

`SendRequest` is the missing option: the same request a `Call` makes, without the wait.

## Asking Without Waiting

```go
ref, err := a.SendRequest(gen.Atom("storage"), ReadRequest{Key: key})
```

The request is an ordinary request. The callee sees it in `HandleCall` and answers as it always does, with a return value or with a later `SendResponse`. Nothing on that side changes, which means any existing server can be used this way without touching it.

What changes is the caller. `SendRequest` returns immediately with a ref, the actor goes back to its mailbox, and the answer arrives later as a callback:

```go
func (a *Reader) HandleResponse(response gen.MessageResponse) error {
    if response.Error != nil {
        a.Log().Warning("read failed: %s", response.Error)
        return nil
    }
    a.cache[response.Ref] = response.Result
    return nil
}
```

Every request ends in exactly one `HandleResponse`: the result, the error the callee replied with, or `gen.ErrTimeout` when the deadline passes. Whatever you remember about a request, you know it will be settled.

Returning an error from `HandleResponse` terminates the process, like any other callback. The default implementation, which you get by not writing one, logs the answer as an error and keeps the actor running.

## The Answer Carries Its Own Context

A ref tells you which request was answered, not what it was about. You could keep a map from ref to your own context, but then both you and the framework track the same request, and you pay a lookup on every answer. Attach the context to the request instead:

```go
for _, node := range nodes {
    a.SendRequestWithLabel(gen.ProcessID{Name: "inspector", Node: node}, RequestInfo{}, node)
}
```

The label comes back with the answer, so the handler reads as a handler and not as bookkeeping:

```go
func (c *Collector) HandleResponse(response gen.MessageResponse) error {
    node := response.Label.(gen.Atom)

    if response.Error != nil {
        c.failed[node] = response.Error
    } else {
        c.result[node] = response.Result.(NodeInfo)
    }

    if len(c.result)+len(c.failed) == c.asked {
        return c.Send(c.Parent(), Collected{Result: c.result, Failed: c.failed})
    }
    return nil
}
```

A label is any value you like: a node name, a request struct, a closure over your own state. The framework keeps it next to the request and hands it back, nothing more.

## Deadlines, Cancelling, Undeliverable

Every request carries a deadline, five seconds by default, and `SendRequestWithTimeout` sets another one. The deadline lives in the ref, so the callee can read it with `ref.IsAlive()` and skip work nobody waits for any more. When it passes, the answer arrives with `gen.ErrTimeout` and the request is done.

If you lose interest earlier, drop it:

```go
a.CancelRequest(ref)
```

A cancelled request produces no answer at all, and a reply that arrives afterwards is discarded as a late one. This is the only way a request ends without a `HandleResponse`, and it is explicit.

A request to a process that does not exist fails at once: `SendRequest` returns the error itself and nothing is registered. Across the network that error cannot be seen immediately, because the peer learns about the missing process, not you. `SendRequestImportant` asks the peer to report it:

```go
ref, err := a.SendRequestImportant(gen.ProcessID{Name: "inspector", Node: node}, RequestInfo{})
```

With the flag set, an undeliverable request comes back as an answer with `gen.ErrProcessUnknown` or `gen.ErrProcessMailboxFull` in about one round trip, instead of waiting out the deadline. In a fan-out over twenty nodes that is the difference between knowing which three are gone right away and learning it from twenty timeouts at once. For the details of what the flag guarantees, see [Important Delivery](important-delivery.md).

`SendRequestWithOptions` takes all of it together when a single request needs more than one knob:

```go
ref, err := a.SendRequestWithOptions(target, request, gen.RequestOptions{
    Label:     order.ID,
    Timeout:   30,
    Important: true,
})
```

## Where the Answer Lands in the Queue

The answer is an ordinary mailbox message. It goes into the queue that matches the priority the request was sent with, and it takes its turn there: an answer never jumps ahead of messages that arrived before it. A request sent with `gen.MessagePriorityHigh` gets its answer on the System queue, which is how you keep an administrative conversation ahead of the regular traffic.

Nothing arrives in `HandleResponse` that you did not ask for. A process that never calls `SendRequest` never sees the callback, which is why adding it to your actors is free until you use it.

## Keeping Call Where It Belongs

`SendRequest` is not a replacement for `Call`. Sequential logic reads better as a sequence:

```go
user, err := a.Call(users, GetUser{ID: id})
if err != nil {
    return err
}
balance, err := a.Call(accounts, GetBalance{User: user.(User).Name})
```

Written with requests and a callback, the same three lines turn into a state machine. Use `Call` when the actor has nothing else to do until the answer comes, and `SendRequest` when it does.

Spawning a process per request also remains a reasonable design, especially when each request needs its own failure isolation, its own deadline and its own retries. What you no longer have to do is spawn one just to avoid blocking.

For the server side of the same conversation, see [Sync Request Handling](handle-sync.md). For what the delivery flag guarantees, see [Important Delivery](important-delivery.md).
