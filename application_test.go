package ergo

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/halturin/ergo/etf"
)

type testApplication struct {
	Application
}

func (a *testApplication) Load(args ...interface{}) (ApplicationSpec, error) {
	lifeSpan := args[0].(time.Duration)
	name := args[1].(string)
	nameGS := "testAppGS"
	if len(args) == 3 {
		nameGS = args[2].(string)
	}
	return ApplicationSpec{
		Name:        name,
		Description: "My Test Applicatoin",
		Version:     "v.0.1",
		Environment: map[string]interface{}{
			"envName1": 123,
			"envName2": "Hello world",
		},
		Children: []ApplicationChildSpec{
			ApplicationChildSpec{
				Child: &testAppGenServer{},
				Name:  nameGS,
			},
		},
		Lifespan: lifeSpan,
	}, nil
}

func (a *testApplication) Start(p *Process, args ...interface{}) {
	//p.SetEnv("env123", 456)
}

// test GenServer
type testAppGenServer struct {
	GenServer
}

func (gs *testAppGenServer) Init(p *Process, args ...interface{}) interface{} {
	//fmt.Println("STARTING TEST GS IN APP")
	p.SetEnv("env123", 456)
	return nil
}

func (gs *testAppGenServer) HandleCast(message etf.Term, state interface{}) (string, interface{}) {
	return "noreply", state
}

func (gs *testAppGenServer) HandleCall(from etf.Tuple, message etf.Term, state interface{}) (string, etf.Term, interface{}) {
	return "stop", message, nil
}

func (gs *testAppGenServer) HandleInfo(message etf.Term, state interface{}) (string, interface{}) {
	return "noreply", state
}

func (gs *testAppGenServer) Terminate(reason string, state interface{}) {
	//fmt.Println("TERMINATING TEST GS IN APP with reason:", reason)
}

// testing application
func TestApplicationBasics(t *testing.T) {

	fmt.Printf("\n=== Test Application load/unload/start/stop\n")
	fmt.Printf("\nStarting node nodeTestAplication@localhost:")
	ctx := context.Background()
	node := CreateNodeWithContext(ctx, "nodeTestApplication@localhost", "cookies", NodeOptions{})
	if node == nil {
		t.Fatal("can't start node")
	} else {
		fmt.Println("OK")
	}

	app := &testApplication{}
	lifeSpan := 0 * time.Second

	//
	// case 1: loading/unloading app
	//
	fmt.Printf("... loading application: ")
	err := node.ApplicationLoad(app, lifeSpan, "testapp")
	if err != nil {
		t.Fatal(err)
	}

	la := node.LoadedApplications()

	if len(la) != 1 {
		t.Fatal("total number of loaded application mismatch")
	}
	if la[0].Name != "testapp" {
		t.Fatal("can't load application")
	}

	fmt.Println("OK")

	wa := node.WhichApplications()
	if len(wa) > 0 {
		t.Fatal("total number of running application mismatch")
	}

	fmt.Printf("... unloading application: ")
	if err := node.ApplicationUnload("testapp"); err != nil {
		t.Fatal(err)
	}
	la = node.LoadedApplications()
	if len(la) > 0 {
		t.Fatal("total number of loaded application mismatch")
	}
	fmt.Println("OK")

	//
	// case 2: start(and try to unload running app)/stop(normal) application
	//
	fmt.Printf("... starting application: ")
	if err := node.ApplicationLoad(app, lifeSpan, "testapp1", "testAppGS1"); err != nil {
		t.Fatal(err)
	}

	p, e := node.ApplicationStart("testapp1")
	if e != nil {
		t.Fatal(e)
	}
	fmt.Println("OK")

	fmt.Printf("... try to unload started application (shouldn't be able): ")
	if e := node.ApplicationUnload("testapp1"); e != ErrAppAlreadyStarted {
		t.Fatal(e)
	}
	fmt.Println("OK")

	fmt.Printf("... check total number of running applications (should be 1): ")
	wa = node.WhichApplications()
	if n := len(wa); n != 1 {
		t.Fatal(n)
	}
	fmt.Println("OK")

	fmt.Printf("... check the name of running application (should be 'testapp1'): ")
	if wa[0].Name != "testapp1" {
		t.Fatal(wa[0].Name)
	}
	fmt.Println("OK")

	// case 2.1: test env vars
	fmt.Printf("... application's environment variables: ")
	p.SetEnv("env123", 123)
	p.SetEnv("envStr", "123")

	gs := node.GetProcessByName("testAppGS1")
	env := gs.GetEnv("env123")
	if env == nil {
		t.Fatal("incorrect environment variable: not found")
	}

	if env.(int) != 456 {
		t.Fatal("incorrect environment variable: value should be overrided by child process")
	}

	if envUnknown := gs.GetEnv("unknown"); envUnknown != nil {
		t.Fatal("incorrect environment variable: undefined variable should have nil value")
	}

	envs := gs.ListEnv()
	if x, ok := envs["env123"]; !ok || x != 456 {
		t.Fatal("incorrect environment variable: list of variables has no env123 value or its wrong")
	}

	if x, ok := envs["envStr"]; !ok || x != "123" {
		t.Fatal("incorrect environment variable: list of variables has no envStr value or its wrong")
	}

	fmt.Println("OK")

	// case 2.2: get list of children' pid
	fmt.Printf("... application's children list: ")
	list := p.GetChildren()
	if len(list) != 1 || list[0] != gs.Self() {
		t.Fatal("incorrect children list")
	}
	fmt.Println("OK")

	// case 2.3: get application info

	fmt.Printf("... getting application info: ")
	info, errInfo := node.GetApplicationInfo("testapp1")
	if errInfo != nil {
		t.Fatal(errInfo)
	}
	if p.Self() != info.PID {
		t.Fatal("incorrect pid in application info")
	}
	fmt.Println("OK")

	fmt.Printf("... stopping application: ")
	if e := node.ApplicationStop("testapp1"); e != nil {
		t.Fatal(e)
	}
	fmt.Println("OK")

	if e := p.WaitWithTimeout(100 * time.Millisecond); e != nil {
		t.Fatal("timed out")
	}
	wa = node.WhichApplications()
	if len(wa) != 0 {
		fmt.Println("waa: ", wa)
		t.Fatal("total number of running application mismatch")
	}

	//
	// case 3: start/stop (brutal) application
	//
	fmt.Printf("... starting application for brutal kill: ")
	if err := node.ApplicationLoad(app, lifeSpan, "testappBrutal", "testAppGS2Brutal"); err != nil {
		t.Fatal(err)
	}
	p, e = node.ApplicationStart("testappBrutal")
	if e != nil {
		t.Fatal(e)
	}
	fmt.Println("OK")
	fmt.Printf("... kill application: ")
	p.Kill()
	if e := p.WaitWithTimeout(100 * time.Millisecond); e != nil {
		t.Fatal("timed out")
	}
	fmt.Println("OK")

	node.ApplicationUnload("testappBrutal")

	//
	// case 4: start with limited lifespan
	//
	fmt.Printf("... starting application with lifespan 150ms: ")
	lifeSpan = 150 * time.Millisecond
	if err := node.ApplicationLoad(app, lifeSpan, "testapp2", "testAppGS2"); err != nil {
		t.Fatal(err)
	}
	tStart := time.Now()
	p, e = node.ApplicationStart("testapp2")
	if e != nil {
		t.Fatal(e)
	}
	// due to small lifespantiming it is ok to get real lifespan longer almost twice
	if e := p.WaitWithTimeout(300 * time.Millisecond); e != nil {
		t.Fatal("application lifespan was longer than 150ms")
	}
	fmt.Println("OK")
	tLifeSpan := time.Since(tStart)

	fmt.Printf("... application should be self stopped in 150ms: ")
	if node.IsProcessAlive(p.Self()) {
		t.Fatal("still alive")
	}

	if tLifeSpan < lifeSpan {
		t.Fatal("lifespan was shorter(", tLifeSpan, ") than ", lifeSpan)
	}

	fmt.Println("OK [ real lifespan:", tLifeSpan, "]")

	node.Stop()
}

func TestApplicationTypePermanent(t *testing.T) {
	fmt.Printf("\n=== Test Application type Permanent\n")
	fmt.Printf("\nStarting node nodeTestAplicationPermanent@localhost:")
	ctx := context.Background()
	node := CreateNodeWithContext(ctx, "nodeTestApplicationPermanent@localhost", "cookies", NodeOptions{})
	if node == nil {
		t.Fatal("can't start node")
	} else {
		fmt.Println("OK")
	}

	fmt.Printf("... starting application: ")
	app := &testApplication{}
	lifeSpan := time.Duration(0)
	if err := node.ApplicationLoad(app, lifeSpan, "testapp"); err != nil {
		t.Fatal(err)
	}

	p, e := node.ApplicationStartPermanent("testapp")
	if e != nil {
		t.Fatal(e)
	}
	fmt.Println("OK")

	gs := node.GetProcessByName("testAppGS")

	fmt.Printf("... stop child with 'abnormal' reason: ")
	gs.Exit(p.Self(), "abnormal")
	if e := gs.WaitWithTimeout(100 * time.Millisecond); e != nil {
		t.Fatal("timeout on waiting child")
	}
	fmt.Println("OK")

	if e := p.WaitWithTimeout(1 * time.Second); e != nil {
		t.Fatal("timeout on waiting application stopping")
	}

	if e := node.WaitWithTimeout(1 * time.Second); e != nil {
		t.Fatal("node shouldn't be alive here")
	}

	if node.IsAlive() {
		t.Fatal("node shouldn't be alive here")
	}

}

func TestApplicationTypeTransient(t *testing.T) {
	fmt.Printf("\n=== Test Application type Transient\n")
	fmt.Printf("\nStarting node nodeTestAplicationTypeTransient@localhost:")
	ctx := context.Background()
	node := CreateNodeWithContext(ctx, "nodeTestApplicationTypeTransient@localhost", "cookies", NodeOptions{})
	if node == nil {
		t.Fatal("can't start node")
	} else {
		fmt.Println("OK")
	}

	app1 := &testApplication{}
	app2 := &testApplication{}
	lifeSpan := time.Duration(0)

	if err := node.ApplicationLoad(app1, lifeSpan, "testapp1", "testAppGS1"); err != nil {
		t.Fatal(err)
	}

	if err := node.ApplicationLoad(app2, lifeSpan, "testapp2", "testAppGS2"); err != nil {
		t.Fatal(err)
	}

	fmt.Printf("... starting application testapp1: ")
	p1, e1 := node.ApplicationStartTransient("testapp1")
	if e1 != nil {
		t.Fatal(e1)
	}
	fmt.Println("OK")

	fmt.Printf("... starting application testapp2: ")
	p2, e2 := node.ApplicationStartTransient("testapp2")
	if e2 != nil {
		t.Fatal(e2)
	}
	fmt.Println("OK")

	fmt.Printf("... stopping testAppGS1 with 'normal' reason (shouldn't affect testAppGS2): ")
	gs := node.GetProcessByName("testAppGS1")
	gs.Exit(gs.Self(), "normal")
	if e := gs.WaitWithTimeout(100 * time.Millisecond); e != nil {
		t.Fatal(e)
	}

	if e := p1.WaitWithTimeout(100 * time.Millisecond); e != ErrTimeout {
		t.Fatal("application testapp1 should be alive here")
	}

	p1.Kill()

	p2.WaitWithTimeout(100 * time.Millisecond)
	if !p2.IsAlive() {
		t.Fatal("testAppGS2 should be alive here")
	}

	if !node.IsAlive() {
		t.Fatal("node should be alive here")
	}

	fmt.Println("OK")

	fmt.Printf("... starting application testapp1: ")
	p1, e1 = node.ApplicationStartTransient("testapp1")
	if e1 != nil {
		t.Fatal(e1)
	}
	fmt.Println("OK")

	fmt.Printf("... stopping testAppGS1 with 'abnormal' reason (node will shotdown): ")
	gs = node.GetProcessByName("testAppGS1")
	gs.Exit(gs.Self(), "abnormal")

	if e := gs.WaitWithTimeout(100 * time.Millisecond); e != nil {
		t.Fatal(e)
	}

	if e := p1.WaitWithTimeout(100 * time.Millisecond); e != nil {
		t.Fatal("testapp1 shouldn't be alive here")
	}

	if e := p2.WaitWithTimeout(100 * time.Millisecond); e != nil {
		t.Fatal("testapp2 shouldn't be alive here")
	}

	if e := node.WaitWithTimeout(100 * time.Millisecond); e != nil {
		t.Fatal("node shouldn't be alive here")
	}
	fmt.Println("OK")
}

func TestApplicationTypeTemporary(t *testing.T) {
	fmt.Printf("\n=== Test Application type Temporary\n")
	fmt.Printf("\nStarting node nodeTestAplicationStop@localhost:")
	ctx := context.Background()
	node := CreateNodeWithContext(ctx, "nodeTestApplicationStop@localhost", "cookies", NodeOptions{})
	if node == nil {
		t.Fatal("can't start node")
	} else {
		fmt.Println("OK")
	}
	fmt.Printf("... starting application: ")
	app := &testApplication{}
	lifeSpan := time.Duration(0)
	if err := node.ApplicationLoad(app, lifeSpan, "testapp"); err != nil {
		t.Fatal(err)
	}

	p, e := node.ApplicationStart("testapp") // default start type is Temporary
	if e != nil {
		t.Fatal(e)
	}
	fmt.Println("OK")

	fmt.Printf("... stopping testAppGS with 'normal' reason: ")
	gs := node.GetProcessByName("testAppGS")
	gs.Exit(p.Self(), "normal")
	if e := gs.WaitWithTimeout(100 * time.Millisecond); e != nil {
		t.Fatal(e)
	}

	if e := node.WaitWithTimeout(100 * time.Millisecond); e != ErrTimeout {
		t.Fatal("node should be alive here")
	}

	if !node.IsAlive() {
		t.Fatal("node should be alive here")
	}
	fmt.Println("OK")

	node.Stop()
}

func TestApplicationStop(t *testing.T) {
	fmt.Printf("\n=== Test Application stopping\n")
	fmt.Printf("\nStarting node nodeTestAplicationTypeTemporary@localhost:")
	ctx := context.Background()
	node := CreateNodeWithContext(ctx, "nodeTestApplicationTypeTemporary@localhost", "cookies", NodeOptions{})
	if node == nil {
		t.Fatal("can't start node")
	} else {
		fmt.Println("OK")
	}
	fmt.Printf("... starting applications testapp1, testapp2: ")
	lifeSpan := time.Duration(0)
	app := &testApplication{}
	if e := node.ApplicationLoad(app, lifeSpan, "testapp1", "testAppGS1"); e != nil {
		t.Fatal(e)
	}

	app1 := &testApplication{}
	if e := node.ApplicationLoad(app1, lifeSpan, "testapp2", "testAppGS2"); e != nil {
		t.Fatal(e)
	}

	_, e1 := node.ApplicationStartPermanent("testapp1")
	if e1 != nil {
		t.Fatal(e1)
	}
	p2, e2 := node.ApplicationStartPermanent("testapp2")
	if e2 != nil {
		t.Fatal(e2)
	}
	fmt.Println("OK")

	// case 1: stopping via node.ApplicatoinStop
	fmt.Printf("... stopping testapp1 via node.ApplicationStop (shouldn't affect testapp2):")
	if e := node.ApplicationStop("testapp1"); e != nil {
		t.Fatal("can't stop application via node.ApplicationStop", e)
	}

	if !p2.IsAlive() {
		t.Fatal("testapp2 should be alive here")
	}

	if !node.IsAlive() {
		t.Fatal("node should be alive here")
	}

	fmt.Println("OK")

	node.Stop()

}
