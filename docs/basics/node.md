---
description: What is a Node in Ergo Framework?
---

# Node

A node is the runtime environment where your actors live. Think of it as the container that hosts processes, routes messages between them, and handles the complexities of distributed communication.

When you start a node, you're launching a complete system with several subsystems working together: process management, message routing, networking, and logging. Each subsystem has a specific responsibility, and they coordinate to provide the foundation for your application.

## What a Node Provides

**Process Management** - The node tracks every process running on it. When you spawn a process, the node assigns it a unique PID, registers it in the process table, and manages its lifecycle. When a process terminates, the node cleans up its resources and notifies any processes that were linked or monitoring it.

**Message Routing** - When a process sends a message, the node figures out where it needs to go. Local process? Route it directly to the mailbox. Remote process? Establish a network connection if needed and send it there. The sender doesn't need to know these details.

**Network Stack** - The node handles all network communication. It discovers other nodes, establishes connections, encodes messages, and manages the complexity of distributed communication. This is what makes network transparency possible.

**Pub/Sub System** - Links, monitors, and events all work through a publisher/subscriber mechanism in the node core. When a process terminates or an event fires, the node knows who's subscribed and delivers the notifications.

**Logging** - Every log message goes through the node, which fans it out to registered loggers. This centralized logging makes it easy to capture, filter, and route log output.

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

When the process terminates (either naturally or through an exit signal), the node calls `ProcessTerminate`, removes it from the process table, and notifies any processes that were linked or monitoring. Resources are cleaned up, and the `gen.PID` becomes invalid.

Processes can register names, making them addressable by name rather than PID. This is useful for well-known processes that other parts of the system need to find. The node maintains a name registry, ensuring each name maps to exactly one process.

## Message Routing

Message routing is one of the node's core responsibilities.

When a process sends a message locally, the node simply places it in the recipient's mailbox. The recipient's goroutine wakes up (if it was sleeping), processes the message, and goes back to sleep if no more messages are waiting.

When the message goes to a remote process, things are more interesting. The node checks if a connection exists to the remote node. If not, it discovers the remote node's address (through the registrar or static routes) and establishes a connection. The message is encoded into the Ergo Data Format, optionally compressed, and sent over the network. The remote node receives it, decodes it, and delivers it to the recipient's mailbox.

From the sender's perspective, both paths look identical. That's network transparency.

## Network Communication

Making remote message delivery work like local delivery requires solving three problems: finding remote nodes, establishing connections, and ensuring compatibility.

The first problem is discovery. When you send to a remote process, the node extracts which node that process belongs to from its identifier. Every node runs a small registrar service by default. For nodes on the same host, you query the local registrar. For nodes on different hosts, you query the registrar on that remote host - the framework derives the hostname from the node name and sends the query there. The registrar responds with connection information.

This default approach works for simple setups but has limitations. You're querying individual hosts, which requires them to be directly reachable. There's no cluster-wide view, no centralized configuration, no way to discover which applications are running where.

That's where etcd or Saturn come in. Instead of each node being its own island with a local registrar, you run a centralized registry service. All nodes register there when they start. All discovery queries go there. The central registrar becomes the source of truth for the cluster, providing not just discovery but configuration management, application tracking, and topology change notifications. It transforms independent nodes into a coordinated cluster.

Once a node is discovered, connections are established. Multiple TCP connections form a pool to that node, enabling parallel message delivery. The connections negotiate protocol details during handshake: which protocol version to use, whether compression is supported, what features are enabled. This negotiation allows nodes with different capabilities to work together.

## Environment and Configuration

Nodes have environment variables that all processes inherit. This provides a way to configure behavior without hardcoding values. A process can override inherited variables or add its own, creating a hierarchy: process environment overrides parent, which overrides leader, which overrides node.

Environment variables are case-insensitive. Whether you set "database_url" or "DATABASE_URL", the process sees the same value. This eliminates a common source of configuration bugs.

## Shutdown

Stopping a node can be graceful or forced.

Graceful shutdown sends exit signals to all processes and waits for them to clean up. Processes receive `gen.TerminateReasonShutdown` and can save state, close connections, or send final messages before terminating. Once all processes have stopped, the network stack shuts down, and the node exits.

Forced shutdown kills all processes immediately without waiting for cleanup. This is useful when you need to stop quickly, but processes don't get a chance to clean up properly.

One subtlety: if you call `Stop` from within a process, you create a deadlock. The process can't terminate because it's waiting for `Stop` to complete, but `Stop` is waiting for all processes (including this one) to terminate. The solution is either to call `Stop` in a separate goroutine or use `StopForce`, which doesn't wait.

### Shutdown Timeout

Graceful shutdown can hang indefinitely if a process is stuck - perhaps blocked on a channel, waiting for an external resource, or caught in incorrect logic. To prevent this, the node has a shutdown timeout. If processes don't terminate within this period, the node force exits with error code 1.

The default timeout is 3 minutes. You can change it through `gen.NodeOptions`:

```go
options := gen.NodeOptions{
    ShutdownTimeout: 30 * time.Second,
}
```

During shutdown, the node logs which processes are still running. Every 5 seconds, it prints a warning with the first 10 pending processes, showing their PID, registered name (if any), behavior type, state, and mailbox queue length. This diagnostic output helps identify what's blocking the shutdown:

```
[warning] node 'myapp@localhost' is still waiting for 3 process(es) to terminate:
[warning]   <ABC123.0.1004> ('worker_1', main.Worker) state: running, queue: 1
[warning]   <ABC123.0.1005> ('worker_2', main.Worker) state: running, queue: 0
[warning]   <ABC123.0.1006> (main.Worker) state: running, queue: 5
```

The state tells you what the process is doing: `running` means it's handling a message, `sleep` means it's idle waiting for messages. The queue count shows how many messages are waiting. A process stuck in `running` with a growing queue indicates it's blocked in a callback and not processing its mailbox.

## Node Incarnation

Every node has a creation timestamp assigned when it starts. This timestamp is embedded in every `gen.PID`, `gen.Ref`, and `gen.Alias` that the node creates.

When two nodes connect, they exchange their creation timestamps during the handshake. Each connection stores the remote node's creation value.

Before sending any message to a remote process, the framework compares the target's `Creation` field against the stored creation of that remote node. If they differ, the operation returns `gen.ErrProcessIncarnation` immediately - no network message is sent.

This mechanism handles a common distributed systems problem: what happens when a remote node restarts? After restart, the node gets a new creation timestamp. Any `gen.PID` or `gen.Alias` from before the restart now contains the old creation value. When you try to send a message using that stale identifier, the framework detects the mismatch and returns an error instead of delivering the message to a wrong process.

The check applies to all remote operations: `Send`, `Call`, `Link`, `Unlink`, `Monitor`, `Demonitor`, `SendExit`, and `SendResponse`.

## The Node's Role

The node is infrastructure, not application logic. It provides the mechanisms - process management, message routing, networking - that your actors use to accomplish work.

This separation is important. Your actors focus on application logic: handling requests, processing data, managing state. The node handles the plumbing: routing messages, establishing connections, managing lifecycles. You don't write code to discover remote nodes or encode messages. The node does that.

This is what makes the framework approachable. You write actors that send and receive messages, and the node makes it all work, whether processes are local or distributed across a cluster.

The following chapters dive into specific node capabilities. [Process](process.md) explains the actor lifecycle and operations. [Networking](../networking/network-stack.md) covers distributed communication. [Links and Monitors](links-and-monitors.md) explains how processes track each other.
