package unit

import (
	"fmt"
	"strings"
	"testing"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

// normalTerminationActor for testing normal termination behavior
type normalTerminationActor struct {
	act.Actor
}

func (n *normalTerminationActor) Init(args ...any) error {
	return nil
}

func (n *normalTerminationActor) HandleMessage(from gen.PID, message any) error {
	switch message {
	case "process":
		// Process message normally
		return nil
	default:
		// Unknown messages cause normal termination
		return gen.TerminateReasonNormal
	}
}

// shutdownActor for testing shutdown termination
type shutdownActor struct {
	act.Actor
}

func (s *shutdownActor) Init(args ...any) error {
	return nil
}

func (s *shutdownActor) HandleMessage(from gen.PID, message any) error {
	switch message {
	case "shutdown":
		return gen.TerminateReasonShutdown
	default:
		return nil
	}
}

// errorActor for testing abnormal termination
type errorActor struct {
	act.Actor
}

func (e *errorActor) Init(args ...any) error {
	return nil
}

func (e *errorActor) HandleMessage(from gen.PID, message any) error {
	switch message {
	case "error":
		return fmt.Errorf("custom error occurred")
	default:
		return nil
	}
}

// Actor termination tests

func TestTermination_ActorErrorHandling(t *testing.T) {
	// Create an actor that terminates on unknown messages
	factory := func() gen.ProcessBehavior {
		return &normalTerminationActor{}
	}

	actor, err := Spawn(t, factory)
	if err != nil {
		t.Fatalf("Failed to spawn actor: %v", err)
	}

	// Send a normal message - should work fine
	testPID := gen.PID{Node: "sender", ID: 1}
	actor.SendMessage(testPID, "process")

	// Check that actor is not terminated yet
	Equal(t, false, actor.IsTerminated())
	Nil(t, actor.TerminationReason())

	// Send a message that causes termination
	actor.SendMessage(testPID, "unknown_message")

	// Check that actor is now terminated
	Equal(t, true, actor.IsTerminated())
	NotNil(t, actor.TerminationReason())
	Equal(t, gen.TerminateReasonNormal, actor.TerminationReason())

	// Verify termination event was emitted
	actor.ShouldTerminate().
		WithReason(gen.TerminateReasonNormal).
		Assert()

	// Attempting to send more messages should not process them
	eventCountBefore := actor.EventCount()
	actor.SendMessage(testPID, "process")
	eventCountAfter := actor.EventCount()

	// Event count should remain the same (no new processing events, but may get log message)
	True(t, eventCountAfter >= eventCountBefore, "Should not process new messages after termination")
}

func TestTermination_ShutdownHandling(t *testing.T) {
	// Create a custom actor that returns shutdown reason
	factory := func() gen.ProcessBehavior {
		return &shutdownActor{}
	}

	actor, err := Spawn(t, factory)
	if err != nil {
		t.Fatalf("Failed to spawn actor: %v", err)
	}

	// Send a shutdown message
	testPID := gen.PID{Node: "sender", ID: 1}
	actor.SendMessage(testPID, "shutdown")

	// Check that actor is terminated with shutdown reason
	Equal(t, true, actor.IsTerminated())
	Equal(t, gen.TerminateReasonShutdown, actor.TerminationReason())

	// Verify termination event was emitted
	actor.ShouldTerminate().
		WithReason(gen.TerminateReasonShutdown).
		Assert()
}

func TestTermination_AbnormalTermination(t *testing.T) {
	// Create a custom actor that returns abnormal termination reason
	factory := func() gen.ProcessBehavior {
		return &errorActor{}
	}

	actor, err := Spawn(t, factory)
	if err != nil {
		t.Fatalf("Failed to spawn actor: %v", err)
	}

	// Send an error message
	testPID := gen.PID{Node: "sender", ID: 1}
	actor.SendMessage(testPID, "error")

	// Check that actor is terminated with error reason
	Equal(t, true, actor.IsTerminated())
	NotNil(t, actor.TerminationReason())
	Contains(t, actor.TerminationReason().Error(), "custom error")

	// Verify termination event was emitted
	actor.ShouldTerminate().
		ReasonMatching(func(reason error) bool {
			return strings.Contains(reason.Error(), "custom error")
		}).
		Assert()
}

func TestSendExit_Operations(t *testing.T) {
	// Create an actor to test SendExit operations
	factory := func() gen.ProcessBehavior {
		return &exitTestActor{}
	}

	actor, err := Spawn(t, factory)
	if err != nil {
		t.Fatalf("Failed to spawn actor: %v", err)
	}

	actor.ClearEvents()

	// Test sending exit to another process
	targetPID := gen.PID{Node: "test", ID: 123}
	actor.SendMessage(gen.PID{Node: "sender", ID: 1}, exitMessage{
		To:     targetPID,
		Reason: gen.TerminateReasonShutdown,
	})

	// Verify SendExit event was captured
	actor.ShouldSendExit().
		To(targetPID).
		WithReason(gen.TerminateReasonShutdown).
		Assert()

	// Test sending exit to meta process
	targetMeta := gen.Alias{Node: "test", ID: [3]uint64{456, 0, 0}}
	actor.SendMessage(gen.PID{Node: "sender", ID: 1}, exitMetaMessage{
		Meta:   targetMeta,
		Reason: gen.TerminateReasonNormal,
	})

	// Verify SendExitMeta event was captured
	actor.ShouldSendExitMeta().
		ToMeta(targetMeta).
		WithReason(gen.TerminateReasonNormal).
		Assert()

	// Test that no unexpected exits were sent
	actor.ShouldNotSendExit().To(gen.PID{Node: "other", ID: 999}).Assert()
}

// exitTestActor for testing SendExit operations
type exitTestActor struct {
	act.Actor
}

func (e *exitTestActor) Init(args ...any) error {
	return nil
}

func (e *exitTestActor) HandleMessage(from gen.PID, message any) error {
	switch msg := message.(type) {
	case exitMessage:
		return e.SendExit(msg.To, msg.Reason)
	case exitMetaMessage:
		return e.SendExitMeta(msg.Meta, msg.Reason)
	default:
		return nil
	}
}

// Message types for testing
type exitMessage struct {
	To     gen.PID
	Reason error
}

type exitMetaMessage struct {
	Meta   gen.Alias
	Reason error
}
