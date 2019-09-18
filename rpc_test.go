package ergonode

// start node1 with RPC
//  start n1gs1

//  start node2 with disabled RPC
//  start n2gs1

//  register func1 as node1.func1 method

//  rpc call n2gs1.call(node1.func1) (receive reply)
//  rpc call n1gs1.call(node2.whatevername) (recekve badrpc error)

import (
	"fmt"
	"testing"

	"github.com/halturin/ergonode/etf"
)

type testRPCGenServer struct {
	GenServer
	process Process
	v       chan interface{}
}

func (trpc *testRPCGenServer) Init(p Process, args ...interface{}) (state interface{}) {
	trpc.v <- p.Self()
	trpc.process = p
	return nil
}
func (trpc *testRPCGenServer) HandleCast(message etf.Term, state interface{}) (string, interface{}) {
	// fmt.Printf("testRPCGenServer ({%s, %s}): HandleCast: %#v\n", trpc.process.name, trpc.process.Node.FullName, message)
	trpc.v <- message
	return "noreply", state
}
func (trpc *testRPCGenServer) HandleCall(from etf.Tuple, message etf.Term, state interface{}) (string, etf.Term, interface{}) {
	// fmt.Printf("testRPCGenServer ({%s, %s}): HandleCall: %#v, From: %#v\n", trpc.process.name, trpc.process.Node.FullName, message, from)
	return "reply", message, state
}
func (trpc *testRPCGenServer) HandleInfo(message etf.Term, state interface{}) (string, interface{}) {
	// fmt.Printf("testRPCGenServer ({%s, %s}): HandleInfo: %#v\n", trpc.process.name, trpc.process.Node.FullName, message)
	trpc.v <- message
	return "noreply", state
}
func (trpc *testRPCGenServer) Terminate(reason string, state interface{}) {
	// fmt.Printf("\ntestRPCGenServer ({%s, %s}): Terminate: %#v\n", trpc.process.name, trpc.process.Node.FullName, reason)
}

func TestRPC(t *testing.T) {
	node1 := CreateNode("node@localhost", "cookies", NodeOptions{})
	gs1 := &testRPCGenServer{}
	node1gs1, _ := node1.Spawn("gs1", ProcessOptions{}, gs1, nil)

	testFun1 := func(a ...etf.Term) etf.Term {
		return a[0]
	}

	if e := node1.ProvideRPC("testMod", "testFun", testFun1); e != nil {
		message := fmt.Sprintf("%s", e)
		t.Fatal(message)
	}

	if v, e := node1gs1.CallRPC("node@localhost", "testMod", "testFun", 12345); e != nil || v != 12345 {
		message := fmt.Sprintf("%s %#v", e, v)
		t.Fatal(message)
	}
}
