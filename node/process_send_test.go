package node

import (
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/mock"
)

// TestProcessSendRouting: Send / SendWithPriority / SendImportant dispatch each `to`
// type through the matching core.RouteSend* and carry the chosen priority/important.
// Targets are local, so an important send returns without waiting for a remote ack.
func TestProcessSendRouting(t *testing.T) {
	targets := []struct {
		name string
		to   any
		want routeKind
	}{
		{"pid", gen.PID{Node: "n@localhost", ID: 200, Creation: 1}, kindPID},
		{"processid", gen.ProcessID{Name: "dest", Node: "n@localhost"}, kindProcessID},
		{"alias", gen.Alias{Node: "n@localhost"}, kindAlias},
		{"atom", gen.Atom("dest"), kindProcessID},
		{"string", "dest", kindProcessID},
	}
	senders := []struct {
		name          string
		call          func(p *process, to any) error
		wantPriority  gen.MessagePriority
		wantImportant bool
	}{
		{"Send", func(p *process, to any) error {
			return p.Send(to, "m")
		}, gen.MessagePriorityNormal, false},
		{"SendWithPriority", func(p *process, to any) error {
			return p.SendWithPriority(to, "m", gen.MessagePriorityMax)
		}, gen.MessagePriorityMax, false},
		{"SendImportant", func(p *process, to any) error {
			return p.SendImportant(to, "m")
		}, gen.MessagePriorityNormal, true},
	}

	for _, s := range senders {
		for _, tg := range targets {
			t.Run(s.name+"/"+tg.name, func(t *testing.T) {
				var kind routeKind
				var opts gen.MessageOptions
				core := mock.NewCore()
				core.OnRouteSendPID(func(from, to gen.PID, o gen.MessageOptions, m any) error {
					kind, opts = kindPID, o
					return nil
				})
				core.OnRouteSendProcessID(func(from gen.PID, to gen.ProcessID, o gen.MessageOptions, m any) error {
					kind, opts = kindProcessID, o
					return nil
				})
				core.OnRouteSendAlias(func(from gen.PID, to gen.Alias, o gen.MessageOptions, m any) error {
					kind, opts = kindAlias, o
					return nil
				})

				p := newEveryProcess(core)
				if err := s.call(p, tg.to); err != nil {
					t.Fatal(err)
				}
				if kind != tg.want {
					t.Fatalf("routed via kind %d, want %d", kind, tg.want)
				}
				if opts.Priority != s.wantPriority {
					t.Fatalf("priority %d, want %d", opts.Priority, s.wantPriority)
				}
				if opts.ImportantDelivery != s.wantImportant {
					t.Fatalf("important %v, want %v", opts.ImportantDelivery, s.wantImportant)
				}
			})
		}
	}
}

// TestProcessCallRouting: Call / CallWithPriority / CallImportant dispatch each `to`
// type through the matching core.RouteCall* and carry the chosen priority/important.
// The mock returns an error so the call returns before waiting for a response.
func TestProcessCallRouting(t *testing.T) {
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
		name          string
		call          func(p *process, to any) (any, error)
		wantPriority  gen.MessagePriority
		wantImportant bool
	}{
		{"Call", func(p *process, to any) (any, error) {
			return p.Call(to, "m")
		}, gen.MessagePriorityNormal, false},
		{"CallWithPriority", func(p *process, to any) (any, error) {
			return p.CallWithPriority(to, "m", gen.MessagePriorityMax)
		}, gen.MessagePriorityMax, false},
		{"CallImportant", func(p *process, to any) (any, error) {
			return p.CallImportant(to, "m")
		}, gen.MessagePriorityNormal, true},
	}

	for _, s := range senders {
		for _, tg := range targets {
			t.Run(s.name+"/"+tg.name, func(t *testing.T) {
				var kind routeKind
				var opts gen.MessageOptions
				core := mock.NewCore()
				core.OnRouteCallPID(func(from, to gen.PID, o gen.MessageOptions, m any) error {
					kind, opts = kindPID, o
					return gen.ErrProcessUnknown
				})
				core.OnRouteCallProcessID(func(from gen.PID, to gen.ProcessID, o gen.MessageOptions, m any) error {
					kind, opts = kindProcessID, o
					return gen.ErrProcessUnknown
				})
				core.OnRouteCallAlias(func(from gen.PID, to gen.Alias, o gen.MessageOptions, m any) error {
					kind, opts = kindAlias, o
					return gen.ErrProcessUnknown
				})

				p := newEveryProcess(core)
				if _, err := s.call(p, tg.to); err != gen.ErrProcessUnknown {
					t.Fatalf("expected the routed error, got %v", err)
				}
				if kind != tg.want {
					t.Fatalf("routed via kind %d, want %d", kind, tg.want)
				}
				if opts.Priority != s.wantPriority {
					t.Fatalf("priority %d, want %d", opts.Priority, s.wantPriority)
				}
				if opts.ImportantDelivery != s.wantImportant {
					t.Fatalf("important %v, want %v", opts.ImportantDelivery, s.wantImportant)
				}
				if opts.Ref == (gen.Ref{}) {
					t.Fatal("call must carry a non-zero response ref")
				}
			})
		}
	}
}

// TestProcessSendResponseRouting: SendResponse / SendResponseError route through the
// matching core method carrying the given ref; important defaults to the process flag
// (false here, so neither waits for an ack).
func TestProcessSendResponseRouting(t *testing.T) {
	ref := gen.Ref{Node: "n@localhost", Creation: 1, ID: [3]uint64{7, 0, 0}}
	to := gen.PID{Node: "n@localhost", ID: 200, Creation: 1}

	t.Run("SendResponse", func(t *testing.T) {
		var opts gen.MessageOptions
		core := mock.NewCore()
		core.OnRouteSendResponse(func(from, to gen.PID, o gen.MessageOptions, m any) error {
			opts = o
			return nil
		})
		p := newEveryProcess(core)
		if err := p.SendResponse(to, ref, "m"); err != nil {
			t.Fatal(err)
		}
		if opts.Ref != ref {
			t.Fatalf("ref %v, want %v", opts.Ref, ref)
		}
		if opts.ImportantDelivery != false {
			t.Fatal("important must default to false")
		}
	})

	t.Run("SendResponseError", func(t *testing.T) {
		var opts gen.MessageOptions
		core := mock.NewCore()
		core.OnRouteSendResponseError(func(from, to gen.PID, o gen.MessageOptions, e error) error {
			opts = o
			return nil
		})
		p := newEveryProcess(core)
		if err := p.SendResponseError(to, ref, gen.ErrTimeout); err != nil {
			t.Fatal(err)
		}
		if opts.Ref != ref {
			t.Fatalf("ref %v, want %v", opts.Ref, ref)
		}
	})
}

// TestProcessSendEventRouting: SendEvent routes through core.RouteSendEvent with the token.
func TestProcessSendEventRouting(t *testing.T) {
	token := gen.Ref{Node: "n@localhost", Creation: 1, ID: [3]uint64{9, 0, 0}}
	var got gen.Ref
	core := mock.NewCore()
	core.OnRouteSendEvent(func(from gen.PID, tok gen.Ref, o gen.MessageOptions, m gen.MessageEvent) error {
		got = tok
		return nil
	})
	p := newEveryProcess(core)
	if err := p.SendEvent(gen.Atom("evt"), token, "m"); err != nil {
		t.Fatal(err)
	}
	if got != token {
		t.Fatalf("token %v, want %v", got, token)
	}
}

// TestProcessTypedEntryPoints: the typed public Send/Call methods are bypassed by the
// generic Send/Call dispatch (which calls the cores directly), so exercise each one
// directly to confirm it routes to the matching core method.
func TestProcessTypedEntryPoints(t *testing.T) {
	pid := gen.PID{Node: "n@localhost", ID: 200, Creation: 1}
	procid := gen.ProcessID{Name: "dest", Node: "n@localhost"}
	alias := gen.Alias{Node: "n@localhost"}

	for _, tc := range []struct {
		name string
		want routeKind
		call func(p *process) error
	}{
		{"SendPID", kindPID, func(p *process) error { return p.SendPID(pid, "m") }},
		{"SendProcessID", kindProcessID, func(p *process) error { return p.SendProcessID(procid, "m") }},
		{"SendAlias", kindAlias, func(p *process) error { return p.SendAlias(alias, "m") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var kind routeKind
			core := mock.NewCore()
			core.OnRouteSendPID(func(from, to gen.PID, o gen.MessageOptions, m any) error { kind = kindPID; return nil })
			core.OnRouteSendProcessID(func(from gen.PID, to gen.ProcessID, o gen.MessageOptions, m any) error {
				kind = kindProcessID
				return nil
			})
			core.OnRouteSendAlias(func(from gen.PID, to gen.Alias, o gen.MessageOptions, m any) error { kind = kindAlias; return nil })
			p := newEveryProcess(core)
			if err := tc.call(p); err != nil {
				t.Fatal(err)
			}
			if kind != tc.want {
				t.Fatalf("routed via %d, want %d", kind, tc.want)
			}
		})
	}

	for _, tc := range []struct {
		name string
		want routeKind
		call func(p *process) (any, error)
	}{
		{"CallPID", kindPID, func(p *process) (any, error) { return p.CallPID(pid, "m", 5) }},
		{"CallProcessID", kindProcessID, func(p *process) (any, error) { return p.CallProcessID(procid, "m", 5) }},
		{"CallAlias", kindAlias, func(p *process) (any, error) { return p.CallAlias(alias, "m", 5) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var kind routeKind
			core := mock.NewCore()
			core.OnRouteCallPID(func(from, to gen.PID, o gen.MessageOptions, m any) error {
				kind = kindPID
				return gen.ErrProcessUnknown
			})
			core.OnRouteCallProcessID(func(from gen.PID, to gen.ProcessID, o gen.MessageOptions, m any) error {
				kind = kindProcessID
				return gen.ErrProcessUnknown
			})
			core.OnRouteCallAlias(func(from gen.PID, to gen.Alias, o gen.MessageOptions, m any) error {
				kind = kindAlias
				return gen.ErrProcessUnknown
			})
			p := newEveryProcess(core)
			if _, err := tc.call(p); err != gen.ErrProcessUnknown {
				t.Fatalf("expected the routed error, got %v", err)
			}
			if kind != tc.want {
				t.Fatalf("routed via %d, want %d", kind, tc.want)
			}
		})
	}
}

// TestProcessSendCallUnsupportedTarget: an unsupported target type is rejected by the
// generic dispatch with gen.ErrUnsupported.
func TestProcessSendCallUnsupportedTarget(t *testing.T) {
	p := newEveryProcess(mock.NewCore())
	if err := p.Send(123, "m"); err != gen.ErrUnsupported {
		t.Fatalf("Send unsupported: got %v, want ErrUnsupported", err)
	}
	if _, err := p.Call(123, "m"); err != gen.ErrUnsupported {
		t.Fatalf("Call unsupported: got %v, want ErrUnsupported", err)
	}
}
