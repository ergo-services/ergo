package ergonode

import (
	"fmt"
	"testing"
	"time"

	"github.com/halturin/ergonode/etf"
)

// This test is checking the cases below:
//
// initiation:
// - starting 2 nodes (node1, node2)
// - starting 5 GenServers
//	 * 3 on node1 - gs1, gs2, gs3
// 	 * 2 on node2 - gs4, gs5
//
// checking:
// - monitor/link processes by Pid (and monitor node test)
//  * node1.gs1 (by Pid) monitor -> gs2
//  * node1.gs2 (by Pid) link -> gs3
//  * call gs3.Stop (gs2 should receive 'exit' message)
//  * call gs2.Stop (gs1 should receive 'down' message)
//  ...  testing remote processes ...
//  * node1.gs1 (by Pid) monitor -> node2 gs4
//  * node1.gs1 (by Pid) link -> node2 gs5
//  * call gs5.Stop (node1.gs1 should receive 'exit')
//  * call gs4.Stop (node1.gs1 should receive 'down')
//  ... start gs2 on node1 and gs4,gs5 on node2 again
//  * node1.gs1 (by Pid) monitor -> node2 gs4
//  * node1.gs1 (by Pid) link -> node2.gs5
//  ... add monitor node ...
//  * node1.gs2 monitor node -> node2
//  ...
//  * call node2.Stop
//      node1.gs1 should receive 'down' message for gs4 with 'noconnection' as reason
//      node1.gs1 should receive 'exit' message for gs5 with 'noconnection' as reason
//      node1.gs2 should receive 'nodedown' message
//
// - monitor/link processes by Name
//  * node1.gs1 (by Name) monitor -> gs2
//  * node1.gs2 (by Name) link -> gs3
//  * call gs3.Stop (gs2 should receive 'exit' message)
//  * call gs2.Stop (gs1 should receive 'down' message)
//  ...  testing remote processes ...
//  * node1.gs1 (by Name) monitor -> node2 gs4
//  * node1.gs1 (by Name) link -> node2 gs5
//  * call gs5.Stop (node1.gs1 should receive 'exit')
//  * call gs4.Stop (node1.gs1 should receive 'down')
//  ... start gs3 on node1 and gs4,gs5 on node2 again
//  * node1.gs1 (by Name) monitor -> node2 gs4
//  * node1.gs1 (by Name) link -> node2.gs5
//  * node1.gs2 monitor node -> node2
//  * call node2.Stop
//      node1.gs1 should receive 'down' message for gs4 with 'noconnection' as reason
//      node1.gs1 should receive 'exit' message for gs5 with 'noconnection' as reason

type testMonitorGenServer struct {
	GenServer
	process Process
	v       chan interface{}
}

func (tgs *testMonitorGenServer) Init(p Process, args ...interface{}) (state interface{}) {
	tgs.v <- p.Self()
	tgs.process = p
	return nil
}
func (tgs *testMonitorGenServer) HandleCast(message etf.Term, state interface{}) (string, interface{}) {
	// fmt.Printf("testMonitorGenServer ({%s, %s}): HandleCast: %#v\n", tgs.process.name, tgs.process.Node.FullName, message)
	tgs.v <- message
	return "noreply", state
}
func (tgs *testMonitorGenServer) HandleCall(from etf.Tuple, message etf.Term, state interface{}) (string, etf.Term, interface{}) {
	// fmt.Printf("testMonitorGenServer ({%s, %s}): HandleCall: %#v, From: %#v\n", tgs.process.name, tgs.process.Node.FullName, message, from)
	return "reply", message, state
}
func (tgs *testMonitorGenServer) HandleInfo(message etf.Term, state interface{}) (string, interface{}) {
	// fmt.Printf("testMonitorGenServer ({%s, %s}): HandleInfo: %#v\n", tgs.process.name, tgs.process.Node.FullName, message)
	tgs.v <- message
	return "noreply", state
}
func (tgs *testMonitorGenServer) Terminate(reason string, state interface{}) {
	// fmt.Printf("\ntestMonitorGenServer ({%s, %s}): Terminate: %#v\n", tgs.process.name, tgs.process.Node.FullName, reason)
}

func TestMonitor(t *testing.T) {
	fmt.Printf("\n== Test Monitor/Link\n")
	fmt.Printf("Starting nodes: nodeM1@localhost, nodeM2@localhost: ")
	node1 := CreateNode("nodeM1@localhost", "cookies", NodeOptions{})
	node2 := CreateNode("nodeM2@localhost", "cookies", NodeOptions{})
	if node1 == nil || node2 == nil {
		t.Fatal("can't start nodes")
	} else {
		fmt.Println("OK")
	}

	gs1 := &testMonitorGenServer{
		v: make(chan interface{}, 2),
	}
	gs2 := &testMonitorGenServer{
		v: make(chan interface{}, 2),
	}
	gs3 := &testMonitorGenServer{
		v: make(chan interface{}, 2),
	}
	gs4 := &testMonitorGenServer{
		v: make(chan interface{}, 2),
	}
	gs5 := &testMonitorGenServer{
		v: make(chan interface{}, 2),
	}

	// starting gen servers
	fmt.Printf("    wait for start of gs1 on %#v: ", node1.FullName)
	node1gs1, _ := node1.Spawn("gs1", ProcessOptions{}, gs1, nil)
	waitForResultWithValue(t, gs1.v, node1gs1.Self())

	fmt.Printf("    wait for start of gs2 on %#v: ", node1.FullName)
	node1gs2, _ := node1.Spawn("gs2", ProcessOptions{}, gs2, nil)
	waitForResultWithValue(t, gs2.v, node1gs2.Self())

	fmt.Printf("    wait for start of gs3 on %#v: ", node1.FullName)
	node1gs3, _ := node1.Spawn("gs3", ProcessOptions{}, gs3, nil)
	waitForResultWithValue(t, gs3.v, node1gs3.Self())

	fmt.Printf("    wait for start of gs4 on %#v: ", node2.FullName)
	node2gs4, _ := node2.Spawn("gs4", ProcessOptions{}, gs4, nil)
	waitForResultWithValue(t, gs4.v, node2gs4.Self())

	fmt.Printf("    wait for start of gs5 on %#v: ", node2.FullName)
	node2gs5, _ := node2.Spawn("gs5", ProcessOptions{}, gs5, nil)
	waitForResultWithValue(t, gs5.v, node2gs5.Self())

	// start testing
	fmt.Println("Testing Monitor/Link process by Pid:")

	fmt.Println("... gs1 -local monitor-> gs2")
	ref := node1gs1.MonitorProcess(node1gs2.Self())
	fmt.Println("... stop gs2")
	node1gs2.Stop()
	fmt.Printf("    wait for 'DOWN' message of gs2 by gs1: ")
	waitFor := etf.Tuple{etf.Atom("DOWN"), ref, etf.Atom("process"), node1gs2.Self(), "shutdown"}
	waitForResultWithValue(t, gs1.v, waitFor)

	fmt.Println("... gs1 -local link-> gs3")
	node1gs1.Link(node1gs3.Self())
	fmt.Println("... stop gs3")
	node1gs3.Stop()
	fmt.Printf("    wait for 'EXIT' message of gs3 by gs1: ")
	waitFor = etf.Tuple{etf.Atom("EXIT"), node1gs3.Self(), "shutdown"}
	waitForResultWithValue(t, gs1.v, waitFor)

	fmt.Println("... restarting gs2:")

	fmt.Printf("    wait for start of gs2 on %#v: ", node1.FullName)
	node1gs2, _ = node1.Spawn("gs2", ProcessOptions{}, gs2, nil)
	waitForResultWithValue(t, gs2.v, node1gs2.Self())

	fmt.Println("... gs2 -local link-> gs1")
	node1gs2.Link(node1gs1.Self())
	fmt.Println("... stop gs2")
	node1gs2.Stop()
	fmt.Printf("    wait for 'EXIT' message of gs2 by gs1: ")
	waitFor = etf.Tuple{etf.Atom("EXIT"), node1gs2.Self(), "shutdown"}
	waitForResultWithValue(t, gs1.v, waitFor)

	fmt.Println("... gs1 -remote monitor-> gs4")
	ref = node1gs1.MonitorProcess(node2gs4.Self())
	// since MonitorProcess is asynchronous for remote calls
	// we have to put some sleep to make sure the stop calling
	// will handle after monitor
	time.Sleep(100 * time.Millisecond)
	fmt.Println("... stop gs4")
	node2gs4.Stop()
	fmt.Printf("    wait for 'DOWN' message of node2.gs4 by gs1: ")
	waitFor = etf.Tuple{etf.Atom("DOWN"), ref, etf.Atom("process"), node2gs4.Self(), "shutdown"}
	waitForResultWithValue(t, gs1.v, waitFor)

	fmt.Println("... gs1 -remote link-> gs5")
	node1gs1.Link(node2gs5.Self())
	time.Sleep(100 * time.Millisecond)
	fmt.Println("... stop gs5")
	node2gs5.Stop()
	fmt.Printf("    wait for 'EXIT' message of node2.gs5 by gs1: ")
	waitFor = etf.Tuple{etf.Atom("EXIT"), node2gs5.Self(), "shutdown"}
	waitForResultWithValue(t, gs1.v, waitFor)

	// starting gs2,gs3,gs4,gs5
	fmt.Println("... restarting gs2, gs3, gs4, gs5:")

	fmt.Printf("    wait for start of gs2 on %#v: ", node1.FullName)
	node1gs2, _ = node1.Spawn("gs2", ProcessOptions{}, gs2, nil)
	waitForResultWithValue(t, gs2.v, node1gs2.Self())

	fmt.Printf("    wait for start of gs3 on %#v: ", node1.FullName)
	node1gs3, _ = node1.Spawn("gs3", ProcessOptions{}, gs3, nil)
	waitForResultWithValue(t, gs3.v, node1gs3.Self())

	fmt.Printf("    wait for start of gs4 on %#v: ", node2.FullName)
	node2gs4, _ = node2.Spawn("gs4", ProcessOptions{}, gs4, nil)
	waitForResultWithValue(t, gs4.v, node2gs4.Self())

	fmt.Printf("    wait for start of gs5 on %#v: ", node2.FullName)
	node2gs5, _ = node2.Spawn("gs5", ProcessOptions{}, gs5, nil)
	waitForResultWithValue(t, gs5.v, node2gs5.Self())

	fmt.Println("... gs1 -remote monitor-> gs4")
	fmt.Println("... gs3 -remote link-> gs5")
	fmt.Println("... gs2 -monitor node-> node2")

	ref = node1gs1.MonitorProcess(node2gs4.Self())
	node1gs3.Link(node2gs5.Self())
	node1gs2.MonitorNode(node2.FullName)

	fmt.Println("... stop node2")
	node2.Stop()

	waitFor = etf.Tuple{etf.Atom("DOWN"), ref, etf.Atom("process"), node2gs4.Self(), "noconnection"}
	fmt.Printf("    wait for 'DOWN' with reason 'noconnection' by gs1: ")
	waitForResultWithValue(t, gs1.v, waitFor)
	waitFor = etf.Tuple{etf.Atom("EXIT"), node2gs5.Self(), "noconnection"}
	fmt.Printf("    wait for 'EXIT' with reason 'noconnection' by gs3: ")
	waitForResultWithValue(t, gs3.v, waitFor)
	waitFor = etf.Tuple{etf.Atom("nodedown"), node2.FullName}
	fmt.Printf("    wait for 'nodedown' by gs2: ")
	waitForResultWithValue(t, gs2.v, waitFor)

	fmt.Printf("Stopping nodes: %v, %v\n", node1.FullName, node2.FullName)
	node1.Stop()
}
