---
description: Grouping and Managing Actors as a Unit
---

# Application

An application groups related actors and manages them as a unit. Instead of starting individual processes and tracking their lifecycles manually, you define an application that specifies which actors to start, in what order, and how the group should behave if individual actors fail.

Think of an application as a recipe. It lists the components (actors and supervisors), describes their startup order, and specifies the rules for what happens when things go wrong. The node follows this recipe when starting the application and monitors the running components according to the specified mode.

{% hint style="info" %}
**Migrating to 3.3.x.** The `gen.ApplicationBehavior` interface changed in this release. To adapt an existing application:

- Embed `app.Application` in your application type. The embed provides framework plumbing plus no-op defaults for the new lifecycle callbacks.
- Drop the `node gen.Node` parameter from `Load`. If you used it inside `Load`, switch to `a.Node()`.
- `Start` now takes an extra `ref gen.Ref` parameter: `Start(ref gen.Ref, mode gen.ApplicationMode)`. If your old `Start` was an empty stub, delete it; the embed default handles it. If it had real logic, rewrite with the new signature.
- `Terminate(reason error)` is unchanged. If empty, delete it; otherwise keep as is.

```go
// before
type MyApp struct{}
func (a *MyApp) Load(node gen.Node, args ...any) (gen.ApplicationSpec, error) { ... }
func (a *MyApp) Start(mode gen.ApplicationMode) {}
func (a *MyApp) Terminate(reason error)         {}

// after
type MyApp struct {
    app.Application
}
func (a *MyApp) Load(args ...any) (gen.ApplicationSpec, error) { ... }
```

New optional callbacks `Init` and `Stop` exist on the interface but are no-op by default through the embed. Override them only if you need pre-start resource initialization or pre-stop drain logic. See [Lifecycle Callbacks](#lifecycle-callbacks).
{% endhint %}

## The Need for Applications

Starting processes one at a time works for simple systems. But as complexity grows, you face coordination problems. Which processes should start first? What if one fails to start - do you continue or abort? If a critical component terminates, should the service keep running in a degraded state or shut down cleanly?

These aren't implementation details - they're architectural decisions about your service's structure and fault tolerance policy. Applications let you declare these decisions explicitly rather than scattering the logic throughout your code. The specification documents what your service consists of. The mode declares your termination policy. The framework enforces both.

## Defining an Application

Applications embed `app.Application` and implement `Load`:

```go
import (
    "ergo.services/ergo/app"
    "ergo.services/ergo/gen"
)

type MyApp struct {
    app.Application
    db *sql.DB
}

func (a *MyApp) Load(args ...any) (gen.ApplicationSpec, error) {
    a.Log().Info("loading config")
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

`Load` returns the application specification: what this application consists of and how it should behave. The embedded `app.Application` provides helper methods (`Node`, `Log`, `Name`, `AddTag`, `SetWeight`, and so on) bound to the running application. They are available from inside `Load` and all other callbacks.

The full lifecycle interface is:

```go
type ApplicationBehavior interface {
    PreLoad(app Application, args ...any) (ApplicationSpec, error)  // do not override
    Load(args ...any) (ApplicationSpec, error)                       // implement this
    Init(ref Ref, mode ApplicationMode) error                        // optional, pre-start
    Start(ref Ref, mode ApplicationMode)                             // optional, post-start
    Stop(ref Ref, reason error)                                      // optional, pre-stop
    Terminate(reason error)                                          // optional, post-stop
}
```

`app.Application` implements `PreLoad` and provides default no-op `Init`, `Start`, `Stop`, `Terminate`. Override only the callbacks you need.

> **Do not override `PreLoad`.** It is the framework entry point that binds the runtime application before dispatching `Load`. Overriding breaks the binding and triggers a panic on the next default callback.

The `Group` lists processes to start. Processes start in the order listed. If a process has a `Name`, it's registered with that name, making it discoverable. Processes without names are anonymous.

Application names and process names exist in separate namespaces. An application named "api" and a process named "api" do not conflict - you can have both registered simultaneously. However, using the same name for both creates confusion when reading code or debugging. Avoid identical names even though the framework allows it.

## Application Modes

The mode determines what happens when a process in the application terminates.

**Temporary Mode** - The application continues running despite individual process terminations. Only when all processes have stopped does the application itself terminate. This mode is for applications where components can fail and restart independently (typically via supervisors) without stopping the whole application.

**Transient Mode** - The application stops if any process terminates abnormally (crashes, panics, errors). Normal termination doesn't trigger shutdown. When an abnormal termination occurs, all remaining processes receive exit signals and the application shuts down. Use this mode when abnormal failures indicate a systemic problem that requires stopping the entire service.

**Permanent Mode** - The application stops if any process terminates, regardless of reason. Even normal termination of one process triggers shutdown of all others and the application itself. This mode is for applications where all components must run together - if one stops, the whole application is incomplete.

## Lifecycle Callbacks

Each callback has a clear responsibility in the application lifecycle:

- **`Load`**: declarative. Validate configuration, return the spec. Avoid side effects.
- **`Init`**: pre-start. Open external resources the `Group` processes will need: database connection pools, caches, message queues. Returning an error aborts the start; `Terminate` is **not** called.
- **`Start`**: post-start. The `Group` is running. Register health checks, export metrics, notify the load balancer that the instance is ready.
- **`Stop`**: pre-stop. The `Group` is still running. Drain in-flight work, deregister health checks, mark unhealthy in the load balancer so traffic stops being routed here.
- **`Terminate`**: post-stop. The `Group` has finished. Close resources opened in `Init`.

Each of `Init`, `Start`, `Stop` receives a `gen.Ref` carrying a deadline. Check `ref.IsAlive()` to detect when your callback has exceeded its timeout budget. If it has, the framework has already moved on; unwind gracefully and return.

Timeouts are configured in `ApplicationSpec` or overridden per-start via `ApplicationOptions`:

```go
gen.ApplicationSpec{
    InitTimeout:  10 * time.Second,
    StartTimeout: 5 * time.Second,
    StopTimeout:  10 * time.Second,
}
```

Default is 15 seconds for each.

## Loading and Starting

Applications go through two phases: loading and starting.

Loading calls your `Load` callback, validates the specification, and registers the application with the node. The application is in `Loaded` state but not running. This separation allows you to load multiple applications and resolve dependencies before starting any of them.

Starting follows this sequence:

1. State transitions from `Loaded` to `Initializing`.
2. `Init` callback runs (within `InitTimeout`). On error or timeout the state reverts to `Loaded` and `Terminate` is **not** called.
3. The framework spawns each process in `Group` in order. If any spawn fails after `Init` succeeded, the state transitions to `Stopping`, already-spawned members are killed, and `Terminate` runs to release resources opened in `Init`.
4. State transitions from `Initializing` to `Running`.
5. `Start` callback runs (within `StartTimeout`). A timeout here is non-fatal; the application stays in `Running` state.

Per-process `gen.ProcessOptions.InitTimeout` has a hard cap of 15 seconds inside an application context. Setting a higher value returns `gen.ErrNotAllowed` and prevents the application from starting.

## Dependencies

Applications can depend on other applications or network services. If application B depends on application A, the node ensures A is running before starting B. Dependencies are declared in `ApplicationSpec.Depends`.

This allows you to structure complex systems with clear startup ordering. A database connection pool application starts before the API server application. The API server starts before the web frontend application. The framework handles the ordering automatically.

## Network Declarations

`ApplicationSpec.Network` is the declarative form for everything the application contributes to the node's network. Currently it covers wire-format registration:

```go
gen.ApplicationSpec{
    Name: "myapp",
    Network: gen.ApplicationNetwork{
        RegisterTypes:  []any{Order{}, Customer{}},
        RegisterErrors: []error{ErrInvalidOrder},
        RegisterAtoms:  []gen.Atom{"my_atom"},
    },
}
```

Entries are processed during `ApplicationLoad`, before any process in the application is spawned. If the node's network mode is `NetworkModeDisabled`, the entries are silently ignored and the application loads as usual. For details on what to register and why, see [Network Transparency](../networking/network-transparency.md#type-registration-requirements).

## Stopping Applications

Applications stop in three ways.

`ApplicationStop` triggers a graceful shutdown: state transitions to `Stopping`, the `Stop` callback runs (within `StopTimeout`), exit signals are sent to all Group processes, and once the last process has terminated the `Terminate` callback runs and the application transitions back to `Loaded` state.

`ApplicationStopForce` skips the `Stop` callback and immediately kills all processes. `Terminate` still runs after the last process is gone. Less graceful, but guaranteed to stop quickly.

The application can also stop itself based on its mode. In Transient or Permanent mode, process failures trigger automatic shutdown according to the mode's rules. The same `Stop` then `Terminate` callback flow runs, dispatched from a coordinator goroutine so the process termination path is not blocked.

A stop is not a restart. The node does not bring a stopped application back; recovery is left to you. The application could announce its own death from `Terminate` by sending a message somewhere, but it is better left to the bus: hand-wiring notifications couples the application to whoever cares and reinvents what events already do. The node publishes the stop for you as a `gen.MessageCoreApplicationStopped` carrying the application name and the reason it stopped; interested processes subscribe and the application never tracks who is watching. Subscribe to `gen.CoreEvent` to act on it, locally or, since events cross nodes, from one observer watching every node in the cluster. See [The Node's Own Event Bus](events.md#the-nodes-own-event-bus).

## Environment and Configuration

Applications have environment variables that all their processes inherit. These override node-level variables but are overridden by process-specific variables. This creates a natural layering: node provides defaults, application provides service-specific values, processes can override for their specific needs.

## Accessing the Application

The runtime application is available from two places.

Inside the application's own callbacks (`Load`, `Init`, `Start`, `Stop`, `Terminate`), the embedded `app.Application` is bound to the running application before `Load` runs. Methods promoted through the embed (`a.Node()`, `a.Log()`, `a.Name()`, `a.Tags()`, `a.AddTag()`, `a.Weight()`, `a.SetWeight()`, and others) reach the runtime directly.

Processes that belong to the application access it through `Process.Application()`. This returns a `gen.Application` interface bound to the same runtime, so a worker can introspect or mutate its parent application:

```go
func (w *Worker) HandleMessage(from gen.PID, msg any) error {
    app := w.Application()
    if app == nil {
        return nil // not running under any application
    }
    if w.isReady() {
        app.AddTag("ready")
    }
    return nil
}
```

`Process.Application()` returns `nil` for processes spawned outside any application (directly via `node.Spawn`).

## Application Logging

Applications have their own log source distinct from the node. Log messages emitted via `a.Log()` from inside any callback are tagged with the application's identity (node hash and application name), making them filterable across cluster-wide log aggregation.

In plain-text output the source appears as `App#<NodeHash.'name'>`, mirroring the format of `gen.PID` and `gen.ProcessID`. In structured JSON output, the source carries `type: application`, the node hash, the application name, and the current mode. Custom loggers can dispatch on `gen.MessageLogApplication` to format application logs differently from node, process, or network logs.

## Tags for Instance Selection

Running multiple instances of the same application across a cluster creates a selection problem. Which instance should handle the request? In blue/green deployments, you run two versions and route traffic based on readiness. Canary deployments send a percentage to the new version. Some instances enter maintenance mode while others serve production traffic.

Tags provide metadata for making these decisions. Label each application instance with tags describing its deployment state, version, or role:

```go
func (a *MyApp) Load(args ...any) (gen.ApplicationSpec, error) {
    return gen.ApplicationSpec{
        Name: "api_service",
        Tags: []gen.Atom{"blue", "v2.1.0"},
        // ... rest of spec
    }, nil
}
```

Tags are always available through `node.ApplicationInfo()` or `remoteNode.ApplicationInfo()`. For clusters using centralized registrars (etcd, Saturn), tags are also published during application route registration. This enables cluster-wide discovery: query the registrar and receive all application instances with their tags.

Tags and weight can also be mutated at runtime through the `gen.Application` interface. From inside any application callback, the embedded `app.Application` provides `AddTag`, `RemoveTag`, `SetTags`, `SetWeight`. Processes within the application can access the same methods through `Process.Application()`:

```go
// inside MyApp callback
a.AddTag("ready")     // mark instance ready, push update to registrar

// inside a process within the application
func (w *Worker) HandleMessage(from gen.PID, msg any) error {
    if w.degraded() {
        w.Application().AddTag("degraded")
    }
    return nil
}
```

Mutations push an updated route to the registrar so other nodes see the change on next route refresh. This lets you flip an instance into maintenance mode, mark it ready after warmup, or adjust its weight based on load, all from inside the application.

The embedded in-memory registrar does not support application route registration, so tags in single-node or statically-routed deployments are only accessible via direct `ApplicationInfo()` calls, not through resolver queries.

In clusters with centralized registrars, resolve the application and chain filter methods on the result:

```go
// Query the registrar for all instances
routes, err := network.ResolveApplication("api_service")
// routes is gen.ApplicationRoutes: a slice of ApplicationRoute with
// chainable filter methods.

// Filter: only blue, ready, in Running state, not draining
selected := routes.
    WithTags("blue", "ready").
    WithoutTags("draining").
    WithState(gen.ApplicationStateRunning)

for _, route := range selected {
    remoteNode, _ := network.GetNode(route.Node)
    info, _ := remoteNode.ApplicationInfo("api_service")
    // Use this instance
}
```

`WithTags(tags...)` keeps only routes that have **all** the given tags. `WithoutTags(tags...)` drops routes that contain **any** of the given tags. `WithState(states...)` keeps routes in the given states. Each method returns a fresh `ApplicationRoutes` so chaining is non-destructive; the original slice is unchanged.

Common tag patterns:
- **Blue/green deployment**: "blue", "green"
- **Canary rollout**: "canary", "stable"
- **Maintenance state**: "maintenance", "active", "draining"

The release itself needs no tag: every route carries `Version` from the application's spec, so `route.Version` already tells one rollout from another. Tags express the role an instance plays in a deployment, which is a different question from which build it runs.
- **Geographic region**: "us-east", "eu-west"

Tags separate deployment strategy from application code. Your application doesn't know it's the "blue" deployment - that's configuration. The routing logic queries tags and makes decisions based on current cluster state.

## Process Role Mapping

Applications contain multiple processes with specific responsibilities. An API server handles requests. A connection pool manages database connections. A cache manager stores frequently accessed data. These are logical roles, but the actual process names might be versioned, generated, or environment-specific.

The `Map` field bridges this gap. Define a mapping from logical role (string) to actual process name (Atom):

```go
func (a *MyApp) Load(args ...any) (gen.ApplicationSpec, error) {
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

## Exposing a Helper API

An application is invoked by code that lives outside it. If that code sends the application's processes raw messages, it has to know their registered names and construct the right message types by hand. That couples every caller to your internals: rename a process or change a message and every call site breaks.

The idiomatic alternative is to give the application package a set of exported helper functions that hide those details. A helper takes the caller's process handle and sends or calls the application's local instance by its registered name (a `gen.Atom`), which stays private to the package:

```go
package orders

const name gen.Atom = "orders" // registered process name, private to this package

// message types are internal - callers never see or construct them
type messagePlace struct{ Item string; Qty int }
type statusRequest struct{ ID string }
type statusResponse struct{ Status OrderStatus }

// Place is fire-and-forget, so it wraps a Send to the local instance.
func Place(process gen.Process, item string, qty int) error {
    return process.Send(name, messagePlace{Item: item, Qty: qty})
}

// Status needs a reply, so it wraps a Call.
func Status(process gen.Process, id string) (OrderStatus, error) {
    result, err := process.Call(name, statusRequest{ID: id})
    if err != nil {
        return OrderStatus{}, err
    }
    return result.(statusResponse).Status, nil
}
```

A caller writes `orders.Place(process, "sku-1", 3)` from inside its own callback. It never constructs a message and never learns the process name.

The messaging stays private. Because callers go through the functions, message types like `messagePlace` and `statusRequest` never appear in any other package and can be unexported. A caller depends only on the helper signatures and ordinary Go types (`item string`, `qty int`), never on your message layout, so you can add a field, split a message, or rename one without changing a single call site. Exposing the message types instead would force them to be exported, since callers construct them directly, and freeze their shape into your public API. Only messages that actually cross nodes need exported fields and EDF registration; a helper talking to the local instance keeps them fully private.

The helper receives the caller's handle as an argument rather than reading a package global, which would break addressing in a multi-node cluster and make the package hard to test. That handle is normally a `gen.Process`. When there is no actor context, for example a web server translating an HTTP request into a call on another node, the helper takes a `gen.Node` and addresses the target explicitly with `node.Call(gen.ProcessID{Name: name, Node: peer}, ...)`; that fuller form is the one case where the node is named.

An application composed of several sub-components re-exports their helpers under one namespace, so callers depend on the application and never import or name the parts. `application/radar` is the model: `radar.RegisterService` delegates to the health actor and `radar.CounterAdd` to the metrics actor, and a caller never learns radar is assembled from separate health and metrics actors.

## The Application Pattern

Applications provide structure to your actor system. Instead of scattered process creation throughout your code, applications centralize the "what runs in this service" question. The specification documents your system's structure. The mode declares your fault tolerance policy. The dependency mechanism ensures correct startup ordering.

This organization becomes especially valuable in distributed systems where services start on different nodes. An application can be started remotely on another node, bringing all its components with the correct configuration and dependencies.

For more details on application lifecycle and options, refer to the `gen.ApplicationBehavior` and `gen.ApplicationSpec` documentation in the code.
