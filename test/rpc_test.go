package test

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/halturin/ergo"
	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/gen"
	"github.com/halturin/ergo/node"
)

type testRPCGenServer struct {
	gen.Server
}

func (trpc *testRPCGenServer) HandleCall(process *gen.ServerProcess, from gen.ServerFrom, message etf.Term) (string, etf.Term) {
	return "reply", message
}

func TestRPC(t *testing.T) {
	fmt.Printf("\n=== Test RPC\n")

	node1, _ := ergo.StartNode("nodeRPC@localhost", "cookies", node.Options{})
	gs1 := &testRPCGenServer{}
	node1gs1, _ := node1.Spawn("gs1", gen.ProcessOptions{}, gs1, nil)

	testFun1 := func(a ...etf.Term) etf.Term {
		return a[len(a)-1]
	}

	fmt.Printf("Registering RPC method 'testMod.testFun' on %s: ", node1.NodeName())
	time.Sleep(100 * time.Millisecond) // waiting for start 'rex' gen_server
	if e := node1.ProvideRPC("testMod", "testFun", testFun1); e != nil {
		t.Fatal(e)
	} else {
		fmt.Println("OK")
	}

	fmt.Printf("Call RPC method 'testMod.testFun' with 1 arg on %s: ", node1.NodeName())
	if v, e := node1gs1.CallRPC("nodeRPC@localhost", "testMod", "testFun", 12345); e != nil || v != 12345 {
		message := fmt.Sprintf("%s %#v", e, v)
		t.Fatal(message)
	}
	fmt.Println("OK")

	fmt.Printf("Call RPC method 'testMod.testFun' with 3 arg on %s: ", node1.NodeName())
	if v, e := node1gs1.CallRPC("nodeRPC@localhost", "testMod", "testFun", 12345, 5.678, node1gs1.Self()); e != nil || v != node1gs1.Self() {
		message := fmt.Sprintf("%s %#v", e, v)
		t.Fatal(message)
	}
	fmt.Println("OK")

	fmt.Printf("Revoking RPC method 'testMod.testFun' on %s: ", node1.NodeName())
	if e := node1.RevokeRPC("testMod", "testFun"); e != nil {
		t.Fatal(e)
	} else {
		fmt.Println("OK")
	}

	fmt.Printf("Call revoked RPC method 'testMod.testFun' with 1 arg on %s: ", node1.NodeName())
	expected1 := etf.Tuple{etf.Atom("badrpc"),
		etf.Tuple{etf.Atom("EXIT"),
			etf.Tuple{etf.Atom("undef"),
				etf.List{
					etf.Tuple{
						etf.Atom("testMod"),
						etf.Atom("testFun"),
						etf.List{12345}, etf.List{}}}}}}
	if v, e := node1gs1.CallRPC("nodeRPC@localhost", "testMod", "testFun", 12345); e != nil {
		message := fmt.Sprintf("%s %#v", e, v)
		t.Fatal(message)
	} else {
		if !reflect.DeepEqual(v, expected1) {
			message := fmt.Sprintf("expected: %#v got: %#v", expected1, v)
			t.Fatal(message)
		}
	}
	fmt.Println("OK")

	fmt.Printf("Call RPC unknown method 'xxx.xxx' on %s: ", node1.NodeName())
	expected2 := etf.Tuple{etf.Atom("badrpc"),
		etf.Tuple{etf.Atom("EXIT"),
			etf.Tuple{etf.Atom("undef"),
				etf.List{
					etf.Tuple{
						etf.Atom("xxx"),
						etf.Atom("xxx"),
						etf.List{12345}, etf.List{}}}}}}

	if v, e := node1gs1.CallRPC("nodeRPC@localhost", "xxx", "xxx", 12345); e != nil {
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
