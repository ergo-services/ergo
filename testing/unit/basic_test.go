package unit

import (
	"testing"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
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

func (a *ExampleActor) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	// Handle test call requests
	switch request {
	case "test_request":
		return "test_response", nil
	default:
		return nil, nil
	}
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
	actor, err := Spawn(t, factoryExampleActor,
		WithLogLevel(gen.LogLevelDebug),
		WithEnv(map[gen.Env]any{"test_mode": true}),
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
	Equal(t, 1, behavior.value, "Initial value should be 1")

	// Test message handling
	actor.SendMessage(gen.PID{}, "increase")

	// Verify the response
	actor.ShouldSend().To(gen.Atom("abc")).Message(9).Once().Assert()
	actor.ShouldCall().To(gen.PID{}).Request(12345).Once().Assert()
	actor.ShouldSend().To(gen.Atom("def")).Once().Assert() // Response from call

	// Verify state change
	Equal(t, 9, behavior.value, "Value should be increased to 9")
}

// Test demonstrating handling of dynamic values
func TestDynamicValues(t *testing.T) {
	actor, err := Spawn(t, factoryExampleActor)
	if err != nil {
		t.Fatal(err)
	}

	// Clear any initialization events
	actor.ClearEvents()

	// Test creating a worker with dynamic ID
	actor.SendMessage(gen.PID{}, "create_worker")

	// Verify that a meta process was spawned
	spawnResult := actor.ShouldSpawnMeta().Once().Capture()
	NotNil(t, spawnResult, "Should capture spawned meta process")

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
		MessageMatching(StructureMatching(WorkerCreated{}, map[string]Matcher{
			"ID": IsValidAlias(),
		})).
		Once().
		Assert()
}

// Test demonstrating built-in assertions (zero dependencies)
func TestBuiltInAssertions(t *testing.T) {
	actor, err := Spawn(t, factoryExampleActor)
	if err != nil {
		t.Fatal(err)
	}

	// Use built-in assertion functions
	NotNil(t, actor.Process(), "Process should not be nil")
	NotNil(t, actor.Node(), "Node should not be nil")
	Equal(t, "test@localhost", string(actor.Node().Name()), "Node name should match")
	True(t, actor.Node().IsAlive(), "Node should be alive")
	False(t, actor.EventCount() < 0, "Event count should not be negative")

	// Test log level
	originalLevel := actor.Process().Log().Level()
	actor.Process().Log().SetLevel(gen.LogLevelWarning)
	Equal(t, gen.LogLevelWarning, actor.Process().Log().Level(), "Log level should be updated")

	// Restore original level
	actor.Process().Log().SetLevel(originalLevel)
}

// Test demonstrating event inspection
func TestEventInspection(t *testing.T) {
	actor, err := Spawn(t, factoryExampleActor)
	if err != nil {
		t.Fatal(err)
	}

	// Check initial event count
	True(t, actor.EventCount() > 0, "Should have initialization events")

	// Get all events
	events := actor.Events()
	True(t, len(events) > 0, "Should have captured events")

	// Find a specific event
	var foundSend bool
	for _, event := range events {
		if sendEvent, ok := event.(SendEvent); ok {
			if sendEvent.To == actor.PID() && sendEvent.Message == "hello" {
				foundSend = true
				break
			}
		}
	}
	True(t, foundSend, "Should find the 'hello' send event")

	// Test event clearing
	actor.ClearEvents()
	Equal(t, 0, actor.EventCount(), "Events should be cleared")

	// Generate new events
	actor.SendMessage(gen.PID{}, "increase")
	True(t, actor.EventCount() > 0, "Should have new events after message")

	// Test last event
	lastEvent := actor.LastEvent()
	NotNil(t, lastEvent, "Should have a last event")
}
