---
description: What is a Node in Ergo Framework?
---

# Node

A node is the runtime that owns processes and their registries. It starts and stops them, routes messages and hosts services such as logging, cron and the network stack.

Start one node to run local actors. Give nodes distinct names and configure their connections when those actors need to communicate across a cluster. This chapter covers startup, options and shutdown.

## What a Node Provides

**Process Management** - The node tracks every process running on it. When you spawn a process, the node assigns it a unique PID, registers it in the process table, and manages its lifecycle. When a process terminates, the node cleans up its resources and notifies any processes that were linked or monitoring it. The node provides `ProcessRangeShortInfo` for efficient iteration over all processes with their current state, including mailbox latency when built with `-tags=latency`.

**Message Routing** - When a process sends a message, the node figures out where it needs to go. Local process? Route it directly to the mailbox. Remote process? Establish a network connection if needed and send it there. The sender doesn't need to know these details.

**Network Stack** - The node handles all network communication. It discovers other nodes, establishes connections, and encodes messages.

**Pub/Sub System** - Links, monitors, and events all work through a publisher/subscriber mechanism in the node core. When a process terminates or an event fires, the node knows who's subscribed and delivers the notifications. The node provides `EventInfo` to query statistics for a specific event and `EventRangeInfo` for callback-based iteration over all registered events with their per-event counters (messages published, local/remote deliveries).

**Logging** - Every log message goes through the node, which fans it out to registered loggers.

## Starting a Node

A node needs a name. The format is `name@hostname`, where the hostname determines which network interface to use for incoming connections.

```go
node, err := ergo.StartNode("myapp@localhost", gen.NodeOptions{})
if err != nil {
    panic(err)
}
defer node.Wait()
```

The name must be unique on the host. Two nodes with the same name can't run on the same machine, but nodes with different names can coexist.

The `gen.NodeOptions` parameter configures the node: which applications to start, environment variables, network settings, logging configuration. If you specify applications in the options, the node loads and starts them automatically. If any application fails to start, the entire node startup fails - this ensures you don't end up in a partially initialized state.

## Process Lifecycle

The node manages the complete process lifecycle.

When you spawn a process, the node creates it, registers it in the process table, calls its `ProcessInit` callback, and transitions it to the sleep state. The process is now live and can receive messages.

When a process terminates, the runtime removes its PID, name and aliases from their registries and runs subscription cleanup. It attempts to notify linked and monitoring processes, cancels pending requests and stops owned meta processes. The behavior's `ProcessTerminate` callback runs after this core cleanup; application termination accounting waits for that callback to return. The old PID no longer addresses a live process.

Processes can register names, making them addressable by name rather than PID. This is useful for well-known processes that other parts of the system need to find. The node maintains a name registry, ensuring each name maps to exactly one process.

## Message Routing

Message routing is one of the node's core responsibilities.

When a process sends a message locally, the node places it in the recipient's mailbox and schedules dispatch if the process is asleep. A dispatch goroutine runs the process callbacks and returns when there is no more work. Sleep describes a process state; it does not mean that every idle process keeps a parked goroutine.

When the message goes to a remote process, the node checks if a connection exists to the remote node. If not, it discovers the remote node's address (through the registrar or static routes) and establishes a connection. The message is encoded into the Ergo Data Format, optionally compressed, and sent over the network. The remote node receives it, decodes it, and delivers it to the recipient's mailbox.

The sender uses the same method and address type for both paths. Their failure reports differ: a local send can report a missing recipient during routing, while an ordinary remote send does not return the peer's routing result. [Important Delivery](../advanced/important-delivery.md) explains how to request that report.

## Network Communication

Making remote message delivery work like local delivery requires solving three problems: finding remote nodes, establishing connections, and ensuring compatibility.

The first problem is discovery. When you send to a remote process, the node extracts which node that process belongs to from its identifier. Every node runs a small registrar service by default. For nodes on the same host, you query the local registrar. For nodes on different hosts, you query the registrar on that remote host - the framework derives the hostname from the node name and sends the query there. The registrar responds with connection information.

This default approach works for simple setups but has limitations. You're querying individual hosts, which requires them to be directly reachable. There's no cluster-wide view, no centralized configuration, no way to discover which applications are running where.

With etcd or Saturn, nodes register with an external service that also tracks application routes and configuration. The client may resolve from a local mirror maintained by watches or events, so a lookup need not contact the service each time. That mirror can temporarily lag a membership change. [Service Discovering](../networking/service-discovering.md) describes the client behavior.

Once a node is discovered, connections are established. Multiple TCP connections form a pool to that node, enabling parallel message delivery. The connections negotiate protocol details during handshake: which protocol version to use, whether compression is supported, what features are enabled. This negotiation allows nodes with different capabilities to work together.

## Environment and Configuration

Nodes have environment variables that all processes inherit. This provides a way to configure behavior without hardcoding values. A process can override inherited variables or add its own, creating a hierarchy: process environment overrides parent, which overrides leader, which overrides node.

Environment variables are case-insensitive. Whether you set "database_url" or "DATABASE_URL", the process sees the same value.

## Shutdown

Stopping a node can be graceful or forced.

Graceful shutdown announces itself first: the node publishes `gen.MessageCoreNodeStopping` on its [event bus](events.md#the-nodes-own-event-bus) and waits `ShutdownDrain` before touching anything. Then it stops the running applications, each one through its own `Stop` and `Terminate` callbacks, and sends exit signals to whatever processes are left. Processes receive `gen.TerminateReasonShutdown` and can save state, close connections, or send final messages before terminating, and the node waits for their `ProcessTerminate` callbacks to return rather than only for them to leave the process table. Once everything has stopped, the network stack shuts down, the loggers are closed, and the node exits.

Applications are stopped from the latest started to the earliest. Since an application is started after everything it declares in `Depends`, a dependency outlives what depends on it: a service that collects metrics or answers health probes stays up while the applications using it go down, provided they declare it.

The drain window exists for the subscribers of that announcement. A readiness probe switching to 503 needs the load balancer to notice before the applications start closing, and that takes seconds rather than microseconds. `ShutdownDrain` is zero by default, so nothing waits unless you ask for it.

```go
options := gen.NodeOptions{
    ShutdownDrain:   5 * time.Second,
    ShutdownTimeout: 30 * time.Second,
}
```

Forced shutdown kills all processes immediately without waiting for cleanup. It neither announces nor drains, and a `StopForce` issued while the node is draining cuts the wait short. This is useful when you need to stop quickly, but processes don't get a chance to clean up properly.

One subtlety: if you call `Stop` from within a process, you create a deadlock. The process can't terminate because it's waiting for `Stop` to complete, but `Stop` is waiting for all processes (including this one) to terminate. The solution is either to call `Stop` in a separate goroutine or use `StopForce`, which doesn't wait.

### Shutdown Timeout

Graceful shutdown can hang indefinitely if a process is stuck - perhaps blocked on a channel, waiting for an external resource, or caught in incorrect logic. To prevent this, the node has a shutdown timeout.

When the timeout expires, the node escalates: it force-kills any remaining processes and waits a short settle window (5 seconds) for them to unregister. If they still refuse to die, the node hard-exits with error code 1 after logging the surviving processes.

The default timeout is 3 minutes. You can change it globally through `gen.NodeOptions`:

```go
options := gen.NodeOptions{
    ShutdownTimeout: 30 * time.Second,
}
```

or per call via `StopWithTimeout`, which overrides the node-level value for that invocation:

```go
node.StopWithTimeout(10 * time.Second)
```

During shutdown, the node logs which processes are still running. Every 5 seconds, it prints a warning with the first 10 pending processes, showing their PID, registered name (if any), behavior type, state, and mailbox queue length. This diagnostic output helps identify what's blocking the shutdown:

```
[warning] node 'myapp@localhost' is still waiting for process(es) to terminate:
[warning]   <ABC123.0.1004> ('worker_1', main.Worker) state: running, queue: 1
[warning]   <ABC123.0.1005> ('worker_2', main.Worker) state: running, queue: 0
[warning]   <ABC123.0.1006> (main.Worker) state: running, queue: 5
```

The state tells you what the process is doing: `running` means it's handling a message, `sleep` means it's idle waiting for messages. The queue count shows how many messages are waiting. A process stuck in `running` with a growing queue indicates it's blocked in a callback and not processing its mailbox.

When the timeout fires, the same listing is printed at error level along with the force-kill notice, and the post-settle hard-exit report (if reached) shows whichever processes survived the kill so you can identify culprits that ignore termination signals.

Under an orchestrator the whole sequence has to fit into the window the orchestrator gives you. In Kubernetes that window is `terminationGracePeriodSeconds`, and what has to fit inside it is `ShutdownDrain`, plus up to five seconds per application that does not finish its own teardown in time, plus `ShutdownTimeout` for the remaining processes, plus the five-second settle window after the force-kill. If the sum exceeds the grace period, the kubelet sends SIGKILL in the middle of it and the rest of the sequence never runs.

## Node Incarnation

Every node has a creation timestamp assigned when it starts. This timestamp is embedded in every `gen.PID`, `gen.Ref`, and `gen.Alias` that the node creates.

When two nodes connect, they exchange their creation timestamps during the handshake. Each connection stores the remote node's creation value.

Before sending any message to a remote process, the framework compares the target's `Creation` field against the stored creation of that remote node. If they differ, the operation returns `gen.ErrProcessIncarnation` immediately - no network message is sent.

This mechanism handles a common distributed systems problem: what happens when a remote node restarts? After restart, the node gets a new creation timestamp. Any `gen.PID` or `gen.Alias` from before the restart now contains the old creation value. When you try to send a message using that stale identifier, the framework detects the mismatch and returns an error instead of delivering the message to a wrong process.

The check applies to all remote operations: `Send`, `Call`, `Link`, `Unlink`, `Monitor`, `Demonitor`, `SendExit`, and `SendResponse`.

## The Node's Role

The node is infrastructure, not application logic. It provides the mechanisms - process management, message routing, networking - that your actors use. You do not write code to discover remote nodes or encode messages.

The following chapters dive into specific node capabilities. [Process](process.md) explains the actor lifecycle and operations. [Networking](../networking/network-stack.md) covers distributed communication. [Links and Monitors](links-and-monitors.md) explains how processes track each other.
