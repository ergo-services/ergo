package tests

// - Supervisor

//  - simple one for one (permanent)
//    start node1
//    start supevisor sv1 with genservers gs1,gs2,gs3
//     .... TODO: describe

import (
	"fmt"
	"testing"

	"github.com/ergo-services/ergo"
	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/node"
)

type testSupervisorSimpleOneForOne struct {
	gen.Supervisor
	ch chan interface{}
}

func TestSupervisorSimpleOneForOne(t *testing.T) {
	fmt.Printf("\n=== Test Supervisor - simple one for one\n")
	fmt.Printf("Starting node nodeSvSimpleOneForOne@localhost: ")
	node1, _ := ergo.StartNode("nodeSvSimpleOneForOne@localhost", "cookies", node.Options{})
	if node1 == nil {
		t.Fatal("can't start node")
	} else {
		fmt.Println("OK")
	}
	sv := &testSupervisorSimpleOneForOne{
		ch: make(chan interface{}, 10),
	}

	// ===================================================================================================
	// test SupervisorStrategyRestartPermanent
	testCases := []ChildrenTestCase{
		ChildrenTestCase{
			reason:   "abnormal",
			statuses: []string{"new", "new", "new", "new", "new", "new"},
			events:   12, // waiting for 6 terminates and 6 restarts
		},
		ChildrenTestCase{
			reason:   "normal",
			statuses: []string{"new", "new", "new", "new", "new", "new"},
			events:   12, // waiting for 6 terminates and 6 restarts
		},
		ChildrenTestCase{
			reason:   "shutdown",
			statuses: []string{"new", "new", "new", "new", "new", "new"},
			events:   12, // waiting for 6 terminates and 6 restarts
		},
	}

	for c := range testCases {
		fmt.Printf("Starting supervisor 'testSupervisorPermanent' (%s)... ", gen.SupervisorStrategyRestartPermanent)
		processSV, err := node1.Spawn("testSupervisorPermanent", gen.ProcessOptions{}, sv, gen.SupervisorStrategyRestartPermanent)
		if err != nil {
			t.Fatal(err)
		}

		children := make([]etf.Pid, 6)
		// starting supervisor shouldn't cause start its children
		children1, err := waitNeventsSupervisorChildren(sv.ch, 0, children)
		if err != nil {
			t.Fatal(err)
		} else {
			// must be equal
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
			p, e := sv.StartChild(processSV, fmt.Sprintf("testGS%d", i/2+1), sv.ch, i)
			if e != nil {
				t.Fatal(e)
			}
			children[i] = p.Self()
			// start twice. must be able to start any number of child processes
			// as it doesn't register this process with the given name.
			p, e = sv.StartChild(processSV, fmt.Sprintf("testGS%d", i/2+1), sv.ch, i+1)
			if e != nil {
				t.Fatal(e)
			}
			children[i+1] = p.Self()
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
		fmt.Printf("... stopping children with '%s' reason and waiting for restarting all of them ... ", testCases[c].reason)

		for k := range children {
			processSV.Send(children[k], testCases[c].reason)
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
		processSV.Exit(testCases[c].reason)
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

	// ===================================================================================================
	// test SupervisorStrategyRestartTransient
	testCases = []ChildrenTestCase{
		ChildrenTestCase{
			reason:   "abnormal",
			statuses: []string{"new", "new", "new", "new", "new", "new"},
			events:   12, // waiting for 6 terminates and 6 restarts
		},
		ChildrenTestCase{
			reason:   "normal",
			statuses: []string{"empty", "empty", "empty", "empty", "empty", "empty"},
			events:   6, // waiting for 6 terminates
		},
		ChildrenTestCase{
			reason:   "shutdown",
			statuses: []string{"empty", "empty", "empty", "empty", "empty", "empty"},
			events:   6, // waiting for 6 terminates
		},
	}

	for c := range testCases {
		fmt.Printf("Starting supervisor 'testSupervisorTransient' (%s)... ", gen.SupervisorStrategyRestartTransient)
		processSV, err := node1.Spawn("testSupervisorTransient", gen.ProcessOptions{}, sv, gen.SupervisorStrategyRestartTransient)
		if err != nil {
			t.Fatal(err)
		}

		children := make([]etf.Pid, 6)
		// starting supervisor shouldn't cause start its children
		children1, err := waitNeventsSupervisorChildren(sv.ch, 0, children)
		if err != nil {
			t.Fatal(err)
		} else {
			// must be equal
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
			p, e := sv.StartChild(processSV, fmt.Sprintf("testGS%d", i/2+1), sv.ch, i)
			if e != nil {
				t.Fatal(e)
			}
			children[i] = p.Self()
			// start twice. must be able to start any number of child processes
			// as it doesn't register this process with the given name.
			p, e = sv.StartChild(processSV, fmt.Sprintf("testGS%d", i/2+1), sv.ch, i+1)
			if e != nil {
				t.Fatal(e)
			}
			children[i+1] = p.Self()
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
			processSV.Send(children[k], testCases[c].reason)
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
		processSV.Exit(testCases[c].reason)
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

	// ===================================================================================================
	// test SupervisorStrategyRestartTemporary
	testCases = []ChildrenTestCase{
		ChildrenTestCase{
			reason:   "abnormal",
			statuses: []string{"empty", "empty", "empty", "empty", "empty", "empty"},
			events:   6, // waiting for 6 terminates
		},
		ChildrenTestCase{
			reason:   "normal",
			statuses: []string{"empty", "empty", "empty", "empty", "empty", "empty"},
			events:   6, // waiting for 6 terminates
		},
		ChildrenTestCase{
			reason:   "shutdown",
			statuses: []string{"empty", "empty", "empty", "empty", "empty", "empty"},
			events:   6, // waiting for 6 terminates
		},
	}

	for c := range testCases {
		fmt.Printf("Starting supervisor 'testSupervisorTemporary' (%s)... ", gen.SupervisorStrategyRestartTemporary)
		processSV, err := node1.Spawn("testSupervisorTemporary", gen.ProcessOptions{}, sv, gen.SupervisorStrategyRestartTemporary)
		if err != nil {
			t.Fatal(err)
		}

		children := make([]etf.Pid, 6)
		// starting supervisor shouldn't cause start its children
		children1, err := waitNeventsSupervisorChildren(sv.ch, 0, children)
		if err != nil {
			t.Fatal(err)
		} else {
			// must be equal
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
			p, e := sv.StartChild(processSV, fmt.Sprintf("testGS%d", i/2+1), sv.ch, i)
			if e != nil {
				t.Fatal(e)
			}
			children[i] = p.Self()
			// start twice. must be able to start any number of child processes
			// as it doesn't register this process with the given name.
			p, e = sv.StartChild(processSV, fmt.Sprintf("testGS%d", i/2+1), sv.ch, i+1)
			if e != nil {
				t.Fatal(e)
			}
			children[i+1] = p.Self()
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
		fmt.Printf("... stopping children with '%s' reason and waiting for exiting all of them ... ", testCases[c].reason)

		for k := range children {
			processSV.Send(children[k], testCases[c].reason)
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
		processSV.Exit(testCases[c].reason)
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

func (ts *testSupervisorSimpleOneForOne) Init(args ...etf.Term) (gen.SupervisorSpec, error) {
	restart := args[0].(string)
	return gen.SupervisorSpec{
		Children: []gen.SupervisorChildSpec{
			gen.SupervisorChildSpec{
				Name:  "testGS1",
				Child: &testSupervisorGenServer{},
			},
			gen.SupervisorChildSpec{
				Name:  "testGS2",
				Child: &testSupervisorGenServer{},
			},
			gen.SupervisorChildSpec{
				Name:  "testGS3",
				Child: &testSupervisorGenServer{},
			},
		},
		Strategy: gen.SupervisorStrategy{
			Type:      gen.SupervisorStrategySimpleOneForOne,
			Intensity: 10,
			Period:    5,
			Restart:   restart,
		},
	}, nil
}
