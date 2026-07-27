package local

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/app"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// appEnvBasic is the application-level env both basic apps carry; members inherit it.
var appEnvBasic = map[gen.Env]any{"TEST": 12345, "VALUE": "09887"}

// appMember is a group member: reports its inherited env and spawns a sibling on demand.
type appMember struct{ act.Actor }

func factoryAppMember() gen.ProcessBehavior { return &appMember{} }

func (m *appMember) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	switch request {
	case "env":
		return m.EnvList(), nil
	case "spawn":
		pid, err := m.Spawn(factoryAppMember, gen.ProcessOptions{})
		if err != nil {
			return nil, err
		}
		return pid, nil
	}
	return "ok", nil
}

// appBasic is the autostarted application with a single named member.
type appBasic struct{ app.Application }

func createAppBasic() gen.ApplicationBehavior { return &appBasic{} }

func (a *appBasic) Load(args ...any) (gen.ApplicationSpec, error) {
	return gen.ApplicationSpec{
		Name:  "test_app",
		Group: []gen.ApplicationMemberSpec{{Name: "test_member", Factory: factoryAppMember}},
		Env:   appEnvBasic,
	}, nil
}

// appDep depends on test_app and must refuse to start until that dependency is loaded.
type appDep struct{ app.Application }

func createAppDep() gen.ApplicationBehavior { return &appDep{} }

func (a *appDep) Load(args ...any) (gen.ApplicationSpec, error) {
	spec := gen.ApplicationSpec{
		Name:  "test_app_dep",
		Group: []gen.ApplicationMemberSpec{{Factory: factoryAppMember}},
		Env:   appEnvBasic,
	}
	spec.Depends.Applications = []gen.Atom{"test_app"}
	return spec, nil
}

// appMode has three killable members and reports its termination reason to a
// collector, making the per-mode auto-stop behavior observable in the stage.
type appMode struct {
	app.Application
	collector gen.PID
	mode      gen.ApplicationMode
}

func createAppMode(collector gen.PID, mode gen.ApplicationMode) gen.ApplicationBehavior {
	return &appMode{collector: collector, mode: mode}
}

func (a *appMode) Load(args ...any) (gen.ApplicationSpec, error) {
	return gen.ApplicationSpec{
		Name: "test_app_mode",
		Group: []gen.ApplicationMemberSpec{
			{Factory: factoryEcho},
			{Factory: factoryEcho},
			{Factory: factoryEcho},
		},
		Env:  appEnvBasic,
		Mode: a.mode,
	}, nil
}

func (a *appMode) Terminate(reason error) {
	a.Node().Send(a.collector, reason)
}

// TestLocalApplicationBasic: an autostarted app exposes its single named member
// (correct app name, member name, inherited env); a member can spawn an unnamed
// sibling that inherits the same app and env; unloading a running app is
// ErrApplicationRunning; Stop leaves it loaded-but-not-running; Unload removes it;
// an app with an unsatisfied dependency is ErrApplicationDepends on start, and
// starts once the dependency is loaded.
func TestLocalApplicationBasic(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n", stage.NodeOptions{
		Applications: []gen.ApplicationBehavior{createAppBasic()},
	})
	nn := n.Native()

	// the app autostarted
	running := nn.ApplicationsRunning()
	check.Equal(t, 1, len(running))
	check.Equal(t, gen.Atom("test_app"), running[0])

	// exactly one process: the named member
	list, err := nn.ProcessList()
	check.NoError(t, err)
	check.Equal(t, 1, len(list))

	info, err := nn.ProcessInfo(list[0])
	check.NoError(t, err)
	check.Equal(t, gen.Atom("test_app"), info.Application)
	check.Equal(t, gen.Atom("test_member"), info.Name)

	// the member's env is the application env
	env, err := n.Call(list[0], "env")
	check.NoError(t, err)
	check.True(t, reflect.DeepEqual(env, appEnvBasic))

	// the member spawns an unnamed sibling that inherits app and env
	npAny, err := n.Call(list[0], "spawn")
	check.NoError(t, err)
	newpid, ok := npAny.(gen.PID)
	check.True(t, ok)
	newinfo, err := nn.ProcessInfo(newpid)
	check.NoError(t, err)
	check.Equal(t, gen.Atom("test_app"), newinfo.Application)
	check.Equal(t, gen.Atom(""), newinfo.Name)
	env2, err := n.Call(newpid, "env")
	check.NoError(t, err)
	check.True(t, reflect.DeepEqual(env2, appEnvBasic))

	// unloading a running app is rejected
	check.ErrorIs(t, nn.ApplicationUnload("test_app"), gen.ErrApplicationRunning)

	// stop: no longer running, but still loaded
	check.NoError(t, nn.ApplicationStop("test_app"))
	check.Equal(t, 0, len(nn.ApplicationsRunning()))
	loaded := nn.Applications()
	check.Equal(t, 1, len(loaded))
	check.Equal(t, gen.Atom("test_app"), loaded[0])

	// unload removes it entirely
	check.NoError(t, nn.ApplicationUnload("test_app"))
	check.Equal(t, 0, len(nn.Applications()))

	// a dependent app refuses to start with its dependency unloaded
	appdep, err := nn.ApplicationLoad(createAppDep())
	check.NoError(t, err)
	check.ErrorIs(t, nn.ApplicationStart(appdep, gen.ApplicationOptions{}), gen.ErrApplicationDepends)

	// load the dependency, then the dependent app starts
	_, err = nn.ApplicationLoad(createAppBasic())
	check.NoError(t, err)
	check.NoError(t, nn.ApplicationStart(appdep, gen.ApplicationOptions{}))
	check.Equal(t, 2, len(nn.ApplicationsRunning()))
}

// TestLocalApplicationProcessListShortInfo: the process list is returned in
// ascending id order (parent before child), and the second return value is the
// number of application processes omitted when the result hits the limit (0 when
// the whole tree fits). Regression for the app-tree truncation count.
func TestLocalApplicationProcessListShortInfo(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n", stage.NodeOptions{
		Applications: []gen.ApplicationBehavior{createAppBasic()},
	})
	nn := n.Native()

	// start with the single named member, then grow the app: each "spawn" adds
	// one process under test_app with a strictly higher id.
	list, err := nn.ProcessList()
	check.NoError(t, err)
	check.Equal(t, 1, len(list))
	root := list[0]
	const extra = 4
	for i := 0; i < extra; i++ {
		if _, err := n.Call(root, "spawn"); err != nil {
			t.Fatalf("spawn %d: %s", i, err)
		}
	}
	total := 1 + extra

	// whole tree fits: every process, ascending id order, nothing omitted
	all, omitted, err := nn.ApplicationProcessListShortInfo("test_app", total)
	check.NoError(t, err)
	check.Equal(t, total, len(all))
	check.Equal(t, 0, omitted)
	for i, p := range all {
		check.Equal(t, gen.Atom("test_app"), p.Application)
		if i > 0 && all[i-1].PID.ID >= p.PID.ID {
			t.Fatalf("not ascending id order at %d: %d >= %d", i, all[i-1].PID.ID, p.PID.ID)
		}
	}

	// limit below total: exactly limit shown (the lowest ids), remainder omitted
	const limit = 2
	shown, omitted2, err := nn.ApplicationProcessListShortInfo("test_app", limit)
	check.NoError(t, err)
	check.Equal(t, limit, len(shown))
	check.Equal(t, total-limit, omitted2)
	for i := 0; i < limit; i++ {
		check.Equal(t, all[i].PID, shown[i].PID)
	}

	// unknown application
	_, _, err = nn.ApplicationProcessListShortInfo("no_such_app", 10)
	check.ErrorIs(t, err, gen.ErrApplicationUnknown)
}

// TestLocalApplicationProcessList: the PID list is returned in ascending id order;
// a limit of 0 returns every process, a positive limit caps to the lowest ids, a
// negative limit is ErrIncorrect.
func TestLocalApplicationProcessList(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n", stage.NodeOptions{
		Applications: []gen.ApplicationBehavior{createAppBasic()},
	})
	nn := n.Native()

	list, err := nn.ProcessList()
	check.NoError(t, err)
	check.Equal(t, 1, len(list))
	root := list[0]
	const extra = 4
	for i := 0; i < extra; i++ {
		if _, err := n.Call(root, "spawn"); err != nil {
			t.Fatalf("spawn %d: %s", i, err)
		}
	}
	total := 1 + extra

	// limit 0 returns all, ascending id order
	all, err := nn.ApplicationProcessList("test_app", 0)
	check.NoError(t, err)
	check.Equal(t, total, len(all))
	for i := 1; i < len(all); i++ {
		if all[i-1].ID >= all[i].ID {
			t.Fatalf("not ascending id order at %d: %d >= %d", i, all[i-1].ID, all[i].ID)
		}
	}

	// positive limit caps to the lowest ids
	capped, err := nn.ApplicationProcessList("test_app", 2)
	check.NoError(t, err)
	check.Equal(t, 2, len(capped))
	check.Equal(t, all[0], capped[0])
	check.Equal(t, all[1], capped[1])

	// negative limit
	_, err = nn.ApplicationProcessList("test_app", -1)
	check.ErrorIs(t, err, gen.ErrIncorrect)

	// unknown application
	_, err = nn.ApplicationProcessList("no_such_app", 0)
	check.ErrorIs(t, err, gen.ErrApplicationUnknown)
}

// TestLocalApplicationMode: an application's mode governs the reason it terminates
// with when its group members die.
//   - Temporary: members dying never auto-stops; the app ends (always Normal) only
//     once the last member is gone.
//   - Transient: a member dying Normal/Shutdown does not stop the app (it ends
//     Normal when the last member is gone); a member dying abnormally stops the
//     app with that exact reason.
//   - Permanent: any single member dying stops the whole app with that exact reason.
//
// The app reports its termination reason to a collector, asserted deterministically.
func TestLocalApplicationMode(t *testing.T) {
	normal, shutdown, kill := gen.TerminateReasonNormal, gen.TerminateReasonShutdown, gen.TerminateReasonKill
	custom := errors.New("whatever")
	reasons := []error{normal, shutdown, kill, custom}

	t.Run("Temporary", func(t *testing.T) {
		s := stage.New(t)
		n := s.StartNode("n")
		nn := n.Native()
		collector := n.Spawn(factoryEcho, gen.ProcessOptions{})

		appName, err := nn.ApplicationLoad(createAppMode(collector, gen.ApplicationModeTemporary))
		check.NoError(t, err)

		for _, reason := range reasons {
			check.NoError(t, nn.ApplicationStart(appName, gen.ApplicationOptions{}))
			gi, err := nn.ApplicationInfo(appName)
			check.NoError(t, err)
			check.Equal(t, 3, len(gi.Group))

			mk := n.Mark()
			// must kill every member for the app to stop; the reason is always Normal
			for _, pid := range gi.Group {
				check.NoError(t, nn.SendExit(pid, reason))
			}
			n.ShouldDeliver().To(collector).Message(normal).Since(mk).Once().Within(time.Second).Must()
		}
	})

	t.Run("Transient", func(t *testing.T) {
		s := stage.New(t)
		n := s.StartNode("n")
		nn := n.Native()
		collector := n.Spawn(factoryEcho, gen.ProcessOptions{})

		appName, err := nn.ApplicationLoad(createAppMode(collector, gen.ApplicationModeTransient))
		check.NoError(t, err)

		for _, reason := range reasons {
			check.NoError(t, nn.ApplicationStart(appName, gen.ApplicationOptions{}))
			gi, err := nn.ApplicationInfo(appName)
			check.NoError(t, err)
			check.Equal(t, 3, len(gi.Group))

			mk := n.Mark()
			want := normal
			for _, pid := range gi.Group {
				check.NoError(t, nn.SendExit(pid, reason))
				if reason == normal || reason == shutdown {
					continue
				}
				// an abnormal member death stops the app with that exact reason
				want = reason
				break
			}
			n.ShouldDeliver().To(collector).Message(want).Since(mk).Once().Within(time.Second).Must()
		}
	})

	t.Run("Permanent", func(t *testing.T) {
		s := stage.New(t)
		n := s.StartNode("n")
		nn := n.Native()
		collector := n.Spawn(factoryEcho, gen.ProcessOptions{})

		appName, err := nn.ApplicationLoad(createAppMode(collector, gen.ApplicationModePermanent))
		check.NoError(t, err)

		for _, reason := range reasons {
			check.NoError(t, nn.ApplicationStart(appName, gen.ApplicationOptions{}))
			gi, err := nn.ApplicationInfo(appName)
			check.NoError(t, err)
			check.Equal(t, 3, len(gi.Group))

			mk := n.Mark()
			// one member is enough to stop the whole app, with that exact reason
			check.NoError(t, nn.SendExit(gi.Group[1], reason))
			n.ShouldDeliver().To(collector).Message(reason).Since(mk).Once().Within(time.Second).Must()
		}
	})
}

// TestLocalApplicationEnvEffective: Application.Env()/EnvList() (seen here via
// ApplicationInfo.Env) report the effective environment - node core env +
// ApplicationSpec.Env + per-start ApplicationOptions.Env while running, and only
// ApplicationSpec.Env while not running. The per-start layer is replaced, not
// accumulated, across starts. Regression for #250.
func TestLocalApplicationEnvEffective(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n", stage.NodeOptions{
		Env:      map[gen.Env]any{"NODE": "core"},
		Security: gen.SecurityOptions{ExposeEnvInfo: true},
	})
	nn := n.Native()

	appName, err := nn.ApplicationLoad(createAppBasic())
	check.NoError(t, err)

	// loaded, not started: node env is merged into the spec at load, but the
	// per-start layer is not applied yet
	info, err := nn.ApplicationInfo(appName)
	check.NoError(t, err)
	check.Equal(t, "core", info.Env["NODE"]) // node env (merged at load)
	check.Equal(t, 12345, info.Env["TEST"])  // spec env
	_, hasOpt := info.Env["OPT"]
	check.Equal(t, false, hasOpt)

	// started: effective env now also carries the per-start env
	check.NoError(t, nn.ApplicationStart(appName, gen.ApplicationOptions{Env: map[gen.Env]any{"OPT": "a"}}))
	info, err = nn.ApplicationInfo(appName)
	check.NoError(t, err)
	check.Equal(t, "core", info.Env["NODE"])
	check.Equal(t, 12345, info.Env["TEST"])
	check.Equal(t, "a", info.Env["OPT"]) // per-start env (the #250 fix)

	// stopped: the per-start layer is gone
	check.NoError(t, nn.ApplicationStop(appName))
	info, err = nn.ApplicationInfo(appName)
	check.NoError(t, err)
	check.Equal(t, 12345, info.Env["TEST"])
	_, hasOpt = info.Env["OPT"]
	check.Equal(t, false, hasOpt)

	// restarted with a different per-start env: no overlap with the previous start
	check.NoError(t, nn.ApplicationStart(appName, gen.ApplicationOptions{Env: map[gen.Env]any{"OPT2": "b"}}))
	info, err = nn.ApplicationInfo(appName)
	check.NoError(t, err)
	check.Equal(t, "b", info.Env["OPT2"])
	_, hasOpt = info.Env["OPT"]
	check.Equal(t, false, hasOpt)
}

// TestLocalApplicationStartModeDependencies: the mode-specific start entry points
// (Permanent/Transient/Temporary) resolve ApplicationSpec.Depends.Applications just
// like ApplicationStart - refused with ErrApplicationDepends while a dependency is
// unloaded, succeeding once it is loaded. Regression for #240.
func TestLocalApplicationStartModeDependencies(t *testing.T) {
	starts := []struct {
		name  string
		start func(gen.Node, gen.Atom) error
	}{
		{"permanent", func(nd gen.Node, name gen.Atom) error { return nd.ApplicationStartPermanent(name, gen.ApplicationOptions{}) }},
		{"transient", func(nd gen.Node, name gen.Atom) error { return nd.ApplicationStartTransient(name, gen.ApplicationOptions{}) }},
		{"temporary", func(nd gen.Node, name gen.Atom) error { return nd.ApplicationStartTemporary(name, gen.ApplicationOptions{}) }},
	}
	for _, tc := range starts {
		t.Run(tc.name, func(t *testing.T) {
			s := stage.New(t)
			n := s.StartNode("n", stage.NodeOptions{})
			nn := n.Native()

			// appDep depends on test_app; starting it with the dependency unloaded is refused
			appdep, err := nn.ApplicationLoad(createAppDep())
			check.NoError(t, err)
			check.ErrorIs(t, tc.start(nn, appdep), gen.ErrApplicationDepends)

			// load the dependency, then the mode-specific start resolves it and succeeds
			_, err = nn.ApplicationLoad(createAppBasic())
			check.NoError(t, err)
			check.NoError(t, tc.start(nn, appdep))
			check.Equal(t, 2, len(nn.ApplicationsRunning()))
		})
	}
}
