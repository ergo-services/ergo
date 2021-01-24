package ergo

// - Supervisor

//  - simple one for one (permanent)
//    start node1
//    start supevisor sv1 with genservers gs1,gs2,gs3
//     .... TODO: describe

import (
	"fmt"
	"testing"

	"github.com/halturin/ergo/etf"
)

type testSupervisorSimpleOneForOne struct {
	Supervisor
	ch chan interface{}
}

func TestSupervisorSimpleOneForOne(t *testing.T) {
	fmt.Printf("\n=== Test Supervisor - simple one for one\n")
	fmt.Printf("Starting node nodeSvSimpleOneForOne@localhost: ")
	node := CreateNode("nodeSvSimpleOneForOne@localhost", "cookies", NodeOptions{})
	if node == nil {
		t.Fatal("can't start node")
	} else {
		fmt.Println("OK")
	}

	testCases := []ChildrenTestCase{
		ChildrenTestCase{
			reason:   "abnormal",
			statuses: []string{"new", "new", "new", "new", "empty", "empty"},
			events:   10, // waiting for 6 terminates and 4 starts
		},
		ChildrenTestCase{
			reason:   "normal",
			statuses: []string{"new", "new", "empty", "empty", "empty", "empty"},
			events:   8, // waiting for 6 terminates and 2 starts
		},
		ChildrenTestCase{
			reason:   "shutdown",
			statuses: []string{"new", "new", "empty", "empty", "empty", "empty"},
			events:   8, // the same as 'normal' reason
		},
	}

	for c := range testCases {
		fmt.Printf("Starting supervisor 'testSupervisor' (reason: %s)... ", testCases[c].reason)
		sv := &testSupervisorSimpleOneForOne{
			ch: make(chan interface{}, 15),
		}
		processSV, _ := node.Spawn("testSupervisor", ProcessOptions{}, sv, sv.ch)
		children := make([]etf.Pid, 6)
		children1, err := waitNeventsSupervisorChildren(sv.ch, 0, children)
		if err != nil {
			t.Fatal(err)
		} else {
			// they should be equal after start
			statuses := []string{"empty", "empty", "empty", "empty", "empty", "empty"}
			if checkExpectedChildrenStatus(children, children1, statuses) {
				fmt.Println("OK")
				children = children1
			} else {
				e := fmt.Errorf("got something else except we expected (%v). old: %v new: %v", statuses, children, children1)
				t.Fatal(e)
			}
		}

		fmt.Printf("... starting 6 children  ... ")

		// start children
		for i := 0; i < 6; i = i + 2 {
			p, _ := sv.StartChild(processSV, fmt.Sprintf("testGS%d", i/2+1), sv.ch, i)
			children[i] = p
			// start twice
			p, _ = sv.StartChild(processSV, fmt.Sprintf("testGS%d", i/2+1), sv.ch, i+1)
			children[i+1] = p
		}
		if children1, err := waitNeventsSupervisorChildren(sv.ch, 6, children); err != nil {
			t.Fatal(err)
		} else {
			// they should be equal after start
			statuses := []string{"old", "old", "old", "old", "old", "old"}
			if checkExpectedChildrenStatus(children, children1, statuses) {
				fmt.Println("OK")
			} else {
				e := fmt.Errorf("got something else except we expected (%v). old: %v new: %v", statuses, children, children1)
				t.Fatal(e)
			}
		}

		// kill them all with reason = testCases[c].reason
		fmt.Printf("... stopping children with '%s' reason and waiting for restarting some of them ... ", testCases[c].reason)

		for k := range children {
			processSV.Cast(children[k], testCases[c].reason)
		}

		if children1, err := waitNeventsSupervisorChildren(sv.ch, testCases[c].events, children); err != nil {
			t.Fatal(err)
		} else {
			if checkExpectedChildrenStatus(children, children1, testCases[c].statuses) {
				fmt.Println("OK")
				children = children1
			} else {
				e := fmt.Errorf("got something else except we expected (%v). old: %v new: %v", testCases[c].statuses, children, children1)
				t.Fatal(e)
			}
		}

		fmt.Printf("Stopping supervisor 'testSupervisor' (reason: %s)... ", testCases[c].reason)
		processSV.Exit(processSV.Self(), testCases[c].reason)
		if children1, err := waitNeventsSupervisorChildren(sv.ch, testCases[c].events-len(children), children); err != nil {
			t.Fatal(err)
		} else {
			statuses := []string{"empty", "empty", "empty", "empty", "empty", "empty"}
			if checkExpectedChildrenStatus(children, children1, statuses) {
				fmt.Println("OK")
			} else {
				e := fmt.Errorf("got something else except we expected (%v). old: %v new: %v", statuses, children, children1)
				t.Fatal(e)
			}
		}

	}

}

func (ts *testSupervisorSimpleOneForOne) Init(args ...interface{}) SupervisorSpec {
	ch := args[0].(chan interface{})
	return SupervisorSpec{
		Children: []SupervisorChildSpec{
			SupervisorChildSpec{
				Name:    "testGS1",
				Child:   &testSupervisorGenServer{},
				Restart: SupervisorChildRestartPermanent,
				Args:    []interface{}{ch, 0},
			},
			SupervisorChildSpec{
				Name:    "testGS2",
				Child:   &testSupervisorGenServer{},
				Restart: SupervisorChildRestartTransient,
				Args:    []interface{}{ch, 1},
			},
			SupervisorChildSpec{
				Name:    "testGS3",
				Child:   &testSupervisorGenServer{},
				Restart: SupervisorChildRestartTemporary,
				Args:    []interface{}{ch, 2},
			},
		},
		Strategy: SupervisorStrategy{
			Type:      SupervisorStrategySimpleOneForOne,
			Intensity: 10,
			Period:    5,
		},
	}
}
