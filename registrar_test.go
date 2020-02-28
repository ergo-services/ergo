package ergonode

import (
	"fmt"
	"testing"
	"time"

	"github.com/halturin/ergonode/etf"
)

type TestRegistrarGenserver struct {
	GenServer
}

func (trg *TestRegistrarGenserver) Init(p *Process, args ...interface{}) (state interface{}) {
	return nil
}
func (trg *TestRegistrarGenserver) HandleCast(message etf.Term, state interface{}) (string, interface{}) {
	// fmt.Printf("TestRegistrarGenserver ({%s, %s}): HandleCast: %#v\n", trg.process.name, trg.process.Node.FullName, message)
	return "noreply", state
}
func (trg *TestRegistrarGenserver) HandleCall(from etf.Tuple, message etf.Term, state interface{}) (string, etf.Term, interface{}) {
	// fmt.Printf("TestRegistrarGenserver ({%s, %s}): HandleCall: %#v, From: %#v\n", trg.process.name, trg.process.Node.FullName, message, from)
	return "reply", message, state
}
func (trg *TestRegistrarGenserver) HandleInfo(message etf.Term, state interface{}) (string, interface{}) {
	// fmt.Printf("TestRegistrarGenserver ({%s, %s}): HandleInfo: %#v\n", trg.process.name, trg.process.Node.FullName, message)
	return "noreply", state
}
func (trg *TestRegistrarGenserver) Terminate(reason string, state interface{}) {
	// fmt.Printf("\nTestRegistrarGenserver ({%s, %s}): Terminate: %#v\n", trg.process.name, trg.process.Node.FullName, reason)
}

func TestRegistrar(t *testing.T) {
	fmt.Printf("\n=== Test Registrar\n")
	fmt.Printf("Starting nodes: nodeR1@localhost, nodeR2@localhost: ")
	node1 := CreateNode("nodeR1@localhost", "cookies", NodeOptions{})
	node2 := CreateNode("nodeR2@localhost", "cookies", NodeOptions{})
	if node1 == nil || node2 == nil {
		t.Fatal("can't start nodes")
	} else {
		fmt.Println("OK")
	}

	gs := &TestRegistrarGenserver{}
	fmt.Printf("Starting TestRegistrarGenserver and registering as 'gs1' on %s: ", node1.FullName)
	node1gs1, _ := node1.Spawn("gs1", ProcessOptions{}, gs, nil)
	if _, ok := node1.registrar.processes[node1gs1.Self()]; !ok {
		message := fmt.Sprintf("missing process %v on %s", node1gs1.Self(), node1.FullName)
		t.Fatal(message)
	}
	fmt.Println("OK")

	fmt.Printf("...registering name 'test' related to %v: ", node1gs1.Self())
	if e := node1.Register("test", node1gs1.Self()); e != nil {
		t.Fatal(e)
	} else {
		if e := node1.Register("test", node1gs1.Self()); e == nil {
			t.Fatal("registered duplicate name")
		}
	}
	fmt.Println("OK")
	fmt.Printf("...unregistering name 'test' related to %v: ", node1gs1.Self())
	node1.Unregister("test")
	if e := node1.Register("test", node1gs1.Self()); e != nil {
		t.Fatal(e)
	}
	fmt.Println("OK")

	fmt.Printf("Starting TestRegistrarGenserver and registering as 'gs2' on %s: ", node2.FullName)
	node2gs2, _ := node2.Spawn("gs2", ProcessOptions{}, gs, nil)
	if _, ok := node2.registrar.processes[node2gs2.Self()]; !ok {
		message := fmt.Sprintf("missing process %v on %s", node2gs2.Self(), node2.FullName)
		t.Fatal(message)
	}
	fmt.Println("OK")

	// tests below are about monitor/link, tbh :). let't it be here for a while

	ref := node1gs1.MonitorProcess(node2gs2.Self())
	// setting remote monitor is async.
	time.Sleep(100 * time.Millisecond)

	if pids, ok := node1.monitor.processes[node2gs2.Self()]; !ok {
		message := fmt.Sprintf("missing monitor %v on %s", node2gs2.Self(), node1.FullName)
		t.Fatal(message)
	} else {
		found := false
		for i := range pids {
			if pids[i].pid == node1gs1.Self() {
				found = true
			}
		}
		if !found {
			message := fmt.Sprintf("missing monitoring by %v on %s", node1gs1.Self(), node1.FullName)
			t.Fatal(message)
		}
	}

	node1gs1.DemonitorProcess(ref)
	time.Sleep(100 * time.Millisecond)

	if pids, ok := node1.monitor.processes[node2gs2.Self()]; ok {
		message := fmt.Sprintf("monitor %v on %s is still present", node2gs2.Self(), node1.FullName)
		t.Fatal(message)
	} else {
		found := false
		for i := range pids {
			if pids[i].pid == node1gs1.Self() {
				found = true
			}
		}
		if found {
			message := fmt.Sprintf("monitoring by %v on %s is still present", node1gs1.Self(), node1.FullName)
			t.Fatal(message)
		}
	}

	node1gs1.Link(node2gs2.Self())
	time.Sleep(100 * time.Millisecond)

	if pids, ok := node1.monitor.links[node2gs2.Self()]; !ok {
		message := fmt.Sprintf("missing link %v on %s", node2gs2.Self(), node1.FullName)
		t.Fatal(message)
	} else {
		found := false
		for i := range pids {
			if pids[i] == node1gs1.Self() {
				found = true
			}
		}
		if !found {
			message := fmt.Sprintf("missing link by %v on %s", node1gs1.Self(), node1.FullName)
			t.Fatal(message)
		}
	}
	if pids, ok := node1.monitor.links[node1gs1.Self()]; !ok {
		message := fmt.Sprintf("missing link %v on %s", node1gs1.Self(), node1.FullName)
		t.Fatal(message)
	} else {
		found := false
		for i := range pids {
			if pids[i] == node2gs2.Self() {
				found = true
			}
		}
		if !found {
			message := fmt.Sprintf("missing link by %v on %s", node2gs2.Self(), node1.FullName)
			t.Fatal(message)
		}
	}

	x := node1.registrar.createNewPID()
	xID := x.Id
	for i := xID; i < xID+10; i++ {
		x = node1.registrar.createNewPID()
	}
	if xID+10 != x.Id {
		t.Fatalf("malformed PID creation sequence")
	}

}
