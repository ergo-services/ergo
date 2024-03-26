package distributed

import (
	"fmt"
	"reflect"
	"testing"

	"ergo.services/ergo"
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

type testServerRemoteSpawn struct {
	act.Actor
}

func factoryTestServerRemoteSpawn() gen.ProcessBehavior {
	return &testServerRemoteSpawn{}
}

func TestT1RemoteNodeSpawn(t *testing.T) {
	options1 := gen.NodeOptions{
		Env: map[gen.Env]any{
			"env1": 123,
			"env2": "example",
		},
	}
	options1.Network.Cookie = "123"
	options1.Log.DefaultLogger.Disable = true
	options1.Log.Level = gen.LogLevelTrace
	options1.Security.ExposeEnvRemoteSpawn = true
	node1, err := ergo.StartNode("distT1node1remotespawn@localhost", options1)
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()

	options2 := gen.NodeOptions{}
	options2.Network.Cookie = "123"
	options2.Log.Level = gen.LogLevelTrace
	options2.Log.DefaultLogger.Disable = true
	options2.Security.ExposeEnvInfo = true
	node2, err := ergo.StartNode("distT1node2remotespawn@localhost", options2)
	if err != nil {
		t.Fatal(err)
	}
	defer node2.Stop()
	node2.Network().EnableSpawn("tst", factoryTestServerRemoteSpawn)

	// make connection to the node2
	remote1, err := node1.Network().GetNode(node2.Name())
	if err != nil {
		t.Fatal(err)
	}

	// TODO spawn unknown name => gen.ErrUnknown

	pid, err := remote1.Spawn("tst", gen.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}
	pidInfo, err := node2.ProcessInfo(pid)
	if err != nil {
		t.Fatal(err)
	}
	if pidInfo.LogLevel != node1.Log().Level() {
		t.Fatalf("mismatch log level %d", pidInfo.LogLevel)
	}
	if pidInfo.Parent != node1.PID() {
		t.Fatal("mismatch parent PID")
	}
	if pidInfo.Leader != node1.PID() {
		t.Fatal("mismatch leader PID")
	}

	if reflect.DeepEqual(pidInfo.Env, node1.EnvList()) == false {
		t.Fatal("mismatch process env")
	}

	// TODO spawn one more process with the same registered name => gen.ErrTaken

	pid, err = remote1.SpawnRegister("regtst", "tst", gen.ProcessOptions{})
	pidInfo, err = node2.ProcessInfo(pid)
	if err != nil {
		t.Fatal(err)
	}
	if pidInfo.Name != "regtst" {
		t.Fatal("process has no registered name")
	}
	if pidInfo.Parent != node1.PID() {
		t.Fatal("mismatch parent PID")
	}
	if pidInfo.Leader != node1.PID() {
		t.Fatal("mismatch leader PID")
	}

}

func factory_t1remotespawn() gen.ProcessBehavior {
	return &t1remotespawn{}
}

type t1remotespawn struct {
	act.Actor

	testcase *testcase
}

func (t *t1remotespawn) HandleMessage(from gen.PID, message any) error {

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

//
// test methods
//

func (t *t1remotespawn) TestSpawn(input any) {
	defer func() {
		t.testcase = nil
	}()

	t.Log().SetLevel(gen.LogLevelWarning)
	t.SetEnv("penv", "pval")

	node2 := t.testcase.input.(gen.Node)

	if _, err := t.RemoteSpawn(node2.Name(), "unknown", gen.ProcessOptions{}); err != gen.ErrNameUnknown {
		t.testcase.err <- fmt.Errorf("expected gen.ErrNameUnknown, got %q", err)
		return

	}
	pid, err := t.RemoteSpawn(node2.Name(), "tst", gen.ProcessOptions{})
	if err != nil {
		t.testcase.err <- err
		return
	}

	pidInfo, err := node2.ProcessInfo(pid)
	if err != nil {
		t.testcase.err <- err
		return
	}
	if pidInfo.LogLevel != t.Log().Level() {
		t.testcase.err <- fmt.Errorf("mismatch log level %d", pidInfo.LogLevel)
		return
	}
	if pidInfo.Parent != t.PID() {
		t.testcase.err <- fmt.Errorf("mismatch parent PID")
		return
	}
	if pidInfo.Leader != t.Leader() {
		t.testcase.err <- fmt.Errorf("mismatch leader PID")
		return
	}

	if reflect.DeepEqual(pidInfo.Env, t.EnvList()) == false {
		t.testcase.err <- fmt.Errorf("mismatch process env")
		return
	}

	t.testcase.err <- nil
}

func (t *t1remotespawn) TestSpawnRegister(input any) {
	defer func() {
		t.testcase = nil
	}()

	t.Log().SetLevel(gen.LogLevelDebug)
	t.SetEnv("penv", "pval")

	node2 := t.testcase.input.(gen.Node)
	pid, err := t.RemoteSpawnRegister(node2.Name(), "tst", "regname", gen.ProcessOptions{})
	if err != nil {
		t.testcase.err <- err
		return
	}

	pidInfo, err := node2.ProcessInfo(pid)
	if err != nil {
		t.testcase.err <- err
		return
	}
	if pidInfo.Name != "regname" {
		t.testcase.err <- fmt.Errorf("mismatch registered process name")
		return
	}
	if pidInfo.LogLevel != t.Log().Level() {
		t.testcase.err <- fmt.Errorf("mismatch log level %d", pidInfo.LogLevel)
		return
	}
	if pidInfo.Parent != t.PID() {
		t.testcase.err <- fmt.Errorf("mismatch parent PID")
		return
	}
	if pidInfo.Leader != t.Leader() {
		t.testcase.err <- fmt.Errorf("mismatch leader PID")
		return
	}

	if reflect.DeepEqual(pidInfo.Env, t.EnvList()) == false {
		t.testcase.err <- fmt.Errorf("mismatch process env")
		return
	}
	if _, err := t.RemoteSpawnRegister(node2.Name(), "tst", "regname", gen.ProcessOptions{}); err != gen.ErrTaken {
		t.testcase.err <- fmt.Errorf("expected gen.ErrTaken, got %q", err)
		return
	}

	t.testcase.err <- nil
}

func TestT1ProcessRemoteSpawn(t *testing.T) {
	options1 := gen.NodeOptions{
		Env: map[gen.Env]any{
			"env1": 123,
			"env2": "example",
		},
	}
	options1.Network.Cookie = "12345"
	options1.Log.DefaultLogger.Disable = true
	options1.Log.Level = gen.LogLevelTrace
	options1.Security.ExposeEnvRemoteSpawn = true
	node1, err := ergo.StartNode("distT1node1processremotespawn@localhost", options1)
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()

	options2 := gen.NodeOptions{}
	options2.Network.Cookie = "12345"
	options2.Log.DefaultLogger.Disable = true
	options2.Security.ExposeEnvInfo = true
	node2, err := ergo.StartNode("distT1node2processremotespawn@localhost", options2)
	if err != nil {
		t.Fatal(err)
	}
	defer node2.Stop()
	node2.Network().EnableSpawn("tst", factoryTestServerRemoteSpawn)

	// make connection to the node2
	if _, err := node1.Network().GetNode(node2.Name()); err != nil {
		t.Fatal(err)
	}

	popt := gen.ProcessOptions{}
	pid, err := node1.Spawn(factory_t1remotespawn, popt)
	if err != nil {
		panic(err)
	}

	cases := []*testcase{
		{"TestSpawn", node2, nil, make(chan error)},
		{"TestSpawnRegister", node2, nil, make(chan error)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node1.Send(pid, tc)
			if err := tc.wait(1); err != nil {
				t.Fatal(err)
			}
		})
	}

}
