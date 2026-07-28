package act_test

import (
	"errors"
	"testing"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/unit"
)

// child process used by the supervisors under test
type supUnitChild struct{ act.Actor }

func factorySupUnitChild() gen.ProcessBehavior { return &supUnitChild{} }

// test supervisor: only Init is implemented; the spec arrives via Init args, so the
// factory stays static. Everything else is inherited from act.Supervisor.
type supUnit struct{ act.Supervisor }

func factorySupUnit() gen.ProcessBehavior { return &supUnit{} }

func (s *supUnit) Init(args ...any) (act.SupervisorSpec, error) {
	return args[0].(act.SupervisorSpec), nil
}

// supControl is the subset of act.Supervisor's dynamic API used by the tests; the
// methods are promoted from the embedded act.Supervisor.
type supControl interface {
	StartChild(name gen.Atom, args ...any) error
	AddChild(child act.SupervisorChildSpec) error
	DisableChild(name gen.Atom) error
	EnableChild(name gen.Atom) error
	Children() []act.SupervisorChild
}

func control(s *unit.Subject) supControl { return s.Behavior().(*supUnit) }

func threeChildren() []act.SupervisorChildSpec {
	return []act.SupervisorChildSpec{
		{Name: "a", Factory: factorySupUnitChild},
		{Name: "b", Factory: factorySupUnitChild},
		{Name: "c", Factory: factorySupUnitChild},
	}
}

// childPIDs spawns the supervisor, asserts the init spawn count and returns the
// ordered child PIDs.
func childPIDs(t *testing.T, s *unit.Subject, want int) []gen.PID {
	t.Helper()
	spawns := s.ShouldSpawn().Collect()
	check.Equal(t, want, len(spawns))
	pids := make([]gen.PID, len(spawns))
	for i, sp := range spawns {
		pids[i] = sp.Child
	}
	return pids
}

//
// init
//

// Init spawns every child once, registered by name, linked both ways.
func TestSupervisorUnitInitSpawnsChildren(t *testing.T) {
	spec := act.SupervisorSpec{Type: act.SupervisorTypeOneForOne, Children: threeChildren()}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)

	s.ShouldSpawn().Factory(factorySupUnitChild).Times(3).Assert()
	for _, name := range []gen.Atom{"a", "b", "c"} {
		s.ShouldSpawn().Register(name).Once().Assert()
	}
	// supervisor links itself to each child both ways
	for _, sp := range s.ShouldSpawn().Collect() {
		check.True(t, sp.Options.LinkChild)
		check.True(t, sp.Options.LinkParent)
	}
}

// Init with an empty children list fails ProcessInit (and thus Spawn).
func TestSupervisorUnitInitEmptyChildrenFails(t *testing.T) {
	spec := act.SupervisorSpec{Type: act.SupervisorTypeOneForOne}
	_, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.Error(t, err)
}

// Init with duplicate child names fails ProcessInit.
func TestSupervisorUnitInitDuplicateNamesFails(t *testing.T) {
	spec := act.SupervisorSpec{
		Type: act.SupervisorTypeOneForOne,
		Children: []act.SupervisorChildSpec{
			{Name: "a", Factory: factorySupUnitChild},
			{Name: "a", Factory: factorySupUnitChild},
		},
	}
	_, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.ErrorIs(t, err, act.ErrSupervisorChildDuplicate)
}

//
// OneForOne
//

// OneForOne + Transient: an abnormal child exit restarts only that child.
func TestSupervisorUnitOFORestartsOnAbnormalExit(t *testing.T) {
	spec := act.SupervisorSpec{Type: act.SupervisorTypeOneForOne, Children: threeChildren()}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	pids := childPIDs(t, s, 3)

	mark := s.Mark()
	s.DeliverExit(pids[0], errors.New("crash"))

	s.ShouldSpawn().Since(mark).Once().Assert() // exactly one restart
	s.ShouldSpawn().Register("a").Since(mark).Once().Assert()
	s.ShouldSendExit().Since(mark).None().Assert() // siblings untouched
}

// OneForOne + Transient: a normal child exit is NOT restarted.
func TestSupervisorUnitOFONoRestartOnNormalExit(t *testing.T) {
	spec := act.SupervisorSpec{
		Type:                act.SupervisorTypeOneForOne,
		DisableAutoShutdown: true,
		Children:            threeChildren(),
	}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	pids := childPIDs(t, s, 3)

	mark := s.Mark()
	s.DeliverExit(pids[0], gen.TerminateReasonNormal)
	s.ShouldSpawn().Since(mark).None().Assert()
}

// OneForOne + per-child Permanent: even a normal exit restarts the child.
func TestSupervisorUnitOFOPermanentRestartsOnNormalExit(t *testing.T) {
	spec := act.SupervisorSpec{
		Type: act.SupervisorTypeOneForOne,
		Children: []act.SupervisorChildSpec{
			{Name: "a", Factory: factorySupUnitChild,
				Restart: act.SupervisorChildRestart{Strategy: act.SupervisorStrategyPermanent}},
		},
	}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	pids := childPIDs(t, s, 1)

	mark := s.Mark()
	s.DeliverExit(pids[0], gen.TerminateReasonNormal)
	s.ShouldSpawn().Register("a").Since(mark).Once().Assert()
}

// OneForOne + per-child Temporary: an abnormal exit is never restarted.
func TestSupervisorUnitOFOTemporaryNeverRestarts(t *testing.T) {
	spec := act.SupervisorSpec{
		Type:                act.SupervisorTypeOneForOne,
		DisableAutoShutdown: true,
		Children: []act.SupervisorChildSpec{
			{Name: "a", Factory: factorySupUnitChild,
				Restart: act.SupervisorChildRestart{Strategy: act.SupervisorStrategyTemporary}},
			{Name: "b", Factory: factorySupUnitChild},
		},
	}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	pids := childPIDs(t, s, 2)

	mark := s.Mark()
	s.DeliverExit(pids[0], errors.New("crash"))
	s.ShouldSpawn().Since(mark).None().Assert()
}

// OneForOne: exceeding the restart intensity terminates the supervisor.
func TestSupervisorUnitOFOIntensityExceededTerminates(t *testing.T) {
	spec := act.SupervisorSpec{
		Type:                act.SupervisorTypeOneForOne,
		DisableAutoShutdown: true,
		Children:            []act.SupervisorChildSpec{{Name: "a", Factory: factorySupUnitChild}},
	}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)

	// default intensity is 5: the first 5 abnormal exits restart, the 6th terminates.
	for i := 0; i < 6 && s.Terminated() == false; i++ {
		spawns := s.ShouldSpawn().Collect()
		s.DeliverExit(spawns[len(spawns)-1].Child, errors.New("crash"))
	}

	check.True(t, s.Terminated())
	s.ShouldTerminate().Once().Assert()
}

//
// AllForOne
//

// AllForOne: an abnormal exit terminates the siblings, then restarts the whole
// group once every sibling's exit has arrived.
func TestSupervisorUnitAFOGroupRestart(t *testing.T) {
	spec := act.SupervisorSpec{
		Type:                act.SupervisorTypeAllForOne,
		DisableAutoShutdown: true,
		Children:            threeChildren(),
	}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	pids := childPIDs(t, s, 3)
	a, b, c := pids[0], pids[1], pids[2]

	// b dies abnormally -> supervisor sends exit to the surviving siblings a and c
	mark := s.Mark()
	s.DeliverExit(b, errors.New("crash"))
	s.ShouldSendExit().To(a).Since(mark).Once().Assert()
	s.ShouldSendExit().To(c).Since(mark).Once().Assert()
	s.ShouldSpawn().Since(mark).None().Assert() // no restart until the group is down

	// siblings terminate -> after the last one, the whole group restarts
	s.DeliverExit(a, gen.TerminateReasonShutdown)
	restartMark := s.Mark()
	s.DeliverExit(c, gen.TerminateReasonShutdown)
	s.ShouldSpawn().Since(restartMark).Times(3).Assert()
	for _, name := range []gen.Atom{"a", "b", "c"} {
		s.ShouldSpawn().Register(name).Since(restartMark).Once().Assert()
	}
}

//
// RestForOne
//

// RestForOne: an abnormal exit terminates only the children started after it,
// leaving the preceding ones untouched.
func TestSupervisorUnitRFORestartsFollowingOnly(t *testing.T) {
	spec := act.SupervisorSpec{
		Type:                act.SupervisorTypeRestForOne,
		DisableAutoShutdown: true,
		Children:            threeChildren(),
	}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	pids := childPIDs(t, s, 3)
	a, b, c := pids[0], pids[1], pids[2]

	// b dies -> only c (the one after b) is terminated; a (before b) is preserved
	mark := s.Mark()
	s.DeliverExit(b, errors.New("crash"))
	s.ShouldSendExit().To(c).Since(mark).Once().Assert()
	s.ShouldSendExit().To(a).Since(mark).None().Assert()
}

//
// SimpleOneForOne
//

// SimpleOneForOne: Init spawns nothing; children are dynamic instances created via
// StartChild, spawned anonymously.
func TestSupervisorUnitSOFOStartChild(t *testing.T) {
	spec := act.SupervisorSpec{
		Type:     act.SupervisorTypeSimpleOneForOne,
		Children: []act.SupervisorChildSpec{{Name: "worker", Factory: factorySupUnitChild}},
	}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	s.ShouldSpawn().None().Assert() // templates only, nothing spawned at init

	mark := s.Mark()
	check.NoError(t, control(s).StartChild("worker"))
	s.ShouldSpawn().Factory(factorySupUnitChild).Since(mark).Once().Assert()
}

// SimpleOneForOne: an instance that exits abnormally is restarted.
func TestSupervisorUnitSOFOInstanceRestart(t *testing.T) {
	spec := act.SupervisorSpec{
		Type:     act.SupervisorTypeSimpleOneForOne,
		Restart:  act.SupervisorRestart{Strategy: act.SupervisorStrategyPermanent},
		Children: []act.SupervisorChildSpec{{Name: "worker", Factory: factorySupUnitChild}},
	}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	check.NoError(t, control(s).StartChild("worker"))
	instance := s.ShouldSpawn().Collect()[0].Child

	mark := s.Mark()
	s.DeliverExit(instance, errors.New("crash"))
	s.ShouldSpawn().Since(mark).Once().Assert()
}

//
// dynamic API
//

// DisableChild stops the running child (graceful exit) and disables it; a later
// exit of that child is not restarted.
func TestSupervisorUnitDisableChild(t *testing.T) {
	spec := act.SupervisorSpec{
		Type:                act.SupervisorTypeOneForOne,
		DisableAutoShutdown: true,
		Children:            threeChildren(),
	}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	pids := childPIDs(t, s, 3)

	mark := s.Mark()
	check.NoError(t, control(s).DisableChild("a"))
	s.ShouldSendExit().To(pids[0]).Since(mark).Once().Assert()
}

//
// EnableHandleChild
//

// EnableHandleChild makes the supervisor notify itself on each child start; the
// notifications are observable as self-sends.
func TestSupervisorUnitHandleChildStartNotifies(t *testing.T) {
	spec := act.SupervisorSpec{
		Type:              act.SupervisorTypeOneForOne,
		EnableHandleChild: true,
		Children:          threeChildren(),
	}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)

	s.ShouldSpawn().Times(3).Assert()
	s.ShouldSend().To(s.PID()).Times(3).Assert() // one child-start notification per child
}

//
// dynamic API
//

// AddChild spawns the new child and Children() reflects it.
func TestSupervisorUnitAddChild(t *testing.T) {
	spec := act.SupervisorSpec{
		Type:     act.SupervisorTypeOneForOne,
		Children: []act.SupervisorChildSpec{{Name: "a", Factory: factorySupUnitChild}},
	}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	childPIDs(t, s, 1)

	mark := s.Mark()
	check.NoError(t, control(s).AddChild(act.SupervisorChildSpec{Name: "b", Factory: factorySupUnitChild}))
	s.ShouldSpawn().Register("b").Since(mark).Once().Assert()
	check.Equal(t, 2, len(control(s).Children()))
}

// DisableChild stops and disables; EnableChild starts it again.
func TestSupervisorUnitEnableAfterDisable(t *testing.T) {
	spec := act.SupervisorSpec{
		Type:                act.SupervisorTypeOneForOne,
		DisableAutoShutdown: true,
		Children:            []act.SupervisorChildSpec{{Name: "a", Factory: factorySupUnitChild}},
	}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	pids := childPIDs(t, s, 1)

	check.NoError(t, control(s).DisableChild("a"))
	// EnableChild refuses while the disabled child is still terminating
	check.ErrorIs(t, control(s).EnableChild("a"), act.ErrSupervisorChildRunning)

	s.DeliverExit(pids[0], gen.TerminateReasonShutdown) // child a has stopped
	mark := s.Mark()
	check.NoError(t, control(s).EnableChild("a"))
	s.ShouldSpawn().Register("a").Since(mark).Once().Assert()
}

//
// default callbacks (inherited from act.Supervisor)
//

func TestSupervisorUnitDefaultCallbacks(t *testing.T) {
	spec := act.SupervisorSpec{
		Type:                act.SupervisorTypeOneForOne,
		DisableAutoShutdown: true,
		Children:            threeChildren(),
	}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)

	s.SendMessage(gen.PID{}, "ignored")       // -> default HandleMessage (nil)
	resp, err := s.Call(gen.PID{}, "ignored") // -> default HandleCall (nil, nil)
	check.NoError(t, err)
	check.Nil(t, resp)
	s.DeliverEvent(gen.Event{Name: "ev"}, "m") // -> default HandleEvent (nil)
	s.ShouldTerminate().None().Assert()
}

//
// RestForOne full cascade
//

// RestForOne: b dies -> only c (after b) is terminated; once c's exit arrives, b and
// c are restarted, a (before b) is preserved.
func TestSupervisorUnitRFOCascadeRestart(t *testing.T) {
	spec := act.SupervisorSpec{
		Type:                act.SupervisorTypeRestForOne,
		DisableAutoShutdown: true,
		Children:            threeChildren(),
	}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	pids := childPIDs(t, s, 3)
	b, c := pids[1], pids[2]

	mark := s.Mark()
	s.DeliverExit(b, errors.New("crash"))
	s.ShouldSendExit().To(c).Since(mark).Once().Assert()

	restartMark := s.Mark()
	s.DeliverExit(c, gen.TerminateReasonShutdown)
	s.ShouldSpawn().Register("b").Since(restartMark).Once().Assert()
	s.ShouldSpawn().Register("c").Since(restartMark).Once().Assert()
	s.ShouldSpawn().Register("a").Since(restartMark).None().Assert()
}

//
// SimpleOneForOne dynamic instances
//

// SOFO: DisableChild terminates every running instance of the template.
func TestSupervisorUnitSOFODisableChild(t *testing.T) {
	spec := act.SupervisorSpec{
		Type:     act.SupervisorTypeSimpleOneForOne,
		Children: []act.SupervisorChildSpec{{Name: "worker", Factory: factorySupUnitChild}},
	}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	check.NoError(t, control(s).StartChild("worker"))
	check.NoError(t, control(s).StartChild("worker"))
	s.ShouldSpawn().Times(2).Assert()

	mark := s.Mark()
	check.NoError(t, control(s).DisableChild("worker"))
	s.ShouldSendExit().Since(mark).Times(2).Assert() // both instances terminated
}

// RestForOne dynamic API: AddChild / Children / DisableChild / EnableChild exercise
// the All/Rest-For-One strategy's dynamic paths.
func TestSupervisorUnitRFODynamicAPI(t *testing.T) {
	spec := act.SupervisorSpec{
		Type:                act.SupervisorTypeRestForOne,
		DisableAutoShutdown: true,
		Children:            []act.SupervisorChildSpec{{Name: "a", Factory: factorySupUnitChild}},
	}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	childPIDs(t, s, 1)

	mark := s.Mark()
	check.NoError(t, control(s).AddChild(act.SupervisorChildSpec{Name: "b", Factory: factorySupUnitChild}))
	spawnB := s.ShouldSpawn().Register("b").Since(mark).Collect()
	check.Equal(t, 1, len(spawnB))
	check.Equal(t, 2, len(control(s).Children()))

	check.NoError(t, control(s).DisableChild("b"))
	// EnableChild refuses while the disabled child is still terminating
	check.ErrorIs(t, control(s).EnableChild("b"), act.ErrSupervisorChildRunning)

	s.DeliverExit(spawnB[0].Child, gen.TerminateReasonShutdown) // child b has stopped
	mark2 := s.Mark()
	check.NoError(t, control(s).EnableChild("b"))
	s.ShouldSpawn().Register("b").Since(mark2).Once().Assert()
}

// SOFO dynamic API: Children reflects running instances; AddChild adds a template.
func TestSupervisorUnitSOFOChildrenAndAdd(t *testing.T) {
	spec := act.SupervisorSpec{
		Type:     act.SupervisorTypeSimpleOneForOne,
		Children: []act.SupervisorChildSpec{{Name: "worker", Factory: factorySupUnitChild}},
	}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	check.NoError(t, control(s).StartChild("worker"))
	check.Equal(t, 1, len(control(s).Children()))

	check.NoError(t, control(s).AddChild(act.SupervisorChildSpec{Name: "worker2", Factory: factorySupUnitChild}))
	mark := s.Mark()
	check.NoError(t, control(s).StartChild("worker2"))
	s.ShouldSpawn().Since(mark).Once().Assert()
}

// a Significant child terminating (and not restarted) shuts the supervisor down.
func TestSupervisorUnitSignificantShutsDown(t *testing.T) {
	spec := act.SupervisorSpec{
		Type: act.SupervisorTypeOneForOne,
		Children: []act.SupervisorChildSpec{
			{Name: "a", Factory: factorySupUnitChild, Significant: true,
				Restart: act.SupervisorChildRestart{Strategy: act.SupervisorStrategyTemporary}},
			{Name: "b", Factory: factorySupUnitChild},
		},
	}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	pids := childPIDs(t, s, 2)

	// significant "a" (temporary -> not restarted) terminates: the supervisor begins
	// shutdown by terminating the sibling "b", then exits once "b" is down.
	mark := s.Mark()
	s.DeliverExit(pids[0], errors.New("crash"))
	s.ShouldSendExit().To(pids[1]).Since(mark).Once().Assert()
	check.False(t, s.Terminated())

	s.DeliverExit(pids[1], gen.TerminateReasonShutdown)
	check.True(t, s.Terminated())
}

// the last child terminating normally triggers auto-shutdown (default).
func TestSupervisorUnitAutoShutdown(t *testing.T) {
	spec := act.SupervisorSpec{
		Type:     act.SupervisorTypeOneForOne,
		Children: []act.SupervisorChildSpec{{Name: "a", Factory: factorySupUnitChild}},
	}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	pids := childPIDs(t, s, 1)

	s.DeliverExit(pids[0], gen.TerminateReasonNormal) // transient -> not restarted -> no children left
	check.True(t, s.Terminated())
}

// per-child OnExceedDisable: exceeding the local intensity disables the child but
// keeps the supervisor alive.
func TestSupervisorUnitOnExceedDisable(t *testing.T) {
	spec := act.SupervisorSpec{
		Type:                act.SupervisorTypeOneForOne,
		Restart:             act.SupervisorRestart{Strategy: act.SupervisorStrategyPermanent},
		DisableAutoShutdown: true,
		Children: []act.SupervisorChildSpec{
			{Name: "a", Factory: factorySupUnitChild},
			{Name: "b", Factory: factorySupUnitChild,
				Restart: act.SupervisorChildRestart{Intensity: 2, Period: 60, OnExceed: act.OnExceedDisable}},
		},
	}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	childPIDs(t, s, 2)

	for i := 0; i < 3; i++ {
		bs := s.ShouldSpawn().Register("b").Collect()
		s.DeliverExit(bs[len(bs)-1].Child, errors.New("boom"))
	}
	check.False(t, s.Terminated()) // OnExceedDisable keeps the supervisor alive
}

//
// init validation error paths
//

func TestSupervisorUnitInitNilFactory(t *testing.T) {
	spec := act.SupervisorSpec{
		Type:     act.SupervisorTypeOneForOne,
		Children: []act.SupervisorChildSpec{{Name: "a", Factory: nil}},
	}
	_, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.Error(t, err)
}

func TestSupervisorUnitInitPerChildIntensityOnAFO(t *testing.T) {
	// per-child Intensity is rejected for All/Rest-For-One
	spec := act.SupervisorSpec{
		Type: act.SupervisorTypeAllForOne,
		Children: []act.SupervisorChildSpec{
			{Name: "a", Factory: factorySupUnitChild,
				Restart: act.SupervisorChildRestart{Intensity: 2, Period: 60}},
		},
	}
	_, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.ErrorIs(t, err, act.ErrSupervisorInvalidSpec)
}

func TestSupervisorUnitInitPreserveMailboxOnAFO(t *testing.T) {
	// PreserveMailbox is rejected for All/Rest-For-One
	spec := act.SupervisorSpec{
		Type: act.SupervisorTypeAllForOne,
		Children: []act.SupervisorChildSpec{
			{Name: "a", Factory: factorySupUnitChild, Options: gen.ProcessOptions{PreserveMailbox: true}},
		},
	}
	_, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.ErrorIs(t, err, act.ErrSupervisorInvalidSpec)
}

//
// more childTerminated branches
//

// AllForOne + per-child Temporary: an abnormal exit of that child does NOT trigger
// the group restart.
func TestSupervisorUnitAFOPerChildTemporaryNoGroup(t *testing.T) {
	spec := act.SupervisorSpec{
		Type:                act.SupervisorTypeAllForOne,
		Restart:             act.SupervisorRestart{Strategy: act.SupervisorStrategyPermanent},
		DisableAutoShutdown: true,
		Children: []act.SupervisorChildSpec{
			{Name: "a", Factory: factorySupUnitChild},
			{Name: "b", Factory: factorySupUnitChild,
				Restart: act.SupervisorChildRestart{Strategy: act.SupervisorStrategyTemporary}},
		},
	}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	pids := childPIDs(t, s, 2)

	mark := s.Mark()
	s.DeliverExit(pids[1], errors.New("crash")) // temporary -> no group restart
	s.ShouldSendExit().Since(mark).None().Assert()
	s.ShouldSpawn().Since(mark).None().Assert()
	check.False(t, s.Terminated())
}

// SOFO + per-instance Temporary: an instance exit is never restarted.
func TestSupervisorUnitSOFOTemporaryNoRestart(t *testing.T) {
	spec := act.SupervisorSpec{
		Type: act.SupervisorTypeSimpleOneForOne,
		Children: []act.SupervisorChildSpec{
			{Name: "worker", Factory: factorySupUnitChild,
				Restart: act.SupervisorChildRestart{Strategy: act.SupervisorStrategyTemporary}},
		},
	}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	check.NoError(t, control(s).StartChild("worker"))
	inst := s.ShouldSpawn().Collect()[0].Child

	mark := s.Mark()
	s.DeliverExit(inst, errors.New("crash"))
	s.ShouldSpawn().Since(mark).None().Assert()
}

// SOFO per-instance intensity exceeded terminates the supervisor.
func TestSupervisorUnitSOFOInstanceIntensityExceeded(t *testing.T) {
	spec := act.SupervisorSpec{
		Type: act.SupervisorTypeSimpleOneForOne,
		Children: []act.SupervisorChildSpec{
			{Name: "worker", Factory: factorySupUnitChild,
				Restart: act.SupervisorChildRestart{Strategy: act.SupervisorStrategyPermanent, Intensity: 2, Period: 60}},
		},
	}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	check.NoError(t, control(s).StartChild("worker"))

	for i := 0; i < 4 && s.Terminated() == false; i++ {
		insts := s.ShouldSpawn().Collect()
		s.DeliverExit(insts[len(insts)-1].Child, errors.New("boom"))
	}
	check.True(t, s.Terminated())
}

// StartChild on a running or disabled OFO/RFO child errors (exercises childSpec).
func TestSupervisorUnitStartChildErrors(t *testing.T) {
	for _, typ := range []act.SupervisorType{act.SupervisorTypeOneForOne, act.SupervisorTypeRestForOne} {
		spec := act.SupervisorSpec{
			Type:                typ,
			DisableAutoShutdown: true,
			Children:            []act.SupervisorChildSpec{{Name: "a", Factory: factorySupUnitChild}},
		}
		s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
		check.NoError(t, err)
		childPIDs(t, s, 1)
		check.Error(t, control(s).StartChild("a")) // already running
		check.NoError(t, control(s).DisableChild("a"))
		check.Error(t, control(s).StartChild("a")) // disabled
	}
}

// Inspect returns the supervisor's state map; after a restart it includes history.
func TestSupervisorUnitInspectWithHistory(t *testing.T) {
	spec := act.SupervisorSpec{
		Type:                act.SupervisorTypeOneForOne,
		DisableAutoShutdown: true,
		Children:            []act.SupervisorChildSpec{{Name: "a", Factory: factorySupUnitChild}},
	}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	pids := childPIDs(t, s, 1)
	s.DeliverExit(pids[0], errors.New("crash")) // restart -> populates history

	m, err := s.Inspect(gen.PID{})
	check.NoError(t, err)
	check.True(t, len(m) > 0)
}

// Inspect reaches the per-strategy inspect() for every supervisor type.
func TestSupervisorUnitInspectStrategies(t *testing.T) {
	for _, typ := range []act.SupervisorType{
		act.SupervisorTypeOneForOne, act.SupervisorTypeAllForOne, act.SupervisorTypeRestForOne,
	} {
		spec := act.SupervisorSpec{Type: typ, DisableAutoShutdown: true, Children: threeChildren()}
		s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
		check.NoError(t, err)
		childPIDs(t, s, 3)
		m, err := s.Inspect(gen.PID{})
		check.NoError(t, err)
		check.NotNil(t, m)
	}

	// SOFO: inspect with a running instance
	spec := act.SupervisorSpec{
		Type:     act.SupervisorTypeSimpleOneForOne,
		Children: []act.SupervisorChildSpec{{Name: "worker", Factory: factorySupUnitChild}},
	}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	check.NoError(t, control(s).StartChild("worker"))
	m, err := s.Inspect(gen.PID{})
	check.NoError(t, err)
	check.NotNil(t, m)
}

// a PreserveMailbox child that dies with a *gen.Error carrying a mailbox is
// restarted with that mailbox adopted (extractMailbox).
func TestSupervisorUnitPreserveMailboxAdopt(t *testing.T) {
	spec := act.SupervisorSpec{
		Type:                act.SupervisorTypeOneForOne,
		DisableAutoShutdown: true,
		Children: []act.SupervisorChildSpec{
			{Name: "a", Factory: factorySupUnitChild, Options: gen.ProcessOptions{PreserveMailbox: true}},
		},
	}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	pids := childPIDs(t, s, 1)

	mark := s.Mark()
	s.DeliverExit(pids[0], &gen.Error{Msg: "boom", Mailbox: &gen.ProcessMailbox{}})
	restarts := s.ShouldSpawn().Register("a").Since(mark).Collect()
	check.Equal(t, 1, len(restarts))
	check.NotNil(t, restarts[0].Options.Mailbox) // adopted mailbox
}

// ProcessRun handles the non-PID exit signal variants without disturbing children.
func TestSupervisorUnitProcessRunExitVariants(t *testing.T) {
	spec := act.SupervisorSpec{
		Type:                act.SupervisorTypeOneForOne,
		DisableAutoShutdown: true,
		Children:            threeChildren(),
	}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	childPIDs(t, s, 3)

	mark := s.Mark()
	s.DeliverExitMessage(gen.MessageExitProcessID{ProcessID: gen.ProcessID{Name: "x", Node: "n@h"}, Reason: errors.New("boom")})
	s.DeliverExitMessage(gen.MessageExitAlias{Alias: gen.Alias{}, Reason: errors.New("boom")})
	s.DeliverExitMessage(gen.MessageExitEvent{Event: gen.Event{Name: "e"}, Reason: errors.New("boom")})
	s.DeliverExitMessage(gen.MessageExitNode{Name: "n@h"})
	// none of these reference a child, so no restart spawns happen
	s.ShouldSpawn().Since(mark).None().Assert()
}

// AllForOne with KeepOrder exercises the ordered group restart paths.
func TestSupervisorUnitAFOKeepOrder(t *testing.T) {
	spec := act.SupervisorSpec{
		Type:                act.SupervisorTypeAllForOne,
		Restart:             act.SupervisorRestart{Strategy: act.SupervisorStrategyPermanent, KeepOrder: true},
		DisableAutoShutdown: true,
		Children:            threeChildren(),
	}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	pids := childPIDs(t, s, 3)
	a, b, c := pids[0], pids[1], pids[2]

	// KeepOrder terminates the group sequentially in reverse start order (c, then a),
	// waiting for each exit, then restarts all in forward order.
	s.DeliverExit(b, errors.New("crash"))
	s.ShouldSendExit().To(c).Once().Assert()
	s.DeliverExit(c, gen.TerminateReasonShutdown)
	s.ShouldSendExit().To(a).Once().Assert()
	restartMark := s.Mark()
	s.DeliverExit(a, gen.TerminateReasonShutdown)
	s.ShouldSpawn().Since(restartMark).Times(3).Assert() // all restarted in order
}

// dynamic API error branches: duplicate add, unknown name operations.
func TestSupervisorUnitDynamicErrors(t *testing.T) {
	spec := act.SupervisorSpec{
		Type:                act.SupervisorTypeOneForOne,
		DisableAutoShutdown: true,
		Children:            []act.SupervisorChildSpec{{Name: "a", Factory: factorySupUnitChild}},
	}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	childPIDs(t, s, 1)

	check.Error(t, control(s).AddChild(act.SupervisorChildSpec{Name: "a", Factory: factorySupUnitChild})) // duplicate
	check.Error(t, control(s).EnableChild("nope"))
	check.Error(t, control(s).DisableChild("nope"))
	check.Error(t, control(s).StartChild("nope"))
}

// AllForOne + per-child Permanent: a NORMAL exit of the permanent child triggers the
// group restart (the override forces restart even on normal exit).
func TestSupervisorUnitAFOPermanentGroupOnNormal(t *testing.T) {
	spec := act.SupervisorSpec{
		Type:                act.SupervisorTypeAllForOne,
		DisableAutoShutdown: true,
		Children: []act.SupervisorChildSpec{
			{Name: "a", Factory: factorySupUnitChild},
			{Name: "b", Factory: factorySupUnitChild,
				Restart: act.SupervisorChildRestart{Strategy: act.SupervisorStrategyPermanent}},
		},
	}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	pids := childPIDs(t, s, 2)

	mark := s.Mark()
	s.DeliverExit(pids[1], gen.TerminateReasonNormal) // permanent -> group restart on normal
	s.ShouldSendExit().To(pids[0]).Since(mark).Once().Assert()
}

// RestForOne + per-child Temporary in the middle: abnormal exit does not trigger the
// group restart.
func TestSupervisorUnitRFOTemporaryMiddleNoGroup(t *testing.T) {
	spec := act.SupervisorSpec{
		Type:                act.SupervisorTypeRestForOne,
		Restart:             act.SupervisorRestart{Strategy: act.SupervisorStrategyPermanent},
		DisableAutoShutdown: true,
		Children: []act.SupervisorChildSpec{
			{Name: "a", Factory: factorySupUnitChild},
			{Name: "b", Factory: factorySupUnitChild,
				Restart: act.SupervisorChildRestart{Strategy: act.SupervisorStrategyTemporary}},
			{Name: "c", Factory: factorySupUnitChild},
		},
	}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	pids := childPIDs(t, s, 3)

	mark := s.Mark()
	s.DeliverExit(pids[1], errors.New("crash")) // temporary middle -> no group restart
	s.ShouldSendExit().Since(mark).None().Assert()
	check.False(t, s.Terminated())
}

// a supervisor whose HandleMessage / HandleCall return an error terminates abnormally.
type supErr struct{ act.Supervisor }

func factorySupErr() gen.ProcessBehavior { return &supErr{} }

func (s *supErr) Init(args ...any) (act.SupervisorSpec, error) {
	return args[0].(act.SupervisorSpec), nil
}
func (s *supErr) HandleMessage(from gen.PID, message any) error { return errors.New("handle boom") }
func (s *supErr) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, errors.New("call boom")
}
func (s *supErr) HandleEvent(message gen.MessageEvent) error { return errors.New("event boom") }

func TestSupervisorUnitHandleMessageError(t *testing.T) {
	spec := act.SupervisorSpec{
		Type:                act.SupervisorTypeOneForOne,
		DisableAutoShutdown: true,
		Children:            threeChildren(),
	}
	s, err := unit.Spawn(t, factorySupErr, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	pids := childPIDs(t, s, 3)

	// HandleMessage error makes the supervisor shut down: it terminates all children,
	// then exits once they are down.
	mark := s.Mark()
	s.SendMessage(gen.PID{}, "boom")
	s.ShouldSendExit().Since(mark).Times(3).Assert()
	for _, p := range pids {
		s.DeliverExit(p, gen.TerminateReasonShutdown)
	}
	check.True(t, s.Terminated())
}

// AllForOne global restart intensity exceeded across repeated group restarts
// terminates the supervisor with a wrapped reason.
func TestSupervisorUnitAFOGlobalIntensityExceeded(t *testing.T) {
	spec := act.SupervisorSpec{
		Type:                act.SupervisorTypeAllForOne,
		Restart:             act.SupervisorRestart{Strategy: act.SupervisorStrategyPermanent, Intensity: 2, Period: 60},
		DisableAutoShutdown: true,
		Children: []act.SupervisorChildSpec{
			{Name: "a", Factory: factorySupUnitChild},
			{Name: "b", Factory: factorySupUnitChild},
		},
	}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	childPIDs(t, s, 2)

	// each round: kill a (abnormal) -> supervisor terminates b -> deliver b's exit ->
	// group restarts. The third group restart exceeds the global intensity (2).
	for round := 0; round < 5 && s.Terminated() == false; round++ {
		a := s.ShouldSpawn().Register("a").Collect()
		b := s.ShouldSpawn().Register("b").Collect()
		s.DeliverExit(a[len(a)-1].Child, errors.New("crash"))
		if s.Terminated() == false {
			s.DeliverExit(b[len(b)-1].Child, gen.TerminateReasonShutdown)
		}
	}
	check.True(t, s.Terminated())
}

// HandleEvent returning an error terminates the supervisor immediately.
func TestSupervisorUnitHandleEventError(t *testing.T) {
	spec := act.SupervisorSpec{Type: act.SupervisorTypeOneForOne, DisableAutoShutdown: true, Children: threeChildren()}
	s, err := unit.Spawn(t, factorySupErr, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	childPIDs(t, s, 3)

	s.DeliverEvent(gen.Event{Name: "ev"}, "m") // HandleEvent error -> terminate
	check.True(t, s.Terminated())
}

// HandleCall returning an error makes the supervisor shut its children down and exit.
func TestSupervisorUnitHandleCallError(t *testing.T) {
	spec := act.SupervisorSpec{Type: act.SupervisorTypeOneForOne, DisableAutoShutdown: true, Children: threeChildren()}
	s, err := unit.Spawn(t, factorySupErr, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	pids := childPIDs(t, s, 3)

	mark := s.Mark()
	s.Call(gen.PID{}, "q") // HandleCall error -> shutdown
	s.ShouldSendExit().Since(mark).Times(3).Assert()
	for _, p := range pids {
		s.DeliverExit(p, gen.TerminateReasonShutdown)
	}
	check.True(t, s.Terminated())
}

// Init validation: unknown restart strategy and Period-without-Intensity are rejected.
func TestSupervisorUnitInitInvalidRestartStrategy(t *testing.T) {
	spec := act.SupervisorSpec{
		Type: act.SupervisorTypeOneForOne,
		Children: []act.SupervisorChildSpec{
			{Name: "a", Factory: factorySupUnitChild, Restart: act.SupervisorChildRestart{Strategy: act.SupervisorStrategy(99)}},
		},
	}
	_, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.Error(t, err)
}

func TestSupervisorUnitInitPeriodWithoutIntensity(t *testing.T) {
	spec := act.SupervisorSpec{
		Type: act.SupervisorTypeOneForOne,
		Children: []act.SupervisorChildSpec{
			{Name: "a", Factory: factorySupUnitChild, Restart: act.SupervisorChildRestart{Period: 60}},
		},
	}
	_, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.Error(t, err)
}

// AllForOne with a Significant child: its (non-restarted) termination shuts the
// whole supervisor down after the group is terminated.
func TestSupervisorUnitAFOSignificant(t *testing.T) {
	spec := act.SupervisorSpec{
		Type:                act.SupervisorTypeAllForOne,
		DisableAutoShutdown: true,
		Children: []act.SupervisorChildSpec{
			{Name: "a", Factory: factorySupUnitChild},
			{Name: "b", Factory: factorySupUnitChild, Significant: true,
				Restart: act.SupervisorChildRestart{Strategy: act.SupervisorStrategyTemporary}},
			{Name: "c", Factory: factorySupUnitChild},
		},
	}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	pids := childPIDs(t, s, 3)

	mark := s.Mark()
	s.DeliverExit(pids[1], errors.New("crash")) // significant temporary -> group shutdown
	exits := s.ShouldSendExit().Since(mark).Collect()
	for _, e := range exits {
		s.DeliverExit(e.To, gen.TerminateReasonShutdown)
	}
	check.True(t, s.Terminated())
}

// AllForOne auto-shutdown: when the last child terminates normally (not restarted)
// and auto-shutdown is enabled, the supervisor stops.
func TestSupervisorUnitAFOAutoShutdown(t *testing.T) {
	spec := act.SupervisorSpec{
		Type:     act.SupervisorTypeAllForOne,
		Children: []act.SupervisorChildSpec{{Name: "a", Factory: factorySupUnitChild}},
	}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	pids := childPIDs(t, s, 1)

	s.DeliverExit(pids[0], gen.TerminateReasonNormal)
	check.True(t, s.Terminated())
}

// RestForOne with KeepOrder restarts the rest sequentially and in order.
func TestSupervisorUnitRFOKeepOrder(t *testing.T) {
	spec := act.SupervisorSpec{
		Type:                act.SupervisorTypeRestForOne,
		Restart:             act.SupervisorRestart{Strategy: act.SupervisorStrategyPermanent, KeepOrder: true},
		DisableAutoShutdown: true,
		Children:            threeChildren(),
	}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	pids := childPIDs(t, s, 3)
	a, b, c := pids[0], pids[1], pids[2]

	mark := s.Mark()
	s.DeliverExit(b, errors.New("crash")) // rest = c
	s.ShouldSendExit().To(c).Since(mark).Once().Assert()
	s.ShouldSendExit().To(a).Since(mark).None().Assert() // a preserved
	restartMark := s.Mark()
	s.DeliverExit(c, gen.TerminateReasonShutdown)
	s.ShouldSpawn().Register("b").Since(restartMark).Once().Assert()
	s.ShouldSpawn().Register("c").Since(restartMark).Once().Assert()
}

// when a restart spawn fails, the supervisor terminates abnormally.
func TestSupervisorUnitRestartSpawnFails(t *testing.T) {
	spec := act.SupervisorSpec{
		Type:                act.SupervisorTypeOneForOne,
		DisableAutoShutdown: true,
		Children:            []act.SupervisorChildSpec{{Name: "a", Factory: factorySupUnitChild}},
	}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	pids := childPIDs(t, s, 1)

	// init already spawned the child; make the restart spawn fail
	s.OnSpawn(factorySupUnitChild).Fail(gen.ErrProcessTerminated)
	s.DeliverExit(pids[0], errors.New("crash"))
	check.True(t, s.Terminated())
}

// an exit signal from a non-child process makes the supervisor shut its children
// down and stop (found == false path).
func TestSupervisorUnitUnknownExitShutsDown(t *testing.T) {
	spec := act.SupervisorSpec{
		Type:                act.SupervisorTypeOneForOne,
		DisableAutoShutdown: true,
		Children: []act.SupervisorChildSpec{
			{Name: "a", Factory: factorySupUnitChild},
			{Name: "b", Factory: factorySupUnitChild},
		},
	}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	pids := childPIDs(t, s, 2)

	stranger := gen.PID{Node: "unit@localhost", ID: 9999, Creation: 1}
	mark := s.Mark()
	s.DeliverExit(stranger, errors.New("boom")) // not a child -> terminate all children
	s.ShouldSendExit().Since(mark).Times(2).Assert()
	for _, p := range pids {
		s.DeliverExit(p, gen.TerminateReasonShutdown)
	}
	check.True(t, s.Terminated())
}

// the exit of an already-disabled child is ignored (spec.disabled path).
func TestSupervisorUnitDisabledChildExitIgnored(t *testing.T) {
	spec := act.SupervisorSpec{
		Type:                act.SupervisorTypeOneForOne,
		DisableAutoShutdown: true,
		Children: []act.SupervisorChildSpec{
			{Name: "a", Factory: factorySupUnitChild},
			{Name: "b", Factory: factorySupUnitChild},
		},
	}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	pids := childPIDs(t, s, 2)

	check.NoError(t, control(s).DisableChild("a"))
	mark := s.Mark()
	s.DeliverExit(pids[0], gen.TerminateReasonShutdown) // disabled child's exit -> no-op
	s.ShouldSpawn().Since(mark).None().Assert()
	check.False(t, s.Terminated())
}

// AllForOne: a non-child exit shuts the group down (found == false path in ARFO).
func TestSupervisorUnitAFOUnknownExitShutsDown(t *testing.T) {
	spec := act.SupervisorSpec{
		Type:                act.SupervisorTypeAllForOne,
		DisableAutoShutdown: true,
		Children: []act.SupervisorChildSpec{
			{Name: "a", Factory: factorySupUnitChild},
			{Name: "b", Factory: factorySupUnitChild},
		},
	}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	pids := childPIDs(t, s, 2)

	stranger := gen.PID{Node: "unit@localhost", ID: 9999, Creation: 1}
	mark := s.Mark()
	s.DeliverExit(stranger, errors.New("boom"))
	s.ShouldSendExit().Since(mark).Times(2).Assert()
	for _, p := range pids {
		s.DeliverExit(p, gen.TerminateReasonShutdown)
	}
	check.True(t, s.Terminated())
}

// AllForOne: the exit of an already-disabled child is ignored (spec.disabled path).
func TestSupervisorUnitAFODisabledChildExitIgnored(t *testing.T) {
	spec := act.SupervisorSpec{
		Type:                act.SupervisorTypeAllForOne,
		DisableAutoShutdown: true,
		Children: []act.SupervisorChildSpec{
			{Name: "a", Factory: factorySupUnitChild},
			{Name: "b", Factory: factorySupUnitChild},
		},
	}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	pids := childPIDs(t, s, 2)

	check.NoError(t, control(s).DisableChild("a"))
	mark := s.Mark()
	s.DeliverExit(pids[0], gen.TerminateReasonShutdown)
	s.ShouldSpawn().Since(mark).None().Assert()
	check.False(t, s.Terminated())
}

// regression: during a KeepOrder group restart the supervisor terminates children
// one at a time in reverse order. If another child dies on its own (out of order)
// while the supervisor is waiting for the current one, it must NOT panic - it keeps
// waiting and finishes the restart. (supervisor_arfo.go childTerminated keeporder path)
func TestSupervisorUnitKeepOrderOutOfOrderExit(t *testing.T) {
	spec := act.SupervisorSpec{
		Type:                act.SupervisorTypeAllForOne,
		Restart:             act.SupervisorRestart{Strategy: act.SupervisorStrategyPermanent, KeepOrder: true},
		DisableAutoShutdown: true,
		Children:            threeChildren(),
	}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	pids := childPIDs(t, s, 3)
	a, b, c := pids[0], pids[1], pids[2]

	// b dies -> group restart begins; KeepOrder terminates the last child (c) first.
	s.DeliverExit(b, errors.New("crash"))
	s.ShouldSendExit().To(c).Once().Assert()

	// while waiting for c, child a dies on its own (out of order) - must not panic.
	s.DeliverExit(a, errors.New("independent crash"))
	check.False(t, s.Terminated())

	// c's exit arrives -> the group restart completes, all children respawn.
	restartMark := s.Mark()
	s.DeliverExit(c, gen.TerminateReasonShutdown)
	check.False(t, s.Terminated())
	s.ShouldSpawn().Since(restartMark).Times(3).Assert()
}

// guard: a KeepOrder group restart survives a burst of out-of-order / independent
// child exits (no panic, no spurious shutdown), for both All- and Rest-For-One.
// Backs the analysis that childForStart's invariants are not exposed to reordering.
func TestSupervisorUnitKeepOrderStressNoPanic(t *testing.T) {
	four := []act.SupervisorChildSpec{
		{Name: "a", Factory: factorySupUnitChild},
		{Name: "b", Factory: factorySupUnitChild},
		{Name: "c", Factory: factorySupUnitChild},
		{Name: "d", Factory: factorySupUnitChild},
	}
	for _, typ := range []act.SupervisorType{act.SupervisorTypeAllForOne, act.SupervisorTypeRestForOne} {
		spec := act.SupervisorSpec{
			Type:                typ,
			Restart:             act.SupervisorRestart{Strategy: act.SupervisorStrategyPermanent, KeepOrder: true},
			DisableAutoShutdown: true,
			Children:            four,
		}
		s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
		check.NoError(t, err)
		pids := childPIDs(t, s, 4)

		s.DeliverExit(pids[1], errors.New("crash"))         // trigger group restart
		s.DeliverExit(pids[0], errors.New("independent a")) // out-of-order independent exits
		s.DeliverExit(pids[3], errors.New("independent d"))
		s.DeliverExit(pids[2], gen.TerminateReasonShutdown) // complete

		check.False(t, s.Terminated())
	}
}

// trivial coverage of the enum String()s and ProcessKind.
func TestSupervisorUnitStringsAndKind(t *testing.T) {
	_ = act.SupervisorTypeAllForOne.String()
	_ = act.SupervisorTypeRestForOne.String()
	_ = act.SupervisorTypeSimpleOneForOne.String()
	_ = act.SupervisorType(99).String() // default branch
	_ = act.SupervisorStrategyPermanent.String()
	_ = act.SupervisorStrategyTransient.String()
	_ = act.SupervisorStrategyTemporary.String()
	_ = act.SupervisorStrategy(99).String() // default branch
	_ = act.OnExceedDisable.String()
	_ = act.OnExceedTerminateSupervisor.String()
	_ = act.OnExceed(99).String() // default branch

	spec := act.SupervisorSpec{Type: act.SupervisorTypeOneForOne, DisableAutoShutdown: true, Children: threeChildren()}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	kind := s.Behavior().(interface{ ProcessKind() gen.ProcessKind }).ProcessKind()
	check.Equal(t, gen.ProcessKindSupervisor, kind)
}

// SOFO EnableChild re-enables a disabled template.
func TestSupervisorUnitSOFOEnableChild(t *testing.T) {
	spec := act.SupervisorSpec{
		Type:     act.SupervisorTypeSimpleOneForOne,
		Children: []act.SupervisorChildSpec{{Name: "worker", Factory: factorySupUnitChild}},
	}
	s, err := unit.Spawn(t, factorySupUnit, gen.ProcessOptions{}, spec)
	check.NoError(t, err)
	check.NoError(t, control(s).DisableChild("worker"))
	check.NoError(t, control(s).EnableChild("worker"))
}
