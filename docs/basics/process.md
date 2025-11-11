---
description: What is a Process in Ergo Framework
---

# Process

In Ergo Framework, a _process_ is a lightweight entity that operates on top of a goroutine and is based on the actor model. Each process has a _mailbox_ for receiving incoming messages. The size of the mailbox is determined by the `gen.ProcessOptions.MailboxSize` parameter when the process is started. By default, this value is set to zero, which makes the mailbox unlimited in size. Inside the mailbox, there are several queues—_Main_, _System_, _Urgent_, and _Log_—that prioritize the processing of messages.

Any running process can send messages and make synchronous calls to other processes, including those running on remote nodes. Additionally, a process can spawn new processes, both locally and on remote nodes (see [Remote Spawn Process](../networking/remote-spawn-process.md)).

### Process Identifiers

Processes in Ergo Framework are created and run within the scope of a node. Each process is assigned a unique identifier called `gen.PID` when it is started. The `gen.PID` consists of several key components:

* `Node`: name of the node on which the process is running.
* `ID`:  unique number that identifies the process within its node.
* `Creation`: property that represents the "incarnation" of the process, meaning the specific instance of the node in which the process was created.

This combination of properties allows for:

* **Identification Across Nodes**: node name is used for routing messages to remote nodes where the process resides. Once routed to the correct node, the process's unique sequential number is used for local message routing on the remote node.
* **Process Incarnation Handling**: `Creation` property allows the system to distinguish between different instances of a node. This property has the same value for all processes within a node's lifecycle. If the node is restarted, the `Creation` value changes (it stores the timestamp of the node's startup).

If you attempt to send a message to a process with a `gen.PID` that belongs to a previous incarnation of the node, an error `gen.ErrProcessIncarnation` will be returned.

This mechanism ensures that processes are accurately identified and that message routing remains reliable, even across restarts of nodes.

#### gen.ProcessID

A process can also be identified by a name associated with it. You can register such a name when starting a process using the `SpawnRegister` method of the `gen.Node` interface. Additionally, a process can register its name after it has started by calling the `RegisterName` method of the `gen.Process` interface.

If you want to change the associated name of a process, you first need to unregister the current name. This is because a process is limited to having only one associated name at a time. The `UnregisterName` method of the `gen.Process`interface allows you to remove the current name registration.

To send a message to a process by its registered name, you can use the `SendProcessID` method of the `gen.Process`interface. For making a synchronous request by name, the `gen.Process` interface provides the `CallProcessID` method.

These methods make it easy to send messages and interact with processes using their associated names, without needing to know their `gen.PID`, especially when dealing with remote or dynamically created processes.

<pre class="language-go"><code class="lang-go">type myActor struct {
    act.Actor
}
...
func (a *myActor) HandleMessage(from gen.PID, message any) error {
    ...
    remoteProcess := gen.ProcessID{Name:"p1", Node:"node2@localhost"}
    // sending async message to the remote process with associated name
    a.SendProcessID(remoteProcess, "hello")
    // making a sync request to the remote process with associated name
    result, err := a.CallProcessID(remoteProcess, "hi")
    ...
    // you can also use methods Send and Call
    a.Send(remoteProcess)
    result, err = a.Call(remoteProcess)
<strong>    ...
</strong>}
</code></pre>

To send a message or make a synchronous request to a local process, you can use the universal methods `Send` and `Call` of the `gen.Process` interface, respectively:

```go
 func (a *myActor) HandleMessage(from gen.PID, message any) error {
    ...
    localProcess := gen.Atom("local")
    a.Send(localProcess, "hello")
    result, err := a.Call(localProcess, "hi")
    ...
 }
```

#### gen.Alias

In Ergo Framework, processes have the ability to create temporary identifiers known as _aliases_. A process can generate an unlimited number of aliases using the `CreateAlias` method of the `gen.Process` interface. These aliases remain valid throughout the lifetime of the process or until the process explicitly removes them using the `DeleteAlias` method of the `gen.Process` interface.

Aliases provide additional flexibility by allowing a process to be referenced by multiple temporary identifiers, useful for handling specific tasks or interactions where a unique, temporary process reference is needed.

### Process states

A process in Ergo Framework can be in one of the following states:

* _Init_ - This is the state of the process during its startup. At this stage, the process is not yet registered in the node, so only a limited set of methods from the `gen.Process` interface are available—such as modifying process properties (environment variables, message compression settings, setting priorities for outgoing messages). Attempts to use methods like `RegisterName`, `RegisterEvent`, `CreateAlias`, `Call*`, `Link*`, and `Monitor*` will return the error `gen.ErrNotAllowed`. However, methods like `Send*` and `Spawn*` remain available in this state.
* _Sleep_ - This state occurs when the process has no messages in its mailbox, and no goroutine is running for the process. The process is idle and consumes no CPU resources in this state.
* _Running_ - When a process receives a message in its mailbox, a goroutine is launched, and the appropriate callback is executed. Once the message has been processed and if there are no other messages in the mailbox, the process returns to the _Sleep_ state, and the goroutine is terminated.
* _WaitResponse_ - The process transitions to this state from _Running_ when it makes a synchronous request (`Call*`). The process waits for a response to the request, and once received, it transitions back to the _Running_ state
* _Zombee_ - This is an intermediate state that occurs when a process was in the _Running_ state but the node forcibly stopped the process using the `Kill` method. In this state, the process cannot access any methods from the `gen.Process` interface. After processing the current message and terminating the goroutine, the process transitions from _Zombie_ to _Terminated_. In this state, the process no longer receives new messages.
* _Terminated_ - This is the final state of the process before it is removed from the node. Messages are not delivered to a process in this state.

You can retrieve the current state of a process using the `ProcessInfo` method of the `gen.Node` interface. Alternatively, you can use the `State` method of the `gen.Process` interface to check the state directly from the process.

These states define the lifecycle of a process and determine what actions it can perform at any given moment in Ergo Framework.

### Environment Variables

Each process in Ergo Framework has its own set of environment variables, which can be retrieved using the `EnvList()`method of the `gen.Process` interface. To modify these variables, the `SetEnv(name gen.Env, value any)` method is used; if `value` is set to `nil`, the environment variable with the specified name will be removed. Environment variable names are case-insensitive, allowing for flexible and dynamic configuration management within processes:

```go
type myActor struct {
    act.Actor
}

func (a *myActor) Init(args ...any) error {
    ...
    a.SetEnv("var", 1) // set variable
    a.SetEnv("Var", 2) // overwrite value, the name is case insensitive
    v, _ := a.Env("vAr") // return 2 as a value
    a.SetEnv("vaR", nil) // remove variable
    ...
}
```

The initialization of a process's environment variables occurs at the moment the process starts. The starting process first inherits the environment variables of the node, then the environment variables of the application and parent process, and finally, the environment variables specified in `gen.ProcessOptions` are added.

### Process starting

To launch a new process in Ergo Framework, there are two methods:

```go
Spawn(factory ProcessFactory, options ProcessOptions, args ...any) (PID, error)
SpawnRegister(register Atom, factory ProcessFactory, options ProcessOptions, args ...any) (PID, error)
```

Both methods are available in the `gen.Node` and `gen.Process` interfaces.

When using the `SpawnRegister` method, the node checks whether the specified name `register` can be assigned to the starting process. If the name is already taken by another process, this function will return the error `gen.ErrTaken`.

#### Parent process

Processes launched by the node (using the `Spawn*` methods of the `gen.Node` interface) are assigned a virtual parent process identifier, known as **CorePID**. You can retrieve this value using the `CorePID` method of the `gen.Node` interface.

If a process is launched using the `Spawn*` methods of the `gen.Process` interface, it becomes a _child process_. The `Parent`method of the `gen.Process` interface in the child process will return the identifier of the process that launched it.

Attempting to send an exit signal to a parent process using the `SendExit` method of the `gen.Process` interface will result in the error `gen.ErrNotAllowed`.

Additionally, the parent process identifier is used in handling exit signals in `act.Actor` (see the section on [TrapExit](../actors/actor.md#trapexit-handling-exit-signal) in [Actor](../actors/actor.md)).

#### Process leader

Each process in Ergo Framework has a _process leader_ identifier, in addition to the parent process identifier. You can retrieve this identifier using the `Leader` method of the `gen.Process` interface. When a process is started by the node, its process leader is set to the virtual identifier **CorePID**. If the process is started by another process, the child process inherits the leader identifier from its parent process.

During process startup, it is also possible to explicitly set a process leader using the `Leader` option in `gen.ProcessOption`. This is commonly done when starting child processes in `act.Supervisor`. While the process leader identifier does not influence the operational logic of processes, it serves an informational role and can be used within your application's logic as needed.

#### gen.ProcessFactory

To start a process in Ergo Framework, you need to specify a `factory` argument, which is a factory function. This function must return an object that implements the `gen.ProcessBehavior` interface. This is a low-level interface that is already implemented in general-purpose actors (such as `act.Actor`, `act.Supervisor`, etc.). Therefore, in most cases, you only need to embed the required actor into your object to satisfy the `gen.ProcessBehavior` interface.&#x20;

Example:

```go
// based on act.Actor
type MyActor struct {
    act.Actor // embed generic actor
}
func factoryMyActor() gen.ProcessBehavior {
    return &MyActor{}
}
...
node.Spawn(factoryMyActor, gen.ProcessOptions{})
```

#### gen.ProcessOptions

The `options` parameter in the `Spawn` or `SpawnRegister` functions allows you to configure various parameters for the process being launched:

* **`MailboxSize`**: defines the size of the mailbox. If not specified, the mailbox will be unlimited in size.
* **`Leader`**: sets the leader of the process group. If not specified, the leader value is inherited from the parent process. This setting is informational and does not affect the process itself but can be used in your application's logic. Typically, this is set by supervisor processes.
* **`Compression`**: specifies the message compression settings when sending messages over the network to remote nodes. This option is ignored if the recipient is a local process.
* **`SendPriority`**: allows you to set the priority for sent messages. There are three priority levels:
  * `gen.MessagePriorityNormal`: Messages are delivered to the _Main_ queue of the recipient's mailbox.
  * `gen.MessagePriorityHigh`: Messages are delivered to the _System_ queue.
  * `gen.MessagePriorityMax`: Highest-priority messages are delivered to the _Urgent_ queue of the recipient's mailbox.
* **`ImportantDelivery`**: enables the _important_ flag for messages sent to remote processes. When enabled, the remote node will send an acknowledgment that the message was delivered to the recipient's mailbox. If the message cannot be delivered, an error (e.g., `gen.ErrProcessTerminated`, `gen.ProcessUnknown`, `gen.ProcessMailboxFull`) will be returned. You can also manage this flag using the `SetImportantDelivery` method of the `gen.Process` interface. This flag only applies to messages sent via the `SendPID`, `SendProcessID`, and `SendAlias` methods or requests made using `CallPID`, `CallProcessID`, or `CallAlias` methods of `gen.Process`. To force a message to be sent with the **important** flag, use the `SendImportant`  or `CallImportant` methods of `gen.Process`.
* **`Fallback`**: specifies a fallback process to which messages will be redirected in the event that the recipient's mailbox is full. Each such message is wrapped in the `gen.MessageFallback` structure. This option is ignored if the recipient process's mailbox is unlimited.
* **`LinkParent`**: Automatically creates a link with the parent process when the process is started (after successful initialization). This option is ignored if the process is started by the node.
* **`LinkChild`**: Automatically creates a link with a child process upon its successful startup (after successful initialization). This option is ignored if the process is started by the node.
* **`Env`**: Sets the environment variables for the process.

{% hint style="info" %}
During the initialization stage, processes do not have access to linking methods (`Link*` from the `gen.Process` interface). However, the `LinkParent` and `LinkChild` options provide a way to bypass this limitation when launching child processes. These options allow the process to automatically create links with the parent and child processes after successful initialization, ensuring that linking is established even during the startup phase
{% endhint %}

### Process termination

In Ergo Framework, a process can be terminated in the following ways:

1. **Forcefully**:  process can be terminated by force using the `Kill` method provided by the `gen.Node` interface.
2. **Sending an Exit Signal**: you can stop a process by sending it an _exit_ signal with a specified reason. This is done using the `SendExit` method, which is available in both the `gen.Node` and `gen.Process` interfaces. The _exit_ signal is always delivered with the highest priority and placed in the _Urgent_ queue. In `act.Actor`, there is a mechanism to intercept _exit_ signals, allowing them to be handled like normal `gen.MessageExit*` messages. More information on this mechanism can be found in the [Actor](../actors/actor.md) section. Sending an _exit_ signal to yourself, your parent process, or the process leader is **not allowed**. In such cases, the `SendExit` method will return the error `gen.ErrNotAllowed`.
3. **Self-Termination**: A process can terminate itself by returning a non-`nil` value (of type `error`) from its message handling callback. The returned value will be treated as the termination reason, and the process will be terminated. You can use `gen.TerminateReasonNormal` or `gen.TerminateReasonShutdown` for normal shutdown scenarios.
4. **Panic**: If a panic occurs during message processing, the process will be terminated with the reason `gen.TerminateReasonPanic`.

Upon termination, all resources associated with the process are released:

* All events registered by the process are unregistered.
* Any associated name, if registered, is freed.
* Aliases, links, and monitors created by the process are removed.
* The process is removed from the logging system if it was previously registered as a logger.
* All meta-processes launched by the process are terminated.

### Process information

You can retrieve summary information about a process using the `ProcessInfo` method of the `gen.Node`interface or the `Info` method of the `gen.Process` interface. Both methods return a `gen.ProcessInfo` structure containing detailed information about the process.



