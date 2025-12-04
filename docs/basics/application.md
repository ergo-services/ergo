---
description: Grouping and Managing Actors as a Unit
---

# Application

An application groups related actors and manages them as a unit. Instead of starting individual processes and tracking their lifecycles manually, you define an application that specifies which actors to start, in what order, and how the group should behave if individual actors fail.

Think of an application as a recipe. It lists the components (actors and supervisors), describes their startup order, and specifies the rules for what happens when things go wrong. The node follows this recipe when starting the application and monitors the running components according to the specified mode.

## The Need for Applications

Starting processes one at a time works for simple systems. But as complexity grows, you face coordination problems. Which processes should start first? What if one fails to start - do you continue or abort? If a critical component terminates, should the service keep running in a degraded state or shut down cleanly?

These aren't implementation details - they're architectural decisions about your service's structure and fault tolerance policy. Applications let you declare these decisions explicitly rather than scattering the logic throughout your code. The specification documents what your service consists of. The mode declares your termination policy. The framework enforces both.

## Defining an Application

Applications implement the `gen.ApplicationBehavior` interface:

```go
type ApplicationBehavior interface {
    Load(node Node, args ...any) (ApplicationSpec, error)
    Start(mode ApplicationMode)
    Terminate(reason error)
}
```

The `Load` callback returns the application specification - what this application consists of and how it should behave. The `Start` callback runs after all processes start successfully. The `Terminate` callback runs when the application stops.

A typical application specification:

```go
func (a *MyApp) Load(node gen.Node, args ...any) (gen.ApplicationSpec, error) {
    return gen.ApplicationSpec{
        Name: "myapp",
        Group: []gen.ApplicationMemberSpec{
            {Name: "worker", Factory: createWorker},
            {Factory: createSupervisor},
        },
        Mode: gen.ApplicationModeTransient,
    }, nil
}
```

The `Group` lists processes to start. Processes start in the order listed. If a process has a `Name`, it's registered with that name, making it discoverable. Processes without names are anonymous.

Application names and process names exist in separate namespaces. An application named "api" and a process named "api" do not conflict - you can have both registered simultaneously. However, using the same name for both creates confusion when reading code or debugging. Avoid identical names even though the framework allows it.

## Application Modes

The mode determines what happens when a process in the application terminates.

**Temporary Mode** - The application continues running despite individual process terminations. Only when all processes have stopped does the application itself terminate. This mode is for applications where components can fail and restart independently (typically via supervisors) without stopping the whole application.

**Transient Mode** - The application stops if any process terminates abnormally (crashes, panics, errors). Normal termination doesn't trigger shutdown. When an abnormal termination occurs, all remaining processes receive exit signals and the application shuts down. Use this mode when abnormal failures indicate a systemic problem that requires stopping the entire service.

**Permanent Mode** - The application stops if any process terminates, regardless of reason. Even normal termination of one process triggers shutdown of all others and the application itself. This mode is for applications where all components must run together - if one stops, the whole application is incomplete.

## Loading and Starting

Applications go through two phases: loading and starting.

Loading calls your `Load` callback, validates the specification, and registers the application with the node. The application is loaded but not running. This separation allows you to load multiple applications and resolve dependencies before starting any of them.

Starting launches the processes in the `Group` according to their order. If dependencies are specified in `ApplicationSpec.Depends`, the node ensures those applications are running first. If any process fails to start (including initialization timeout), previously started processes are killed and the application fails to start.

Application processes have a maximum `InitTimeout` of 15 seconds (3x DefaultRequestTimeout). Setting a higher value in `gen.ProcessOptions` returns `gen.ErrNotAllowed` and prevents the application from starting.

Once all processes are running, the `Start` callback is called and the application enters the running state.

## Dependencies

Applications can depend on other applications or network services. If application B depends on application A, the node ensures A is running before starting B. Dependencies are declared in `ApplicationSpec.Depends`.

This allows you to structure complex systems with clear startup ordering. A database connection pool application starts before the API server application. The API server starts before the web frontend application. The framework handles the ordering automatically.

## Stopping Applications

Applications stop in three ways.

You can call `ApplicationStop`, which sends exit signals to all processes and waits for them to terminate gracefully (5 second timeout by default). Once all processes have stopped, the `Terminate` callback runs and the application transitions to the loaded state.

You can call `ApplicationStopForce`, which kills all processes immediately without waiting. Less graceful, but guaranteed to stop quickly.

The application can stop itself based on its mode. In Transient or Permanent mode, process failures trigger automatic shutdown according to the mode's rules.

## Environment and Configuration

Applications have environment variables that all their processes inherit. These override node-level variables but are overridden by process-specific variables. This creates a natural layering: node provides defaults, application provides service-specific values, processes can override for their specific needs.

## Tags for Instance Selection

Running multiple instances of the same application across a cluster creates a selection problem. Which instance should handle the request? In blue/green deployments, you run two versions and route traffic based on readiness. Canary deployments send a percentage to the new version. Some instances enter maintenance mode while others serve production traffic.

Tags provide metadata for making these decisions. Label each application instance with tags describing its deployment state, version, or role:

```go
func (a *MyApp) Load(node gen.Node, args ...any) (gen.ApplicationSpec, error) {
    return gen.ApplicationSpec{
        Name: "api_service",
        Tags: []gen.Atom{"blue", "v2.1.0"},
        // ... rest of spec
    }, nil
}
```

Tags are always available through `node.ApplicationInfo()` or `remoteNode.ApplicationInfo()`. For clusters using centralized registrars (etcd, Saturn), tags are also published during application route registration. This enables cluster-wide discovery: query the registrar and receive all application instances with their tags.

The embedded in-memory registrar does not support application route registration, so tags in single-node or statically-routed deployments are only accessible via direct `ApplicationInfo()` calls, not through resolver queries.

In clusters with centralized registrars:

```go
// Query registrar for all instances
routes, err := resolver.ResolveApplication("api_service")
// Returns []ApplicationRoute, each with Node, Tags, Weight, State

// Filter by tag
for _, route := range routes {
    hasBlue := false
    for _, tag := range route.Tags {
        if tag == "blue" {
            hasBlue = true
            break
        }
    }
    if hasBlue {
        remoteNode, _ := network.GetNode(route.Node)
        info, _ := remoteNode.ApplicationInfo("api_service")
        // Use this instance
    }
}
```

Common tag patterns:
- **Blue/green deployment**: "blue", "green"
- **Canary rollout**: "canary", "stable"
- **Maintenance state**: "maintenance", "active", "draining"
- **Version tracking**: "v1.0.0", "v2.0.0"
- **Geographic region**: "us-east", "eu-west"

Tags separate deployment strategy from application code. Your application doesn't know it's the "blue" deployment - that's configuration. The routing logic queries tags and makes decisions based on current cluster state.

## Process Role Mapping

Applications contain multiple processes with specific responsibilities. An API server handles requests. A connection pool manages database connections. A cache manager stores frequently accessed data. These are logical roles, but the actual process names might be versioned, generated, or environment-specific.

The `Map` field bridges this gap. Define a mapping from logical role (string) to actual process name (Atom):

```go
func (a *MyApp) Load(node gen.Node, args ...any) (gen.ApplicationSpec, error) {
    return gen.ApplicationSpec{
        Name: "backend",
        Map: map[string]gen.Atom{
            "api":   "api_server_v2",
            "db":    "postgres_pool",
            "cache": "redis_manager",
        },
        Group: []gen.ApplicationMemberSpec{
            {Name: "api_server_v2", Factory: createAPI},
            {Name: "postgres_pool", Factory: createDB},
            {Name: "redis_manager", Factory: createCache},
        },
    }, nil
}
```

To communicate with a process by role, get the application info, look up the role in the map, then use the returned name:

```go
// Query application info (works locally or remotely)
info, err := node.ApplicationInfo("backend")
// or: info, err := remoteNode.ApplicationInfo("backend")

// Find process name by role
apiName, found := info.Map["api"]
if found {
    // Use the actual process name to communicate
    response, err := node.Call(apiName, APIRequest{})
}
```

This works for both local and remote applications. When querying a remote application, `RemoteNode.ApplicationInfo()` retrieves the map from the remote node, letting you discover process names without prior knowledge of the remote application's internal structure.

Why use mapping:
- **Version changes**: Update "api_server_v2" to "api_server_v3" without changing client code
- **Implementation swaps**: Map "db" to different pool implementations based on deployment
- **Remote discovery**: Remote nodes query the map to find process names in foreign applications
- **Stable interface**: Clients depend on roles ("api", "db"), not implementation details

The map provides a service contract. External code knows the application has an "api" role and a "db" role. The actual implementations can change as long as the roles remain consistent.

## The Application Pattern

Applications provide structure to your actor system. Instead of scattered process creation throughout your code, applications centralize the "what runs in this service" question. The specification documents your system's structure. The mode declares your fault tolerance policy. The dependency mechanism ensures correct startup ordering.

This organization becomes especially valuable in distributed systems where services start on different nodes. An application can be started remotely on another node, bringing all its components with the correct configuration and dependencies.

For more details on application lifecycle and options, refer to the `gen.ApplicationBehavior` and `gen.ApplicationSpec` documentation in the code.
