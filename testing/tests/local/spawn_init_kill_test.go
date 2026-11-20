package local

import (
	"errors"
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/stage"
)

type skiRan struct{}

type skiSpawn struct{}

type skiStatus struct {
	Child  gen.PID
	Exits  int
	Reason error
	Ran    int
}

// skiVictim kills itself from within Init, so the kill lands while the process is
// still in the Init state. The message it sends to itself is handled only if the
// process is started afterwards, which the kill must prevent.
type skiVictim struct {
	act.Actor
	parent gen.PID
}

func factorySKIVictim() gen.ProcessBehavior { return &skiVictim{} }

func (v *skiVictim) Init(args ...any) error {
	if len(args) > 0 {
		v.parent = args[0].(gen.PID)
	}
	if err := v.Send(v.PID(), skiRan{}); err != nil {
		return err
	}
	return v.Node().Kill(v.PID())
}

func (v *skiVictim) HandleMessage(from gen.PID, message any) error {
	if v.parent != (gen.PID{}) {
		return v.Send(v.parent, skiRan{})
	}
	return nil
}

// skiParent spawns the victim as a linked child, so a kill during the child Init
// must still reach the parent as an exit.
type skiParent struct {
	act.Actor
	status skiStatus
}

func factorySKIParent() gen.ProcessBehavior { return &skiParent{} }

func (p *skiParent) Init(args ...any) error {
	p.SetTrapExit(true)
	return nil
}

func (p *skiParent) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case skiSpawn:
		pid, err := p.Spawn(factorySKIVictim, gen.ProcessOptions{LinkChild: true}, p.PID())
		if err != nil {
			return err
		}
		p.status.Child = pid
	case gen.MessageExitPID:
		p.status.Exits++
		p.status.Reason = m.Reason
	case skiRan:
		p.status.Ran++
	}
	return nil
}

func (p *skiParent) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return p.status, nil
}

// skiFailing fails its Init, leaving the behavior holding a process handle.
type skiFailing struct{ act.Actor }

var errSKIInit = errors.New("init refused")

func (f *skiFailing) Init(args ...any) error { return errSKIInit }

// A kill during Init terminates the process instead of being lost: the linked parent
// gets the exit, and the process never handles the message it sent to itself.
func TestLocalSpawnKillDuringInit(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")

	parent := n.Spawn(factorySKIParent, gen.ProcessOptions{})
	n.Send(parent, skiSpawn{})

	var status skiStatus
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		v, err := n.Call(parent, skiStatus{})
		if err != nil {
			t.Fatalf("unable to read the parent status: %s", err)
		}
		status = v.(skiStatus)
		if status.Exits > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}

	if status.Exits != 1 {
		t.Fatalf("the parent got %d exit(s) for the killed child, expected 1", status.Exits)
	}
	if status.Reason != gen.TerminateReasonKill {
		t.Fatalf("exit reason is %v, expected %v", status.Reason, gen.TerminateReasonKill)
	}
	if status.Ran != 0 {
		t.Fatalf("the killed child handled its mailbox %d time(s)", status.Ran)
	}
	if _, err := n.Native().ProcessInfo(status.Child); errors.Is(err, gen.ErrProcessUnknown) == false {
		t.Fatalf("the killed child is still known: %v", err)
	}
}

// The name of a process killed during Init is released, so it can be taken again.
func TestLocalSpawnKillDuringInitReleasesName(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")

	n.SpawnRegister("ski_victim", factorySKIVictim, gen.ProcessOptions{})

	if _, err := n.ProcessPID("ski_victim"); errors.Is(err, gen.ErrProcessUnknown) == false {
		t.Fatalf("the name of the killed process is still resolvable: %v", err)
	}
	n.SpawnRegister("ski_victim", factorySKIParent, gen.ProcessOptions{})
}

// A process whose Init fails is terminated, so the handle its behavior kept no longer
// acts as a live process.
func TestLocalSpawnInitErrorHandleNotAlive(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")

	failing := &skiFailing{}
	_, err := n.Native().Spawn(func() gen.ProcessBehavior { return failing }, gen.ProcessOptions{})
	if errors.Is(err, errSKIInit) == false {
		t.Fatalf("spawn returned %v, expected %v", err, errSKIInit)
	}

	if err := failing.SetSendPriority(gen.MessagePriorityHigh); errors.Is(err, gen.ErrNotAllowed) == false {
		t.Fatalf("the handle of the failed process accepts changes: %v", err)
	}
}
