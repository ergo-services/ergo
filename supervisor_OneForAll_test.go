package ergonode

// - Supervisor

// - one for all (permanent)
//    start node1
//    start supevisor sv1 with genservers gs1,gs2,gs3
//    gs1.stop(normal) (sv1 stoping gs1)
//                     (sv1 stoping gs2,gs3)
//                     (sv1 starting gs1,gs2,gs3)
//    gs2.stop(shutdown) (sv1 stoping gs2)
//                     (sv1 stoping gs1,gs3)
//                     (sv1 starting gs1,gs2,gs3)
//    gs3.stop(panic) (sv1 stoping gs3)
//                     (sv1 stoping gs1,gs2)
//                     (sv1 starting gs1,gs2,gs3)

// - one for all (transient)
//    start node1
//    start supevisor sv1 with genservers gs1,gs2,gs3
//    gs3.stop(panic) (sv1 stoping gs3)
//                     (sv1 stopping gs1, gs2)
//                     (sv1 starting gs1, gs2, gs3)

//    gs1.stop(normal) (sv1 stoping gs1)
//                     ( gs2, gs3 - still working)
//    gs2.stop(shutdown) (sv1 stoping gs2)
//                     (gs3 - still working)

// - one for all (temoporary)
//   start node1
//    start supevisor sv1 with genservers gs1,gs2,gs3

//    gs3.stop(panic) (sv1 stoping gs3)
//                     (sv1 stopping gs1, gs2)

//    start again gs1, gs2, gs3 via sv1
//    gs1.stop(normal) (sv1 stopping gs1)
//                     (gs2, gs3 are still running)
//    gs2.stop(shutdown) (sv1 stopping gs2)
//                     (gs3 are still running)

import (
	"fmt"
	"testing"

	"github.com/halturin/ergonode/etf"
	// "time"
	// "github.com/halturin/ergonode/etf"
)

type testSupervisorOneForAll struct {
	Supervisor
	ch chan interface{}
}

func TestSupervisorOneForAll(t *testing.T) {
	var children [3]etf.Pid
	var err error

	fmt.Printf("\n== Test Supervisor - one for all\n")
	fmt.Printf("Starting node nodeSvOneForAll@localhost: ")
	node := CreateNode("nodeSvOneForAll@localhost", "cookies", NodeOptions{})
	if node == nil {
		t.Fatal("can't start node")
	} else {
		fmt.Println("OK")
	}

	// ===================================================================================================
	// test SupervisorChildRestartPermanent
	fmt.Printf("Starting supervisor 'testSupervisorPermanent' (%s)... ", SupervisorChildRestartPermanent)
	sv := &testSupervisorOneForAll{
		ch: make(chan interface{}, 10),
	}
	processSV, _ := node.Spawn("testSupervisorPermanent", ProcessOptions{}, sv, SupervisorChildRestartPermanent, sv.ch)
	children, err = waitNeventsSupervisorChildren(sv.ch, 3, children)
	if err != nil {
		t.Fatal(err)
	} else {
		fmt.Println("OK")
	}

	fmt.Printf("... stopping child 1 with 'normal' reason and waiting for restarting all of them ... ")
	processSV.Cast(children[0], "normal") // stopping child

	if children1, err := waitNeventsSupervisorChildren(sv.ch, 6, children); err != nil { // waiting for 3 terminates and 3 starts
		t.Fatal(err)
	} else {
		statuses := []string{"new", "new", "new"}
		if checkExpectedChildrenStatus(children, children1, statuses) {
			fmt.Println("OK")
			children = children1
		} else {
			e := fmt.Errorf("got something else except we expected (%v). old: %v new: %v", statuses, children, children1)
			t.Fatal(e)
		}
	}

	fmt.Printf("... stopping child 2 with 'normal' reason and waiting for restarting all of them ... ")
	processSV.Cast(children[1], "normal") // stopping child

	if children1, err := waitNeventsSupervisorChildren(sv.ch, 6, children); err != nil { // waiting for 3 terminates and 3 starts
		t.Fatal(err)
	} else {
		statuses := []string{"new", "new", "new"}
		if checkExpectedChildrenStatus(children, children1, statuses) {
			fmt.Println("OK")
			children = children1
		} else {
			e := fmt.Errorf("got something else except we expected (%v). old: %v new: %v", statuses, children, children1)
			t.Fatal(e)
		}
	}

	fmt.Printf("... stopping child 3 with 'normal' reason and waiting for restarting all of them ... ")
	processSV.Cast(children[2], "normal") // stopping child

	if children1, err := waitNeventsSupervisorChildren(sv.ch, 6, children); err != nil { // waiting for 3 terminates and 3 starts
		t.Fatal(err)
	} else {
		statuses := []string{"new", "new", "new"}
		if checkExpectedChildrenStatus(children, children1, statuses) {
			fmt.Println("OK")
			children = children1
		} else {
			e := fmt.Errorf("got something else except we expected (%v). old: %v new: %v", statuses, children, children1)
			t.Fatal(e)
		}
	}

	fmt.Printf("Stopping supervisor 'testSupervisorPermanent' (%s)... ", SupervisorChildRestartPermanent)
	processSV.Exit(processSV.Self(), "x")
	if children1, err := waitNeventsSupervisorChildren(sv.ch, 3, children); err != nil {
		t.Fatal(err)
	} else {
		statuses := []string{"empty", "empty", "empty"}
		if checkExpectedChildrenStatus(children, children1, statuses) {
			fmt.Println("OK")
			children = children1
		} else {
			e := fmt.Errorf("got something else except we expected (%v). old: %v new: %v", statuses, children, children1)
			t.Fatal(e)
		}
	}

	// ===================================================================================================
	// test SupervisorChildRestartTransient
	fmt.Printf("Starting supervisor 'testSupervisorTransient' (%s)... ", SupervisorChildRestartTransient)
	sv = &testSupervisorOneForAll{
		ch: make(chan interface{}, 10),
	}
	processSV, _ = node.Spawn("testSupervisorTransient", ProcessOptions{}, sv, SupervisorChildRestartTransient, sv.ch)
	children, err = waitNeventsSupervisorChildren(sv.ch, 3, children)
	if err != nil {
		t.Fatal(err)
	} else {
		fmt.Println("OK")
	}

	fmt.Printf("... stopping child 1 with 'abnormal' reason and waiting for restarting all of them ... ")
	processSV.Cast(children[0], "abnormal") // stopping child

	if children1, err := waitNeventsSupervisorChildren(sv.ch, 6, children); err != nil { // waiting for 3 terminates and 3 starts
		t.Fatal(err)
	} else {
		statuses := []string{"new", "new", "new"}
		if checkExpectedChildrenStatus(children, children1, statuses) {
			fmt.Println("OK")
			children = children1
		} else {
			e := fmt.Errorf("got something else except we expected (%v). old: %v new: %v", statuses, children, children1)
			t.Fatal(e)
		}
	}

	fmt.Printf("... stopping child 1 with 'normal' reason and waiting for restarting children 2,3 ... ")
	processSV.Cast(children[0], "normal") // stopping child

	if children1, err := waitNeventsSupervisorChildren(sv.ch, 5, children); err != nil { // waiting for 3 terminates and 2 starts
		t.Fatal(err)
	} else {
		statuses := []string{"empty", "new", "new"}
		if checkExpectedChildrenStatus(children, children1, statuses) {
			fmt.Println("OK")
			children = children1
		} else {
			e := fmt.Errorf("got something else except we expected (%v). old: %v new: %v", statuses, children, children1)
			t.Fatal(e)
		}
	}

}

func (ts *testSupervisorOneForAll) Init(args ...interface{}) SupervisorSpec {
	restart := args[0].(string)
	ch := args[1].(chan interface{})
	return SupervisorSpec{
		Children: []SupervisorChildSpec{
			SupervisorChildSpec{
				Name:    "testGS1",
				Child:   &testSupervisorGenServer{},
				Restart: restart,
				Args:    []interface{}{ch, 0},
			},
			SupervisorChildSpec{
				Name:    "testGS2",
				Child:   &testSupervisorGenServer{},
				Restart: restart,
				Args:    []interface{}{ch, 1},
			},
			SupervisorChildSpec{
				Name:    "testGS3",
				Child:   &testSupervisorGenServer{},
				Restart: restart,
				Args:    []interface{}{ch, 2},
			},
		},
		Strategy: SupervisorStrategy{
			Type:      SupervisorStrategyOneForAll,
			Intensity: 10,
			Period:    5,
		},
	}
}
