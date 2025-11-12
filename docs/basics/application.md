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

## Application Modes

The mode determines what happens when a process in the application terminates.

**Temporary Mode** - The application continues running despite individual process terminations. Only when all processes have stopped does the application itself terminate. This mode is for applications where components can fail and restart independently (typically via supervisors) without stopping the whole application.

**Transient Mode** - The application stops if any process terminates abnormally (crashes, panics, errors). Normal termination doesn't trigger shutdown. When an abnormal termination occurs, all remaining processes receive exit signals and the application shuts down. Use this mode when abnormal failures indicate a systemic problem that requires stopping the entire service.

**Permanent Mode** - The application stops if any process terminates, regardless of reason. Even normal termination of one process triggers shutdown of all others and the application itself. This mode is for applications where all components must run together - if one stops, the whole application is incomplete.

## Loading and Starting

Applications go through two phases: loading and starting.

Loading calls your `Load` callback, validates the specification, and registers the application with the node. The application is loaded but not running. This separation allows you to load multiple applications and resolve dependencies before starting any of them.

Starting launches the processes in the `Group` according to their order. If dependencies are specified in `ApplicationSpec.Depends`, the node ensures those applications are running first. If any process fails to start, previously started processes are killed and the application fails to start.

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

## The Application Pattern

Applications provide structure to your actor system. Instead of scattered process creation throughout your code, applications centralize the "what runs in this service" question. The specification documents your system's structure. The mode declares your fault tolerance policy. The dependency mechanism ensures correct startup ordering.

This organization becomes especially valuable in distributed systems where services start on different nodes. An application can be started remotely on another node, bringing all its components with the correct configuration and dependencies.

For more details on application lifecycle and options, refer to the `gen.ApplicationBehavior` and `gen.ApplicationSpec` documentation in the code.
