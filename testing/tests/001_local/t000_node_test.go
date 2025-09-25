package local

import (
	"reflect"
	"testing"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/node"
)

func factory_t0() gen.ProcessBehavior {
	return &t0{}
}

type t0 struct {
	act.Actor
}

func TestT0NodeBasic(t *testing.T) {
	nodename := gen.Atom("t0node@localhost")
	env := map[gen.Env]any{
		gen.Env("A"): 1,
		gen.Env("B"): 1.23,
		gen.Env("C"): "d",
	}

	nopt := gen.NodeOptions{
		Env: env,
	}
	nopt.Log.DefaultLogger.Disable = true
	// use the direct method to start node with no applications
	//node, err := ergo.StartNode(nodename, nopt)
	node, err := node.Start(nodename, nopt, gen.Version{})
	if err != nil {
		t.Fatal(err)
	}
	if node.Name() != nodename {
		t.Fatal("wrong nodename")
	}

	if node.IsAlive() == false {
		t.Fatal("must be alive")
	}

	info, err := node.Info()
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != nodename {
		t.Fatal("wrong nodename")
	}

	if info.ProcessesTotal > 0 {
		t.Fatal(errIncorrect)
	}
	if info.ProcessesRunning > 0 {
		t.Fatal(errIncorrect)
	}
	if info.ProcessesZombee > 0 {
		t.Fatal(errIncorrect)
	}
	if info.RegisteredAliases > 0 {
		t.Fatal(errIncorrect)
	}
	if info.RegisteredNames > 0 {
		t.Fatal(errIncorrect)
	}

	// check node env
	nenv := node.EnvList()
	if reflect.DeepEqual(env, nenv) == false {
		t.Fatal("unequal env")
	}

	// get the value of env variable
	if v, exist := node.Env("a"); exist {
		if v != nenv["A"] {
			t.Fatal(errIncorrect)
		}
	} else {
		t.Fatal(errIncorrect)
	}

	// removing env variable
	node.SetEnv("a", nil)
	if _, exist := node.Env("a"); exist {
		t.Fatal(errIncorrect)
	}

	// set env variable
	v := "v"
	node.SetEnv("a", v)
	if nv, exist := node.Env("a"); exist {
		if nv != v {
			t.Fatal(errIncorrect)
		}
	} else {
		t.Fatal(errIncorrect)
	}

	// spawn process
	pid, err := node.Spawn(factory_t0, gen.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// register associated name with the process
	if err := node.RegisterName("test", pid); err != nil {
		t.Fatal(err)
	}
	// register associated name with the process one more time
	if err := node.RegisterName("test", pid); err != gen.ErrTaken {
		t.Fatal(err)
	}
	// register associated name with the non existing process
	unkpid := gen.PID{}
	if err := node.RegisterName("test", unkpid); err != gen.ErrProcessUnknown {
		t.Fatal(err)
	}

	// check process info
	pinfo, err := node.ProcessInfo(pid)
	if err != nil {
		t.Fatal(err)
	}
	if pinfo.PID != pid {
		t.Fatal(errIncorrect)
	}

	if pinfo.Name != "test" {
		t.Fatal(errIncorrect)
	}

	if pinfo.State != gen.ProcessStateSleep && pinfo.State != gen.ProcessStateRunning {
		t.Fatal(errIncorrect)
	}

	if pinfo.Parent != node.PID() {
		t.Fatal(errIncorrect)
	}
	if pinfo.Leader != node.PID() {
		t.Fatal(errIncorrect)
	}

	// check node info with 1 running process
	info, err = node.Info()
	if info.ProcessesTotal != 1 {
		t.Fatal(errIncorrect)
	}
	// ProcessesRunning should be 0 or 1 (depending on process state timing)
	if info.ProcessesRunning > 1 {
		t.Fatal(errIncorrect)
	}
	if info.RegisteredNames != 1 {
		t.Fatal(errIncorrect)
	}

	// unregister associated name
	if p, err := node.UnregisterName("test"); err != nil || p != pid {
		t.Fatal(errIncorrect)
	}

	// unregister unknown/unregistered name
	if _, err := node.UnregisterName("test"); err != gen.ErrNameUnknown {
		t.Fatal(errIncorrect)
	}

	// check process list
	if l, err := node.ProcessList(); err != nil {
		t.Fatal(err)
	} else {
		if reflect.DeepEqual(l, []gen.PID{pid}) == false {
			t.Fatal(errIncorrect)
		}
	}

	// check kill
	if err := node.Kill(pid); err != nil {
		t.Fatal(err)
	}
	if l, err := node.ProcessList(); err != nil || len(l) != 0 {
		// it can be in zombee or terminated state
		if inf, err := node.ProcessInfo(l[0]); err == nil {
			if inf.State != gen.ProcessStateZombee && inf.State != gen.ProcessStateTerminated {
				t.Fatal(errIncorrect)
			}
		} else {
			if _, err := node.ProcessInfo(pid); err != gen.ErrProcessUnknown {
				t.Fatal(errIncorrect)
			}
			info, err = node.Info()
			if info.ProcessesTotal != 0 {
				t.Fatal(errIncorrect)
			}
		}
	}

	// spawn a new process with registered name
	pid, err = node.SpawnRegister("test2", factory_t0, gen.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if info.RegisteredNames != 1 {
		t.Fatal(errIncorrect)
	}
	pinfo, err = node.ProcessInfo(pid)
	if pinfo.PID != pid {
		t.Fatal(errIncorrect)
	}
	if pinfo.Name != "test2" {
		t.Fatal(errIncorrect)
	}

	// check sending message
	if err := node.Send(pid, 1); err != nil {
		t.Fatal(err)
	}
	if err := node.Send(gen.Atom("test2"), 1); err != nil {
		t.Fatal(err)
	}
	if err := node.Send(gen.Atom("unknown"), 1); err != gen.ErrProcessUnknown {
		t.Fatal(err)
	}

	// unregister name that was associated with the process on spawning
	if p, err := node.UnregisterName("test2"); err != nil || p != pid {
		t.Fatal(errIncorrect)
	}
	if pinfo, err = node.ProcessInfo(pid); err != nil || pinfo.Name != "" {
		t.Fatal(errIncorrect)
	}

	// check exit signal (nil can't be used as a reason)
	if err := node.SendExit(pid, nil); err != gen.ErrIncorrect {
		t.Fatal(err)
	}
	// check sending exit signal. with no checking if this process has been terminated
	if err := node.SendExit(pid, gen.TerminateReasonNormal); err != nil {
		t.Fatal(errIncorrect)
	}

	if err := node.WaitWithTimeout(0); err != gen.ErrTimeout {
		t.Fatal(err)
	}
	node.Stop()
	if node.IsAlive() == true {
		t.Fatal("still alive")
	}
	if _, err := node.Info(); err != gen.ErrNodeTerminated {
		t.Fatal(errIncorrect)
	}
	if err := node.WaitWithTimeout(0); err != gen.ErrNodeTerminated {
		t.Fatal(err)
	}
}
