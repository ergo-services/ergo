package unit

import (
	"errors"
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

func TestFailureInjectionSystem(t *testing.T) {
	// Create test components
	events := lib.NewQueueMPSC()
	options := TestOptions{
		NodeName:     "test@localhost",
		NodeCreation: 1234567890,
		LogLevel:     gen.LogLevelInfo,
	}

	// Test TestNode failure injection
	t.Run("TestNode failure injection", func(t *testing.T) {
		node := NewTestNode(t, events, options)
		testErr := errors.New("spawn failed")

		// Set failure for Spawn method
		node.SetMethodFailure("Spawn", testErr)

		// Try to spawn - should fail
		_, err := node.Spawn(nil, gen.ProcessOptions{}, "test")
		if err == nil {
			t.Error("Expected spawn to fail, but it succeeded")
		}
		if err.Error() != "spawn failed" {
			t.Errorf("Expected error 'spawn failed', got '%s'", err.Error())
		}

		// Check that call count was tracked
		if count := node.GetMethodCallCount("Spawn"); count != 1 {
			t.Errorf("Expected call count 1, got %d", count)
		}

		// Clear failure
		node.ClearMethodFailure("Spawn")

		// Try again - should succeed
		_, err = node.Spawn(nil, gen.ProcessOptions{}, "test")
		if err != nil {
			t.Errorf("Expected spawn to succeed after clearing failure, got error: %s", err.Error())
		}
	})

	// Test TestProcess failure injection
	t.Run("TestProcess failure injection", func(t *testing.T) {
		node := NewTestNode(t, events, options)
		process := NewTestProcess(t, events, node, options)
		testErr := errors.New("process spawn failed")

		// Set failure after 2 calls
		process.SetMethodFailureAfter("Spawn", 2, testErr)

		// First call should succeed
		_, err := process.Spawn(nil, gen.ProcessOptions{}, "test1")
		if err != nil {
			t.Errorf("Expected first spawn to succeed, got error: %s", err.Error())
		}

		// Second call should succeed
		_, err = process.Spawn(nil, gen.ProcessOptions{}, "test2")
		if err != nil {
			t.Errorf("Expected second spawn to succeed, got error: %s", err.Error())
		}

		// Third call should fail
		_, err = process.Spawn(nil, gen.ProcessOptions{}, "test3")
		if err == nil {
			t.Error("Expected third spawn to fail, but it succeeded")
		}
		if err.Error() != "process spawn failed" {
			t.Errorf("Expected error 'process spawn failed', got '%s'", err.Error())
		}
	})

	// Test pattern-based failure
	t.Run("Pattern-based failure", func(t *testing.T) {
		node := NewTestNode(t, events, options)
		testErr := errors.New("worker spawn failed")

		// Set failure for names containing "worker"
		node.SetMethodFailurePattern("RegisterName", "worker", testErr)

		// Register normal name - should succeed
		err := node.RegisterName("supervisor", gen.PID{})
		if err != nil {
			t.Errorf("Expected supervisor registration to succeed, got error: %s", err.Error())
		}

		// Register worker name - should fail
		err = node.RegisterName("worker1", gen.PID{})
		if err == nil {
			t.Error("Expected worker registration to fail, but it succeeded")
		}
		if err.Error() != "worker spawn failed" {
			t.Errorf("Expected error 'worker spawn failed', got '%s'", err.Error())
		}
	})

	// Test failure events are recorded
	t.Run("Failure events recorded", func(t *testing.T) {
		node := NewTestNode(t, events, options)
		testErr := errors.New("test failure")

		// Set failure
		node.SetMethodFailure("Kill", testErr)

		// Trigger failure
		_ = node.Kill(gen.PID{})

		// Check events were recorded
		found := false
		for {
			event, ok := events.Pop()
			if !ok {
				break // No more events
			}
			if failure, ok := event.(FailureEvent); ok {
				if failure.Component == "node" && failure.Method == "Kill" {
					found = true
					break
				}
			}
		}
		if !found {
			t.Error("Expected failure event to be recorded, but none found")
		}
	})

	// Test common error helpers
	t.Run("Common error helpers", func(t *testing.T) {
		node := NewTestNode(t, events, options)

		spawnErr := errors.New("spawn failure")
		node.SetMethodFailure("Spawn", spawnErr)

		// Use common errors
		node.SetMethodFailure("RegisterName", CommonErrors.ProcessAlreadyExists())
		node.SetMethodFailure("Kill", CommonErrors.ProcessNotFound())

		// Test spawn failure
		_, err := node.Spawn(nil, gen.ProcessOptions{}, "test")
		if err == nil || err.Error() != "spawn failure" {
			t.Errorf("Expected spawn failure, got: %v", err)
		}

		// Test register failure
		err = node.RegisterName("test", gen.PID{})
		if err == nil || err != gen.ErrTaken {
			t.Errorf("Expected ErrTaken, got: %v", err)
		}

		// Test kill failure
		err = node.Kill(gen.PID{})
		if err == nil || err != gen.ErrProcessUnknown {
			t.Errorf("Expected ErrProcessUnknown, got: %v", err)
		}
	})
}

func TestFailureInjectionMultipleComponents(t *testing.T) {
	events := lib.NewQueueMPSC()
	options := TestOptions{
		NodeName:     "test@localhost",
		NodeCreation: 1234567890,
		LogLevel:     gen.LogLevelInfo,
	}

	// Create test actor first
	testActor := &TestActor{
		t:        t,
		events:   events,
		options:  options,
		captures: make(map[string]any),
	}

	// Create all components
	node := NewTestNode(t, events, options)
	process := NewTestProcess(t, events, node, options)
	network := newTestNetwork(testActor)

	// Set different failures for each component
	node.SetMethodFailure("Spawn", errors.New("node spawn failed"))
	process.SetMethodFailure("Send", errors.New("process send failed"))
	network.SetMethodFailure("GetNode", errors.New("network node failed"))

	// Test each component fails appropriately
	_, err := node.Spawn(nil, gen.ProcessOptions{}, "test")
	if err == nil || err.Error() != "node spawn failed" {
		t.Errorf("Expected node spawn failure, got: %v", err)
	}

	err = process.Send("test", "message")
	if err == nil || err.Error() != "process send failed" {
		t.Errorf("Expected process send failure, got: %v", err)
	}

	_, err = network.GetNode("remote")
	if err == nil || err.Error() != "network node failed" {
		t.Errorf("Expected network node failure, got: %v", err)
	}

	// Verify each component tracks its own call counts
	if count := node.GetMethodCallCount("Spawn"); count != 1 {
		t.Errorf("Expected node spawn count 1, got %d", count)
	}
	if count := process.GetMethodCallCount("Send"); count != 1 {
		t.Errorf("Expected process send count 1 (call counted even when failed), got %d", count)
	}
	if count := network.GetMethodCallCount("GetNode"); count != 1 {
		t.Errorf("Expected network getnode count 1 (call counted even when failed), got %d", count)
	}
}
