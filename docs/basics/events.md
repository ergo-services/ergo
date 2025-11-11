# Events

The _events_ mechanism in Ergo Framework is built on top of the pub/sub subsystem. It allows any process to become an _event producer_, while other processes can subscribe to these _events_. This enables flexible event-driven architectures, where processes can publish and consume events across the system.

### Producer

To register an _event_ (`gen.Event`), you use the `RegisterEvent` method available in the `gen.Process` or `gen.Node`interface. When registering an _event_, you can configure the following parameters using the `gen.EventOptions`:

* **`Notify`**: This flag controls whether the producer should be notified about the presence or absence of subscribers for the event. If notifications are enabled, the process will receive a `gen.MessageEventStart` message when the first subscriber appears and a `gen.MessageEventStop` message when the last subscriber unsubscribes. This allows the producer to generate events only when there are active subscribers. If the _event_ is registered using the `RegisterEvent` method of the `gen.Node` interface, this field is ignored.
* **`Buffer`**: This specifies how many of the most recent _events_ should be stored in the event buffer. If this option is set to zero, buffering is disabled.

The `RegisterEvent` function returns a token of type `gen.Ref` upon success. This token is used to generate _events_. Only the process that owns the event, or a process that has been delegated the token by the _event_ owner, can produce _events_ for that registration.

To generate _events_, the `gen.Process` interface provides the `SendEvent` method. This method accepts the following arguments:

* **`name`**: The name of the registered _event_ (`gen.Atom`).
* **`token`**: The key obtained during the _event_ registration (`gen.Ref`).
* **`message`**: The _event_ payload, which can be of any type.

{% hint style="info" %}
It's important to note that the `RegisterEvent` method is not available to a process during its initialization state.
{% endhint %}

To generate _events_, the `gen.Node` interface provides the `SendEvent` method. This method is similar to the `SendEvent`method in the `gen.Process` interface but includes an additional parameter, `gen.MessageOptions`. This extra parameter allows for further customization of how the _event_ message is sent, such as setting priority, compression, or other message-related options.

### Consumer

To subscribe to a registered _event_, the `gen.Process` interface provides the following methods:

* **`LinkEvent`**: This method creates a _link_ to a `gen.Event`. The process will receive an _exit signal_ if the producer process of the _event_ terminates or if the _event_ is unregistered. To remove the _link_, use the `UnlinkEvent` method. When the _link_ is created, the method returns a list of the most recent events from the producer's buffer.
* **`MonitorEvent`**: This method creates a _monitor_ on a `gen.Event`. The process will receive a `gen.MessageDownEvent`if the producer process terminates or if the _event_ is unregistered. If the _event_ is unregistered, the `Reason` field in `gen.MessageDownEvent` will contain `gen.ErrUnregistered`. To remove the _monitor_, use the `DemonitorEvent`method.

Both methods accept an argument of type `gen.Event`. For an event registered on the local node, you only need to specify the `Name` field, and you can leave the `Node` field empty. This simplifies subscribing to local _events_ while still providing flexibility for handling events from remote nodes.

```go
type myActor {
    act.Actor
}
...
func (a *myActor) HandleMessage(from gen.PID, message any) error {
    ...
    // local event
    event := gen.Event{Name: "exampleEvent"}
    lastEvents, err := a.LinkEvent(event)
    ...
    // remote event
    event := gen.Event{Name: "remoteEvent", Node: "remoteNode"}
    lastEvents, err := a.LinkEvent(event)
}
```

Upon successfully creating a _link_ or _monitor_, the function returns a list of events (`gen.MessageEvent`) provided by the producer process from its message buffer. Each event contains the following information: the _event_ name (`gen.Event`), the timestamp of when the event was generated (obtained using `time.Now().UnixNano()`), and the actual _event_ value sent by the producer process. If the list of _events_ is empty, it means that either the producer process has not yet generated any _events_ or the producer registered the _event_ with a zero-sized buffer.

For an example demonstrating the capabilities of the _events_ mechanism, you can refer to the [events project](https://github.com/ergo-services/examples) in the `ergo-services/examples` repository:

<figure><img src="../.gitbook/assets/image (8).png" alt=""><figcaption></figcaption></figure>
