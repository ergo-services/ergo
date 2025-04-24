package local

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/node"
)

func createTestApp() gen.ApplicationBehavior {
	return &testApp{}
}

type testApp struct{}

func (ta *testApp) Load(node gen.Node, args ...any) (gen.ApplicationSpec, error) {
	return gen.ApplicationSpec{
		Name: "test_app",
		Group: []gen.ApplicationMemberSpec{
			{
				Name:    "test_member",
				Factory: factory_t12,
			},
		},
		Env: appEnv,
	}, nil
}

func (ta *testApp) Start(mode gen.ApplicationMode) {}
func (ta *testApp) Terminate(reason error)         {}

func createTestAppDep() gen.ApplicationBehavior {
	return &testAppDep{}
}

type testAppDep struct{}

func (tad *testAppDep) Load(node gen.Node, args ...any) (gen.ApplicationSpec, error) {
	spec := gen.ApplicationSpec{
		Name: "test_app_dep",
		Group: []gen.ApplicationMemberSpec{
			{
				Factory: factory_t12,
			},
		},
		Env: appEnv,
	}

	spec.Depends.Applications = []gen.Atom{"test_app"}
	return spec, nil
}
func (tad *testAppDep) Start(mode gen.ApplicationMode) {}
func (tad *testAppDep) Terminate(reason error)         {}

func createTestAppMode(testcase *testcase, mode gen.ApplicationMode) gen.ApplicationBehavior {
	return &testAppMode{
		testcase: testcase,
		mode:     mode,
	}
}

type testAppMode struct {
	testcase *testcase
	mode     gen.ApplicationMode
}

func (tam *testAppMode) Load(node gen.Node, args ...any) (gen.ApplicationSpec, error) {
	spec := gen.ApplicationSpec{
		Name: "test_app_mode",
		Group: []gen.ApplicationMemberSpec{
			{
				Factory: factory_t12,
			},
			{
				Factory: factory_t12,
			},
			{
				Factory: factory_t12,
			},
		},
		Env:  appEnv,
		Mode: tam.mode,
	}
	return spec, nil
}
func (tad *testAppMode) Start(mode gen.ApplicationMode) {}
func (tad *testAppMode) Terminate(reason error) {
	select {
	case tad.testcase.err <- reason:
	default:
	}
}

var (
	appEnv = map[gen.Env]any{
		"TEST":  12345,
		"VALUE": "09887",
	}
	t12cases []*testcase
)

func factory_t12() gen.ProcessBehavior {
	return &t12{}
}

type t12 struct {
	act.Actor

	testcase *testcase
}

func (t *t12) HandleMessage(from gen.PID, message any) error {
	if t.testcase == nil {
		t.testcase = message.(*testcase)
		message = initcase{}
	}
	// get method by name
	method := reflect.ValueOf(t).MethodByName(t.testcase.name)
	if method.IsValid() == false {
		t.testcase.err <- fmt.Errorf("unknown method %q", t.testcase.name)
		t.testcase = nil
		return nil
	}
	method.Call([]reflect.Value{reflect.ValueOf(message)})
	return nil
}

func (t *t12) TestBasic(input any) {
	defer func() {
		t.testcase = nil
	}()

	nopt := gen.NodeOptions{}
	// nopt.Log.Level = gen.LogLevelTrace
	nopt.Applications = append(nopt.Applications, createTestApp())
	nopt.Log.DefaultLogger.Disable = true
	node, err := node.Start("t12nodeBasic@localhost", nopt, gen.Version{})
	if err != nil {
		t.Log().Error("unable to start new node: %s", node)
		t.testcase.err <- err
		return
	}
	defer node.Stop()

	// check autostarted app
	if apps := node.ApplicationsRunning(); len(apps) != 1 || apps[0] != "test_app" {
		t.Log().Error("incorrect application list: %s", apps)
		t.testcase.err <- errIncorrect
		return
	}

	list, err := node.ProcessList()
	if err != nil {
		t.Log().Error("process list failed: %s", err)
		t.testcase.err <- err
		return
	}

	// must be 1 process
	if len(list) != 1 {
		t.Log().Error("incorrect process list (exp 1 member) : %s", list)
		t.testcase.err <- errIncorrect
		return
	}

	info, err := node.ProcessInfo(list[0])
	if err != nil {
		t.Log().Error("process info failed: %s", err)
		t.testcase.err <- err
		return
	}
	// check app name of the process
	if info.Application != "test_app" {
		t.Log().Error("process app name incorrect: %s", info.Application)
		t.testcase.err <- errIncorrect
		return
	}
	if info.Name != "test_member" {
		t.Log().Error("incorrect member process name: %s", info.Name)
		t.testcase.err <- errIncorrect
		return
	}

	docase := &testcase{"Do", nil, nil, make(chan error)}
	node.Send(list[0], docase)
	if err := docase.wait(1); err != nil {
		t.Log().Error("unable to get process env: %s", err)
		t.testcase.err <- err
		return
	}

	if reflect.DeepEqual(docase.output, appEnv) == false {
		t.Log().Error("incorrect env vars: %#v exp %#v ", docase.output, appEnv)
		t.testcase.err <- err
		return
	}

	// ---- new member start tests
	//ask to spawn a new proc
	node.Send(list[0], 1)
	if err := docase.wait(1); err != nil {
		t.Log().Error("unable to get process env: %s", err)
		t.testcase.err <- err
		return
	}
	newpid, ok := docase.output.(gen.PID)
	if ok == false {
		t.Log().Error("incorrect pid value: %#v ", docase.output)
		t.testcase.err <- errIncorrect
	}
	newinfo, err := node.ProcessInfo(newpid)
	if err != nil {
		t.Log().Error("new process info failed: %s", err)
		t.testcase.err <- err
		return
	}

	// check app name of the new process
	if newinfo.Application != "test_app" {
		t.Log().Error("new process app name incorrect: %s", newinfo.Application)
		t.testcase.err <- errIncorrect
		return
	}
	if newinfo.Name != "" {
		t.Log().Error("incorrect new member process name: %s", newinfo.Name)
		t.testcase.err <- errIncorrect
		return
	}

	newdocase := &testcase{"Do", nil, nil, make(chan error)}
	node.Send(newpid, newdocase)
	if err := newdocase.wait(1); err != nil {
		t.Log().Error("unable to get new process env: %s", err)
		t.testcase.err <- err
		return
	}

	if reflect.DeepEqual(newdocase.output, appEnv) == false {
		t.Log().Error("incorrect env vars: %#v exp %#v ", newdocase.output, appEnv)
		t.testcase.err <- err
		return
	}
	// ---- new member stop tests

	// try to unload running app. must be err
	if err := node.ApplicationUnload("test_app"); err != gen.ErrApplicationRunning {
		t.Log().Error("expecting gen.ErrApplicationRunning, got: %s", err)
		t.testcase.err <- err
		return
	}

	// stop app and check
	if err := node.ApplicationStop("test_app"); err != nil {
		t.Log().Error("unable to stop application: %s", err)
		t.testcase.err <- err
		return
	}

	if apps := node.ApplicationsRunning(); len(apps) != 0 {
		t.Log().Error("incorrect application list: %s", apps)
		t.testcase.err <- errIncorrect
		return
	}
	// check the app. must be still loaded
	if apps := node.Applications(); len(apps) != 1 || apps[0] != "test_app" {
		t.Log().Error("incorrect application list: %s", apps)
		t.testcase.err <- errIncorrect
		return
	}

	// check unload
	if err := node.ApplicationUnload("test_app"); err != nil {
		t.Log().Error("unable to unload application: %s", err)
		t.testcase.err <- err
		return
	}

	if l := node.Applications(); len(l) != 0 {
		t.Log().Error("incorrect application list: %v", l)
		t.testcase.err <- err
		return

	}

	// load app with dep
	appdep, err := node.ApplicationLoad(createTestAppDep())
	if err != nil {
		t.Log().Error("unable to load test_app_dep: %s", err)
		t.testcase.err <- err
		return
	}

	// try to start it (with no loaded dep app). must fail
	if err := node.ApplicationStart(appdep, gen.ApplicationOptions{}); err != gen.ErrApplicationDepends {
		t.Log().Error("must be failed on start test_app_dep with unloaded test_app. got: %s", err)
		t.testcase.err <- errIncorrect
	}

	// load dep app
	if _, err := node.ApplicationLoad(createTestApp()); err != nil {
		t.Log().Error("unable to load test_app again: %s", err)
		t.testcase.err <- err
	}

	// now it must be started
	if err := node.ApplicationStart(appdep, gen.ApplicationOptions{}); err != nil {
		t.Log().Error("unable to start test_app_dep. got: %s", err)
		t.testcase.err <- err
	}

	// check application list (running)
	if l := node.ApplicationsRunning(); len(l) != 2 {
		t.Log().Error("incorrect application list: %v", l)
		t.testcase.err <- errIncorrect
	}

	t.testcase.err <- nil
}

func (t *t12) TestApplicationMode(input any) {
	defer func() {
		t.testcase = nil
	}()

	mode := t.testcase.input.(string)
	switch mode {
	case "Temporary":
		nopt := gen.NodeOptions{}
		nopt.Log.DefaultLogger.Disable = true
		node, err := node.Start("t12nodeAppTemporary@localhost", nopt, gen.Version{})
		if err != nil {
			t.Log().Error("unable to start new node: %s", node)
			t.testcase.err <- err
			return
		}
		defer node.Stop()

		appcase := &testcase{"appcase", nil, nil, make(chan error)}
		app := createTestAppMode(appcase, gen.ApplicationModeTemporary)
		appName, err := node.ApplicationLoad(app)
		if err != nil {
			t.Log().Error("unable to load app: %s", err)
			t.testcase.err <- err
			return
		}

		for _, reason := range []error{gen.TerminateReasonNormal, gen.TerminateReasonShutdown, gen.TerminateReasonKill, errors.New("whatever")} {

			if err := node.ApplicationStart(appName, gen.ApplicationOptions{}); err != nil {
				t.Log().Error("unable to start %s. got: %s", appName, err)
				t.testcase.err <- err
				return
			}

			appInfo, err := node.ApplicationInfo(appName)
			if err != nil {
				t.Log().Error("unable to get app info %s. got: %s", appName, err)
				t.testcase.err <- err
				return
			}

			for _, pid := range appInfo.Group {
				if err := node.SendExit(pid, reason); err != nil {
					panic(err)
				}
			}
			// app termination reason must be gen.TerminateReasonNormal. always.
			if err := appcase.wait(1); err != gen.TerminateReasonNormal {
				t.Log().Error("application must be terminated with reason 'normal'. got: %s", err)
				if err == nil {
					t.testcase.err <- errIncorrect
					return
				}
				t.testcase.err <- err
				return
			}

		}
	case "Transient":
		nopt := gen.NodeOptions{}
		nopt.Log.DefaultLogger.Disable = true
		node, err := node.Start("t12nodeAppTransient@localhost", nopt, gen.Version{})
		if err != nil {
			t.Log().Error("unable to start new node: %s", node)
			t.testcase.err <- err
			return
		}
		defer node.Stop()

		appcase := &testcase{"appcase", nil, nil, make(chan error)}
		app := createTestAppMode(appcase, gen.ApplicationModeTransient)
		appName, err := node.ApplicationLoad(app)
		if err != nil {
			t.Log().Error("unable to load app: %s", err)
			t.testcase.err <- err
			return
		}

		for _, reason := range []error{gen.TerminateReasonNormal, gen.TerminateReasonShutdown, gen.TerminateReasonKill, errors.New("whatever")} {

			if err := node.ApplicationStart(appName, gen.ApplicationOptions{}); err != nil {
				t.Log().Error("unable to start %s. got: %s", appName, err)
				t.testcase.err <- err
				return
			}

			appInfo, err := node.ApplicationInfo(appName)
			if err != nil {
				t.Log().Error("unable to get app info %s. got: %s", appName, err)
				t.testcase.err <- err
				return
			}

			expectingAppTermReason := gen.TerminateReasonNormal
			for _, pid := range appInfo.Group {
				if err := node.SendExit(pid, reason); err != nil {
					panic(err)
				}
				if reason == gen.TerminateReasonNormal || reason == gen.TerminateReasonShutdown {
					continue
				}
				expectingAppTermReason = reason
				break
			}
			if err := appcase.wait(1); err != expectingAppTermReason {
				t.Log().Error("application must be terminated with reason '%s'. got: %s", expectingAppTermReason, err)
				if err == nil {
					t.testcase.err <- errIncorrect
					return
				}
				t.testcase.err <- err
				return
			}

		}

	case "Permanent":
		nopt := gen.NodeOptions{}
		nopt.Log.DefaultLogger.Disable = true
		node, err := node.Start("t12nodeAppPermanent@localhost", nopt, gen.Version{})
		if err != nil {
			t.Log().Error("unable to start new node: %s", node)
			t.testcase.err <- err
			return
		}
		defer node.Stop()

		appcase := &testcase{"appcase", nil, nil, make(chan error)}
		app := createTestAppMode(appcase, gen.ApplicationModePermanent)
		appName, err := node.ApplicationLoad(app)
		if err != nil {
			t.Log().Error("unable to load app: %s", err)
			t.testcase.err <- err
			return
		}

		for _, reason := range []error{gen.TerminateReasonNormal, gen.TerminateReasonShutdown, gen.TerminateReasonKill, errors.New("whatever")} {

			if err := node.ApplicationStart(appName, gen.ApplicationOptions{}); err != nil {
				t.Log().Error("unable to start %s. got: %s", appName, err)
				t.testcase.err <- err
				return
			}

			appInfo, err := node.ApplicationInfo(appName)
			if err != nil {
				t.Log().Error("unable to get app info %s. got: %s", appName, err)
				t.testcase.err <- err
				return
			}

			// must be enough to terminate just one member to terminate whole app
			if err := node.SendExit(appInfo.Group[1], reason); err != nil {
				panic(err)
			}
			if err := appcase.wait(1); err != reason {
				t.Log().Error("application must be terminated with reason '%s'. got: %s", reason, err)
				if err == nil {
					t.testcase.err <- errIncorrect
					return
				}
				t.testcase.err <- err
				return
			}

		}

	default:
	}

	t.testcase.err <- nil
}

func (t *t12) Do(input any) {
	if _, ok := input.(initcase); ok {
		env := t.EnvList()
		t.testcase.output = env
		t.testcase.err <- nil
		return
	}

	// pid, err := t.Spawn(factory_t12, gen.ProcessOptions{LinkParent: true})
	pid, err := t.Spawn(factory_t12, gen.ProcessOptions{})
	if err != nil {
		t.Log().Error("unable to spawn new process by request: %s", err)
		t.testcase.err <- err
		return
	}
	t.testcase.output = pid
	t.testcase.err <- nil
}

func TestT12Application(t *testing.T) {
	nopt := gen.NodeOptions{}
	nopt.Log.DefaultLogger.Disable = true
	// nopt.Log.Level = gen.LogLevelTrace
	node, err := node.Start("t12node@localhost", nopt, gen.Version{})
	if err != nil {
		t.Fatal(err)
	}

	popt := gen.ProcessOptions{}
	pid, err := node.Spawn(factory_t12, popt)
	if err != nil {
		panic(err)
	}

	t12cases = []*testcase{
		{"TestBasic", nil, nil, make(chan error)},
		{"TestApplicationMode", "Temporary", nil, make(chan error)},
		{"TestApplicationMode", "Transient", nil, make(chan error)},
		{"TestApplicationMode", "Permanent", nil, make(chan error)},
	}
	for _, tc := range t12cases {
		name := tc.name
		if tc.input != nil {
			name = fmt.Sprintf("%s:%s", tc.name, tc.input)
		}
		t.Run(name, func(t *testing.T) {
			node.Send(pid, tc)
			if err := tc.wait(3); err != nil {
				t.Fatal(err)
			}
		})
	}

	node.Stop()
}
