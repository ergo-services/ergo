package node

import (
	"sync/atomic"
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/mock"
)

// Forward is documented Running-only; a caller in any non-Running state must
// get ErrNotAllowed before the message is routed.
func TestProcessForwardRunningOnly(t *testing.T) {
	for _, st := range []gen.ProcessState{
		gen.ProcessStateInit,
		gen.ProcessStateWaitResponse,
		gen.ProcessStateZombee,
		gen.ProcessStateTerminated,
	} {
		p := newEveryProcess(mock.NewCore())
		atomic.StoreInt32(&p.state, int32(st))
		if err := p.Forward(gen.PID{}, &gen.MailboxMessage{}, gen.MessagePriorityNormal); err != gen.ErrNotAllowed {
			t.Fatalf("state %v: Forward must return ErrNotAllowed, got %v", st, err)
		}
	}
}
