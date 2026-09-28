package node

import (
	"testing"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/mock"
)

// newWaitProcess builds a process that can be driven through waitResponse.
func newWaitProcess() *process {
	p := &process{
		pid:      gen.PID{Node: "n@localhost", ID: 100, Creation: 1},
		core:     mock.NewCore(),
		node:     &node{name: "n@localhost"},
		response: make(chan response, 10),
	}
	p.compression.Store(&gen.Compression{})
	p.state = int32(gen.ProcessStateRunning)
	return p
}

type waitResult struct {
	value any
	err   error
}

// waitInBackground starts a wait and returns the channel its result lands in.
func waitInBackground(p *process, ref gen.Ref, timeout int) <-chan waitResult {
	done := make(chan waitResult, 1)
	go func() {
		value, err := p.waitResponse(ref, timeout)
		done <- waitResult{value: value, err: err}
	}()
	return done
}

// awaitWait blocks until the process is waiting for a response.
func awaitWait(t *testing.T, p *process) {
	t.Helper()
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		if p.State() == gen.ProcessStateWaitResponse {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("the process did not enter the wait response state")
}

// takeResult reads the outcome of a wait, failing if it does not finish.
func takeResult(t *testing.T, done <-chan waitResult) waitResult {
	t.Helper()
	select {
	case r := <-done:
		return r
	case <-time.After(3 * time.Second):
		t.Fatal("the wait did not finish")
	}
	return waitResult{}
}

// A cancel on the awaited ref ends the wait with ErrCanceled, and the process goes
// back to the state it waited from, publishing no wait any more.
func TestProcessWaitResponseCanceled(t *testing.T) {
	p := newWaitProcess()
	ref := gen.Ref{Node: "n@localhost", Creation: 1, ID: [3]uint64{1, 0, 0}}

	done := waitInBackground(p, ref, 60)
	awaitWait(t, p)

	if awaiting := p.awaitingRef(); awaiting != ref {
		t.Fatalf("the process publishes %s, expected %s", awaiting, ref)
	}
	if p.cancelWait(ref) == false {
		t.Fatal("the cancel did not reach the process")
	}

	r := takeResult(t, done)
	if r.err != gen.ErrCanceled {
		t.Fatalf("wait returned %v, expected ErrCanceled", r.err)
	}
	if p.State() != gen.ProcessStateRunning {
		t.Fatalf("process state is %s, expected running", p.State())
	}
	if awaiting := p.awaitingRef(); awaiting != (gen.Ref{}) {
		t.Fatalf("the finished wait still publishes %s", awaiting)
	}
}

// A cancel on a ref the process does not wait for is dropped like any late
// response, so a cancel issued for a wait that is already over cannot end the
// next one.
func TestProcessWaitResponseStaleCancelDropped(t *testing.T) {
	p := newWaitProcess()
	stale := gen.Ref{Node: "n@localhost", Creation: 1, ID: [3]uint64{1, 0, 0}}
	ref := gen.Ref{Node: "n@localhost", Creation: 1, ID: [3]uint64{2, 0, 0}}

	p.cancelWait(stale) // nothing waits for it: the cancel stays in the channel

	done := waitInBackground(p, ref, 60)
	awaitWait(t, p)

	// and one that arrives while the next wait is in progress
	p.cancelWait(stale)

	select {
	case r := <-done:
		t.Fatalf("a stale cancel ended the current wait: %v", r.err)
	case <-time.After(100 * time.Millisecond):
	}

	p.response <- response{ref: ref, message: "pong"}
	r := takeResult(t, done)
	if r.err != nil {
		t.Fatalf("wait returned %v, expected the response", r.err)
	}
	if r.value != "pong" {
		t.Fatalf("wait returned %v, expected pong", r.value)
	}
}

// A wait that nothing interrupts still gives up on its own budget.
func TestProcessWaitResponseTimeout(t *testing.T) {
	p := newWaitProcess()
	ref := gen.Ref{Node: "n@localhost", Creation: 1, ID: [3]uint64{1, 0, 0}}

	start := time.Now()
	r := takeResult(t, waitInBackground(p, ref, 1))
	if r.err != gen.ErrTimeout {
		t.Fatalf("wait returned %v, expected ErrTimeout", r.err)
	}
	if elapsed := time.Since(start); elapsed < 900*time.Millisecond {
		t.Fatalf("the wait gave up after %s, expected about a second", elapsed)
	}
}
