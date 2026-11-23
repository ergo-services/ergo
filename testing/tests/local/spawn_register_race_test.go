package local

import (
	"errors"
	"runtime"
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

const snrRounds = 2000

const snrName = gen.Atom("snr_target")

type snrStop struct{}

// snrIdle asks for the only message it will ever get and dies on it, so the name
// it holds is free again without anyone else ending it.
type snrIdle struct {
	act.Actor
}

func factorySNRIdle() gen.ProcessBehavior { return &snrIdle{} }

func (p *snrIdle) Init(args ...any) error {
	return p.Send(p.PID(), snrStop{})
}

func (p *snrIdle) HandleMessage(from gen.PID, message any) error {
	return gen.TerminateReasonNormal
}

// A name resolves to the pid of the process holding it, never to an empty one.
// Run under -race: a lookup that finds the process through the name has no
// ordering against a pid written after the name was published.
func TestLocalSpawnRegisterNameResolvesToPID(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n").Native()

	stop := make(chan struct{})
	reader := make(chan error, 1)
	go func() {
		var found error
		for {
			select {
			case <-stop:
				reader <- found
				return
			default:
			}
			pid, err := n.ProcessPID(snrName)
			if err != nil {
				continue
			}
			if pid == (gen.PID{}) && found == nil {
				found = errors.New("the name resolved to an empty pid")
			}
		}
	}()

	for i := 0; i < snrRounds; i++ {
		if _, err := n.SpawnRegister(snrName, factorySNRIdle, gen.ProcessOptions{}); err != nil {
			close(stop)
			<-reader
			t.Fatalf("spawn %d: %s", i, err)
		}
		deadline := time.Now().Add(5 * time.Second)
		for {
			if _, err := n.ProcessPID(snrName); errors.Is(err, gen.ErrProcessUnknown) {
				break
			}
			if time.Now().After(deadline) {
				close(stop)
				<-reader
				t.Fatalf("the process spawned on round %d still holds the name", i)
			}
			runtime.Gosched()
		}
	}

	close(stop)
	check.NoError(t, <-reader)
}

// snrHolder stays alive, keeping the name it was spawned under.
type snrHolder struct {
	act.Actor
}

func factorySNRHolder() gen.ProcessBehavior { return &snrHolder{} }

// A name already taken refuses the spawn without a pid, and leaves the process
// holding that name registered under it.
func TestLocalSpawnRegisterNameTaken(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	native := n.Native()

	holder := n.SpawnRegister("snr_holder", factorySNRHolder, gen.ProcessOptions{})

	pid, err := native.SpawnRegister("snr_holder", factorySNRHolder, gen.ProcessOptions{})
	check.ErrorIs(t, err, gen.ErrTaken)
	check.Equal(t, gen.PID{}, pid)

	got, err := native.ProcessPID("snr_holder")
	check.NoError(t, err)
	check.Equal(t, holder, got)
}
