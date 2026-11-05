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

const registerRaceSpawns = 20000

type registerRaceMode int

const (
	registerRaceMonitorPID registerRaceMode = iota
	registerRaceLinkPID
	registerRaceMonitorProcessID
	registerRaceMonitorEvent
	registerRaceLinkProcessID
	registerRaceLinkEvent
)

type registerRaceBatch struct{}

type registerRaceTally struct {
	spawned int
	failed  int
	pending int
	extra   int
}

// instantProducer registers the event named by its argument, then dies on its first
// message, taking the event down with it.
type instantProducer struct{ act.Actor }

func factoryInstantProducer() gen.ProcessBehavior { return &instantProducer{} }

func (p *instantProducer) Init(args ...any) error {
	if _, err := p.RegisterEvent(args[0].(gen.Atom), gen.EventOptions{}); err != nil {
		return err
	}
	return p.Send(p.PID(), "stop")
}

func (p *instantProducer) HandleMessage(from gen.PID, message any) error {
	return gen.TerminateReasonNormal
}

// registerRaceWatcher spawns members that die on their first message and links or
// monitors each one right after Spawn, so the registration races the member's
// termination. pending holds targets registered successfully and not yet reported.
type registerRaceWatcher struct {
	act.Actor
	mode    registerRaceMode
	spawned int
	failed  int
	extra   int
	pending map[any]bool
}

func factoryRegisterRaceWatcher() gen.ProcessBehavior { return &registerRaceWatcher{} }

func (w *registerRaceWatcher) Init(args ...any) error {
	w.mode = args[0].(registerRaceMode)
	w.pending = make(map[any]bool)
	w.SetTrapExit(w.mode == registerRaceLinkPID || w.mode == registerRaceLinkProcessID || w.mode == registerRaceLinkEvent)
	return nil
}

func (w *registerRaceWatcher) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case registerRaceBatch:
		for i := 0; i < 100 && w.spawned < registerRaceSpawns; i++ {
			w.spawned++
			name := gen.Atom(fmt.Sprintf("race_member_%d", w.spawned))
			var target any
			var err error
			switch w.mode {
			case registerRaceMonitorPID:
				pid, serr := w.Spawn(factoryInstantMember, gen.ProcessOptions{})
				if serr != nil {
					return serr
				}
				target = pid
				err = w.MonitorPID(pid)
			case registerRaceLinkPID:
				pid, serr := w.Spawn(factoryInstantMember, gen.ProcessOptions{})
				if serr != nil {
					return serr
				}
				target = pid
				err = w.LinkPID(pid)
			case registerRaceMonitorProcessID:
				if _, serr := w.SpawnRegister(name, factoryInstantMember, gen.ProcessOptions{}); serr != nil {
					return serr
				}
				process := gen.ProcessID{Name: name, Node: w.Node().Name()}
				target = process
				err = w.MonitorProcessID(process)
			case registerRaceMonitorEvent:
				if _, serr := w.Spawn(factoryInstantProducer, gen.ProcessOptions{}, name); serr != nil {
					return serr
				}
				event := gen.Event{Name: name, Node: w.Node().Name()}
				target = event
				_, err = w.MonitorEvent(event)
			case registerRaceLinkProcessID:
				if _, serr := w.SpawnRegister(name, factoryInstantMember, gen.ProcessOptions{}); serr != nil {
					return serr
				}
				process := gen.ProcessID{Name: name, Node: w.Node().Name()}
				target = process
				err = w.LinkProcessID(process)
			case registerRaceLinkEvent:
				if _, serr := w.Spawn(factoryInstantProducer, gen.ProcessOptions{}, name); serr != nil {
					return serr
				}
				event := gen.Event{Name: name, Node: w.Node().Name()}
				target = event
				_, err = w.LinkEvent(event)
			}
			if err != nil {
				w.failed++
				continue
			}
			w.pending[target] = true
		}
		if w.spawned < registerRaceSpawns {
			return w.Send(w.PID(), registerRaceBatch{})
		}

	case gen.MessageDownPID:
		w.settle(m.PID)
	case gen.MessageExitPID:
		w.settle(m.PID)
	case gen.MessageDownProcessID:
		w.settle(m.ProcessID)
	case gen.MessageDownEvent:
		w.settle(m.Event)
	case gen.MessageExitProcessID:
		w.settle(m.ProcessID)
	case gen.MessageExitEvent:
		w.settle(m.Event)
	}
	return nil
}

func (w *registerRaceWatcher) settle(target any) {
	if w.pending[target] == false {
		w.extra++
		return
	}
	delete(w.pending, target)
}

func (w *registerRaceWatcher) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return registerRaceTally{
		spawned: w.spawned,
		failed:  w.failed,
		pending: len(w.pending),
		extra:   w.extra,
	}, nil
}

// runRegisterRace: every registration that succeeded on a dying member must be
// followed by exactly one exit/down, and a failed one by none.
func runRegisterRace(t *testing.T, mode registerRaceMode) {
	s := stage.New(t)
	n := s.StartNode("n")
	w := n.Spawn(factoryRegisterRaceWatcher, gen.ProcessOptions{}, mode)
	n.Send(w, registerRaceBatch{})

	var tally registerRaceTally
	deadline := time.Now().Add(30 * time.Second)
	for {
		v, err := n.Call(w, registerRaceTally{})
		check.NoError(t, err)
		tally = v.(registerRaceTally)
		if tally.spawned == registerRaceSpawns && tally.pending == 0 {
			break
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if tally.spawned != registerRaceSpawns || tally.pending != 0 || tally.extra != 0 {
		t.Fatalf("spawned=%d failed=%d: %d registrations never got a signal, %d signals for failed registrations",
			tally.spawned, tally.failed, tally.pending, tally.extra)
	}
}

func TestLocalMonitorPIDTerminateRace(t *testing.T) {
	runRegisterRace(t, registerRaceMonitorPID)
}

func TestLocalLinkPIDTerminateRace(t *testing.T) {
	runRegisterRace(t, registerRaceLinkPID)
}

func TestLocalMonitorProcessIDTerminateRace(t *testing.T) {
	runRegisterRace(t, registerRaceMonitorProcessID)
}

func TestLocalMonitorEventTerminateRace(t *testing.T) {
	runRegisterRace(t, registerRaceMonitorEvent)
}

func TestLocalLinkProcessIDTerminateRace(t *testing.T) {
	runRegisterRace(t, registerRaceLinkProcessID)
}

func TestLocalLinkEventTerminateRace(t *testing.T) {
	runRegisterRace(t, registerRaceLinkEvent)
}
