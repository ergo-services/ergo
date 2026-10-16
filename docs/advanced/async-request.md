---
description: Making a request without taking the caller out of service
---

# Requests Without Blocking

While an actor waits in `Call`, it does not process its mailbox. This is often appropriate for a worker with one job, but a coordinator may also need to react to child exits, incoming work or answers from other nodes.

`SendRequest` sends the same kind of request without waiting in the caller. The receiver handles it in `HandleCall`; the caller handles the eventual answer in `HandleResponse`. This chapter follows a request from dispatch to completion, including cancellation and a full mailbox.

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

Every request ends in one `HandleResponse`: the result, the error the callee replied with, or `gen.ErrTimeout` when the deadline passes. Whatever you remember about a request, you know it will be settled - unless you cancel it, or your own mailbox has no room for the answer, both described below.

Returning an error from `HandleResponse` terminates the process, like any other callback. The default implementation, which you get by not writing one, logs the answer as an error and keeps the actor running.

## The Answer Carries Its Own Context

A ref identifies the request. A label associates application context with it, avoiding a separate ref-to-context map when the handler needs only the information supplied at dispatch:

```go
for _, node := range nodes {
    a.SendRequestWithLabel(gen.ProcessID{Name: "inspector", Node: node}, RequestInfo{}, node)
}
```

Read the label in `HandleResponse`:

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

A cancelled request produces no answer at all, and a reply that arrives afterwards is discarded as a late one. This is one of two ways a request ends without a `HandleResponse`, and the only one you choose.

The other is a mailbox with no room. An answer is a message, and it needs a place in your mailbox like any other. If the mailbox is full when the answer arrives - a process with a `MailboxSize` limit that fell behind - the answer is dropped and the error is logged on your side. A callee on the same node learns it from `SendResponse`, which returns `gen.ErrProcessMailboxFull`; a remote one learns it only if it answered with `SendResponseImportant`. The request stays pending and ends with `gen.ErrTimeout` at its deadline. If the mailbox is still full by then, the timeout is dropped and logged the same way, and the request ends with no answer at all. The framework does not hold answers for a process that cannot take them.

A request to a process that does not exist fails at once: `SendRequest` returns the error itself and nothing is registered. Across the network that error cannot be seen immediately, because the peer learns about the missing process, not you. `SendRequestImportant` asks the peer to report it:

```go
ref, err := a.SendRequestImportant(gen.ProcessID{Name: "inspector", Node: node}, RequestInfo{})
```

With the flag set, the peer returns request-routing failure as an answer containing `gen.ErrProcessUnknown` or `gen.ErrProcessMailboxFull`. On a working connection this takes a round trip rather than waiting for the request deadline. If the error answer cannot return, the deadline still applies. See [Important Delivery](important-delivery.md) for the distinction between routing reports and processing results.

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

`HandleResponse` receives answers only to requests initiated by that process. Actors using only `Call` do not receive answers through this callback.

## From a Meta-Process

A meta-process has no `Call`. Of its two goroutines, the External Reader is busy with I/O and the Actor Handler exists only while there are messages to handle, so neither can sit and wait for an answer. A request without waiting needs nobody to wait, and a meta-process has the whole family: `SendRequest`, `SendRequestWithLabel`, `SendRequestWithTimeout`, `SendRequestImportant`, `SendRequestWithOptions` and `CancelRequest`.

```go
func (g *Gateway) Start() error {
    for {
        frame, err := readFrame(g.conn)
        if err != nil {
            return err
        }
        g.SendRequestWithLabel(gen.Atom("auth"), CheckToken{Token: frame.Token}, frame.ID)
    }
}
```

Whichever goroutine asked, the answer arrives in the meta-process's own `HandleResponse`, in the Actor Handler. The request is made with the voice of the parent: the callee sees the parent's PID, as it does for everything a meta-process sends, yet the answer goes to the meta-process and never to the parent. For the same reason a request to the parent - by PID, by registered name or by its alias - and a request to the meta-process itself return `gen.ErrNotAllowed`: either would be the parent asking itself.

How the two goroutines share a request, and what that means for the code around it, is described in [Meta-Process](../basics/meta-process.md#requests-without-blocking).

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
