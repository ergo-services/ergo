package local

import (
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/app"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// instantMember completes Init, self-sends a message, then terminates normally on the
// first HandleMessage - so it dies right after spawn launches its run goroutine, inside
// the window where start() used to record its pid in the app group only after spawn had
// returned.
type instantMember struct{ act.Actor }

func factoryInstantMember() gen.ProcessBehavior { return &instantMember{} }

func (m *instantMember) Init(args ...any) error {
	return m.Send(m.PID(), "stop")
}

func (m *instantMember) HandleMessage(from gen.PID, message any) error {
	return gen.TerminateReasonNormal
}

// instantApp starts many immediately-terminating group members. Temporary so their
// normal termination does not drive per-mode auto-stop while we observe the group.
type instantApp struct {
	app.Application
	members int
}

func createInstantApp(members int) gen.ApplicationBehavior {
	return &instantApp{members: members}
}

func (a *instantApp) Load(args ...any) (gen.ApplicationSpec, error) {
	group := make([]gen.ApplicationMemberSpec, a.members)
	for i := range group {
		group[i] = gen.ApplicationMemberSpec{Factory: factoryInstantMember}
	}
	return gen.ApplicationSpec{
		Name:  "instant_app",
		Group: group,
		Mode:  gen.ApplicationModeTemporary,
	}, nil
}

// TestLocalApplicationGroupMemberTerminateRace: a group member that terminates in the
// window between spawn returning and start() recording its pid must not leave a
// phantom entry in the app group. With the fix each member is registered before it can run,
// so terminate() always drains it and the group empties. Without the fix, members that win
// the race leave phantom entries, the group never empties, and the app wedges in Stopping.
func TestLocalApplicationGroupMemberTerminateRace(t *testing.T) {
	const members = 50
	s := stage.New(t)
	n := s.StartNode("n")
	nn := n.Native()

	name, err := nn.ApplicationLoad(createInstantApp(members))
	check.NoError(t, err)
	check.NoError(t, nn.ApplicationStart(name, gen.ApplicationOptions{}))

	deadline := time.Now().Add(5 * time.Second)
	for {
		info, err := nn.ApplicationInfo(name)
		if err == nil && len(info.Group) == 0 {
			return
		}
		if time.Now().After(deadline) {
			cnt := -1
			if err == nil {
				cnt = len(info.Group)
			}
			t.Fatalf("app group still has %d phantom member(s) after all terminated (err=%v)", cnt, err)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
