# Network Setup Guide for Ergo Testing Library

This guide explains how to add nodes and routes to the Network in the Ergo testing library to simulate various network configurations for testing.

## Quick Reference

```go
// Get the network from your test actor
network := actor.Node().Network()
registrar, _ := network.Registrar()

// Add static routes (Method 1)
network.AddRoute("pattern", networkRoute, weight)

// Add remote nodes (Method 3)  
network.(*unit.TestNetwork).AddRemoteNode("node@host", connected)

// Add nodes to registrar (Method 4)
registrar.(*unit.TestRegistrar).AddNode("node@host", routes)

// Set registrar config (Method 5)
registrar.(*unit.TestRegistrar).SetConfig("key", value)
```

## Method 1: Add Static Routes to Network

Static routes are used for outgoing connections. When your actor sends a message to a remote node, the network uses these routes to determine how to connect.

```go
func TestStaticRoutes(t *testing.T) {
    actor, _ := unit.Spawn(t, factoryMyActor)
    network := actor.Node().Network()

    // Basic route
    basicRoute := gen.NetworkRoute{
        Route: gen.Route{
            Host: "192.168.1.100",
            Port: 4369,
            TLS:  false,
        },
    }
    
    // Add route with pattern matching
    err := network.AddRoute("worker.*", basicRoute, 100)
    unit.Nil(t, err)
    
    // Add route for specific node (exact match)
    specificRoute := gen.NetworkRoute{
        Route: gen.Route{
            Host: "10.0.0.50", 
            Port: 4370,
            TLS:  true,
        },
    }
    err = network.AddRoute("^database@prod$", specificRoute, 200) // Higher weight = priority
    unit.Nil(t, err)
    
    // Test route resolution
    routes, err := network.Route("worker@node1") // Matches "worker.*"
    unit.Nil(t, err)
    unit.Equal(t, 1, len(routes))
    unit.Equal(t, "192.168.1.100", routes[0].Route.Host)
}
```

### Route Patterns

- `"worker.*"` - Matches any node starting with "worker"
- `"^database@prod$"` - Exact match for "database@prod"
- `".*@cluster"` - Matches any node ending with "@cluster"
- `"test"` - Simple substring match

### Route Weights

Routes with higher weights have priority. If multiple routes match the same node name, the one with the highest weight is used.

## Method 2: Add Proxy Routes

Proxy routes allow routing through intermediate nodes.

```go
func TestProxyRoutes(t *testing.T) {
    actor, _ := unit.Spawn(t, factoryMyActor)
    network := actor.Node().Network()
    
    // Route through a proxy
    proxyRoute := gen.NetworkProxyRoute{
        Route: gen.ProxyRoute{
            To:    "target@remote",      // Final destination
            Proxy: "gateway@proxy",      // Intermediate proxy node
        },
    }
    
    err := network.AddProxyRoute("target.*", proxyRoute, 50)
    unit.Nil(t, err)
    
    // Test proxy route resolution
    proxyRoutes, err := network.ProxyRoute("target@remote")
    unit.Nil(t, err)
    unit.Equal(t, 1, len(proxyRoutes))
    unit.Equal(t, "gateway@proxy", string(proxyRoutes[0].Route.Proxy))
}
```

## Method 3: Add Remote Nodes (Mock Connections)

This simulates existing connections to remote nodes.

```go
func TestRemoteNodes(t *testing.T) {
    actor, _ := unit.Spawn(t, factoryMyActor)
    network := actor.Node().Network()
    
    // Add connected remote nodes
    workerNode := network.(*unit.TestNetwork).AddRemoteNode("worker@node1", true)  // connected
    dbNode := network.(*unit.TestNetwork).AddRemoteNode("database@prod", true)     // connected
    network.(*unit.TestNetwork).AddRemoteNode("offline@node", false)              // not connected
    
    // Configure node properties
    workerNode.SetUptime(3600) // 1 hour uptime
    workerNode.SetVersion(gen.Version{Name: "worker", Release: "2.1"})
    workerNode.SetConnectionUptime(600) // Connected for 10 minutes
    
    dbNode.SetVersion(gen.Version{Name: "database", Release: "1.5"})
    
    // Test node access
    node, err := network.Node("worker@node1")
    unit.Nil(t, err, "Should find connected node")
    unit.Equal(t, "worker@node1", string(node.Name()))
    unit.Equal(t, int64(3600), node.Uptime())
    
    // Test disconnected node
    _, err = network.Node("offline@node")
    unit.Equal(t, gen.ErrNoConnection, err, "Should not find disconnected node")
    
    // Test connected nodes list
    connectedNodes := network.Nodes()
    unit.True(t, len(connectedNodes) >= 2, "Should have connected nodes")
    
    // Verify specific nodes are in the list
    nodeMap := make(map[gen.Atom]bool)
    for _, node := range connectedNodes {
        nodeMap[node] = true
    }
    unit.True(t, nodeMap["worker@node1"], "Should find worker@node1")
    unit.True(t, nodeMap["database@prod"], "Should find database@prod")
    unit.False(t, nodeMap["offline@node"], "Should not find offline@node")
}
```

### Remote Node Methods

```go
// Available configuration methods:
remoteNode.SetUptime(seconds)
remoteNode.SetConnectionUptime(seconds) 
remoteNode.SetVersion(gen.Version{...})
remoteNode.SetConnected(bool)
remoteNode.Disconnect() // Sets connected to false

// Remote node operations (for testing):
pid, err := remoteNode.Spawn("process_name", options, args...)
pid, err := remoteNode.SpawnRegister("register_name", "process_name", options, args...)
err := remoteNode.ApplicationStart("app_name", options)
err := remoteNode.ApplicationStartTemporary("app_name", options)
// etc.
```

## Method 4: Add Nodes to Registrar (Service Discovery)

This simulates nodes registered on the registrar for service discovery.

```go
func TestRegistrarNodes(t *testing.T) {
    actor, _ := unit.Spawn(t, factoryMyActor)
    registrar, _ := actor.Node().Network().Registrar()
    
    // Add nodes with multiple routes (load balancing)
    workerRoutes := []gen.Route{
        {Host: "192.168.1.101", Port: 4369, TLS: false},
        {Host: "192.168.1.102", Port: 4369, TLS: false}, 
        {Host: "192.168.1.103", Port: 4369, TLS: false},
    }
    registrar.(*unit.TestRegistrar).AddNode("worker@cluster", workerRoutes)
    
    // Add single route node
    dbRoutes := []gen.Route{
        {Host: "10.0.0.50", Port: 4370, TLS: true},
    }
    registrar.(*unit.TestRegistrar).AddNode("database@prod", dbRoutes)
    
    // Test registrar resolution
    resolver := registrar.Resolver()
    
    // Resolve multiple routes
    routes, err := resolver.Resolve("worker@cluster")
    unit.Nil(t, err)
    unit.Equal(t, 3, len(routes), "Should resolve multiple routes")
    
    // Verify route details
    unit.Equal(t, "192.168.1.101", routes[0].Host)
    unit.Equal(t, uint16(4369), routes[0].Port)
    unit.False(t, routes[0].TLS)
    
    // Resolve single route
    routes, err = resolver.Resolve("database@prod")
    unit.Nil(t, err)
    unit.Equal(t, 1, len(routes))
    unit.Equal(t, "10.0.0.50", routes[0].Host)
    unit.True(t, routes[0].TLS)
    
    // Test non-existent node
    _, err = resolver.Resolve("nonexistent@node")
    unit.Equal(t, gen.ErrNoRoute, err)
    
    // List all registered nodes
    nodes, err := registrar.Nodes()
    unit.Nil(t, err)
    unit.True(t, len(nodes) >= 2)
}
```

## Method 5: Set Registrar Configuration

Configure the registrar with key-value pairs for testing configuration access.

```go
func TestRegistrarConfig(t *testing.T) {
    actor, _ := unit.Spawn(t, factoryMyActor)
    registrar, _ := actor.Node().Network().Registrar()
    
    // Set configuration values
    testRegistrar := registrar.(*unit.TestRegistrar)
    testRegistrar.SetConfig("cluster.name", "test-cluster")
    testRegistrar.SetConfig("cluster.region", "us-west-2")
    testRegistrar.SetConfig("worker.pool_size", 10)
    testRegistrar.SetConfig("worker.timeout", 30)
    testRegistrar.SetConfig("debug.enabled", true)
    
    // Test single config item
    value, err := registrar.ConfigItem("cluster.name")
    unit.Nil(t, err)
    unit.Equal(t, "test-cluster", value)
    
    // Test multiple config items
    config, err := registrar.Config("cluster.name", "worker.pool_size", "debug.enabled")
    unit.Nil(t, err)
    unit.Equal(t, "test-cluster", config["cluster.name"])
    unit.Equal(t, 10, config["worker.pool_size"])
    unit.Equal(t, true, config["debug.enabled"])
    
    // Test all config (no arguments)
    allConfig, err := registrar.Config()
    unit.Nil(t, err)
    unit.True(t, len(allConfig) >= 5, "Should have all config items")
    
    // Test non-existent config
    _, err = registrar.ConfigItem("nonexistent.key")
    unit.Equal(t, gen.ErrUnknown, err)
}
```

## Method 6: Register Application Routes

Set up application routes for testing application discovery.

```go
func TestApplicationRoutes(t *testing.T) {
    actor, _ := unit.Spawn(t, factoryMyActor)
    registrar, _ := actor.Node().Network().Registrar()
    
    // Register applications
    webApp := gen.ApplicationRoute{
        Node:   "worker@node1",
        Name:   "web-server",
        Weight: 100,
        Mode:   gen.ApplicationModePermanent,
        State:  gen.ApplicationStateRunning,
    }
    err := registrar.RegisterApplicationRoute(webApp)
    unit.Nil(t, err)
    
    apiApp := gen.ApplicationRoute{
        Node:   "worker@node2", 
        Name:   "api-server",
        Weight: 90,
        Mode:   gen.ApplicationModeTransient,
        State:  gen.ApplicationStateRunning,
    }
    err = registrar.RegisterApplicationRoute(apiApp)
    unit.Nil(t, err)
    
    // Add multiple instances of same app
    webApp2 := gen.ApplicationRoute{
        Node:   "worker@node3",
        Name:   "web-server", // Same app name, different node
        Weight: 80,
        Mode:   gen.ApplicationModePermanent,
        State:  gen.ApplicationStateRunning,
    }
    err = registrar.RegisterApplicationRoute(webApp2)
    unit.Nil(t, err)
    
    // Test application resolution
    resolver := registrar.Resolver()
    
    // Find all instances of web-server
    routes, err := resolver.ResolveApplication("web-server")
    unit.Nil(t, err)
    unit.Equal(t, 2, len(routes), "Should find 2 instances")
    
    // Verify route details
    for _, route := range routes {
        unit.Equal(t, "web-server", string(route.Name))
        unit.Equal(t, gen.ApplicationModePermanent, route.Mode)
        unit.Equal(t, gen.ApplicationStateRunning, route.State)
        unit.True(t, route.Node == "worker@node1" || route.Node == "worker@node3")
    }
    
    // Find single instance app
    routes, err = resolver.ResolveApplication("api-server")
    unit.Nil(t, err)
    unit.Equal(t, 1, len(routes))
    unit.Equal(t, "worker@node2", string(routes[0].Node))
    
    // Test non-existent app
    _, err = resolver.ResolveApplication("nonexistent-app")
    unit.Equal(t, gen.ErrNoRoute, err)
}
```

## Complete Example: Complex Network Setup

```go
func TestCompleteNetworkSetup(t *testing.T) {
    actor, _ := unit.Spawn(t, factoryMyActor)
    network := actor.Node().Network()
    registrar, _ := network.Registrar()
    
    // === STEP 1: Configure static routes ===
    
    // Routes for worker nodes (pattern matching)
    workerRoute := gen.NetworkRoute{
        Route: gen.Route{Host: "worker.cluster.local", Port: 4369, TLS: false},
    }
    network.AddRoute("worker@.*", workerRoute, 100)
    
    // Route for database (exact match, higher priority)
    dbRoute := gen.NetworkRoute{
        Route: gen.Route{Host: "db.cluster.local", Port: 4370, TLS: true},
    }
    network.AddRoute("^database@prod$", dbRoute, 200)
    
    // === STEP 2: Add proxy routes ===
    
    proxyRoute := gen.NetworkProxyRoute{
        Route: gen.ProxyRoute{To: "external@service", Proxy: "gateway@dmz"},
    }
    network.AddProxyRoute("external@.*", proxyRoute, 50)
    
    // === STEP 3: Add remote nodes (simulated connections) ===
    
    workerNode := network.(*unit.TestNetwork).AddRemoteNode("worker@node1", true)
    workerNode.SetUptime(7200) // 2 hours
    workerNode.SetVersion(gen.Version{Name: "worker", Release: "2.1.0"})
    
    dbNode := network.(*unit.TestNetwork).AddRemoteNode("database@prod", true)  
    dbNode.SetVersion(gen.Version{Name: "postgres", Release: "13.4"})
    
    // === STEP 4: Configure registrar nodes ===
    
    // Multi-instance worker pool
    workerRoutes := []gen.Route{
        {Host: "worker1.local", Port: 4369, TLS: false},
        {Host: "worker2.local", Port: 4369, TLS: false},
        {Host: "worker3.local", Port: 4369, TLS: false},
    }
    registrar.(*unit.TestRegistrar).AddNode("worker@pool", workerRoutes)
    
    // === STEP 5: Set configuration ===
    
    testRegistrar := registrar.(*unit.TestRegistrar)
    testRegistrar.SetConfig("cluster.name", "production")
    testRegistrar.SetConfig("cluster.size", 5)
    testRegistrar.SetConfig("worker.max_tasks", 1000)
    testRegistrar.SetConfig("database.connection_pool", 20)
    
    // === STEP 6: Register applications ===
    
    webApps := []gen.ApplicationRoute{
        {Node: "worker@node1", Name: "web", Weight: 100, Mode: gen.ApplicationModePermanent, State: gen.ApplicationStateRunning},
        {Node: "worker@node2", Name: "web", Weight: 90, Mode: gen.ApplicationModePermanent, State: gen.ApplicationStateRunning},
    }
    
    for _, app := range webApps {
        registrar.RegisterApplicationRoute(app)
    }
    
    // === VERIFICATION ===
    
    // Test static route resolution
    routes, _ := network.Route("worker@node5") // Matches pattern
    unit.Equal(t, "worker.cluster.local", routes[0].Route.Host)
    
    routes, _ = network.Route("database@prod") // Exact match  
    unit.Equal(t, "db.cluster.local", routes[0].Route.Host)
    unit.True(t, routes[0].Route.TLS)
    
    // Test remote node access
    node, _ := network.Node("worker@node1")
    unit.Equal(t, int64(7200), node.Uptime())
    
    // Test registrar resolution
    resolver := registrar.Resolver()
    poolRoutes, _ := resolver.Resolve("worker@pool")
    unit.Equal(t, 3, len(poolRoutes))
    
    // Test configuration access
    config, _ := registrar.Config("cluster.name", "worker.max_tasks")
    unit.Equal(t, "production", config["cluster.name"])
    unit.Equal(t, 1000, config["worker.max_tasks"])
    
    // Test application discovery
    webRoutes, _ := resolver.ResolveApplication("web")
    unit.Equal(t, 2, len(webRoutes))
}
```

## Best Practices

1. **Setup Order**: Configure routes before adding nodes that will use them
2. **Pattern Specificity**: Use specific patterns for exact matches (`^node@exact$`) and general patterns for groups (`worker.*`)
3. **Weight Strategy**: Use higher weights (200+) for critical/priority routes, lower weights (50-) for fallback routes
4. **Node States**: Set realistic uptimes and versions for more accurate testing
5. **Configuration**: Use hierarchical keys (`cluster.name`, `worker.timeout`) for better organization
6. **Cleanup**: The test environment automatically cleans up between tests

## Troubleshooting

### Route Not Found
```go
routes, err := network.Route("mynode@host")
if err != nil || len(routes) == 0 {
    // Check your pattern matching - might need "mynode.*" instead of exact match
}
```

### Node Connection Issues  
```go
node, err := network.Node("remote@host")
if err == gen.ErrNoConnection {
    // Node exists but isn't connected - check AddRemoteNode(name, true)
}
```

### Configuration Not Found
```go  
value, err := registrar.ConfigItem("my.key")
if err == gen.ErrUnknown {
    // Key doesn't exist - check SetConfig() calls
}
```

This guide covers all the methods for setting up network configurations in your tests. Use these patterns to create realistic network environments for testing your Ergo Framework actors. 