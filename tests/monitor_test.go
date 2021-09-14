package test

import (
	"fmt"
	"testing"

	"github.com/halturin/ergo"
	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/gen"
	"github.com/halturin/ergo/node"
)

type testMonitor struct {
	gen.Server
	v chan interface{}
}

func (tgs *testMonitor) Init(process *gen.ServerProcess, args ...etf.Term) error {
	tgs.v <- process.Self()
	return nil
}
func (tgs *testMonitor) HandleCast(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	tgs.v <- message
	return gen.ServerStatusOK
}
func (tgs *testMonitor) HandleCall(process *gen.ServerProcess, from gen.ServerFrom, message etf.Term) (etf.Term, gen.ServerStatus) {
	return message, gen.ServerStatusOK
}
func (tgs *testMonitor) HandleInfo(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	tgs.v <- message
	return gen.ServerStatusOK
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
	node1, _ := ergo.StartNode("nodeM1LocalLocal@localhost", "cookies", node.Options{})
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

	if !node1gs2.IsMonitor(ref) {
		t.Fatal("monitor reference has been lost")
	}
	node1gs1.DemonitorProcess(ref)
	if err := checkCleanProcessRef(node1gs1, ref); err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	fmt.Printf("... by Pid Local-Local: gs1 -> gs2. terminate: ")
	ref = node1gs1.MonitorProcess(node1gs2.Self())
	node1gs2.Exit("normal")
	result := gen.MessageDown{
		Ref:    ref,
		Pid:    node1gs2.Self(),
		Reason: "normal",
	}
	waitForResultWithValue(t, gs1.v, result)

	if err := checkCleanProcessRef(node1gs2, ref); err != nil {
		t.Fatal(err)
	}

	fmt.Print("... by Pid Local-Local: gs1 -> unknownPid: ")
	ref = node1gs1.MonitorProcess(node1gs2.Self())
	result = gen.MessageDown{
		Ref:    ref,
		Pid:    node1gs2.Self(),
		Reason: "noproc",
	}
	waitForResultWithValue(t, gs1.v, result)

	fmt.Printf("    wait for start of gs2 on %#v: ", node1.Name())
	node1gs2, _ = node1.Spawn("gs2", gen.ProcessOptions{}, gs2, nil)
	waitForResultWithValue(t, gs2.v, node1gs2.Self())
	// by Name
	fmt.Printf("... by Name Local-Local: gs1 -> gs2. demonitor: ")
	ref = node1gs1.MonitorProcess("gs2")
	if err := checkCleanProcessRef(node1gs1, ref); err == nil {
		t.Fatal("monitor reference has been lost")
	}
	node1gs1.DemonitorProcess(ref)
	if err := checkCleanProcessRef(node1gs1, ref); err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	fmt.Printf("... by Name Local-Local: gs1 -> gs2. terminate: ")
	ref = node1gs1.MonitorProcess("gs2")
	node1gs2.Exit("normal")
	result = gen.MessageDown{
		Ref:       ref,
		ProcessID: gen.ProcessID{Name: "gs2", Node: node1.Name()},
		Reason:    "normal",
	}
	waitForResultWithValue(t, gs1.v, result)
	if err := checkCleanProcessRef(node1gs1, ref); err != nil {
		t.Fatal(err)
	}
	fmt.Print("... by Name Local-Local: gs1 -> unknownPid: ")
	ref = node1gs1.MonitorProcess("asdfasdf")
	result = gen.MessageDown{
		Ref:       ref,
		ProcessID: gen.ProcessID{Name: "asdfasdf", Node: node1.Name()},
		Reason:    "noproc",
	}
	waitForResultWithValue(t, gs1.v, result)

	fmt.Printf("    wait for start of gs2 on %#v: ", node1.Name())
	node1gs2, _ = node1.Spawn("gs2", gen.ProcessOptions{}, gs2, nil)
	waitForResultWithValue(t, gs2.v, node1gs2.Self())

	// by Name gen.ProcessID{ProcessName, Node}
	fmt.Printf("... by gen.ProcessID{Name, Node} Local-Local: gs1 -> gs2. demonitor: ")
	processID := gen.ProcessID{Name: "gs2", Node: node1.Name()}
	ref = node1gs1.MonitorProcess(processID)
	if err := checkCleanProcessRef(node1gs1, ref); err == nil {
		t.Fatal("monitor reference has been lost")
	}
	node1gs1.DemonitorProcess(ref)
	if err := checkCleanProcessRef(node1gs1, ref); err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")
	fmt.Printf("... by gen.ProcessID{Name, Node} Local-Local: gs1 -> gs2. terminate: ")
	ref = node1gs1.MonitorProcess(processID)
	node1gs2.Exit("normal")
	result = gen.MessageDown{
		Ref:       ref,
		ProcessID: processID,
		Reason:    "normal",
	}
	waitForResultWithValue(t, gs1.v, result)
	if err := checkCleanProcessRef(node1gs1, ref); err != nil {
		t.Fatal(err)
	}
	fmt.Print("... by gen.ProcessID{Name, Node} Local-Local: gs1 -> unknownPid: ")
	processID = gen.ProcessID{Name: "gs2222", Node: node1.Name()}
	ref = node1gs1.MonitorProcess(processID)
	result = gen.MessageDown{
		Ref:       ref,
		ProcessID: processID,
		Reason:    "noproc",
	}
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
	node1, err1 := ergo.StartNode("nodeM1LocalRemoteByPid@localhost", "cookies", node.Options{})
	node2, err2 := ergo.StartNode("nodeM2LocalRemoteByPid@localhost", "cookies", node.Options{})
	if err1 != nil {
		t.Fatal("can't start node1:", err1)
	}
	if err2 != nil {
		t.Fatal("can't start node2:", err2)
	}

	fmt.Println("OK")

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
	// wait a bit for the MessageDown if something went wrong
	waitForTimeout(t, gs1.v)
	if err := checkCleanProcessRef(node1gs1, ref); err == nil {
		t.Fatal("monitor reference has been lost on node 1")
	}
	if err := checkCleanProcessRef(node2gs2, ref); err == nil {
		t.Fatal("monitor reference has been lost on node 2")
	}
	if found := node1gs1.DemonitorProcess(ref); found == false {
		t.Fatal("lost monitoring reference on node1")
	}
	// Demonitoring is the async message with nothing as a feedback.
	// use waitForTimeout just as a short timer
	waitForTimeout(t, gs1.v)
	if err := checkCleanProcessRef(node1gs1, ref); err != nil {
		t.Fatal(err)
	}
	if err := checkCleanProcessRef(node2gs2, ref); err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	fmt.Printf("... by Pid Local-Remote: gs1 -> gs2. terminate: ")
	ref = node1gs1.MonitorProcess(node2gs2.Self())
	// wait a bit for the MessageDown if something went wrong
	waitForTimeout(t, gs1.v)
	node2gs2.Exit("normal")
	result := gen.MessageDown{
		Ref:    ref,
		Pid:    node2gs2.Self(),
		Reason: "normal",
	}
	waitForResultWithValue(t, gs1.v, result)

	if err := checkCleanProcessRef(node1gs1, ref); err != nil {
		t.Fatal(err)
	}
	if err := checkCleanProcessRef(node2gs2, ref); err != nil {
		t.Fatal(err)
	}

	fmt.Printf("... by Pid Local-Remote: gs1 -> unknownPid: ")
	ref = node1gs1.MonitorProcess(node2gs2.Self())
	result = gen.MessageDown{
		Ref:    ref,
		Pid:    node2gs2.Self(),
		Reason: "noproc",
	}
	waitForResultWithValue(t, gs1.v, result)
	if err := checkCleanProcessRef(node1gs1, ref); err != nil {
		t.Fatal(err)
	}

	fmt.Printf("    wait for start of gs2 on %#v: ", node2.Name())
	node2gs2, _ = node2.Spawn("gs2", gen.ProcessOptions{}, gs2, nil)
	waitForResultWithValue(t, gs2.v, node2gs2.Self())

	fmt.Printf("... by Pid Local-Remote: gs1 -> gs2. onNodeDown: ")
	ref = node1gs1.MonitorProcess(node2gs2.Self())
	// wait a bit for the MessageDown if something went wrong
	waitForTimeout(t, gs1.v)
	node2.Stop()
	result = gen.MessageDown{
		Ref:    ref,
		Pid:    node2gs2.Self(),
		Reason: "noconnection",
	}
	waitForResultWithValue(t, gs1.v, result)
	if err := checkCleanProcessRef(node1gs1, ref); err != nil {
		t.Fatal(err)
	}

	fmt.Printf("... by Pid Local-Remote: gs1 -> gs2. UnknownNode: ")
	ref = node1gs1.MonitorProcess(node2gs2.Self())
	result.Ref = ref
	waitForResultWithValue(t, gs1.v, result)
	if err := checkCleanProcessRef(node1gs1, ref); err != nil {
		t.Fatal(err)
	}
	node1.Stop()
}

func TestMonitorLocalRemoteByName(t *testing.T) {
	fmt.Printf("\n=== Test Monitor Local-Remote by Name\n")
	fmt.Printf("Starting nodes: nodeM1LocalRemoteByTuple@localhost, nodeM2LocalRemoteByTuple@localhost: ")
	node1, _ := ergo.StartNode("nodeM1LocalRemoteByTuple@localhost", "cookies", node.Options{})
	node2, _ := ergo.StartNode("nodeM2LocalRemoteByTuple@localhost", "cookies", node.Options{})
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

	processID := gen.ProcessID{Name: "gs2", Node: node2.Name()}

	fmt.Printf("... by gen.ProcessID{Name, Node} Local-Remote: gs1 -> gs2. demonitor: ")
	ref := node1gs1.MonitorProcess(processID)
	// wait a bit for the MessageDown if something went wrong
	waitForTimeout(t, gs1.v)
	if err := checkCleanProcessRef(node1gs1, ref); err == nil {
		t.Fatal("monitor reference has been lost on node 1")
	}
	if err := checkCleanProcessRef(node2gs2, ref); err == nil {
		t.Fatal("monitor reference has been lost on node 2")
	}
	if found := node1gs1.DemonitorProcess(ref); found == false {
		t.Fatal("lost monitoring reference on node1")
	}
	// Demonitoring is the async message with nothing as a feedback.
	// use waitForTimeout just as a short timer
	waitForTimeout(t, gs1.v)
	if err := checkCleanProcessRef(node1gs1, ref); err != nil {
		t.Fatal(err)
	}
	if err := checkCleanProcessRef(node2gs2, ref); err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	fmt.Printf("... by gen.ProcessID{Name, Node} Local-Remote: gs1 -> gs2. terminate: ")
	ref = node1gs1.MonitorProcess(processID)
	// wait a bit for the MessageDown if something went wrong
	waitForTimeout(t, gs1.v)
	node2gs2.Exit("normal")
	result := gen.MessageDown{
		Ref:       ref,
		ProcessID: processID,
		Reason:    "normal",
	}
	waitForResultWithValue(t, gs1.v, result)

	if node1gs1.IsMonitor(ref) {
		t.Fatal("monitor ref is still alive")
	}
	if node2gs2.IsMonitor(ref) {
		t.Fatal("monitor ref is still alive")
	}

	fmt.Printf("... by gen.ProcessID{Name, Node} Local-Remote: gs1 -> unknownPid: ")
	ref = node1gs1.MonitorProcess(processID)
	result = gen.MessageDown{
		Ref:       ref,
		ProcessID: processID,
		Reason:    "noproc",
	}
	waitForResultWithValue(t, gs1.v, result)
	if node1gs1.IsMonitor(ref) {
		t.Fatal("monitor ref is still alive")
	}

	fmt.Printf("    wait for start of gs2 on %#v: ", node2.Name())
	node2gs2, _ = node2.Spawn("gs2", gen.ProcessOptions{}, gs2, nil)
	waitForResultWithValue(t, gs2.v, node2gs2.Self())

	fmt.Printf("... by gen.ProcessID{Name, Node} Local-Remote: gs1 -> gs2. onNodeDown: ")
	ref = node1gs1.MonitorProcess(processID)
	result = gen.MessageDown{
		Ref:       ref,
		ProcessID: processID,
		Reason:    "noconnection",
	}
	// wait a bit for the MessageDown if something went wrong
	waitForTimeout(t, gs1.v)
	node2.Stop()
	waitForResultWithValue(t, gs1.v, result)
	if node1gs1.IsMonitor(ref) {
		t.Fatal("monitor ref is still alive")
	}

	fmt.Printf("... by gen.ProcessID{Name, Node} Local-Remote: gs1 -> gs2. UnknownNode: ")
	ref = node1gs1.MonitorProcess(processID)
	result.Ref = ref
	waitForResultWithValue(t, gs1.v, result)
	if node1gs1.IsMonitor(ref) {
		t.Fatal("monitor ref is still alive")
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
	node1, _ := ergo.StartNode("nodeL1LocalLocal@localhost", "cookies", node.Options{})
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

	fmt.Printf("Testing Link process (by Pid only) Local-Local: gs1 -> gs1 (link to itself is not allowed): ")
	node1gs1.Link(node1gs1.Self())
	if err := checkCleanLinkPid(node1gs1, node1gs1.Self()); err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	fmt.Printf("Testing Link process (by Pid only) Local-Local: gs1 -> gs2. link/unlink: ")
	node1gs1.Link(node1gs2.Self())

	if err := checkLinkPid(node1gs1, node1gs2.Self()); err != nil {
		t.Fatal(err)
	}
	if err := checkLinkPid(node1gs2, node1gs1.Self()); err != nil {
		t.Fatal(err)
	}

	node1gs1.Unlink(node1gs2.Self())
	if err := checkCleanLinkPid(node1gs1, node1gs2.Self()); err != nil {
		t.Fatal(err)
	}
	if err := checkCleanLinkPid(node1gs2, node1gs1.Self()); err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	fmt.Printf("Testing Link process (by Pid only) Local-Local: gs1 -> gs2. already_linked: ")
	node1gs1.Link(node1gs2.Self())
	if err := checkLinkPid(node1gs1, node1gs2.Self()); err != nil {
		t.Fatal(err)
	}
	if err := checkLinkPid(node1gs2, node1gs1.Self()); err != nil {
		t.Fatal("link missing for node1gs2")
	}
	gs1links := node1gs1.Links()
	gs2links := node1gs2.Links()
	node1gs2.Link(node1gs1.Self())

	if len(gs1links) != len(node1gs1.Links()) || len(gs2links) != len(node1gs2.Links()) {
		t.Fatal("number of links has changed on the second Link call")
	}
	fmt.Println("OK")

	fmt.Printf("Testing Link process (by Pid only) Local-Local: gs1 -> gs2. terminate (trap_exit = true): ")
	// do not link these process since they are already linked after the previous test
	//node1gs1.Link(node1gs2.Self())

	if checkLinkPid(node1gs1, node1gs2.Self()) != nil {
		t.Fatal("link missing for node1gs1")
	}
	if checkLinkPid(node1gs2, node1gs1.Self()) != nil {
		t.Fatal("link missing for node1gs2")
	}

	node1gs1.SetTrapExit(true)
	node1gs2.Exit("normal")
	result := gen.MessageExit{Pid: node1gs2.Self(), Reason: "normal"}
	waitForResultWithValue(t, gs1.v, result)

	if err := checkCleanLinkPid(node1gs2, node1gs1.Self()); err != nil {
		t.Fatal(err)
	}
	if node1gs2.IsAlive() {
		t.Fatal("node1gs2 must be terminated")
	}
	if err := checkCleanLinkPid(node1gs1, node1gs2.Self()); err != nil {
		t.Fatal(err)
	}
	if !node1gs1.IsAlive() {
		t.Fatal("gs1 should be alive after gs2 exit due to enabled trap exit on gs1")
	}

	fmt.Printf("Testing Link process (by Pid only) Local-Local: gs1 -> gs2. doesnt_exist: ")
	node1gs1.Link(node1gs2.Self())
	result = gen.MessageExit{Pid: node1gs2.Self(), Reason: "noproc"}
	waitForResultWithValue(t, gs1.v, result)

	fmt.Printf("    wait for start of gs2 on %#v: ", node1.Name())
	node1gs2, _ = node1.Spawn("gs2", gen.ProcessOptions{}, gs2, nil)
	waitForResultWithValue(t, gs2.v, node1gs2.Self())

	node1gs1.SetTrapExit(false)
	fmt.Printf("Testing Link process (by Pid only) Local-Local: gs1 -> gs2. terminate (trap_exit = false): ")
	node1gs1.Link(node1gs2.Self())

	if checkLinkPid(node1gs2, node1gs1.Self()) != nil {
		t.Fatal("link missing for node1gs1")
	}
	if checkLinkPid(node1gs1, node1gs2.Self()) != nil {
		t.Fatal("link missing for node1gs2")
	}

	node1gs2.Exit("normal")

	// wait a bit to make sure if we receive anything (shouldnt receive)
	waitForTimeout(t, gs1.v)
	fmt.Println("OK")

	if err := checkCleanLinkPid(node1gs2, node1gs1.Self()); err != nil {
		t.Fatal(err)
	}
	if err := checkCleanLinkPid(node1gs1, node1gs2.Self()); err != nil {
		t.Fatal(err)
	}
	if node1gs1.IsAlive() {
		t.Fatal("gs1 shouldnt be alive after gs2 exit due to disable trap exit on gs1")
	}
	if node1gs2.IsAlive() {
		t.Fatal("node1gs2 must be terminated")
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
	node1, _ := ergo.StartNode("nodeL1LocalRemoteByPid@localhost", "cookies", node.Options{})
	node2, _ := ergo.StartNode("nodeL2LocalRemoteByPid@localhost", "cookies", node.Options{})
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
	// wait a bit since linking process is async
	waitForTimeout(t, gs1.v)

	if checkLinkPid(node1gs1, node2gs2.Self()) != nil {
		t.Fatal("link missing on node1gs1")
	}
	if checkLinkPid(node2gs2, node1gs1.Self()) != nil {
		t.Fatal("link missing on node2gs2 ")
	}

	node1gs1.Unlink(node2gs2.Self())
	// wait a bit since unlinking process is async
	waitForTimeout(t, gs1.v)
	if err := checkCleanLinkPid(node1gs1, node2gs2.Self()); err != nil {
		t.Fatal(err)
	}
	if err := checkCleanLinkPid(node2gs2, node1gs1.Self()); err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")

	fmt.Printf("Testing Link process (by Pid only) Local-Remote: gs1 -> gs2. already_linked: ")
	node1gs1.Link(node2gs2.Self())
	if checkLinkPid(node1gs1, node2gs2.Self()) != nil {
		t.Fatal("link missing on node1gs1")
	}
	// wait a bit since linking process is async
	waitForTimeout(t, gs1.v)
	if checkLinkPid(node2gs2, node1gs1.Self()) != nil {
		t.Fatal("link missing on node2gs2")
	}
	ll1 := len(node1gs1.Links())
	ll2 := len(node2gs2.Links())

	node2gs2.Link(node1gs1.Self())
	// wait a bit since linking process is async
	waitForTimeout(t, gs2.v)

	if ll1 != len(node1gs1.Links()) || ll2 != len(node2gs2.Links()) {
		t.Fatal("number of links has changed on the second Link call")
	}
	fmt.Println("OK")

	fmt.Printf("Testing Link process (by Pid only) Local-Remote: gs1 -> gs2. terminate (trap_exit = true): ")
	// do not link these process since they are already linked after the previous test

	node1gs1.SetTrapExit(true)

	node2gs2.Exit("normal")
	result := gen.MessageExit{Pid: node2gs2.Self(), Reason: "normal"}
	waitForResultWithValue(t, gs1.v, result)

	if err := checkCleanLinkPid(node1gs1, node2gs2.Self()); err != nil {
		t.Fatal(err)
	}
	if err := checkCleanLinkPid(node2gs2, node1gs1.Self()); err != nil {
		t.Fatal(err)
	}
	if !node1gs1.IsAlive() {
		t.Fatal("gs1 should be alive after gs2 exit due to enabled trap exit on gs1")
	}

	fmt.Printf("Testing Link process (by Pid only) Local-Remote: gs1 -> gs2. doesnt_exist: ")
	ll1 = len(node1gs1.Links())
	node1gs1.Link(node2gs2.Self())
	result = gen.MessageExit{Pid: node2gs2.Self(), Reason: "noproc"}
	waitForResultWithValue(t, gs1.v, result)
	if ll1 != len(node1gs1.Links()) {
		t.Fatal("number of links has changed on the second Link call")
	}

	fmt.Printf("    wait for start of gs2 on %#v: ", node1.Name())
	node2gs2, _ = node2.Spawn("gs2", gen.ProcessOptions{}, gs2, nil)
	waitForResultWithValue(t, gs2.v, node2gs2.Self())

	node1gs1.SetTrapExit(false)
	fmt.Printf("Testing Link process (by Pid only) Local-Local: gs1 -> gs2. terminate (trap_exit = false): ")
	node1gs1.Link(node2gs2.Self())
	waitForTimeout(t, gs2.v)

	if checkLinkPid(node1gs1, node2gs2.Self()) != nil {
		t.Fatal("link missing on node1gs1")
	}
	if checkLinkPid(node2gs2, node1gs1.Self()) != nil {
		t.Fatal("link missing on node2gs2")
	}

	node2gs2.Exit("normal")

	// wait a bit to make sure if we receive anything (shouldnt receive)
	waitForTimeout(t, gs1.v)

	if err := checkCleanLinkPid(node1gs1, node2gs2.Self()); err != nil {
		t.Fatal(err)
	}
	if err := checkCleanLinkPid(node2gs2, node1gs1.Self()); err != nil {
		t.Fatal(err)
	}
	if node1gs1.IsAlive() {
		t.Fatal("gs1 shouldnt be alive after gs2 exit due to disable trap exit on gs1")
	}
	if node2gs2.IsAlive() {
		t.Fatal("gs2 must be terminated")
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

	if checkLinkPid(node1gs1, node2gs2.Self()) != nil {
		t.Fatal("link missing for node1gs1")
	}
	if checkCleanLinkPid(node2gs2, node1gs1.Self()) == nil {
		t.Fatal("link missing for node2gs2")
	}

	// race conditioned case.
	// processing of the process termination (on the remote peer) can be done faster than
	// the link termination there, so MessageExit with "kill" reason will be arrived
	// earlier.
	node2.Stop()
	result1 := gen.MessageExit{Pid: node2gs2.Self(), Reason: "noconnection"}
	result2 := gen.MessageExit{Pid: node2gs2.Self(), Reason: "kill"}

	waitForResultWithValueOrValue(t, gs1.v, result1, result2)

	if err := checkCleanLinkPid(node1gs1, node2gs2.Self()); err != nil {
		t.Fatal(err)
	}
	// must wait a bit
	waitForTimeout(t, gs1.v)
	if err := checkCleanLinkPid(node2gs2, node1gs1.Self()); err != nil {
		t.Fatal(err)
	}

	ll1 = len(node1gs1.Links())
	fmt.Printf("Testing Link process (by Pid only) Local-Remote: gs1 -> gs2. node_unknown: ")
	node1gs1.Link(node2gs2.Self())
	result = gen.MessageExit{Pid: node2gs2.Self(), Reason: "noconnection"}
	waitForResultWithValue(t, gs1.v, result)

	if ll1 != len(node1gs1.Links()) {
		t.Fatal("number of links has changed on the second Link call")
	}
	node1.Stop()
}

// helpers
func checkCleanProcessRef(p gen.Process, ref etf.Ref) error {
	if p.IsMonitor(ref) {
		return fmt.Errorf("monitor process reference hasnt clean correctly")
	}

	return nil
}

func checkCleanLinkPid(p gen.Process, pid etf.Pid) error {
	for _, l := range p.Links() {
		if l == pid {
			return fmt.Errorf("process link reference hasnt cleaned correctly")
		}
	}
	return nil
}
func checkLinkPid(p gen.Process, pid etf.Pid) error {
	for _, l := range p.Links() {
		if l == pid {
			return nil
		}
	}
	return fmt.Errorf("process %s has no link to %s", p.Self(), pid)
}
