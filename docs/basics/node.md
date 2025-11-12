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

When you spawn a process, the node creates it, calls its `ProcessInit` callback, registers it in the process table, and transitions it to the sleep state. The process is now live and can receive messages.

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

## The Node's Role

The node is infrastructure, not application logic. It provides the mechanisms - process management, message routing, networking - that your actors use to accomplish work.

This separation is important. Your actors focus on application logic: handling requests, processing data, managing state. The node handles the plumbing: routing messages, establishing connections, managing lifecycles. You don't write code to discover remote nodes or encode messages. The node does that.

This is what makes the framework approachable. You write actors that send and receive messages, and the node makes it all work, whether processes are local or distributed across a cluster.

The following chapters dive into specific node capabilities. [Process](process.md) explains the actor lifecycle and operations. [Networking](../networking/network-stack.md) covers distributed communication. [Links and Monitors](links-and-monitors.md) explains how processes track each other.
