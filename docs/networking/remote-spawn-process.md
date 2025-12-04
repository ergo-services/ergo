---
description: Spawning processes on remote nodes
---

# Remote Spawn Process

Remote spawning means starting a process on another node from your code. You call a method, provide a factory name and options, and a process starts on the remote node. From the caller's perspective, it's nearly identical to spawning locally - you get back a `gen.PID` and can communicate with it immediately.

This capability enables dynamic workload distribution. Your node needs to process a job but doesn't have capacity? Spawn a worker on a remote node with available resources. Your application needs to scale horizontally? Spawn processes across multiple nodes and distribute load. Remote spawning makes the cluster feel like one large computing resource rather than isolated nodes.

But remote spawning isn't automatic. Security matters. You don't want arbitrary nodes spawning arbitrary processes on your infrastructure. The framework requires explicit permission - the remote node must enable each process factory individually and can restrict which nodes are allowed to use it.

## Security Model

Remote spawning is disabled by default at the framework level. To enable it, set the `EnableRemoteSpawn` flag in your node's network configuration:

```go
node, err := ergo.StartNode("worker@localhost", gen.NodeOptions{
    Network: gen.NetworkOptions{
        Flags: gen.NetworkFlags{
            Enable:            true,
            EnableRemoteSpawn: true,  // allow remote nodes to spawn processes
        },
    },
})
```

This flag is a global switch. With it disabled, all remote spawn requests fail immediately with `gen.ErrNotAllowed`. With it enabled, requests proceed to the next level of security: per-factory permission.

### Enabling Process Factories

Even with `EnableRemoteSpawn` turned on, remote nodes can't spawn anything until you explicitly enable specific process factories:

```go
network := node.Network()

err := network.EnableSpawn("worker", createWorker)
if err != nil {
    // handle error
}
```

Now remote nodes can request spawning using the factory name `"worker"`. The factory function `createWorker` returns a `gen.ProcessBehavior`, just like local spawning. When a remote spawn request arrives with name `"worker"`, the framework calls `createWorker()` to instantiate the process.

The factory name is the permission token. Remote nodes must use this exact name when requesting spawns. If they request `"worker"` and you haven't enabled it, the request fails. If they request `"admin_process"` without permission, it fails. You control the namespace of what's spawnable.

### Access Control Lists

By default, `EnableSpawn` allows all nodes to use the factory. But you can restrict it to specific nodes:

```go
// Allow only these nodes to spawn workers
network.EnableSpawn("worker", createWorker, 
    "scheduler@node1", 
    "scheduler@node2",
)
```

Now only those two nodes can spawn workers. Requests from other nodes fail with `gen.ErrNotAllowed`.

You can update the access list dynamically:

```go
// Add more nodes to the allowed list
network.EnableSpawn("worker", createWorker,
    "scheduler@node1",
    "scheduler@node2", 
    "scheduler@node3",  // newly allowed
)
```

Calling `EnableSpawn` again with the same factory name updates the access list. The factory must be the same (same type) - you can't change which factory is associated with a name after the first `EnableSpawn` call. Attempting to do so returns an error.

### Disabling Access

To remove nodes from the access list:

```go
// Remove specific nodes
network.DisableSpawn("worker", "scheduler@node2")
```

This removes `scheduler@node2` from the allowed list. Other nodes in the list remain allowed.

To completely disable a factory:

```go
// No nodes can spawn workers anymore
network.DisableSpawn("worker")
```

Without any node arguments, `DisableSpawn` removes the factory entirely. All future spawn requests for that name fail.

To re-enable the factory with an open access list (any node can spawn):

```go
// Re-enable for all nodes
network.EnableSpawn("worker", createWorker)  // no node arguments
```

This is the explicit "allow all nodes" configuration.

## Spawning on Remote Nodes

To spawn a process on a remote node, first get a `gen.RemoteNode` interface:

```go
network := node.Network()
remote, err := network.GetNode("worker@otherhost")
if err != nil {
    return err  // node unreachable, no route, etc
}
```

`GetNode` establishes a connection if needed. If a connection already exists, it returns immediately. If discovery or connection fails, you get an error.

With the remote node handle, spawn a process:

```go
pid, err := remote.Spawn("worker", gen.ProcessOptions{})
if err != nil {
    // handle error - not allowed, factory not found, remote node terminated, etc
}

// pid is the process running on the remote node
process.Send(pid, WorkRequest{Job: "process-data"})
```

The `gen.ProcessOptions` are the same as local spawning: mailbox size, compression settings, parent process options. The remote node respects these options when creating the process.

### Spawn with Arguments

You can pass initialization arguments to the remote process:

```go
pid, err := remote.Spawn("worker", gen.ProcessOptions{}, 
    ConfigData{WorkerID: 42, BatchSize: 100},
)
```

These arguments are passed to the factory's `Init` callback, just like local spawning. The arguments must be serializable via EDF - primitives, registered structs, framework types. Complex arguments require type registration on both sides.

### Spawn with Registration

To spawn and register the process with a name:

```go
pid, err := remote.SpawnRegister("worker-001", "worker", gen.ProcessOptions{})
```

The first argument is the registration name. The remote process is registered under that name on the remote node, allowing other processes on that node (or other nodes) to find it via `gen.ProcessID{Name: "worker-001", Node: "worker@otherhost"}`.

## Spawning from Processes

The `gen.Process` interface provides methods for remote spawning from within a process:

```go
pid, err := process.RemoteSpawn("worker@otherhost", "worker", gen.ProcessOptions{})
```

This differs from using `RemoteNode.Spawn` in a subtle but important way: the spawned process inherits properties from the calling process, not from the node.

**Inherited properties:**
- Application name - if the caller is part of an application, the remote process becomes part of that application too
- Logging level - the remote process uses the same log level as the caller
- Environment variables - if `ExposeEnvRemoteSpawn` security flag is enabled, the remote process gets a copy of the caller's environment

This inheritance enables application-level distribution. If your application spawns processes remotely using `process.RemoteSpawn`, those processes belong to your application's supervision tree (conceptually), inherit your configuration, and operate as extensions of your application rather than independent processes.

```go
pid, err := process.RemoteSpawnRegister(
    "worker@otherhost",
    "worker", 
    "worker-001",  // registration name
    gen.ProcessOptions{},
)
```

The same inheritance applies.

### Parent Relationship and Inheritance

Remote spawn behavior differs based on whether you spawn from a process or from the node:

**From a process (`process.RemoteSpawn`)**:

The spawned process inherits attributes from the calling process:

- **Parent PID**: Set to the calling process's PID
- **Group Leader**: Set to the calling process's group leader
- **Application**: Set to the calling process's application name (if caller belongs to an application)
- **Log Level**: Inherits the calling process's log level
- **Environment**: Inherits the calling process's environment (if `SecurityOptions.ExposeEnvRemoteSpawn` is enabled)

The remote process can send messages to its parent using `process.Parent()`. If `LinkChild: true` is set in options, the link is established after spawn. However, the parent is on a different node - if the network connection drops, the remote process receives an exit signal for the lost parent and may terminate if linked.

**From the node (`RemoteNode.Spawn`)**:

The spawned process receives attributes from the requesting node's core:

- **Parent PID**: Set to the requesting node's core PID
- **Group Leader**: Set to the requesting node's core PID
- **Application**: Not set (empty - process doesn't belong to any application)
- **Log Level**: Inherits the requesting node's default log level
- **Environment**: Inherits the requesting node's environment (if `SecurityOptions.ExposeEnvRemoteSpawn` is enabled)

This creates independent processes without application affiliation. Use this for standalone remote workers that don't need to be part of an application's logical structure.

## Environment Variable Inheritance

By default, remote processes don't inherit environment variables. This is a security decision - you probably don't want to expose your node's configuration to remote processes.

To enable environment inheritance:

```go
node, err := ergo.StartNode("myapp@localhost", gen.NodeOptions{
    Security: gen.SecurityOptions{
        ExposeEnvRemoteSpawn: true,  // allow env inheritance for remote spawn
    },
})
```

Now when you use `process.RemoteSpawn`, the remote process receives a copy of the calling process's environment. The remote node reads these values and sets them on the spawned process.

**Important:** Environment variable values must be EDF-serializable. Strings, numbers, booleans work fine. Custom types require registration via `edf.RegisterTypeOf`. If an environment variable contains a non-serializable value (e.g., a channel, function, or unregistered struct), the remote spawn fails entirely with an error like `"no encoder for type <type>"`. The framework doesn't skip problematic variables - any non-serializable value causes the entire spawn request to fail.

Environment inheritance only works with `process.RemoteSpawn`. Using `RemoteNode.Spawn` doesn't inherit environment because there's no calling process - it's a node-level operation.

## How It Works

When you call `remote.Spawn`:

1. **Check capabilities** - The local node checks if the remote node's `EnableRemoteSpawn` flag is true (learned during handshake). If false, fail immediately.

2. **Create spawn message** - Package the factory name, process options, and arguments into a `MessageSpawn` protocol message. Include a reference for tracking the response.

3. **Send request** - Encode and send the message to the remote node. Wait for a response (this is synchronous - remote spawning blocks until the remote node replies).

4. **Remote processing** - The remote node receives the message, checks if the factory is enabled, checks if the requesting node is allowed, calls the factory function, spawns the process with the given options.

5. **Response** - The remote node sends back a `MessageResult` containing either the spawned PID or an error. The local node receives this, resolves the waiting request, and returns the PID to the caller.

If anything fails (factory not found, access denied, remote node terminating, initialization timeout), the error is returned to the caller. The entire operation is synchronous from the caller's perspective - you call `Spawn` and block until the process is created or an error occurs.

## Practical Considerations

**Performance** - Remote spawning is slower than local spawning. There's network latency, message encoding, and a synchronous request-response roundtrip. If you're spawning hundreds of processes, doing it remotely will be noticeably slower. Consider spawning a pool locally and distributing work via messages rather than spawning on-demand remotely.

**Timeouts** - Remote spawn has a maximum `InitTimeout` of 15 seconds (3x DefaultRequestTimeout). If the remote process's `ProcessInit` takes longer, spawn fails with `gen.ErrTimeout`. Setting `InitTimeout` higher than 15 seconds returns `gen.ErrNotAllowed` immediately without attempting the spawn.

**Failure modes** - Remote spawn can fail in ways local spawn can't. The network connection can drop mid-request. The remote node can crash before responding. The factory might exist but lack permission. Handle errors explicitly and have fallback strategies (retry, spawn locally, defer the work).

**Resource ownership** - A process spawned on a remote node runs on that node's resources (CPU, memory). It's part of that node's process table. If the remote node terminates, the process dies. If you're distributing workload, be aware of which node owns which processes.

**Linking** - Both `LinkChild` and `LinkParent` options work for remote spawn. The link is established after the remote process is created. If the network connection drops, linked processes receive exit signals for the lost peer.

**Application membership** - Processes spawned via `RemoteNode.Spawn` don't belong to any application. Processes spawned via `process.RemoteSpawn` inherit the caller's application. This affects supervision, lifecycle, and monitoring.

**Registration names** - Use `SpawnRegister` carefully. The name you provide is registered on the remote node. If that name is already taken, spawn fails. Ensure your naming strategy avoids conflicts, especially if multiple nodes are spawning on the same target.

## When to Use Remote Spawn

**Dynamic scaling** - Your application detects high load and spawns additional workers on remote nodes to handle the burst. When load decreases, workers terminate naturally and resources are freed.

**Specialized hardware** - Some nodes have GPUs, fast storage, or special network access. Spawn processes on those nodes when you need their capabilities, rather than sending data back and forth.

**Fault isolation** - Spawn risky operations on remote nodes. If they crash or consume excessive resources, they don't affect your local node's stability.

**Data locality** - If data lives on a specific node (in memory, on local disk), spawn processing near the data rather than transferring it across the network.

**Heterogeneous clusters** - Different nodes run different process types. Scheduler nodes spawn job processors on worker nodes. API nodes spawn request handlers on computation nodes. Remote spawning enables this separation.

Remote spawning isn't always the right answer. For static topologies where processes have fixed homes, use supervision trees and let supervisors spawn locally. For message-passing workloads where spawning overhead matters, use process pools and distribute work via messages. Remote spawning shines when you need dynamic, on-demand process creation across a cluster.

For understanding the underlying network mechanics, see [Network Stack](network-stack.md). For controlling connections to remote nodes, see [Static Routes](static-routes.md).
