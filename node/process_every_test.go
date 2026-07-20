package node

import (
	"sync/atomic"
	"testing"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/mock"
)

func newEveryProcess(core gen.Core) *process {
	p := &process{
		pid:  gen.PID{Node: "n@localhost", ID: 100, Creation: 1},
		core: core,
		node: &node{name: "n@localhost"}, // gen.Atom target resolves to a ProcessID on this node
	}
	p.state = int32(gen.ProcessStateRunning)
	return p
}

// SendEvery fires the message to self repeatedly until cancelled.
func TestProcessSendEvery(t *testing.T) {
	core := mock.NewCoreT(t)
	var ticks atomic.Int64
	core.OnRouteSendPID(func(from, to gen.PID, opts gen.MessageOptions, message any) error {
		if message == "tick" && from == to {
			ticks.Add(1)
		}
		return nil
	})

	p := newEveryProcess(core)
	cancel, err := p.SendEvery(p.pid, "tick", 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(55 * time.Millisecond)
	cancel()
	time.Sleep(15 * time.Millisecond) // let any in-flight callback settle

	if ticks.Load() == 0 {
		t.Fatal("expected periodic self-sends, got none")
	}

	// after cancel no further ticks
	settled := ticks.Load()
	time.Sleep(30 * time.Millisecond)
	if ticks.Load() != settled {
		t.Fatalf("ticker kept firing after cancel: %d -> %d", settled, ticks.Load())
	}
}

// SendWithPriorityEvery propagates the priority on every tick.
func TestProcessSendWithPriorityEvery(t *testing.T) {
	core := mock.NewCoreT(t)
	var ticks atomic.Int64
	var prio atomic.Int32
	core.OnRouteSendPID(func(from, to gen.PID, opts gen.MessageOptions, message any) error {
		prio.Store(int32(opts.Priority))
		ticks.Add(1)
		return nil
	})

	p := newEveryProcess(core)
	cancel, err := p.SendWithPriorityEvery(p.pid, "tick", gen.MessagePriorityHigh, 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(35 * time.Millisecond)
	cancel()
	time.Sleep(15 * time.Millisecond)

	if ticks.Load() == 0 {
		t.Fatal("expected periodic self-sends, got none")
	}
	if prio.Load() != int32(gen.MessagePriorityHigh) {
		t.Fatalf("priority not propagated: got %d, want %d", prio.Load(), gen.MessagePriorityHigh)
	}
}

// routeKind identifies which core.RouteSend* method a timed send dispatched to.
type routeKind int32

const (
	kindNone routeKind = iota
	kindPID
	kindProcessID
	kindAlias
)

// TestProcessTimedSendRouting: every timed-send method dispatches each `to` type
// through the matching core.RouteSend* method and carries the chosen priority.
// gen.Atom is resolved to a ProcessID on the owner's node.
func TestProcessTimedSendRouting(t *testing.T) {
	targets := []struct {
		name string
		to   any
		want routeKind
	}{
		{"pid", gen.PID{Node: "n@localhost", ID: 200, Creation: 1}, kindPID},
		{"processid", gen.ProcessID{Name: "dest", Node: "n@localhost"}, kindProcessID},
		{"alias", gen.Alias{Node: "n@localhost"}, kindAlias},
		{"atom", gen.Atom("dest"), kindProcessID},
	}

	senders := []struct {
		name         string
		call         func(p *process, to any) (gen.CancelFunc, error)
		wantPriority gen.MessagePriority
	}{
		{"SendAfter", func(p *process, to any) (gen.CancelFunc, error) {
			return p.SendAfter(to, "m", 5*time.Millisecond)
		}, gen.MessagePriorityNormal},
		{"SendWithPriorityAfter", func(p *process, to any) (gen.CancelFunc, error) {
			return p.SendWithPriorityAfter(to, "m", gen.MessagePriorityMax, 5*time.Millisecond)
		}, gen.MessagePriorityMax},
		{"SendEvery", func(p *process, to any) (gen.CancelFunc, error) {
			return p.SendEvery(to, "m", 8*time.Millisecond)
		}, gen.MessagePriorityNormal},
		{"SendWithPriorityEvery", func(p *process, to any) (gen.CancelFunc, error) {
			return p.SendWithPriorityEvery(to, "m", gen.MessagePriorityHigh, 8*time.Millisecond)
		}, gen.MessagePriorityHigh},
	}

	for _, s := range senders {
		for _, tg := range targets {
			t.Run(s.name+"/"+tg.name, func(t *testing.T) {
				var kind atomic.Int32
				var prio atomic.Int32
				core := mock.NewCore() // no-op recorder: a tick may fire after the subtest returns
				core.OnRouteSendPID(func(from, to gen.PID, opts gen.MessageOptions, message any) error {
					prio.Store(int32(opts.Priority))
					kind.Store(int32(kindPID))
					return nil
				})
				core.OnRouteSendProcessID(func(from gen.PID, to gen.ProcessID, opts gen.MessageOptions, message any) error {
					prio.Store(int32(opts.Priority))
					kind.Store(int32(kindProcessID))
					return nil
				})
				core.OnRouteSendAlias(func(from gen.PID, to gen.Alias, opts gen.MessageOptions, message any) error {
					prio.Store(int32(opts.Priority))
					kind.Store(int32(kindAlias))
					return nil
				})

				p := newEveryProcess(core)
				cancel, err := s.call(p, tg.to)
				if err != nil {
					t.Fatal(err)
				}
				time.Sleep(40 * time.Millisecond)
				cancel()
				time.Sleep(15 * time.Millisecond)

				if routeKind(kind.Load()) != tg.want {
					t.Fatalf("routed via kind %d, want %d", kind.Load(), tg.want)
				}
				if gen.MessagePriority(prio.Load()) != s.wantPriority {
					t.Fatalf("priority %d, want %d", prio.Load(), s.wantPriority)
				}
			})
		}
	}
}

// TestProcessEveryStopsWhenOwnerNotAlive: once the owning process is no longer
// alive, the next tick stops the ticker instead of delivering.
func TestProcessEveryStopsWhenOwnerNotAlive(t *testing.T) {
	for _, tc := range []struct {
		name string
		arm  func(p *process) (gen.CancelFunc, error)
	}{
		{"SendEvery", func(p *process) (gen.CancelFunc, error) {
			return p.SendEvery(p.pid, "tick", 10*time.Millisecond)
		}},
		{"SendWithPriorityEvery", func(p *process) (gen.CancelFunc, error) {
			return p.SendWithPriorityEvery(p.pid, "tick", gen.MessagePriorityHigh, 10*time.Millisecond)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			core := mock.NewCore()
			var ticks atomic.Int64
			core.OnRouteSendPID(func(from, to gen.PID, opts gen.MessageOptions, message any) error {
				ticks.Add(1)
				return nil
			})

			p := newEveryProcess(core)
			cancel, err := tc.arm(p)
			if err != nil {
				t.Fatal(err)
			}
			defer cancel()

			// owner dies before the first tick fires
			atomic.StoreInt32(&p.state, int32(gen.ProcessStateTerminated))
			time.Sleep(50 * time.Millisecond)
			settled := ticks.Load()
			time.Sleep(30 * time.Millisecond)
			if ticks.Load() != settled {
				t.Fatalf("ticker kept firing after owner death: %d -> %d", settled, ticks.Load())
			}
		})
	}
}

// The timed-send methods are rejected outside the Init/Running states.
func TestProcessTimedSendNotAllowed(t *testing.T) {
	calls := []struct {
		name string
		call func(p *process) (gen.CancelFunc, error)
	}{
		{"SendAfter", func(p *process) (gen.CancelFunc, error) {
			return p.SendAfter(p.pid, "m", time.Millisecond)
		}},
		{"SendWithPriorityAfter", func(p *process) (gen.CancelFunc, error) {
			return p.SendWithPriorityAfter(p.pid, "m", gen.MessagePriorityHigh, time.Millisecond)
		}},
		{"SendEvery", func(p *process) (gen.CancelFunc, error) {
			return p.SendEvery(p.pid, "m", time.Millisecond)
		}},
		{"SendWithPriorityEvery", func(p *process) (gen.CancelFunc, error) {
			return p.SendWithPriorityEvery(p.pid, "m", gen.MessagePriorityHigh, time.Millisecond)
		}},
	}
	for _, c := range calls {
		t.Run(c.name, func(t *testing.T) {
			p := newEveryProcess(mock.NewCore())
			p.state = int32(gen.ProcessStateTerminated)
			if _, err := c.call(p); err != gen.ErrNotAllowed {
				t.Fatalf("expected ErrNotAllowed, got %v", err)
			}
		})
	}
}
