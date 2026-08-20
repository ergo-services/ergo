package unit_test

import (
	"testing"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/unit"
)

// timerMeta arms both a one-shot timer addressed to its parent and a periodic
// timer addressed to itself.
type timerMeta struct {
	mp    gen.MetaProcess
	ticks int
}

func (m *timerMeta) Init(process gen.MetaProcess) error {
	m.mp = process
	if _, err := m.mp.SendAfter(m.mp.Parent(), "late", time.Second); err != nil {
		return err
	}
	_, err := m.mp.SendEvery(m.mp.ID(), "tick", 100*time.Millisecond)
	return err
}

func (m *timerMeta) Start() error { return nil }

func (m *timerMeta) HandleMessage(from gen.PID, message any) error {
	if message == "tick" {
		m.ticks++
	}
	return nil
}

func (m *timerMeta) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, nil
}

func (m *timerMeta) Terminate(reason error) {}

func (m *timerMeta) HandleInspect(from gen.PID, item ...string) map[string]string {
	return nil
}

// A meta's timed sends are recorded (as originating from the parent PID) and only
// delivered when the test fires timers: self-addressed ones through the meta's own
// FireTimers, parent-addressed ones through the parent Subject's.
func TestMetaTimers(t *testing.T) {
	sub, err := unit.Spawn(t, factoryMetaParent, gen.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}
	m, err := sub.SpawnMeta(&timerMeta{}, gen.MetaOptions{})
	if err != nil {
		t.Fatal(err)
	}

	m.ShouldSendAfter().From(sub.PID()).To(sub.PID()).Message("late").After(time.Second).Once().Assert()
	m.ShouldSendEvery().From(sub.PID()).To(m.ID()).Message("tick").Period(100 * time.Millisecond).Once().Assert()
	m.ShouldSend().None().Assert()

	check.Equal(t, 1, m.FireTimers())
	check.Equal(t, 1, m.Behavior().(*timerMeta).ticks)

	// the parent-addressed one-shot is the parent's to fire
	check.Equal(t, 1, sub.FireTimers())
	check.Equal(t, 0, m.FireTimers())
}

// A terminated meta rejects timed sends.
func TestMetaTimersTerminated(t *testing.T) {
	sub, _ := unit.Spawn(t, factoryMetaParent, gen.ProcessOptions{})
	m, err := sub.SpawnMeta(&timerMeta{}, gen.MetaOptions{})
	if err != nil {
		t.Fatal(err)
	}
	m.Terminate(gen.TerminateReasonNormal)

	behavior := m.Behavior().(*timerMeta)
	if _, err := behavior.mp.SendAfter(sub.PID(), "late", time.Second); err != gen.ErrNotAllowed {
		t.Fatalf("SendAfter on a terminated meta: got %v, want ErrNotAllowed", err)
	}
	if _, err := behavior.mp.SendEvery(sub.PID(), "tick", time.Second); err != gen.ErrNotAllowed {
		t.Fatalf("SendEvery on a terminated meta: got %v, want ErrNotAllowed", err)
	}
}
