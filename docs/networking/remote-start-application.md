# Remote Start Application

In addition to the ability to spawn processes on remote nodes, Ergo Framework also allows you to start applications remotely. Similar to process spawning, access to this functionality is controlled by the `EnableRemoteApplicationStart`flag in `gen.NetworkFlags`. By default, this flag is enabled.

### Access Control

To allow an application to be started from a remote node, it must be registered in the network stack. The `gen.Network`interface provides the following methods for controlling this functionality:

```go
EnableApplicationStart(name gen.Atom, nodes ...gen.Atom) error
DisableApplicationStart(name gen.Atom, nodes ...gen.Atom) error
```

The `name` argument represents the application's name, and the `nodes` argument controls which nodes are permitted to start the application remotely. This gives you fine-grained control over which nodes can initiate the remote startup of specific applications.

Using the `EnableApplicationStart` method with the `nodes` argument creates an access list of remote nodes that are permitted to start the specified application.

When using the `DisableApplicationStart` method with the `nodes` argument, it removes the specified nodes from the access list. If the access list becomes empty after removing nodes, access is opened to all remote nodes. To fully disable access to starting the application, use the `DisableApplicationStart` method without the `nodes` argument.

### Start Application

To start an application on a remote node, use the `ApplicationStart` method of the `gen.RemoteNode` interface (which you can access via the `GetNode` or `Node` methods of the `gen.Network` interface). This interface also provides additional methods to control the application's [startup mode](../basics/application.md#application-startup-modes):

```go
ApplicationStartTemporary(name gen.Atom, options gen.ApplicationOptions) error
ApplicationStartTransient(name gen.Atom, options gen.ApplicationOptions) error
ApplicationStartPermanent(name gen.Atom, options gen.ApplicationOptions) error
```

When an application is started remotely, the `Parent` property of the application is assigned the name of the node that initiated the startup. You can retrieve information about the application and its status using the `ApplicationInfo` method of the `gen.Node` interface.&#x20;
