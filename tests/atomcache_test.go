package tests

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/ergo-services/ergo"
	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/lib"
	"github.com/ergo-services/ergo/node"
	"github.com/ergo-services/ergo/proto/dist"
)

func TestAtomCacheLess255Uniq(t *testing.T) {
	fmt.Printf("\n=== Test Atom Cache (less 255 uniq) \n")
	opts1 := node.Options{}
	protoOptions := node.DefaultProtoOptions()
	protoOptions.NumHandlers = 2
	opts1.Proto = dist.CreateProto(protoOptions)
	fmt.Printf("Starting node: nodeAtomCache1Less255@localhost with NumHandlers = 2: ")
	node1, e := ergo.StartNode("nodeAtomCache1Less255@localhost", "cookie", opts1)
	if e != nil {
		t.Fatal(e)
	}
	fmt.Println("OK")

	fmt.Printf("Starting node: nodeAtomCache2Less255@localhost with NubHandlers = 2: ")
	opts2 := node.Options{}
	opts2.Proto = dist.CreateProto(protoOptions)
	node2, e := ergo.StartNode("nodeAtomCache2Less255@localhost", "cookie", opts2)
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
	s := lib.RandomString(240)
	for i := range atoms2K {
		atoms2K[i] = etf.Atom(fmt.Sprintf("%s%d", s, i/10))
	}

	long := make([]byte, 66*1024)
	for i := range long {
		long[i] = byte(i % 255)
	}

	fmt.Println("case 1: sending 2.1K atoms ...")
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 5; i++ {
		result := atoms2K
		fmt.Printf("    send message %d: ", i+1)
		node1gs1.Send(node2gs2.Self(), result)
		waitForResultWithValue(t, gs2.v, result)
	}
	fmt.Println("case 2: sending a tuple with 2.1K atoms and 66K binary...")
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 5; i++ {
		result := etf.Tuple{atoms2K, long}
		fmt.Printf("    send message %d: ", i+1)
		node1gs1.Send(node2gs2.Self(), result)
		waitForResultWithValue(t, gs2.v, result)
	}

	fmt.Println("case 1: sending 2.1K UTF-8 long atoms ...")
	s = strings.Repeat("🚀", 252)
	for i := range atoms2K {
		atoms2K[i] = etf.Atom(fmt.Sprintf("%s%d", s, i/10))
	}
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 5; i++ {
		result := atoms2K
		fmt.Printf("    send message %d: ", i+1)
		node1gs1.Send(node2gs2.Self(), result)
		waitForResultWithValue(t, gs2.v, result)
	}
	fmt.Println("case 2: sending a tuple with 2.1K UTF-8 long atoms and 66K binary...")
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 5; i++ {
		result := etf.Tuple{atoms2K, long}
		fmt.Printf("    send message %d: ", i+1)
		node1gs1.Send(node2gs2.Self(), result)
		waitForResultWithValue(t, gs2.v, result)
	}
}

func TestAtomCacheMore255Uniq(t *testing.T) {
	fmt.Printf("\n=== Test Atom Cache (more 255 uniq) \n")
	opts1 := node.Options{}
	protoOptions := node.DefaultProtoOptions()
	protoOptions.NumHandlers = 2
	opts1.Proto = dist.CreateProto(protoOptions)
	fmt.Printf("Starting node: nodeAtomCache1More255@localhost with NumHandlers = 2: ")
	node1, e := ergo.StartNode("nodeAtomCache1More255@localhost", "cookie", opts1)
	if e != nil {
		t.Fatal(e)
	}
	fmt.Println("OK")

	fmt.Printf("Starting node: nodeAtomCache2More255@localhost with NubHandlers = 2: ")
	opts2 := node.Options{}
	opts2.Proto = dist.CreateProto(protoOptions)
	node2, e := ergo.StartNode("nodeAtomCache2More255@localhost", "cookie", opts2)
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
	s := lib.RandomString(240)
	for i := range atoms2K {
		atoms2K[i] = etf.Atom(fmt.Sprintf("%s%d", s, i))
	}

	long := make([]byte, 66*1024)
	for i := range long {
		long[i] = byte(i % 255)
	}

	fmt.Println("case 1: sending 2.1K atoms ...")
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 5; i++ {
		result := atoms2K
		fmt.Printf("    send message %d: ", i+1)
		node1gs1.Send(node2gs2.Self(), result)
		waitForResultWithValue(t, gs2.v, result)
	}
	fmt.Println("case 2: sending a tuple with 2.1K atoms and 66K binary...")
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 5; i++ {
		result := etf.Tuple{atoms2K, long}
		fmt.Printf("    send message %d: ", i+1)
		node1gs1.Send(node2gs2.Self(), result)
		waitForResultWithValue(t, gs2.v, result)
	}

	fmt.Println("case 1: sending 2.1K UTF-8 long atoms ...")
	s = strings.Repeat("🚀", 251)
	for i := range atoms2K {
		atoms2K[i] = etf.Atom(fmt.Sprintf("%s%d", s, i))
	}
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 5; i++ {
		result := atoms2K
		fmt.Printf("    send message %d: ", i+1)
		node1gs1.Send(node2gs2.Self(), result)
		waitForResultWithValue(t, gs2.v, result)
	}
	fmt.Println("case 2: sending a tuple with 2.1K UTF-8 long atoms  and 66K binary...")
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 5; i++ {
		result := etf.Tuple{atoms2K, long}
		fmt.Printf("    send message %d: ", i+1)
		node1gs1.Send(node2gs2.Self(), result)
		waitForResultWithValue(t, gs2.v, result)
	}
}
