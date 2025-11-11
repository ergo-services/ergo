---
description: What is a Node in Ergo Framework?
---

# Node

A Node is the core of the service you create using Ergo Framework. This core includes:

* **Process Management Subsystem:** handles the starting/stopping of processes, and the registration of process names/aliases.
* **Message Routing Subsystem:** manages the routing of asynchronous messages and synchronous requests between processes.
* **Pub/Sub Subsystem:** powers the [event](events.md), [link, and monitor](links-and-monitors.md) functionalities, enabling distributed event handling.
* **Network Stack:** provides [service discovery](../networking/service-discovering.md) and [network transparency](../networking/network-transparency.md), facilitating seamless communication between nodes.
* [**Logging**](logging.md) **Subsystem**

### Starting a node

To start a node, the first step is to define its name. The name consists of two parts: `<name>@<hostname>`, where the hostname determines on which network interface the port for incoming connections will be opened.

The node's name must be unique on the host. This means that two nodes with the same name cannot be running on the same host.

Below is an example code for starting a node:

```go
import (
    "ergo.services/ergo"
    "ergo.services/ergo/gen"
)

func main() {
    name := "example@localhost"

    // Start node. Returns gen.Node interface
    node, err := ergo.StartNode(name, opts)
    if err != nil {
        panic(err)
    }
    
    node.Wait()
}
```

If the `gen.NodeOptions.Applications` option includes applications when starting the node, they will be automatically loaded and started. However, if any application fails to start, the `ergo.StartNode(...)` function will return an error, and the node will shut down. Upon a successful start, this function returns the `gen.Node` interface.

You can also specify environment variables for the node using the `Env` option in `gen.NodeOptions`. All processes started on this node will inherit these variables. The `gen.Node` interface provides the ability to manage environment variables through methods like `EnvList()`, `SetEnv(...)`, and `Env(...)`. However, changes to environment variables will only affect newly started processes. Environment variable names are case-insensitive.

Additionally, the `gen.Node` interface provides the following methods:

* **Starting/Stopping Processes**: methods like `Spawn(...)`, `SpawnRegister(...)` allow you to start processes, while `Kill(...)` and `SendExit(...)` are used to stop them.
* **Retrieving Process Information**: use the `ProcessInfo(...)` method to get information about a process, and the `MetaInfo(...)` method to retrieve information about meta-processes.
* **Managing the Node's Network Stack**: access the `gen.Network` interface through the `Network()` method to manage the node's network operations.
* **Getting Node Uptime**: the `Uptime()` method provides the node's uptime in seconds.
* **General Node Information**: use the `Info()` method to retrieve general information about the node.
* **Sending Asynchronous Messages**: the `Send(...)` method allows you to send asynchronous messages to a process.

The full list of available methods for the `gen.Node` interface can be found in the reference documentation.

### Process management

The mechanism for starting and stopping processes is provided by the node's core. Each process is assigned a unique identifier, `gen.PID`, which facilitates message routing.

The node also allows you to register a name associated with a process. You can register such a name at the time of process startup by using the `SpawnRegister` method and specifying the desired name. Alternatively, you can use the `RegisterName` method from the `gen.Node` or `gen.Process` interfaces to assign a name to an already running process.

A process can only have one associated name. If you want to change a process's name, you must first unregister the existing name using the `UnregisterName` method from the `gen.Node` or `gen.Process` interfaces. Upon success, this method returns the `gen.PID` of the process previously associated with that name.

A process can be terminated by the node by sending it an exit signal. This is done via the `SendExit` method in the `gen.Node` interface. If necessary, the node can also forcefully stop a process using the `Kill` method provided by the `gen.Node` interface.

### Message routing

One of the key responsibilities of a node is message routing between processes. This routing is transparent to both the sender and the receiver processes. If the recipient process is located on a remote node, the node will automatically attempt to establish a network connection to the remote node and deliver the message to the recipient. This is the [**network transparency**](../networking/network-transparency.md) provided by the Ergo Framework—you don't need to worry about how to send the message or how to encode it, as the node handles all of this automatically and transparently for you.

Message routing is not limited to the `gen.PID` process identifier. A message can also be addressed using:

* **Local Process Name (`gen.Atom`)**: messages can be sent to a local process by referencing its registered name.
* **`gen.Alias`**: this is a unique identifier that can be created by the process itself. It is often used for [meta-processes](meta-process.md) or as a process temporary identifier. You can read more about this feature in the [Process](process.md) section.
* **`gen.ProcessID`**: This is used for sending messages by the recipient process's name. The structure contains two fields—`Name` and `Node`. It is a convenient way to send a message to a process on a remote node when you do not know the `gen.PID` or `gen.Alias` of the remote process.

Through these flexible options, the Ergo Framework provides a robust and seamless message routing system, simplifying communication between processes whether they are local or remote.

### Pub/Sub subsystem

The core of the node implements a **publisher/subscriber** mechanism, which serves as the foundation for process linking, monitoring, and connections with other nodes.

* **Monitoring Functionality**: this allows any process to monitor other processes or nodes. In the event that a monitored process stops or the connection to a node is lost, the process that created the monitor will receive a `gen.MessageDownPID` or `gen.MessageDownNode` message, respectively.
* **Linking Functionality**: similar to monitoring, linking differs in that when a linked process terminates or the connection to a node is lost, the process that created the link will receive an _exit signal_, causing it to stop as well.
* **Event System**: the publisher/subscriber mechanism is also used in the _events_ functionality. It allows any process to register its own event type and act as a producer of those events, while other processes can subscribe to those events.

Thanks to network transparency, the pub/sub subsystem works not only for local processes but also for remote ones.

You can read more about these features in the sections on [Links and Monitors](links-and-monitors.md) and [Events](events.md).

### Network stack

The process of sending a message to a remote node involves several steps:

1. **Connection Establishment**: if a connection to the remote node has not yet been created, the node [queries the registrar](../networking/service-discovering.md) on the host where the target node (owner of the recipient `gen.PID`, `gen.ProcessID`, or `gen.Alias`) is running. The registrar returns the port number on which the target node accepts incoming connections. The connection to this node is then established. This connection remains active until one of the nodes explicitly closes it by calling `Disconnect` from the `gen.RemoteNode` interface.
2. **Message Encoding**: the message is encoded into the binary [EDF format](../networking/network-transparency.md#edf-ergo-framework-data-format).
3. **Data Compression**: if compression was enabled for the sender process, the binary data is compressed.
4. **Message Transmission**: the message is sent over the network using the ENP protocol.
5. **Message Decoding and Delivery**: remote node automatically decodes the received message and ensures its delivery to the recipient process's mailbox.

For more detailed information on network interactions, refer to the [Network Stack](../networking/network-stack.md) section.

### Node shutdown

You can stop a node using the `Stop()` or `StopForce()` methods from the `gen.Node` interface.

* **`Stop`**: all processes will be sent an exit signal with the reason `gen.TerminateReasonShutdown` from the parent process, and the node will wait for all processes to terminate. Once all processes have stopped, the node's network stack and the node itself will be shut down.
* **`StopForce`**: in the case of a forced shutdown, all processes will be terminated using the `Kill` method from the `gen.Node` interface, without waiting for their graceful shutdown.

{% hint style="info" %}
If you call the `Stop` method of the `gen.Node` interface from a running process (for example, within the `HandleMessage`callback of your actor), this will create a _deadlock_. The process will remain in the _running_ state and will be unable to terminate, while the `Stop` method will wait for all processes to stop before shutting down the node.

To avoid this issue, you should either invoke the `Stop` method in a separate goroutine or use the `StopForce` method of the `gen.Node` interface to stop the node from within a process.&#x20;
{% endhint %}

