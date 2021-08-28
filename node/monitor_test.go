package node

import (
	"context"
	"fmt"
	"testing"

	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/gen"
)

type testMonitor struct {
	gen.Server
	v chan interface{}
}

func (tgs *testMonitor) Init(process *gen.ServerProcess, args ...etf.Term) error {
	tgs.v <- process.Self()
	return nil
}
func (tgs *testMonitor) HandleCast(process *gen.ServerProcess, message etf.Term) string {
	tgs.v <- message
	return "noreply"
}
func (tgs *testMonitor) HandleCall(process *gen.ServerProcess, from gen.ServerFrom, message etf.Term) (string, etf.Term) {
	return "reply", message
}
func (tgs *testMonitor) HandleInfo(process *gen.ServerProcess, message etf.Term) string {
	tgs.v <- message
	return "noreply"
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
	node1, _ := StartWithContext(context.Background(), "nodeM1LocalLocal@localhost", "cookies", Options{})
	if node1 == nil {
		t.Fatal("can't start node")
	} else {
		fmt.Println("OK")
	}
	gs1 := &testMonitor{
		v: make(chan interface{}, 2),
	}
	gs2 := &testMonitor{
		v: make(chan interface{}, 2),
	}
	// starting gen servers

	fmt.Printf("    wait for start of gs1 on %#v: ", node1.Name())
	node1gs1, _ := node1.Spawn("gs1", gen.ProcessOptions{}, gs1, nil)
	waitForResultWithValue(t, gs1.v, node1gs1.Self())

	fmt.Printf("    wait for start of gs2 on %#v: ", node1.Name())
	node1gs2, _ := node1.Spawn("gs2", gen.ProcessOptions{}, gs2, nil)
	waitForResultWithValue(t, gs2.v, node1gs2.Self())

	// by Pid
	fmt.Printf("... by Pid Local-Local: gs1 -> gs2. demonitor: ")
	ref := node1gs1.MonitorProcess(node1gs2.Self())
	node1.isMonitor(ref)
	if err := checkCleanProcessRef(node1, ref); err == nil {
		t.Fatal("monitor reference has been lost")
	}
	node1gs1.DemonitorProcess(ref)
	if err := checkCleanProcessRef(node1, ref); err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	fmt.Printf("... by Pid Local-Local: gs1 -> gs2. terminate: ")
	ref = node1gs1.MonitorProcess(node1gs2.Self())
	node1gs2.Exit("normal")
	result := etf.Tuple{etf.Atom("DOWN"), ref, etf.Atom("process"), node1gs2.Self(), etf.Atom("normal")}
	waitForResultWithValue(t, gs1.v, result)

	if err := checkCleanProcessRef(node1, ref); err != nil {
		t.Fatal(err)
	}

	fmt.Print("... by Pid Local-Local: gs1 -> unknownPid: ")
	ref = node1gs1.MonitorProcess(node1gs2.Self())
	result = etf.Tuple{etf.Atom("DOWN"), ref, etf.Atom("process"), node1gs2.Self(), etf.Atom("noproc")}
	waitForResultWithValue(t, gs1.v, result)

	fmt.Printf("    wait for start of gs2 on %#v: ", node1.Name())
	node1gs2, _ = node1.Spawn("gs2", gen.ProcessOptions{}, gs2, nil)
	waitForResultWithValue(t, gs2.v, node1gs2.Self())
	// by Name
	fmt.Printf("... by Name Local-Local: gs1 -> gs2. demonitor: ")
	ref = node1gs1.MonitorProcess("gs2")
	if err := checkCleanProcessRef(node1, ref); err == nil {
		t.Fatal("monitor reference has been lost")
	}
	node1gs1.DemonitorProcess(ref)
	if err := checkCleanProcessRef(node1, ref); err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	fmt.Printf("... by Name Local-Local: gs1 -> gs2. terminate: ")
	ref = node1gs1.MonitorProcess("gs2")
	node1gs2.Exit("normal")
	procName := etf.Tuple{"gs2", node1.Name()}
	result = etf.Tuple{etf.Atom("DOWN"), ref, etf.Atom("process"), procName, etf.Atom("normal")}
	waitForResultWithValue(t, gs1.v, result)
	if err := checkCleanProcessRef(node1, ref); err != nil {
		t.Fatal(err)
	}
	fmt.Print("... by Name Local-Local: gs1 -> unknownPid: ")
	ref = node1gs1.MonitorProcess("asdfasdf")
	procName = etf.Tuple{"asdfasdf", node1.Name()}
	result = etf.Tuple{etf.Atom("DOWN"), ref, etf.Atom("process"), procName, etf.Atom("noproc")}
	waitForResultWithValue(t, gs1.v, result)

	fmt.Printf("    wait for start of gs2 on %#v: ", node1.Name())
	node1gs2, _ = node1.Spawn("gs2", gen.ProcessOptions{}, gs2, nil)
	waitForResultWithValue(t, gs2.v, node1gs2.Self())
	// by Tuple {Name, Node}
	fmt.Printf("... by Tuple Local-Local: gs1 -> gs2. demonitor: ")
	tuple := etf.Tuple{"gs2", node1.Name()}
	ref = node1gs1.MonitorProcess(tuple)
	if err := checkCleanProcessRef(node1, ref); err == nil {
		t.Fatal("monitor reference has been lost")
	}
	node1gs1.DemonitorProcess(ref)
	if err := checkCleanProcessRef(node1, ref); err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")
	fmt.Printf("... by Tuple Local-Local: gs1 -> gs2. terminate: ")
	ref = node1gs1.MonitorProcess(tuple)
	node1gs2.Exit("normal")
	result = etf.Tuple{etf.Atom("DOWN"), ref, etf.Atom("process"), tuple, etf.Atom("normal")}
	waitForResultWithValue(t, gs1.v, result)
	if err := checkCleanProcessRef(node1, ref); err != nil {
		t.Fatal(err)
	}
	fmt.Print("... by Tuple Local-Local: gs1 -> unknownPid: ")
	tupleUnknownProc := etf.Tuple{"gs2222", node1.Name()}
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
	node1, _ := StartWithContext(context.Background(), "nodeM1LocalRemoteByPid@localhost", "cookies", Options{})
	node2, _ := StartWithContext(context.Background(), "nodeM2LocalRemoteByPid@localhost", "cookies", Options{})
	if node1 == nil || node2 == nil {
		t.Fatal("can't start nodes")
	} else {
		fmt.Println("OK")
	}

	gs1 := &testMonitor{
		v: make(chan interface{}, 2),
	}
	gs2 := &testMonitor{
		v: make(chan interface{}, 2),
	}

	// starting gen servers
	fmt.Printf("    wait for start of gs1 on %#v: ", node1.Name())
	node1gs1, _ := node1.Spawn("gs1", gen.ProcessOptions{}, gs1, nil)
	waitForResultWithValue(t, gs1.v, node1gs1.Self())

	fmt.Printf("    wait for start of gs2 on %#v: ", node2.Name())
	node2gs2, _ := node2.Spawn("gs2", gen.ProcessOptions{}, gs2, nil)
	waitForResultWithValue(t, gs2.v, node2gs2.Self())

	// by Pid
	fmt.Printf("... by Pid Local-Remote: gs1 -> gs2. demonitor: ")
	ref := node1gs1.MonitorProcess(node2gs2.Self())
	// wait a bit for the 'DOWN' message if something went wrong
	waitForTimeout(t, gs1.v)
	if err := checkCleanProcessRef(node1, ref); err == nil {
		t.Fatal("monitor reference has been lost on node 1")
	}
	if err := checkCleanProcessRef(node2, ref); err == nil {
		t.Fatal("monitor reference has been lost on node 2")
	}
	if found := node1gs1.DemonitorProcess(ref); found == false {
		t.Fatal("lost monitoring reference on node1")
	}
	// Demonitoring is the async message with nothing as a feedback.
	// use waitForTimeout just as a short timer
	waitForTimeout(t, gs1.v)
	if err := checkCleanProcessRef(node1, ref); err != nil {
		t.Fatal(err)
	}
	if err := checkCleanProcessRef(node2, ref); err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	fmt.Printf("... by Pid Local-Remote: gs1 -> gs2. terminate: ")
	ref = node1gs1.MonitorProcess(node2gs2.Self())
	// wait a bit for the 'DOWN' message if something went wrong
	waitForTimeout(t, gs1.v)
	node2gs2.Exit("normal")
	result := etf.Tuple{etf.Atom("DOWN"), ref, etf.Atom("process"), node2gs2.Self(), etf.Atom("normal")}
	waitForResultWithValue(t, gs1.v, result)

	if err := checkCleanProcessRef(node1, ref); err != nil {
		t.Fatal(err)
	}
	if err := checkCleanProcessRef(node2, ref); err != nil {
		t.Fatal(err)
	}

	fmt.Printf("... by Pid Local-Remote: gs1 -> unknownPid: ")
	ref = node1gs1.MonitorProcess(node2gs2.Self())
	result = etf.Tuple{etf.Atom("DOWN"), ref, etf.Atom("process"), node2gs2.Self(), etf.Atom("noproc")}
	waitForResultWithValue(t, gs1.v, result)
	if err := checkCleanProcessRef(node1, ref); err != nil {
		t.Fatal(err)
	}

	fmt.Printf("    wait for start of gs2 on %#v: ", node2.Name())
	node2gs2, _ = node2.Spawn("gs2", gen.ProcessOptions{}, gs2, nil)
	waitForResultWithValue(t, gs2.v, node2gs2.Self())

	fmt.Printf("... by Pid Local-Remote: gs1 -> gs2. onNodeDown: ")
	ref = node1gs1.MonitorProcess(node2gs2.Self())
	// wait a bit for the 'DOWN' message if something went wrong
	waitForTimeout(t, gs1.v)
	node2.Stop()
	result = etf.Tuple{etf.Atom("DOWN"), ref, etf.Atom("process"), node2gs2.Self(), etf.Atom("noconnection")}
	waitForResultWithValue(t, gs1.v, result)
	if err := checkCleanProcessRef(node1, ref); err != nil {
		t.Fatal(err)
	}

	fmt.Printf("... by Pid Local-Remote: gs1 -> gs2. UnknownNode: ")
	ref = node1gs1.MonitorProcess(node2gs2.Self())
	result = etf.Tuple{etf.Atom("DOWN"), ref, etf.Atom("process"), node2gs2.Self(), etf.Atom("noconnection")}
	waitForResultWithValue(t, gs1.v, result)
	if err := checkCleanProcessRef(node1, ref); err != nil {
		t.Fatal(err)
	}
	node1.Stop()
}

func TestMonitorLocalRemoteByTuple(t *testing.T) {
	fmt.Printf("\n=== Test Monitor Local-Remote by Tuple\n")
	fmt.Printf("Starting nodes: nodeM1LocalRemoteByTuple@localhost, nodeM2LocalRemoteByTuple@localhost: ")
	node1, _ := StartWithContext(context.Background(), "nodeM1LocalRemoteByTuple@localhost", "cookies", Options{})
	node2, _ := StartWithContext(context.Background(), "nodeM2LocalRemoteByTuple@localhost", "cookies", Options{})
	if node1 == nil || node2 == nil {
		t.Fatal("can't start nodes")
	} else {
		fmt.Println("OK")
	}

	gs1 := &testMonitor{
		v: make(chan interface{}, 2),
	}
	gs2 := &testMonitor{
		v: make(chan interface{}, 2),
	}

	// starting gen servers
	fmt.Printf("    wait for start of gs1 on %#v: ", node1.Name())
	node1gs1, _ := node1.Spawn("gs1", gen.ProcessOptions{}, gs1, nil)
	waitForResultWithValue(t, gs1.v, node1gs1.Self())

	fmt.Printf("    wait for start of gs2 on %#v: ", node2.Name())
	node2gs2, _ := node2.Spawn("gs2", gen.ProcessOptions{}, gs2, nil)
	waitForResultWithValue(t, gs2.v, node2gs2.Self())

	tuple2 := etf.Tuple{"gs2", node2.Name()}

	fmt.Printf("... by Tuple Local-Remote: gs1 -> gs2. demonitor: ")
	ref := node1gs1.MonitorProcess(tuple2)
	// wait a bit for the 'DOWN' message if something went wrong
	waitForTimeout(t, gs1.v)
	if err := checkCleanProcessRef(node1, ref); err == nil {
		t.Fatal("monitor reference has been lost on node 1")
	}
	if err := checkCleanProcessRef(node2, ref); err == nil {
		t.Fatal("monitor reference has been lost on node 2")
	}
	if found := node1gs1.DemonitorProcess(ref); found == false {
		t.Fatal("lost monitoring reference on node1")
	}
	// Demonitoring is the async message with nothing as a feedback.
	// use waitForTimeout just as a short timer
	waitForTimeout(t, gs1.v)
	if err := checkCleanProcessRef(node1, ref); err != nil {
		t.Fatal(err)
	}
	if err := checkCleanProcessRef(node2, ref); err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	fmt.Printf("... by Tuple Local-Remote: gs1 -> gs2. terminate: ")
	ref = node1gs1.MonitorProcess(tuple2)
	// wait a bit for the 'DOWN' message if something went wrong
	waitForTimeout(t, gs1.v)
	node2gs2.Exit("normal")
	result := etf.Tuple{etf.Atom("DOWN"), ref, etf.Atom("process"), tuple2, etf.Atom("normal")}
	waitForResultWithValue(t, gs1.v, result)

	if err := checkCleanProcessRef(node1, ref); err != nil {
		t.Fatal(err)
	}
	if err := checkCleanProcessRef(node2, ref); err != nil {
		t.Fatal(err)
	}

	fmt.Printf("... by Tuple Local-Remote: gs1 -> unknownPid: ")
	ref = node1gs1.MonitorProcess(tuple2)
	result = etf.Tuple{etf.Atom("DOWN"), ref, etf.Atom("process"), tuple2, etf.Atom("noproc")}
	waitForResultWithValue(t, gs1.v, result)
	if err := checkCleanProcessRef(node1, ref); err != nil {
		t.Fatal(err)
	}

	fmt.Printf("    wait for start of gs2 on %#v: ", node2.Name())
	node2gs2, _ = node2.Spawn("gs2", gen.ProcessOptions{}, gs2, nil)
	waitForResultWithValue(t, gs2.v, node2gs2.Self())

	fmt.Printf("... by Tuple Local-Remote: gs1 -> gs2. onNodeDown: ")
	ref = node1gs1.MonitorProcess(tuple2)
	// wait a bit for the 'DOWN' message if something went wrong
	waitForTimeout(t, gs1.v)
	node2.Stop()
	result = etf.Tuple{etf.Atom("DOWN"), ref, etf.Atom("process"), tuple2, etf.Atom("noconnection")}
	waitForResultWithValue(t, gs1.v, result)
	if err := checkCleanProcessRef(node1, ref); err != nil {
		t.Fatal(err)
	}

	fmt.Printf("... by Tuple Local-Remote: gs1 -> gs2. UnknownNode: ")
	ref = node1gs1.MonitorProcess(tuple2)
	result = etf.Tuple{etf.Atom("DOWN"), ref, etf.Atom("process"), tuple2, etf.Atom("noconnection")}
	waitForResultWithValue(t, gs1.v, result)
	if err := checkCleanProcessRef(node1, ref); err != nil {
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
	node1, _ := StartWithContext(context.Background(), "nodeL1LocalLocal@localhost", "cookies", Options{})
	if node1 == nil {
		t.Fatal("can't start node")
	} else {
		fmt.Println("OK")
	}
	gs1 := &testMonitor{
		v: make(chan interface{}, 2),
	}
	gs2 := &testMonitor{
		v: make(chan interface{}, 2),
	}
	// starting gen servers
	fmt.Printf("    wait for start of gs1 on %#v: ", node1.Name())
	node1gs1, _ := node1.Spawn("gs1", gen.ProcessOptions{}, gs1, nil)
	waitForResultWithValue(t, gs1.v, node1gs1.Self())

	fmt.Printf("    wait for start of gs2 on %#v: ", node1.Name())
	node1gs2, _ := node1.Spawn("gs2", gen.ProcessOptions{}, gs2, nil)
	waitForResultWithValue(t, gs2.v, node1gs2.Self())

	fmt.Printf("Testing Link process (by Pid only) Local-Local: gs1 -> gs1. equals: ")
	node1gs1.Link(node1gs1.Self())
	if err := checkCleanLinkPid(node1, node1gs1.Self()); err != nil {
		t.Fatal("link to itself shouldnt happen")
	}
	fmt.Println("OK")

	fmt.Printf("Testing Link process (by Pid only) Local-Local: gs1 -> gs2. unlink: ")
	node1gs1.Link(node1gs2.Self())

	if checkCleanLinkPid(node1, node1gs1.Self()) == nil {
		t.Fatal("link missing for node1gs1")
	}
	if checkCleanLinkPid(node1, node1gs2.Self()) == nil {
		t.Fatal("link missing for node1gs2")
	}

	node1gs1.Unlink(node1gs2.Self())
	if err := checkCleanLinkPid(node1, node1gs1.Self()); err != nil {
		t.Fatal(err)
	}
	if err := checkCleanLinkPid(node1, node1gs2.Self()); err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	fmt.Printf("Testing Link process (by Pid only) Local-Local: gs1 -> gs2. already_linked: ")
	node1gs1.Link(node1gs2.Self())
	if checkCleanLinkPid(node1, node1gs1.Self()) == nil {
		t.Fatal("link missing for node1gs1")
	}
	if checkCleanLinkPid(node1, node1gs2.Self()) == nil {
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
	if checkCleanLinkPid(node1, node1gs1.Self()) == nil {
		t.Fatal("link missing for node1gs1")
	}
	if checkCleanLinkPid(node1, node1gs2.Self()) == nil {
		t.Fatal("link missing for node1gs2")
	}

	node1gs2.Exit("normal")
	result := etf.Tuple{etf.Atom("EXIT"), node1gs2.Self(), etf.Atom("normal")}
	waitForResultWithValue(t, gs1.v, result)

	if err := checkCleanLinkPid(node1, node1gs1.Self()); err != nil {
		t.Fatal(err)
	}
	if err := checkCleanLinkPid(node1, node1gs2.Self()); err != nil {
		t.Fatal(err)
	}
	if !node1gs1.IsAlive() {
		t.Fatal("gs1 should be alive after gs2 exit due to enabled trap exit on gs1")
	}

	fmt.Printf("Testing Link process (by Pid only) Local-Local: gs1 -> gs2. doesnt_exist: ")
	node1gs1.Link(node1gs2.Self())
	result = etf.Tuple{etf.Atom("EXIT"), node1gs2.Self(), etf.Atom("noproc")}
	waitForResultWithValue(t, gs1.v, result)

	fmt.Printf("    wait for start of gs2 on %#v: ", node1.Name())
	node1gs2, _ = node1.Spawn("gs2", gen.ProcessOptions{}, gs2, nil)
	waitForResultWithValue(t, gs2.v, node1gs2.Self())

	node1gs1.SetTrapExit(false)
	fmt.Printf("Testing Link process (by Pid only) Local-Local: gs1 -> gs2. terminate (trap_exit = false): ")
	node1gs1.Link(node1gs2.Self())

	if checkCleanLinkPid(node1, node1gs1.Self()) == nil {
		t.Fatal("link missing for node1gs1")
	}
	if checkCleanLinkPid(node1, node1gs2.Self()) == nil {
		t.Fatal("link missing for node1gs2")
	}

	node1gs2.Exit("normal")

	// wait a bit to make sure if we receive anything (shouldnt receive)
	waitForTimeout(t, gs1.v)
	fmt.Println("OK")

	if err := checkCleanLinkPid(node1, node1gs1.Self()); err != nil {
		t.Fatal(err)
	}
	if err := checkCleanLinkPid(node1, node1gs2.Self()); err != nil {
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
	node1, _ := StartWithContext(context.Background(), "nodeL1LocalRemoteByPid@localhost", "cookies", Options{})
	node2, _ := StartWithContext(context.Background(), "nodeL2LocalRemoteByPid@localhost", "cookies", Options{})
	if node1 == nil || node2 == nil {
		t.Fatal("can't start nodes")
	} else {
		fmt.Println("OK")
	}

	gs1 := &testMonitor{
		v: make(chan interface{}, 2),
	}
	gs2 := &testMonitor{
		v: make(chan interface{}, 2),
	}

	// starting gen servers
	fmt.Printf("    wait for start of gs1 on %#v: ", node1.Name())
	node1gs1, _ := node1.Spawn("gs1", gen.ProcessOptions{}, gs1, nil)
	waitForResultWithValue(t, gs1.v, node1gs1.Self())

	fmt.Printf("    wait for start of gs2 on %#v: ", node2.Name())
	node2gs2, _ := node2.Spawn("gs2", gen.ProcessOptions{}, gs2, nil)
	waitForResultWithValue(t, gs2.v, node2gs2.Self())

	fmt.Printf("Testing Link process (by Pid only) Local-Remote: gs1 -> gs2. unlink: ")
	node1gs1.Link(node2gs2.Self())

	if checkCleanLinkPid(node1, node1gs1.Self()) == nil {
		t.Fatal("link missing for node1gs1")
	}
	if checkCleanLinkPid(node1, node2gs2.Self()) == nil {
		t.Fatal("link missing for node2gs2 on node1")
	}
	// wait a bit since linking process is async
	waitForTimeout(t, gs1.v)
	if checkCleanLinkPid(node2, node2gs2.Self()) == nil {
		t.Fatal("link missing for node2gs2")
	}
	node1gs1.Unlink(node2gs2.Self())
	if err := checkCleanLinkPid(node1, node1gs1.Self()); err != nil {
		t.Fatal(err)
	}
	if err := checkCleanLinkPid(node1, node2gs2.Self()); err != nil {
		t.Fatal(err)
	}
	// wait a bit since unlinking process is async
	waitForTimeout(t, gs1.v)
	if err := checkCleanLinkPid(node2, node2gs2.Self()); err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	fmt.Printf("Testing Link process (by Pid only) Local-Remote: gs1 -> gs2. already_linked: ")
	node1gs1.Link(node2gs2.Self())
	if checkCleanLinkPid(node1, node1gs1.Self()) == nil {
		t.Fatal("link missing for node1gs1")
	}
	// wait a bit since linking process is async
	waitForTimeout(t, gs1.v)
	if checkCleanLinkPid(node2, node2gs2.Self()) == nil {
		t.Fatal("link missing for node1gs2")
	}
	ll1 := len(node1.monitor.links)
	ll2 := len(node2.monitor.links)
	node2gs2.Link(node1gs1.Self())
	// wait a bit since linking process is async
	waitForTimeout(t, gs2.v)

	if ll1 != len(node1.monitor.links) || ll2 != len(node2.monitor.links) {
		t.Fatal("number of links has changed on the second Link call")
	}
	fmt.Println("OK")

	fmt.Printf("Testing Link process (by Pid only) Local-Remote: gs1 -> gs2. terminate (trap_exit = true): ")
	// do not link these process since they are already linked after the previous test

	node1gs1.SetTrapExit(true)

	node2gs2.Exit("normal")
	result := etf.Tuple{etf.Atom("EXIT"), node2gs2.Self(), etf.Atom("normal")}
	waitForResultWithValue(t, gs1.v, result)

	if err := checkCleanLinkPid(node1, node1gs1.Self()); err != nil {
		t.Fatal(err)
	}
	if err := checkCleanLinkPid(node1, node2gs2.Self()); err != nil {
		t.Fatal(err)
	}
	if err := checkCleanLinkPid(node2, node2gs2.Self()); err != nil {
		t.Fatal(err)
	}
	if err := checkCleanLinkPid(node2, node1gs1.Self()); err != nil {
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

	fmt.Printf("    wait for start of gs2 on %#v: ", node1.Name())
	node2gs2, _ = node2.Spawn("gs2", gen.ProcessOptions{}, gs2, nil)
	waitForResultWithValue(t, gs2.v, node2gs2.Self())

	node1gs1.SetTrapExit(false)
	fmt.Printf("Testing Link process (by Pid only) Local-Local: gs1 -> gs2. terminate (trap_exit = false): ")
	node1gs1.Link(node2gs2.Self())

	if checkCleanLinkPid(node1, node1gs1.Self()) == nil {
		t.Fatal("link missing for node1gs1")
	}
	if checkCleanLinkPid(node1, node2gs2.Self()) == nil {
		t.Fatal("link missing for node2gs2")
	}

	node2gs2.Exit("normal")

	// wait a bit to make sure if we receive anything (shouldnt receive)
	waitForTimeout(t, gs1.v)

	if err := checkCleanLinkPid(node1, node1gs1.Self()); err != nil {
		t.Fatal(err)
	}
	if err := checkCleanLinkPid(node1, node2gs2.Self()); err != nil {
		t.Fatal(err)
	}
	if err := checkCleanLinkPid(node2, node2gs2.Self()); err != nil {
		t.Fatal(err)
	}
	if node1gs1.IsAlive() {
		t.Fatal("gs1 shouldnt be alive after gs2 exit due to disable trap exit on gs1")
	}
	fmt.Println("OK")

	fmt.Printf("    wait for start of gs1 on %#v: ", node1.Name())
	node1gs1, _ = node1.Spawn("gs1", gen.ProcessOptions{}, gs1, nil)
	waitForResultWithValue(t, gs1.v, node1gs1.Self())
	fmt.Printf("    wait for start of gs2 on %#v: ", node2.Name())
	node2gs2, _ = node2.Spawn("gs2", gen.ProcessOptions{}, gs2, nil)
	waitForResultWithValue(t, gs2.v, node2gs2.Self())

	node1gs1.SetTrapExit(true)
	fmt.Printf("Testing Link process (by Pid only) Local-Remote: gs1 -> gs2. node_down: ")
	node1gs1.Link(node2gs2.Self())
	waitForTimeout(t, gs1.v)

	if checkCleanLinkPid(node1, node1gs1.Self()) == nil {
		t.Fatal("link missing for node1gs1")
	}
	if checkCleanLinkPid(node1, node2gs2.Self()) == nil {
		t.Fatal("link missing for node2gs2")
	}
	if checkCleanLinkPid(node2, node2gs2.Self()) == nil {
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

	if err := checkCleanLinkPid(node1, node1gs1.Self()); err != nil {
		t.Fatal(err)
	}
	if err := checkCleanLinkPid(node1, node2gs2.Self()); err != nil {
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
func checkCleanProcessRef(node Node, ref etf.Ref) error {
	node.monitor.mutexProcesses.Lock()
	defer node.monitor.mutexProcesses.Unlock()
	if _, ok := node.monitor.ref2pid[ref]; ok {
		return fmt.Errorf("monitor process reference hasnt clean correctly")
	}

	return nil
}

func checkCleanLinkPid(node Node, pid etf.Pid) error {
	node.monitor.mutexLinks.Lock()
	defer node.monitor.mutexLinks.Unlock()
	if _, ok := node.monitor.links[pid]; ok {
		return fmt.Errorf("process link reference hasnt cleaned correctly")
	}
	return nil
}
