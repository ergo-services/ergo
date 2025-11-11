# etcd Client

{% hint style="info" %}
Introduced for Ergo Framework 3.1.0 and above (not yet released. available in `v310` branch)
{% endhint %}

This package implements the `gen.Registrar` interface and serves as a client library for [etcd](https://etcd.io/), a distributed key-value store that provides a reliable way to store data that needs to be accessed by a distributed system or cluster of machines. In addition to the primary _Service Discovery_ function, it automatically notifies all connected nodes about cluster configuration changes and supports hierarchical configuration management with type conversion.

To create a client, use the `Create` function from the `etcd` package. The function requires a set of options `etcd.Options` to configure the connection and behavior.

Then, set this client in the `gen.NetworkOption.Registrar` options:

```go
import (
     "ergo.services/ergo"
     "ergo.services/ergo/gen"

     "ergo.services/registrar/etcd"
)

func main() {
     var options gen.NodeOptions
     ...
     registrarOptions := etcd.Options{
         Endpoints: []string{"localhost:2379"},
         Cluster:   "production",
     }
     options.Network.Registrar = etcd.Create(registrarOptions)
     ...
     node, err := ergo.StartNode("demo@localhost", options)
     ...
}
```

Using `etcd.Options`, you can specify:

* `Cluster` - The cluster name for your node (default: "default")
* `Endpoints` - List of etcd endpoints (default: \["localhost:2379"])
* `Username` - Username for etcd authentication (optional)
* `Password` - Password for etcd authentication (optional)
* `TLS` - TLS configuration for secure connections (optional)
* `InsecureSkipVerify` - Option to ignore TLS certificate verification
* `DialTimeout` - Connection timeout (default: 10s)
* `RequestTimeout` - Request timeout (default: 10s)
* `KeepAlive` - Keep-alive timeout (default: 10s)

When the node starts, it will register with the etcd cluster and maintain a lease to ensure automatic cleanup if the node becomes unavailable.

## Configuration Management

The etcd registrar provides hierarchical configuration management with four priority levels:

1. **Cross-cluster node-specific**: `services/ergo/config/{cluster}/{node}/{item}`
2. **Cluster node-specific**: `services/ergo/cluster/{cluster}/config/{node}/{item}`
3. **Cluster-wide default**: `services/ergo/cluster/{cluster}/config/*/{item}`
4. **Global default**: `services/ergo/config/global/{item}`

### Typed Configuration

The etcd registrar supports typed configuration values using string prefixes. Configuration values are stored as **strings** in etcd and automatically converted to the appropriate Go types when read by the registrar:

* `"int:123"` → `int64(123)`
* `"float:3.14"` → `float64(3.14)`
* `"bool:true"` → `bool(true)`, `"bool:false"` → `bool(false)`
* `"hello"` → `"hello"` (strings without prefixes remain unchanged)

**Important**: All configuration values must be stored as strings in etcd. The type conversion happens automatically when the registrar reads the configuration.

Example configuration setup using etcdctl:

```bash
# Node-specific integer configuration (stored as string, converted to int64)
etcdctl put services/ergo/cluster/production/config/web1/database.port "int:5432"

# Cluster-wide float configuration (stored as string, converted to float64)
etcdctl put services/ergo/cluster/production/config/*/cache.ratio "float:0.75"

# Boolean configuration (stored as string, converted to bool)
etcdctl put services/ergo/cluster/production/config/*/debug.enabled "bool:true"
etcdctl put services/ergo/cluster/production/config/web1/ssl.enabled "bool:false"

# Application-specific configuration (visible to all nodes using wildcard format)
etcdctl put services/ergo/cluster/production/config/*/myapp.cache.size "int:256"
etcdctl put services/ergo/cluster/production/config/*/client.timeout "int:30"

# Global string configuration (stored and returned as string)
etcdctl put services/ergo/config/global/log.level "info"
```

Access configuration in your application:

```go
registrar, err := node.Network().Registrar()
if err != nil {
    return err
}

// Get single configuration item
port, err := registrar.ConfigItem("database.port")
if err != nil {
    return err
}
// port will be int64(5432)

// Get multiple configuration items
config, err := registrar.Config("database.port", "cache.ratio", "debug.enabled", "log.level")
if err != nil {
    return err
}
// config["database.port"] = int64(5432)
// config["cache.ratio"] = float64(0.75) 
// config["debug.enabled"] = bool(true)
// config["log.level"] = "info"
```

## Event System

The etcd registrar registers a `gen.Event` and generates messages based on changes in the etcd cluster within the specified cluster. This allows the node to stay informed of any updates or changes within the cluster, ensuring real-time event-driven communication and responsiveness to cluster configurations:

* `etcd.EventNodeJoined` - Triggered when another node is registered in the same cluster
* `etcd.EventNodeLeft` - Triggered when a node disconnects or its lease expires
* `etcd.EventApplicationLoaded` - An application was loaded on a remote node
* `etcd.EventApplicationStarted` - Triggered when an application starts on a remote node
* `etcd.EventApplicationStopping` - Triggered when an application begins stopping on a remote node
* `etcd.EventApplicationStopped` - Triggered when an application is stopped on a remote node
* `etcd.EventConfigUpdate` - The cluster configuration was updated

To receive such messages, you need to subscribe to etcd client events using the `LinkEvent` or `MonitorEvent` methods from the `gen.Process` interface. You can obtain the name of the registered event using the `Event` method from the `gen.Registrar` interface:

```go
type myActor struct {
    act.Actor
}

func (m *myActor) HandleMessage(from gen.PID, message any) error {
    reg, err := m.Node().Network().Registrar()
    if err != nil {
        m.Log().Error("unable to get Registrar interface: %s", err)
        return nil
    }
    
    ev, err := reg.Event()
    if err != nil {
        m.Log().Error("Registrar has no registered Event: %s", err)
        return nil
    }
    
    m.MonitorEvent(ev)
    return nil
}

func (m *myActor) HandleEvent(event gen.MessageEvent) error {
    switch msg := event.Message.(type) {
    case etcd.EventNodeJoined:
        m.Log().Info("Node %s joined cluster", msg.Name)
    case etcd.EventApplicationStarted:
        m.Log().Info("Application %s started on node %s", msg.Name, msg.Node)
    case etcd.EventConfigUpdate:
        m.Log().Info("Configuration %s updated", msg.Item)
        
        // Handle specific configuration changes
        if msg.Item == "ssl.enabled" {
            if enabled, ok := msg.Value.(bool); ok {
                m.Log().Info("SSL %s", map[bool]string{true: "enabled", false: "disabled"}[enabled])
            }
        }
    }
    return nil
}
```

## Application Discovery

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

* `Name` - The name of the application
* `Node` - The name of the node where the application is loaded or running
* `Weight` - The weight assigned to the application in `gen.ApplicationSpec`
* `Mode` - The application's startup mode (`gen.ApplicationModeTemporary`, `gen.ApplicationModePermanent`, `gen.ApplicationModeTransient`)
* `State` - The current state of the application (`gen.ApplicationStateLoaded`, `gen.ApplicationStateRunning`, `gen.ApplicationStateStopping`)

You can access the `gen.Resolver` interface using the `Resolver` method from the `gen.Registrar` interface:

```go
resolver := registrar.Resolver()

// Resolve application routes
routes, err := resolver.ResolveApplication("web-server")
if err != nil {
    return err
}

for _, route := range routes {
    log.Printf("Application %s running on node %s (weight: %d, state: %s)", 
        route.Name, route.Node, route.Weight, route.State)
}
```

## Node Discovery

Get a list of all nodes in the cluster:

```go
nodes, err := registrar.Nodes()
if err != nil {
    return err
}

for _, nodeName := range nodes {
    log.Printf("Node in cluster: %s", nodeName)
}
```

## Data Storage Structure

The etcd registrar organizes data in etcd using the following key structure:

```
services/ergo/cluster/{cluster}/
├── routes/                         # Non-overlapping with config paths
│   ├── nodes/{node}               # Node registration with lease (edf.Encode + base64)
│   └── applications/{app}/{node}  # Application routes (edf.Encode + base64)
└── config/                        # Configuration data (string + type prefixes) 
    ├── {node}/{item}             # Node-specific config
    └── */{item}                  # Cluster-wide config

services/ergo/config/
├── {cluster}/{node}/{item}        # Cross-cluster node config
└── global/{item}                  # Global config
```

**Important Architecture Notes:**
- **Routes** (nodes/applications) use `edf.Encode + base64` encoding and are stored in the `routes/` subpath. Don't change anything there. 
- **Configuration** uses string encoding with type prefixes and is stored in the `config/` subpath  

## Example

A fully featured example can be found at [GitHub - Ergo Services Examples](https://github.com/ergo-services/examples) in the `docker` directory.

This example demonstrates how to run multiple Ergo nodes using etcd as a registrar for service discovery. It showcases service discovery, actor communication, typed configuration management, and **real-time configuration event monitoring** across a cluster.

## Development and Testing

The etcd registrar includes comprehensive testing infrastructure:

### Docker Testing Setup

Use the included Docker Compose setup for testing:

```bash
# Start etcd for testing
make start-etcd

# Run tests with coverage
make test-coverage

# Run integration tests only
make test-integration

# Clean up
make clean
```

### Manual etcd Operations

For debugging and manual operations:

```bash
# Check cluster health
etcdctl --endpoints=localhost:12379 endpoint health

# List all keys in cluster
etcdctl --endpoints=localhost:12379 get --prefix "services/ergo/"

# Set configuration manually (values must be strings)
etcdctl --endpoints=localhost:12379 put \
  "services/ergo/cluster/production/config/web1/database.timeout" "int:30"

etcdctl --endpoints=localhost:12379 put \
  "services/ergo/cluster/production/config/web1/debug.enabled" "bool:true"

# Watch for changes
etcdctl --endpoints=localhost:12379 watch --prefix "services/ergo/cluster/production/"
```

The etcd registrar provides a robust, scalable solution for service discovery and configuration management in distributed Ergo applications, with the reliability and consistency guarantees of etcd.
