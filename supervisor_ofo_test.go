package ergo

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

	"github.com/halturin/ergo/etf"
)

type testSupervisorOneForOne struct {
	Supervisor
	ch chan interface{}
}

type testSupervisorGenServer struct {
	GenServer
	// process Process
	// ch      chan interface{}
	// order   int
}

type testMessageStarted struct {
	pid   etf.Pid
	name  string
	order int
}

type testMessageTerminated struct {
	name  string
	order int
	pid   etf.Pid
}

type testSupervisorGenServerState struct {
	process *Process
	ch      chan interface{}
	order   int
}

func (tsv *testSupervisorGenServer) Init(p *Process, args ...interface{}) (state interface{}) {
	st := testSupervisorGenServerState{
		process: p,
		ch:      args[0].(chan interface{}),
		order:   args[1].(int),
	}

	// fmt.Printf("\ntestSupervisorGenServer ({%s, %s}) %d: Init\n", st.process.name, st.process.Node.FullName, st.order)
	st.ch <- testMessageStarted{
		pid:   p.Self(),
		name:  p.Name(),
		order: st.order,
	}

	return &st
}
func (tsv *testSupervisorGenServer) HandleCast(message etf.Term, state interface{}) (string, interface{}) {
	// fmt.Printf("testSupervisorGenServer ({%s, %s}): HandleCast: %#v\n", tsv.process.name, tsv.process.Node.FullName, message)
	// tsv.v <- message
	return "stop", message
	// return "noreply", state
}
func (tsv *testSupervisorGenServer) HandleCall(from etf.Tuple, message etf.Term, state interface{}) (string, etf.Term, interface{}) {
	// fmt.Printf("testSupervisorGenServer ({%s, %s}): HandleCall: %#v, From: %#v\n", tsv.process.name, tsv.process.Node.FullName, message, from)
	return "reply", message, state
}
func (tsv *testSupervisorGenServer) HandleInfo(message etf.Term, state interface{}) (string, interface{}) {
	// fmt.Printf("testSupervisorGenServer ({%s, %s}): HandleInfo: %#v\n", tsv.process.name, tsv.process.Node.FullName, message)
	// tsv.v <- message
	return "noreply", state
}
func (tsv *testSupervisorGenServer) Terminate(reason string, state interface{}) {
	// fmt.Printf("\ntestSupervisorGenServer ({%s, %s}): Terminate: %#v\n", tsv.process.name, tsv.process.Node.FullName, reason)
	st := state.(*testSupervisorGenServerState)
	st.ch <- testMessageTerminated{
		name:  st.process.Name(),
		pid:   st.process.Self(),
		order: st.order,
	}
}

func TestSupervisorOneForOne(t *testing.T) {
	var err error

	fmt.Printf("\n=== Test Supervisor - one for one\n")
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
	children := make([]etf.Pid, 3)

	children, err = waitNeventsSupervisorChildren(sv.ch, 3, children)
	if err != nil {
		t.Fatal(err)
	} else {
		fmt.Println("OK")
	}

	fmt.Printf("... stopping children with 'normal' reason and waiting for their starting ... ")
	for i := range children {
		processSV.Cast(children[i], "normal") // stopping child
	}

	if children1, err := waitNeventsSupervisorChildren(sv.ch, 6, children); err != nil { // waiting for 3 terminates and 3 starts
		t.Fatal(err)
	} else {
		statuses := []string{"new", "new", "new"}
		if checkExpectedChildrenStatus(children[:], children1[:], statuses) {
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
	sv = &testSupervisorOneForOne{
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
	fmt.Printf("... stopping children with 'abnormal' reason and waiting for their starting ... ")
	for i := range children {
		processSV.Cast(children[i], "abnormal") // stopping child
	}

	if children1, err := waitNeventsSupervisorChildren(sv.ch, 6, children); err != nil { // waiting for 3 terminates and 3 starts
		t.Fatal(err)
	} else {
		statuses := []string{"new", "new", "new"}
		if checkExpectedChildrenStatus(children[:], children1[:], statuses) {
			fmt.Println("OK")
			children = children1
		} else {
			e := fmt.Errorf("got something else except we expected (%v). old: %v new: %v", statuses, children, children1)
			t.Fatal(e)
		}
	}

	fmt.Printf("... stopping children with 'normal' reason and they are haven't be restarted ... ")
	for i := range children {
		processSV.Cast(children[i], "normal") // stopping child
	}

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
	children = make([]etf.Pid, 3)

	children, err = waitNeventsSupervisorChildren(sv.ch, 3, children)
	if err != nil {
		t.Fatal(err)
	} else {
		fmt.Println("OK")
	}

	fmt.Printf("... stopping children with 'normal', 'abnornal','shutdown' reasons and they are haven't be restarted ... ")
	processSV.Cast(children[0], "normal")   // stopping child
	processSV.Cast(children[1], "abnormal") // stopping child
	processSV.Cast(children[2], "shutdown") // stopping child

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
			Type:      SupervisorStrategyOneForOne,
			Intensity: 10,
			Period:    5,
		},
	}
}

func waitNeventsSupervisorChildren(ch chan interface{}, n int, children []etf.Pid) ([]etf.Pid, error) {
	// n - number of events that have to be awaited
	// start for-loop with 'n+1' to handle exceeded number of events
	childrenNew := make([]etf.Pid, len(children))
	copy(childrenNew, children)
	for i := 0; i < n+1; i++ {
		select {
		case c := <-ch:
			switch child := c.(type) {
			case testMessageTerminated:
				// fmt.Println("TERM", child)
				childrenNew[child.order] = etf.Pid{} // set empty pid
			case testMessageStarted:
				// fmt.Println("START", child)
				childrenNew[child.order] = child.pid
			}

		case <-time.After(200 * time.Millisecond):
			if i == n {
				return childrenNew, nil
			}
			if i < n {
				return childrenNew, fmt.Errorf("expected %d events, but got %d. TIMEOUT", n, i)
			}

		}
	}
	return childrenNew, fmt.Errorf("expected %d events, but got %d. ", n, n+1)
}

func checkExpectedChildrenStatus(children, children1 []etf.Pid, statuses []string) bool {
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
				return true
			}
		}

	}
	return true
}
