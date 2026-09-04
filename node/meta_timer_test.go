package node

import (
	"sync/atomic"
	"testing"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/mock"
)

var metaTimerTarget = gen.PID{Node: "n@localhost", ID: 200, Creation: 1}

// countingMeta returns a meta whose PID sends land in the returned counter.
func countingMeta(t *testing.T, sent *atomic.Int64, prio *atomic.Int32) *meta {
	core := mock.NewCoreT(t)
	core.OnRouteSendPID(func(from, to gen.PID, opts gen.MessageOptions, message any) error {
		if prio != nil {
			prio.Store(int32(opts.Priority))
		}
		sent.Add(1)
		return nil
	})
	return newTestMeta(core)
}

// SendAfter delivers the message once the timer expires and counts it as egress.
func TestMetaSendAfter(t *testing.T) {
	core := mock.NewCoreT(t)
	fired := make(chan any, 1)
	core.OnRouteSendPID(func(from, to gen.PID, opts gen.MessageOptions, message any) error {
		fired <- message
		return nil
	})

	m := newTestMeta(core)
	if _, err := m.SendAfter(metaTimerTarget, "tick", 10*time.Millisecond); err != nil {
		t.Fatal(err)
	}

	select {
	case message := <-fired:
		check.Equal(t, "tick", message)
	case <-time.After(time.Second):
		t.Fatal("SendAfter did not fire")
	}
	check.Equal(t, uint64(1), atomic.LoadUint64(&m.messagesOut))
}

// The SendAfter CancelFunc discards the pending send.
func TestMetaSendAfterCancel(t *testing.T) {
	var sent atomic.Int64
	m := countingMeta(t, &sent, nil)

	cancel, err := m.SendAfter(metaTimerTarget, "tick", 30*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if cancel() == false {
		t.Fatal("cancel returned false for a pending timer")
	}

	time.Sleep(60 * time.Millisecond)
	check.Equal(t, int64(0), sent.Load())
}

// A pending one-shot is dropped when the meta terminates before it expires.
func TestMetaSendAfterDroppedOnTerminate(t *testing.T) {
	var sent atomic.Int64
	m := countingMeta(t, &sent, nil)

	if _, err := m.SendAfter(metaTimerTarget, "tick", 20*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	atomic.StoreInt32(&m.state, int32(gen.MetaStateTerminated))

	time.Sleep(60 * time.Millisecond)
	check.Equal(t, int64(0), sent.Load())
}

// SendEvery keeps firing until cancelled.
func TestMetaSendEvery(t *testing.T) {
	var sent atomic.Int64
	m := countingMeta(t, &sent, nil)

	cancel, err := m.SendEvery(metaTimerTarget, "tick", 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(55 * time.Millisecond)
	if cancel() == false {
		t.Fatal("first cancel returned false")
	}
	if cancel() == true {
		t.Fatal("second cancel returned true")
	}
	time.Sleep(15 * time.Millisecond) // let any in-flight callback settle

	if sent.Load() == 0 {
		t.Fatal("expected periodic sends, got none")
	}
	settled := sent.Load()
	time.Sleep(30 * time.Millisecond)
	check.Equal(t, settled, sent.Load())
}

// A periodic timer stops on its own once the meta terminates.
func TestMetaSendEveryStopsOnTerminate(t *testing.T) {
	var sent atomic.Int64
	m := countingMeta(t, &sent, nil)

	if _, err := m.SendEvery(metaTimerTarget, "tick", 10*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(35 * time.Millisecond)
	if sent.Load() == 0 {
		t.Fatal("expected periodic sends, got none")
	}

	atomic.StoreInt32(&m.state, int32(gen.MetaStateTerminated))
	time.Sleep(25 * time.Millisecond)
	settled := sent.Load()
	time.Sleep(30 * time.Millisecond)
	check.Equal(t, settled, sent.Load())
}

// The WithPriority variants carry the given priority on every tick.
func TestMetaSendDeferredPriority(t *testing.T) {
	var sent atomic.Int64
	var prio atomic.Int32
	m := countingMeta(t, &sent, &prio)

	if _, err := m.SendWithPriorityAfter(metaTimerTarget, "tick", gen.MessagePriorityMax, 10*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(40 * time.Millisecond)
	check.Equal(t, int32(gen.MessagePriorityMax), prio.Load())

	cancel, err := m.SendWithPriorityEvery(metaTimerTarget, "tick", gen.MessagePriorityHigh, 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(35 * time.Millisecond)
	cancel()
	check.Equal(t, int32(gen.MessagePriorityHigh), prio.Load())
}

// Every timed-send method is rejected on a terminated meta.
func TestMetaSendDeferredTerminated(t *testing.T) {
	senders := []struct {
		name string
		call func(m *meta) (gen.CancelFunc, error)
	}{
		{"SendAfter", func(m *meta) (gen.CancelFunc, error) {
			return m.SendAfter(metaTimerTarget, "m", time.Millisecond)
		}},
		{"SendWithPriorityAfter", func(m *meta) (gen.CancelFunc, error) {
			return m.SendWithPriorityAfter(metaTimerTarget, "m", gen.MessagePriorityHigh, time.Millisecond)
		}},
		{"SendEvery", func(m *meta) (gen.CancelFunc, error) {
			return m.SendEvery(metaTimerTarget, "m", time.Millisecond)
		}},
		{"SendWithPriorityEvery", func(m *meta) (gen.CancelFunc, error) {
			return m.SendWithPriorityEvery(metaTimerTarget, "m", gen.MessagePriorityHigh, time.Millisecond)
		}},
	}

	for _, s := range senders {
		t.Run(s.name, func(t *testing.T) {
			var sent atomic.Int64
			m := countingMeta(t, &sent, nil)
			atomic.StoreInt32(&m.state, int32(gen.MetaStateTerminated))

			cancel, err := s.call(m)
			check.ErrorIs(t, err, gen.ErrNotAllowed)
			if cancel != nil {
				t.Fatal("expected no cancel func")
			}
		})
	}
}

// Timers are armed from the Sleep state, where the Start() goroutine runs.
func TestMetaSendDeferredAllowedInSleep(t *testing.T) {
	var sent atomic.Int64
	m := countingMeta(t, &sent, nil)
	atomic.StoreInt32(&m.state, int32(gen.MetaStateSleep))

	cancel, err := m.SendEvery(metaTimerTarget, "tick", 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(35 * time.Millisecond)
	cancel()

	if sent.Load() == 0 {
		t.Fatal("expected periodic sends from the sleep state, got none")
	}
}

// SendEvery rejects a non-positive period at schedule time.
func TestMetaSendEveryNonPositivePeriod(t *testing.T) {
	var sent atomic.Int64
	m := countingMeta(t, &sent, nil)

	for _, period := range []time.Duration{0, -time.Second} {
		if _, err := m.SendEvery(metaTimerTarget, "m", period); err != gen.ErrIncorrect {
			t.Fatalf("SendEvery(%s): got %v, want ErrIncorrect", period, err)
		}
		_, err := m.SendWithPriorityEvery(metaTimerTarget, "m", gen.MessagePriorityHigh, period)
		if err != gen.ErrIncorrect {
			t.Fatalf("SendWithPriorityEvery(%s): got %v, want ErrIncorrect", period, err)
		}
	}
}

// An unroutable target is rejected at schedule time, not on the timer goroutine.
// A meta routes PID, ProcessID, Alias and Atom only (a string is not a target here).
func TestMetaSendDeferredIncorrectTarget(t *testing.T) {
	var sent atomic.Int64
	m := countingMeta(t, &sent, nil)

	for _, to := range []any{"dest", 123, nil} {
		if _, err := m.SendAfter(to, "m", time.Millisecond); err != gen.ErrIncorrect {
			t.Fatalf("SendAfter(%v): got %v, want ErrIncorrect", to, err)
		}
		if _, err := m.SendEvery(to, "m", time.Millisecond); err != gen.ErrIncorrect {
			t.Fatalf("SendEvery(%v): got %v, want ErrIncorrect", to, err)
		}
	}
}

// A timed send dispatches each target type through the matching core route.
func TestMetaSendDeferredRouting(t *testing.T) {
	targets := []struct {
		name string
		to   any
	}{
		{"pid", metaTimerTarget},
		{"processid", gen.ProcessID{Name: "dest", Node: "n@localhost"}},
		{"alias", gen.Alias{Node: "n@localhost", ID: [3]uint64{9, 0, 0}}},
		{"atom", gen.Atom("dest")},
	}

	for _, target := range targets {
		t.Run(target.name, func(t *testing.T) {
			core := mock.NewCoreT(t)
			routed := make(chan string, 1)
			core.OnRouteSendPID(func(from, to gen.PID, opts gen.MessageOptions, message any) error {
				routed <- "pid"
				return nil
			})
			core.OnRouteSendProcessID(func(from gen.PID, to gen.ProcessID, opts gen.MessageOptions, message any) error {
				routed <- "processid"
				return nil
			})
			core.OnRouteSendAlias(func(from gen.PID, to gen.Alias, opts gen.MessageOptions, message any) error {
				routed <- "alias"
				return nil
			})

			m := newTestMeta(core)
			if _, err := m.SendAfter(target.to, "m", 10*time.Millisecond); err != nil {
				t.Fatal(err)
			}

			want := target.name
			if want == "atom" {
				want = "processid"
			}
			select {
			case kind := <-routed:
				check.Equal(t, want, kind)
			case <-time.After(time.Second):
				t.Fatal("timed send did not route")
			}
		})
	}
}
