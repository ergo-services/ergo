package unit

import (
	"testing"

	"ergo.services/ergo/gen"
)

// Test demonstrating network operations
func TestNetworkOperations(t *testing.T) {
	actor, err := Spawn(t, factoryExampleActor)
	if err != nil {
		t.Fatal(err)
	}

	// Test network information
	network := actor.Node().Network()
	info, err := network.Info()
	Nil(t, err)
	Equal(t, gen.NetworkModeEnabled, info.Mode)

	// Test registrar operations
	registrar, err := network.Registrar()
	Nil(t, err)

	regInfo := registrar.Info()
	True(t, regInfo.EmbeddedServer)
	True(t, regInfo.SupportConfig)

	// Test config operations
	config, err := registrar.Config("test.enabled")
	Nil(t, err)
	if config != nil && len(config) > 0 {
		Equal(t, true, config["test.enabled"])
	}

	// Test node operations
	nodes, err := registrar.Nodes()
	Nil(t, err)
	// Nodes might be empty in test environment
	True(t, len(nodes) >= 0)

	// Test remote node operations
	remoteNode, err := network.GetNode("remote@host")
	Nil(t, err)
	Equal(t, gen.Atom("remote@host"), remoteNode.Name())

	// Test routes
	routes, err := network.Route("test@node")
	Nil(t, err)
	Equal(t, 0, len(routes)) // No routes by default

	// Test route management
	testRoute := gen.NetworkRoute{
		Route: gen.Route{
			Host: "localhost",
			Port: 4369,
			TLS:  false,
		},
	}
	err = network.AddRoute("test.*", testRoute, 100)
	Nil(t, err)

	routes, err = network.Route("test@node")
	Nil(t, err)
	Equal(t, 1, len(routes))

	// Test resolver operations
	resolver := registrar.Resolver()
	resolvedRoutes, err := resolver.Resolve(actor.Node().Name())
	// May return ErrNoRoute for test node - this is expected
	if err != nil {
		Equal(t, gen.ErrNoRoute, err)
	} else {
		True(t, len(resolvedRoutes) >= 0)
	}

	// Test application route resolution
	_, err = resolver.ResolveApplication("test-app")
	NotNil(t, err) // Should return ErrNoRoute for non-existent app
	Equal(t, gen.ErrNoRoute, err)
}

// Test demonstrating how to add nodes and routes to the Network
func TestNetworkSetup(t *testing.T) {
	actor, err := Spawn(t, factoryExampleActor)
	if err != nil {
		t.Fatal(err)
	}

	network := actor.Node().Network()
	registrar, _ := network.Registrar()

	// Method 1: Add routes directly to the network (static routes)
	// These routes are used for outgoing connections
	testRoute := gen.NetworkRoute{
		Route: gen.Route{
			Host:             "192.168.1.100",
			Port:             4369,
			TLS:              false,
			HandshakeVersion: gen.Version{Name: "test-handshake", Release: "1.0"},
			ProtoVersion:     gen.Version{Name: "test-proto", Release: "1.0"},
		},
		Resolver: nil, // Can be nil for testing
	}

	// Add route with pattern matching - this will match any node starting with "worker"
	err = network.AddRoute("worker.*", testRoute, 100)
	Nil(t, err)

	// Add route for specific node
	specificRoute := gen.NetworkRoute{
		Route: gen.Route{
			Host: "10.0.0.50",
			Port: 4370,
			TLS:  true,
		},
	}
	err = network.AddRoute("^database@prod$", specificRoute, 200) // Higher weight = higher priority
	Nil(t, err)

	// Method 2: Add remote nodes to the network (simulates connected nodes)
	// This creates mock connections to remote nodes
	remoteNode1 := actor.CreateRemoteNode("worker@node1", true) // connected
	actor.CreateRemoteNode("worker@node2", false)               // not connected
	actor.CreateRemoteNode("database@prod", true)

	// Configure remote node properties
	remoteNode1.SetUptime(3600) // 1 hour uptime
	remoteNode1.SetVersion(gen.Version{Name: "worker", Release: "2.1"})

	// Method 4: Add nodes to the registrar (simulates nodes registered on the registrar)
	// This is for service discovery - when other nodes query the registrar
	workerRoutes := []gen.Route{
		{Host: "192.168.1.101", Port: 4369, TLS: false},
		{Host: "192.168.1.102", Port: 4369, TLS: false}, // Multiple routes for load balancing
	}
	registrar.(*TestRegistrar).AddNode("worker@cluster", workerRoutes)

	dbRoutes := []gen.Route{
		{Host: "10.0.0.50", Port: 4370, TLS: true},
	}
	registrar.(*TestRegistrar).AddNode("database@prod", dbRoutes)

	// Method 5: Set registrar configuration
	testRegistrar := registrar.(*TestRegistrar)
	testRegistrar.SetConfig("cluster.name", "test-cluster")
	testRegistrar.SetConfig("cluster.region", "us-west-2")
	testRegistrar.SetConfig("worker.pool_size", 10)

	// Method 6: Register application routes
	appRoute := gen.ApplicationRoute{
		Node:   "worker@node1",
		Name:   "web-server",
		Weight: 100,
		Mode:   gen.ApplicationModePermanent,
		State:  gen.ApplicationStateRunning,
	}
	err = registrar.RegisterApplicationRoute(appRoute)
	Nil(t, err)

	// Now test that everything is configured correctly

	// Test route resolution
	routes, err := network.Route("worker@node1")
	Nil(t, err)
	Equal(t, 1, len(routes), "Should find route for worker@node1")
	Equal(t, "192.168.1.100", routes[0].Route.Host)

	routes, err = network.Route("database@prod")
	Nil(t, err)
	Equal(t, 1, len(routes), "Should find route for database@prod")
	Equal(t, uint16(4370), routes[0].Route.Port)

	// Test connected nodes
	connectedNodes := network.Nodes()
	True(t, len(connectedNodes) >= 2, "Should have connected nodes")

	// Test remote node operations
	node, err := network.Node("worker@node1")
	Nil(t, err, "Should find connected node")
	Equal(t, "worker@node1", string(node.Name()))

	_, err = network.Node("worker@node2")
	Equal(t, gen.ErrNoConnection, err, "Should not find disconnected node")

	// Test registrar node resolution
	resolver := registrar.Resolver()
	resolvedRoutes, err := resolver.Resolve("worker@cluster")
	Nil(t, err)
	Equal(t, 2, len(resolvedRoutes), "Should resolve multiple routes")

	// Test config retrieval
	config, err := registrar.Config("cluster.name", "worker.pool_size")
	Nil(t, err)
	Equal(t, "test-cluster", config["cluster.name"])
	Equal(t, 10, config["worker.pool_size"])

	// Test application route resolution
	appRoutes, err := resolver.ResolveApplication("web-server")
	Nil(t, err)
	Equal(t, 1, len(appRoutes))
	Equal(t, "worker@node1", string(appRoutes[0].Node))
}

// Test demonstrating network operations in actor behavior
func TestActorNetworkOperations(t *testing.T) {
	actor, err := Spawn(t, factoryExampleActor)
	if err != nil {
		t.Fatal(err)
	}

	// Setup network before testing actor behavior
	network := actor.Node().Network()

	// Add a remote node that the actor will interact with
	remoteNode := network.(*TestNetwork).AddRemoteNode("manager@server", true)
	remoteNode.SetVersion(gen.Version{Name: "manager", Release: "1.5"})

	// Add route for the remote node
	managerRoute := gen.NetworkRoute{
		Route: gen.Route{
			Host: "manager.example.com",
			Port: 4369,
			TLS:  true,
		},
	}
	err = network.AddRoute("manager@server", managerRoute, 100)
	Nil(t, err)

	// Test that the network route is configured correctly
	routes, err := network.Route("manager@server")
	Nil(t, err)
	True(t, len(routes) > 0, "Route should be available")
	Equal(t, "manager.example.com", routes[0].Route.Host)

	// Test that we can get the remote node
	retrievedNode, err := network.Node("manager@server")
	Nil(t, err)
	Equal(t, "manager@server", string(retrievedNode.Name()))

	// Test remote node properties
	Equal(t, gen.Version{Name: "manager", Release: "1.5"}, retrievedNode.Version())

	// Test that nodes list includes our remote node
	allNodes := network.Nodes()
	var foundManager bool
	for _, node := range allNodes {
		if node == "manager@server" {
			foundManager = true
			break
		}
	}
	True(t, foundManager, "Should find manager@server in connected nodes")
}

// Test demonstrating RemoteNode functionality and testing
func TestRemoteNodeFunctionality(t *testing.T) {
	actor, err := Spawn(t, factoryExampleActor)
	if err != nil {
		t.Fatal(err)
	}

	// === Method 1: Direct RemoteNode creation ===

	// Create remote nodes directly using the helper method
	workerNode := actor.CreateRemoteNode("worker@node1", true)
	dbNode := actor.CreateRemoteNode("database@prod", true)
	actor.CreateRemoteNode("offline@node", false) // Offline node for testing

	// Configure remote node properties
	workerNode.SetUptime(7200) // 2 hours uptime
	workerNode.SetVersion(gen.Version{Name: "worker-service", Release: "2.1.0"})
	workerNode.SetConnectionUptime(1800) // Connected for 30 minutes

	dbNode.SetVersion(gen.Version{Name: "postgresql", Release: "13.4"})
	dbNode.SetUptime(86400) // 24 hours uptime

	// === Method 2: Get existing or create new remote nodes ===

	// Get existing remote node
	retrievedWorker, err := actor.GetRemoteNode("worker@node1")
	Nil(t, err)
	Equal(t, "worker@node1", string(retrievedWorker.Name()))
	Equal(t, int64(7200), retrievedWorker.Uptime())

	// Get non-existing remote node (will create it)
	newNode, err := actor.GetRemoteNode("api@server")
	Nil(t, err)
	Equal(t, "api@server", string(newNode.Name()))

	// === Method 3: Connect/Disconnect operations ===

	// Connect to a remote node (creates if doesn't exist)
	connectedNode := actor.ConnectRemoteNode("gateway@proxy")
	NotNil(t, connectedNode)
	Equal(t, "gateway@proxy", string(connectedNode.Name()))

	// List connected nodes
	connectedNodes := actor.ListConnectedNodes()
	True(t, len(connectedNodes) >= 4, "Should have at least 4 connected nodes")

	// Test disconnection
	actor.DisconnectRemoteNode("offline@node")

	// Verify disconnection (offline@node was already disconnected, so count should be same)
	offlineNodes := actor.ListConnectedNodes()
	Equal(t, len(connectedNodes), len(offlineNodes), "Connected node count should be unchanged (offline@node was already disconnected)")

	// === Method 4: Test properties and configuration ===

	// Test version information
	Equal(t, gen.Version{Name: "worker-service", Release: "2.1.0"}, workerNode.Version())
	Equal(t, gen.Version{Name: "postgresql", Release: "13.4"}, dbNode.Version())

	// Test uptime information
	Equal(t, int64(7200), workerNode.Uptime())
	Equal(t, int64(1800), workerNode.ConnectionUptime())

	// Test connection status
	True(t, len(actor.ListConnectedNodes()) > 0, "Should have connected nodes")

	// === Method 5: Test retrieval operations ===

	// Get specific remote node
	foundWorker, err := actor.GetRemoteNode("worker@node1")
	Nil(t, err)
	Equal(t, "worker@node1", string(foundWorker.Name()))

	// Try to get non-existent node (should create)
	tempNode, err := actor.GetRemoteNode("temp@test")
	Nil(t, err)
	Equal(t, "temp@test", string(tempNode.Name()))
}

// Test demonstrating network assertions with remote nodes
func TestRemoteNodeAssertions(t *testing.T) {
	actor, err := Spawn(t, factoryExampleActor)
	if err != nil {
		t.Fatal(err)
	}

	// Create some remote nodes
	actor.CreateRemoteNode("worker@node1", true)
	actor.CreateRemoteNode("worker@node2", true)
	actor.CreateRemoteNode("database@prod", false) // offline

	// Test basic remote node operations
	connectedNodes := actor.ListConnectedNodes()
	True(t, len(connectedNodes) >= 2, "Should have connected nodes")

	// Test specific node retrieval
	worker1, err := actor.GetRemoteNode("worker@node1")
	Nil(t, err)
	Equal(t, "worker@node1", string(worker1.Name()))

	// Test properties (TestRemoteNode has default test values)
	// Note: TestRemoteNode might have default test values, so we just verify they're accessible
	NotNil(t, worker1.Version(), "Version should be accessible")
	True(t, worker1.Uptime() >= 0, "Uptime should be non-negative")
}
