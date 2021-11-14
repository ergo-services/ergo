package tests

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

	"github.com/ergo-services/ergo"
	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/node"
)

type testSupervisorOneForAll struct {
	gen.Supervisor
	ch chan interface{}
}

type ChildrenTestCase struct {
	reason   string
	statuses []string
	events   int
}

func TestSupervisorOneForAll(t *testing.T) {
	fmt.Printf("\n=== Test Supervisor - one for all\n")
	fmt.Printf("Starting node nodeSvOneForAll@localhost: ")
	node1, _ := ergo.StartNode("nodeSvOneForAll@localhost", "cookies", node.Options{})
	if node1 == nil {
		t.Fatal("can't start node")
	} else {
		fmt.Println("OK")
	}

	// ===================================================================================================
	// test SupervisorStrategyRestartPermanent
	fmt.Printf("Starting supervisor 'testSupervisorPermanent' (%s)... ", gen.SupervisorStrategyRestartPermanent)
	sv := &testSupervisorOneForAll{
		ch: make(chan interface{}, 10),
	}
	processSV, err := node1.Spawn("testSupervisorPermanent", gen.ProcessOptions{}, sv, gen.SupervisorStrategyRestartPermanent, sv.ch)
	if err != nil {
		t.Fatal(err)
	}
	children := make([]etf.Pid, 3)
	children, err = waitNeventsSupervisorChildren(sv.ch, 3, children)
	if err != nil {
		t.Fatal(err)
	} else {
		fmt.Println("OK")
	}

	// testing permanent
	testCases := []ChildrenTestCase{
		{
			reason:   "normal",
			statuses: []string{"new", "new", "new"},
			events:   6, // waiting for 3 terminating and 3 starting
		},
		{
			reason:   "abnormal",
			statuses: []string{"new", "new", "new"},
			events:   6,
		},
		{
			reason:   "shutdown",
			statuses: []string{"new", "new", "new"},
			events:   6,
		},
	}
	for i := range children {
		fmt.Printf("... stopping child %d with '%s' reason and waiting for restarting all of them ... ", i+1, testCases[i].reason)
		processSV.Send(children[i], testCases[i].reason) // stopping child

		if children1, err := waitNeventsSupervisorChildren(sv.ch, testCases[i].events, children); err != nil {
			t.Fatal(err)
		} else {
			if checkExpectedChildrenStatus(children, children1, testCases[i].statuses) {
				fmt.Println("OK")
				children = children1
			} else {
				e := fmt.Errorf("got something else except we expected (%v). old: %v new: %v", testCases[i].statuses, children, children1)
				t.Fatal(e)
			}
		}
	}

	fmt.Printf("Stopping supervisor 'testSupervisorPermanent' (%s)... ", gen.SupervisorStrategyRestartPermanent)
	processSV.Exit("x")
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
	// test SupervisorStrategyRestartTransient
	fmt.Printf("Starting supervisor 'testSupervisorTransient' (%s)... ", gen.SupervisorStrategyRestartTransient)
	sv = &testSupervisorOneForAll{
		ch: make(chan interface{}, 10),
	}
	processSV, _ = node1.Spawn("testSupervisorTransient", gen.ProcessOptions{}, sv, gen.SupervisorStrategyRestartTransient, sv.ch)
	children, err = waitNeventsSupervisorChildren(sv.ch, 3, children)
	if err != nil {
		t.Fatal(err)
	} else {
		fmt.Println("OK")
	}

	// testing transient
	testCases = []ChildrenTestCase{
		{
			reason:   "normal",
			statuses: []string{"empty", "new", "new"},
			events:   5, // waiting for 3 terminates and 2 starts
		},
		{
			reason:   "abnormal",
			statuses: []string{"empty", "new", "new"},
			events:   4, // waiting for 2 terminates and 2 starts
		},
		{
			reason:   "shutdown",
			statuses: []string{"empty", "new", "empty"},
			events:   3, // waiting for 2 terminates and 1 start
		},
	}
	for i := range children {
		fmt.Printf("... stopping child %d with '%s' reason and waiting for restarting all of them ... ", i+1, testCases[i].reason)
		processSV.Send(children[i], testCases[i].reason) // stopping child

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

	fmt.Printf("Stopping supervisor 'testSupervisorTransient' (%s)... ", gen.SupervisorStrategyRestartTransient)
	processSV.Exit("x")
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
	// test SupervisorStrategyRestartTemporary

	// testing temporary
	// A temporary child process is never restarted (even when the supervisor's
	// restart strategy is rest_for_one or one_for_all and a sibling's death
	// causes the temporary process to be terminated).
	testCases = []ChildrenTestCase{
		{
			reason:   "normal",
			statuses: []string{"empty", "empty", "empty"},
			events:   3, // waiting for 3 terminates
		},
		{
			reason:   "abnormal",
			statuses: []string{"empty", "empty", "empty"},
			events:   3, // waiting for 3 terminates
		},
		{
			reason:   "shutdown",
			statuses: []string{"empty", "empty", "empty"},
			events:   3, // waiting for 3 terminate
		},
	}

	for i := range testCases {
		fmt.Printf("Starting supervisor 'testSupervisorTemporary' (%s)... ", gen.SupervisorStrategyRestartTemporary)
		sv = &testSupervisorOneForAll{
			ch: make(chan interface{}, 10),
		}
		processSV, _ = node1.Spawn("testSupervisorTemporary", gen.ProcessOptions{}, sv, gen.SupervisorStrategyRestartTemporary, sv.ch)
		children, err = waitNeventsSupervisorChildren(sv.ch, 3, children)
		if err != nil {
			t.Fatal(err)
		} else {
			fmt.Println("OK")
		}

		fmt.Printf("... stopping child %d with '%s' reason and waiting for restarting all of them ... ", i+1, testCases[i].reason)
		processSV.Send(children[i], testCases[i].reason) // stopping child

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

		fmt.Printf("Stopping supervisor 'testSupervisorTemporary' (%s)... ", gen.SupervisorStrategyRestartTemporary)
		processSV.Exit("x")
		if children1, err := waitNeventsSupervisorChildren(sv.ch, 0, children); err != nil {
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

func (ts *testSupervisorOneForAll) Init(args ...etf.Term) (gen.SupervisorSpec, error) {
	restart := args[0].(string)
	ch := args[1].(chan interface{})
	return gen.SupervisorSpec{
		Children: []gen.SupervisorChildSpec{
			{
				Name:  "testGS1",
				Child: &testSupervisorGenServer{},
				Args:  []etf.Term{ch, 0},
			},
			{
				Name:  "testGS2",
				Child: &testSupervisorGenServer{},
				Args:  []etf.Term{ch, 1},
			},
			{
				Name:  "testGS3",
				Child: &testSupervisorGenServer{},
				Args:  []etf.Term{ch, 2},
			},
		},
		Strategy: gen.SupervisorStrategy{
			Type:      gen.SupervisorStrategyOneForAll,
			Intensity: 10,
			Period:    5,
			Restart:   restart,
		},
	}, nil
}
