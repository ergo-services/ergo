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

// teardownEvents records the order of the lifecycle callbacks under test. The sends are
// non-blocking, so a callback never waits on the recorder.
var teardownEvents = make(chan string, 64)

func teardownRecord(event string) {
	select {
	case teardownEvents <- event:
	default:
	}
}

func teardownCollected() []string {
	var out []string
	for {
		select {
		case event := <-teardownEvents:
			out = append(out, event)
		default:
			return out
		}
	}
}

// teardownIndex is the position of an event in the recorded order, -1 if it never happened.
func teardownIndex(events []string, event string) int {
	for i, e := range events {
		if e == event {
			return i
		}
	}
	return -1
}

// tdMember is a group member whose Terminate takes long enough to be observable.
type tdMember struct{ act.Actor }

func factoryTdMember() gen.ProcessBehavior { return &tdMember{} }

func (m *tdMember) Terminate(reason error) {
	teardownRecord("member.begin")
	time.Sleep(200 * time.Millisecond)
	teardownRecord("member.end")
}

// tdChild is supervised by a group member, one level below the application group.
type tdChild struct{ act.Actor }

func factoryTdChild() gen.ProcessBehavior { return &tdChild{} }

func (c *tdChild) Terminate(reason error) {
	teardownRecord("child.begin")
	time.Sleep(200 * time.Millisecond)
	teardownRecord("child.end")
}

type tdSup struct{ act.Supervisor }

func factoryTdSup() gen.ProcessBehavior { return &tdSup{} }

func (s *tdSup) Init(args ...any) (act.SupervisorSpec, error) {
	var spec act.SupervisorSpec
	spec.Type = act.SupervisorTypeOneForOne
	spec.Children = []act.SupervisorChildSpec{{Name: "td_child", Factory: factoryTdChild}}
	return spec, nil
}

func (s *tdSup) Terminate(reason error) {
	teardownRecord("sup.begin")
	time.Sleep(100 * time.Millisecond)
	teardownRecord("sup.end")
}

type tdApp struct{ app.Application }

func createTdApp() gen.ApplicationBehavior { return &tdApp{} }

func (a *tdApp) Load(args ...any) (gen.ApplicationSpec, error) {
	return gen.ApplicationSpec{
		Name: "td_app",
		Mode: gen.ApplicationModeTemporary,
		Group: []gen.ApplicationMemberSpec{
			{Name: "td_member", Factory: factoryTdMember},
			{Name: "td_sup", Factory: factoryTdSup},
		},
	}, nil
}

func (a *tdApp) Stop(ref gen.Ref, reason error) { teardownRecord("app.stop") }
func (a *tdApp) Terminate(reason error)         { teardownRecord("app.terminate") }

// TestLocalApplicationTeardownOrder: the application Terminate callback releases what Init
// opened, so it must run after every process of the application has finished its own
// Terminate - a member, and a child one level below it. Stop still runs first, before any
// member is asked to exit. ApplicationStop returns only once all of that is done, so the
// application is already Loaded when the call comes back.
func TestLocalApplicationTeardownOrder(t *testing.T) {
	teardownCollected()

	s := stage.New(t)
	n := s.StartNode("n")
	nn := n.Native()

	name, err := nn.ApplicationLoad(createTdApp())
	check.NoError(t, err)
	check.NoError(t, nn.ApplicationStart(name, gen.ApplicationOptions{}))
	check.NoError(t, nn.ApplicationStop(name))

	info, err := nn.ApplicationInfo(name)
	check.NoError(t, err)
	check.Equal(t, gen.ApplicationStateLoaded, info.State)

	events := teardownCollected()
	terminate := teardownIndex(events, "app.terminate")
	if terminate < 0 {
		t.Fatalf("the application Terminate callback did not run: %v", events)
	}
	if terminate != len(events)-1 {
		t.Fatalf("the application Terminate callback is not the last event: %v", events)
	}
	for _, event := range []string{"app.stop", "member.end", "sup.end", "child.end"} {
		if teardownIndex(events, event) < 0 {
			t.Fatalf("%s is missing: %v", event, events)
		}
	}
	if teardownIndex(events, "app.stop") != 0 {
		t.Fatalf("the Stop callback did not run first: %v", events)
	}
}

// tdDetached is spawned by a group member directly, so it belongs to the application but
// is not supervised by anything inside it.
type tdDetached struct{ act.Actor }

func factoryTdDetached() gen.ProcessBehavior { return &tdDetached{} }

func (d *tdDetached) Terminate(reason error) {
	teardownRecord("detached.begin")
	time.Sleep(150 * time.Millisecond)
	teardownRecord("detached.end")
}

type tdSpawner struct{ act.Actor }

func factoryTdSpawner() gen.ProcessBehavior { return &tdSpawner{} }

func (s *tdSpawner) Init(args ...any) error {
	_, err := s.Spawn(factoryTdDetached, gen.ProcessOptions{})
	return err
}

type tdDetachedApp struct{ app.Application }

func createTdDetachedApp() gen.ApplicationBehavior { return &tdDetachedApp{} }

func (a *tdDetachedApp) Load(args ...any) (gen.ApplicationSpec, error) {
	return gen.ApplicationSpec{
		Name:  "td_detached_app",
		Mode:  gen.ApplicationModeTemporary,
		Group: []gen.ApplicationMemberSpec{{Name: "td_spawner", Factory: factoryTdSpawner}},
	}, nil
}

func (a *tdDetachedApp) Terminate(reason error) { teardownRecord("app.terminate") }

// TestLocalApplicationTeardownDetached: a process spawned by a member without a supervisor
// above it still belongs to the application. Stopping the application stops it too, and the
// Terminate callback of the application waits for it - otherwise it would outlive the
// application and keep using resources the application has already closed.
func TestLocalApplicationTeardownDetached(t *testing.T) {
	teardownCollected()

	s := stage.New(t)
	n := s.StartNode("n")
	nn := n.Native()

	name, err := nn.ApplicationLoad(createTdDetachedApp())
	check.NoError(t, err)
	check.NoError(t, nn.ApplicationStart(name, gen.ApplicationOptions{}))

	pids, err := nn.ApplicationProcessList(name, 0)
	check.NoError(t, err)
	check.Equal(t, 2, len(pids)) // the member and the process it spawned

	info, err := nn.ApplicationInfo(name)
	check.NoError(t, err)
	check.Equal(t, 1, len(info.Group))     // but only the member is a group member
	check.Equal(t, 2, info.ProcessesTotal) // while both belong to the application

	check.NoError(t, nn.ApplicationStop(name))

	alive, err := nn.ProcessList()
	check.NoError(t, err)
	check.Equal(t, 0, len(alive))

	events := teardownCollected()
	if teardownIndex(events, "detached.end") < 0 {
		t.Fatalf("the detached process was not stopped: %v", events)
	}
	if teardownIndex(events, "app.terminate") != len(events)-1 {
		t.Fatalf("the application Terminate callback is not the last event: %v", events)
	}
}

// tdDying terminates as soon as it starts running.
type tdDying struct{ act.Actor }

func factoryTdDying() gen.ProcessBehavior { return &tdDying{} }

func (d *tdDying) Init(args ...any) error { return d.Send(d.PID(), "die") }
func (d *tdDying) HandleMessage(from gen.PID, message any) error {
	return gen.TerminateReasonNormal
}

// tdSlow keeps the application in Initializing long enough for the member ahead of it to die.
type tdSlow struct{ act.Actor }

func factoryTdSlow() gen.ProcessBehavior { return &tdSlow{} }

func (s *tdSlow) Init(args ...any) error {
	time.Sleep(300 * time.Millisecond)
	return nil
}

type tdPermanentApp struct{ app.Application }

func createTdPermanentApp() gen.ApplicationBehavior { return &tdPermanentApp{} }

func (a *tdPermanentApp) Load(args ...any) (gen.ApplicationSpec, error) {
	return gen.ApplicationSpec{
		Name: "td_permanent_app",
		Mode: gen.ApplicationModePermanent,
		Group: []gen.ApplicationMemberSpec{
			{Name: "td_dying", Factory: factoryTdDying},
			{Name: "td_slow", Factory: factoryTdSlow},
		},
	}, nil
}

func (a *tdPermanentApp) Terminate(reason error) { teardownRecord("app.terminate") }

// TestLocalApplicationMemberDiesDuringStart: a permanent application whose group member
// dies while the rest of the group is still being spawned must not reach Running with that
// member missing. The start fails, the members already spawned are stopped, and Terminate
// runs once.
func TestLocalApplicationMemberDiesDuringStart(t *testing.T) {
	teardownCollected()

	s := stage.New(t)
	n := s.StartNode("n")
	nn := n.Native()

	name, err := nn.ApplicationLoad(createTdPermanentApp())
	check.NoError(t, err)

	err = nn.ApplicationStart(name, gen.ApplicationOptions{})
	if err == nil {
		t.Fatal("the application started with a dead group member")
	}

	info, err := nn.ApplicationInfo(name)
	check.NoError(t, err)
	check.Equal(t, gen.ApplicationStateLoaded, info.State)
	check.Equal(t, 0, len(info.Group))

	alive, err := nn.ProcessList()
	check.NoError(t, err)
	check.Equal(t, 0, len(alive))

	check.Equal(t, []string{"app.terminate"}, teardownCollected())
}
