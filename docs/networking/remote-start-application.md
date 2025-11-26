---
description: Starting applications on remote nodes
---

# Remote Start Application

Remote application starting means launching an application on another node from your code. The remote node has the application loaded but not running. You send a start request, and the application starts on that node with the mode and options you specify. The application runs under the remote node's supervision, part of the remote node's application tree.

This capability enables dynamic application deployment and orchestration. You have a cluster of nodes, each with applications loaded but waiting. A coordinator node decides which applications should run where, based on load, topology, or scheduling logic. Remote application starting makes this coordination explicit and controllable.

Like remote spawning, remote application starting isn't automatic. Security matters. You don't want arbitrary nodes starting arbitrary applications. The framework requires explicit permission - the remote node must enable each application individually and can restrict which nodes are allowed to start it.

## Security Model

Remote application starting is disabled by default at the framework level. To enable it, set the `EnableRemoteApplicationStart` flag in your node's network configuration:

```go
node, err := ergo.StartNode("worker@localhost", gen.NodeOptions{
    Network: gen.NetworkOptions{
        Flags: gen.NetworkFlags{
            Enable:                       true,
            EnableRemoteApplicationStart: true,  // allow remote nodes to start apps
        },
    },
})
```

This flag is a global switch. With it disabled, all remote application start requests fail immediately with `gen.ErrNotAllowed`. With it enabled, requests proceed to per-application permission.

### Enabling Applications

Even with `EnableRemoteApplicationStart` turned on, remote nodes can't start anything until you explicitly enable specific applications:

```go
network := node.Network()

err := network.EnableApplicationStart("workers")
if err != nil {
    // handle error
}
```

Now remote nodes can request starting the `"workers"` application. The application must be loaded on this node (via `node.ApplicationLoad`). If it's not loaded, remote start requests fail with `gen.ErrApplicationUnknown`. If it's already running, remote start requests fail because you can't start a running application again.

The application name is the permission token. Remote nodes must use this exact name when requesting starts. If they request `"workers"` and you haven't enabled it, the request fails. If they request `"admin_app"` without permission, it fails. You control what's startable remotely.

### Access Control Lists

By default, `EnableApplicationStart` allows all nodes to start the application. But you can restrict it to specific nodes:

```go
// Allow only these nodes to start the workers app
network.EnableApplicationStart("workers",
    "scheduler@node1",
    "scheduler@node2",
)
```

Now only those two nodes can start the workers application. Requests from other nodes fail with `gen.ErrNotAllowed`.

You can update the access list dynamically:

```go
// Add more nodes to the allowed list
network.EnableApplicationStart("workers",
    "scheduler@node1",
    "scheduler@node2",
    "scheduler@node3",  // newly allowed
)
```

Calling `EnableApplicationStart` again with the same application name updates the access list.

### Disabling Access

To remove nodes from the access list:

```go
// Remove specific nodes
network.DisableApplicationStart("workers", "scheduler@node2")
```

This removes `scheduler@node2` from the allowed list. Other nodes in the list remain allowed.

To completely disable remote starting for an application:

```go
// No nodes can start this application remotely anymore
network.DisableApplicationStart("workers")
```

Without any node arguments, `DisableApplicationStart` removes the permission entirely. All future start requests for that application fail.

To re-enable with an open access list (any node can start):

```go
// Re-enable for all nodes
network.EnableApplicationStart("workers")  // no node arguments
```

This is the explicit "allow all nodes" configuration.

## Starting Applications on Remote Nodes

To start an application on a remote node, first get a `gen.RemoteNode` interface:

```go
network := node.Network()
remote, err := network.GetNode("worker@otherhost")
if err != nil {
    return err  // node unreachable, no route, etc
}
```

With the remote node handle, start an application:

```go
err := remote.ApplicationStart("workers", gen.ApplicationOptions{})
if err != nil {
    // handle error - not allowed, app not loaded, already running, etc
}
```

The application starts on the remote node. The start is synchronous - the call blocks until the remote node confirms the application started or returns an error.

### Application Startup Modes

Applications have three startup modes: Temporary, Transient, and Permanent. These modes control restart behavior when the application terminates. For remote starts, you can specify the mode explicitly:

```go
// Start as temporary (not restarted if it terminates)
err := remote.ApplicationStartTemporary("workers", gen.ApplicationOptions{})

// Start as transient (restarted only if it terminates abnormally)
err := remote.ApplicationStartTransient("workers", gen.ApplicationOptions{})

// Start as permanent (always restarted if it terminates)
err := remote.ApplicationStartPermanent("workers", gen.ApplicationOptions{})
```

If you use `ApplicationStart` without specifying a mode, the application starts with the mode it was loaded with (set during `ApplicationLoad`).

The mode affects how the remote node's application supervisor handles termination. If the application crashes, does it restart automatically? The mode determines this. Choose based on your operational requirements - critical services should be Permanent, optional services can be Temporary, and services that should restart only on failure can be Transient.

For details on application modes, see [Application Startup Modes](../basics/application.md#application-startup-modes).

## Application Parent and Process Hierarchy

When an application starts remotely, parent tracking is set at multiple levels:

**Application Parent**: Set to the requesting node name:

```go
// On the remote node
info, err := node.ApplicationInfo("workers")
// info.Parent == "scheduler@node1" (requesting node name)
```

**Process Parent for Group Members**: Processes started directly by the application (listed in `Group`) receive the requesting node's core PID as their parent:

```go
processInfo, err := node.ProcessInfo(workerPID)
// processInfo.Parent == <scheduler@node1.0.1> (core PID of requesting node)
```

**Process Parent for Descendants**: If those processes spawn children, the children receive their spawning process PID as parent (normal process hierarchy):

```go
// Worker spawns a child process
childPID, _ := worker.Spawn(factory, options)
childInfo, _ := node.ProcessInfo(childPID)
// childInfo.Parent == workerPID (not the remote core PID)
```

Only the first-level processes (application group members) have the cross-node parent relationship. Subsequent generations follow standard process parent-child relationships within the local node.

This parent information is for tracking and auditing, not supervision. The application is supervised by the local application supervisor on the remote node. Terminating the requesting node does not affect the running application.

## Environment Variable Inheritance

By default, remote applications don't inherit environment variables from the requesting node. To enable environment inheritance:

```go
node, err := ergo.StartNode("scheduler@localhost", gen.NodeOptions{
    Security: gen.SecurityOptions{
        ExposeEnvRemoteApplicationStart: true,  // allow env inheritance for remote app starts
    },
})
```

Now when you start an application remotely, the application's processes receive a copy of the requesting node's core environment. This enables configuration propagation - your scheduler node has configuration in its environment, and applications started remotely inherit it.

**Important:** Environment variable values must be EDF-serializable. Strings, numbers, booleans work fine. Custom types require registration via `edf.RegisterTypeOf`. If an environment variable contains a non-serializable value (e.g., a channel, function, or unregistered struct), the remote application start fails entirely with an error like `"no encoder for type <type>"`. The framework doesn't skip problematic variables - any non-serializable value causes the entire start request to fail.

## How It Works

When you call `remote.ApplicationStart`:

1. **Check capabilities** - The local node checks if the remote node's `EnableRemoteApplicationStart` flag is true (learned during handshake). If false, fail immediately.

2. **Create start message** - Package the application name, startup mode, and options into a `MessageApplicationStart` protocol message. Include a reference for tracking the response.

3. **Send request** - Encode and send the message to the remote node. Wait for a response (this is synchronous - remote application start blocks until the remote node replies).

4. **Remote processing** - The remote node receives the message, checks if the application is enabled for remote start, checks if the requesting node is allowed, verifies the application exists and isn't already running, calls the application's start logic with the given mode.

5. **Response** - The remote node sends back a `MessageResult` containing either success or an error. The local node receives this, resolves the waiting request, and returns the result to the caller.

If anything fails (application not found, access denied, already running, remote node terminating), the error is returned to the caller. The entire operation is synchronous - you call `ApplicationStart` and block until the application is running or an error occurs.

## Practical Considerations

**Idempotency** - Starting an already-running application returns an error. If you're unsure of the application's state, query it first using `remote.ApplicationInfo` to check if it's already running. Or handle the error gracefully and treat "already running" as success.

**Startup time** - Some applications take time to start - they might load configuration, establish connections, initialize state. The remote start call blocks during this entire startup sequence. If startup is slow, the caller waits. For long-running startup logic, consider using async patterns or monitoring application state separately.

**Failure modes** - Remote application start can fail in ways local start can't. The network connection can drop mid-request. The remote node can crash before responding. The application might fail to start for reasons specific to that node (missing dependencies, configuration issues). Handle errors explicitly.

**Resource contention** - An application starting on a remote node consumes that node's resources (CPU, memory, file descriptors). If multiple nodes simultaneously request starting applications on the same remote node, it could become resource-constrained. Coordinate start requests to avoid overwhelming nodes.

**Application lifecycle** - Once started remotely, the application runs until explicitly stopped or until the remote node terminates. The requesting node has no automatic control over the running application. If you want to stop it later, you need to send another request (the framework doesn't currently support remote application stop, but you can implement custom coordination via messages).

**Supervision independence** - The application is supervised by the remote node, not by the requesting node. If the requesting node crashes, the application keeps running. If the remote node crashes, the application terminates. This independence is important for operational reasoning - the application's lifecycle is tied to where it runs, not to who started it.

**Configuration management** - Applications often need configuration. With `ExposeEnvRemoteApplicationStart`, you can propagate environment variables. But this creates coupling - the application depends on the requesting node's configuration. Consider whether configuration should come from the remote node's local environment, from a centralized configuration service, or from the requesting node. The right answer depends on your architecture.

## When to Use Remote Application Start

**Dynamic orchestration** - A coordinator node decides which applications should run on which nodes based on cluster state, resource availability, or scheduling logic. The coordinator starts applications dynamically as needed.

**Staged deployment** - Applications are pre-loaded on nodes but not started. A deployment controller starts them in a specific order, waiting for health checks between stages. This enables controlled rollouts.

**Capacity management** - Some applications run only during high-load periods. A resource manager monitors load and starts applications on additional nodes when needed, then stops them when load decreases.

**Geographic distribution** - Applications are loaded across multiple regions. A traffic manager starts applications in specific regions based on user distribution, latency requirements, or failover needs.

**Testing and validation** - Test frameworks load applications on test nodes but don't start them until test execution. Tests start applications with specific configurations, run scenarios, then stop them. This enables repeatable, isolated testing.

**Maintenance windows** - During maintenance, you stop applications on a node, perform updates, then start them again. Remote start enables coordinated maintenance across a cluster without manually SSHing to each node.

Remote application starting is about control and coordination. If your cluster has static application deployment (applications always run on specific nodes), you don't need this feature - use supervision trees and let supervisors start applications automatically. If your cluster has dynamic application deployment (applications move between nodes based on conditions), remote application starting enables that flexibility.

For understanding the underlying network mechanics, see [Network Stack](network-stack.md). For controlling connections to remote nodes, see [Static Routes](static-routes.md). For understanding application lifecycle and modes, see [Application](../basics/application.md).
