# Overview

Ergo is an actor runtime for Go. A node hosts processes, and each process handles messages and keeps its own state. Processes have addresses that other processes can use on the same node or across a node connection.

This documentation assumes familiarity with Go. The Basics chapters introduce the runtime; Actors describes the supplied behaviors; Meta Processes connects actors to external I/O. Networking follows messages between nodes. Testing and Advanced cover tests, deployment and diagnosis. The extra library and Tools sections describe separately distributed packages.

## Reading the Documentation

For a first reading, follow [Actor Model](basics/actor-model.md), [Node](basics/node.md), [Process](basics/process.md) and [Actor](actors/actor.md). These chapters explain the execution model and the callbacks used in the example below.

Next, read [Links and Monitors](basics/links-and-monitors.md), [Supervision Tree](basics/supervision-tree.md), [Supervisor](actors/supervisor.md) and [Application](basics/application.md). They describe process relationships, recovery and startup. [Project Structure](basics/project-structure.md) combines them in a project layout.

Once you can run a local application, choose a branch according to the work you need to do:

- [Pool](actors/pool.md) and [Router](actors/router.md) distribute work among processes.
- [Meta-Process](basics/meta-process.md) introduces the I/O behaviors, including [TCP](meta-processes/tcp.md) and [Web](meta-processes/web.md).
- [Network Stack](networking/network-stack.md) starts the explanation of node connections and remote delivery.
- [Testing](testing/overview.md) leads to a first actor test with [Unit](testing/unit.md).
- [Debugging](advanced/debugging.md) and [Observer](advanced/observer.md) explain how to investigate a running system.

[Generic Types](basics/generic-types.md) is a reference for the identifiers used throughout these chapters. API signatures and comments are also available in the package godoc.

## First Process

A behavior factory creates a fresh instance for each process. This actor accepts a string request, logs it and returns a reply:

```go
package main

import (
    "ergo.services/ergo"
    "ergo.services/ergo/act"
    "ergo.services/ergo/gen"
)

type greeting struct {
    act.Actor
}

func factoryGreeting() gen.ProcessBehavior {
    return &greeting{}
}

func (g *greeting) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
    if text, ok := request.(string); ok {
        g.Log().Info("%s", text)
        return "received", nil
    }
    return "unsupported request", nil
}

func main() {
    node, err := ergo.StartNode("example@localhost", gen.NodeOptions{})
    if err != nil {
        panic(err)
    }
    defer node.Stop()

    pid, err := node.Spawn(factoryGreeting, gen.ProcessOptions{})
    if err != nil {
        panic(err)
    }
    if _, err := node.Call(pid, "hello"); err != nil {
        panic(err)
    }
}
```

`Spawn` runs initialization before returning the PID. `Call` waits for a response, so the greeting is handled before `main` exits. The deferred node shutdown then stops the actor. [Actor](actors/actor.md) introduces asynchronous messages and extends the behavior with state and termination handling.

## Core Components

A **node** owns process lifecycles, names, aliases and connections. It also provides logging and scheduled jobs.

A **process** has a mailbox with urgent, system, main and log queues. The runtime runs its callbacks sequentially. When there is no work, it stops dispatching until a message arrives. State used only by that process needs no lock. Go still permits shared references: passing a map, slice or pointer in a local message can expose the same memory to both processes.

A **supervisor** starts children and applies a restart policy when they terminate. Restart creates a new behavior instance. State recovery and retrying application work require an explicit design.

A **meta process** integrates blocking I/O. One execution path reads a socket or runs an external program; another handles messages addressed to its alias. Its owner controls its lifecycle.

## Network Transparency

A remote address uses the same messaging methods as a local address. The node resolves a route, opens a connection if needed, encodes the value and sends it to the peer. Message types crossing the connection must be registered for encoding.

Delivery results differ across this boundary. Ordinary remote `Send` does not report a missing remote recipient. [Important Delivery](advanced/important-delivery.md) requests that report, but even an acknowledged message can be lost if the recipient dies before handling it. A timeout does not establish whether delivery occurred.

Nodes use an embedded registrar by default. External registrars such as etcd and Saturn add cluster-wide discovery, application routes and configuration. [Service Discovering](networking/service-discovering.md) describes their scope and configuration.

## What This Enables

Applications can communicate through process addresses without defining an HTTP or RPC interface between every pair of services. You still design the message contracts, choose where applications run and handle unavailable peers. [Building a Cluster](advanced/building-a-cluster.md) follows those decisions in a deployment example.

## Performance

Mailboxes use lock-free queues. Node connections use a pool of TCP links to carry traffic concurrently. Measurements depend on the message size, number of processes and deployment; benchmark programs are available at [ergo-services/benchmarks](https://github.com/ergo-services/benchmarks).

## Zero Dependencies

The core module uses the Go standard library and declares Go 1.21 as its minimum version. Packages in the extra library are separate modules and may have their own dependencies.
