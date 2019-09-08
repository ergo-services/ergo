package ergonode

import (
	"fmt"
	"testing"

	"github.com/halturin/ergonode/etf"
)

type testGenServer struct {
	GenServer
	process Process
}

func (tgs *testGenServer) Init(p Process, args ...interface{}) (state interface{}) {
	tgs.process = p
	return nil
}
func (tgs *testGenServer) HandleCast(message etf.Term, state interface{}) (string, interface{}) {
	fmt.Printf("testGenServer ({%s, %s}): HandleCast: %#v\n", tgs.process.name, tgs.process.Node.FullName, message)
	return "noreply", state
}
func (tgs *testGenServer) HandleCall(from etf.Tuple, message etf.Term, state interface{}) (string, etf.Term, interface{}) {
	fmt.Printf("testGenServer ({%s, %s}): HandleCall: %#v, From: %#v\n", tgs.process.name, tgs.process.Node.FullName, message, from)
	reply := etf.Term(etf.Atom("reply"))
	return "reply", reply, state
}
func (tgs *testGenServer) HandleInfo(message etf.Term, state interface{}) (string, interface{}) {
	fmt.Printf("testGenServer ({%s, %s}): HandleInfo: %#v\n", tgs.process.name, tgs.process.Node.FullName, message)
	return "noreply", state
}
func (tgs *testGenServer) Terminate(reason string, state interface{}) {
	fmt.Printf("testGenServer ({%s, %s}): Terminate: %#v\n", tgs.process.name, tgs.process.Node.FullName, reason)
}

func TestGenServer(t *testing.T) {
	node1 := CreateNode("node1@localhost", "cookies", NodeOptions{})
	node2 := CreateNode("node2@localhost", "cookies", NodeOptions{})

	gs1 := &testGenServer{}
	gs2 := &testGenServer{}
	gs3 := &testGenServer{}
	gs4 := &testGenServer{}

	node1p1, _ := node1.Spawn("gs1", ProcessOptions{}, gs1, nil)
	node1p2, _ := node1.Spawn("gs2", ProcessOptions{}, gs2, nil)
	node2p1, _ := node2.Spawn("gs3", ProcessOptions{}, gs3, nil)
	node2p2, _ := node2.Spawn("gs4", ProcessOptions{}, gs4, nil)

	fmt.Println("Testing GenServer process:")
	node1p1.Send(node1p2.Self(), etf.Atom("hi"))
	fmt.Println("    process.Send local -> local: ok")

	node1p1.Cast(node1p2.Self(), etf.Atom("hi cast"))
	fmt.Println("    process.Cast local -> local : ok")

	if _, err := node1p1.Call(node1p2.Self(), etf.Atom("hi call")); err != nil {
		fmt.Println("    process.Call local -> local: failed")
		t.Fatal(err)
	} else {
		fmt.Println("    process.Call local -> local: ok")
	}

	node1p1.Call(node2p1.Self(), etf.Atom("hi remote"))
	node2p2.Call(node1p1.Self(), etf.Atom("remote hi"))
	// fmt.Println("    process.Send local -> remote: ok")

	// node1p1.Cast(node2p2.Self(), etf.Atom("hi cast"))
	// fmt.Println("    process.Cast local -> remote : ok")

	// if _, err := node1p1.Call(node2p2.Self(), etf.Atom("hi call")); err != nil {
	// 	fmt.Println("    process.Call local -> remote: failed")
	// 	t.Fatal(err)
	// } else {
	// 	fmt.Println("    process.Call local -> remote: ok")
	// }

	node1.Stop()
	node2.Stop()
}
