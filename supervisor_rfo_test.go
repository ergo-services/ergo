package ergonode

// - Supervisor

// - rest for one (permanent)
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
//
// - rest for one (transient)
//    start node1
//    start supevisor sv1 with genservers gs1,gs2,gs3
//    gs3.stop(panic) (sv1 stoping gs3)
//                     (sv1 stopping gs1, gs2)
//                     (sv1 starting gs1, gs2, gs3)

//    gs1.stop(normal) (sv1 stoping gs1)
//                     ( gs2, gs3 - still working)
//    gs2.stop(shutdown) (sv1 stoping gs2)
//                     (gs3 - still working)
//
// - rest for one (temoporary)
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

type testSupervisorRestForOne struct {
	Supervisor
	ch chan interface{}
}

func TestSupervisorRestForOne(t *testing.T) {
	var err error
	fmt.Printf("\n=== Test Supervisor - rest for one\n")
	fmt.Printf("Starting node nodeSvRestForOne@localhost: ")
	node := CreateNode("nodeSvRestForOne@localhost", "cookies", NodeOptions{})
	if node == nil {
		t.Fatal("can't start node")
	} else {
		fmt.Println("OK")
	}

	// ===================================================================================================
	// test SupervisorChildRestartPermanent
	fmt.Printf("Starting supervisor 'testSupervisorPermanent' (%s)... ", SupervisorChildRestartPermanent)
	sv := &testSupervisorRestForOne{
		ch: make(chan interface{}, 10),
	}
	processSV, _ := node.Spawn("testSupervisorPermanent", ProcessOptions{}, sv, SupervisorChildRestartPermanent, sv.ch)
	children := make([]etf.Pid, 3)

	children, err = waitNeventsSupervisorChildren(sv.ch, 3, children)
	if err != nil {
		t.Fatal(err)
	} else {
		fmt.Println("OK")
	}

	// testing permanent
	testCases := []ChildrenTestCase{
		ChildrenTestCase{
			reason:   "normal",
			statuses: []string{"new", "new", "new"},
			events:   6, // waiting for 3 terminates and 3 starts
		},
		ChildrenTestCase{
			reason:   "abnormal",
			statuses: []string{"old", "new", "new"},
			events:   4, // waiting for 2 terminates and 2 starts
		},
		ChildrenTestCase{
			reason:   "shutdown",
			statuses: []string{"old", "old", "new"},
			events:   2, // waiting for 1 terminates and 1 starts
		},
	}
	for i := range children {
		fmt.Printf("... stopping child %d with '%s' reason and waiting for restarting rest of them ... ", i+1, testCases[i].reason)
		processSV.Cast(children[i], testCases[i].reason) // stopping child

		if children1, err := waitNeventsSupervisorChildren(sv.ch, testCases[i].events, children); err != nil {
			t.Fatal(err)
		} else {
			if checkExpectedChildrenStatus(children[:], children1[:], testCases[i].statuses) {
				fmt.Println("OK")
				children = children1
			} else {
				e := fmt.Errorf("got something else except we expected (%v). old: %v new: %v", testCases[i].statuses, children, children1)
				t.Fatal(e)
			}
		}
	}

	fmt.Printf("Stopping supervisor 'testSupervisorPermanent' (%s)... ", SupervisorChildRestartPermanent)
	processSV.Exit(processSV.Self(), "x")
	if children1, err := waitNeventsSupervisorChildren(sv.ch, 3, children); err != nil {
		t.Fatal(err)
	} else {
		statuses := []string{"empty", "empty", "empty"}
		if checkExpectedChildrenStatus(children[:], children1[:], statuses) {
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
	sv = &testSupervisorRestForOne{
		ch: make(chan interface{}, 10),
	}
	processSV, _ = node.Spawn("testSupervisorTransient", ProcessOptions{}, sv, SupervisorChildRestartTransient, sv.ch)
	children = make([]etf.Pid, 3)

	children, err = waitNeventsSupervisorChildren(sv.ch, 3, children)
	if err != nil {
		t.Fatal(err)
	} else {
		fmt.Println("OK")
	}

	// testing transient
	testCases = []ChildrenTestCase{
		ChildrenTestCase{
			reason:   "abnormal",
			statuses: []string{"new", "new", "new"},
			events:   6, // waiting for 3 terminates and 3 starts
		},
		ChildrenTestCase{
			reason:   "normal",
			statuses: []string{"old", "empty", "new"},
			events:   3, // waiting for 2 terminates and 1 starts
		},
		ChildrenTestCase{
			reason:   "shutdown",
			statuses: []string{"old", "empty", "empty"},
			events:   1, // waiting for 1 terminates
		},
	}
	for i := range children {
		fmt.Printf("... stopping child %d with '%s' reason and waiting for restarting rest of them ... ", i+1, testCases[i].reason)
		processSV.Cast(children[i], testCases[i].reason) // stopping child

		if children1, err := waitNeventsSupervisorChildren(sv.ch, testCases[i].events, children); err != nil {
			t.Fatal(err)
		} else {
			if checkExpectedChildrenStatus(children[:], children1[:], testCases[i].statuses) {
				fmt.Println("OK")
				children = children1
			} else {
				e := fmt.Errorf("got something else except we expected (%v). old: %v new: %v", testCases[i].statuses, children, children1)
				t.Fatal(e)
			}
		}
	}

	fmt.Printf("Stopping supervisor 'testSupervisorTransient' (%s)... ", SupervisorChildRestartTransient)
	processSV.Exit(processSV.Self(), "x")
	if children1, err := waitNeventsSupervisorChildren(sv.ch, 1, children); err != nil {
		t.Fatal(err)
	} else {
		statuses := []string{"empty", "empty", "empty"}
		if checkExpectedChildrenStatus(children[:], children1[:], statuses) {
			fmt.Println("OK")
			children = children1
		} else {
			e := fmt.Errorf("got something else except we expected (%v). old: %v new: %v", statuses, children, children1)
			t.Fatal(e)
		}
	}

	// ===================================================================================================
	// test SupervisorChildRestartTemporary

	// testing temporary
	// A temporary child process is never restarted (even when the supervisor's
	// restart strategy is rest_for_one or one_for_all and a sibling's death
	// causes the temporary process to be terminated).
	testCases = []ChildrenTestCase{
		ChildrenTestCase{
			reason:   "normal",
			statuses: []string{"empty", "empty", "empty"},
			events:   3, // waiting for 3 terminates
		},
		ChildrenTestCase{
			reason:   "abnormal",
			statuses: []string{"old", "empty", "empty"},
			events:   2, // waiting for 2 terminates
		},
		ChildrenTestCase{
			reason:   "shutdown",
			statuses: []string{"old", "old", "empty"},
			events:   1, // waiting for 1 terminate
		},
	}

	for i := range testCases {
		fmt.Printf("Starting supervisor 'testSupervisorTemporary' (%s)... ", SupervisorChildRestartTemporary)
		sv = &testSupervisorRestForOne{
			ch: make(chan interface{}, 10),
		}
		processSV, _ = node.Spawn("testSupervisorTemporary", ProcessOptions{}, sv, SupervisorChildRestartTemporary, sv.ch)
		children = make([]etf.Pid, 3)

		children, err = waitNeventsSupervisorChildren(sv.ch, 3, children)
		if err != nil {
			t.Fatal(err)
		} else {
			fmt.Println("OK")
		}

		fmt.Printf("... stopping child %d with '%s' reason and without restarting  ... ", i+1, testCases[i].reason)
		processSV.Cast(children[i], testCases[i].reason) // stopping child

		if children1, err := waitNeventsSupervisorChildren(sv.ch, testCases[i].events, children); err != nil {
			t.Fatal(err)
		} else {
			if checkExpectedChildrenStatus(children[:], children1[:], testCases[i].statuses) {
				fmt.Println("OK")
				children = children1
			} else {
				e := fmt.Errorf("got something else except we expected (%v). old: %v new: %v", testCases[i].statuses, children, children1)
				t.Fatal(e)
			}
		}

		fmt.Printf("Stopping supervisor 'testSupervisorTemporary' (%s)... ", SupervisorChildRestartTemporary)
		processSV.Exit(processSV.Self(), "x")
		if children1, err := waitNeventsSupervisorChildren(sv.ch, 3-testCases[i].events, children); err != nil {
			t.Fatal(err)
		} else {
			statuses := []string{"empty", "empty", "empty"}
			if checkExpectedChildrenStatus(children[:], children1[:], statuses) {
				fmt.Println("OK")
				children = children1
			} else {
				e := fmt.Errorf("got something else except we expected (%v). old: %v new: %v", statuses, children, children1)
				t.Fatal(e)
			}
		}
	}

}

func (ts *testSupervisorRestForOne) Init(args ...interface{}) SupervisorSpec {
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
			Type:      SupervisorStrategyRestForOne,
			Intensity: 10,
			Period:    5,
		},
	}
}
