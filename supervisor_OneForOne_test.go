package ergonode

// - Supervisor

//  - one for one (permanent)
//    start node1
//    start supevisor sv1 with genservers gs1,gs2,gs3
//    gs1.stop(normal) (sv1 restarting gs1)
//    gs2.stop(shutdown) (sv1 restarting gs2)
//    gs3.stop(panic) (sv1 restarting gs3)

//  - one for one (transient)
//    start node1
//    start supevisor sv1 with genservers gs1,gs2,gs3
//    gs1.stop(normal) (sv1 wont restart gs1)
//    gs2.stop(shutdown) (sv1 wont restart gs2)
//    gs3.stop(panic) (sv1 restarting gs3 only)

//  - one for one (temporary)
//    start node1
//    start supevisor sv1 with genservers gs1,gs2,gs3
//    gs1.stop(normal) (sv1 wont restart gs1)
//    gs2.stop(shutdown) (sv1 wont restart gs2)
//    gs3.stop(panic) (sv1 wont gs3 only)

import (
	"fmt"
	"testing"
	"time"

	"github.com/halturin/ergonode/etf"
)

type testSupervisorOneForOne struct {
	Supervisor
}

type testSupervisorOneForOneGenServer struct {
	GenServer
	process Process
	ch      chan interface{}
}

type testMessageStarted struct {
	pid  etf.Pid
	name string
}

type testMessageTerminated struct {
	name string
}

func (tsv *testSupervisorOneForOneGenServer) Init(p Process, args ...interface{}) (state interface{}) {
	tsv.process = p
	tsv.ch = args[0].(chan interface{})
	// fmt.Printf("\ntestSupervisorOneForOneGenServer ({%s, %s}): Init\n", tsv.process.name, tsv.process.Node.FullName)
	tsv.ch <- testMessageStarted{
		pid:  p.Self(),
		name: p.Name(),
	}
	return nil
}
func (tsv *testSupervisorOneForOneGenServer) HandleCast(message etf.Term, state interface{}) (string, interface{}) {
	fmt.Printf("testSupervisorOneForOneGenServer ({%s, %s}): HandleCast: %#v\n", tsv.process.name, tsv.process.Node.FullName, message)
	// tsv.v <- message
	return "stop", "test"
	// return "noreply", state
}
func (tsv *testSupervisorOneForOneGenServer) HandleCall(from etf.Tuple, message etf.Term, state interface{}) (string, etf.Term, interface{}) {
	// fmt.Printf("testSupervisorOneForOneGenServer ({%s, %s}): HandleCall: %#v, From: %#v\n", tsv.process.name, tsv.process.Node.FullName, message, from)
	return "reply", message, state
}
func (tsv *testSupervisorOneForOneGenServer) HandleInfo(message etf.Term, state interface{}) (string, interface{}) {
	// fmt.Printf("testSupervisorOneForOneGenServer ({%s, %s}): HandleInfo: %#v\n", tsv.process.name, tsv.process.Node.FullName, message)
	// tsv.v <- message
	return "noreply", state
}
func (tsv *testSupervisorOneForOneGenServer) Terminate(reason string, state interface{}) {
	// fmt.Printf("\ntestSupervisorOneForOneGenServer ({%s, %s}): Terminate: %#v\n", tsv.process.name, tsv.process.Node.FullName, reason)
	tsv.ch <- testMessageTerminated{
		name: tsv.process.Name(),
	}
}

func TestSupervisorOneForOne(t *testing.T) {
	fmt.Printf("\n== Test Supervisor - one for one\n")
	fmt.Printf("Starting node nodeSvOneForOne@localhost: ")
	node := CreateNode("nodeSvOneForOne@localhost", "cookies", NodeOptions{})
	if node == nil {
		t.Fatal("can't start node")
	} else {
		fmt.Println("OK")
	}
	sv := &testSupervisorOneForOne{}
	ch := make(chan interface{}, 3)
	children := make(map[string]etf.Pid)

	// test SupervisorChildRestartPermanent
	fmt.Printf("Starting supervisor 'testSupervisorPermanent' (%s)... ", SupervisorChildRestartPermanent)
	processSV, _ := node.Spawn("testSupervisorPermanent", ProcessOptions{}, sv, SupervisorChildRestartPermanent, ch)
	if err := waitForStartSupervisor(ch, children); err != nil {
		t.Fatal(err)
	} else {
		fmt.Println("OK")
	}

	// do some tests

	fmt.Printf("Stopping supervisor 'testSupervisorPermanent' (%s)... ", SupervisorChildRestartPermanent)
	processSV.Exit(processSV.Self(), "x")
	if err := waitForStopSupervisor(ch, children); err != nil {
		t.Fatal(err)
	} else {
		fmt.Println("OK")
	}

	// test SupervisorChildRestartTransient
	fmt.Printf("Starting supervisor 'testSupervisorTransient' (%s)... ", SupervisorChildRestartTransient)
	processSV, _ = node.Spawn("testSupervisorTransient", ProcessOptions{}, sv, SupervisorChildRestartTransient, ch)
	if err := waitForStartSupervisor(ch, children); err != nil {
		t.Fatal(err)
	} else {
		fmt.Println("OK")
	}

	// do some tests

	fmt.Printf("Stopping supervisor 'testSupervisorTransient' (%s)... ", SupervisorChildRestartTransient)
	processSV.Exit(processSV.Self(), "x")
	if err := waitForStopSupervisor(ch, children); err != nil {
		t.Fatal(err)
	} else {
		fmt.Println("OK")
	}

	// test SupervisorChildRestartTemporary
	fmt.Printf("Starting supervisor 'testSupervisorTemporary' (%s)... ", SupervisorChildRestartTemporary)
	processSV, _ = node.Spawn("testSupervisorTemporary", ProcessOptions{}, sv, SupervisorChildRestartTemporary, ch)
	if err := waitForStartSupervisor(ch, children); err != nil {
		t.Fatal(err)
	} else {
		fmt.Println("OK")
	}

	// do some tests

	fmt.Printf("Stopping supervisor 'testSupervisorTemporary' (%s)... ", SupervisorChildRestartTemporary)
	processSV.Exit(processSV.Self(), "x")
	if err := waitForStopSupervisor(ch, children); err != nil {
		t.Fatal(err)
	} else {
		fmt.Println("OK")
	}

}

func (ts *testSupervisorOneForOne) Init(args ...interface{}) SupervisorSpec {
	restart := args[0].(string)
	ch := args[1].(chan interface{})
	return SupervisorSpec{
		Children: []SupervisorChildSpec{
			SupervisorChildSpec{
				Name:    "testGS1",
				Child:   &testSupervisorOneForOneGenServer{},
				Restart: restart,
				Args:    []interface{}{ch},
			},
			SupervisorChildSpec{
				Name:    "testGS2",
				Child:   &testSupervisorOneForOneGenServer{},
				Restart: restart,
				Args:    []interface{}{ch},
			},
			SupervisorChildSpec{
				Name:    "testGS3",
				Child:   &testSupervisorOneForOneGenServer{},
				Restart: restart,
				Args:    []interface{}{ch},
			},
		},
		Strategy: SupervisorStrategy{
			Type:      SupervisorStrategyOneForOne,
			Intensity: 10,
			Period:    5,
		},
	}
}

func waitForStopSupervisor(ch chan interface{}, children map[string]etf.Pid) error {
	for i := 0; i < 3; i++ {
		select {
		case c := <-ch:
			delete(children, c.(testMessageTerminated).name)
		case <-time.After(100 * time.Millisecond):
			return fmt.Errorf("stopping child timeout")
		}
	}
	if len(children) != 0 {
		fmt.Errorf("there are children that are not stopped: %v", children)
	}
	return nil
}

func waitForStartSupervisor(ch chan interface{}, children map[string]etf.Pid) error {
	for i := 0; i < 3; i++ {
		select {
		case c := <-ch:
			children[c.(testMessageStarted).name] = c.(testMessageStarted).pid
		case <-time.After(100 * time.Millisecond):
			return fmt.Errorf("starting child timeout")
		}
	}
	if len(children) != 0 {
		fmt.Errorf("there are children that are not stopped: %v", children)
	}
	return nil
}
