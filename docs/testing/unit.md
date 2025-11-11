---
description: A zero-dependency library for testing Ergo Framework actors with fluent API
hidden: true
---

# Unit

{% hint style="info" %}
Introduced for Ergo Framework 3.1.0 and above (not yet released. available in `v310` branch)
{% endhint %}

The Ergo Unit Testing Library makes testing actor-based systems simple and reliable. It provides specialized tools designed specifically for the unique challenges of testing actors, with zero external dependencies and an intuitive, readable API.

## What You'll Learn

This guide takes you from simple actor tests to complex distributed scenarios. Here's the journey:

### Getting Started (You Are Here!)
- Your First Test - Simple echo and counter examples  
- Built-in Assertions - Simple tools for common checks
- Basic Message Testing - Verify actors send the right messages
- Basic Logging Testing - Verify your actors provide good observability

### Intermediate Skills (Next Steps)
- Configuration Testing - Test environment-driven behavior
- Complex Message Patterns - Handle sophisticated message flows
- Basic Process Spawning - Test actor creation and lifecycle
- Event Inspection - Debug and analyze actor behavior

### Advanced Features (When You Need Them)
- Actor Termination - Test error handling and graceful shutdowns
- Exit Signals - Manage process lifecycles in supervision trees
- Scheduled Operations - Test cron jobs and time-based behavior
- Network & Distribution - Test multi-node actor systems

### Expert Level (Complex Scenarios)
- Dynamic Value Capture - Handle generated IDs, timestamps, and random data
- Complex Workflows - Test multi-step business processes  
- Performance & Load Testing - Verify behavior under stress

Tip: The documentation follows this learning path. You can jump to advanced topics if needed, but starting from the beginning ensures you understand the foundations.

## Why Testing Actors is Different

Traditional testing tools don't work well with actors. Here's why:

### The Challenge: Actors Are Not Functions

Regular code testing follows a simple pattern:
```go
// Traditional testing - call function, check result
result := calculateTax(income, rate)
assert.Equal(t, 1500.0, result)
```

But actors are fundamentally different:
- They run asynchronously - you send a message and the response comes later
- They maintain state - previous messages affect future behavior  
- They spawn other actors - creating complex hierarchies
- They communicate only via messages - no direct access to internal state
- They can fail and restart - requiring lifecycle testing

### What Makes Actor Testing Hard

1. Asynchronous Communication
```go
// This doesn't work with actors:
actor.SendMessage("process_order")
result := actor.GetResult() // ❌ No direct way to get result
```

2. Message Flow Complexity
```go
// An actor might send multiple messages to different targets:
actor.SendMessage("start_workflow")
// ❌ How do you verify it sent the right messages to the right places?
```

3. Dynamic Process Creation
```go
// Actors spawn other actors with generated IDs:
actor.SendMessage("create_worker")
// ❌ How do you test the spawned worker when you don't know its PID?
```

4. State Changes Over Time
```go
// Actor behavior changes based on message history:
actor.SendMessage("login", user1)
actor.SendMessage("login", user2)  
actor.SendMessage("get_users")
// ❌ How do you verify the internal state without breaking encapsulation?
```

## How This Library Solves Actor Testing

The Ergo Unit Testing Library addresses each of these challenges:

### Event Capture - See Everything Your Actor Does
Instead of guessing what happened, the library automatically captures every actor operation:
```go
actor.SendMessage("process_order")
// Library automatically captures:
// - What messages were sent
// - Which processes were spawned  
// - What was logged
// - When the actor terminated
```

### Fluent Assertions - Test What Matters
Express your test intentions clearly:
```go
actor.SendMessage("create_user", userData)
actor.ShouldSend().To("database").Message(SaveUser{...}).Once().Assert()
actor.ShouldSpawn().Factory(userWorkerFactory).Once().Assert()
actor.ShouldLog().Level(Info).Containing("User created").Assert()
```

### Dynamic Value Handling - Work With Generated Data
Capture and reuse dynamically generated values:
```go
actor.SendMessage("create_session")
sessionResult := actor.ShouldSpawn().Once().Capture()
sessionPID := sessionResult.PID // Use the actual generated PID in further tests
```

### State Testing Through Behavior - Verify State Changes
Test state indirectly by verifying behavioral changes:
```go
actor.SendMessage("login", user1)
actor.SendMessage("get_status")
actor.ShouldSend().To(user1).Message(StatusResponse{LoggedIn: true}).Assert()
```

### Why Zero Dependencies Matters

Actor testing is complex enough without dependency management headaches:
- No version conflicts - Works with any Go testing setup
- No external tools - Everything needed is built-in
- Simple imports - Just `import "ergo.services/ergo/testing/unit"`
- Fast execution - No overhead from external libraries

## Core Concepts

Now that you understand why actor testing is different, let's explore the key concepts that make this library work.

### The Event-Driven Testing Model

Everything your actor does becomes a testable "event".

When you run this simple test:
```go
actor.SendMessage(sender, "hello")
actor.ShouldSend().To(sender).Message("hello").Assert()
```

Here's what happens behind the scenes:

1. Your actor receives the message - Normal actor behavior
2. Your actor sends a response - Normal actor behavior  
3. The library captures a `SendEvent` - Testing magic
4. You verify the captured event - Your assertion

The library automatically captures these events:
- `SendEvent` - When your actor sends a message
- `SpawnEvent` - When your actor creates child processes
- `LogEvent` - When your actor writes log messages
- `TerminateEvent` - When your actor shuts down

### Why Events Matter

Events solve the fundamental challenge of testing asynchronous systems:

Instead of this (impossible):
```go
actor.SendMessage("process_order")
result := actor.WaitForResult() // ❌ Actors don't work this way
```

You do this (works perfectly):
```go
actor.SendMessage("process_order")
// Verify the actor did what it should do:
actor.ShouldSend().To("database").Message(SaveOrder{...}).Assert()
actor.ShouldSend().To("inventory").Message(CheckStock{...}).Assert()
actor.ShouldLog().Level(Info).Containing("Processing order").Assert()
```

### The Fluent Assertion API

The library provides a readable, chainable API that expresses test intentions clearly:

```go
// Basic pattern: Actor.Should[Action]().Details().Assert()

actor.ShouldSend().To(recipient).Message(content).Once().Assert()
actor.ShouldSpawn().Factory(workerFactory).Times(3).Assert()
actor.ShouldLog().Level(Info).Containing("started").Assert()
actor.ShouldTerminate().WithReason(normalShutdown).Assert()
```

Benefits of the fluent API:
- Readable - Tests read like English sentences
- Discoverable - IDE autocomplete guides you through options
- Flexible - Chain only the validations you need
- Precise - Specify exactly what matters for each test

## Installation

```bash
go get ergo.services/ergo/testing/unit
```

## Your First Actor Test

Let's start with the simplest possible actor test to understand the basics:

### A Simple Echo Actor

```go
package main

import (
    "testing"
    "ergo.services/ergo/act"
    "ergo.services/ergo/gen"
    "ergo.services/ergo/testing/unit"
)

// EchoActor - receives a message and sends it back
type EchoActor struct {
    act.Actor
}

func (e *EchoActor) HandleMessage(from gen.PID, message any) error {
    // Simply echo the message back to sender
    e.Send(from, message)
    return nil
}

// Factory function to create the actor
func newEchoActor() gen.ProcessBehavior {
    return &EchoActor{}
}
```

### Testing the Echo Actor

```go
func TestEchoActor_BasicBehavior(t *testing.T) {
    // 1. Create a test actor
    actor, err := unit.Spawn(t, newEchoActor)
    if err != nil {
        t.Fatal(err)
    }

    // 2. Create a sender PID (who is sending the message)
    sender := gen.PID{Node: "test", ID: 123}

    // 3. Send a message to the actor
    actor.SendMessage(sender, "hello world")

    // 4. Verify the actor sent the message back
    actor.ShouldSend().
        To(sender).                    // Should send to the original sender
        Message("hello world").        // Should send back the same message
        Once().                        // Should happen exactly once
        Assert()                       // Check that it actually happened
}
```

### What Just Happened?

This simple test demonstrates the core pattern:

1. `unit.Spawn()` - Creates a test actor in an isolated environment
2. `actor.SendMessage()` - Sends a message to your actor (like prod would)
3. `actor.ShouldSend()` - Verifies that your actor sent the expected message

Key insight: You're not testing internal state - you're testing behavior. You verify what the actor *does* (sends messages) rather than what it *contains* (internal variables).

### Why This Works

The testing library automatically captures everything your actor does:
- Every message sent by your actor
- Every process spawned by your actor  
- Every log message written by your actor
- When your actor terminates

Then it provides fluent assertions to verify these captured events.

### Adding Slightly More Complexity

Let's test an actor that maintains some state:

```go
type CounterActor struct {
    act.Actor
    count int
}

func (c *CounterActor) HandleMessage(from gen.PID, message any) error {
    switch message {
    case "increment":
        c.count++
        c.Send(from, c.count)
    case "get":
        c.Send(from, c.count)
    case "reset":
        c.count = 0
        c.Send(from, "reset complete")
    }
    return nil
}

func TestCounterActor_StatefulBehavior(t *testing.T) {
    actor, _ := unit.Spawn(t, func() gen.ProcessBehavior { return &CounterActor{} })
    client := gen.PID{Node: "test", ID: 456}

    // Test incrementing
    actor.SendMessage(client, "increment")
    actor.ShouldSend().To(client).Message(1).Once().Assert()

    actor.SendMessage(client, "increment")
    actor.ShouldSend().To(client).Message(2).Once().Assert()

    // Test getting current value
    actor.SendMessage(client, "get")
    actor.ShouldSend().To(client).Message(2).Once().Assert()

    // Test reset
    actor.SendMessage(client, "reset")
    actor.ShouldSend().To(client).Message("reset complete").Once().Assert()

    // Verify reset worked
    actor.SendMessage(client, "get")
    actor.ShouldSend().To(client).Message(0).Once().Assert()
}
```

This shows how you test stateful behavior without accessing internal state - by observing how the actor's responses change over time.

## Built-in Assertions

Before diving into complex actor testing, let's cover the simple assertion utilities you'll use throughout your tests.

Why Built-in Assertions Matter for Actor Testing:

Actor tests often need to verify simple conditions alongside complex event assertions. Rather than forcing you to import external testing libraries (which could conflict with your project dependencies), the unit testing library provides everything you need:

```go
func TestActorWithBuiltInAssertions(t *testing.T) {
    actor, _ := unit.Spawn(t, newEchoActor)
    
    // Use built-in assertions for simple checks
    unit.NotNil(t, actor, "Actor should be created successfully")
    unit.Equal(t, false, actor.IsTerminated(), "New actor should not be terminated")
    
    // Combine with actor-specific assertions
    actor.SendMessage(gen.PID{Node: "test", ID: 1}, "hello")
    actor.ShouldSend().Message("hello").Once().Assert()
}
```

### Available Assertions

Equality Testing:
```go
unit.Equal(t, expected, actual)        // Values must be equal
unit.NotEqual(t, unexpected, actual)   // Values must be different
```

Boolean Testing:
```go
unit.True(t, condition)               // Condition must be true
unit.False(t, condition)              // Condition must be false
```

Nil Testing:
```go
unit.Nil(t, value)                    // Value must be nil
unit.NotNil(t, value)                 // Value must not be nil
```

String Testing:
```go
unit.Contains(t, "hello world", "world")  // String must contain substring
```

Type Testing:
```go
unit.IsType(t, "", actualValue)       // Value must be of specific type
```

### Why Zero Dependencies Matter

No Import Conflicts:
```go
// ❌ This could cause version conflicts:
import "github.com/stretchr/testify/assert"
import "github.com/other/testing/lib"

// This always works:
import "ergo.services/ergo/testing/unit"
```

Consistent Error Messages:
All assertions provide clear, consistent error messages that integrate well with the actor testing output.

Framework Agnostic:
Works with any Go testing setup - standard `go test`, IDE test runners, CI/CD systems, etc.

## Basic Message Testing

Now that you understand the fundamentals, let's explore message testing in more depth.

## What Comes Next

Now you'll learn how to test different aspects of actor behavior, building from simple to complex:

Fundamentals (You're here!)
- Basic message sending and receiving
- Simple process creation  
- Logging and observability
- Configuration testing

Intermediate Skills
- Complex message patterns
- Event inspection and debugging
- Actor lifecycle and termination
- Error handling and recovery

Advanced Features
- Scheduled operations (cron jobs)
- Network and distribution
- Performance and load testing

## Basic Logging Testing

Logging is crucial for production actors - it provides visibility into what your actors are doing and helps with debugging. Let's learn how to test logging behavior.

### Why Test Logging?

**Logging tests ensure:**
- Your actors provide sufficient information for monitoring
- Debug information is available when needed  
- Log levels are respected (don't log debug in production)
- Sensitive operations are properly audited

### Simple Logging Test

```go
func TestGreeter_LogsWelcomeMessage(t *testing.T) {
    actor, _ := unit.Spawn(t, newGreeter, unit.WithLogLevel(gen.LogLevelInfo))
    
    actor.SendMessage(gen.PID{}, Welcome{Name: "Alice"})
    
    // Verify the actor logged the welcome
    actor.ShouldLog().
        Level(gen.LogLevelInfo).
        Containing("Welcome Alice").
        Once().
        Assert()
}
```

### Testing Different Log Levels

```go
func TestDataProcessor_LogLevels(t *testing.T) {
    actor, _ := unit.Spawn(t, newDataProcessor, unit.WithLogLevel(gen.LogLevelDebug))
    
    actor.SendMessage(gen.PID{}, ProcessData{Data: "sample"})
    
    // Should log at info level for important events
    actor.ShouldLog().Level(gen.LogLevelInfo).Containing("Processing started").Once().Assert()
    
    // Should log at debug level for detailed info
    actor.ShouldLog().Level(gen.LogLevelDebug).Containing("Processing sample data").Once().Assert()
    
    // Should never log at error level for normal operations
    actor.ShouldLog().Level(gen.LogLevelError).Times(0).Assert()
}
```

### Testing Log Content

```go
func TestAuditLogger_SecurityEvents(t *testing.T) {
    actor, _ := unit.Spawn(t, newAuditLogger)
    
    actor.SendMessage(gen.PID{}, LoginAttempt{User: "admin", Success: false})
    
    // Verify security events are properly logged
    actor.ShouldLog().MessageMatching(func(msg string) bool {
        return strings.Contains(msg, "SECURITY") && 
               strings.Contains(msg, "admin") && 
               strings.Contains(msg, "failed")
    }).Once().Assert()
}
```

### Logging Best Practices for Testing

Structure your log messages to make them easy to test:
```go
// Good: Structured, predictable format
log.Info("User login: user=%s success=%t", userID, success)

// Poor: Hard to test reliably  
log.Info("User " + userID + " tried to login and it " + result)
```

**Test log levels appropriately**:
- `Error` - Test that errors are logged when they occur
- `Warning` - Test that concerning but non-fatal events are captured
- `Info` - Test that important business events are recorded
- `Debug` - Test that detailed troubleshooting info is available

## Intermediate Skills

Now that you've mastered the basics, let's tackle more complex testing scenarios.

### Configuration and Environment Testing

Real actors often behave differently based on configuration. Let's test this:

The `Spawn` function creates an isolated testing environment for your actor. Unlike production actors that run in a complex node environment, test actors run in a controlled sandbox where every operation is captured for verification.

**Key Benefits:**
- **Isolation**: Each test actor runs independently without affecting other tests
- **Deterministic**: Test outcomes are predictable and repeatable
- **Observable**: All actor operations are automatically captured as events
- **Configurable**: Fine-tune the testing environment to match your needs

**Example Actor:**
```go
type messageCounter struct {
    act.Actor
    count int
}

func (m *messageCounter) Init(args ...any) error {
    m.count = 0
    m.Log().Info("Counter initialized")
    return nil
}

func (m *messageCounter) HandleMessage(from gen.PID, message any) error {
    switch msg := message.(type) {
    case "increment":
        m.count++
        m.Send("output", CountChanged{Count: m.count})
        m.Log().Debug("Count incremented to %d", m.count)
        return nil
    case "get_count":
        m.Send(from, CountResponse{Count: m.count})
        return nil
    case "reset":
        m.count = 0
        m.Send("output", CountReset{})
        return nil
    }
    return nil
}

type CountChanged struct{ Count int }
type CountResponse struct{ Count int }
type CountReset struct{}

func factoryMessageCounter() gen.ProcessBehavior {
    return &messageCounter{}
}
```

**Test Implementation:**
```go
func TestMessageCounter_BasicUsage(t *testing.T) {
    // Create test actor with configuration
    actor, err := unit.Spawn(t, factoryMessageCounter,
        unit.WithLogLevel(gen.LogLevelDebug),
        unit.WithEnv(map[gen.Env]any{
            "test_mode": true,
            "timeout":   30,
        }),
    )
    if err != nil {
        t.Fatal(err)
    }

    // Test initialization
    actor.ShouldLog().Level(gen.LogLevelInfo).Containing("Counter initialized").Once().Assert()

    // Test message handling
    actor.SendMessage(gen.PID{}, "increment")
    actor.ShouldSend().To("output").Message(CountChanged{Count: 1}).Once().Assert()
    actor.ShouldLog().Level(gen.LogLevelDebug).Containing("Count incremented to 1").Once().Assert()

    // Test state query
    actor.SendMessage(gen.PID{Node: "test", ID: 123}, "get_count")
    actor.ShouldSend().To(gen.PID{Node: "test", ID: 123}).Message(CountResponse{Count: 1}).Once().Assert()

    // Test reset
    actor.SendMessage(gen.PID{}, "reset")
    actor.ShouldSend().To("output").Message(CountReset{}).Once().Assert()
}
```

#### Configuration Options - Fine-Tuning the Test Environment

Test configuration allows you to simulate different runtime conditions without requiring complex setup:

```go
// Available options for unit.Spawn()
unit.WithLogLevel(gen.LogLevelDebug)                    // Set log level
unit.WithEnv(map[gen.Env]any{"key": "value"})          // Environment variables
unit.WithParent(gen.PID{Node: "parent", ID: 100})      // Parent process
unit.WithRegister(gen.Atom("registered_name"))         // Register with name
unit.WithNodeName(gen.Atom("test_node@localhost"))     // Node name
```

**Environment Variables** (`WithEnv`): Test how your actors behave with different configurations without changing production code. Useful for testing feature flags, database URLs, timeout values, and other configuration-driven behavior.

**Log Levels** (`WithLogLevel`): Control the verbosity of test output and verify that your actors log appropriately at different levels. Critical for testing monitoring and debugging capabilities.

**Process Hierarchy** (`WithParent`, `WithRegister`): Test actors that need to interact with parent processes or require specific naming for registration-based lookups.

### Message Testing

#### `ShouldSend()` - Verifying Actor Communication

Message testing is the heart of actor validation. Since actors communicate exclusively through messages, verifying message flow is crucial for ensuring correct behavior.

**Why Message Testing Matters:**
- **Validates Integration**: Ensures actors communicate correctly with their dependencies
- **Confirms Business Logic**: Verifies that the right messages are sent in response to inputs
- **Detects Side Effects**: Catches unintended message sends that could cause bugs
- **Tests Message Content**: Validates that message payloads contain correct data

**Example Actor:**
```go
type notificationService struct {
    act.Actor
    subscribers []gen.PID
}

func (n *notificationService) HandleMessage(from gen.PID, message any) error {
    switch msg := message.(type) {
    case Subscribe:
        n.subscribers = append(n.subscribers, msg.PID)
        n.Send(msg.PID, SubscriptionConfirmed{})
        return nil
    case Broadcast:
        for _, subscriber := range n.subscribers {
            n.Send(subscriber, Notification{
                ID:      msg.ID,
                Message: msg.Message,
                Sender:  from,
            })
        }
        n.Send("analytics", BroadcastSent{
            ID:          msg.ID,
            Subscribers: len(n.subscribers),
        })
        return nil
    }
    return nil
}

type Subscribe struct{ PID gen.PID }
type SubscriptionConfirmed struct{}
type Broadcast struct{ ID string; Message string }
type Notification struct{ ID, Message string; Sender gen.PID }
type BroadcastSent struct{ ID string; Subscribers int }
```

**Test Implementation:**
```go
func TestNotificationService_MessageSending(t *testing.T) {
    actor, _ := unit.Spawn(t, factoryNotificationService)

    subscriber1 := gen.PID{Node: "test", ID: 101}
    subscriber2 := gen.PID{Node: "test", ID: 102}

    // Test subscription
    actor.SendMessage(gen.PID{}, Subscribe{PID: subscriber1})
    actor.SendMessage(gen.PID{}, Subscribe{PID: subscriber2})

    // Verify subscription confirmations
    actor.ShouldSend().To(subscriber1).Message(SubscriptionConfirmed{}).Once().Assert()
    actor.ShouldSend().To(subscriber2).Message(SubscriptionConfirmed{}).Once().Assert()

    // Test broadcast
    broadcaster := gen.PID{Node: "test", ID: 200}
    actor.SendMessage(broadcaster, Broadcast{ID: "msg-123", Message: "Hello World"})

    // Verify notifications sent to all subscribers
    actor.ShouldSend().To(subscriber1).MessageMatching(func(msg any) bool {
        if notif, ok := msg.(Notification); ok {
            return notif.ID == "msg-123" && 
                   notif.Message == "Hello World" &&
                   notif.Sender == broadcaster
        }
        return false
    }).Once().Assert()

    actor.ShouldSend().To(subscriber2).MessageMatching(func(msg any) bool {
        if notif, ok := msg.(Notification); ok {
            return notif.ID == "msg-123" && notif.Message == "Hello World"
        }
        return false
    }).Once().Assert()

    // Verify analytics
    actor.ShouldSend().To("analytics").Message(BroadcastSent{
        ID:          "msg-123",
        Subscribers: 2,
    }).Once().Assert()

    // Test multiple sends to same target
    actor.SendMessage(broadcaster, Broadcast{ID: "msg-124", Message: "Second message"})
    actor.ShouldSend().To("analytics").Times(2).Assert() // Total of 2 analytics messages
}
```

#### Advanced Message Matching - Flexible Validation Patterns

When testing complex message structures or dynamic content, the library provides powerful matching capabilities:

```go
// Message type matching
actor.ShouldSend().MessageMatching(unit.IsTypeGeneric[CountChanged]()).Assert()

// Field-based matching
actor.ShouldSend().MessageMatching(unit.HasField("Count", unit.Equals(5))).Assert()

// Structure matching with custom field validation
actor.ShouldSend().MessageMatching(
    unit.StructureMatching(Notification{}, map[string]unit.Matcher{
        "ID":      unit.Equals("msg-123"),
        "Sender":  unit.IsValidPID(),
    }),
).Assert()

// Never sent verification
actor.ShouldNotSend().To("error_handler").Message("error").Assert()
```

**Pattern Matching Benefits:**
- **Partial Validation**: Test only the fields that matter for your specific test case
- **Dynamic Content Handling**: Validate messages with timestamps, UUIDs, or generated IDs
- **Type Safety**: Ensure messages are of the correct type even when content varies
- **Negative Testing**: Verify that certain messages are NOT sent in specific scenarios

### Process Spawning

#### `ShouldSpawn()` - Testing Process Lifecycle Management

Process spawning is a fundamental actor pattern for building hierarchical systems. The testing library provides comprehensive tools for verifying that actors create, configure, and manage child processes correctly.

**Why Process Spawning Tests Matter:**
- **Resource Management**: Ensure actors don't spawn too many or too few processes
- **Configuration Propagation**: Verify that child processes receive correct configuration
- **Error Handling**: Test behavior when process spawning fails
- **Supervision Trees**: Validate that supervisors manage their children appropriately

**Example Actor:**
```go
type workerSupervisor struct {
    act.Actor
    workers    map[string]gen.PID
    maxWorkers int
}

func (w *workerSupervisor) Init(args ...any) error {
    w.workers = make(map[string]gen.PID)
    w.maxWorkers = 3
    return nil
}

func (w *workerSupervisor) HandleMessage(from gen.PID, message any) error {
    switch msg := message.(type) {
    case StartWorker:
        if len(w.workers) >= w.maxWorkers {
            w.Send(from, WorkerError{Error: "max workers reached"})
            return nil
        }

        // Spawn worker with dynamic name
        workerPID, err := w.Spawn(factoryWorker, gen.ProcessOptions{}, msg.WorkerID)
        if err != nil {
            w.Send(from, WorkerError{Error: err.Error()})
            return nil
        }

        w.workers[msg.WorkerID] = workerPID
        w.Send(from, WorkerStarted{WorkerID: msg.WorkerID, PID: workerPID})
        w.Send("monitor", SupervisorStatus{
            ActiveWorkers: len(w.workers),
            MaxWorkers:    w.maxWorkers,
        })
        return nil

    case StopWorker:
        if pid, exists := w.workers[msg.WorkerID]; exists {
            w.SendExit(pid, gen.TerminateReasonShutdown)
            delete(w.workers, msg.WorkerID)
            w.Send(from, WorkerStopped{WorkerID: msg.WorkerID})
        }
        return nil

    case StopAllWorkers:
        for workerID, pid := range w.workers {
            w.SendExit(pid, gen.TerminateReasonShutdown)
            delete(w.workers, workerID)
        }
        w.Send(from, AllWorkersStopped{Count: len(w.workers)})
        return nil
    }
    return nil
}

type StartWorker struct{ WorkerID string }
type StopWorker struct{ WorkerID string }
type StopAllWorkers struct{}
type WorkerStarted struct{ WorkerID string; PID gen.PID }
type WorkerStopped struct{ WorkerID string }
type WorkerError struct{ Error string }
type AllWorkersStopped struct{ Count int }
type SupervisorStatus struct{ ActiveWorkers, MaxWorkers int }

func factoryWorker() gen.ProcessBehavior { return &worker{} }
func factoryWorkerSupervisor() gen.ProcessBehavior { return &workerSupervisor{} }

type worker struct{ act.Actor }
func (w *worker) HandleMessage(from gen.PID, message any) error { return nil }
```

**Test Implementation:**
```go
func TestWorkerSupervisor_SpawnManagement(t *testing.T) {
    actor, _ := unit.Spawn(t, factoryWorkerSupervisor)
    client := gen.PID{Node: "test", ID: 999}

    // Test worker spawning
    actor.SendMessage(client, StartWorker{WorkerID: "worker-1"})

    // Capture the spawn event to get the PID
    spawnResult := actor.ShouldSpawn().Factory(factoryWorker).Once().Capture()
    unit.NotNil(t, spawnResult)

    // Verify worker started response
    actor.ShouldSend().To(client).MessageMatching(func(msg any) bool {
        if started, ok := msg.(WorkerStarted); ok {
            return started.WorkerID == "worker-1" && started.PID == spawnResult.PID
        }
        return false
    }).Once().Assert()

    // Verify monitor notification
    actor.ShouldSend().To("monitor").Message(SupervisorStatus{
        ActiveWorkers: 1,
        MaxWorkers:    3,
    }).Once().Assert()

    // Test multiple workers
    actor.SendMessage(client, StartWorker{WorkerID: "worker-2"})
    actor.SendMessage(client, StartWorker{WorkerID: "worker-3"})

    // Should have spawned 3 workers total
    actor.ShouldSpawn().Factory(factoryWorker).Times(3).Assert()

    // Test max worker limit
    actor.SendMessage(client, StartWorker{WorkerID: "worker-4"})
    actor.ShouldSend().To(client).Message(WorkerError{Error: "max workers reached"}).Once().Assert()
    
    // Should still only have 3 spawned workers
    actor.ShouldSpawn().Factory(factoryWorker).Times(3).Assert()

    // Test stopping a worker
    actor.SendMessage(client, StopWorker{WorkerID: "worker-1"})
    actor.ShouldSend().To(client).Message(WorkerStopped{WorkerID: "worker-1"}).Once().Assert()
}
```

#### Dynamic Process Testing - Handling Generated Values

Real-world actors often generate dynamic values like session IDs, request tokens, or timestamps. The library provides sophisticated tools for capturing and validating these dynamic values.

```go
func TestDynamicProcessCreation(t *testing.T) {
    actor, _ := unit.Spawn(t, factoryTaskProcessor)

    // Test dynamic process creation with captured PIDs
    actor.SendMessage(gen.PID{}, CreateSessionWorker{UserID: "user123"})

    // Capture the spawn to get dynamic PID
    spawnResult := actor.ShouldSpawn().Once().Capture()
    sessionPID := spawnResult.PID

    // Verify session was registered with the dynamic PID
    actor.ShouldSend().To("session_registry").MessageMatching(func(msg any) bool {
        if reg, ok := msg.(SessionRegistered); ok {
            return reg.UserID == "user123" && reg.SessionPID == sessionPID
        }
        return false
    }).Once().Assert()

    // Test sending work to the dynamic session
    actor.SendMessage(gen.PID{}, SendToSession{
        UserID: "user123", 
        Task:   "process_data",
    })

    // Should route to the captured session PID
    actor.ShouldSend().To(sessionPID).MessageMatching(func(msg any) bool {
        if task, ok := msg.(SessionTask); ok {
            return task.Task == "process_data"
        }
        return false
    }).Once().Assert()
}

// Required message types for this example:
type CreateSessionWorker struct{ UserID string }
type SessionRegistered struct{ UserID string; SessionPID gen.PID }
type SendToSession struct{ UserID, Task string }
type SessionTask struct{ Task string }
// factoryTaskProcessor() gen.ProcessBehavior function would be defined separately
```

**Dynamic Value Testing Scenarios:**
- **Session Management**: Test actors that create sessions with generated IDs
- **Request Tracking**: Verify that request tokens are properly generated and used
- **Time-based Operations**: Validate actors that schedule work or create timestamps
- **Resource Allocation**: Test dynamic assignment of resources to processes

### Remote Spawn Testing

#### `ShouldRemoteSpawn()` - Testing Distributed Actor Creation

Remote spawn testing allows you to verify that actors correctly create processes on remote nodes in a distributed system. The testing library captures `RemoteSpawnEvent` operations and provides fluent assertions for validation.

**Why Test Remote Spawning:**
- **Distribution Logic**: Ensure actors spawn processes on the correct remote nodes
- **Load Distribution**: Verify round-robin or other distribution strategies work correctly
- **Error Handling**: Test behavior when remote nodes are unavailable
- **Resource Management**: Validate that remote spawning respects capacity limits

**Example Actor:**
```go
type distributedCoordinator struct {
    act.Actor
    nodeAvailability map[gen.Atom]bool
    roundRobin      int
}

func (dc *distributedCoordinator) HandleMessage(from gen.PID, message any) error {
    switch msg := message.(type) {
    case SpawnRemoteWorker:
        if !dc.isNodeAvailable(msg.NodeName) {
            dc.Send(from, RemoteSpawnError{
                NodeName: msg.NodeName,
                Error:    "node not available",
            })
            return nil
        }

        // Use RemoteSpawn which generates RemoteSpawnEvent
        pid, err := dc.RemoteSpawn(msg.NodeName, msg.WorkerName, gen.ProcessOptions{}, msg.Config)
        if err != nil {
            dc.Send(from, RemoteSpawnError{NodeName: msg.NodeName, Error: err.Error()})
            return nil
        }

        dc.Send(from, RemoteWorkerSpawned{
            NodeName:   msg.NodeName,
            WorkerName: msg.WorkerName,
            PID:        pid,
        })
        return nil

    case SpawnRemoteService:
        // Use RemoteSpawnRegister which generates RemoteSpawnEvent with registration
        pid, err := dc.RemoteSpawnRegister(msg.NodeName, msg.ServiceName, msg.RegisterName, gen.ProcessOptions{})
        if err != nil {
            dc.Send(from, RemoteSpawnError{NodeName: msg.NodeName, Error: err.Error()})
            return nil
        }

        dc.Send(from, RemoteServiceSpawned{
            NodeName:     msg.NodeName,
            ServiceName:  msg.ServiceName,
            RegisterName: msg.RegisterName,
            PID:          pid,
        })
        return nil
    }
    return nil
}

type SpawnRemoteWorker struct{ NodeName, WorkerName gen.Atom; Config map[string]any }
type SpawnRemoteService struct{ NodeName, ServiceName, RegisterName gen.Atom }
type RemoteWorkerSpawned struct{ NodeName, WorkerName gen.Atom; PID gen.PID }
type RemoteServiceSpawned struct{ NodeName, ServiceName, RegisterName gen.Atom; PID gen.PID }
type RemoteSpawnError struct{ NodeName gen.Atom; Error string }
```

**Test Implementation:**
```go
func TestDistributedCoordinator_RemoteSpawn(t *testing.T) {
    actor, _ := unit.Spawn(t, factoryDistributedCoordinator)

    // Setup remote nodes for testing
    actor.CreateRemoteNode("worker@node1", true)  // Available
    actor.CreateRemoteNode("worker@node2", false) // Unavailable

    clientPID := gen.PID{Node: "test", ID: 100}
    actor.ClearEvents() // Clear initialization events

    // Test basic remote spawn
    actor.SendMessage(clientPID, SpawnRemoteWorker{
        NodeName:   "worker@node1",
        WorkerName: "data-processor",
        Config:     map[string]any{"timeout": 30},
    })

    // Verify remote spawn event
    actor.ShouldRemoteSpawn().
        ToNode("worker@node1").
        WithName("data-processor").
        Once().
        Assert()

    // Test remote spawn with registration
    actor.SendMessage(clientPID, SpawnRemoteService{
        NodeName:     "worker@node1",
        ServiceName:  "user-service",
        RegisterName: "users",
    })

    // Verify remote spawn with register
    actor.ShouldRemoteSpawn().
        ToNode("worker@node1").
        WithName("user-service").
        WithRegister("users").
        Once().
        Assert()

    // Test total remote spawns
    actor.ShouldRemoteSpawn().Times(2).Assert()

    // Test negative assertion - should not spawn on unavailable node
    actor.SendMessage(clientPID, SpawnRemoteWorker{
        NodeName:   "worker@node2",
        WorkerName: "test-worker",
    })

    actor.ShouldNotRemoteSpawn().ToNode("worker@node2").Assert()
}
```

**Advanced Remote Spawn Patterns:**
- **Multi-Node Distribution**: Test round-robin or other distribution strategies across multiple nodes
- **Error Scenarios**: Verify proper error handling when nodes are unavailable
- **Event Inspection**: Direct inspection of `RemoteSpawnEvent` for detailed validation
- **Negative Assertions**: Ensure remote spawns don't happen under certain conditions

### Actor Termination Testing

#### `ShouldTerminate()` - Testing Actor Lifecycle Completion

Actor termination is a critical aspect of actor systems. Actors can terminate for various reasons: normal completion, explicit shutdown, or errors. The testing library provides comprehensive tools for validating termination behavior and ensuring proper cleanup.

**Why Test Actor Termination:**
- **Resource Cleanup**: Ensure actors properly clean up resources when terminating
- **Error Propagation**: Verify that errors are handled correctly and lead to appropriate termination
- **Graceful Shutdown**: Test that actors respond correctly to shutdown signals
- **Supervision Trees**: Validate that supervisors handle child termination appropriately

**Termination Reasons:**
- `gen.TerminateReasonNormal` - Normal completion of actor work
- `gen.TerminateReasonShutdown` - Graceful shutdown request
- Custom errors - Abnormal termination due to specific errors

**Example Actor:**
```go
type connectionManager struct {
    act.Actor
    connections map[string]*Connection
    maxRetries  int
}

func (c *connectionManager) Init(args ...any) error {
    c.connections = make(map[string]*Connection)
    c.maxRetries = 3
    c.Log().Info("Connection manager started")
    return nil
}

func (c *connectionManager) HandleMessage(from gen.PID, message any) error {
    switch msg := message.(type) {
    case CreateConnection:
        conn := &Connection{ID: msg.ID, Status: "active"}
        c.connections[msg.ID] = conn
        c.Send(from, ConnectionCreated{ID: msg.ID})
        c.Log().Info("Created connection %s", msg.ID)
        return nil

    case CloseConnection:
        if conn, exists := c.connections[msg.ID]; exists {
            conn.Close()
            delete(c.connections, msg.ID)
            c.Send(from, ConnectionClosed{ID: msg.ID})
            c.Log().Info("Closed connection %s", msg.ID)
        }
        return nil

    case "shutdown":
        // Graceful shutdown - close all connections
        for id, conn := range c.connections {
            conn.Close()
            c.Log().Info("Shutdown: closed connection %s", id)
        }
        c.Send("monitor", ShutdownComplete{ConnectionsClosed: len(c.connections)})
        return gen.TerminateReasonShutdown

    case ConnectionError:
        c.Log().Error("Connection error for %s: %s", msg.ID, msg.Error)
        msg.RetryCount++
        
        if msg.RetryCount >= c.maxRetries {
            c.Log().Error("Max retries exceeded for connection %s", msg.ID)
            return fmt.Errorf("connection failed after %d retries: %s", c.maxRetries, msg.Error)
        }
        
        // Retry the connection
        c.Send(c.PID(), CreateConnection{ID: msg.ID})
        return nil

    case "force_error":
        // Simulate critical error
        return fmt.Errorf("critical system error: database unavailable")
    }
    return nil
}

type CreateConnection struct{ ID string }
type CloseConnection struct{ ID string }
type ConnectionCreated struct{ ID string }
type ConnectionClosed struct{ ID string }
type ConnectionError struct{ ID, Error string; RetryCount int }
type ShutdownComplete struct{ ConnectionsClosed int }

type Connection struct {
    ID     string
    Status string
}

func (c *Connection) Close() { c.Status = "closed" }

func factoryConnectionManager() gen.ProcessBehavior {
    return &connectionManager{}
}
```

**Test Implementation:**
```go
func TestConnectionManager_TerminationHandling(t *testing.T) {
    actor, _ := unit.Spawn(t, factoryConnectionManager)
    client := gen.PID{Node: "test", ID: 100}

    // Test normal operation first
    actor.SendMessage(client, CreateConnection{ID: "conn-1"})
    actor.ShouldSend().To(client).Message(ConnectionCreated{ID: "conn-1"}).Once().Assert()
    
    // Verify actor is not terminated during normal operation
    unit.Equal(t, false, actor.IsTerminated())
    unit.Nil(t, actor.TerminationReason())

    // Test graceful shutdown
    actor.SendMessage(client, "shutdown")
    
    // Verify shutdown message sent
    actor.ShouldSend().To("monitor").MessageMatching(func(msg any) bool {
        if shutdown, ok := msg.(ShutdownComplete); ok {
            return shutdown.ConnectionsClosed == 1
        }
        return false
    }).Once().Assert()

    // Verify graceful termination
    unit.Equal(t, true, actor.IsTerminated())
    unit.Equal(t, gen.TerminateReasonShutdown, actor.TerminationReason())

    // Verify termination event was captured
    actor.ShouldTerminate().
        WithReason(gen.TerminateReasonShutdown).
        Once().
        Assert()
}

func TestConnectionManager_ErrorTermination(t *testing.T) {
    actor, _ := unit.Spawn(t, factoryConnectionManager)

    // Test abnormal termination due to critical error
    actor.SendMessage(gen.PID{}, "force_error")

    // Verify actor terminated with error
    unit.Equal(t, true, actor.IsTerminated())
    unit.NotNil(t, actor.TerminationReason())
    unit.Contains(t, actor.TerminationReason().Error(), "critical system error")

    // Verify termination event with specific error
    actor.ShouldTerminate().
        ReasonMatching(func(reason error) bool {
            return strings.Contains(reason.Error(), "database unavailable")
        }).
        Once().
        Assert()
}

func TestConnectionManager_RetryBeforeTermination(t *testing.T) {
    actor, _ := unit.Spawn(t, factoryConnectionManager)

    // Test retry logic before termination
    actor.SendMessage(gen.PID{}, CreateConnection{ID: "conn-retry"})
    actor.ClearEvents() // Clear creation events

    // Send connection errors that should trigger retries
    for i := 0; i < 2; i++ {
        actor.SendMessage(gen.PID{}, ConnectionError{
            ID:         "conn-retry",
            Error:      "network timeout",
            RetryCount: i,
        })

        // Should not terminate yet
        unit.Equal(t, false, actor.IsTerminated())
        
        // Should retry by sending CreateConnection
        actor.ShouldSend().To(actor.PID()).MessageMatching(func(msg any) bool {
            if create, ok := msg.(CreateConnection); ok {
                return create.ID == "conn-retry"
            }
            return false
        }).Once().Assert()
    }

    // Final error that exceeds max retries
    actor.SendMessage(gen.PID{}, ConnectionError{
        ID:         "conn-retry",
        Error:      "network timeout",
        RetryCount: 3, // Exceeds maxRetries
    })

    // Now should terminate with error
    unit.Equal(t, true, actor.IsTerminated())
    unit.Contains(t, actor.TerminationReason().Error(), "connection failed after 3 retries")

    // Verify termination assertion
    actor.ShouldTerminate().
        ReasonMatching(func(reason error) bool {
            return strings.Contains(reason.Error(), "retries") && 
                   strings.Contains(reason.Error(), "network timeout")
        }).
        Once().
        Assert()
}

func TestTerminatedActor_NoFurtherProcessing(t *testing.T) {
    actor, _ := unit.Spawn(t, factoryConnectionManager)

    // Terminate the actor
    actor.SendMessage(gen.PID{}, "force_error")
    unit.Equal(t, true, actor.IsTerminated())

    actor.ClearEvents() // Clear termination events

    // Try to send more messages - should not be processed
    actor.SendMessage(gen.PID{}, CreateConnection{ID: "should-not-work"})
    
    // Should not process the message (no CreateConnection response)
    actor.ShouldNotSend().To(gen.PID{}).Message(ConnectionCreated{ID: "should-not-work"}).Assert()
    
    // Should not create any new events
    events := actor.Events()
    unit.Equal(t, 0, len(events), "Terminated actor should not process messages")
}

#### Termination Testing Methods

**TestActor Termination Status:**
```go
// Check if actor is terminated
isTerminated := actor.IsTerminated() // bool

// Get termination reason (nil if not terminated)
reason := actor.TerminationReason() // error or nil

// Test that actor should terminate
actor.ShouldTerminate().Once().Assert()

// Test with specific reason
actor.ShouldTerminate().WithReason(gen.TerminateReasonShutdown).Assert()

// Test with reason matching
actor.ShouldTerminate().ReasonMatching(func(reason error) bool {
    return strings.Contains(reason.Error(), "expected error")
}).Assert()

// Test that actor should NOT terminate
actor.ShouldNotTerminate().Assert()
```

**Advanced Termination Patterns:**
```go
// Test multiple termination attempts
actor.ShouldTerminate().Times(1).Assert() // Should terminate exactly once

// Capture termination for detailed analysis
terminationResult := actor.ShouldTerminate().Once().Capture()
unit.NotNil(t, terminationResult)
unit.Equal(t, expectedReason, terminationResult.Reason)

// Test termination with timeout
success := unit.WithTimeout(func() {
    actor.SendMessage(gen.PID{}, "shutdown")
    actor.ShouldTerminate().Once().Assert()
}, 5*time.Second)
unit.True(t, success(), "Actor should terminate within timeout")
```

### Exit Signal Testing

#### `ShouldSendExit()` - Testing Graceful Process Termination

Exit signals (`SendExit` and `SendExitMeta`) are used to gracefully terminate other processes. This is different from actor self-termination - it's about one actor telling another to exit. The testing library provides comprehensive assertions for validating exit signal behavior.

**Why Test Exit Signals:**
- **Graceful Shutdown**: Ensure supervisors can properly terminate child processes
- **Resource Cleanup**: Verify that exit signals trigger proper cleanup in target processes
- **Error Propagation**: Test that failure conditions are communicated via exit signals
- **Supervision Trees**: Validate that supervisors manage process lifecycles correctly

**Example Actor:**
```go
type processSupervisor struct {
    act.Actor
    workers map[string]gen.PID
    maxWorkers int
}

func (p *processSupervisor) Init(args ...any) error {
    p.workers = make(map[string]gen.PID)
    p.maxWorkers = 5
    return nil
}

func (p *processSupervisor) HandleMessage(from gen.PID, message any) error {
    switch msg := message.(type) {
    case StartWorker:
        if len(p.workers) >= p.maxWorkers {
            p.Send(from, WorkerStartError{Error: "max workers reached"})
            return nil
        }

        workerPID, err := p.Spawn(factoryWorkerProcess, gen.ProcessOptions{}, msg.WorkerID)
        if err != nil {
            p.Send(from, WorkerStartError{Error: err.Error()})
            return nil
        }

        p.workers[msg.WorkerID] = workerPID
        p.Send(from, WorkerStarted{WorkerID: msg.WorkerID, PID: workerPID})
        return nil

    case StopWorker:
        if workerPID, exists := p.workers[msg.WorkerID]; exists {
            // Send exit signal to worker
            p.SendExit(workerPID, gen.TerminateReasonShutdown)
            delete(p.workers, msg.WorkerID)
            p.Send(from, WorkerStopped{WorkerID: msg.WorkerID})
            p.Log().Info("Sent exit signal to worker %s", msg.WorkerID)
        } else {
            p.Send(from, WorkerStopError{WorkerID: msg.WorkerID, Error: "worker not found"})
        }
        return nil

    case EmergencyShutdown:
        // Send exit signals to all workers with error reason
        shutdownReason := fmt.Errorf("emergency shutdown: %s", msg.Reason)
        
        for workerID, workerPID := range p.workers {
            p.SendExit(workerPID, shutdownReason)
            p.Log().Warning("Emergency shutdown: sent exit to worker %s", workerID)
        }
        
        // Send meta exit signal to monitoring system
        p.SendExitMeta(gen.PID{Node: "monitor", ID: 999}, shutdownReason)
        
        p.Send(from, EmergencyShutdownComplete{
            WorkersTerminated: len(p.workers),
            Reason:           msg.Reason,
        })
        
        p.workers = make(map[string]gen.PID) // Clear workers map
        return nil

    case TerminateWorkerWithError:
        if workerPID, exists := p.workers[msg.WorkerID]; exists {
            errorReason := fmt.Errorf("worker error: %s", msg.Error)
            p.SendExit(workerPID, errorReason)
            delete(p.workers, msg.WorkerID)
            
            p.Send(from, WorkerTerminated{
                WorkerID: msg.WorkerID,
                Reason:   msg.Error,
            })
        }
        return nil
    }
    return nil
}

type StartWorker struct{ WorkerID string }
type StopWorker struct{ WorkerID string }
type EmergencyShutdown struct{ Reason string }
type TerminateWorkerWithError struct{ WorkerID, Error string }

type WorkerStarted struct{ WorkerID string; PID gen.PID }
type WorkerStopped struct{ WorkerID string }
type WorkerStartError struct{ Error string }
type WorkerStopError struct{ WorkerID, Error string }
type EmergencyShutdownComplete struct{ WorkersTerminated int; Reason string }
type WorkerTerminated struct{ WorkerID, Reason string }

type workerProcess struct{ act.Actor }
func (w *workerProcess) HandleMessage(from gen.PID, message any) error { return nil }
func factoryWorkerProcess() gen.ProcessBehavior { return &workerProcess{} }
func factoryProcessSupervisor() gen.ProcessBehavior { return &processSupervisor{} }
```

**Test Implementation:**
```go
func TestProcessSupervisor_ExitSignals(t *testing.T) {
    actor, _ := unit.Spawn(t, factoryProcessSupervisor)
    client := gen.PID{Node: "test", ID: 100}

    // Start some workers
    actor.SendMessage(client, StartWorker{WorkerID: "worker-1"})
    actor.SendMessage(client, StartWorker{WorkerID: "worker-2"})

    // Capture worker PIDs for validation
    spawn1 := actor.ShouldSpawn().Factory(factoryWorkerProcess).Once().Capture()
    spawn2 := actor.ShouldSpawn().Factory(factoryWorkerProcess).Once().Capture()
    
    worker1PID := spawn1.PID
    worker2PID := spawn2.PID

    actor.ClearEvents() // Clear spawn events

    // Test graceful worker stop
    actor.SendMessage(client, StopWorker{WorkerID: "worker-1"})

    // Verify exit signal sent to worker
    actor.ShouldSendExit().
        To(worker1PID).
        WithReason(gen.TerminateReasonShutdown).
        Once().
        Assert()

    // Verify stop confirmation
    actor.ShouldSend().To(client).Message(WorkerStopped{WorkerID: "worker-1"}).Once().Assert()

    // Test worker termination with custom error
    actor.SendMessage(client, TerminateWorkerWithError{
        WorkerID: "worker-2",
        Error:    "memory leak detected",
    })

    // Verify exit signal with custom error reason
    actor.ShouldSendExit().
        To(worker2PID).
        ReasonMatching(func(reason error) bool {
            return strings.Contains(reason.Error(), "memory leak detected")
        }).
        Once().
        Assert()

    // Verify termination response
    actor.ShouldSend().To(client).MessageMatching(func(msg any) bool {
        if terminated, ok := msg.(WorkerTerminated); ok {
            return terminated.WorkerID == "worker-2" && 
                   terminated.Reason == "memory leak detected"
        }
        return false
    }).Once().Assert()
}

func TestProcessSupervisor_EmergencyShutdown(t *testing.T) {
    actor, _ := unit.Spawn(t, factoryProcessSupervisor)
    client := gen.PID{Node: "test", ID: 100}

    // Start multiple workers
    for i := 1; i <= 3; i++ {
        actor.SendMessage(client, StartWorker{WorkerID: fmt.Sprintf("worker-%d", i)})
    }

    // Capture all worker PIDs
    workers := make([]gen.PID, 3)
    for i := 0; i < 3; i++ {
        spawn := actor.ShouldSpawn().Factory(factoryWorkerProcess).Once().Capture()
        workers[i] = spawn.PID
    }

    actor.ClearEvents() // Clear spawn events

    // Trigger emergency shutdown
    actor.SendMessage(client, EmergencyShutdown{Reason: "system overload"})

    // Verify exit signals sent to all workers
    for _, workerPID := range workers {
        actor.ShouldSendExit().
            To(workerPID).
            ReasonMatching(func(reason error) bool {
                return strings.Contains(reason.Error(), "emergency shutdown") &&
                       strings.Contains(reason.Error(), "system overload")
            }).
            Once().
            Assert()
    }

    // Verify meta exit signal sent to monitoring
    monitorPID := gen.PID{Node: "monitor", ID: 999}
    actor.ShouldSendExitMeta().
        To(monitorPID).
        ReasonMatching(func(reason error) bool {
            return strings.Contains(reason.Error(), "system overload")
        }).
        Once().
        Assert()

    // Verify shutdown completion message
    actor.ShouldSend().To(client).MessageMatching(func(msg any) bool {
        if complete, ok := msg.(EmergencyShutdownComplete); ok {
            return complete.WorkersTerminated == 3 && 
                   complete.Reason == "system overload"
        }
        return false
    }).Once().Assert()

    // Verify total exit signals (3 workers + 1 meta)
    actor.ShouldSendExit().Times(3).Assert()
    actor.ShouldSendExitMeta().Times(1).Assert()
}

func TestExitSignal_NegativeAssertions(t *testing.T) {
    actor, _ := unit.Spawn(t, factoryProcessSupervisor)
    client := gen.PID{Node: "test", ID: 100}

    // Try to stop non-existent worker
    actor.SendMessage(client, StopWorker{WorkerID: "non-existent"})

    // Should not send any exit signals
    actor.ShouldNotSendExit().Assert()
    actor.ShouldNotSendExitMeta().Assert()

    // Should send error response instead
    actor.ShouldSend().To(client).MessageMatching(func(msg any) bool {
        if stopError, ok := msg.(WorkerStopError); ok {
            return stopError.WorkerID == "non-existent" && 
                   stopError.Error == "worker not found"
        }
        return false
    }).Once().Assert()
}
```

#### Exit Signal Testing Methods

**Basic Exit Signal Assertions:**
```go
// Test that exit signal was sent
actor.ShouldSendExit().To(targetPID).Once().Assert()

// Test with specific reason
actor.ShouldSendExit().To(targetPID).WithReason(gen.TerminateReasonShutdown).Assert()

// Test with reason matching
actor.ShouldSendExit().ReasonMatching(func(reason error) bool {
    return strings.Contains(reason.Error(), "expected error")
}).Assert()

// Test meta exit signals
actor.ShouldSendExitMeta().To(monitorPID).WithReason(errorReason).Assert()

// Negative assertions
actor.ShouldNotSendExit().To(targetPID).Assert()
actor.ShouldNotSendExitMeta().Assert()
```

**Advanced Exit Signal Patterns:**
```go
// Test multiple exit signals
actor.ShouldSendExit().Times(3).Assert() // Should send exactly 3 exit signals

// Test exit signals to specific targets
actor.ShouldSendExit().To(worker1PID).Once().Assert()
actor.ShouldSendExit().To(worker2PID).Once().Assert()

// Capture exit signal for detailed analysis
exitResult := actor.ShouldSendExit().Once().Capture()
unit.NotNil(t, exitResult)
unit.Equal(t, expectedPID, exitResult.To)
unit.Equal(t, expectedReason, exitResult.Reason)

// Combined assertions
actor.ShouldSendExit().To(workerPID).WithReason(gen.TerminateReasonShutdown).Once().Assert()
actor.ShouldSendExitMeta().To(monitorPID).ReasonMatching(func(r error) bool {
    return strings.Contains(r.Error(), "shutdown complete")
}).Once().Assert()
```

### Cron Testing

#### `ShouldAddCronJob()`, `ShouldExecuteCronJob()` - Testing Scheduled Operations

Cron job testing allows you to validate scheduled operations in your actors without waiting for real time to pass. The testing library provides comprehensive mock time support and detailed cron job lifecycle management.

**Why Test Cron Jobs:**
- **Schedule Validation**: Ensure cron expressions are correct and jobs run at expected times
- **Job Management**: Test job addition, removal, enabling, and disabling operations
- **Execution Logic**: Verify that scheduled operations perform correctly when triggered
- **Time Control**: Use mock time to test time-dependent behavior deterministically

**Cron Testing Features:**
- **Mock Time Support**: Control time flow for deterministic testing
- **Job Lifecycle Testing**: Validate job creation, scheduling, execution, and cleanup
- **Event Tracking**: Monitor all cron-related operations and state changes
- **Schedule Simulation**: Test complex scheduling scenarios without real time delays

**Example Actor:**
```go
type taskScheduler struct {
    act.Actor
    taskCounter int
    schedules   map[string]gen.CronJobSchedule
}

func (t *taskScheduler) Init(args ...any) error {
    t.taskCounter = 0
    t.schedules = make(map[string]gen.CronJobSchedule)
    t.Log().Info("Task scheduler started")
    return nil
}

func (t *taskScheduler) HandleMessage(from gen.PID, message any) error {
    switch msg := message.(type) {
    case ScheduleTask:
        // Add a new cron job
        jobID, err := t.Cron().AddJob(msg.Schedule, gen.CronJobFunction(func() {
            t.taskCounter++
            t.Send("output", TaskExecuted{
                TaskID:    msg.TaskID,
                Count:     t.taskCounter,
                Timestamp: time.Now(),
            })
            t.Log().Info("Executed scheduled task %s (count: %d)", msg.TaskID, t.taskCounter)
        }))
        
        if err != nil {
            t.Send(from, ScheduleError{TaskID: msg.TaskID, Error: err.Error()})
            return nil
        }

        t.schedules[msg.TaskID] = gen.CronJobSchedule{ID: jobID, Schedule: msg.Schedule}
        t.Send(from, TaskScheduled{TaskID: msg.TaskID, JobID: jobID})
        t.Log().Debug("Scheduled task %s with job ID %s", msg.TaskID, jobID)
        return nil

    case UnscheduleTask:
        if schedule, exists := t.schedules[msg.TaskID]; exists {
            err := t.Cron().RemoveJob(schedule.ID)
            if err != nil {
                t.Send(from, UnscheduleError{TaskID: msg.TaskID, Error: err.Error()})
                return nil
            }
            
            delete(t.schedules, msg.TaskID)
            t.Send(from, TaskUnscheduled{TaskID: msg.TaskID})
            t.Log().Debug("Unscheduled task %s", msg.TaskID)
        } else {
            t.Send(from, UnscheduleError{TaskID: msg.TaskID, Error: "task not found"})
        }
        return nil

    case EnableTask:
        if schedule, exists := t.schedules[msg.TaskID]; exists {
            err := t.Cron().EnableJob(schedule.ID)
            if err != nil {
                t.Send(from, TaskError{TaskID: msg.TaskID, Error: err.Error()})
                return nil
            }
            t.Send(from, TaskEnabled{TaskID: msg.TaskID})
        }
        return nil

    case DisableTask:
        if schedule, exists := t.schedules[msg.TaskID]; exists {
            err := t.Cron().DisableJob(schedule.ID)
            if err != nil {
                t.Send(from, TaskError{TaskID: msg.TaskID, Error: err.Error()})
                return nil
            }
            t.Send(from, TaskDisabled{TaskID: msg.TaskID})
        }
        return nil

    case GetTaskInfo:
        if schedule, exists := t.schedules[msg.TaskID]; exists {
            info, err := t.Cron().JobInfo(schedule.ID)
            if err != nil {
                t.Send(from, TaskError{TaskID: msg.TaskID, Error: err.Error()})
                return nil
            }
            t.Send(from, TaskInfo{
                TaskID:   msg.TaskID,
                JobID:    schedule.ID,
                Schedule: schedule.Schedule,
                Enabled:  info.Enabled,
                NextRun:  info.NextRun,
            })
        } else {
            t.Send(from, TaskError{TaskID: msg.TaskID, Error: "task not found"})
        }
        return nil
    }
    return nil
}

type ScheduleTask struct{ TaskID, Schedule string }
type UnscheduleTask struct{ TaskID string }
type EnableTask struct{ TaskID string }
type DisableTask struct{ TaskID string }
type GetTaskInfo struct{ TaskID string }

type TaskScheduled struct{ TaskID, JobID string }
type TaskUnscheduled struct{ TaskID string }
type TaskEnabled struct{ TaskID string }
type TaskDisabled struct{ TaskID string }
type TaskExecuted struct{ TaskID string; Count int; Timestamp time.Time }
type TaskInfo struct{ TaskID, JobID, Schedule string; Enabled bool; NextRun time.Time }
type ScheduleError struct{ TaskID, Error string }
type UnscheduleError struct{ TaskID, Error string }
type TaskError struct{ TaskID, Error string }

func factoryTaskScheduler() gen.ProcessBehavior {
    return &taskScheduler{}
}
```

**Test Implementation:**
```go
func TestTaskScheduler_CronJobs(t *testing.T) {
    actor, _ := unit.Spawn(t, factoryTaskScheduler)
    client := gen.PID{Node: "test", ID: 100}

    // Test basic job scheduling
    actor.SendMessage(client, ScheduleTask{
        TaskID:   "daily-backup",
        Schedule: "0 2 * * *", // Daily at 2 AM
    })

    // Verify cron job was added
    actor.ShouldAddCronJob().
        WithSchedule("0 2 * * *").
        Once().
        Assert()

    // Verify scheduling response
    actor.ShouldSend().To(client).MessageMatching(func(msg any) bool {
        if scheduled, ok := msg.(TaskScheduled); ok {
            return scheduled.TaskID == "daily-backup" && scheduled.JobID != ""
        }
        return false
    }).Once().Assert()

    // Test job execution by triggering it
    actor.TriggerCronJob("0 2 * * *") // Manually trigger the scheduled job

    // Verify job execution
    actor.ShouldExecuteCronJob().
        WithSchedule("0 2 * * *").
        Once().
        Assert()

    // Verify task execution message
    actor.ShouldSend().To("output").MessageMatching(func(msg any) bool {
        if executed, ok := msg.(TaskExecuted); ok {
            return executed.TaskID == "daily-backup" && executed.Count == 1
        }
        return false
    }).Once().Assert()
}

func TestTaskScheduler_MockTimeControl(t *testing.T) {
    actor, _ := unit.Spawn(t, factoryTaskScheduler)
    client := gen.PID{Node: "test", ID: 100}

    // Set initial mock time
    baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
    actor.SetCronMockTime(baseTime)

    // Schedule a job for every minute
    actor.SendMessage(client, ScheduleTask{
        TaskID:   "minute-task",
        Schedule: "* * * * *", // Every minute
    })

    cronJob := actor.ShouldAddCronJob().Once().Capture()
    actor.ClearEvents()

    // Advance time by 1 minute - should trigger the job
    actor.SetCronMockTime(baseTime.Add(1 * time.Minute))

    // Verify job executed
    actor.ShouldExecuteCronJob().
        WithJobID(cronJob.ID).
        Once().
        Assert()

    // Advance time by another minute
    actor.SetCronMockTime(baseTime.Add(2 * time.Minute))

    // Should execute again
    actor.ShouldExecuteCronJob().
        WithJobID(cronJob.ID).
        Times(2). // Total of 2 executions
        Assert()
}
```

#### Cron Testing Methods

**Job Lifecycle Assertions:**
```go
// Test that cron job was added
actor.ShouldAddCronJob().WithSchedule("0 2 * * *").Once().Assert()

// Test job execution
actor.ShouldExecuteCronJob().WithSchedule("0 * * * *").Times(3).Assert()

// Test job removal
actor.ShouldRemoveCronJob().WithJobID("job-123").Once().Assert()

// Test job enable/disable
actor.ShouldEnableCronJob().WithJobID("job-123").Once().Assert()
actor.ShouldDisableCronJob().WithJobID("job-123").Once().Assert()

// Negative assertions
actor.ShouldNotAddCronJob().Assert()
actor.ShouldNotExecuteCronJob().Assert()
```

**Mock Time Control:**
```go
// Set mock time for deterministic testing
baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
actor.SetCronMockTime(baseTime)

// Advance time to trigger scheduled jobs
actor.SetCronMockTime(baseTime.Add(1 * time.Hour))

// Manually trigger cron jobs for testing
actor.TriggerCronJob("0 * * * *") // Trigger hourly job
actor.TriggerCronJob("job-id-123") // Trigger by job ID
```

**Advanced Cron Patterns:**
```go
// Capture cron job for detailed analysis
cronJob := actor.ShouldAddCronJob().Once().Capture()
jobID := cronJob.ID
schedule := cronJob.Schedule

// Test multiple job executions with time control
for i := 0; i < 5; i++ {
    actor.SetCronMockTime(baseTime.Add(time.Duration(i) * time.Minute))
    actor.TriggerCronJob("* * * * *") // Every minute
}
actor.ShouldExecuteCronJob().Times(5).Assert()
```

## Built-in Assertions

The library includes a comprehensive set of zero-dependency assertion functions that cover common testing scenarios without requiring external testing frameworks:

```go
func TestBuiltInAssertions(t *testing.T) {
    // Equality assertions
    unit.Equal(t, "expected", "expected")
    unit.NotEqual(t, "different", "value")

    // Boolean assertions  
    unit.True(t, true)
    unit.False(t, false)

    // Nil assertions
    unit.Nil(t, nil)
    unit.NotNil(t, "not nil")

    // String assertions
    unit.Contains(t, "hello world", "world")
    
    // Type assertions
    unit.IsType(t, "", "string value")
}
```

**Why Built-in Assertions:**
- **Zero Dependencies**: Avoid version conflicts and complex dependency management
- **Consistent Interface**: All assertions follow the same pattern and error reporting
- **Testing Framework Agnostic**: Works with any Go testing approach
- **Actor-Specific**: Designed specifically for the needs of actor testing

## Advanced Features

### Dynamic Value Capture - Testing Generated Content

Real-world actors frequently generate dynamic values like timestamps, UUIDs, session IDs, or auto-incrementing counters. Traditional testing approaches struggle with these values because they're unpredictable. The library provides sophisticated capture mechanisms to handle these scenarios elegantly.

**The Challenge of Dynamic Values:**
- **Timestamps**: Created at runtime, impossible to predict exact values
- **UUIDs**: Randomly generated, different in every test run
- **Auto-incrementing IDs**: Dependent on execution order and system state
- **Process IDs**: Assigned by the actor system, not controllable in tests

**The Solution - Value Capture:**
```go
func TestDynamicValues(t *testing.T) {
    actor, _ := unit.Spawn(t, factorySessionManager)

    // Send request that will generate dynamic session ID
    actor.SendMessage(gen.PID{}, CreateSession{UserID: "user123"})

    // Capture the spawn to get the dynamic session PID
    spawnResult := actor.ShouldSpawn().Once().Capture()
    sessionPID := spawnResult.PID

    // Use captured PID in subsequent assertions
    actor.ShouldSend().MessageMatching(func(msg any) bool {
        if created, ok := msg.(SessionCreated); ok {
            return created.SessionPID == sessionPID && created.UserID == "user123"
        }
        return false
    }).Once().Assert()
}
```

**Capture Strategies:**
- **Immediate Capture**: Capture values as soon as they're generated
- **Pattern Matching**: Use validation functions to identify and validate dynamic content
- **Structured Matching**: Validate message structure while ignoring specific dynamic fields
- **Cross-Reference Testing**: Use captured values in multiple assertions to ensure consistency

### Event Inspection - Deep System Analysis

For complex testing scenarios or debugging difficult issues, the library provides direct access to the complete event timeline. This allows you to perform sophisticated analysis of actor behavior beyond what's possible with standard assertions.

#### `Events()` - Complete Event History

Access all captured events for detailed analysis:

```go
func TestEventInspection(t *testing.T) {
    actor, _ := unit.Spawn(t, factoryComplexActor)

    // Perform operations
    actor.SendMessage(gen.PID{}, ComplexOperation{})

    // Get all events for inspection
    events := actor.Events()
    
    var sendCount, spawnCount, logCount, remoteSpawnCount int
    for _, event := range events {
        switch event.(type) {
        case unit.SendEvent:
            sendCount++
        case unit.SpawnEvent:
            spawnCount++
        case unit.LogEvent:
            logCount++
        case unit.RemoteSpawnEvent:
            remoteSpawnCount++
        }
    }

    unit.True(t, sendCount > 0, "Should have send events")
    unit.True(t, spawnCount == 2, "Should spawn exactly 2 processes")
    unit.True(t, logCount >= 1, "Should have log events")
}
```

#### `LastEvent()` - Most Recent Operation

Get the most recently captured event:

```go
func TestLastEvent(t *testing.T) {
    actor, _ := unit.Spawn(t, factoryExampleActor)

    actor.SendMessage(gen.PID{}, "test")
    
    // Get the most recent event
    lastEvent := actor.LastEvent()
    unit.NotNil(t, lastEvent, "Should have a last event")
    unit.Equal(t, "send", lastEvent.Type())
    
    if sendEvent, ok := lastEvent.(unit.SendEvent); ok {
        unit.Equal(t, "test", sendEvent.Message)
    }
}
```

#### `ClearEvents()` - Reset Event History

Clear all captured events, useful for isolating test phases:

```go
func TestClearEvents(t *testing.T) {
    actor, _ := unit.Spawn(t, factoryExampleActor)

    // Perform some operations
    actor.SendMessage(gen.PID{}, "setup")
    actor.ShouldSend().Once().Assert()

    // Clear events before main test
    actor.ClearEvents()

    // Now test the main functionality
    actor.SendMessage(gen.PID{}, "main_operation")
    
    // Only the main operation events are captured
    events := actor.Events()
    unit.Equal(t, 1, len(events), "Should only have main operation event")
}
```

**Event Inspection Use Cases:**
- **Performance Analysis**: Count operations to identify performance bottlenecks
- **Workflow Validation**: Ensure complex multi-step processes execute in the correct order
- **Error Investigation**: Analyze the complete event sequence leading to failures
- **Integration Testing**: Verify that multiple actors interact correctly in complex scenarios

### Timeout Support - Assertion Timing Control

The library provides timeout support for assertions that might need time-based validation:

```go
import (
    "testing"
    "time"
    "ergo.services/ergo/testing/unit"
)

func TestWithTimeout(t *testing.T) {
    actor, _ := unit.Spawn(t, factoryExampleActor)

    // Test that assertion completes within timeout
    success := unit.WithTimeout(func() {
        actor.SendMessage(gen.PID{}, "test")
        actor.ShouldSend().Once().Assert()
    }, 5*time.Second)

    unit.True(t, success(), "Assertion should complete within timeout")
}
```

**Timeout Function Usage:**
- **Assertion Wrapping**: Wrap assertion functions to add timeout behavior
- **Integration Testing**: Useful when testing with external systems that might have delays
- **Performance Validation**: Ensure assertions complete within expected time limits

## Testing Patterns and Best Practices

### Test Organization Strategies

**Single Responsibility Testing:**
Each test should focus on one specific behavior or scenario. This makes tests easier to understand, debug, and maintain.

```go
// Good: Tests one specific behavior
func TestUserManager_CreateUser_Success(t *testing.T) { ... }
func TestUserManager_CreateUser_DuplicateEmail(t *testing.T) { ... }
func TestUserManager_CreateUser_InvalidData(t *testing.T) { ... }

// Poor: Tests multiple behaviors in one test
func TestUserManager_AllOperations(t *testing.T) { ... }
```

**State Isolation:**
Each test should start with a clean state and not depend on other tests. Use `actor.ClearEvents()` when needed to reset event history between test phases.

**Error Path Testing:**
Don't just test the happy path. Actor systems need robust error handling, so test failure scenarios thoroughly:

```go
func TestWorkerSupervisor_MaxWorkersReached(t *testing.T) {
    // Test that supervisor properly rejects requests when at capacity
    // Test that appropriate error messages are sent
    // Test that the supervisor remains functional after rejecting requests
}
```

### Message Design for Testability

**Structured Messages:**
Design your messages to be easily testable by using structured types rather than primitive values:

```go
// Good: Easy to test with pattern matching
type UserCreated struct {
    UserID   string
    Email    string
    Created  time.Time
}

// Poor: Hard to validate in tests
type GenericMessage struct {
    Type string
    Data map[string]interface{}
}
```

**Predictable vs Dynamic Content:**
Separate predictable content from dynamic content in your messages to make testing easier:

```go
type OrderProcessed struct {
    OrderID   string    // Predictable - can be set in test
    Total     float64   // Predictable - can be set in test
    ProcessedAt time.Time // Dynamic - use pattern matching
    RequestID string     // Dynamic - capture and validate
}
```

### Performance Testing Considerations

**Event Overhead:**
While event capture is lightweight, be aware that every operation creates events. For performance-critical tests, you can:
- Clear events periodically with `ClearEvents()`
- Focus assertions on specific time windows
- Use event inspection to identify performance bottlenecks

**Scaling Testing:**
Test how your actors behave under load by simulating multiple concurrent operations:

```go
import (
    "fmt"
    "testing"
    "ergo.services/ergo/testing/unit"
)

func TestWorkerPool_ConcurrentRequests(t *testing.T) {
    actor, _ := unit.Spawn(t, factoryWorkerPool)
    
    // Send multiple requests concurrently
    for i := 0; i < 100; i++ {
        actor.SendMessage(gen.PID{}, ProcessRequest{ID: fmt.Sprintf("req-%d", i)})
    }
    
    // Verify all requests were processed
    actor.ShouldSend().To("output").Times(100).Assert()
}

// Note: This example assumes you have defined:
// - type ProcessRequest struct{ ID string }
// - factoryWorkerPool() gen.ProcessBehavior function
```

## Best Practices

1. **Use descriptive test names** that clearly indicate what behavior is being tested
2. **Test all message types** your actor handles, including edge cases
3. **Capture dynamic values early** using the `Capture()` method for generated IDs
4. **Test error conditions** not just the happy path
5. **Use pattern matching** for complex message validation
6. **Clear events** between test phases when needed with `ClearEvents()`
7. **Configure appropriate log levels** for debugging vs production testing
8. **Test temporal behaviors** with timeout mechanisms
9. **Validate distributed scenarios** using network simulation
10. **Organize tests by behavior** rather than by implementation details

This testing library provides comprehensive coverage for all Ergo Framework actor patterns while maintaining zero external dependencies and excellent readability. By following these patterns and practices, you can build robust, well-tested actor systems that behave correctly in both simple and complex scenarios.

## Complete Examples and Use Cases

The library includes comprehensive test examples organized into feature-specific files that demonstrate all capabilities through real-world scenarios:

### Feature-Based Test Files

**`basic_test.go`** - Fundamental Actor Testing
- Basic actor functionality and message handling
- Dynamic value capture and validation
- Built-in assertions and event tracking
- Core testing patterns and best practices

**`network_test.go`** - Distributed System Testing  
- Remote node simulation and connectivity
- Network configuration and route management
- Remote spawn operations and event capture
- Multi-node interaction patterns

**`workflow_test.go`** - Complex Business Logic
- Multi-step order processing workflows
- State machine validation and transitions
- Business process orchestration
- Error handling and recovery scenarios

**`call_test.go`** - Synchronous Communication
- Call operations and response handling
- Async call patterns and timeouts
- Send/response communication flows
- Concurrent request management

**`cron_test.go`** - Scheduled Operations
- Cron job lifecycle management
- Mock time control and schedule validation
- Job execution tracking and assertions
- Time-dependent behavior testing

**`termination_test.go`** - Actor Lifecycle Management
- Actor termination handling and cleanup
- Exit signal testing (SendExit/SendExitMeta)
- Normal vs abnormal termination scenarios
- Resource cleanup validation

### Comprehensive Test Examples

1. **Complex State Machine Testing** (`workflow_test.go`)
   - Multi-step order processing workflow
   - Validation, payment, and fulfillment pipeline
   - State transition validation and error handling

2. **Process Management** (`basic_test.go`)
   - Dynamic worker spawning and management
   - Resource capacity limits and monitoring
   - Worker lifecycle (start, stop, restart)

3. **Advanced Pattern Matching** (`basic_test.go`)
   - Structure matching with partial validation
   - Dynamic value handling and field validation
   - Complex conditional message matching

4. **Remote Spawn Testing** (`network_test.go`)
   - Remote spawn operations on multiple nodes
   - Round-robin distribution testing
   - Error handling for unavailable nodes
   - Event inspection and workflow validation

5. **Cron Job Management** (`cron_test.go`)
   - Job scheduling and execution validation
   - Mock time control for deterministic testing
   - Schedule expression testing and validation

6. **Actor Termination** (`termination_test.go`)
   - Normal and abnormal termination scenarios
   - Exit signal testing and process cleanup
   - Termination reason validation
   - Post-termination behavior verification

7. **Concurrent Operations** (`call_test.go`)
   - Multi-client concurrent request handling
   - Resource contention and capacity management
   - Load testing and performance validation

8. **Environment & Configuration** (`basic_test.go`)
   - Environment variable management
   - Runtime configuration changes
   - Feature flag and conditional behavior testing

### Getting Started with Examples

```go
// Import the testing library
import "ergo.services/ergo/testing/unit"

// Run all tests
go test -v ergo.services/ergo/testing/unit

// Run feature-specific tests
go test -v -run TestBasic ergo.services/ergo/testing/unit
go test -v -run TestNetwork ergo.services/ergo/testing/unit
go test -v -run TestWorkflow ergo.services/ergo/testing/unit
go test -v -run TestCall ergo.services/ergo/testing/unit
go test -v -run TestCron ergo.services/ergo/testing/unit
go test -v -run TestTermination ergo.services/ergo/testing/unit
```

### Learning Path

1. **Start with Basic Examples**: `basic_test.go` - Core functionality and patterns
2. **Explore Message Testing**: `basic_test.go` - Message flow and assertions
3. **Learn Process Management**: `basic_test.go` - Spawn operations and lifecycle
4. **Master Synchronous Communication**: `call_test.go` - Calls and responses
5. **Study Complex Workflows**: `workflow_test.go` - Business logic testing
6. **Practice Network Testing**: `network_test.go` - Distributed operations
7. **Explore Scheduling**: `cron_test.go` - Time-based operations
8. **Understand Termination**: `termination_test.go` - Lifecycle completion

Each test file provides complete, working implementations of specific actor patterns and demonstrates best practices for testing each scenario. All tests include comprehensive comments explaining the testing strategy and validation approach.

### Configuration and Environment Testing

Real actors often behave differently based on configuration. Let's test this:

```go
func TestDatabaseActor_ConfigurationBehavior(t *testing.T) {
    // Test with different configurations
    
    // Development configuration
    devActor, _ := unit.Spawn(t, newDatabaseActor, 
        unit.WithEnv(map[gen.Env]any{
            "DB_POOL_SIZE": 5,
            "LOG_QUERIES":  true,
        }))
    
    devActor.SendMessage(gen.PID{}, ExecuteQuery{SQL: "SELECT * FROM users"})
    devActor.ShouldLog().Level(gen.LogLevelDebug).Containing("SELECT * FROM users").Assert()
    
    // Production configuration  
    prodActor, _ := unit.Spawn(t, newDatabaseActor,
        unit.WithEnv(map[gen.Env]any{
            "DB_POOL_SIZE": 50,
            "LOG_QUERIES":  false,
        }))
    
    prodActor.SendMessage(gen.PID{}, ExecuteQuery{SQL: "SELECT * FROM users"})
    prodActor.ShouldLog().Level(gen.LogLevelDebug).Times(0).Assert() // No query logging in prod
}
```

### Complex Message Patterns

As your actors become more sophisticated, your message testing needs to handle more complex scenarios:

#### Testing Message Sequences

```go
func TestOrderProcessor_WorkflowSteps(t *testing.T) {
    actor, _ := unit.Spawn(t, newOrderProcessor)
    client := gen.PID{Node: "client", ID: 1}
    
    // Start an order
    actor.SendMessage(client, CreateOrder{Items: []string{"book", "pen"}})
    
    // Should trigger a sequence of operations
    actor.ShouldSend().To("inventory").Message("check_availability").Once().Assert()
    actor.ShouldSend().To("payment").Message("calculate_total").Once().Assert()
    actor.ShouldSend().To("shipping").Message("estimate_delivery").Once().Assert()
    
    // Should send status back to client
    actor.ShouldSend().To(client).MessageMatching(func(msg any) bool {
        if status, ok := msg.(OrderStatus); ok {
            return status.Status == "processing"
        }
        return false
    }).Once().Assert()
}
```

#### Testing Conditional Logic

```go
func TestSecurityGate_AccessControl(t *testing.T) {
    actor, _ := unit.Spawn(t, newSecurityGate)
    
    // Test admin access
    admin := gen.PID{Node: "admin", ID: 1}
    actor.SendMessage(admin, AccessRequest{Resource: "admin_panel", User: "admin"})
    actor.ShouldSend().To(admin).Message(AccessGranted{}).Once().Assert()
    
    // Test regular user access to admin panel
    user := gen.PID{Node: "user", ID: 2}
    actor.SendMessage(user, AccessRequest{Resource: "admin_panel", User: "regular_user"})
    actor.ShouldSend().To(user).Message(AccessDenied{Reason: "insufficient privileges"}).Once().Assert()
    
    // Test regular user access to public resources
    actor.SendMessage(user, AccessRequest{Resource: "public_content", User: "regular_user"})
    actor.ShouldSend().To(user).Message(AccessGranted{}).Once().Assert()
}
```

### Basic Process Spawning

Many actors need to create child processes. Here's how to test this:

```go
func TestTaskManager_WorkerCreation(t *testing.T) {
    actor, _ := unit.Spawn(t, newTaskManager)
    client := gen.PID{Node: "client", ID: 1}
    
    // Request a new worker
    actor.SendMessage(client, CreateWorker{TaskType: "data_processing"})
    
    // Should spawn a worker process
    actor.ShouldSpawn().Once().Assert()
    
    // Should confirm to client
    actor.ShouldSend().To(client).MessageMatching(func(msg any) bool {
        if response, ok := msg.(WorkerCreated); ok {
            return response.TaskType == "data_processing"
        }
        return false
    }).Once().Assert()
}
```

#### Capturing Dynamic Process IDs

When actors spawn processes, you often need to use the generated PID in subsequent tests:

```go
func TestSessionManager_UserSessions(t *testing.T) {
    actor, _ := unit.Spawn(t, newSessionManager)
    client := gen.PID{Node: "client", ID: 1}
    
    // Create a session for a user
    actor.SendMessage(client, CreateSession{UserID: "alice"})
    
    // Capture the spawned session process
    sessionSpawn := actor.ShouldSpawn().Once().Capture()
    sessionPID := sessionSpawn.PID
    
    // Verify session was registered
    actor.ShouldSend().To(client).MessageMatching(func(msg any) bool {
        if response, ok := msg.(SessionCreated); ok {
            return response.UserID == "alice" && response.SessionPID == sessionPID
        }
        return false
    }).Once().Assert()
    
    // Send work to the session
    actor.SendMessage(client, SendToSession{UserID: "alice", Data: "important_data"})
    
    // Should route to the captured session PID
    actor.ShouldSend().To(sessionPID).Message("important_data").Once().Assert()
}
```

### Event Inspection for Debugging

When tests fail, you need to understand what actually happened:

```go
func TestComplexActor_DebugFailures(t *testing.T) {
    actor, _ := unit.Spawn(t, newComplexActor)
    
    // Perform some operations
    actor.SendMessage(gen.PID{}, TriggerComplexWorkflow{})
    
    // If something goes wrong, inspect all events
    events := actor.Events()
    t.Logf("Total events captured: %d", len(events))
    
    for i, event := range events {
        t.Logf("Event %d: %s - %s", i, event.Type(), event.String())
    }
    
    // Clear events and test specific behavior
    actor.ClearEvents()
    actor.SendMessage(gen.PID{}, SimpleBehavior{})
    
    // Now only simple behavior events are captured
    simpleEvents := actor.Events()
    unit.Equal(t, 1, len(simpleEvents), "Should only have one event after clearing")
}
```
## Failure Injection Testing

### Overview

The Ergo Unit Testing Library includes a failure injection system that allows you to test how your actors handle various error conditions. This is essential for building robust actor systems that can gracefully handle failures in production.

### Method Failure Injection

Access failure injection through the actor's `Process()` method:

```go
func TestActorWithFailureInjection(t *testing.T) {
    actor, err := unit.Spawn(t, factoryMyActor)
    if err != nil {
        t.Fatal(err)
    }
    
    // Inject failure for spawn operations
    actor.Process().SetMethodFailure("Spawn", errors.New("resource limit exceeded"))
    
    // Test how the actor handles spawn failures
    actor.SendMessage(gen.PID{}, CreateWorker{WorkerType: "data_processor"})
    
    // Verify the actor handles the failure gracefully
    actor.ShouldSend().MessageMatching(func(msg any) bool {
        if err, ok := msg.(WorkerCreationError); ok {
            return strings.Contains(err.Error, "resource limit exceeded")
        }
        return false
    }).Once().Assert()
}
```

### Available Failure Methods

The failure injection system provides several methods on `TestProcess`:

```go
// Fail every call to the method
actor.Process().SetMethodFailure("Send", errors.New("network error"))

// Fail only once
actor.Process().SetMethodFailureOnce("Spawn", errors.New("temporary failure"))

// Fail after N successful calls
actor.Process().SetMethodFailureAfter("Send", 3, errors.New("rate limit"))

// Fail when arguments match a pattern
actor.Process().SetMethodFailurePattern("RegisterName", "worker", errors.New("pattern match"))

// Clear specific failure
actor.Process().ClearMethodFailure("Send")

// Clear all failures
actor.Process().ClearMethodFailures()

// Get call count for a method
count := actor.Process().GetMethodCallCount("Spawn")
```

### Common Use Cases

#### Testing Spawn Failures
```go
func TestSupervisor_SpawnFailures(t *testing.T) {
    supervisor, _ := unit.Spawn(t, factorySupervisor)
    
    // Inject spawn failure
    supervisor.Process().SetMethodFailure("Spawn", errors.New("resource exhausted"))
    
    supervisor.SendMessage(gen.PID{}, StartChild{ID: "worker-1"})
    
    // Verify supervisor handles spawn failure
    supervisor.ShouldSend().MessageMatching(func(msg any) bool {
        if resp, ok := msg.(StartChildResponse); ok {
            return !resp.Success && strings.Contains(resp.Error, "resource exhausted")
        }
        return false
    }).Once().Assert()
}
```

#### Testing Message Send Failures
```go
func TestRouter_SendFailures(t *testing.T) {
    router, _ := unit.Spawn(t, factoryMessageRouter)
    
    // Inject send failure
    router.Process().SetMethodFailure("Send", errors.New("destination unreachable"))
    
    router.SendMessage(gen.PID{}, RouteMessage{
        Destination: "remote_service",
        Message:     "important_data",
    })
    
    // Verify router handles send failure
    router.ShouldSend().MessageMatching(func(msg any) bool {
        if err, ok := msg.(RoutingError); ok {
            return strings.Contains(err.Error, "destination unreachable")
        }
        return false
    }).Once().Assert()
}
```

#### Testing Intermittent Failures
```go
func TestProcessor_IntermittentFailures(t *testing.T) {
    processor, _ := unit.Spawn(t, factoryDataProcessor)
    
    // Fail after 2 successful operations
    processor.Process().SetMethodFailureAfter("Send", 2, errors.New("network timeout"))
    
    // First two sends succeed
    processor.SendMessage(gen.PID{}, ProcessData{ID: "1"})
    processor.SendMessage(gen.PID{}, ProcessData{ID: "2"})
    processor.ShouldSend().Times(2).Assert()
    
    // Third send fails
    processor.SendMessage(gen.PID{}, ProcessData{ID: "3"})
    processor.ShouldSend().MessageMatching(func(msg any) bool {
        if err, ok := msg.(ProcessingError); ok {
            return strings.Contains(err.Error, "network timeout")
        }
        return false
    }).Once().Assert()
}
```

#### Testing Pattern-Based Failures
```go
func TestRegistry_PatternFailures(t *testing.T) {
    registry, _ := unit.Spawn(t, factoryRegistry)
    
    // Fail registration for names containing "temp"
    registry.Process().SetMethodFailurePattern("RegisterName", "temp", errors.New("temporary names not allowed"))
    
    // Normal registration succeeds
    registry.SendMessage(gen.PID{}, Register{Name: "service"})
    registry.ShouldSend().Message(RegisterSuccess{Name: "service"}).Once().Assert()
    
    // Temporary registration fails
    registry.SendMessage(gen.PID{}, Register{Name: "temp_worker"})
    registry.ShouldSend().MessageMatching(func(msg any) bool {
        if err, ok := msg.(RegisterError); ok {
            return strings.Contains(err.Error, "temporary names not allowed")
        }
        return false
    }).Once().Assert()
}
```

#### Testing One-Time Failures
```go
func TestResilience_RecoveryFromFailure(t *testing.T) {
    actor, _ := unit.Spawn(t, factoryResilientActor)
    
    // Inject one-time failure
    actor.Process().SetMethodFailureOnce("Send", errors.New("temporary network error"))
    
    // First attempt fails
    actor.SendMessage(gen.PID{}, SendData{Data: "attempt1"})
    actor.ShouldSend().MessageMatching(func(msg any) bool {
        if err, ok := msg.(SendError); ok {
            return strings.Contains(err.Error, "temporary network error")
        }
        return false
    }).Once().Assert()
    
    // Second attempt succeeds (failure was one-time only)
    actor.SendMessage(gen.PID{}, SendData{Data: "attempt2"})
    actor.ShouldSend().Message(SendSuccess{Data: "attempt2"}).Once().Assert()
}
```

### Advanced Testing Scenarios

#### Testing Supervisor Restart Strategies
```go
func TestSupervisor_RestartBehavior(t *testing.T) {
    supervisor, _ := unit.Spawn(t, factoryOneForOneSupervisor)
    
    // Start children
    supervisor.SendMessage(gen.PID{}, StartChildren{Count: 3})
    supervisor.ShouldSpawn().Times(3).Assert()
    
    // Clear events before failure injection
    supervisor.ClearEvents()
    
    // Make child restarts fail after first success
    supervisor.Process().SetMethodFailureAfter("Spawn", 1, errors.New("restart failed"))
    
    // Simulate child failure requiring restart
    supervisor.SendMessage(gen.PID{}, ChildFailed{ID: "child-2"})
    
    // Verify supervisor attempts restart and handles failure
    supervisor.ShouldSpawn().Once().Assert() // First restart attempt
    supervisor.ShouldSend().MessageMatching(func(msg any) bool {
        if status, ok := msg.(SupervisorStatus); ok {
            return status.RestartsFailed == 1
        }
        return false
    }).Once().Assert()
}
```

#### Testing Method Call Tracking
```go
func TestRateLimiter_CallCounting(t *testing.T) {
    limiter, _ := unit.Spawn(t, factoryRateLimiter)
    
    // Send multiple requests
    for i := 0; i < 5; i++ {
        limiter.SendMessage(gen.PID{}, Request{ID: i})
    }
    
    // Check how many times Send was called
    sendCount := limiter.Process().GetMethodCallCount("Send")
    unit.Equal(t, 5, sendCount, "Should have called Send 5 times")
    
    // Inject failure after checking count
    limiter.Process().SetMethodFailure("Send", errors.New("rate limit exceeded"))
    
    // Next request should fail
    limiter.SendMessage(gen.PID{}, Request{ID: 6})
    limiter.ShouldSend().MessageMatching(func(msg any) bool {
        if err, ok := msg.(RateLimitError); ok {
            return err.CallCount == 6 // Should include the failed attempt
        }
        return false
    }).Once().Assert()
}
```

### Best Practices

1. **Clear Events Between Test Phases**: Use `ClearEvents()` when transitioning between test phases to avoid assertion confusion.

2. **Test Recovery**: Always test that your actors can recover after failures are cleared or when using one-time failures.

3. **Verify Call Counts**: Use `GetMethodCallCount()` to ensure methods are called the expected number of times.

4. **Pattern Matching**: Use pattern-based failures to test scenarios where only specific inputs should fail.

5. **Combine with Supervision**: Test how supervisors handle child failures by injecting spawn failures during restart attempts.

### Common Pitfalls

1. **Event Accumulation**: Events accumulate across multiple operations. Use `ClearEvents()` to reset between test phases.

2. **Timing Issues**: Some assertions may need time to complete. Use appropriate timeouts and consider async patterns.

3. **Message Ordering**: In high-throughput scenarios, message ordering might not be guaranteed. Test for this explicitly.

4. **State Leakage**: Each test should start with clean state. Don't rely on previous test state.

5. **Failure Persistence**: Remember that `SetMethodFailure` persists until cleared, while `SetMethodFailureOnce` only fails once.

## Conclusion

The Ergo Framework unit testing library provides comprehensive tools for testing actor-based systems. From simple message exchanges to complex distributed workflows, you can validate every aspect of your actor behavior with confidence.

**Key Takeaways:**

- **Start Simple**: Begin with basic message testing and gradually add complexity
- **Test Comprehensively**: Cover happy paths, error conditions, and edge cases
- **Use Fluent Assertions**: Take advantage of the readable assertion API
- **Inspect Events**: Use event inspection for debugging and understanding actor behavior
- **Organize Tests**: Structure tests by behavior and keep them focused
- **Handle Async Patterns**: Use appropriate timeouts and pattern matching for async operations

The library's zero-dependency design, comprehensive feature set, and integration with Go's testing framework make it the ideal choice for building robust, well-tested actor systems with the Ergo Framework.

**Next Steps:**
1. Explore the complete test examples in the framework repository
2. Start with simple actors and gradually build complexity
3. Integrate testing into your development workflow
4. Use the debugging features when tests fail
5. Share testing patterns with your team

Happy testing!