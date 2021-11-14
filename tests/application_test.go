package tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ergo-services/ergo"
	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/node"
)

type testApplication struct {
	gen.Application
}

func (a *testApplication) Load(args ...etf.Term) (gen.ApplicationSpec, error) {
	lifeSpan := args[0].(time.Duration)
	name := args[1].(string)
	nameGS := "testAppGS"
	if len(args) == 3 {
		nameGS = args[2].(string)
	}
	return gen.ApplicationSpec{
		Name:        name,
		Description: "My Test Applicatoin",
		Version:     "v.0.1",
		Environment: map[string]interface{}{
			"envName1": 123,
			"envName2": "Hello world",
		},
		Children: []gen.ApplicationChildSpec{
			{
				Child: &testAppGenServer{},
				Name:  nameGS,
			},
		},
		Lifespan: lifeSpan,
	}, nil
}

func (a *testApplication) Start(p gen.Process, args ...etf.Term) {
	//p.SetEnv("env123", 456)
}

// test gen.Server
type testAppGenServer struct {
	gen.Server
}

func (gs *testAppGenServer) Init(process *gen.ServerProcess, args ...etf.Term) error {
	process.SetEnv("env123", 456)
	return nil
}

func (gs *testAppGenServer) HandleCall(process *gen.ServerProcess, from gen.ServerFrom, message etf.Term) (etf.Term, gen.ServerStatus) {
	return nil, gen.ServerStatusStop
}

// testing application
func TestApplicationBasics(t *testing.T) {

	fmt.Printf("\n=== Test Application load/unload/start/stop\n")
	fmt.Printf("\nStarting node nodeTestAplication@localhost:")
	ctx := context.Background()
	mynode, err := ergo.StartNodeWithContext(ctx, "nodeTestApplication@localhost", "cookies", node.Options{})
	if err != nil {
		t.Fatal(err)
	} else {
		fmt.Println("OK")
	}

	app := &testApplication{}
	lifeSpan := 0 * time.Second

	//
	// case 1: loading/unloading app
	//
	fmt.Printf("... loading application: ")
	loaded, err := mynode.ApplicationLoad(app, lifeSpan, "testapp")
	if err != nil {
		t.Fatal(err)
	}
	if loaded != "testapp" {
		t.Fatal("can't load application")
	}

	la := mynode.LoadedApplications()

	// there is yet another default application - KernelApp. thats why it
	// should be equal 2.
	if len(la) != 2 {
		t.Fatal("total number of loaded application mismatch")
	}
	fmt.Println("OK")

	wa := mynode.WhichApplications()
	if len(wa) > 1 {
		t.Fatal("total number of running application mismatch")
	}

	fmt.Printf("... unloading application: ")
	if err := mynode.ApplicationUnload("testapp"); err != nil {
		t.Fatal(err)
	}
	la = mynode.LoadedApplications()
	if len(la) > 1 {
		t.Fatal("total number of loaded application mismatch")
	}
	fmt.Println("OK")

	//
	// case 2: start(and try to unload running app)/stop(normal) application
	//
	fmt.Printf("... starting application: ")
	if _, err := mynode.ApplicationLoad(app, lifeSpan, "testapp1", "testAppGS1"); err != nil {
		t.Fatal(err)
	}

	p, e := mynode.ApplicationStart("testapp1")
	if e != nil {
		t.Fatal(e)
	}
	fmt.Println("OK")

	fmt.Printf("... try to unload started application (shouldn't be able): ")
	if e := mynode.ApplicationUnload("testapp1"); e != node.ErrAppAlreadyStarted {
		t.Fatal(e)
	}
	fmt.Println("OK")

	fmt.Printf("... check total number of running applications (should be 2 including KernelApp): ")
	wa = mynode.WhichApplications()
	if n := len(wa); n != 2 {
		t.Fatal(n)
	}
	fmt.Println("OK")

	fmt.Printf("... check the name of running application (should be 'testapp1'): ")
	found := false
	for _, a := range wa {
		if a.Name == "testapp1" {
			found = true
			break
		}

	}
	if !found {
		t.Fatal("can't find testapp1 among the running applications")
	}
	fmt.Println("OK")

	// case 2.1: test env vars
	fmt.Printf("... application's environment variables: ")
	p.SetEnv("env123", 123)
	p.SetEnv("envStr", "123")

	gs := mynode.ProcessByName("testAppGS1")
	if gs == nil {
		t.Fatal("process testAppGS1 is not found by name")
	}
	env := gs.Env("env123")
	if env == nil {
		t.Fatal("incorrect environment variable: not found")
	}

	if i, ok := env.(int); !ok || i != 456 {
		t.Fatal("incorrect environment variable: value should be overrided by child process")
	}

	if envUnknown := gs.Env("unknown"); envUnknown != nil {
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
	list, _ := p.Children()
	if len(list) != 1 || list[0] != gs.Self() {
		t.Fatal("incorrect children list")
	}
	fmt.Println("OK")

	// case 2.3: get application info

	fmt.Printf("... getting application info: ")
	info, errInfo := mynode.ApplicationInfo("testapp1")
	if errInfo != nil {
		t.Fatal(errInfo)
	}
	if p.Self() != info.PID {
		t.Fatal("incorrect pid in application info")
	}
	fmt.Println("OK")

	fmt.Printf("... stopping application: ")
	if e := mynode.ApplicationStop("testapp1"); e != nil {
		t.Fatal(e)
	}
	fmt.Println("OK")

	if e := p.WaitWithTimeout(100 * time.Millisecond); e != nil {
		t.Fatal(e)
	}
	wa = mynode.WhichApplications()
	if len(wa) != 1 {
		fmt.Println("waa: ", wa)
		t.Fatal("total number of running application mismatch")
	}

	//
	// case 3: start/stop (brutal) application
	//
	fmt.Printf("... starting application for brutal kill: ")
	if _, err := mynode.ApplicationLoad(app, lifeSpan, "testappBrutal", "testAppGS2Brutal"); err != nil {
		t.Fatal(err)
	}
	p, e = mynode.ApplicationStart("testappBrutal")
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

	mynode.ApplicationUnload("testappBrutal")

	//
	// case 4: start with limited lifespan
	//
	fmt.Printf("... starting application with lifespan 150ms: ")
	lifeSpan = 150 * time.Millisecond
	if _, err := mynode.ApplicationLoad(app, lifeSpan, "testapp2", "testAppGS2"); err != nil {
		t.Fatal(err)
	}
	tStart := time.Now()
	p, e = mynode.ApplicationStart("testapp2")
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
	if mynode.IsProcessAlive(p) {
		t.Fatal("still alive")
	}

	if tLifeSpan < lifeSpan {
		t.Fatal("lifespan was shorter(", tLifeSpan, ") than ", lifeSpan)
	}

	fmt.Println("OK [ real lifespan:", tLifeSpan, "]")

	mynode.Stop()
}

func TestApplicationTypePermanent(t *testing.T) {
	fmt.Printf("\n=== Test Application type Permanent\n")
	fmt.Printf("\nStarting node nodeTestAplicationPermanent@localhost:")
	ctx := context.Background()
	mynode, _ := ergo.StartNodeWithContext(ctx, "nodeTestApplicationPermanent@localhost", "cookies", node.Options{})
	if mynode == nil {
		t.Fatal("can't start node")
	} else {
		fmt.Println("OK")
	}

	fmt.Printf("... starting application: ")
	app := &testApplication{}
	lifeSpan := time.Duration(0)
	if _, err := mynode.ApplicationLoad(app, lifeSpan, "testapp"); err != nil {
		t.Fatal(err)
	}

	p, e := mynode.ApplicationStartPermanent("testapp")
	if e != nil {
		t.Fatal(e)
	}
	fmt.Println("OK")

	gs := mynode.ProcessByName("testAppGS")
	if gs == nil {
		t.Fatal("process testAppGS is not found by name")
	}
	fmt.Printf("... stop child with 'abnormal' reason: ")
	gs.Exit("abnormal")
	if e := gs.WaitWithTimeout(100 * time.Millisecond); e != nil {
		t.Fatal("timeout on waiting child")
	}
	fmt.Println("OK")

	if e := p.WaitWithTimeout(1 * time.Second); e != nil {
		t.Fatal("timeout on waiting application stopping")
	}

	if e := mynode.WaitWithTimeout(1 * time.Second); e != nil {
		t.Fatal("node shouldn't be alive here")
	}

	if mynode.IsAlive() {
		t.Fatal("node shouldn't be alive here")
	}

}

func TestApplicationTypeTransient(t *testing.T) {
	fmt.Printf("\n=== Test Application type Transient\n")
	fmt.Printf("\nStarting node nodeTestAplicationTypeTransient@localhost:")
	ctx := context.Background()
	mynode, _ := ergo.StartNodeWithContext(ctx, "nodeTestApplicationTypeTransient@localhost", "cookies", node.Options{})
	if mynode == nil {
		t.Fatal("can't start node")
	} else {
		fmt.Println("OK")
	}

	app1 := &testApplication{}
	app2 := &testApplication{}
	lifeSpan := time.Duration(0)

	if _, err := mynode.ApplicationLoad(app1, lifeSpan, "testapp1", "testAppGS1"); err != nil {
		t.Fatal(err)
	}

	if _, err := mynode.ApplicationLoad(app2, lifeSpan, "testapp2", "testAppGS2"); err != nil {
		t.Fatal(err)
	}

	fmt.Printf("... starting application testapp1: ")
	p1, e1 := mynode.ApplicationStartTransient("testapp1")
	if e1 != nil {
		t.Fatal(e1)
	}
	fmt.Println("OK")

	fmt.Printf("... starting application testapp2: ")
	p2, e2 := mynode.ApplicationStartTransient("testapp2")
	if e2 != nil {
		t.Fatal(e2)
	}
	fmt.Println("OK")

	fmt.Printf("... stopping testAppGS1 with 'normal' reason (shouldn't affect testAppGS2): ")
	gs := mynode.ProcessByName("testAppGS1")
	if gs == nil {
		t.Fatal("process testAppGS1 is not found by name")
	}
	gs.Exit("normal")
	if e := gs.WaitWithTimeout(100 * time.Millisecond); e != nil {
		t.Fatal(e)
	}

	if e := p1.WaitWithTimeout(100 * time.Millisecond); e != node.ErrTimeout {
		t.Fatal("application testapp1 should be alive here")
	}

	fmt.Printf("... stopping application testapp1: ")
	p1.Kill()
	if e := p1.WaitWithTimeout(100 * time.Millisecond); e != nil {
		t.Fatal("application testapp1 shouldn't be alive here:", e)
	}
	fmt.Println("OK")

	p2.WaitWithTimeout(100 * time.Millisecond)
	if !p2.IsAlive() {
		t.Fatal("testAppGS2 should be alive here")
	}

	if !mynode.IsAlive() {
		t.Fatal("node should be alive here")
	}

	fmt.Println("OK")

	fmt.Printf("... starting application testapp1: ")
	p1, e1 = mynode.ApplicationStartTransient("testapp1")
	if e1 != nil {
		t.Fatal(e1)
	}
	fmt.Println("OK")

	fmt.Printf("... stopping testAppGS1 with 'abnormal' reason (node will shutdown): ")
	gs = mynode.ProcessByName("testAppGS1")
	gs.Exit("abnormal")

	if e := gs.WaitWithTimeout(100 * time.Millisecond); e != nil {
		t.Fatal(e)
	}

	if e := p1.WaitWithTimeout(100 * time.Millisecond); e != nil {
		t.Fatal("testapp1 shouldn't be alive here")
	}

	if e := p2.WaitWithTimeout(100 * time.Millisecond); e != nil {
		t.Fatal("testapp2 shouldn't be alive here")
	}

	if e := mynode.WaitWithTimeout(100 * time.Millisecond); e != nil {
		t.Fatal("node shouldn't be alive here")
	}
	fmt.Println("OK")
}

func TestApplicationTypeTemporary(t *testing.T) {
	fmt.Printf("\n=== Test Application type Temporary\n")
	fmt.Printf("\nStarting node nodeTestAplicationStop@localhost:")
	ctx := context.Background()
	mynode, _ := ergo.StartNodeWithContext(ctx, "nodeTestApplicationStop@localhost", "cookies", node.Options{})
	if mynode == nil {
		t.Fatal("can't start node")
	} else {
		fmt.Println("OK")
	}
	fmt.Printf("... starting application: ")
	app := &testApplication{}
	lifeSpan := time.Duration(0)
	if _, err := mynode.ApplicationLoad(app, lifeSpan, "testapp"); err != nil {
		t.Fatal(err)
	}

	_, e := mynode.ApplicationStart("testapp") // default start type is Temporary
	if e != nil {
		t.Fatal(e)
	}
	fmt.Println("OK")

	fmt.Printf("... stopping testAppGS with 'normal' reason: ")
	gs := mynode.ProcessByName("testAppGS")
	if gs == nil {
		t.Fatal("process testAppGS is not found by name")
	}
	gs.Exit("normal")
	if e := gs.WaitWithTimeout(100 * time.Millisecond); e != nil {
		t.Fatal(e)
	}

	if e := mynode.WaitWithTimeout(100 * time.Millisecond); e != node.ErrTimeout {
		t.Fatal("node should be alive here")
	}

	if !mynode.IsAlive() {
		t.Fatal("node should be alive here")
	}
	fmt.Println("OK")

	mynode.Stop()
}

func TestApplicationStop(t *testing.T) {
	fmt.Printf("\n=== Test Application stopping\n")
	fmt.Printf("\nStarting node nodeTestAplicationTypeTemporary@localhost:")
	ctx := context.Background()
	mynode, _ := ergo.StartNodeWithContext(ctx, "nodeTestApplicationTypeTemporary@localhost", "cookies", node.Options{})
	if mynode == nil {
		t.Fatal("can't start node")
	} else {
		fmt.Println("OK")
	}
	fmt.Printf("... starting applications testapp1, testapp2: ")
	lifeSpan := time.Duration(0)
	app := &testApplication{}
	if _, e := mynode.ApplicationLoad(app, lifeSpan, "testapp1", "testAppGS1"); e != nil {
		t.Fatal(e)
	}

	app1 := &testApplication{}
	if _, e := mynode.ApplicationLoad(app1, lifeSpan, "testapp2", "testAppGS2"); e != nil {
		t.Fatal(e)
	}

	_, e1 := mynode.ApplicationStartPermanent("testapp1")
	if e1 != nil {
		t.Fatal(e1)
	}
	p2, e2 := mynode.ApplicationStartPermanent("testapp2")
	if e2 != nil {
		t.Fatal(e2)
	}
	fmt.Println("OK")

	// case 1: stopping via node.ApplicatoinStop
	fmt.Printf("... stopping testapp1 via node.ApplicationStop (shouldn't affect testapp2):")
	if e := mynode.ApplicationStop("testapp1"); e != nil {
		t.Fatal("can't stop application via node.ApplicationStop", e)
	}

	if !p2.IsAlive() {
		t.Fatal("testapp2 should be alive here")
	}

	if !mynode.IsAlive() {
		t.Fatal("node should be alive here")
	}

	fmt.Println("OK")

	mynode.Stop()

}
