package ergonode

// - Supervisor

//  - simple one for one (permanent)
//    start node1
//    start supevisor sv1 with genservers gs1,gs2,gs3
//     .... TODO: describe

import (
	"fmt"
	"testing"

	"github.com/halturin/ergonode/etf"
)

type testSupervisorSimpleOneForOne struct {
	Supervisor
	ch chan interface{}
}

func TestSupervisorSimpleOneForOne(t *testing.T) {
	var children [3]etf.Pid
	var err error

	fmt.Printf("\n== Test Supervisor - simple one for one\n")
	fmt.Printf("Starting node nodeSvSimpleOneForOne@localhost: ")
	node := CreateNode("nodeSvSimpleOneForOne@localhost", "cookies", NodeOptions{})
	if node == nil {
		t.Fatal("can't start node")
	} else {
		fmt.Println("OK")
	}

	// ===================================================================================================
	// test SupervisorChildRestartPermanent
	fmt.Printf("Starting supervisor 'testSupervisorPermanent' (%s)... ", SupervisorChildRestartPermanent)
	sv := &testSupervisorSimpleOneForOne{
		ch: make(chan interface{}, 10),
	}
	processSV, _ := node.Spawn("testSupervisorPermanent", ProcessOptions{}, sv, SupervisorChildRestartPermanent, sv.ch)
	children, err = waitNeventsSupervisorChildren(sv.ch, 0, [3]etf.Pid{})
	if err != nil {
		t.Fatal(err)
	} else {
		statuses := []string{"empty", "empty", "empty"}
		if checkExpectedChildrenStatus(children, [3]etf.Pid{}, statuses) {
			fmt.Println("OK")
		} else {
			e := fmt.Errorf("got something else except we expected (%v). old: %v new: %v", statuses, children, [3]etf.Pid{})
			t.Fatal(e)
		}
	}

	sv.StartChild(*processSV, "testGS2")
	sv.StartChild(*processSV, "testGS2")
	// sv.StartChild(*processSV, "testGS2")

	fmt.Printf("Stopping supervisor 'testSupervisorPermanent' (%s)... ", SupervisorChildRestartPermanent)
	processSV.Exit(processSV.Self(), "x")
	if children1, err := waitNeventsSupervisorChildren(sv.ch, 0, children); err != nil {
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

}

func (ts *testSupervisorSimpleOneForOne) Init(args ...interface{}) SupervisorSpec {
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
			Type:      SupervisorStrategySimpleOneForOne,
			Intensity: 10,
			Period:    5,
		},
	}
}
