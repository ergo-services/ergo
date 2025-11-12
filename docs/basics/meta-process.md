---
description: Bridging the Actor Model with Blocking I/O
---

# Meta-Process

The actor model works beautifully for asynchronous message passing, but what about when you need to integrate with the synchronous world? HTTP servers that block waiting for requests. TCP accept loops that wait for connections. File I/O that blocks on reads. These don't fit naturally into the one-message-at-a-time actor model.

Meta processes solve this problem by using two goroutines instead of one.

## The Two-Goroutine Design

A meta process has a forever-running goroutine that executes blocking operations, and a message-handling goroutine that processes messages from other actors when they arrive.

The forever-running goroutine executes the `Start` method. This is where you put blocking code: accepting TCP connections, listening for HTTP requests, reading from files, or waiting on any synchronous API. This goroutine runs for the meta process's entire lifetime.

The message-handling goroutine is created on-demand when messages arrive, just like a regular process. It runs `HandleMessage` or `HandleCall` callbacks, processes the message, and terminates if nothing else is waiting. This goroutine handles actor-model message passing.

This design bridges the two worlds. The `Start` goroutine interacts with blocking I/O. The message-handling goroutine interacts with other actors. Both can safely call meta process methods like `Send` because the framework handles the synchronization.

## Why This Matters

Consider an HTTP server. The server needs to block waiting for requests - that's how HTTP libraries work. But when a request arrives, you want to send it to a worker actor for processing. Without meta processes, you'd have to spawn goroutines, manage synchronization, and break the actor model.

With a meta process, the HTTP server runs in the `Start` goroutine. When a request arrives, you send it to a worker actor using the regular `Send` method. The worker processes it asynchronously and sends the response back. The meta process receives the response in `HandleMessage` and writes it to the HTTP connection. The actor model stays intact while integrating with blocking HTTP operations.

## Creating Meta Processes

Meta processes implement the `gen.MetaBehavior` interface:

```go
type MetaBehavior interface {
    Init(process MetaProcess) error
    Start() error
    HandleMessage(from PID, message any) error
    HandleCall(from PID, ref Ref, request any) (any, error)
    Terminate(reason error)
    HandleInspect(from PID, item ...string) map[string]string
}
```

The `Init` callback runs once during creation. The `Start` callback is your blocking code - it runs in the main goroutine until it returns, at which point the meta process terminates. The `HandleMessage` and `HandleCall` callbacks handle messages from other actors. The `Terminate` callback runs during shutdown for cleanup.

Spawn a meta process from a regular process:

```go
meta := createWebHandler(options)
alias, err := process.SpawnMeta(meta, gen.MetaOptions{})
```

The returned alias is how other processes address this meta process.

## Built-In Meta Processes

Ergo Framework provides ready-to-use meta processes for common scenarios:

**TCP** - Server and client meta processes for TCP connections. The server accepts connections and spawns a meta process for each. The client maintains a connection and exchanges messages.

**UDP** - Server meta process for UDP sockets. Receives datagrams and sends them as messages to workers.

**Web** - HTTP server and handler meta processes. The server listens for requests. Handlers process requests and can delegate to worker actors.

**Port** - Wraps external programs, reading their stdout and writing to stdin. Useful for integrating with non-Ergo programs.

**WebSocket** - Server and client for WebSocket connections (separate package: `ergo.services/meta/websocket`).

These implementations handle the complexity of integrating blocking I/O with the actor model, so you don't have to.

## Limitations and Trade-offs

Meta processes can send messages and spawn other meta processes, but they can't make synchronous calls, create links, or establish monitors. These limitations exist because meta processes operate outside the standard actor model - they have two goroutines, so synchronous operations would be ambiguous (which goroutine waits for the response?).

Meta processes don't have their own environment variables. They share the parent process's environment. This keeps the relationship clear - a meta process is an extension of its parent, not an independent entity.

When the parent process terminates, all its meta processes terminate too. This cascading termination ensures cleanup happens automatically.

The two-goroutine design means you need to be careful about concurrent access to the meta process's data. Both goroutines can access the same fields. If your `Start` method modifies state that `HandleMessage` reads, you need synchronization. However, if `Start` only does I/O and `HandleMessage` only sends messages, no synchronization is needed.

## Practical Patterns

The typical pattern is to use meta processes as bridges. The `Start` method handles blocking I/O. When external events occur (HTTP request, TCP connection, data received), the meta process sends messages to worker actors. Workers process requests asynchronously and send results back. The meta process receives results in `HandleMessage` and bridges them back to the synchronous world.

This keeps the actor model intact. Workers are pure actors with sequential message handling. The meta process handles the messy details of integrating with blocking APIs.

For details on specific meta process implementations, see the chapters on [TCP](../meta-processes/tcp.md), [UDP](../meta-processes/udp.md), [Web](../meta-processes/web.md), and [Port](../meta-processes/port.md).
