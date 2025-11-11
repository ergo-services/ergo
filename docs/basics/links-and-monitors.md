---
description: Linking and Monitoring Mechanisms
---

# Links And Monitors

These mechanisms allow processes to respond to termination events of targets with which links or monitors have been created. Such targets can include:

* A process identifier (`gen.PID`)
* A process name (`gen.Atom` or `gen.ProcessID`)
* A process alias (`gen.Alias`)
* An event (`gen.Event`)
* A node name (`gen.Atom`, in the case of monitoring a node connection)

The key difference between linking and monitoring lies in the response to the termination event of the target. When a process creates a _link_ with a target, and that target terminates, the linked process receives an _exit signal_, which generally causes it to terminate as well.

In contrast, when a process creates a _monitor_ with a target, and the target terminates, the monitoring process receives a `gen.MessageDown*` message, which contains the reason for the target's termination. This notification allows the monitoring process to react accordingly without necessarily terminating itself.

### Links

In Ergo Framework, links are created between processes to ensure that termination events of one process propagate to linked processes. The `gen.Process` interface provides the following methods for creating and managing links:

* **`Link`**: This universal method allows you to create a _link_ to a target, which can be a `gen.Atom` (for a local process), `gen.PID`, `gen.ProcessID`, or `gen.Alias`. The process will receive an _exit signal_ upon the target process's termination. To remove the link, use the `Unlink` method with the same argument.
* **`LinkPID`**: Creates a _link_ to a process by its `gen.PID`. The process will receive an _exit signal_ upon the target process's termination. To remove the link, use `UnlinkPID` or `Unlink` with the same argument.
* **`LinkProcessID`**: Creates a _link_ to a process by its `gen.ProcessID`. The process will receive an _exit signal_ upon the target process's termination or when the associated name is unregistered. To remove the _link_, use `UnlinkProcessID`or `Unlink`.
* **`LinkAlias`**: Creates a _link_ to a process by its `gen.Alias`. The process will receive an _exit signal_ upon the termination of the process that created the alias or upon the alias's deletion. To remove the _link_, use `UnlinkAlias` or `Unlink`.
* **`LinkNode`**: Creates a _link_ to a node connection using the node name (`gen.Atom`). The process will receive an _exit signal_ if the connection to the target node is lost. To remove the _link_, use `UnlinkNode`.
* **`LinkEvent`**: Creates a _link_ to an event (`gen.Event`). The process will receive an _exit signal_ when the producer process of the event terminates or when the `gen.Event` is unregistered. To remove the _link_, use `UnlinkEvent`. More information about this mechanism can be found in the [Events](events.md) section.

_Exit signals_ are always delivered with the highest priority and placed in the _Urgent_ queue of the process's mailbox.

{% hint style="info" %}
In Erlang, the linking mechanism is **bidirectional**, meaning that if a process that created a _link_ with a target process terminates, it will also cause the termination of the target process.

In contrast, in Ergo Framework, the linking mechanism is **unidirectional**. The termination of the process that created the _link_ has no effect on the target process. Only the termination of the target process triggers an _exit signal_ to the linked process. This design allows more flexibility and control, preventing unintended cascading failures in linked processes.
{% endhint %}

### Monitors

Monitors allow a process to observe the lifecycle of a target process or event. The `gen.Process` interface provides several methods for creating monitors:

* **`Monitor`**: A universal method for creating a _monitor_. The target can be a `gen.Atom` (for a local process), `gen.PID`, `gen.ProcessID`, or `gen.Alias`. The monitoring process will receive a `gen.MessageDown*` message upon the termination of the target process. To remove the _monitor_, use the `Demonitor` method with the same argument.
* **`MonitorPID`**: Creates a _monitor_ for a process by its `gen.PID`. The monitoring process will receive a `gen.MessageDownPID` message upon the target process's termination, with the reason for termination included in the `Reason` field. To remove the _monitor_, use `DemonitorPID` or `Demonitor`.
* **`MonitorProcessID`**: Creates a _monitor_ for a process by its `gen.ProcessID`. The monitoring process will receive a `gen.MessageDownProcessID` message upon the target process's termination, with the reason for termination in the `Reason` field. If the target process unregisters its name, the `Reason` field of the `gen.MessageDownProcessID` will contain `gen.ErrUnregistered`. To remove the _monitor_, use `DemonitorProcessID` or `Demonitor`.
* **`MonitorAlias`**: Creates a _monitor_ for a process by its `gen.Alias`. The monitoring process will receive a `gen.MessageDownAlias` message upon the termination of the process that created the alias or if the alias is deleted. In the case of alias deletion, the `Reason` field of the `gen.MessageDownAlias` will contain `gen.ErrUnregistered`. To remove the _monitor_, use `DemonitorAlias` or `Demonitor`.
* **`MonitorNode`**: Creates a _monitor_ for a node connection. The monitoring process will receive a `gen.MessageDownNode` message upon the disconnection of the node. To remove the _monitor_, use `DemonitorNode`.
* **`MonitorEvent`**: Creates a _monitor_ for an event (`gen.Event`). The monitoring process will receive a `gen.MessageDownEvent` message when the producer of the event terminates or if the event is unregistered. If the event is unregistered, the `Reason` field of the `gen.MessageDownEvent` will contain `gen.ErrUnregistered`. To remove the _monitor_, use `DemonitorEvent`. More details about this mechanism can be found in the [Events](events.md) section.

Messages of type `gen.MessageDown*` are delivered with high priority and placed in the _System_ queue of the process's mailbox.&#x20;

### Remote targets

Thanks to [network transparency](../networking/network-transparency.md), the linking and monitoring mechanisms in Ergo Framework can be used with targets on remote nodes.

When a process creates a _link_ or _monitor_ with a remote target (such as a process, alias, or event), if the connection to the node where the target resides is lost, the process will receive an _exit signal_ or a `gen.MessageDown*` message, depending on the mechanism used. In this case, the reason for termination will be `gen.ErrNoConnection`. This allows processes to handle network disconnections and remote target failures seamlessly, as part of the same fault-tolerant mechanisms used for local processes.

{% hint style="info" %}
It's important to remember that the methods for creating _links_ and _monitors_ are not available to a process during its initialization state (see the [Process](process.md) section for more details).
{% endhint %}
