package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/halturin/ergo"
	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/gen"
	"github.com/halturin/ergo/node"
)

type testRPCGenServer struct {
	gen.Server
	res chan interface{}
}

type testRPCCase1 struct {
	node string
	mod  string
	fun  string
	args []etf.Term
}

func (trpc *testRPCGenServer) HandleCall(process *gen.ServerProcess, from gen.ServerFrom, message etf.Term) (etf.Term, gen.ServerStatus) {
	return message, gen.ServerStatusOK
}

func (trpc *testRPCGenServer) HandleInfo(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	switch m := message.(type) {
	case testRPCCase1:
		if v, e := process.CallRPC(m.node, m.mod, m.fun, m.args...); e != nil {
			trpc.res <- e
		} else {
			trpc.res <- v
		}
		return gen.ServerStatusOK

	}

	return gen.ServerStatusStop
}
func TestRPC(t *testing.T) {
	fmt.Printf("\n=== Test RPC\n")

	node1, _ := ergo.StartNode("nodeRPC@localhost", "cookies", node.Options{})
	gs1 := &testRPCGenServer{
		res: make(chan interface{}, 2),
	}

	testFun1 := func(a ...etf.Term) etf.Term {
		// return the last argument as a result
		return a[len(a)-1]
	}

	fmt.Printf("Registering RPC method 'testMod.testFun' on %s: ", node1.NodeName())
	time.Sleep(100 * time.Millisecond) // waiting for start 'rex' gen_server
	if e := node1.ProvideRPC("testMod", "testFun", testFun1); e != nil {
		t.Fatal(e)
	} else {
		fmt.Println("OK")
	}

	node1gs1, _ := node1.Spawn("gs1", gen.ProcessOptions{}, gs1, nil)

	fmt.Printf("Call RPC method 'testMod.testFun' with 1 arg on %s: ", node1.NodeName())
	case1 := testRPCCase1{
		node: "nodeRPC@localhost",
		mod:  "testMod",
		fun:  "testFun",
		args: []etf.Term{12345},
	}
	if err := node1gs1.Send(node1gs1.Self(), case1); err != nil {
		t.Fatal(err)
	}
	waitForResultWithValue(t, gs1.res, 12345)

	fmt.Printf("Call RPC method 'testMod.testFun' with 3 arg on %s: ", node1.NodeName())
	case1 = testRPCCase1{
		node: "nodeRPC@localhost",
		mod:  "testMod",
		fun:  "testFun",
		args: []etf.Term{12345, 5.678, node1gs1.Self()},
	}
	if err := node1gs1.Send(node1gs1.Self(), case1); err != nil {
		t.Fatal(err)
	}
	waitForResultWithValue(t, gs1.res, node1gs1.Self())

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
	case1 = testRPCCase1{
		node: "nodeRPC@localhost",
		mod:  "testMod",
		fun:  "testFun",
		args: []etf.Term{12345},
	}
	if err := node1gs1.Send(node1gs1.Self(), case1); err != nil {
		t.Fatal(err)
	}
	waitForResultWithValue(t, gs1.res, expected1)

	fmt.Printf("Call RPC unknown method 'xxx.xxx' on %s: ", node1.NodeName())
	expected2 := etf.Tuple{etf.Atom("badrpc"),
		etf.Tuple{etf.Atom("EXIT"),
			etf.Tuple{etf.Atom("undef"),
				etf.List{
					etf.Tuple{
						etf.Atom("xxx"),
						etf.Atom("xxx"),
						etf.List{12345}, etf.List{}}}}}}
	case1 = testRPCCase1{
		node: "nodeRPC@localhost",
		mod:  "xxx",
		fun:  "xxx",
		args: []etf.Term{12345},
	}
	if err := node1gs1.Send(node1gs1.Self(), case1); err != nil {
		t.Fatal(err)
	}
	waitForResultWithValue(t, gs1.res, expected2)

}
