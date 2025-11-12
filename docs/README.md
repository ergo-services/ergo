# Overview

Building reliable concurrent and distributed systems is hard. In Go, you might start with goroutines and channels. As the system grows, you add mutexes to protect shared state. Then you need to coordinate across multiple services, so you introduce message queues or RPC. Before long, you're managing synchronization primitives, handling partial failures, and debugging race conditions that only appear under load.

Ergo Framework offers a different foundation. Think of it as making goroutines addressable and message-passing-only, then extending that model across a cluster. Processes are like goroutines - lightweight, multiplexed onto OS threads - but isolated and communicating only through messages. Each process has an identifier that works whether the process is local or on a remote node. Sending a message looks the same either way.

The actor model isn't new. Erlang proved these patterns work for systems requiring massive concurrency and high reliability. Ergo brings them to Go: no external dependencies, familiar Go idioms, and performance that doesn't sacrifice correctness for speed.

## Core Components

The framework consists of a few fundamental pieces that work together.

A **node** provides the runtime environment. It manages process lifecycles, routes messages, handles network connections, and provides services like logging and scheduled tasks. When you start a node, you get infrastructure. When you spawn a process, the node handles the mechanics.

**Processes** are lightweight actors. Each has a mailbox where messages queue up, priority-sorted into urgent, system, main, and log queues. The process handles messages one at a time in its own goroutine. When the mailbox empties, the goroutine sleeps. This makes processes efficient - you can have thousands without resource problems. It also makes them safe - sequential message handling means no race conditions within a process.

**Supervision trees** provide fault tolerance. Supervisors monitor worker processes. When a worker crashes, the supervisor restarts it according to a configured strategy. Supervisors can supervise other supervisors, creating a hierarchy. Failures are isolated to subtrees. The rest of the system continues running while the failed part recovers.

**Meta processes** solve a specific problem: integrating blocking I/O with the actor model. HTTP servers block waiting for requests. TCP servers block accepting connections. A meta process uses two goroutines - one runs your blocking code (like `http.ListenAndServe`), the other handles messages from other actors. This bridges synchronous APIs with asynchronous actor communication.

## Network Transparency

The framework treats local and remote processes identically. Send a message to a process on the same node or a process on a remote node - the code is the same. The framework handles the difference.

When you send to a remote process, the node extracts the target node from the process identifier, discovers that node's address (through static routes or a registrar), establishes a connection if needed, encodes the message, and sends it. The remote node receives it, decodes it, and delivers it to the target process's mailbox. This happens automatically. Your code just sends a message.

This transparency extends to failure detection. Use the Important delivery flag and you get the same error semantics for remote processes as for local ones. Without it, a message to a missing remote process times out (was it slow or dead?). With it, you get immediate error notification (process doesn't exist), just like local delivery. The network becomes transparent not just for success cases but for failures too.

Nodes discover each other through a registrar. By default, each node runs a minimal registrar. Nodes on the same host find each other through localhost. For remote nodes, the framework queries the registrar on the remote host. For production clusters, configure an external registrar like etcd or Saturn for centralized discovery, cluster configuration, and application deployment tracking.

## What This Enables

You write business logic using message passing between processes. The framework handles concurrency (processes run in parallel but each is sequential internally), fault tolerance (supervisors restart failures), and distribution (messages route automatically to remote processes). You're not writing code to manage connections, encode messages, or handle network failures explicitly. Those are solved problems handled by the framework.

Systems built this way have useful properties. They scale by adding nodes and distributing processes across them. The code doesn't change - deployment topology is operational configuration. They handle failures through supervision rather than defensive programming everywhere. They evolve through composition - add new process types, adjust supervision strategies, change message flows - without restructuring the foundation.

The development experience differs from typical microservices. No REST endpoints to define. No service discovery to configure (it's built in). No serialization libraries to manage (the framework handles it). No retry logic scattered throughout (supervision handles recovery). You model your domain as processes exchanging messages, and the framework provides the infrastructure.

## Performance

Lock-free queues in process mailboxes avoid contention. Processes sleep when idle, consuming no CPU. Connection pooling uses multiple TCP connections per remote node for parallel delivery. These design choices add up to performance comparable to hand-written concurrent code, but without the complexity.

The real performance benefit is development velocity. You're not debugging race conditions or deadlocks. You're not coordinating distributed transactions. You're not managing connection pools or implementing retry logic. The framework handles those concerns, leaving you to focus on what your system does.

Benchmarks measuring message passing, network communication, and serialization performance are available at [github.com/ergo-services/benchmarks](https://github.com/ergo-services/benchmarks).

## Zero Dependencies

The framework uses only the Go standard library. No external dependencies means no version conflicts, no supply chain vulnerabilities, no surprise breaking changes from third-party packages. The requirement is just Go 1.20 or higher.

This isn't ideological purity. It's practical stability. The framework's behavior depends only on Go itself. Updates are predictable. Supply chain is simple. The code you write today will compile and run the same way years from now, assuming Go maintains backward compatibility (which it does).

For detailed explanations of these concepts, start with [Actor Model](basics/actor-model.md) and explore the [Basics](basics/actor-model.md) section. For API documentation, see the godoc comments in the source code.
