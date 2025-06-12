# Ergo Testing Unit Library

A comprehensive testing library for Ergo Framework actors with zero external dependencies.

## Features

- **Zero Dependencies**: Uses only Go standard library - no testify, no go-spew
- **Fluent API**: Chain assertions for readable and maintainable tests
- **Complete Coverage**: Supports all gen.Process, gen.Node, gen.Network interfaces
- **Dynamic Value Testing**: Handle dynamic PIDs, Aliases, and complex message patterns
- **Event Capture**: Capture and validate all actor operations (sends, spawns, calls, logs)
- **Built-in Assertions**: Common assertions without external libraries

## Quick Start

```go
package myactor_test

import (
    "testing"
    "ergo.services/ergo/testing/unit"
    "ergo.services/ergo/gen"
)

func TestMyActor(t *testing.T) {
    // Spawn actor for testing
    actor, err := unit.Spawn(t, factoryMyActor,
        unit.WithLogLevel(gen.LogLevelDebug),
        unit.WithEnv(map[gen.Env]any{"test_mode": true}),
    )
    if err != nil {
        t.Fatal(err)
    }

    // Test actor behavior with fluent assertions
    actor.ShouldSend().To(actor.PID()).Message("hello").Once().Assert()
    actor.ShouldSpawn().Times(2).Assert()
    actor.ShouldLog().Level(gen.LogLevelDebug).Containing("started").Assert()

    // Test message handling
    actor.SendMessage(gen.PID{}, "increase")
    actor.ShouldSend().To(gen.Atom("target")).Message(42).Once().Assert()
}
```

## API Reference

### Spawning Test Actors

```go
// Basic spawn
actor, err := unit.Spawn(t, factoryMyActor)

// With configuration
actor, err := unit.Spawn(t, factoryMyActor,
    unit.WithLogLevel(gen.LogLevelTrace),
    unit.WithEnv(map[gen.Env]any{"key": "value"}),
    unit.WithParent(parentPID),
    unit.WithRegister("my_actor"),
    unit.WithNodeName("test@localhost"),
)
```

### Fluent Assertions

#### Send Assertions
```go
// Basic send assertion
actor.ShouldSend().To(targetPID).Message("hello").Once().Assert()

// Pattern matching
actor.ShouldSend().To("manager").MessageMatching(func(msg any) bool {
    wc, ok := msg.(WorkerCreated)
    return ok && wc.ID.Node != ""
}).Assert()

// Structure matching with field validation
actor.ShouldSend().To("manager").MessageMatching(
    unit.StructureMatching(WorkerCreated{}, map[string]unit.Matcher{
        "ID": unit.IsValidAlias(),
    })
).Assert()

// Negative assertions
actor.ShouldNotSend().To(targetPID).Assert()
```

#### Spawn Assertions
```go
// Basic spawn
actor.ShouldSpawn().Factory(factoryWorker).Once().Assert()

// Capture spawn result for later use
spawnResult := actor.ShouldSpawn().Capture()
unit.NotNil(t, spawnResult)
```

#### SpawnMeta Assertions
```go
// Test meta process spawning
metaResult := actor.ShouldSpawnMeta().Once().Capture()
unit.NotNil(t, metaResult)

// Use captured alias in subsequent tests
actor.ShouldSend().To("manager").MessageMatching(func(msg any) bool {
    wc, ok := msg.(WorkerCreated)
    return ok && wc.ID == metaResult.ID
}).Assert()
```

#### Log Assertions
```go
// Basic log assertions
actor.ShouldLog().Level(gen.LogLevelInfo).Once().Assert()
actor.ShouldLog().Containing("started successfully").Assert()
actor.ShouldLog().Level(gen.LogLevelError).Times(0).Assert() // No errors
```

#### Call Assertions
```go
// Test synchronous calls
actor.ShouldCall().To(targetPID).Request(42).Once().Assert()

// Test call responses
result := actor.Call(gen.PID{}, "ping")
result.ShouldReturn("pong")
result.ShouldSucceed() // no error
```

### Testing Dynamic Values

Handle cases where spawned PIDs or Aliases are generated dynamically:

```go
func TestDynamicWorkerCreation(t *testing.T) {
    actor, err := unit.Spawn(t, factoryMyActor)
    if err != nil {
        t.Fatal(err)
    }

    // Send message that creates a worker
    actor.SendMessage(gen.PID{}, "create_worker")

    // Capture the spawned meta process
    metaResult := actor.ShouldSpawnMeta().Once().Capture()

    // Verify message contains the dynamic ID
    actor.ShouldSend().To("manager").MessageMatching(func(msg any) bool {
        wc, ok := msg.(WorkerCreated)
        return ok && wc.ID == metaResult.ID && wc.ID.Node == actor.Node().Name()
    }).Once().Assert()
}
```

### Built-in Matchers

```go
// Type validation
unit.IsValidPID()      // Validates gen.PID structure
unit.IsValidAlias()    // Validates gen.Alias structure
unit.IsTypeGeneric[WorkerCreated]() // Type assertion

// Field validation
unit.HasField("ID", unit.IsValidAlias())
unit.Equals(expectedValue)

// Structure matching with selective field validation
unit.StructureMatching(template, map[string]unit.Matcher{
    "ID": unit.IsValidAlias(),
    "Status": unit.Equals("active"),
})
```

### Built-in Assertions (Zero Dependencies)

```go
// Standard assertions without external libraries
unit.Equal(t, expected, actual, "Values should match")
unit.NotEqual(t, expected, actual)
unit.True(t, condition, "Condition should be true")
unit.False(t, condition, "Condition should be false")
unit.Nil(t, value, "Value should be nil")
unit.NotNil(t, value, "Value should not be nil")
unit.Contains(t, "haystack", "needle", "Should contain substring")
unit.IsType(t, expectedType{}, actualValue)
```

### Event Inspection

```go
// Access all captured events
events := actor.Events()

// Count events
count := actor.EventCount()

// Get last event
lastEvent := actor.LastEvent()

// Clear events (useful between test phases)
actor.ClearEvents()
```

### Direct Actor Access

```go
// Access the underlying behavior for state inspection
behavior := actor.Behavior().(*MyActor)
unit.Equal(t, 42, behavior.counter)

// Access test process and node
process := actor.Process()
node := actor.Node()
pid := actor.PID()
```

### RemoteNode Testing

The library provides comprehensive RemoteNode testing capabilities with helper methods and assertions:

```go
func TestRemoteNodeOperations(t *testing.T) {
    actor, _ := unit.Spawn(t, factoryMyActor)
    
    // Method 1: Create remote nodes directly
    workerNode := actor.CreateRemoteNode("worker@node1", true)  // connected
    offlineNode := actor.CreateRemoteNode("offline@node", false) // disconnected
    
    // Configure remote node properties
    workerNode.SetUptime(7200) // 2 hours uptime
    workerNode.SetVersion(gen.Version{Name: "worker", Release: "2.1.0"})
    workerNode.SetConnectionUptime(1800) // Connected for 30 minutes
    
    // Method 2: Get existing or create new remote nodes
    retrievedWorker, err := actor.GetRemoteNode("worker@node1")
    unit.Nil(t, err)
    unit.Equal(t, "worker@node1", string(retrievedWorker.Name()))
    
    // Method 3: Connect/Disconnect operations
    connectedNode := actor.ConnectRemoteNode("gateway@proxy")
    unit.NotNil(t, connectedNode)
    
    // List connected nodes
    connectedNodes := actor.ListConnectedNodes()
    unit.True(t, len(connectedNodes) >= 2)
    
    // Disconnect from a node
    err = actor.DisconnectRemoteNode("gateway@proxy")
    unit.Nil(t, err)
    
    // Method 4: Remote operations testing
    pid, err := workerNode.Spawn("test-process", gen.ProcessOptions{}, "arg1")
    unit.Nil(t, err)
    unit.Equal(t, "worker@node1", string(pid.Node))
    
    // Remote spawn with register
    regPid, err := workerNode.SpawnRegister("my-service", "service-process", gen.ProcessOptions{})
    unit.Nil(t, err)
    
    // Remote application start
    err = workerNode.ApplicationStart("web-app", gen.ApplicationOptions{})
    unit.Nil(t, err)
    
    err = workerNode.ApplicationStartPermanent("database-service", gen.ApplicationOptions{})
    unit.Nil(t, err)
    
    // Method 5: Access network through RemoteNode helper
    network := actor.RemoteNode() // Gets TestNetwork
    info, err := network.Info()
    unit.Nil(t, err)
    
    // Method 6: Advanced configuration
    advancedNode := actor.CreateRemoteNode("advanced@cluster", true)
    advancedNode.SetInfo(gen.RemoteNodeInfo{
        Node:             "advanced@cluster",
        MaxMessageSize:   2048000,
        MessagesIn:       1000,
        MessagesOut:      850,
        BytesIn:          5000000,
        BytesOut:         4200000,
    })
    
    nodeInfo := advancedNode.Info()
    unit.Equal(t, uint64(1000), nodeInfo.MessagesIn)
    unit.Equal(t, 2048000, nodeInfo.MaxMessageSize)
}
```

#### RemoteNode Helper Methods

```go
// Direct RemoteNode creation and management
actor.CreateRemoteNode(name, connected) *TestRemoteNode
actor.GetRemoteNode(name) (gen.RemoteNode, error)
actor.ConnectRemoteNode(name) gen.RemoteNode
actor.DisconnectRemoteNode(name) error
actor.ListConnectedNodes() []gen.Atom
actor.RemoteNode() *TestNetwork // Access to TestNetwork

// RemoteNode configuration methods
remoteNode.SetUptime(seconds)
remoteNode.SetConnectionUptime(seconds)
remoteNode.SetVersion(gen.Version{...})
remoteNode.SetInfo(gen.RemoteNodeInfo{...})
remoteNode.SetConnected(bool)
remoteNode.Disconnect()

// RemoteNode operations (testing)
remoteNode.Spawn(name, options, args...)
remoteNode.SpawnRegister(register, name, options, args...)
remoteNode.ApplicationStart(name, options)
remoteNode.ApplicationStartTemporary(name, options)
remoteNode.ApplicationStartTransient(name, options)
remoteNode.ApplicationStartPermanent(name, options)
```

#### RemoteNode Assertions (Future)

```go
// Remote spawn assertions
actor.ShouldRemoteSpawn().ToNode("worker@node").WithName("process").Once().Assert()
actor.ShouldNotRemoteSpawn().ToNode("worker@node").Assert()

// Remote application start assertions  
actor.ShouldRemoteApplicationStart().OnNode("api@server").Application("web-app").Once().Assert()
actor.ShouldNotRemoteApplicationStart().OnNode("api@server").Assert()

// Node connection assertions
actor.ShouldConnect().ToNode("worker@node").Once().Assert()
actor.ShouldDisconnect().ToNode("worker@node").Once().Assert()
```

### Network Testing

The library provides complete implementations of `gen.Network`, `gen.Registrar`, and `gen.Resolver` interfaces:

```go
func TestNetworkOperations(t *testing.T) {
    actor, _ := unit.Spawn(t, factoryMyActor)
    
    // Test network operations
    network := actor.Node().Network()
    
    // Test network info
    info, err := network.Info()
    unit.Nil(t, err)
    unit.Equal(t, gen.NetworkModeEnabled, info.Mode)
    
    // Test remote node operations
    remoteNode, err := network.GetNode("remote@host")
    unit.Nil(t, err)
    unit.Equal(t, gen.Atom("remote@host"), remoteNode.Name())
    
    // Test registrar operations
    registrar, err := network.Registrar()
    unit.Nil(t, err)
    
    regInfo := registrar.Info()
    unit.True(t, regInfo.EmbeddedServer)
    unit.True(t, regInfo.SupportConfig)
    
    // Test config access
    config, err := registrar.Config("test.enabled")
    unit.Nil(t, err)
    unit.Equal(t, true, config["test.enabled"])
    
    // Test route management
    testRoute := gen.NetworkRoute{
        Route: gen.Route{Host: "localhost", Port: 4369},
    }
    err = network.AddRoute("test.*", testRoute, 100)
    unit.Nil(t, err)
    
    routes, err := network.Route("test@node")
    unit.Nil(t, err)
    unit.Equal(t, 1, len(routes))
    
    // Test proxy routes
    proxyRoute := gen.NetworkProxyRoute{
        Route: gen.ProxyRoute{To: "target@node", Proxy: "proxy@node"},
    }
    err = network.AddProxyRoute("target.*", proxyRoute, 50)
    unit.Nil(t, err)
    
    // Test resolver operations
    resolver := registrar.Resolver()
    resolvedRoutes, err := resolver.Resolve(actor.Node().Name())
    unit.Nil(t, err)
    
    // Test sending to remote nodes
    actor.ShouldSend().To("remote@node").Message("ping").Once().Assert()
}
```

#### Network Interface Coverage

- **gen.Network**: Full implementation with route management, remote nodes, acceptors
- **gen.Registrar**: Complete registrar with config, proxy, and application route support  
- **gen.Resolver**: Route resolution for nodes, proxies, and applications
- **gen.RemoteNode**: Mock remote node operations (spawn, application start)
- **gen.Acceptor**: Network acceptor configuration and info

## Comparison with Previous Library

| Feature | Old Library (testify-based) | New Library (zero deps) |
|---------|----------------------------|-------------------------|
| **Dependencies** | testify, go-spew | Zero (Go stdlib only) |
| **API Style** | Artifact validation | Fluent assertions |
| **Dynamic Values** | Manual artifact inspection | Pattern matching & capture |
| **Assertions** | `require.Equal()` | `unit.Equal()` |
| **Event Handling** | `ValidateArtifacts()` | `.ShouldSend().Assert()` |
| **Readability** | Verbose artifact arrays | Fluent method chains |

## Migration Example

### Old Library
```go
// Old testify-based approach
expected := []any{
    unit.ArtifactSend{From: process.PID(), To: gen.Atom("abc"), Message: 16},
    unit.ArtifactCall{From: process.PID(), To: gen.PID{}, Request: 12345},
}
process.ValidateArtifacts(t, expected)
require.Equal(t, 16, behavior.value)
```

### New Library
```go
// New fluent approach
actor.ShouldSend().To(gen.Atom("abc")).Message(16).Once().Assert()
actor.ShouldCall().To(gen.PID{}).Request(12345).Once().Assert()
unit.Equal(t, 16, behavior.value)
```

## Advanced Usage

### Custom Matchers
```go
// Create custom matchers for domain-specific validation
func IsValidOrder() unit.Matcher {
    return func(v any) bool {
        order, ok := v.(Order)
        return ok && order.ID > 0 && order.Status != ""
    }
}

// Use in assertions
actor.ShouldSend().To("processor").MessageMatching(IsValidOrder()).Assert()
```

### Timeout Support
```go
// Add timeout to assertions (if needed)
success := unit.WithTimeout(func() {
    actor.ShouldSend().To(target).Message("done").Assert()
}, 5*time.Second)()

unit.True(t, success, "Should complete within timeout")
```

### Complex Scenario Testing
```go
func TestComplexWorkflow(t *testing.T) {
    actor, _ := unit.Spawn(t, factoryMyActor)
    
    // Phase 1: Initialization
    actor.ShouldSend().To(actor.PID()).Message("init").Assert()
    actor.ClearEvents() // Clear for next phase
    
    // Phase 2: Processing
    actor.SendMessage(gen.PID{}, "start_processing")
    worker := actor.ShouldSpawn().Capture()
    
    // Phase 3: Completion
    actor.ShouldSend().To("supervisor").MessageMatching(func(msg any) bool {
        status, ok := msg.(ProcessingComplete)
        return ok && status.WorkerID == worker.PID
    }).Assert()
}
```

This library provides a modern, dependency-free approach to testing Ergo actors with improved readability and powerful features for handling dynamic values and complex scenarios. 