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
	ch chan interface{}
}

type testSupervisorOneForOneGenServer struct {
	GenServer
	process Process
	ch      chan interface{}
	order   int
}

type testMessageStarted struct {
	pid   etf.Pid
	name  string
	order int
}

type testMessageTerminated struct {
	name  string
	order int
}

func (tsv *testSupervisorOneForOneGenServer) Init(p Process, args ...interface{}) (state interface{}) {
	tsv.process = p
	tsv.ch = args[0].(chan interface{})
	tsv.order = args[1].(int)
	// fmt.Printf("\ntestSupervisorOneForOneGenServer ({%s, %s}): Init\n", tsv.process.name, tsv.process.Node.FullName)
	tsv.ch <- testMessageStarted{
		pid:   p.Self(),
		name:  p.Name(),
		order: tsv.order,
	}
	return nil
}
func (tsv *testSupervisorOneForOneGenServer) HandleCast(message etf.Term, state interface{}) (string, interface{}) {
	// fmt.Printf("testSupervisorOneForOneGenServer ({%s, %s}): HandleCast: %#v\n", tsv.process.name, tsv.process.Node.FullName, message)
	// tsv.v <- message
	return "stop", message
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
		name:  tsv.process.Name(),
		order: tsv.order,
	}
}

func TestSupervisorOneForOne(t *testing.T) {
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

	// test SupervisorChildRestartPermanent
	fmt.Printf("Starting supervisor 'testSupervisorPermanent' (%s)... ", SupervisorChildRestartPermanent)
	sv := &testSupervisorOneForOne{
		ch: make(chan interface{}, 10),
	}
	processSV, _ := node.Spawn("testSupervisorPermanent", ProcessOptions{}, sv, SupervisorChildRestartPermanent, sv.ch)
	children, err = waitNeventsSupervisorChildren(sv.ch, 3, children)
	if err != nil {
		t.Fatal(err)
	} else {
		fmt.Println("OK")
	}

	fmt.Printf("  stopping children with 'normal' reason and waiting for their starting ... ")
	for i := range children {
		processSV.Cast(children[i], "normal") // stopping child
	}

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
	sv = &testSupervisorOneForOne{
		ch: make(chan interface{}, 10),
	}
	processSV, _ = node.Spawn("testSupervisorTransient", ProcessOptions{}, sv, SupervisorChildRestartTransient, sv.ch)
	children = [3]etf.Pid{}
	children, err = waitNeventsSupervisorChildren(sv.ch, 3, children)
	if err != nil {
		t.Fatal(err)
	} else {
		fmt.Println("OK")
	}
	fmt.Printf("  stopping children with 'abnormal' reason and waiting for their starting ... ")
	for i := range children {
		processSV.Cast(children[i], "abnormal") // stopping child
	}

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

	fmt.Printf("  stopping children with 'normal' reason and they are haven't be restarted ... ")
	for i := range children {
		processSV.Cast(children[i], "normal") // stopping child
	}

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
	fmt.Printf("Stopping supervisor 'testSupervisorTransient' (%s)... ", SupervisorChildRestartTransient)
	processSV.Exit(processSV.Self(), "x")
	fmt.Println("OK")

	// ===================================================================================================
	// test SupervisorChildRestartTemporary

	fmt.Printf("Starting supervisor 'testSupervisorTemporary' (%s)... ", SupervisorChildRestartTemporary)
	sv = &testSupervisorOneForOne{
		ch: make(chan interface{}, 10),
	}
	processSV, _ = node.Spawn("testSupervisorTemporary", ProcessOptions{}, sv, SupervisorChildRestartTemporary, sv.ch)
	children = [3]etf.Pid{}
	children, err = waitNeventsSupervisorChildren(sv.ch, 3, children)
	if err != nil {
		t.Fatal(err)
	} else {
		fmt.Println("OK")
	}

	fmt.Printf("  stopping children with 'normal', 'abnornal','shutdown' reasons and they are haven't be restarted ... ")
	processSV.Cast(children[0], "normal")   // stopping child
	processSV.Cast(children[1], "abnormal") // stopping child
	processSV.Cast(children[2], "shutdown") // stopping child

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
	fmt.Printf("Stopping supervisor 'testSupervisorTemporary' (%s)... ", SupervisorChildRestartTemporary)
	processSV.Exit(processSV.Self(), "x")
	fmt.Println("OK")

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
				Args:    []interface{}{ch, 0},
			},
			SupervisorChildSpec{
				Name:    "testGS2",
				Child:   &testSupervisorOneForOneGenServer{},
				Restart: restart,
				Args:    []interface{}{ch, 1},
			},
			SupervisorChildSpec{
				Name:    "testGS3",
				Child:   &testSupervisorOneForOneGenServer{},
				Restart: restart,
				Args:    []interface{}{ch, 2},
			},
		},
		Strategy: SupervisorStrategy{
			Type:      SupervisorStrategyOneForOne,
			Intensity: 10,
			Period:    5,
		},
	}
}

func waitNeventsSupervisorChildren(ch chan interface{}, n int, children [3]etf.Pid) ([3]etf.Pid, error) {
	// n - is the number events have to be waiting for
	// start for-loop with 'n+1' to handle exceeded number of events we are waiting for
	for i := 0; i < n+1; i++ {
		select {
		case c := <-ch:
			switch child := c.(type) {
			case testMessageTerminated:
				children[child.order] = etf.Pid{} // set empty pid
			case testMessageStarted:
				children[child.order] = child.pid
			}

		case <-time.After(100 * time.Millisecond):
			if i == n {
				return children, nil
			}
			if i < n {
				return children, fmt.Errorf("expected %d events, but got %d. timeout", n, i)
			}

		}
	}
	return children, fmt.Errorf("expected %d events, but got %d. ", n, n+1)
}

func checkExpectedChildrenStatus(children, children1 [3]etf.Pid, statuses []string) bool {
	empty := etf.Pid{}
	for i := 0; i < len(statuses); i++ {
		switch statuses[i] {
		case "new":
			if children1[i] == empty { // is the epmty pid (child has been stopped)
				return false
			}
			if children[i] == children1[i] { // this value has to be different
				return false
			}

		case "epmty":
			if children1[i] != empty {
				return false
			}

		case "old":
			if children[i] == children1[i] {
				return false
			}
		}

	}
	return true
}
