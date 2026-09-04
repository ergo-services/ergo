package local

import (
	"fmt"
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

type breakerOp struct {
	Kind   string
	Target any
}

type breakerResult struct{ Err error }

type breaker struct{ act.Actor }

func factoryBreaker() gen.ProcessBehavior { return &breaker{} }

func (b *breaker) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	op, ok := request.(breakerOp)
	if ok == false {
		return breakerResult{Err: gen.ErrUnsupported}, nil
	}
	return breakerResult{Err: b.apply(op)}, nil
}

func (b *breaker) apply(op breakerOp) error {
	switch op.Kind {
	case "link":
		if event, ok := op.Target.(gen.Event); ok {
			_, err := b.LinkEvent(event)
			return err
		}
		return b.Link(op.Target)

	case "unlink":
		if event, ok := op.Target.(gen.Event); ok {
			return b.UnlinkEvent(event)
		}
		return b.Unlink(op.Target)

	case "linkTyped":
		switch t := op.Target.(type) {
		case gen.PID:
			return b.LinkPID(t)
		case gen.ProcessID:
			return b.LinkProcessID(t)
		case gen.Alias:
			return b.LinkAlias(t)
		case gen.Event:
			_, err := b.LinkEvent(t)
			return err
		}

	case "unlinkTyped":
		switch t := op.Target.(type) {
		case gen.PID:
			return b.UnlinkPID(t)
		case gen.ProcessID:
			return b.UnlinkProcessID(t)
		case gen.Alias:
			return b.UnlinkAlias(t)
		case gen.Event:
			return b.UnlinkEvent(t)
		}

	case "monitor":
		if event, ok := op.Target.(gen.Event); ok {
			_, err := b.MonitorEvent(event)
			return err
		}
		return b.Monitor(op.Target)

	case "demonitor":
		if event, ok := op.Target.(gen.Event); ok {
			return b.DemonitorEvent(event)
		}
		return b.Demonitor(op.Target)

	case "monitorTyped":
		switch t := op.Target.(type) {
		case gen.PID:
			return b.MonitorPID(t)
		case gen.ProcessID:
			return b.MonitorProcessID(t)
		case gen.Alias:
			return b.MonitorAlias(t)
		case gen.Event:
			_, err := b.MonitorEvent(t)
			return err
		}

	case "demonitorTyped":
		switch t := op.Target.(type) {
		case gen.PID:
			return b.DemonitorPID(t)
		case gen.ProcessID:
			return b.DemonitorProcessID(t)
		case gen.Alias:
			return b.DemonitorAlias(t)
		case gen.Event:
			return b.DemonitorEvent(t)
		}

	case "linkNode":
		return b.LinkNode(op.Target.(gen.Atom))

	case "unlinkNode":
		return b.UnlinkNode(op.Target.(gen.Atom))

	case "monitorNode":
		return b.MonitorNode(op.Target.(gen.Atom))

	case "demonitorNode":
		return b.DemonitorNode(op.Target.(gen.Atom))
	}

	return gen.ErrUnsupported
}

func breakerDo(t *testing.T, n *stage.Node, b gen.PID, kind string, target any) error {
	t.Helper()
	result, err := n.Call(b, breakerOp{Kind: kind, Target: target})
	check.NoError(t, err)
	return result.(breakerResult).Err
}

type addressed struct {
	pid  gen.PID
	addr any
}

func spawnAddressed(t *testing.T, n *stage.Node, kind string, name gen.Atom) addressed {
	t.Helper()

	if kind == "ProcessID" {
		pid := n.SpawnRegister(name, factoryTarget, gen.ProcessOptions{})
		return addressed{pid: pid, addr: gen.ProcessID{Name: name, Node: n.Name()}}
	}

	pid := n.Spawn(factoryTarget, gen.ProcessOptions{})
	if kind == "PID" {
		return addressed{pid: pid, addr: pid}
	}

	info, err := n.Call(pid, "info")
	check.NoError(t, err)
	if kind == "Alias" {
		return addressed{pid: pid, addr: info.(targetInfo).Alias}
	}
	return addressed{pid: pid, addr: info.(targetInfo).Event}
}

var addressings = []string{"PID", "ProcessID", "Alias", "Event"}

func TestLocalUnlink(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	w := n.Spawn(factoryWatcher, gen.ProcessOptions{})

	for _, flavor := range []string{"", "Typed"} {
		for _, kind := range addressings {
			t.Run(kind+flavor, func(t *testing.T) {
				tgt := spawnAddressed(t, n, kind, gen.Atom(fmt.Sprintf("unlink-%s%s", kind, flavor)))
				b := n.Spawn(factoryBreaker, gen.ProcessOptions{})

				n.Send(w, monitorCmd{Target: b})
				n.ShouldMonitor().From(w).Target(b).Once().Within(time.Second).Must()

				check.ErrorIs(t, breakerDo(t, n, b, "unlink"+flavor, tgt.addr), gen.ErrTargetUnknown)

				check.NoError(t, breakerDo(t, n, b, "link"+flavor, tgt.addr))
				info, err := n.Native().ProcessInfo(b)
				check.NoError(t, err)
				cnt, has := linksOf(info, tgt.addr)
				check.Equal(t, 1, cnt)
				check.True(t, has)

				check.ErrorIs(t, breakerDo(t, n, b, "link"+flavor, tgt.addr), gen.ErrTargetExist)

				mk := n.Mark()
				check.NoError(t, breakerDo(t, n, b, "unlink"+flavor, tgt.addr))
				n.ShouldUnlink().From(b).Target(tgt.addr).Since(mk).Once().Within(time.Second).Must()

				info, err = n.Native().ProcessInfo(b)
				check.NoError(t, err)
				cnt, has = linksOf(info, tgt.addr)
				check.Equal(t, 0, cnt)
				check.True(t, has == false)

				n.Kill(tgt.pid)
				n.ShouldReceiveDown().To(w).About(b).Since(mk).None().Within(500 * time.Millisecond).Assert()

				if _, err := n.Native().ProcessInfo(b); err != nil {
					t.Fatalf("the unlinked process died with its former partner: %s", err)
				}
			})
		}
	}
}

func TestLocalDemonitor(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	w := n.Spawn(factoryWatcher, gen.ProcessOptions{})

	for _, flavor := range []string{"", "Typed"} {
		for _, kind := range addressings {
			t.Run(kind+flavor, func(t *testing.T) {
				tgt := spawnAddressed(t, n, kind, gen.Atom(fmt.Sprintf("demon-%s%s", kind, flavor)))
				b := n.Spawn(factoryBreaker, gen.ProcessOptions{})

				n.Send(w, monitorCmd{Target: b})
				n.ShouldMonitor().From(w).Target(b).Once().Within(time.Second).Must()

				check.ErrorIs(t, breakerDo(t, n, b, "demonitor"+flavor, tgt.addr), gen.ErrTargetUnknown)

				check.NoError(t, breakerDo(t, n, b, "monitor"+flavor, tgt.addr))
				info, err := n.Native().ProcessInfo(b)
				check.NoError(t, err)
				cnt, has := monitorsOf(info, tgt.addr)
				check.Equal(t, 1, cnt)
				check.True(t, has)

				check.ErrorIs(t, breakerDo(t, n, b, "monitor"+flavor, tgt.addr), gen.ErrTargetExist)

				mk := n.Mark()
				check.NoError(t, breakerDo(t, n, b, "demonitor"+flavor, tgt.addr))
				n.ShouldDemonitor().From(b).Target(tgt.addr).Since(mk).Once().Within(time.Second).Must()

				info, err = n.Native().ProcessInfo(b)
				check.NoError(t, err)
				cnt, has = monitorsOf(info, tgt.addr)
				check.Equal(t, 0, cnt)
				check.True(t, has == false)

				n.Kill(tgt.pid)
				downAbout(n.ShouldReceiveDown().To(b), tgt.addr).Since(mk).None().Within(500 * time.Millisecond).Assert()

				if _, err := n.Native().ProcessInfo(b); err != nil {
					t.Fatalf("the process that stopped monitoring died: %s", err)
				}
			})
		}
	}
}

func TestLocalUnlinkDemonitorNodeUnknownTarget(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	b := n.Spawn(factoryBreaker, gen.ProcessOptions{})

	check.ErrorIs(t, breakerDo(t, n, b, "unlinkNode", gen.Atom("other@localhost")), gen.ErrTargetUnknown)
	check.ErrorIs(t, breakerDo(t, n, b, "demonitorNode", gen.Atom("other@localhost")), gen.ErrTargetUnknown)
}

func TestLocalLinkMonitorSelfRefused(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	b := n.SpawnRegister("selfref", factoryBreaker, gen.ProcessOptions{})
	self := gen.ProcessID{Name: "selfref", Node: n.Name()}

	for _, kind := range []string{"link", "linkTyped", "monitor", "monitorTyped"} {
		check.ErrorIs(t, breakerDo(t, n, b, kind, b), gen.ErrNotAllowed)
		check.ErrorIs(t, breakerDo(t, n, b, kind, self), gen.ErrNotAllowed)
	}
}

func TestLocalLinkMonitorUnsupportedTarget(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	b := n.Spawn(factoryBreaker, gen.ProcessOptions{})

	check.ErrorIs(t, breakerDo(t, n, b, "link", 42), gen.ErrUnsupported)
	check.ErrorIs(t, breakerDo(t, n, b, "unlink", 42), gen.ErrUnsupported)
	check.ErrorIs(t, breakerDo(t, n, b, "monitor", 42), gen.ErrUnsupported)
	check.ErrorIs(t, breakerDo(t, n, b, "demonitor", 42), gen.ErrUnsupported)
}

func TestLocalLinkMonitorByRegisteredAtom(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	b := n.Spawn(factoryBreaker, gen.ProcessOptions{})
	n.SpawnRegister("atomtarget", factoryTarget, gen.ProcessOptions{})
	target := gen.ProcessID{Name: "atomtarget", Node: n.Name()}

	check.NoError(t, breakerDo(t, n, b, "link", gen.Atom("atomtarget")))
	check.NoError(t, breakerDo(t, n, b, "monitor", gen.Atom("atomtarget")))

	info, err := n.Native().ProcessInfo(b)
	check.NoError(t, err)
	if _, has := linksOf(info, target); has == false {
		t.Fatal("linking by registered name did not register the link")
	}
	if _, has := monitorsOf(info, target); has == false {
		t.Fatal("monitoring by registered name did not register the monitor")
	}

	check.NoError(t, breakerDo(t, n, b, "unlink", gen.Atom("atomtarget")))
	check.NoError(t, breakerDo(t, n, b, "demonitor", gen.Atom("atomtarget")))

	info, err = n.Native().ProcessInfo(b)
	check.NoError(t, err)
	if _, has := linksOf(info, target); has {
		t.Fatal("unlinking by registered name left the link in place")
	}
	if _, has := monitorsOf(info, target); has {
		t.Fatal("demonitoring by registered name left the monitor in place")
	}
}
