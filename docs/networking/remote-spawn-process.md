# Remote Spawn Process

The network stack in Ergo Framework includes the capability to spawn processes on remote nodes. Nodes can control access to this feature or disable it entirely by using the `EnableRemoteSpawn` flag in `gen.NetworkFlags`. By default, this feature is enabled.

### Access Control

To manage the ability of remote nodes to spawn processes, the `gen.Network` interface provides two methods:

```go
EnableSpawn(name gen.Atom, factory gen.ProcessFactory, nodes ...gen.Atom) error
DisableSpawn(name gen.Atom, nodes ...gen.Atom) error
```

The `name` argument specifies the name under which the process factory will be registered in the network stack. Remote nodes must use this name when requesting to spawn a process.

Using `EnableSpawn` with the `nodes` argument creates an access list, allowing only the specified remote nodes to spawn processes with the registered factory. Conversely, using `DisableSpawn` with the `nodes` argument removes the specified nodes from the access list. If the list becomes empty, access is open to all nodes. To fully disable access to the process factory, use `DisableSpawn` without the `nodes` argument.

### Spawn&#x20;

To spawn a process on a remote node, you need to use the `gen.RemoteNode` interface, which can be obtained using the `GetNode` or `Node` methods of the `gen.Network` interface. The `gen.RemoteNode` interface provides two methods:

```go
Spawn(name gen.Atom, options gen.ProcessOptions, args ...any) (gen.PID, error)
SpawnRegister(register gen.Atom, name gen.Atom, options gen.ProcessOptions, args ...any) (PID, error)
```

For the `name` argument, you must use the name under which the process factory is registered on the remote node.

Just like with local process spawning, the process started on the remote node will inherit certain parameters from its parent. In this case, the parent is the virtual identifier of the node that sent the spawn request. The process will also inherit the logging level.

To inherit environment variables, you need to enable the `ExposeEnvRemoteSpawn` option in `gen.NodeOptions.Security`when starting your node. The environment variable values must be encodable and decodable using the EDF format. If you are using a custom type as the value of an environment variable, that type must be registered (see the [Network Transparency](network-transparency.md) section).

Upon success, these methods return the process identifier (`gen.PID`) of the process that was successfully started on the remote node.

You can also spawn a process on a remote node using the methods provided by the `gen.Process` interface:

```go
RemoteSpawn(node gen.Atom, name gen.Atom, options gen.ProcessOptions, args ...any) (gen.PID, error)
RemoteSpawnRegister(node gen.Atom, name gen.Atom, register gen.Atom, options gen.ProcessOptions, args ...any) (gen.PID, error)
```

When using these methods, the process started on the remote node will inherit parameters from the parent process, including the _application name_, _logging level_, and _environment variables_ (if the `ExposeEnvRemoteSpawn` flag in `gen.NodeOptions.Security` was enabled).

Note that linking options in `gen.ProcessOptions` will be ignored when spawning processes remotely. &#x20;
