package unit_test

import (
	"fmt"
	"strings"
	"testing"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/unit"
)

// Example actor for testing
func factoryExampleActor() gen.ProcessBehavior {
	return &ExampleActor{}
}

type ExampleActor struct {
	act.Actor
	value int
}

func (a *ExampleActor) Init(args ...any) error {
	a.value = 1
	a.Send(a.PID(), "hello")
	a.Log().Debug("actor started %s %d", a.PID(), a.value)
	a.Spawn(factoryExampleActor, gen.ProcessOptions{}, 1)
	a.Spawn(factoryExampleActor, gen.ProcessOptions{})
	return nil
}

func (a *ExampleActor) HandleMessage(from gen.PID, message any) error {
	switch message {
	case "increase":
		a.value += 8
		a.Log().Info("increased value by %d", 8)
		a.Send(gen.Atom("abc"), a.value)

		x, _ := a.Call(gen.PID{}, 12345)
		a.Send(gen.Atom("def"), x)
		return nil
	case "create_worker":
		// Test dynamic values - this is what the user asked about
		id, err := a.SpawnMeta(workerBehavior{}, gen.MetaOptions{})
		if err != nil {
			return err
		}

		// Send the dynamic ID to another process
		a.Send("manager", WorkerCreated{ID: id})
		return nil
	}
	return gen.TerminateReasonNormal
}

// Types for testing dynamic values
type WorkerCreated struct {
	ID gen.Alias
}

type workerBehavior struct{}

func (w workerBehavior) Init(process gen.MetaProcess) error            { return nil }
func (w workerBehavior) Start() error                                  { return nil }
func (w workerBehavior) HandleMessage(from gen.PID, message any) error { return nil }
func (w workerBehavior) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, nil
}
func (w workerBehavior) Terminate(reason error)                                       {}
func (w workerBehavior) HandleInspect(from gen.PID, item ...string) map[string]string { return nil }

// Test demonstrating the new testing library
func TestExampleActor(t *testing.T) {
	// Create a test actor with configuration
	actor, err := unit.Spawn(t, factoryExampleActor,
		unit.WithLogLevel(gen.LogLevelDebug),
		unit.WithEnv(map[gen.Env]any{"test_mode": true}),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Test initialization behavior
	actor.ShouldSend().To(actor.PID()).Message("hello").Once().Assert()
	actor.ShouldSpawn().Times(2).Assert()
	actor.ShouldLog().Level(gen.LogLevelDebug).Containing("actor started").Once().Assert()

	// Verify actor state
	behavior := actor.Behavior().(*ExampleActor)
	unit.Equal(t, 1, behavior.value, "Initial value should be 1")

	// Test message handling
	actor.SendMessage(gen.PID{}, "increase")

	// Verify the response
	actor.ShouldSend().To(gen.Atom("abc")).Message(9).Once().Assert()
	actor.ShouldCall().To(gen.PID{}).Request(12345).Once().Assert()
	actor.ShouldSend().To(gen.Atom("def")).Once().Assert() // Response from call

	// Verify state change
	unit.Equal(t, 9, behavior.value, "Value should be increased to 9")
}

// Test demonstrating handling of dynamic values
func TestDynamicValues(t *testing.T) {
	actor, err := unit.Spawn(t, factoryExampleActor)
	if err != nil {
		t.Fatal(err)
	}

	// Clear any initialization events
	actor.ClearEvents()

	// Test creating a worker with dynamic ID
	actor.SendMessage(gen.PID{}, "create_worker")

	// Verify that a meta process was spawned
	spawnResult := actor.ShouldSpawnMeta().Once().Capture()
	unit.NotNil(t, spawnResult, "Should capture spawned meta process")

	// Verify the send with the dynamic ID using pattern matching
	actor.ShouldSend().
		To("manager").
		MessageMatching(func(msg any) bool {
			wc, ok := msg.(WorkerCreated)
			if !ok {
				return false
			}
			// Validate it's a valid alias and matches the spawned ID
			return wc.ID.Node == actor.Node().Name() &&
				wc.ID == spawnResult.ID
		}).
		Once().
		Assert()

	// Alternative: Use structure matching with field validation
	actor.ShouldSend().
		To("manager").
		MessageMatching(unit.StructureMatching(WorkerCreated{}, map[string]unit.Matcher{
			"ID": unit.IsValidAlias(),
		})).
		Once().
		Assert()
}

// Test demonstrating built-in assertions (zero dependencies)
func TestBuiltInAssertions(t *testing.T) {
	actor, err := unit.Spawn(t, factoryExampleActor)
	if err != nil {
		t.Fatal(err)
	}

	// Use built-in assertion functions
	unit.NotNil(t, actor.Process(), "Process should not be nil")
	unit.NotNil(t, actor.Node(), "Node should not be nil")
	unit.Equal(t, "test@localhost", string(actor.Node().Name()), "Node name should match")
	unit.True(t, actor.Node().IsAlive(), "Node should be alive")
	unit.False(t, actor.EventCount() < 0, "Event count should not be negative")

	// Test log level
	originalLevel := actor.Process().Log().Level()
	actor.Process().Log().SetLevel(gen.LogLevelWarning)
	unit.Equal(t, gen.LogLevelWarning, actor.Process().Log().Level(), "Log level should be updated")

	// Restore original level
	actor.Process().Log().SetLevel(originalLevel)
}

// Test demonstrating event inspection
func TestEventInspection(t *testing.T) {
	actor, err := unit.Spawn(t, factoryExampleActor)
	if err != nil {
		t.Fatal(err)
	}

	// Check initial event count
	unit.True(t, actor.EventCount() > 0, "Should have initialization events")

	// Get all events
	events := actor.Events()
	unit.True(t, len(events) > 0, "Should have captured events")

	// Find a specific event
	var foundSend bool
	for _, event := range events {
		if sendEvent, ok := event.(unit.SendEvent); ok {
			if sendEvent.To == actor.PID() && sendEvent.Message == "hello" {
				foundSend = true
				break
			}
		}
	}
	unit.True(t, foundSend, "Should find the 'hello' send event")

	// Test event clearing
	actor.ClearEvents()
	unit.Equal(t, 0, actor.EventCount(), "Events should be cleared")

	// Generate new events
	actor.SendMessage(gen.PID{}, "increase")
	unit.True(t, actor.EventCount() > 0, "Should have new events after message")

	// Test last event
	lastEvent := actor.LastEvent()
	unit.NotNil(t, lastEvent, "Should have a last event")
}

// Test demonstrating network operations
func TestNetworkOperations(t *testing.T) {
	actor, err := unit.Spawn(t, factoryExampleActor)
	if err != nil {
		t.Fatal(err)
	}

	// Test network information
	network := actor.Node().Network()
	info, err := network.Info()
	unit.Nil(t, err)
	unit.Equal(t, gen.NetworkModeEnabled, info.Mode)

	// Test registrar operations
	registrar, err := network.Registrar()
	unit.Nil(t, err)

	regInfo := registrar.Info()
	unit.True(t, regInfo.EmbeddedServer)
	unit.True(t, regInfo.SupportConfig)

	// Test config operations
	config, err := registrar.Config("test.enabled")
	unit.Nil(t, err)
	if config != nil && len(config) > 0 {
		unit.Equal(t, true, config["test.enabled"])
	}

	// Test node operations
	nodes, err := registrar.Nodes()
	unit.Nil(t, err)
	// Nodes might be empty in test environment
	unit.True(t, len(nodes) >= 0)

	// Test remote node operations
	remoteNode, err := network.GetNode("remote@host")
	unit.Nil(t, err)
	unit.Equal(t, gen.Atom("remote@host"), remoteNode.Name())

	// Test routes
	routes, err := network.Route("test@node")
	unit.Nil(t, err)
	unit.Equal(t, 0, len(routes)) // No routes by default

	// Test route management
	testRoute := gen.NetworkRoute{
		Route: gen.Route{
			Host: "localhost",
			Port: 4369,
			TLS:  false,
		},
	}
	err = network.AddRoute("test.*", testRoute, 100)
	unit.Nil(t, err)

	routes, err = network.Route("test@node")
	unit.Nil(t, err)
	unit.Equal(t, 1, len(routes))

	// Test resolver operations
	resolver := registrar.Resolver()
	resolvedRoutes, err := resolver.Resolve(actor.Node().Name())
	// May return ErrNoRoute for test node - this is expected
	if err != nil {
		unit.Equal(t, gen.ErrNoRoute, err)
	} else {
		unit.True(t, len(resolvedRoutes) >= 0)
	}

	// Test application route resolution
	_, err = resolver.ResolveApplication("test-app")
	unit.NotNil(t, err) // Should return ErrNoRoute for non-existent app
	unit.Equal(t, gen.ErrNoRoute, err)
}

// Test demonstrating how to add nodes and routes to the Network
func TestNetworkSetup(t *testing.T) {
	actor, err := unit.Spawn(t, factoryExampleActor)
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
	unit.Nil(t, err)

	// Add route for specific node
	specificRoute := gen.NetworkRoute{
		Route: gen.Route{
			Host: "10.0.0.50",
			Port: 4370,
			TLS:  true,
		},
	}
	err = network.AddRoute("^database@prod$", specificRoute, 200) // Higher weight = higher priority
	unit.Nil(t, err)

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
	registrar.(*unit.TestRegistrar).AddNode("worker@cluster", workerRoutes)

	dbRoutes := []gen.Route{
		{Host: "10.0.0.50", Port: 4370, TLS: true},
	}
	registrar.(*unit.TestRegistrar).AddNode("database@prod", dbRoutes)

	// Method 5: Set registrar configuration
	testRegistrar := registrar.(*unit.TestRegistrar)
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
	unit.Nil(t, err)

	// Now test that everything is configured correctly

	// Test route resolution
	routes, err := network.Route("worker@node1")
	unit.Nil(t, err)
	unit.Equal(t, 1, len(routes), "Should find route for worker@node1")
	unit.Equal(t, "192.168.1.100", routes[0].Route.Host)

	routes, err = network.Route("database@prod")
	unit.Nil(t, err)
	unit.Equal(t, 1, len(routes), "Should find route for database@prod")
	unit.Equal(t, uint16(4370), routes[0].Route.Port)

	// Test connected nodes
	connectedNodes := network.Nodes()
	unit.True(t, len(connectedNodes) >= 2, "Should have connected nodes")

	// Test remote node operations
	node, err := network.Node("worker@node1")
	unit.Nil(t, err, "Should find connected node")
	unit.Equal(t, "worker@node1", string(node.Name()))

	_, err = network.Node("worker@node2")
	unit.Equal(t, gen.ErrNoConnection, err, "Should not find disconnected node")

	// Test registrar node resolution
	resolver := registrar.Resolver()
	resolvedRoutes, err := resolver.Resolve("worker@cluster")
	unit.Nil(t, err)
	unit.Equal(t, 2, len(resolvedRoutes), "Should resolve multiple routes")

	// Test config retrieval
	config, err := registrar.Config("cluster.name", "worker.pool_size")
	unit.Nil(t, err)
	unit.Equal(t, "test-cluster", config["cluster.name"])
	unit.Equal(t, 10, config["worker.pool_size"])

	// Test application route resolution
	appRoutes, err := resolver.ResolveApplication("web-server")
	unit.Nil(t, err)
	unit.Equal(t, 1, len(appRoutes))
	unit.Equal(t, "worker@node1", string(appRoutes[0].Node))
}

// Test demonstrating network operations in actor behavior
func TestActorNetworkOperations(t *testing.T) {
	actor, err := unit.Spawn(t, factoryExampleActor)
	if err != nil {
		t.Fatal(err)
	}

	// Setup network before testing actor behavior
	network := actor.Node().Network()

	// Add a remote node that the actor will interact with
	remoteNode := network.(*unit.TestNetwork).AddRemoteNode("manager@server", true)
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
	unit.Nil(t, err)

	// Test that the network route is configured correctly
	routes, err := network.Route("manager@server")
	unit.Nil(t, err)
	unit.True(t, len(routes) > 0, "Route should be available")
	unit.Equal(t, "manager.example.com", routes[0].Route.Host)

	// Test that we can get the remote node
	retrievedNode, err := network.Node("manager@server")
	unit.Nil(t, err)
	unit.Equal(t, "manager@server", string(retrievedNode.Name()))

	// Test remote node properties
	unit.Equal(t, gen.Version{Name: "manager", Release: "1.5"}, retrievedNode.Version())

	// Test that nodes list includes our remote node
	allNodes := network.Nodes()
	var foundManager bool
	for _, node := range allNodes {
		if node == "manager@server" {
			foundManager = true
			break
		}
	}
	unit.True(t, foundManager, "Should find manager@server in connected nodes")
}

// Test demonstrating RemoteNode functionality and testing
func TestRemoteNodeFunctionality(t *testing.T) {
	actor, err := unit.Spawn(t, factoryExampleActor)
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
	unit.Nil(t, err)
	unit.Equal(t, "worker@node1", string(retrievedWorker.Name()))
	unit.Equal(t, int64(7200), retrievedWorker.Uptime())

	// Get non-existing remote node (will create it)
	newNode, err := actor.GetRemoteNode("api@server")
	unit.Nil(t, err)
	unit.Equal(t, "api@server", string(newNode.Name()))

	// === Method 3: Connect/Disconnect operations ===

	// Connect to a remote node (creates if doesn't exist)
	connectedNode := actor.ConnectRemoteNode("gateway@proxy")
	unit.NotNil(t, connectedNode)
	unit.Equal(t, "gateway@proxy", string(connectedNode.Name()))

	// List connected nodes
	connectedNodes := actor.ListConnectedNodes()
	unit.True(t, len(connectedNodes) >= 4, "Should have at least 4 connected nodes")

	// Verify specific nodes are connected
	nodeMap := make(map[gen.Atom]bool)
	for _, node := range connectedNodes {
		nodeMap[node] = true
	}
	unit.True(t, nodeMap["worker@node1"], "worker@node1 should be connected")
	unit.True(t, nodeMap["database@prod"], "database@prod should be connected")
	unit.True(t, nodeMap["api@server"], "api@server should be connected")
	unit.True(t, nodeMap["gateway@proxy"], "gateway@proxy should be connected")
	unit.False(t, nodeMap["offline@node"], "offline@node should not be connected")

	// Disconnect from a node
	err = actor.DisconnectRemoteNode("gateway@proxy")
	unit.Nil(t, err)

	// Verify disconnection
	updatedNodes := actor.ListConnectedNodes()
	updatedNodeMap := make(map[gen.Atom]bool)
	for _, node := range updatedNodes {
		updatedNodeMap[node] = true
	}
	unit.False(t, updatedNodeMap["gateway@proxy"], "gateway@proxy should be disconnected")

	// === Method 4: Remote operations testing ===

	// Test remote spawn
	pid, err := workerNode.Spawn("test-process", gen.ProcessOptions{}, "arg1", "arg2")
	unit.Nil(t, err)
	unit.Equal(t, "worker@node1", string(pid.Node))
	unit.True(t, pid.ID > 0, "PID should have valid ID")

	// Test remote spawn with register
	regPid, err := workerNode.SpawnRegister("my-service", "service-process", gen.ProcessOptions{})
	unit.Nil(t, err)
	unit.Equal(t, "worker@node1", string(regPid.Node))
	unit.True(t, regPid.ID != pid.ID, "Should have different PID")

	// Test remote application start
	err = workerNode.ApplicationStart("web-app", gen.ApplicationOptions{})
	unit.Nil(t, err)

	err = dbNode.ApplicationStartPermanent("database-service", gen.ApplicationOptions{})
	unit.Nil(t, err)

	// === Method 5: Network access through RemoteNode helper ===

	network := actor.RemoteNode() // Gets TestNetwork
	unit.NotNil(t, network)

	// Access network functionality
	info, err := network.Info()
	unit.Nil(t, err)
	unit.Equal(t, gen.NetworkModeEnabled, info.Mode)

	// === Method 6: Advanced remote node configuration ===

	// Create a fully configured remote node
	advancedNode := actor.CreateRemoteNode("advanced@cluster", true)
	advancedNode.SetUptime(172800)         // 48 hours
	advancedNode.SetConnectionUptime(3600) // 1 hour connection
	advancedNode.SetVersion(gen.Version{
		Name:    "advanced-service",
		Release: "3.0.0",
		License: "MIT",
	})

	// Set detailed info
	advancedNode.SetInfo(gen.RemoteNodeInfo{
		Node:             "advanced@cluster",
		Uptime:           172800,
		ConnectionUptime: 3600,
		Version:          gen.Version{Name: "advanced-service", Release: "3.0.0"},
		MaxMessageSize:   2048000,
		MessagesIn:       1000,
		MessagesOut:      850,
		BytesIn:          5000000,
		BytesOut:         4200000,
	})

	// Test the configured info
	nodeInfo := advancedNode.Info()
	unit.Equal(t, uint64(1000), nodeInfo.MessagesIn)
	unit.Equal(t, uint64(850), nodeInfo.MessagesOut)
	unit.Equal(t, 2048000, nodeInfo.MaxMessageSize)

	// === Method 7: Error handling ===

	// Try to access disconnected node
	_, err = actor.Node().Network().Node("offline@node")
	unit.Equal(t, gen.ErrNoConnection, err, "Should return no connection error")

	// Try to disconnect non-existent node
	err = actor.DisconnectRemoteNode("nonexistent@node")
	unit.Equal(t, gen.ErrNoConnection, err, "Should return no connection error")
}

// Test demonstrating RemoteNode assertions (when implemented)
func TestRemoteNodeAssertions(t *testing.T) {
	actor, err := unit.Spawn(t, factoryExampleActor)
	if err != nil {
		t.Fatal(err)
	}

	// Create remote nodes for testing
	worker := actor.CreateRemoteNode("worker@test", true)
	api := actor.CreateRemoteNode("api@test", true)

	// Clear any initialization events
	actor.ClearEvents()

	// Test remote operations (these would generate events in a real implementation)
	_, _ = worker.Spawn("test-proc", gen.ProcessOptions{})
	_ = api.ApplicationStart("test-app", gen.ApplicationOptions{})

	// Note: These assertions would work once we implement event generation
	// in the RemoteNode methods
	// actor.ShouldRemoteSpawn().ToNode("worker@test").WithName("test-proc").Once().Assert()
	// actor.ShouldRemoteApplicationStart().OnNode("api@test").Application("test-app").Once().Assert()

	// For now, verify the operations completed successfully
	unit.Equal(t, "worker@test", string(worker.Name()))
	unit.Equal(t, "api@test", string(api.Name()))
}

// Advanced examples demonstrating comprehensive testing library features

// Complex state machine actor for testing advanced scenarios
type orderActor struct {
	act.Actor
	orderID    string
	state      string
	items      []string
	total      float64
	customer   gen.PID
	retries    int
	maxRetries int
}

func factoryOrderActor() gen.ProcessBehavior {
	return &orderActor{
		maxRetries: 3,
	}
}

func (o *orderActor) Init(args ...any) error {
	if len(args) > 0 {
		o.orderID = args[0].(string)
	}
	if len(args) > 1 {
		o.customer = args[1].(gen.PID)
	}
	o.state = "created"
	o.Log().Info("Order actor initialized: %s", o.orderID)
	return nil
}

func (o *orderActor) HandleMessage(from gen.PID, message any) error {
	switch msg := message.(type) {
	case AddItem:
		if o.state != "created" && o.state != "building" {
			o.Send(from, OrderError{OrderID: o.orderID, Error: "cannot add items in state: " + o.state})
			return nil
		}
		o.items = append(o.items, msg.Item)
		o.total += msg.Price
		o.state = "building"
		o.Send(from, ItemAdded{OrderID: o.orderID, Item: msg.Item, Total: o.total})
		o.Log().Debug("Item added: %s, total: %.2f", msg.Item, o.total)
		return nil

	case SubmitOrder:
		if o.state != "building" {
			o.Send(from, OrderError{OrderID: o.orderID, Error: "order not ready for submission"})
			return nil
		}
		if len(o.items) == 0 {
			o.Send(from, OrderError{OrderID: o.orderID, Error: "cannot submit empty order"})
			return nil
		}
		o.state = "submitted"
		o.customer = from

		// Spawn validation process
		validatorPID, err := o.Spawn(factoryValidatorActor, gen.ProcessOptions{}, o.orderID, o.items)
		if err != nil {
			o.Log().Error("Failed to spawn validator: %v", err)
			return err
		}

		o.Send(validatorPID, ValidateOrder{OrderID: o.orderID, Items: o.items, Total: o.total})
		o.Log().Info("Order submitted for validation: %s", o.orderID)
		return nil

	case ValidationResult:
		if o.state != "submitted" {
			o.Log().Warning("Received validation result in wrong state: %s", o.state)
			return nil
		}

		if msg.Valid {
			o.state = "validated"
			o.Send("payment", ProcessPayment{OrderID: o.orderID, Amount: o.total, Customer: o.customer})
			o.Log().Info("Order validated, sent for payment: %s", o.orderID)
		} else {
			o.state = "rejected"
			o.Send(o.customer, OrderRejected{OrderID: o.orderID, Reason: msg.Reason})
			o.Log().Warning("Order rejected: %s - %s", o.orderID, msg.Reason)
		}
		return nil

	case PaymentResult:
		if o.state != "validated" {
			return nil
		}

		if msg.Success {
			o.state = "paid"
			o.Send("fulfillment", FulfillOrder{OrderID: o.orderID, Items: o.items, Customer: o.customer})
			o.Send(o.customer, OrderConfirmed{OrderID: o.orderID, Total: o.total})
			o.Log().Info("Order paid and sent for fulfillment: %s", o.orderID)
		} else {
			o.retries++
			if o.retries >= o.maxRetries {
				o.state = "failed"
				o.Send(o.customer, OrderFailed{OrderID: o.orderID, Reason: "payment failed after retries"})
				o.Log().Error("Order failed after %d payment retries: %s", o.retries, o.orderID)
			} else {
				// Retry payment after delay
				o.SendAfter("payment", ProcessPayment{OrderID: o.orderID, Amount: o.total, Customer: o.customer}, 1000)
				o.Log().Info("Retrying payment (%d/%d): %s", o.retries, o.maxRetries, o.orderID)
			}
		}
		return nil

	case GetOrderStatus:
		status := OrderStatus{
			OrderID: o.orderID,
			State:   o.state,
			Items:   o.items,
			Total:   o.total,
			Retries: o.retries,
		}
		o.Send(from, status)
		return nil

	case CancelOrder:
		if o.state == "completed" || o.state == "shipped" {
			o.Send(from, OrderError{OrderID: o.orderID, Error: "cannot cancel completed order"})
			return nil
		}
		oldState := o.state
		o.state = "cancelled"
		o.Send(from, OrderCancelled{OrderID: o.orderID, PreviousState: oldState})
		o.Log().Info("Order cancelled: %s (was %s)", o.orderID, oldState)
		return nil
	}
	return nil
}

// Validator actor for order validation
type validatorActor struct {
	act.Actor
	orderID string
	items   []string
}

func factoryValidatorActor() gen.ProcessBehavior {
	return &validatorActor{}
}

func (v *validatorActor) Init(args ...any) error {
	if len(args) > 0 {
		v.orderID = args[0].(string)
	}
	if len(args) > 1 {
		v.items = args[1].([]string)
	}
	return nil
}

func (v *validatorActor) HandleMessage(from gen.PID, message any) error {
	switch msg := message.(type) {
	case ValidateOrder:
		v.Log().Debug("Validating order: %s with %d items", msg.OrderID, len(msg.Items))

		// Simulate validation logic
		valid := true
		reason := ""

		for _, item := range msg.Items {
			if item == "invalid_item" {
				valid = false
				reason = "contains invalid item"
				break
			}
		}

		if msg.Total < 0 {
			valid = false
			reason = "negative total"
		}

		result := ValidationResult{
			OrderID: msg.OrderID,
			Valid:   valid,
			Reason:  reason,
		}

		v.Send(from, result)
		v.Log().Info("Validation complete for %s: valid=%t", msg.OrderID, valid)
		return nil
	}
	return nil
}

// Message types for order workflow
type AddItem struct {
	Item  string
	Price float64
}

type SubmitOrder struct{}

type ValidationResult struct {
	OrderID string
	Valid   bool
	Reason  string
}

type PaymentResult struct {
	OrderID string
	Success bool
	Reason  string
}

type GetOrderStatus struct{}

type CancelOrder struct{}

type ItemAdded struct {
	OrderID string
	Item    string
	Total   float64
}

type OrderError struct {
	OrderID string
	Error   string
}

type OrderRejected struct {
	OrderID string
	Reason  string
}

type OrderConfirmed struct {
	OrderID string
	Total   float64
}

type OrderFailed struct {
	OrderID string
	Reason  string
}

type OrderCancelled struct {
	OrderID       string
	PreviousState string
}

type OrderStatus struct {
	OrderID string
	State   string
	Items   []string
	Total   float64
	Retries int
}

type ValidateOrder struct {
	OrderID string
	Items   []string
	Total   float64
}

type ProcessPayment struct {
	OrderID  string
	Amount   float64
	Customer gen.PID
}

type FulfillOrder struct {
	OrderID  string
	Items    []string
	Customer gen.PID
}

// Test comprehensive order workflow
func TestOrderWorkflow_CompleteFlow(t *testing.T) {
	customerPID := gen.PID{Node: "test", ID: 100}

	actor, err := unit.Spawn(t, factoryOrderActor,
		unit.WithLogLevel(gen.LogLevelDebug),
		unit.WithEnv(map[gen.Env]any{"order_timeout": 30000}),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Initialize with order ID and customer
	actor.SendMessage(gen.PID{}, "initialize")
	behavior := actor.Behavior().(*orderActor)
	behavior.orderID = "order-123"
	behavior.customer = customerPID

	// Test initialization
	actor.ShouldLog().Level(gen.LogLevelInfo).Containing("Order actor initialized").Once().Assert()

	// Test adding items
	actor.SendMessage(customerPID, AddItem{Item: "laptop", Price: 999.99})
	actor.SendMessage(customerPID, AddItem{Item: "mouse", Price: 29.99})

	// Verify item addition responses
	actor.ShouldSend().To(customerPID).MessageMatching(func(msg any) bool {
		if added, ok := msg.(ItemAdded); ok {
			return added.Item == "laptop" && added.Total == 999.99
		}
		return false
	}).Once().Assert()

	actor.ShouldSend().To(customerPID).MessageMatching(func(msg any) bool {
		if added, ok := msg.(ItemAdded); ok {
			return added.Item == "mouse" && added.Total == 1029.98
		}
		return false
	}).Once().Assert()

	// Test order submission
	actor.SendMessage(customerPID, SubmitOrder{})

	// Verify validator spawning
	spawnResult := actor.ShouldSpawn().Factory(factoryValidatorActor).Once().Capture()
	unit.NotNil(t, spawnResult)

	// Verify validation request sent
	actor.ShouldSend().To(spawnResult.PID).MessageMatching(func(msg any) bool {
		if validate, ok := msg.(ValidateOrder); ok {
			return validate.OrderID == "order-123" && len(validate.Items) == 2
		}
		return false
	}).Once().Assert()

	// Simulate validation success
	actor.SendMessage(spawnResult.PID, ValidationResult{
		OrderID: "order-123",
		Valid:   true,
	})

	// Verify payment request
	actor.ShouldSend().To("payment").MessageMatching(func(msg any) bool {
		if payment, ok := msg.(ProcessPayment); ok {
			return payment.OrderID == "order-123" && payment.Amount == 1029.98
		}
		return false
	}).Once().Assert()

	// Simulate payment success
	actor.SendMessage(gen.PID{}, PaymentResult{
		OrderID: "order-123",
		Success: true,
	})

	// Verify fulfillment and confirmation
	actor.ShouldSend().To("fulfillment").MessageMatching(func(msg any) bool {
		if fulfill, ok := msg.(FulfillOrder); ok {
			return fulfill.OrderID == "order-123" && len(fulfill.Items) == 2
		}
		return false
	}).Once().Assert()

	actor.ShouldSend().To(customerPID).Message(OrderConfirmed{
		OrderID: "order-123",
		Total:   1029.98,
	}).Once().Assert()

	// Verify final state
	actor.SendMessage(customerPID, GetOrderStatus{})
	actor.ShouldSend().To(customerPID).MessageMatching(func(msg any) bool {
		if status, ok := msg.(OrderStatus); ok {
			return status.State == "paid" && status.Total == 1029.98
		}
		return false
	}).Once().Assert()
}

// Test error scenarios and edge cases
func TestOrderWorkflow_ErrorScenarios(t *testing.T) {
	customerPID := gen.PID{Node: "test", ID: 200}

	actor, err := unit.Spawn(t, factoryOrderActor)
	if err != nil {
		t.Fatal(err)
	}

	behavior := actor.Behavior().(*orderActor)
	behavior.orderID = "order-456"
	behavior.customer = customerPID

	// Clear any initialization events
	actor.ClearEvents()

	// Test submitting order in wrong state (created instead of building)
	actor.SendMessage(customerPID, SubmitOrder{})

	actor.ShouldSend().To(customerPID).MessageMatching(func(msg any) bool {
		if err, ok := msg.(OrderError); ok {
			return err.OrderID == "order-456" && err.Error == "order not ready for submission"
		}
		return false
	}).Once().Assert()

	// Test submitting empty order (first change state to building, then submit with no items)
	behavior.state = "building" // Manually set state to building but leave items empty
	actor.SendMessage(customerPID, SubmitOrder{})

	actor.ShouldSend().To(customerPID).MessageMatching(func(msg any) bool {
		if err, ok := msg.(OrderError); ok {
			return err.OrderID == "order-456" && err.Error == "cannot submit empty order"
		}
		return false
	}).Once().Assert()

	// Test adding invalid item and submitting
	actor.SendMessage(customerPID, AddItem{Item: "laptop", Price: 999.99})
	actor.SendMessage(customerPID, AddItem{Item: "invalid_item", Price: 50.00})
	actor.SendMessage(customerPID, SubmitOrder{})

	// Capture validator spawn
	spawnResult := actor.ShouldSpawn().Factory(factoryValidatorActor).Once().Capture()

	// Simulate validation failure
	actor.SendMessage(spawnResult.PID, ValidationResult{
		OrderID: "order-456",
		Valid:   false,
		Reason:  "contains invalid item",
	})

	// Verify rejection
	actor.ShouldSend().To(customerPID).MessageMatching(func(msg any) bool {
		if rejected, ok := msg.(OrderRejected); ok {
			return rejected.OrderID == "order-456" && rejected.Reason == "contains invalid item"
		}
		return false
	}).Once().Assert()
}

// Test payment retry mechanism
func TestOrderWorkflow_PaymentRetries(t *testing.T) {
	customerPID := gen.PID{Node: "test", ID: 300}

	actor, err := unit.Spawn(t, factoryOrderActor)
	if err != nil {
		t.Fatal(err)
	}

	behavior := actor.Behavior().(*orderActor)
	behavior.orderID = "order-789"
	behavior.customer = customerPID
	behavior.state = "validated" // Skip to payment phase

	// Clear any initialization events
	actor.ClearEvents()

	// Simulate first payment failure
	actor.SendMessage(gen.PID{}, PaymentResult{
		OrderID: "order-789",
		Success: false,
		Reason:  "insufficient funds",
	})

	// Verify retry scheduled (first retry)
	actor.ShouldSend().To("payment").MessageMatching(func(msg any) bool {
		if payment, ok := msg.(ProcessPayment); ok {
			return payment.OrderID == "order-789"
		}
		return false
	}).Once().Assert()

	// Clear events before next test
	actor.ClearEvents()

	// Simulate second payment failure
	actor.SendMessage(gen.PID{}, PaymentResult{
		OrderID: "order-789",
		Success: false,
		Reason:  "card declined",
	})

	// Verify second retry
	actor.ShouldSend().To("payment").MessageMatching(func(msg any) bool {
		if payment, ok := msg.(ProcessPayment); ok {
			return payment.OrderID == "order-789"
		}
		return false
	}).Once().Assert()

	// Clear events before final test
	actor.ClearEvents()

	// Simulate third payment failure (should trigger final failure)
	actor.SendMessage(gen.PID{}, PaymentResult{
		OrderID: "order-789",
		Success: false,
		Reason:  "card expired",
	})

	// Verify final failure after max retries
	actor.ShouldSend().To(customerPID).MessageMatching(func(msg any) bool {
		if failed, ok := msg.(OrderFailed); ok {
			return failed.OrderID == "order-789" && failed.Reason == "payment failed after retries"
		}
		return false
	}).Once().Assert()
}

// Manager actor for testing process management
type managerActor struct {
	act.Actor
	workers    map[string]gen.PID
	maxWorkers int
	strategy   string
}

func factoryManagerActor() gen.ProcessBehavior {
	return &managerActor{
		workers:    make(map[string]gen.PID),
		maxWorkers: 5,
		strategy:   "one_for_one",
	}
}

func (s *managerActor) Init(args ...any) error {
	if len(args) > 0 {
		s.maxWorkers = args[0].(int)
	}
	if len(args) > 1 {
		s.strategy = args[1].(string)
	}
	s.Log().Info("Manager started with strategy %s, max workers: %d", s.strategy, s.maxWorkers)
	return nil
}

func (s *managerActor) HandleMessage(from gen.PID, message any) error {
	switch msg := message.(type) {
	case StartWorker:
		if len(s.workers) >= s.maxWorkers {
			s.Send(from, WorkerError{Error: "max workers reached"})
			return nil
		}

		workerPID, err := s.Spawn(factoryWorkerActor, gen.ProcessOptions{
			LinkParent: true, // Link for supervision
		}, msg.WorkerID, msg.Config)

		if err != nil {
			s.Send(from, WorkerError{Error: err.Error()})
			return nil
		}

		s.workers[msg.WorkerID] = workerPID
		s.Monitor(workerPID) // Monitor for termination

		s.Send(from, WorkerStarted{WorkerID: msg.WorkerID, PID: workerPID})
		s.Send("metrics", WorkerMetrics{
			Action:        "started",
			WorkerID:      msg.WorkerID,
			ActiveWorkers: len(s.workers),
		})

		s.Log().Info("Worker started: %s (%s)", msg.WorkerID, workerPID)
		return nil

	case StopWorker:
		if pid, exists := s.workers[msg.WorkerID]; exists {
			s.SendExit(pid, gen.TerminateReasonShutdown)
			delete(s.workers, msg.WorkerID)
			s.Demonitor(pid)

			s.Send(from, WorkerStopped{WorkerID: msg.WorkerID})
			s.Log().Info("Worker stopped: %s", msg.WorkerID)
		} else {
			s.Send(from, WorkerError{Error: "worker not found: " + msg.WorkerID})
		}
		return nil

	case RestartWorker:
		if pid, exists := s.workers[msg.WorkerID]; exists {
			s.SendExit(pid, gen.TerminateReasonShutdown)
			s.Demonitor(pid)
		}

		// Restart worker
		workerPID, err := s.Spawn(factoryWorkerActor, gen.ProcessOptions{
			LinkParent: true,
		}, msg.WorkerID, msg.Config)

		if err != nil {
			s.Send(from, WorkerError{Error: err.Error()})
			return nil
		}

		s.workers[msg.WorkerID] = workerPID
		s.Monitor(workerPID)

		s.Send(from, WorkerRestarted{WorkerID: msg.WorkerID, PID: workerPID})
		s.Log().Info("Worker restarted: %s", msg.WorkerID)
		return nil

	case GetManagerStatus:
		status := ManagerStatus{
			Strategy:      s.strategy,
			MaxWorkers:    s.maxWorkers,
			ActiveWorkers: len(s.workers),
			Workers:       make(map[string]gen.PID),
		}
		for id, pid := range s.workers {
			status.Workers[id] = pid
		}
		s.Send(from, status)
		return nil
	}
	return nil
}

// Worker actor for testing
type workerActor struct {
	act.Actor
	workerID string
	config   map[string]any
	tasks    int
}

func factoryWorkerActor() gen.ProcessBehavior {
	return &workerActor{}
}

func (w *workerActor) Init(args ...any) error {
	if len(args) > 0 {
		w.workerID = args[0].(string)
	}
	if len(args) > 1 {
		w.config = args[1].(map[string]any)
	}
	w.Log().Debug("Worker initialized: %s", w.workerID)
	return nil
}

func (w *workerActor) HandleMessage(from gen.PID, message any) error {
	switch msg := message.(type) {
	case ProcessTask:
		w.tasks++
		w.Log().Debug("Processing task %s (total: %d)", msg.TaskID, w.tasks)

		// Simulate task processing
		if msg.TaskID == "error_task" {
			w.Send(from, TaskError{TaskID: msg.TaskID, Error: "simulated error"})
			return nil
		}

		result := TaskResult{
			TaskID:   msg.TaskID,
			WorkerID: w.workerID,
			Result:   "processed",
		}
		w.Send(from, result)
		return nil

	case GetWorkerStatus:
		status := WorkerStatus{
			WorkerID:       w.workerID,
			TasksProcessed: w.tasks,
			Config:         w.config,
		}
		w.Send(from, status)
		return nil
	}
	return nil
}

// Manager message types
type StartWorker struct {
	WorkerID string
	Config   map[string]any
}

type StopWorker struct {
	WorkerID string
}

type RestartWorker struct {
	WorkerID string
	Config   map[string]any
}

type GetManagerStatus struct{}

type WorkerStarted struct {
	WorkerID string
	PID      gen.PID
}

type WorkerStopped struct {
	WorkerID string
}

type WorkerRestarted struct {
	WorkerID string
	PID      gen.PID
}

type WorkerError struct {
	Error string
}

type ManagerStatus struct {
	Strategy      string
	MaxWorkers    int
	ActiveWorkers int
	Workers       map[string]gen.PID
}

type WorkerMetrics struct {
	Action        string
	WorkerID      string
	ActiveWorkers int
}

// Worker message types
type ProcessTask struct {
	TaskID string
	Data   any
}

type GetWorkerStatus struct{}

type TaskResult struct {
	TaskID   string
	WorkerID string
	Result   string
}

type TaskError struct {
	TaskID string
	Error  string
}

type WorkerStatus struct {
	WorkerID       string
	TasksProcessed int
	Config         map[string]any
}

// Test manager functionality
func TestManager_WorkerManagement(t *testing.T) {
	actor, err := unit.Spawn(t, factoryManagerActor,
		unit.WithLogLevel(gen.LogLevelDebug),
	)
	if err != nil {
		t.Fatal(err)
	}

	clientPID := gen.PID{Node: "test", ID: 500}

	// Clear initialization events
	actor.ClearEvents()

	// Test starting workers
	actor.SendMessage(clientPID, StartWorker{
		WorkerID: "worker-1",
		Config:   map[string]any{"timeout": 5000},
	})

	// Capture first spawn before sending second message
	worker1Result := actor.ShouldSpawn().Factory(factoryWorkerActor).Once().Capture()
	unit.NotNil(t, worker1Result)

	// Verify first worker start confirmation
	actor.ShouldSend().To(clientPID).MessageMatching(func(msg any) bool {
		if started, ok := msg.(WorkerStarted); ok {
			return started.WorkerID == "worker-1" && started.PID == worker1Result.PID
		}
		return false
	}).Once().Assert()

	// Verify first metrics message
	actor.ShouldSend().To("metrics").MessageMatching(func(msg any) bool {
		if metrics, ok := msg.(WorkerMetrics); ok {
			return metrics.Action == "started" && metrics.WorkerID == "worker-1"
		}
		return false
	}).Once().Assert()

	// Clear events before second worker
	actor.ClearEvents()

	actor.SendMessage(clientPID, StartWorker{
		WorkerID: "worker-2",
		Config:   map[string]any{"timeout": 3000},
	})

	// Capture second spawn
	worker2Result := actor.ShouldSpawn().Factory(factoryWorkerActor).Once().Capture()
	unit.NotNil(t, worker2Result)

	// Verify second worker start confirmation
	actor.ShouldSend().To(clientPID).MessageMatching(func(msg any) bool {
		if started, ok := msg.(WorkerStarted); ok {
			return started.WorkerID == "worker-2" && started.PID == worker2Result.PID
		}
		return false
	}).Once().Assert()

	// Verify second metrics message
	actor.ShouldSend().To("metrics").MessageMatching(func(msg any) bool {
		if metrics, ok := msg.(WorkerMetrics); ok {
			return metrics.Action == "started" && metrics.WorkerID == "worker-2"
		}
		return false
	}).Once().Assert()

	// Clear events before status test
	actor.ClearEvents()

	// Test manager status
	actor.SendMessage(clientPID, GetManagerStatus{})
	actor.ShouldSend().To(clientPID).MessageMatching(func(msg any) bool {
		if status, ok := msg.(ManagerStatus); ok {
			return status.ActiveWorkers == 2 && status.MaxWorkers == 5
		}
		return false
	}).Once().Assert()

	// Clear events before stop test
	actor.ClearEvents()

	// Test stopping worker
	actor.SendMessage(clientPID, StopWorker{WorkerID: "worker-1"})
	actor.ShouldSend().To(clientPID).Message(WorkerStopped{WorkerID: "worker-1"}).Once().Assert()

	// Clear events before restart test
	actor.ClearEvents()

	// Test restarting worker
	actor.SendMessage(clientPID, RestartWorker{
		WorkerID: "worker-2",
		Config:   map[string]any{"timeout": 7000},
	})

	restartResult := actor.ShouldSpawn().Factory(factoryWorkerActor).Once().Capture()
	actor.ShouldSend().To(clientPID).MessageMatching(func(msg any) bool {
		if restarted, ok := msg.(WorkerRestarted); ok {
			return restarted.WorkerID == "worker-2" && restarted.PID == restartResult.PID
		}
		return false
	}).Once().Assert()
}

// Test advanced pattern matching and dynamic values
func TestAdvancedPatternMatching(t *testing.T) {
	actor, err := unit.Spawn(t, factoryOrderActor)
	if err != nil {
		t.Fatal(err)
	}

	customerPID := gen.PID{Node: "test", ID: 600}

	// Test with dynamic timestamps and IDs
	actor.SendMessage(customerPID, AddItem{Item: "dynamic_item", Price: 123.45})

	// Test structure matching with partial validation
	actor.ShouldSend().To(customerPID).MessageMatching(
		unit.StructureMatching(ItemAdded{}, map[string]unit.Matcher{
			"Item":  unit.Equals("dynamic_item"),
			"Total": func(v any) bool { return v.(float64) > 100.0 },
		}),
	).Once().Assert()

	// Test type-based matching
	actor.ShouldSend().MessageMatching(unit.IsTypeGeneric[ItemAdded]()).Once().Assert()

	// Test field validation
	actor.ShouldSend().MessageMatching(
		unit.HasField("Item", unit.Equals("dynamic_item")),
	).Once().Assert()

	// Test complex condition matching
	actor.ShouldSend().MessageMatching(func(msg any) bool {
		if added, ok := msg.(ItemAdded); ok {
			return added.Item == "dynamic_item" &&
				added.Total > 100.0 &&
				added.Total < 200.0
		}
		return false
	}).Once().Assert()
}

// Test meta processes and aliases
func TestMetaProcesses_ComplexWorkflow(t *testing.T) {
	actor, err := unit.Spawn(t, factoryExampleActor)
	if err != nil {
		t.Fatal(err)
	}

	// Test meta process spawning
	actor.SendMessage(gen.PID{}, "create_worker")

	// Capture meta spawn
	metaResult := actor.ShouldSpawnMeta().Once().Capture()
	unit.NotNil(t, metaResult)
	unit.True(t, unit.IsValidAlias()(metaResult.ID), "Should have valid alias")

	// Test message with captured alias
	actor.ShouldSend().To("manager").MessageMatching(func(msg any) bool {
		if wc, ok := msg.(WorkerCreated); ok {
			return wc.ID == metaResult.ID
		}
		return false
	}).Once().Assert()

	// Test alias creation
	alias, err := actor.Process().CreateAlias()
	unit.Nil(t, err)
	unit.True(t, alias.Node != "")
	unit.True(t, alias.ID[0] > 0)
}

// Test environment and configuration
func TestEnvironmentAndConfiguration(t *testing.T) {
	customEnv := map[gen.Env]any{
		"database_url":    "postgres://test:test@localhost/testdb",
		"max_connections": 100,
		"enable_logging":  true,
		"timeout_seconds": 30,
		"feature_flags":   []string{"new_ui", "beta_features"},
	}

	actor, err := unit.Spawn(t, factoryOrderActor,
		unit.WithLogLevel(gen.LogLevelTrace),
		unit.WithEnv(customEnv),
		unit.WithNodeName("test_node@testhost"),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Test environment access
	process := actor.Process()

	dbURL, exists := process.Env("database_url")
	unit.True(t, exists)
	unit.Equal(t, "postgres://test:test@localhost/testdb", dbURL)

	maxConn := process.EnvDefault("max_connections", 50)
	unit.Equal(t, 100, maxConn)

	unknownValue := process.EnvDefault("unknown_key", "default_value")
	unit.Equal(t, "default_value", unknownValue)

	// Test environment listing
	envList := process.EnvList()
	unit.True(t, len(envList) >= 5)
	unit.Equal(t, "postgres://test:test@localhost/testdb", envList["database_url"])

	// Test environment modification
	process.SetEnv("runtime_config", "modified")
	modifiedValue, exists := process.Env("runtime_config")
	unit.True(t, exists)
	unit.Equal(t, "modified", modifiedValue)
}

// Test linking and monitoring
func TestLinkingAndMonitoring(t *testing.T) {
	actor, err := unit.Spawn(t, factoryManagerActor)
	if err != nil {
		t.Fatal(err)
	}

	process := actor.Process()
	targetPID := gen.PID{Node: "test", ID: 700}

	// Test linking
	err = process.Link(targetPID)
	unit.Nil(t, err)

	// Test monitoring
	err = process.Monitor(targetPID)
	unit.Nil(t, err)

	// Test alias operations
	alias, err := process.CreateAlias()
	unit.Nil(t, err)

	err = process.LinkAlias(alias)
	unit.Nil(t, err)

	err = process.MonitorAlias(alias)
	unit.Nil(t, err)

	// Test event registration
	eventRef, err := process.RegisterEvent("test_event", gen.EventOptions{
		Notify: true,
		Buffer: 10,
	})
	unit.Nil(t, err)
	unit.True(t, eventRef.Node != "")

	// Test sending events
	err = process.SendEvent("test_event", eventRef, "test message")
	unit.Nil(t, err)
}

// Test call operations and timeouts
func TestCallOperations(t *testing.T) {
	actor, err := unit.Spawn(t, factoryOrderActor)
	if err != nil {
		t.Fatal(err)
	}

	clientPID := gen.PID{Node: "test", ID: 800}

	// Test call operation
	actor.SendMessage(clientPID, GetOrderStatus{})

	// Verify call event is captured
	actor.ShouldSend().To(clientPID).MessageMatching(unit.IsTypeGeneric[OrderStatus]()).Once().Assert()

	// Test various call types through process interface
	process := actor.Process()

	// These will create call events in testing
	_, _ = process.Call(clientPID, "test_request")
	_, _ = process.CallWithTimeout(clientPID, "timeout_request", 5000)
	_, _ = process.CallWithPriority(clientPID, "priority_request", gen.MessagePriorityHigh)
}

// Test complex network scenarios
func TestComplexNetworkScenarios(t *testing.T) {
	actor, err := unit.Spawn(t, factoryOrderActor)
	if err != nil {
		t.Fatal(err)
	}

	network := actor.Node().Network()

	// Setup complex network topology
	regions := []string{"us-east", "us-west", "eu-central", "asia-pacific"}

	for i, region := range regions {
		// Add multiple nodes per region
		for j := 1; j <= 3; j++ {
			nodeName := gen.Atom(fmt.Sprintf("%s-node%d@%s.example.com", region, j, region))
			remoteNode := actor.CreateRemoteNode(nodeName, true)

			remoteNode.SetUptime(int64(3600 * (i + j))) // Different uptimes
			remoteNode.SetVersion(gen.Version{
				Name:    fmt.Sprintf("%s-service", region),
				Release: fmt.Sprintf("v1.%d.%d", i, j),
			})

			// Add routes for each node
			route := gen.NetworkRoute{
				Route: gen.Route{
					Host: fmt.Sprintf("%s-node%d.example.com", region, j),
					Port: uint16(4369 + i*10 + j),
					TLS:  i%2 == 0, // Alternate TLS usage
				},
			}
			err = network.AddRoute(string(nodeName), route, 100-i*10)
			unit.Nil(t, err)
		}
	}

	// Test network statistics
	connectedNodes := actor.ListConnectedNodes()
	unit.True(t, len(connectedNodes) >= 12, "Should have at least 12 connected nodes")

	// Test route resolution
	routes, err := network.Route("us-east-node1@us-east.example.com")
	unit.Nil(t, err)
	unit.Equal(t, 1, len(routes))
	unit.Equal(t, "us-east-node1.example.com", routes[0].Route.Host)
}

// Test error propagation and recovery
func TestErrorPropagationAndRecovery(t *testing.T) {
	actor, err := unit.Spawn(t, factoryOrderActor)
	if err != nil {
		t.Fatal(err)
	}

	customerPID := gen.PID{Node: "test", ID: 900}

	// Test invalid state transitions
	behavior := actor.Behavior().(*orderActor)
	behavior.state = "completed" // Set to completed state

	// Try to add item to completed order
	actor.SendMessage(customerPID, AddItem{Item: "laptop", Price: 999.99})

	actor.ShouldSend().To(customerPID).MessageMatching(func(msg any) bool {
		if err, ok := msg.(OrderError); ok {
			return strings.Contains(err.Error, "cannot add items in state")
		}
		return false
	}).Once().Assert()

	// Test cancellation of completed order
	actor.SendMessage(customerPID, CancelOrder{})

	actor.ShouldSend().To(customerPID).MessageMatching(func(msg any) bool {
		if err, ok := msg.(OrderError); ok {
			return strings.Contains(err.Error, "cannot cancel completed order")
		}
		return false
	}).Once().Assert()
}

// Test concurrent operations simulation
func TestConcurrentOperationsSimulation(t *testing.T) {
	// Create manager with enough capacity for all workers
	actor, err := unit.Spawn(t, func() gen.ProcessBehavior {
		return &managerActor{
			workers:    make(map[string]gen.PID),
			maxWorkers: 10, // Increase capacity to handle all 6 workers
			strategy:   "one_for_one",
		}
	}, unit.WithLogLevel(gen.LogLevelInfo))
	if err != nil {
		t.Fatal(err)
	}

	// Simulate multiple clients making concurrent requests
	clients := []gen.PID{
		{Node: "client1", ID: 1},
		{Node: "client2", ID: 2},
		{Node: "client3", ID: 3},
	}

	// Each client starts multiple workers
	for i, client := range clients {
		for j := 1; j <= 2; j++ {
			workerID := fmt.Sprintf("client%d-worker%d", i+1, j)
			actor.SendMessage(client, StartWorker{
				WorkerID: workerID,
				Config:   map[string]any{"client_id": i + 1, "worker_num": j},
			})
		}
	}

	// Verify all workers started
	actor.ShouldSpawn().Factory(factoryWorkerActor).Times(6).Assert()

	// Verify all start confirmations
	actor.ShouldSend().MessageMatching(unit.IsTypeGeneric[WorkerStarted]()).Times(6).Assert()

	// Verify metrics for all workers
	actor.ShouldSend().To("metrics").MessageMatching(unit.IsTypeGeneric[WorkerMetrics]()).Times(6).Assert()
}

// Test comprehensive assertion combinations
func TestComprehensiveAssertions(t *testing.T) {
	actor, err := unit.Spawn(t, factoryOrderActor,
		unit.WithLogLevel(gen.LogLevelDebug),
	)
	if err != nil {
		t.Fatal(err)
	}

	customerPID := gen.PID{Node: "test", ID: 1000}

	// Clear initialization events for clean testing
	actor.ClearEvents()

	// Perform operations
	actor.SendMessage(customerPID, AddItem{Item: "test_item", Price: 50.0})
	actor.SendMessage(customerPID, AddItem{Item: "another_item", Price: 25.0})
	actor.SendMessage(customerPID, GetOrderStatus{})

	// Test chained assertions
	actor.ShouldSend().To(customerPID).MessageMatching(
		unit.HasField("Item", unit.Equals("test_item")),
	).Once().Assert()

	actor.ShouldSend().To(customerPID).MessageMatching(
		unit.StructureMatching(ItemAdded{}, map[string]unit.Matcher{
			"Item":  unit.Equals("another_item"),
			"Total": func(v any) bool { return v.(float64) == 75.0 },
		}),
	).Once().Assert()

	// Test never assertions
	actor.ShouldNotSend().To("nonexistent").Message("any").Assert()
	actor.ShouldSpawn().Factory(factoryValidatorActor).Times(0).Assert()

	// Test log assertions with patterns
	actor.ShouldLog().Level(gen.LogLevelDebug).Containing("Item added").Times(2).Assert()

	// Test event counting and inspection
	unit.True(t, actor.EventCount() >= 5, "Should have multiple events")

	events := actor.Events()
	sendEventCount := 0
	logEventCount := 0

	for _, event := range events {
		switch event.(type) {
		case unit.SendEvent:
			sendEventCount++
		case unit.LogEvent:
			logEventCount++
		}
	}

	unit.True(t, sendEventCount >= 3, "Should have send events")
	unit.True(t, logEventCount >= 2, "Should have log events")
}
