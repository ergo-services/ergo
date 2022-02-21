package tests

import (
	"fmt"
	"testing"

	"github.com/ergo-services/ergo"
	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/node"
	"github.com/ergo-services/ergo/proto/dist"
)

func TestAtomCache(t *testing.T) {
	fmt.Printf("\n=== Test Atom Cache \n")
	opts1 := node.Options{}
	protoOptions := node.DefaultProtoOptions()
	protoOptions.NumHandlers = 2
	opts1.Proto = dist.CreateProto(protoOptions)
	fmt.Printf("Starting node: nodeAtomCache1@localhost: ")
	node1, e := ergo.StartNode("nodeAtomCache1@localhost", "cookie", opts1)
	if e != nil {
		t.Fatal(e)
	}
	fmt.Println("OK")

	fmt.Printf("Starting node: nodeAtomCache1@localhost: ")
	opts2 := node.Options{}
	opts2.Proto = dist.CreateProto(protoOptions)
	node2, e := ergo.StartNode("nodeAtomCache2@localhost", "cookie", opts2)
	if e != nil {
		t.Fatal(e)
	}
	fmt.Println("OK")
	defer node1.Stop()
	defer node2.Stop()

	gs1 := &testMonitor{
		v: make(chan interface{}, 2),
	}
	gs2 := &testMonitor{
		v: make(chan interface{}, 2),
	}

	fmt.Printf("    wait for start of gs1 on %#v: ", node1.Name())
	node1gs1, _ := node1.Spawn("gs1", gen.ProcessOptions{}, gs1, nil)
	waitForResultWithValue(t, gs1.v, node1gs1.Self())

	fmt.Printf("    wait for start of gs2 on %#v: ", node2.Name())
	node2gs2, _ := node2.Spawn("gs2", gen.ProcessOptions{}, gs2, nil)
	waitForResultWithValue(t, gs2.v, node2gs2.Self())

	atoms2K := make(etf.List, 2100)
	for i := range atoms2K {
		atoms2K[i] = etf.Atom(fmt.Sprintf("проверкапроверкапроверкапроверкапроверкапроверкапроверкапроверкапроверкапроверкапроверкапроверкапроверкапроверкапроверкапроверкапроверкапроверкапроверкапроверкапроверкапроверкапроверкапроверкапроверкапроверкапроверкапроверкапроверкапровер%d", i))
	}

	long := make([]byte, 66000)
	for i := range long {
		long[i] = byte(i % 255)
	}

	result := etf.Tuple{atoms2K, long}
	fmt.Println("   send 1: ")
	node1gs1.Send(node2gs2.Self(), result)
	waitForResultWithValue(t, gs2.v, result)
	fmt.Println("   send 2: ")
	node1gs1.Send(node2gs2.Self(), result)
	waitForResultWithValue(t, gs2.v, result)
	fmt.Println("   send 3: ")
	node1gs1.Send(node2gs2.Self(), result)
	waitForResultWithValue(t, gs2.v, result)
	fmt.Println("   send 4: ")
	node1gs1.Send(node2gs2.Self(), result)
	waitForResultWithValue(t, gs2.v, result)
	fmt.Println("   send 5: ")
	node1gs1.Send(node2gs2.Self(), result)
	waitForResultWithValue(t, gs2.v, result)

}
