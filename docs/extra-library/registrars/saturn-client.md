# Saturn Сlient

This package implements the `gen.Registrar` interface and serves as a client library for the central registrar, [Saturn](../../tools/saturn.md). In addition to the primary _Service Discovery_ function, it automatically notifies all connected nodes about cluster configuration changes.

To create a client, use the `Create` function from the `saturn` package. The function requires:

* The hostname where the central registrar is running (default port: `4499`, unless specified in `saturn.Options`)
* A token for connecting to Saturn
* a set of options `saturn.Options`

Then, set this client in the `gen.NetworkOption.Registrar` options

```go
import (
     "ergo.services/ergo"
     "ergo.services/ergo/gen"

     "ergo.services/registrar/saturn"
)

func main() {
     var options gen.NodeOptions
     ...
     host := "localhost"
     token := "IwOBhgAEAGzPt"
     options.Network.Registrar = saturn.Create(host, token, saturn.Options{})
     ...
     node, err := ergo.StartNode("demo@localhost", options)
     ...
}
```

Using `saturn.Options`, you can specify:

* `Cluster` - The cluster name for your node
* `Port` - The port number for the central Saturn registrar
* `KeepAlive` - The keep-alive parameter for the TCP connection with Saturn
* `InsecureSkipVerify` - Option to ignore TLS certificate verification

When the node starts, it will register with the [Saturn](../../tools/saturn.md) central registrar in the specified cluster.&#x20;

Additionally, this library registers a `gen.Event` and generates messages based on events received from the central Saturn registrar within the specified cluster. This allows the node to stay informed of any updates or changes within the cluster, ensuring real-time event-driven communication and responsiveness to cluster configurations:

* `saturn.EventNodeJoined` - Triggered when another node is registered in the same cluster.
* `saturn.EventNodeLeft` - Triggered when a node disconnects from the central registrar
* `saturn.EventApplicationLoaded` - An application was loaded on a remote node. Use `ResolveApplication` from the `gen.Resolver` interface to get application details
* `saturn.EventApplicationStarted` - Triggered when an application starts on a remote node.
* `saturn.EventApplicationStopping` - Triggered when an application begins stopping on a remote node.
* `satrun.EventApplicationStopped` - Triggered when an application is stopped on a remote node.
* `saturn.EventApplicationUnloaded` - Triggered when an application is unloaded on a remote node&#x20;
* `saturn.EventConfigUpdate` - The node's configuration was updated&#x20;

To receive such messages, you need to subscribe to Saturn client events using the `LinkEvent` or `MonitorEvent` methods from the `gen.Process` interface. You can obtain the name of the registered event using the `Event` method from the `gen.Registrar` interface. This allows your node to listen for important cluster events like node joins, application starts, configuration updates, and more, ensuring real-time updates and handling of cluster changes. &#x20;

```go
type myActor struct {
    act.Actor
}

func (m *myActor) HandleMessage(from gen.PID, message any) error {
    reg, e := a.Node().Network().Registrar()
    if e != nil {
	a.Log().Error("unable to get Registrar interface %s", e)
	return nil
    }
    ev, e := reg.Event()
    if e != nil {
	a.Log().Error("Registrar has no registered Event: %s", e)
	return nil
    }
    
    a.MonitorEvent(ev)
    return nil
}

func (m *myActor) HandleEvent(event gen.MessageEvent) error {
    m.Log().Info("got event message: %v", event)
    return nil
}
```

Using the `saturn.EventApplication*` events and the [Remote Start Application](../../networking/remote-start-application.md) feature, you can dynamically manage the functionality of your cluster. The `saturn.EventConfigUpdate` events allow you to adjust the cluster configuration on the fly without restarting nodes, such as updating the cookie value for all nodes or refreshing the TLS certificate. Refer to the [Saturn - Central Registrar](../../tools/saturn.md) section for more details.&#x20;

You can also use the `Config` and `ConfigItem` methods from the `gen.Registrar` interface to retrieve configuration parameters from the registrar.&#x20;

To get information about available applications in the cluster, use the `ResolveApplication` method from the `gen.Resolver` interface, which returns a list of `gen.ApplicationRoute` structures:

```go
type ApplicationRoute struct {
	Node   Atom
	Name   Atom
	Weight int
	Mode   ApplicationMode
	State  ApplicationState
}
```

* `Name` The name of the application
* `Node` The name of the node where the application is loaded or running
* `Weight` The weight assigned to the application in `gen.ApplicationSpec`
* `Mode` The application's startup mode (`gen.ApplicationModeTemporary`, `gen.ApplicationModePermanent`, `gen.ApplicationModeTransient`)..&#x20;
* `State` The current state of the application (`gen.ApplicationStateLoaded`, `gen.ApplicationStateRunning`, `gen.ApplicationStateStopping`)&#x20;

You can access the `gen.Resolver` interface using the `Resolver` method from the `gen.Registrar` interface.
