package local

import (
	"errors"
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// callCmd makes the caller spend its whole budget on a process that never answers.
type callCmd struct {
	Target  gen.PID
	Timeout int
}

// callResult asks the caller what its last call ended with.
type callResult struct{}

// caller performs one synchronous call on command and keeps its outcome, so the
// test can read the outcome once the caller is free again.
type caller struct {
	act.Actor
	err error
}

func factoryCaller() gen.ProcessBehavior { return &caller{} }

func (c *caller) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case callCmd:
		_, c.err = c.CallWithTimeout(m.Target, "anybody there", m.Timeout)
	}
	return nil
}

func (c *caller) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	switch request.(type) {
	case callResult:
		return c.err, nil
	}
	return nil, nil
}

// awaitWaitResponse blocks until the process is waiting for a response and returns
// its info, so a test never races the actor into the wait response state.
func awaitWaitResponse(t *testing.T, n *stage.Node, pid gen.PID) gen.ProcessInfo {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		info, err := n.Native().ProcessInfo(pid)
		if err != nil {
			t.Fatalf("process info: %s", err)
		}
		if info.State == gen.ProcessStateWaitResponse {
			return info
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("the process did not enter the wait response state")
	return gen.ProcessInfo{}
}

// TestLocalCancelWaitResponse: a process stuck on a call with a large budget is
// released by node.CancelWaitResponse. The call returns ErrCanceled, the process
// stays alive and handles the next message, and the late response is dropped.
func TestLocalCancelWaitResponse(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	silent := n.Spawn(factorySilentActor, gen.ProcessOptions{})
	pid := n.Spawn(factoryCaller, gen.ProcessOptions{})

	armed := time.Now().Unix()
	n.Send(pid, callCmd{Target: silent, Timeout: 600})
	info := awaitWaitResponse(t, n, pid)

	// the awaited ref tells how long the process still intends to wait
	if deadline := int64(info.WaitResponseRef.Deadline()); deadline < armed+600 || deadline > time.Now().Unix()+600 {
		t.Fatalf("deadline %d is not 600 seconds ahead of %d", deadline, armed)
	}

	start := time.Now()
	check.NoError(t, n.Native().CancelWaitResponse(pid, info.WaitResponseRef))

	// the caller is free again and answers what its call ended with
	result, err := n.Native().CallWithTimeout(pid, callResult{}, 5)
	check.NoError(t, err)
	failed, ok := result.(error)
	if ok == false {
		t.Fatalf("the caller reported %#v, expected an error", result)
	}
	if errors.Is(failed, gen.ErrCanceled) == false {
		t.Fatalf("the call ended with %v, expected ErrCanceled", failed)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("the caller was released after %s, expected at once", elapsed)
	}

	info, err = n.Native().ProcessInfo(pid)
	check.NoError(t, err)
	if info.WaitResponseRef != (gen.Ref{}) {
		t.Fatalf("the process is not waiting any more but publishes %s", info.WaitResponseRef)
	}
}

// TestLocalCancelWaitResponseRefused: the node refuses to cancel what it cannot
// find, what is not waiting, and a wait other than the one named by the ref.
func TestLocalCancelWaitResponseRefused(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	silent := n.Spawn(factorySilentActor, gen.ProcessOptions{})
	pid := n.Spawn(factoryCaller, gen.ProcessOptions{})

	someRef, err := n.Native().MakeRefWithDeadline(time.Now().Unix() + 600)
	check.NoError(t, err)

	err = n.Native().CancelWaitResponse(pid, someRef)
	if errors.Is(err, gen.ErrIncorrect) == false {
		t.Fatalf("cancel on an idle process returned %v, expected ErrIncorrect", err)
	}

	unknown := gen.PID{Node: n.Name(), ID: 99999, Creation: 1}
	err = n.Native().CancelWaitResponse(unknown, someRef)
	if errors.Is(err, gen.ErrProcessUnknown) == false {
		t.Fatalf("cancel on an unknown process returned %v, expected ErrProcessUnknown", err)
	}

	// waiting, but for another ref: the process keeps waiting
	n.Send(pid, callCmd{Target: silent, Timeout: 600})
	info := awaitWaitResponse(t, n, pid)

	err = n.Native().CancelWaitResponse(pid, someRef)
	if errors.Is(err, gen.ErrIncorrect) == false {
		t.Fatalf("cancel with a foreign ref returned %v, expected ErrIncorrect", err)
	}
	if again := awaitWaitResponse(t, n, pid); again.WaitResponseRef != info.WaitResponseRef {
		t.Fatal("the foreign ref ended the wait")
	}
	check.NoError(t, n.Native().CancelWaitResponse(pid, info.WaitResponseRef))
}

// TestLocalKillWaitingProcess: killing a process that waits for a response ends it
// at once instead of leaving it hanging until its budget expires.
func TestLocalKillWaitingProcess(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	silent := n.Spawn(factorySilentActor, gen.ProcessOptions{})
	pid := n.Spawn(factoryCaller, gen.ProcessOptions{})

	n.Send(pid, callCmd{Target: silent, Timeout: 600})
	awaitWaitResponse(t, n, pid)

	start := time.Now()
	check.NoError(t, n.Native().Kill(pid))

	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if _, err := n.Native().ProcessInfo(pid); errors.Is(err, gen.ErrProcessUnknown) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("the killed process is still alive %s after the kill", time.Since(start))
}
