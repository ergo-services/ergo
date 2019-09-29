package ergonode

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/halturin/ergonode/etf"
)

type testRPCGenServer struct {
	GenServer
	process Process
	// v       chan interface{}
}

func (trpc *testRPCGenServer) Init(p Process, args ...interface{}) (state interface{}) {
	// trpc.v <- p.Self()
	trpc.process = p
	return nil
}
func (trpc *testRPCGenServer) HandleCast(message etf.Term, state interface{}) (string, interface{}) {
	// fmt.Printf("testRPCGenServer ({%s, %s}): HandleCast: %#v\n", trpc.process.name, trpc.process.Node.FullName, message)
	// trpc.v <- message
	return "noreply", state
}
func (trpc *testRPCGenServer) HandleCall(from etf.Tuple, message etf.Term, state interface{}) (string, etf.Term, interface{}) {
	// fmt.Printf("testRPCGenServer ({%s, %s}): HandleCall: %#v, From: %#v\n", trpc.process.name, trpc.process.Node.FullName, message, from)
	return "reply", message, state
}
func (trpc *testRPCGenServer) HandleInfo(message etf.Term, state interface{}) (string, interface{}) {
	// fmt.Printf("testRPCGenServer ({%s, %s}): HandleInfo: %#v\n", trpc.process.name, trpc.process.Node.FullName, message)
	// trpc.v <- message
	return "noreply", state
}
func (trpc *testRPCGenServer) Terminate(reason string, state interface{}) {
	// fmt.Printf("\ntestRPCGenServer ({%s, %s}): Terminate: %#v\n", trpc.process.name, trpc.process.Node.FullName, reason)
}

func TestRPC(t *testing.T) {
	fmt.Printf("\n== Test RPC\n")

	node1 := CreateNode("node@localhost", "cookies", NodeOptions{})
	gs1 := &testRPCGenServer{}
	node1gs1, _ := node1.Spawn("gs1", ProcessOptions{}, gs1, nil)

	testFun1 := func(a ...etf.Term) etf.Term {
		return a[len(a)-1]
	}

	fmt.Printf("Registering RPC method 'testMod.testFun' on %s: ", node1.FullName)

	if e := node1.ProvideRPC("testMod", "testFun", testFun1); e != nil {
		message := fmt.Sprintf("%s", e)
		t.Fatal(message)
	} else {
		fmt.Println("OK")
	}

	fmt.Printf("Call RPC method 'testMod.testFun' with 1 arg on %s: ", node1.FullName)
	if v, e := node1gs1.CallRPC("node@localhost", "testMod", "testFun", 12345); e != nil || v != 12345 {
		message := fmt.Sprintf("%s %#v", e, v)
		t.Fatal(message)
	}
	fmt.Println("OK")

	fmt.Printf("Call RPC method 'testMod.testFun' with 3 arg on %s: ", node1.FullName)
	if v, e := node1gs1.CallRPC("node@localhost", "testMod", "testFun", 12345, 5.678, node1gs1.Self()); e != nil || v != node1gs1.Self() {
		message := fmt.Sprintf("%s %#v", e, v)
		t.Fatal(message)
	}
	fmt.Println("OK")

	fmt.Printf("Revoking RPC method 'testMod.testFun' on %s: ", node1.FullName)
	if e := node1.RevokeRPC("testMod", "testFun"); e != nil {
		message := fmt.Sprintf("%s", e)
		t.Fatal(message)
	} else {
		fmt.Println("OK")
	}

	fmt.Printf("Call revoked RPC method 'testMod.testFun' with 1 arg on %s: ", node1.FullName)
	expected1 := etf.Tuple{etf.Atom("badrpc"),
		etf.Tuple{etf.Atom("EXIT"),
			etf.Tuple{etf.Atom("undef"),
				etf.List{
					etf.Tuple{
						etf.Atom("testMod"),
						etf.Atom("testFun"),
						etf.List{12345}, etf.List{}}}}}}
	if v, e := node1gs1.CallRPC("node@localhost", "testMod", "testFun", 12345); e != nil {
		message := fmt.Sprintf("%s %#v", e, v)
		t.Fatal(message)
	} else {
		if !reflect.DeepEqual(v, expected1) {
			message := fmt.Sprintf("expected: %#v got: %#v", expected1, v)
			t.Fatal(message)
		}
	}
	fmt.Println("OK")

	fmt.Printf("Call RPC unknown method 'xxx.xxx' on %s: ", node1.FullName)
	expected2 := etf.Tuple{etf.Atom("badrpc"),
		etf.Tuple{etf.Atom("EXIT"),
			etf.Tuple{etf.Atom("undef"),
				etf.List{
					etf.Tuple{
						etf.Atom("xxx"),
						etf.Atom("xxx"),
						etf.List{12345}, etf.List{}}}}}}

	if v, e := node1gs1.CallRPC("node@localhost", "xxx", "xxx", 12345); e != nil {
		message := fmt.Sprintf("%s %#v", e, v)
		t.Fatal(message)
	} else {
		if !reflect.DeepEqual(v, expected2) {
			message := fmt.Sprintf("expected: %#v got: %#v", expected2, v)
			t.Fatal(message)
		}
	}
	fmt.Println("OK")

}
