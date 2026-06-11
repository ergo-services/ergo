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
	n := s.Node("n", stage.NodeOptions{
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
		n := s.Node("n")
		nn := n.Native()
		collector := n.Spawn(factoryEcho)

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
		n := s.Node("n")
		nn := n.Native()
		collector := n.Spawn(factoryEcho)

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
		n := s.Node("n")
		nn := n.Native()
		collector := n.Spawn(factoryEcho)

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
