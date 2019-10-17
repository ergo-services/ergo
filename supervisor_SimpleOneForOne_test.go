//+build supervisor

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

// - one for rest (permanent)
//     start node1
//    start supevisor sv1 with genservers gs1,gs2,gs3

//    gs2.stop(panic) (sv1 stopping gs2, gs3)
//               (sv1 starting gs2,gs3)
//    gs1.stop(panic) (sv1 stopping gs1, gs2, gs3)
//               (sv1 starting gs1, gs2,gs3)
//    gs3.stop(panic) (sv1 stopping gs3)
//               (sv1 starting gs3)

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

	fmt.Printf("\n== Test Supervisor - one for one\n")
	fmt.Printf("Starting node nodeSvOneForOne@localhost: ")
	node := CreateNode("nodeSvOneForOne@localhost", "cookies", NodeOptions{})
	if node == nil {
		t.Fatal("can't start node")
	} else {
		fmt.Println("OK")
	}

	// ===================================================================================================
	// test SupervisorChildRestartPermanent
	fmt.Printf("Starting supervisor 'testSupervisorPermanent' (%s)... ", SupervisorChildRestartPermanent)
	sv := &testSupervisorOneForOne{
		ch: make(chan interface{}, 10),
	}
	processSV, _ := node.Spawn("testSupervisorPermanent", ProcessOptions{}, sv, SupervisorChildRestartPermanent, sv.ch)
	children, err = waitNeventsSupervisorChildren(sv.ch, 0, [3]etf.Pid{})
	if err != nil {
		t.Fatal(err)
	} else {
		fmt.Println("OK")
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
