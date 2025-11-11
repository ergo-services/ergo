# Actor

An actor in Ergo Framework implements the low-level `gen.ProcessBehavior` interface. To launch a process based on `act.Actor`, you need to create an object with an embedded `act.Actor` and implement a factory function for it. For example:

```go
type MyActor struct {
    act.Actor
    i int
}
func factoryMyActor() gen.ProcessBehavior {
    return &MyActor{}
}
```



`act.Actor` uses the `act.ActorBehavior` interface to interact with your object. This interface defines a set of callback methods:

```go
type ActorBehavior interface {
	gen.ProcessBehavior
	
	// main callbacks
	Init(args ...any) error
	HandleMessage(from gen.PID, message any) error
	HandleCall(from gen.PID, ref gen.Ref, request any) (any, error)
	Terminate(reason error)
	
	// extended callbacks used if SplitHandle was enabled
	HandleMessageName(name gen.Atom, from gen.PID, message any) error
	HandleMessageAlias(alias gen.Alias, from gen.PID, message any) error
	HandleCallName(name gen.Atom, from gen.PID, ref gen.Ref, request any) (any, error)
	HandleCallAlias(alias gen.Alias, from gen.PID, ref gen.Ref, request any) (any, error)
	
	// specialized callbacks
	HandleInspect() map[string]string
	HandleLog(message gen.MessageLog) error
	HandleEvent(message gen.MessageEvent) error
	
}

```

All methods in the `act.ActorBehavior` interface are optional, allowing you to implement only the necessary callbacks in your object. Since `act.Actor` embeds the `gen.Process` interface, you can directly use its methods from within your actor object.&#x20;

Example:

```go
func (a *MyActor) Init(args...) error {
    // get the gen.Log interface using Log method of embedded gen.Process interface
    a.Log().Info("starting process %s", a.PID())
    // initialize value
    a.i = 100
    // sending message to itself with methods Send and PID of embedded gen.Process
    a.Send(a.PID(), "hello")
    return nil
}

func (a *MyActor) HandleMessage(from gen.PID, message any) error {
    a.Log().Info("got message from %s: %s", from, message)
    ...
    // handling message
    ...
    return nil
}

func (a *MyActor) Terminate(reason error) {
    a.Log().Info("%s terminated with reason: %s", a.PID(), reason)
}
```

### Process Initialization

Process initialization begins when `act.Actor` invokes the `Init` method of the `act.ActorBehavior` interface, passing the `args` provided during `Spawn` (or `SpawnRegister`). If `Init` completes successfully, the node registers the process, allowing it to receive messages or synchronous requests. If `Init` fails, `Spawn` (or `SpawnRegister`) returns the error. It's important to note that during initialization, the process is not yet registered with the node, limiting access to certain methods of the embedded `gen.Process` interface until the process is fully initialized.

### Handling Asynchronous Messages and Synchronous Requests

The process mailbox contains several queues: _Main_, _System_, _Urgent_, and a special _Log_ queue. In `act.Actor`, messages are processed in the following order: _Urgent_, _System_, _Main_, and then _Log_.

Asynchronous messages are sent using the `Send` method of the `gen.Node` or `gen.Process` interface. Upon receiving such a message, `act.Actor` calls the `HandleMessage` method of the `gen.ActorBehavior` interface.

For synchronous requests, the `HandleCall` callback method is invoked in an actor based on `act.Actor`. The result returned by this method is sent as the response to the request.

A process can send asynchronous messages to itself, but attempting to make a synchronous request to itself will return the error `gen.ErrTimeout`..&#x20;

`act.Actor` also allows handling synchronous requests asynchronously. This feature lets you delegate the processing of synchronous requests to other processes or delay the response. To do this, the `HandleCall` method should return `nil`, and you should use the `SendResponse` method of the `gen.Process` interface to send the response. You must provide the `gen.PID` of the requesting process and the `gen.Ref` reference of the request.

For debugging or monitoring the internal state, the `Inspect` method allows high-priority synchronous requests. In `act.Actor`, this triggers the `HandleInspect` method. This feature is widely used in the [Observer](../tools/observer.md) tool.

If the actor process is registered as a logger, the `HandleLog` method is called when log messages (`gen.MessageLog`) are received, generated using the `gen.Log` interface. If the actor process has subscribed to events using the `LinkEvent` or `MonitorEvent` methods of `gen.Process`, the `HandleEvent` method is called when an event message is received.

### Process Termination

To stop a process, simply return a non-nil `error` value from the message-handling callback. After termination, the `Terminate` callback method will be called, with the `reason` argument being the returned error value.

Additionally, you can terminate a process using predefined constants like:

* `gen.TerminateReasonNormal` for a normal (non-failure) termination.
* `gen.TerminateReasonKill` when the process is forcibly stopped using the `Kill` method from the `gen.Node` interface.

In Ergo Framework, two additional termination reasons are defined:

* **gen.TerminateReasonPanic**: Occurs when a panic happens during message processing.
* **gen.TerminateReasonShutdown**: This termination reason can be sent by the parent process or the node when the node is stopped using the `StopForce` method of the `gen.Node` interface. It is not considered a failure.

At the moment the `Terminate` callback is invoked, the process has already been removed from the node. Therefore, most methods of the `gen.Process` interface will return the `gen.ErrNotAllowed` error.

### SplitHandle

The `SplitHandle` option allows an actor to call the callback methods of the `act.ActorBehavior` interface based on the process identifier used to send a message or make a synchronous call to your process. You can enable this option using the `SetSplitHandle(split bool)` method defined in `act.Actor`. To check the current value of this option, use the `SplitHandle` method. Here is an example usage:

```go
func (a *MyActor) Init(args...) error {
    a.SetSplitHandle(true)
    ...
    return nil
}
```

If your process has a registered associated name and receives a message using that name, `act.Actor` will call the `HandleMessageName` callback. The `name` argument will be the value used to send the message. For synchronous requests, the `HandleCallName` callback will be invoked.

If a process alias (`gen.Alias`) was used, the callbacks `HandleMessageAlias` and `HandleCallAlias` will be triggered for messages and synchronous requests, respectively. This enables customized handling based on the way the message or request was sent.

### TrapExit - handling _exit-signal_

In Ergo Framework, an _exit signal_ can be sent to your process by another process or node using the `SendExit` method from the `gen.Process` or `gen.Node` interfaces. It can also be generated by the node if your process had a link created via the `Link*` methods in the `gen.Process` interface. By default, when an actor receives an _exit signal_, it terminates and calls the `Terminate` callback with the reason specified in the _exit signal_.

To prevent your actor from terminating upon receiving an _exit signal_, you can enable the option to intercept such signals. This can be done using the `SetTrapExit(trap bool)` method in `act.Actor`. The default value is `false`, but you can enable it at any time, for example, during initialization:

```go
func (a *MyActor) Init(args...) error {
    a.SetTrapExit(true)
    ...
    return nil
}
```

When the `TrapExit` option is enabled, `act.Actor` converts _exit signals_ into standard `gen.MessageExit*` messages:

* **gen.MessageExitPID**: The source of the _exit signal_ was a process.
* **gen.MessageExitProcessID**: The source was `gen.ProcessID` (a _link_ created using `LinkProcessID`).
* **gen.MessageExitAlias**: The source was `gen.Alias` (_link_ created using `LinkAlias`).
* **gen.MessageExitEvent**: The source was `gen.Event` (_link_ created using `LinkEvent`).
* **gen.MessageExitNode**: The source was a network connection with a node (_link_ created using `LinkNode`).

{% hint style="info" %}
Scenarios where the _exit signal_ **cannot be intercepted**:

* **Parent Process Sends Exit**: When the parent process sends an _exit signal_ via `SendExit` using the `gen.Process` interface, it cannot be intercepted.
* **Parent Process Terminates**: If the parent process, with which a _link_ was created via `gen.PID` (using `Link` or `LinkPID`), terminates, or if the parent process used the `gen.ProcessOptions.LinkParent` option during child process startup.

In these cases, the actor will terminate, and the `Terminate` callback will be called, even with `TrapExit` enabled. All other _exit signals_ can be intercepted when `TrapExit` is active.
{% endhint %}
