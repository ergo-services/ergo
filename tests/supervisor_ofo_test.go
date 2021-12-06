package tests

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

	"github.com/ergo-services/ergo"
	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/node"
)

type testSupervisorOneForOne struct {
	gen.Supervisor
	ch chan interface{}
}

type testSupervisorGenServer struct {
	gen.Server
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
	ch    chan interface{}
	order int
}

func (tsv *testSupervisorGenServer) Init(process *gen.ServerProcess, args ...etf.Term) error {
	st := &testSupervisorGenServerState{
		ch:    args[0].(chan interface{}),
		order: args[1].(int),
	}
	process.State = st

	st.ch <- testMessageStarted{
		pid:   process.Self(),
		name:  process.Name(),
		order: st.order,
	}

	return nil
}
func (tsv *testSupervisorGenServer) HandleInfo(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	// message has the stop reason

	return gen.ServerStatusStopWithReason(message.(string))
}
func (tsv *testSupervisorGenServer) HandleCall(process *gen.ServerProcess, from gen.ServerFrom, message etf.Term) (etf.Term, gen.ServerStatus) {
	return message, gen.ServerStatusOK
}

func (tsv *testSupervisorGenServer) HandleDirect(process *gen.ServerProcess, message interface{}) (interface{}, error) {
	switch m := message.(type) {
	case makeCall:
		return process.Call(m.to, m.message)
	case makeCast:
		return nil, process.Cast(m.to, m.message)
	}
	return nil, gen.ErrUnsupportedRequest
}

func (tsv *testSupervisorGenServer) Terminate(process *gen.ServerProcess, reason string) {
	st := process.State.(*testSupervisorGenServerState)
	st.ch <- testMessageTerminated{
		name:  process.Name(),
		pid:   process.Self(),
		order: st.order,
	}
}

func TestSupervisorOneForOne(t *testing.T) {
	var err error

	fmt.Printf("\n=== Test Supervisor - one for one\n")
	fmt.Printf("Starting node nodeSvOneForOne@localhost: ")
	node1, _ := ergo.StartNode("nodeSvOneForOne@localhost", "cookies", node.Options{})
	if node1 == nil {
		t.Fatal("can't start node")
	} else {
		fmt.Println("OK")
	}

	// ===================================================================================================
	// test SupervisorStrategyRestartPermanent
	fmt.Printf("Starting supervisor 'testSupervisorPermanent' (%s)... ", gen.SupervisorStrategyRestartPermanent)
	sv := &testSupervisorOneForOne{
		ch: make(chan interface{}, 10),
	}
	processSV, err := node1.Spawn("testSupervisorPermanent", gen.ProcessOptions{}, sv, gen.SupervisorStrategyRestartPermanent, sv.ch)
	children := make([]etf.Pid, 3)

	children, err = waitNeventsSupervisorChildren(sv.ch, 3, children)
	if err != nil {
		t.Fatal(err)
	} else {
		fmt.Println("OK")
	}

	fmt.Printf("... stopping children with 'normal' reason and waiting for their starting ... ")
	for i := range children {
		processSV.Send(children[i], "normal") // stopping child with reason "normal"
	}

	time.Sleep(1 * time.Second)
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
	sv = &testSupervisorOneForOne{
		ch: make(chan interface{}, 10),
	}
	processSV, _ = node1.Spawn("testSupervisorTransient", gen.ProcessOptions{}, sv, gen.SupervisorStrategyRestartTransient, sv.ch)
	children = make([]etf.Pid, 3)

	children, err = waitNeventsSupervisorChildren(sv.ch, 3, children)
	if err != nil {
		t.Fatal(err)
	} else {
		fmt.Println("OK")
	}
	fmt.Printf("... stopping children with 'abnormal' reason and waiting for their starting ... ")
	for i := range children {
		processSV.Send(children[i], "abnormal") // stopping child
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
		processSV.Send(children[i], "normal") // stopping child
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
	fmt.Printf("Stopping supervisor 'testSupervisorTransient' (%s)... ", gen.SupervisorStrategyRestartTransient)
	processSV.Exit("x")
	fmt.Println("OK")

	// ===================================================================================================
	// test SupervisorStrategyRestartTemporary

	fmt.Printf("Starting supervisor 'testSupervisorTemporary' (%s)... ", gen.SupervisorStrategyRestartTemporary)
	sv = &testSupervisorOneForOne{
		ch: make(chan interface{}, 10),
	}
	processSV, _ = node1.Spawn("testSupervisorTemporary", gen.ProcessOptions{}, sv, gen.SupervisorStrategyRestartTemporary, sv.ch)
	children = make([]etf.Pid, 3)

	children, err = waitNeventsSupervisorChildren(sv.ch, 3, children)
	if err != nil {
		t.Fatal(err)
	} else {
		fmt.Println("OK")
	}

	fmt.Printf("... stopping children with 'normal', 'abnornal','shutdown' reasons and they are haven't be restarted ... ")
	processSV.Send(children[0], "normal")   // stopping child
	processSV.Send(children[1], "abnormal") // stopping child
	processSV.Send(children[2], "shutdown") // stopping child

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
	fmt.Printf("Stopping supervisor 'testSupervisorTemporary' (%s)... ", gen.SupervisorStrategyRestartTemporary)
	processSV.Exit("x")
	fmt.Println("OK")

}

func (ts *testSupervisorOneForOne) Init(args ...etf.Term) (gen.SupervisorSpec, error) {
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
			Type:      gen.SupervisorStrategyOneForOne,
			Intensity: 10,
			Period:    5,
			Restart:   restart,
		},
	}, nil
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
				childrenNew[child.order] = etf.Pid{} // set empty pid
			case testMessageStarted:
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
