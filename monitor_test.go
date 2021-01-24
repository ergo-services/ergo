package ergo

import (
	"fmt"
	"testing"

	"github.com/halturin/ergo/etf"
)

type testMonitorGenServer struct {
	GenServer
	process *Process
	v       chan interface{}
}

func (tgs *testMonitorGenServer) Init(p *Process, args ...interface{}) (state interface{}) {
	tgs.v <- p.Self()
	tgs.process = p
	return nil
}
func (tgs *testMonitorGenServer) HandleCast(message etf.Term, state interface{}) (string, interface{}) {
	tgs.v <- message
	return "noreply", state
}
func (tgs *testMonitorGenServer) HandleCall(from etf.Tuple, message etf.Term, state interface{}) (string, etf.Term, interface{}) {
	return "reply", message, state
}
func (tgs *testMonitorGenServer) HandleInfo(message etf.Term, state interface{}) (string, interface{}) {
	tgs.v <- message
	return "noreply", state
}
func (tgs *testMonitorGenServer) Terminate(reason string, state interface{}) {
}

/*
	Test cases for Local-Local
	Monitor
		by Pid		- doesnt_exist, terminate, demonitor
		by Name		- doesnt_exist, terminate, demonitor
		by Tuple	- doesnt_exist, terminate, demonitor
*/
func TestMonitorLocalLocal(t *testing.T) {
	fmt.Printf("\n=== Test Monitor Local-Local\n")
	fmt.Printf("Starting node: nodeM1LocalLocal@localhost: ")
	node1 := CreateNode("nodeM1LocalLocal@localhost", "cookies", NodeOptions{})
	if node1 == nil {
		t.Fatal("can't start node")
	} else {
		fmt.Println("OK")
	}
	gs1 := &testMonitorGenServer{
		v: make(chan interface{}, 2),
	}
	gs2 := &testMonitorGenServer{
		v: make(chan interface{}, 2),
	}
	// starting gen servers

	fmt.Printf("    wait for start of gs1 on %#v: ", node1.FullName)
	node1gs1, _ := node1.Spawn("gs1", ProcessOptions{}, gs1, nil)
	waitForResultWithValue(t, gs1.v, node1gs1.Self())

	fmt.Printf("    wait for start of gs2 on %#v: ", node1.FullName)
	node1gs2, _ := node1.Spawn("gs2", ProcessOptions{}, gs2, nil)
	waitForResultWithValue(t, gs2.v, node1gs2.Self())

	// by Pid
	fmt.Printf("... by Pid Local-Local: gs1 -> gs2. demonitor: ")
	ref := node1gs1.MonitorProcess(node1gs2.Self())
	if err := chechCleanProcessRef(node1, ref); err == nil {
		t.Fatal("monitor reference has been lost")
	}
	node1gs1.DemonitorProcess(ref)
	if err := chechCleanProcessRef(node1, ref); err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	fmt.Printf("... by Pid Local-Local: gs1 -> gs2. terminate: ")
	ref = node1gs1.MonitorProcess(node1gs2.Self())
	node1gs2.Exit(etf.Pid{}, "normal")
	result := etf.Tuple{etf.Atom("DOWN"), ref, etf.Atom("process"), node1gs2.Self(), etf.Atom("normal")}
	waitForResultWithValue(t, gs1.v, result)

	if err := chechCleanProcessRef(node1, ref); err != nil {
		t.Fatal(err)
	}

	fmt.Print("... by Pid Local-Local: gs1 -> unknownPid: ")
	ref = node1gs1.MonitorProcess(node1gs2.Self())
	result = etf.Tuple{etf.Atom("DOWN"), ref, etf.Atom("process"), node1gs2.Self(), etf.Atom("noproc")}
	waitForResultWithValue(t, gs1.v, result)

	fmt.Printf("    wait for start of gs2 on %#v: ", node1.FullName)
	node1gs2, _ = node1.Spawn("gs2", ProcessOptions{}, gs2, nil)
	waitForResultWithValue(t, gs2.v, node1gs2.Self())
	// by Name
	fmt.Printf("... by Name Local-Local: gs1 -> gs2. demonitor: ")
	ref = node1gs1.MonitorProcess("gs2")
	if err := chechCleanProcessRef(node1, ref); err == nil {
		t.Fatal("monitor reference has been lost")
	}
	node1gs1.DemonitorProcess(ref)
	if err := chechCleanProcessRef(node1, ref); err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	fmt.Printf("... by Name Local-Local: gs1 -> gs2. terminate: ")
	ref = node1gs1.MonitorProcess("gs2")
	node1gs2.Exit(etf.Pid{}, "normal")
	procName := etf.Tuple{"gs2", node1.FullName}
	result = etf.Tuple{etf.Atom("DOWN"), ref, etf.Atom("process"), procName, etf.Atom("normal")}
	waitForResultWithValue(t, gs1.v, result)
	if err := chechCleanProcessRef(node1, ref); err != nil {
		t.Fatal(err)
	}
	fmt.Print("... by Name Local-Local: gs1 -> unknownPid: ")
	ref = node1gs1.MonitorProcess("asdfasdf")
	procName = etf.Tuple{"asdfasdf", node1.FullName}
	result = etf.Tuple{etf.Atom("DOWN"), ref, etf.Atom("process"), procName, etf.Atom("noproc")}
	waitForResultWithValue(t, gs1.v, result)

	fmt.Printf("    wait for start of gs2 on %#v: ", node1.FullName)
	node1gs2, _ = node1.Spawn("gs2", ProcessOptions{}, gs2, nil)
	waitForResultWithValue(t, gs2.v, node1gs2.Self())
	// by Tuple {Name, Node}
	fmt.Printf("... by Tuple Local-Local: gs1 -> gs2. demonitor: ")
	tuple := etf.Tuple{"gs2", node1.FullName}
	ref = node1gs1.MonitorProcess(tuple)
	if err := chechCleanProcessRef(node1, ref); err == nil {
		t.Fatal("monitor reference has been lost")
	}
	node1gs1.DemonitorProcess(ref)
	if err := chechCleanProcessRef(node1, ref); err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")
	fmt.Printf("... by Tuple Local-Local: gs1 -> gs2. terminate: ")
	ref = node1gs1.MonitorProcess(tuple)
	node1gs2.Exit(etf.Pid{}, "normal")
	result = etf.Tuple{etf.Atom("DOWN"), ref, etf.Atom("process"), tuple, etf.Atom("normal")}
	waitForResultWithValue(t, gs1.v, result)
	if err := chechCleanProcessRef(node1, ref); err != nil {
		t.Fatal(err)
	}
	fmt.Print("... by Tuple Local-Local: gs1 -> unknownPid: ")
	tupleUnknownProc := etf.Tuple{"gs2222", node1.FullName}
	ref = node1gs1.MonitorProcess(tupleUnknownProc)
	result = etf.Tuple{etf.Atom("DOWN"), ref, etf.Atom("process"), tupleUnknownProc, etf.Atom("noproc")}
	waitForResultWithValue(t, gs1.v, result)

	node1.Stop()
}

/*
	Test cases for Local-Remote
	Monitor
		by Pid		- doesnt_exist, terminate, demonitor, on_node_down, node_unknown
		by Tuple	- doesnt_exist, terminate, demonitor, on_node_down, node_unknown
	Link
		by Pid		- doesnt_exist, terminate, unlink, node_down, node_unknown
*/
func TestMonitorLocalRemoteByPid(t *testing.T) {
	fmt.Printf("\n=== Test Monitor Local-Remote by Pid\n")
	fmt.Printf("Starting nodes: nodeM1LocalRemoteByPid@localhost, nodeM2LocalRemoteByPid@localhost: ")
	node1 := CreateNode("nodeM1LocalRemoteByPid@localhost", "cookies", NodeOptions{})
	node2 := CreateNode("nodeM2LocalRemoteByPid@localhost", "cookies", NodeOptions{})
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

	// starting gen servers
	fmt.Printf("    wait for start of gs1 on %#v: ", node1.FullName)
	node1gs1, _ := node1.Spawn("gs1", ProcessOptions{}, gs1, nil)
	waitForResultWithValue(t, gs1.v, node1gs1.Self())

	fmt.Printf("    wait for start of gs2 on %#v: ", node2.FullName)
	node2gs2, _ := node2.Spawn("gs2", ProcessOptions{}, gs2, nil)
	waitForResultWithValue(t, gs2.v, node2gs2.Self())

	// by Pid
	fmt.Printf("... by Pid Local-Remote: gs1 -> gs2. demonitor: ")
	ref := node1gs1.MonitorProcess(node2gs2.Self())
	// wait a bit for the 'DOWN' message if something went wrong
	if err := waitForTimeout(gs1.v); err != nil {
		t.Fatal(err)
	}
	if err := chechCleanProcessRef(node1, ref); err == nil {
		t.Fatal("monitor reference has been lost on node 1")
	}
	if err := chechCleanProcessRef(node2, ref); err == nil {
		t.Fatal("monitor reference has been lost on node 2")
	}
	if found := node1gs1.DemonitorProcess(ref); found == false {
		t.Fatal("lost monitoring reference on node1")
	}
	// Demonitoring is the async message with nothing as a feedback.
	// use waitForTimeout just as a short timer
	if err := waitForTimeout(gs1.v); err != nil {
		t.Fatal(err)
	}
	if err := chechCleanProcessRef(node1, ref); err != nil {
		t.Fatal(err)
	}
	if err := chechCleanProcessRef(node2, ref); err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	fmt.Printf("... by Pid Local-Remote: gs1 -> gs2. terminate: ")
	ref = node1gs1.MonitorProcess(node2gs2.Self())
	// wait a bit for the 'DOWN' message if something went wrong
	if err := waitForTimeout(gs1.v); err != nil {
		t.Fatal(err)
	}
	node2gs2.Exit(etf.Pid{}, "normal")
	result := etf.Tuple{etf.Atom("DOWN"), ref, etf.Atom("process"), node2gs2.Self(), etf.Atom("normal")}
	waitForResultWithValue(t, gs1.v, result)

	if err := chechCleanProcessRef(node1, ref); err != nil {
		t.Fatal(err)
	}
	if err := chechCleanProcessRef(node2, ref); err != nil {
		t.Fatal(err)
	}

	fmt.Printf("... by Pid Local-Remote: gs1 -> unknownPid: ")
	ref = node1gs1.MonitorProcess(node2gs2.Self())
	result = etf.Tuple{etf.Atom("DOWN"), ref, etf.Atom("process"), node2gs2.Self(), etf.Atom("noproc")}
	waitForResultWithValue(t, gs1.v, result)
	if err := chechCleanProcessRef(node1, ref); err != nil {
		t.Fatal(err)
	}

	fmt.Printf("    wait for start of gs2 on %#v: ", node2.FullName)
	node2gs2, _ = node2.Spawn("gs2", ProcessOptions{}, gs2, nil)
	waitForResultWithValue(t, gs2.v, node2gs2.Self())

	fmt.Printf("... by Pid Local-Remote: gs1 -> gs2. onNodeDown: ")
	ref = node1gs1.MonitorProcess(node2gs2.Self())
	// wait a bit for the 'DOWN' message if something went wrong
	if err := waitForTimeout(gs1.v); err != nil {
		t.Fatal(err)
	}
	node2.Stop()
	result = etf.Tuple{etf.Atom("DOWN"), ref, etf.Atom("process"), node2gs2.Self(), etf.Atom("noconnection")}
	waitForResultWithValue(t, gs1.v, result)
	if err := chechCleanProcessRef(node1, ref); err != nil {
		t.Fatal(err)
	}

	fmt.Printf("... by Pid Local-Remote: gs1 -> gs2. UnknownNode: ")
	ref = node1gs1.MonitorProcess(node2gs2.Self())
	result = etf.Tuple{etf.Atom("DOWN"), ref, etf.Atom("process"), node2gs2.Self(), etf.Atom("noconnection")}
	waitForResultWithValue(t, gs1.v, result)
	if err := chechCleanProcessRef(node1, ref); err != nil {
		t.Fatal(err)
	}
	node1.Stop()
}

func TestMonitorLocalRemoteByTuple(t *testing.T) {
	fmt.Printf("\n=== Test Monitor Local-Remote by Tuple\n")
	fmt.Printf("Starting nodes: nodeM1LocalRemoteByTuple@localhost, nodeM2LocalRemoteByTuple@localhost: ")
	node1 := CreateNode("nodeM1LocalRemoteByTuple@localhost", "cookies", NodeOptions{})
	node2 := CreateNode("nodeM2LocalRemoteByTuple@localhost", "cookies", NodeOptions{})
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

	// starting gen servers
	fmt.Printf("    wait for start of gs1 on %#v: ", node1.FullName)
	node1gs1, _ := node1.Spawn("gs1", ProcessOptions{}, gs1, nil)
	waitForResultWithValue(t, gs1.v, node1gs1.Self())

	fmt.Printf("    wait for start of gs2 on %#v: ", node2.FullName)
	node2gs2, _ := node2.Spawn("gs2", ProcessOptions{}, gs2, nil)
	waitForResultWithValue(t, gs2.v, node2gs2.Self())

	tuple2 := etf.Tuple{"gs2", node2.FullName}

	fmt.Printf("... by Tuple Local-Remote: gs1 -> gs2. demonitor: ")
	ref := node1gs1.MonitorProcess(tuple2)
	// wait a bit for the 'DOWN' message if something went wrong
	if err := waitForTimeout(gs1.v); err != nil {
		t.Fatal(err)
	}
	if err := chechCleanProcessRef(node1, ref); err == nil {
		t.Fatal("monitor reference has been lost on node 1")
	}
	if err := chechCleanProcessRef(node2, ref); err == nil {
		t.Fatal("monitor reference has been lost on node 2")
	}
	if found := node1gs1.DemonitorProcess(ref); found == false {
		t.Fatal("lost monitoring reference on node1")
	}
	// Demonitoring is the async message with nothing as a feedback.
	// use waitForTimeout just as a short timer
	if err := waitForTimeout(gs1.v); err != nil {
		t.Fatal(err)
	}
	if err := chechCleanProcessRef(node1, ref); err != nil {
		t.Fatal(err)
	}
	if err := chechCleanProcessRef(node2, ref); err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	fmt.Printf("... by Tuple Local-Remote: gs1 -> gs2. terminate: ")
	ref = node1gs1.MonitorProcess(tuple2)
	// wait a bit for the 'DOWN' message if something went wrong
	if err := waitForTimeout(gs1.v); err != nil {
		t.Fatal(err)
	}
	node2gs2.Exit(etf.Pid{}, "normal")
	result := etf.Tuple{etf.Atom("DOWN"), ref, etf.Atom("process"), tuple2, etf.Atom("normal")}
	waitForResultWithValue(t, gs1.v, result)

	if err := chechCleanProcessRef(node1, ref); err != nil {
		t.Fatal(err)
	}
	if err := chechCleanProcessRef(node2, ref); err != nil {
		t.Fatal(err)
	}

	fmt.Printf("... by Tuple Local-Remote: gs1 -> unknownPid: ")
	ref = node1gs1.MonitorProcess(tuple2)
	result = etf.Tuple{etf.Atom("DOWN"), ref, etf.Atom("process"), tuple2, etf.Atom("noproc")}
	waitForResultWithValue(t, gs1.v, result)
	if err := chechCleanProcessRef(node1, ref); err != nil {
		t.Fatal(err)
	}

	fmt.Printf("    wait for start of gs2 on %#v: ", node2.FullName)
	node2gs2, _ = node2.Spawn("gs2", ProcessOptions{}, gs2, nil)
	waitForResultWithValue(t, gs2.v, node2gs2.Self())

	fmt.Printf("... by Tuple Local-Remote: gs1 -> gs2. onNodeDown: ")
	ref = node1gs1.MonitorProcess(tuple2)
	// wait a bit for the 'DOWN' message if something went wrong
	if err := waitForTimeout(gs1.v); err != nil {
		t.Fatal(err)
	}
	node2.Stop()
	result = etf.Tuple{etf.Atom("DOWN"), ref, etf.Atom("process"), tuple2, etf.Atom("noconnection")}
	waitForResultWithValue(t, gs1.v, result)
	if err := chechCleanProcessRef(node1, ref); err != nil {
		t.Fatal(err)
	}

	fmt.Printf("... by Tuple Local-Remote: gs1 -> gs2. UnknownNode: ")
	ref = node1gs1.MonitorProcess(tuple2)
	result = etf.Tuple{etf.Atom("DOWN"), ref, etf.Atom("process"), tuple2, etf.Atom("noconnection")}
	waitForResultWithValue(t, gs1.v, result)
	if err := chechCleanProcessRef(node1, ref); err != nil {
		t.Fatal(err)
	}
	node1.Stop()
}

/*
	Test cases for Local-Local
	Link
		by Pid		- equal_pids, already_linked, doesnt_exist, terminate, unlink
*/
func TestLinkLocalLocal(t *testing.T) {
	fmt.Printf("\n=== Test Link Local-Local\n")
	fmt.Printf("Starting node: nodeL1LocalLocal@localhost: ")
	node1 := CreateNode("nodeL1LocalLocal@localhost", "cookies", NodeOptions{})
	if node1 == nil {
		t.Fatal("can't start node")
	} else {
		fmt.Println("OK")
	}
	gs1 := &testMonitorGenServer{
		v: make(chan interface{}, 2),
	}
	gs2 := &testMonitorGenServer{
		v: make(chan interface{}, 2),
	}
	// starting gen servers
	fmt.Printf("    wait for start of gs1 on %#v: ", node1.FullName)
	node1gs1, _ := node1.Spawn("gs1", ProcessOptions{}, gs1, nil)
	waitForResultWithValue(t, gs1.v, node1gs1.Self())

	fmt.Printf("    wait for start of gs2 on %#v: ", node1.FullName)
	node1gs2, _ := node1.Spawn("gs2", ProcessOptions{}, gs2, nil)
	waitForResultWithValue(t, gs2.v, node1gs2.Self())

	fmt.Printf("Testing Link process (by Pid only) Local-Local: gs1 -> gs1. equals: ")
	node1gs1.Link(node1gs1.Self())
	if err := chechCleanLinkPid(node1, node1gs1.Self()); err != nil {
		t.Fatal("link to itself shouldnt happen")
	}
	fmt.Println("OK")

	fmt.Printf("Testing Link process (by Pid only) Local-Local: gs1 -> gs2. unlink: ")
	node1gs1.Link(node1gs2.Self())

	if chechCleanLinkPid(node1, node1gs1.Self()) == nil {
		t.Fatal("link missing for node1gs1")
	}
	if chechCleanLinkPid(node1, node1gs2.Self()) == nil {
		t.Fatal("link missing for node1gs2")
	}
	node1gs1.Unlink(node1gs2.Self())
	if err := chechCleanLinkPid(node1, node1gs1.Self()); err != nil {
		t.Fatal(err)
	}
	if err := chechCleanLinkPid(node1, node1gs2.Self()); err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	fmt.Printf("Testing Link process (by Pid only) Local-Local: gs1 -> gs2. already_linked: ")
	node1gs1.Link(node1gs2.Self())
	if chechCleanLinkPid(node1, node1gs1.Self()) == nil {
		t.Fatal("link missing for node1gs1")
	}
	if chechCleanLinkPid(node1, node1gs2.Self()) == nil {
		t.Fatal("link missing for node1gs2")
	}
	ll := len(node1.monitor.links)
	node1gs2.Link(node1gs1.Self())

	if ll != len(node1.monitor.links) {
		t.Fatal("number of links has changed on the second Link call")
	}
	fmt.Println("OK")

	fmt.Printf("Testing Link process (by Pid only) Local-Local: gs1 -> gs2. terminate (trap_exit = true): ")
	// do not link these process since they are already linked after the previous test
	//node1gs1.Link(node1gs2.Self())

	node1gs1.SetTrapExit(true)
	if chechCleanLinkPid(node1, node1gs1.Self()) == nil {
		t.Fatal("link missing for node1gs1")
	}
	if chechCleanLinkPid(node1, node1gs2.Self()) == nil {
		t.Fatal("link missing for node1gs2")
	}

	node1gs2.Exit(etf.Pid{}, "normal")
	result := etf.Tuple{etf.Atom("EXIT"), node1gs2.Self(), etf.Atom("normal")}
	waitForResultWithValue(t, gs1.v, result)

	if err := chechCleanLinkPid(node1, node1gs1.Self()); err != nil {
		t.Fatal(err)
	}
	if err := chechCleanLinkPid(node1, node1gs2.Self()); err != nil {
		t.Fatal(err)
	}
	if !node1gs1.IsAlive() {
		t.Fatal("gs1 should be alive after gs2 exit due to enabled trap exit on gs1")
	}

	fmt.Printf("Testing Link process (by Pid only) Local-Local: gs1 -> gs2. doesnt_exist: ")
	node1gs1.Link(node1gs2.Self())
	result = etf.Tuple{etf.Atom("EXIT"), node1gs2.Self(), etf.Atom("noproc")}
	waitForResultWithValue(t, gs1.v, result)

	fmt.Printf("    wait for start of gs2 on %#v: ", node1.FullName)
	node1gs2, _ = node1.Spawn("gs2", ProcessOptions{}, gs2, nil)
	waitForResultWithValue(t, gs2.v, node1gs2.Self())

	node1gs1.SetTrapExit(false)
	fmt.Printf("Testing Link process (by Pid only) Local-Local: gs1 -> gs2. terminate (trap_exit = false): ")
	node1gs1.Link(node1gs2.Self())

	if chechCleanLinkPid(node1, node1gs1.Self()) == nil {
		t.Fatal("link missing for node1gs1")
	}
	if chechCleanLinkPid(node1, node1gs2.Self()) == nil {
		t.Fatal("link missing for node1gs2")
	}

	node1gs2.Exit(etf.Pid{}, "normal")

	// wait a bit to make sure if we recieve anything (shouldnt receive)
	if err := waitForTimeout(gs1.v); err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	if err := chechCleanLinkPid(node1, node1gs1.Self()); err != nil {
		t.Fatal(err)
	}
	if err := chechCleanLinkPid(node1, node1gs2.Self()); err != nil {
		t.Fatal(err)
	}
	if node1gs1.IsAlive() {
		t.Fatal("gs1 shouldnt be alive after gs2 exit due to disable trap exit on gs1")
	}

	node1.Stop()
}

/*
	Test cases for Local-Remote
	Link
		by Pid		- already_linked, doesnt_exist, terminate, unlink, node_down, node_unknown
*/
func TestLinkLocalRemote(t *testing.T) {
	fmt.Printf("\n=== Test Link Local-Remote by Pid\n")
	fmt.Printf("Starting nodes: nodeL1LocalRemoteByPid@localhost, nodeL2LocalRemoteByPid@localhost: ")
	node1 := CreateNode("nodeL1LocalRemoteByPid@localhost", "cookies", NodeOptions{})
	node2 := CreateNode("nodeL2LocalRemoteByPid@localhost", "cookies", NodeOptions{})
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

	// starting gen servers
	fmt.Printf("    wait for start of gs1 on %#v: ", node1.FullName)
	node1gs1, _ := node1.Spawn("gs1", ProcessOptions{}, gs1, nil)
	waitForResultWithValue(t, gs1.v, node1gs1.Self())

	fmt.Printf("    wait for start of gs2 on %#v: ", node2.FullName)
	node2gs2, _ := node2.Spawn("gs2", ProcessOptions{}, gs2, nil)
	waitForResultWithValue(t, gs2.v, node2gs2.Self())

	fmt.Printf("Testing Link process (by Pid only) Local-Remote: gs1 -> gs2. unlink: ")
	node1gs1.Link(node2gs2.Self())

	if chechCleanLinkPid(node1, node1gs1.Self()) == nil {
		t.Fatal("link missing for node1gs1")
	}
	if chechCleanLinkPid(node1, node2gs2.Self()) == nil {
		t.Fatal("link missing for node2gs2 on node1")
	}
	// wait a bit since linking process is async
	if err := waitForTimeout(gs1.v); err != nil {
		t.Fatal(err)
	}
	if chechCleanLinkPid(node2, node2gs2.Self()) == nil {
		t.Fatal("link missing for node2gs2")
	}
	node1gs1.Unlink(node2gs2.Self())
	if err := chechCleanLinkPid(node1, node1gs1.Self()); err != nil {
		t.Fatal(err)
	}
	if err := chechCleanLinkPid(node1, node2gs2.Self()); err != nil {
		t.Fatal(err)
	}
	// wait a bit since unlinking process is async
	if err := waitForTimeout(gs1.v); err != nil {
		t.Fatal(err)
	}
	if err := chechCleanLinkPid(node2, node2gs2.Self()); err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	fmt.Printf("Testing Link process (by Pid only) Local-Remote: gs1 -> gs2. already_linked: ")
	node1gs1.Link(node2gs2.Self())
	if chechCleanLinkPid(node1, node1gs1.Self()) == nil {
		t.Fatal("link missing for node1gs1")
	}
	// wait a bit since linking process is async
	if err := waitForTimeout(gs1.v); err != nil {
		t.Fatal(err)
	}
	if chechCleanLinkPid(node2, node2gs2.Self()) == nil {
		t.Fatal("link missing for node1gs2")
	}
	ll1 := len(node1.monitor.links)
	ll2 := len(node2.monitor.links)
	node2gs2.Link(node1gs1.Self())
	// wait a bit since linking process is async
	if err := waitForTimeout(gs2.v); err != nil {
		t.Fatal(err)
	}

	if ll1 != len(node1.monitor.links) || ll2 != len(node2.monitor.links) {
		t.Fatal("number of links has changed on the second Link call")
	}
	fmt.Println("OK")

	fmt.Printf("Testing Link process (by Pid only) Local-Remote: gs1 -> gs2. terminate (trap_exit = true): ")
	// do not link these process since they are already linked after the previous test

	node1gs1.SetTrapExit(true)

	node2gs2.Exit(etf.Pid{}, "normal")
	result := etf.Tuple{etf.Atom("EXIT"), node2gs2.Self(), etf.Atom("normal")}
	waitForResultWithValue(t, gs1.v, result)

	if err := chechCleanLinkPid(node1, node1gs1.Self()); err != nil {
		t.Fatal(err)
	}
	if err := chechCleanLinkPid(node1, node2gs2.Self()); err != nil {
		t.Fatal(err)
	}
	if err := chechCleanLinkPid(node2, node2gs2.Self()); err != nil {
		t.Fatal(err)
	}
	if err := chechCleanLinkPid(node2, node1gs1.Self()); err != nil {
		t.Fatal(err)
	}
	if !node1gs1.IsAlive() {
		t.Fatal("gs1 should be alive after gs2 exit due to enabled trap exit on gs1")
	}

	fmt.Printf("Testing Link process (by Pid only) Local-Remote: gs1 -> gs2. doesnt_exist: ")
	ll1 = len(node1.monitor.links)
	node1gs1.Link(node2gs2.Self())
	result = etf.Tuple{etf.Atom("EXIT"), node2gs2.Self(), etf.Atom("noproc")}
	waitForResultWithValue(t, gs1.v, result)
	if ll1 != len(node1.monitor.links) {
		t.Fatal("number of links has changed on the second Link call")
	}

	fmt.Printf("    wait for start of gs2 on %#v: ", node1.FullName)
	node2gs2, _ = node2.Spawn("gs2", ProcessOptions{}, gs2, nil)
	waitForResultWithValue(t, gs2.v, node2gs2.Self())

	node1gs1.SetTrapExit(false)
	fmt.Printf("Testing Link process (by Pid only) Local-Local: gs1 -> gs2. terminate (trap_exit = false): ")
	node1gs1.Link(node2gs2.Self())

	if chechCleanLinkPid(node1, node1gs1.Self()) == nil {
		t.Fatal("link missing for node1gs1")
	}
	if chechCleanLinkPid(node1, node2gs2.Self()) == nil {
		t.Fatal("link missing for node2gs2")
	}

	node2gs2.Exit(etf.Pid{}, "normal")

	// wait a bit to make sure if we recieve anything (shouldnt receive)
	if err := waitForTimeout(gs1.v); err != nil {
		t.Fatal(err)
	}

	if err := chechCleanLinkPid(node1, node1gs1.Self()); err != nil {
		t.Fatal(err)
	}
	if err := chechCleanLinkPid(node1, node2gs2.Self()); err != nil {
		t.Fatal(err)
	}
	if err := chechCleanLinkPid(node2, node2gs2.Self()); err != nil {
		t.Fatal(err)
	}
	if node1gs1.IsAlive() {
		t.Fatal("gs1 shouldnt be alive after gs2 exit due to disable trap exit on gs1")
	}
	fmt.Println("OK")

	fmt.Printf("    wait for start of gs1 on %#v: ", node1.FullName)
	node1gs1, _ = node1.Spawn("gs1", ProcessOptions{}, gs1, nil)
	waitForResultWithValue(t, gs1.v, node1gs1.Self())
	fmt.Printf("    wait for start of gs2 on %#v: ", node2.FullName)
	node2gs2, _ = node2.Spawn("gs2", ProcessOptions{}, gs2, nil)
	waitForResultWithValue(t, gs2.v, node2gs2.Self())

	node1gs1.SetTrapExit(true)
	fmt.Printf("Testing Link process (by Pid only) Local-Remote: gs1 -> gs2. node_down: ")
	node1gs1.Link(node2gs2.Self())
	if err := waitForTimeout(gs1.v); err != nil {
		t.Fatal(err)
	}

	if chechCleanLinkPid(node1, node1gs1.Self()) == nil {
		t.Fatal("link missing for node1gs1")
	}
	if chechCleanLinkPid(node1, node2gs2.Self()) == nil {
		t.Fatal("link missing for node2gs2")
	}
	if chechCleanLinkPid(node2, node2gs2.Self()) == nil {
		t.Fatal("link missing for node2gs2 on node2")
	}

	// its very interesting case. sometimes the hadnling of 'Stop' method
	// goes so fast (on a remote node) so we receive here "kill" as a reason
	// because Stop method starts a sequence of graceful shutdown for all the
	// process on the node
	node2.Stop()
	result1 := etf.Tuple{etf.Atom("EXIT"), node2gs2.Self(), etf.Atom("noconnection")}
	result2 := etf.Tuple{etf.Atom("EXIT"), node2gs2.Self(), etf.Atom("kill")}
	waitForResultWithValueOrValue(t, gs1.v, result1, result2)

	if err := chechCleanLinkPid(node1, node1gs1.Self()); err != nil {
		t.Fatal(err)
	}
	if err := chechCleanLinkPid(node1, node2gs2.Self()); err != nil {
		t.Fatal(err)
	}

	ll1 = len(node1.monitor.links)
	fmt.Printf("Testing Link process (by Pid only) Local-Remote: gs1 -> gs2. node_unknown: ")
	node1gs1.Link(node2gs2.Self())
	result = etf.Tuple{etf.Atom("EXIT"), node2gs2.Self(), etf.Atom("noconnection")}
	waitForResultWithValue(t, gs1.v, result)

	if ll1 != len(node1.monitor.links) {
		t.Fatal("number of links has changed on the second Link call")
	}
	node1.Stop()
}

// helpers
func chechCleanProcessRef(node *Node, ref etf.Ref) error {
	node.monitor.mutexProcesses.Lock()
	defer node.monitor.mutexProcesses.Unlock()
	key := ref2key(ref)
	if _, ok := node.monitor.ref2pid[key]; ok {
		return fmt.Errorf("monitor process reference hasnt clean correctly")
	}

	return nil
}

func chechCleanLinkPid(node *Node, pid etf.Pid) error {
	node.monitor.mutexLinks.Lock()
	defer node.monitor.mutexLinks.Unlock()
	if _, ok := node.monitor.links[pid]; ok {
		return fmt.Errorf("process link reference hasnt clean correctly")
	}
	return nil
}
