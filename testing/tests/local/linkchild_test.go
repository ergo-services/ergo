package local

import (
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/stage"
)

type lcChild struct{ act.Actor }

func factoryLCChild() gen.ProcessBehavior { return &lcChild{} }

func (c *lcChild) Init(args ...any) error { return c.Send(c.PID(), "die") }

func (c *lcChild) HandleMessage(from gen.PID, message any) error {
	return gen.TerminateReasonNormal
}

type lcBurst struct{ N int }

type lcStatus struct {
	Spawned int
	Exits   int
}

type lcParent struct {
	act.Actor
	status lcStatus
}

func factoryLCParent() gen.ProcessBehavior { return &lcParent{} }

func (p *lcParent) Init(args ...any) error {
	p.SetTrapExit(true)
	return nil
}

func (p *lcParent) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case lcBurst:
		for i := 0; i < m.N; i++ {
			if _, err := p.Spawn(factoryLCChild, gen.ProcessOptions{LinkChild: true}); err != nil {
				return err
			}
			p.status.Spawned++
		}
	case gen.MessageExitPID:
		p.status.Exits++
	}
	return nil
}

func (p *lcParent) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return p.status, nil
}

// a child spawned with LinkChild may die before Spawn returns: its exit must arrive
func TestLocalLinkChildExitNotLost(t *testing.T) {
	const rounds = 50
	const perRound = 1000

	s := stage.New(t)
	n := s.StartNode("n")

	parent := n.Spawn(factoryLCParent, gen.ProcessOptions{})

	var status lcStatus
	for r := 0; r < rounds; r++ {
		n.Send(parent, lcBurst{N: perRound})

		want := (r + 1) * perRound
		deadline := time.Now().Add(10 * time.Second)
		for {
			v, err := n.Call(parent, "status")
			if err != nil {
				t.Fatalf("round %d: status: %v", r, err)
			}
			status = v.(lcStatus)
			if status.Exits == want {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("round %d: spawned %d, got %d exits, lost %d",
					r, status.Spawned, status.Exits, status.Spawned-status.Exits)
			}
			time.Sleep(2 * time.Millisecond)
		}
	}
}
