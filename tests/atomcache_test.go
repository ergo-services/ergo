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

	fmt.Printf("case 1: sending 2.1K atoms: ")
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 10; i++ {
		result := atoms2K
		node1gs1.Send(node2gs2.Self(), result)
		if err := waitForResultWithValueReturnError(t, gs2.v, result); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("OK")

	fmt.Printf("case 2: sending a tuple with 2.1K atoms and 66K binary: ")
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 10; i++ {
		result := etf.Tuple{atoms2K, long}
		node1gs1.Send(node2gs2.Self(), result)
		if err := waitForResultWithValueReturnError(t, gs2.v, result); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("OK")

	fmt.Printf("case 3: sending 2.1K UTF-8 long atoms: ")
	s = strings.Repeat("🚀", 252)
	for i := range atoms2K {
		atoms2K[i] = etf.Atom(fmt.Sprintf("%s%d", s, i/10))
	}
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 10; i++ {
		result := atoms2K
		node1gs1.Send(node2gs2.Self(), result)
		if err := waitForResultWithValueReturnError(t, gs2.v, result); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("OK")
	fmt.Printf("case 4: sending a tuple with 2.1K UTF-8 long atoms and 66K binary: ")
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 10; i++ {
		result := etf.Tuple{atoms2K, long}
		node1gs1.Send(node2gs2.Self(), result)
		if err := waitForResultWithValueReturnError(t, gs2.v, result); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("OK")
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

	fmt.Printf("case 1: sending 2.1K atoms: ")
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 10; i++ {
		result := atoms2K
		node1gs1.Send(node2gs2.Self(), result)
		if err := waitForResultWithValueReturnError(t, gs2.v, result); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("OK")
	fmt.Printf("case 2: sending a tuple with 2.1K atoms and 66K binary: ")
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 10; i++ {
		result := etf.Tuple{atoms2K, long}
		node1gs1.Send(node2gs2.Self(), result)
		if err := waitForResultWithValueReturnError(t, gs2.v, result); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("OK")

	fmt.Printf("case 3: sending 2.1K UTF-8 long atoms: ")
	s = strings.Repeat("🚀", 251)
	for i := range atoms2K {
		atoms2K[i] = etf.Atom(fmt.Sprintf("%s%d", s, i))
	}
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 10; i++ {
		result := atoms2K
		node1gs1.Send(node2gs2.Self(), result)
		if err := waitForResultWithValueReturnError(t, gs2.v, result); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("OK")

	fmt.Printf("case 4: sending a tuple with 2.1K UTF-8 long atoms  and 66K binary: ")
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 10; i++ {
		result := etf.Tuple{atoms2K, long}
		node1gs1.Send(node2gs2.Self(), result)
		if err := waitForResultWithValueReturnError(t, gs2.v, result); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("OK")
}

func TestAtomCacheLess255UniqWithCompression(t *testing.T) {
	fmt.Printf("\n=== Test Atom Cache (less 255 uniq) with Compression \n")
	opts1 := node.Options{}
	protoOptions := node.DefaultProtoOptions()
	protoOptions.NumHandlers = 2
	opts1.Proto = dist.CreateProto(protoOptions)
	fmt.Printf("Starting node: nodeAtomCache1Less255Compression@localhost with NumHandlers = 2: ")
	node1, e := ergo.StartNode("nodeAtomCache1Less255Compression@localhost", "cookie", opts1)
	if e != nil {
		t.Fatal(e)
	}
	fmt.Println("OK")

	fmt.Printf("Starting node: nodeAtomCache2Less255Compression@localhost with NubHandlers = 2: ")
	opts2 := node.Options{}
	opts2.Proto = dist.CreateProto(protoOptions)
	node2, e := ergo.StartNode("nodeAtomCache2Less255Compression@localhost", "cookie", opts2)
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

	node1gs1.SetCompression(true)

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

	fmt.Printf("case 1: sending 2.1K atoms: ")
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 10; i++ {
		result := atoms2K
		node1gs1.Send(node2gs2.Self(), result)
		if err := waitForResultWithValueReturnError(t, gs2.v, result); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("OK")
	fmt.Printf("case 2: sending a tuple with 2.1K atoms and 66K binary: ")
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 10; i++ {
		result := etf.Tuple{atoms2K, long}
		node1gs1.Send(node2gs2.Self(), result)
		if err := waitForResultWithValueReturnError(t, gs2.v, result); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("OK")

	fmt.Printf("case 3: sending 2.1K UTF-8 long atoms: ")
	s = strings.Repeat("🚀", 252)
	for i := range atoms2K {
		atoms2K[i] = etf.Atom(fmt.Sprintf("%s%d", s, i/10))
	}
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 10; i++ {
		result := atoms2K
		node1gs1.Send(node2gs2.Self(), result)
		if err := waitForResultWithValueReturnError(t, gs2.v, result); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("OK")
	fmt.Printf("case 4: sending a tuple with 2.1K UTF-8 long atoms and 66K binary: ")
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 10; i++ {
		result := etf.Tuple{atoms2K, long}
		node1gs1.Send(node2gs2.Self(), result)
		if err := waitForResultWithValueReturnError(t, gs2.v, result); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("OK")
}
func TestAtomCacheMore255UniqWithCompression(t *testing.T) {
	fmt.Printf("\n=== Test Atom Cache (more 255 uniq) with Compression \n")
	opts1 := node.Options{}
	protoOptions := node.DefaultProtoOptions()
	protoOptions.NumHandlers = 2
	opts1.Proto = dist.CreateProto(protoOptions)
	fmt.Printf("Starting node: nodeAtomCache1More255Compression@localhost with NumHandlers = 2: ")
	node1, e := ergo.StartNode("nodeAtomCache1More255Compression@localhost", "cookie", opts1)
	if e != nil {
		t.Fatal(e)
	}
	fmt.Println("OK")

	fmt.Printf("Starting node: nodeAtomCache2More255Compression@localhost with NubHandlers = 2: ")
	opts2 := node.Options{}
	opts2.Proto = dist.CreateProto(protoOptions)
	node2, e := ergo.StartNode("nodeAtomCache2More255Compression@localhost", "cookie", opts2)
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

	node1gs1.SetCompression(true)

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

	fmt.Printf("case 1: sending 2.1K atoms: ")
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 10; i++ {
		result := atoms2K
		node1gs1.Send(node2gs2.Self(), result)
		if err := waitForResultWithValueReturnError(t, gs2.v, result); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("OK")
	fmt.Printf("case 2: sending a tuple with 2.1K atoms and 66K binary: ")
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 10; i++ {
		result := etf.Tuple{atoms2K, long}
		node1gs1.Send(node2gs2.Self(), result)
		if err := waitForResultWithValueReturnError(t, gs2.v, result); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("OK")

	fmt.Printf("case 3: sending 2.1K UTF-8 long atoms: ")
	s = strings.Repeat("🚀", 251)
	for i := range atoms2K {
		atoms2K[i] = etf.Atom(fmt.Sprintf("%s%d", s, i))
	}
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 10; i++ {
		result := atoms2K
		node1gs1.Send(node2gs2.Self(), result)
		if err := waitForResultWithValueReturnError(t, gs2.v, result); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("OK")
	fmt.Printf("case 4: sending a tuple with 2.1K UTF-8 long atoms  and 66K binary: ")
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 10; i++ {
		result := etf.Tuple{atoms2K, long}
		node1gs1.Send(node2gs2.Self(), result)
		if err := waitForResultWithValueReturnError(t, gs2.v, result); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("OK")
}

func TestAtomCacheLess255UniqViaProxy(t *testing.T) {
	fmt.Printf("\n=== Test Atom Cache (less 255 uniq) via Proxy connection \n")
	opts1 := node.Options{}
	protoOptions := node.DefaultProtoOptions()
	protoOptions.NumHandlers = 2
	opts1.Proto = dist.CreateProto(protoOptions)
	fmt.Printf("Starting node: nodeAtomCache1Less255ViaProxy@localhost with NumHandlers = 2: ")
	node1, e := ergo.StartNode("nodeAtomCache1Less255ViaProxy@localhost", "cookie", opts1)
	if e != nil {
		t.Fatal(e)
	}
	fmt.Println("OK")

	fmt.Printf("Starting node: nodeAtomCacheTLess255ViaProxy@localhost with NubHandlers = 2: ")
	optsT := node.Options{}
	optsT.Proxy.Transit = true
	optsT.Proto = dist.CreateProto(protoOptions)
	nodeT, e := ergo.StartNode("nodeAtomCacheTLess255ViaProxy@localhost", "cookie", optsT)
	if e != nil {
		t.Fatal(e)
	}
	fmt.Println("OK")

	fmt.Printf("Starting node: nodeAtomCache2Less255ViaProxy@localhost with NubHandlers = 2: ")
	opts2 := node.Options{}
	opts2.Proxy.Accept = true
	opts2.Proto = dist.CreateProto(protoOptions)
	node2, e := ergo.StartNode("nodeAtomCache2Less255ViaProxy@localhost", "cookie", opts2)
	if e != nil {
		t.Fatal(e)
	}
	fmt.Println("OK")
	defer node1.Stop()
	defer node2.Stop()
	defer nodeT.Stop()

	fmt.Printf("    connect %s with %s via proxy %s: ", node1.Name(), node2.Name(), nodeT.Name())
	route := node.ProxyRoute{
		Name:  node2.Name(),
		Proxy: nodeT.Name(),
	}
	node1.AddProxyRoute(route)

	if err := node1.Connect(node2.Name()); err != nil {
		t.Fatal(err)
	}

	indirectNodes := node2.NodesIndirect()
	if len(indirectNodes) != 1 || indirectNodes[0] != node1.Name() {
		t.Fatal("wrong result:", indirectNodes)
	}
	fmt.Println("OK")

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

	fmt.Printf("case 1: sending 2.1K atoms: ")
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 10; i++ {
		result := atoms2K
		node1gs1.Send(node2gs2.Self(), result)
		if err := waitForResultWithValueReturnError(t, gs2.v, result); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("OK")
	fmt.Printf("case 2: sending a tuple with 2.1K atoms and 66K binary: ")
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 10; i++ {
		result := etf.Tuple{atoms2K, long}
		node1gs1.Send(node2gs2.Self(), result)
		if err := waitForResultWithValueReturnError(t, gs2.v, result); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("OK")

	fmt.Printf("case 3: sending 2.1K UTF-8 long atoms: ")
	s = strings.Repeat("🚀", 252)
	for i := range atoms2K {
		atoms2K[i] = etf.Atom(fmt.Sprintf("%s%d", s, i/10))
	}
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 10; i++ {
		result := atoms2K
		node1gs1.Send(node2gs2.Self(), result)
		if err := waitForResultWithValueReturnError(t, gs2.v, result); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("OK")
	fmt.Printf("case 4: sending a tuple with 2.1K UTF-8 long atoms and 66K binary: ")
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 10; i++ {
		result := etf.Tuple{atoms2K, long}
		node1gs1.Send(node2gs2.Self(), result)
		if err := waitForResultWithValueReturnError(t, gs2.v, result); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("OK")
}

func TestAtomCacheMore255UniqViaProxy(t *testing.T) {
	fmt.Printf("\n=== Test Atom Cache (more 255 uniq) via Proxy connection \n")
	opts1 := node.Options{}
	protoOptions := node.DefaultProtoOptions()
	protoOptions.NumHandlers = 2
	opts1.Proto = dist.CreateProto(protoOptions)
	fmt.Printf("Starting node: nodeAtomCache1More255ViaProxy@localhost with NumHandlers = 2: ")
	node1, e := ergo.StartNode("nodeAtomCache1More255ViaProxy@localhost", "cookie", opts1)
	if e != nil {
		t.Fatal(e)
	}
	fmt.Println("OK")

	fmt.Printf("Starting node: nodeAtomCacheTMore255ViaProxy@localhost with NubHandlers = 2: ")
	optsT := node.Options{}
	optsT.Proxy.Transit = true
	optsT.Proto = dist.CreateProto(protoOptions)
	nodeT, e := ergo.StartNode("nodeAtomCacheTMore255ViaProxy@localhost", "cookie", optsT)
	if e != nil {
		t.Fatal(e)
	}
	fmt.Println("OK")

	fmt.Printf("Starting node: nodeAtomCache2More255ViaProxy@localhost with NubHandlers = 2: ")
	opts2 := node.Options{}
	opts2.Proxy.Accept = true
	opts2.Proto = dist.CreateProto(protoOptions)
	node2, e := ergo.StartNode("nodeAtomCache2More255ViaProxy@localhost", "cookie", opts2)
	if e != nil {
		t.Fatal(e)
	}
	fmt.Println("OK")
	defer node1.Stop()
	defer node2.Stop()
	defer nodeT.Stop()

	fmt.Printf("    connect %s with %s via proxy %s: ", node1.Name(), node2.Name(), nodeT.Name())
	route := node.ProxyRoute{
		Name:  node2.Name(),
		Proxy: nodeT.Name(),
	}
	node1.AddProxyRoute(route)

	if err := node1.Connect(node2.Name()); err != nil {
		t.Fatal(err)
	}

	indirectNodes := node2.NodesIndirect()
	if len(indirectNodes) != 1 || indirectNodes[0] != node1.Name() {
		t.Fatal("wrong result:", indirectNodes)
	}
	fmt.Println("OK")

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

	fmt.Printf("case 1: sending 2.1K atoms: ")
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 10; i++ {
		result := atoms2K
		node1gs1.Send(node2gs2.Self(), result)
		if err := waitForResultWithValueReturnError(t, gs2.v, result); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("OK")
	fmt.Printf("case 2: sending a tuple with 2.1K atoms and 66K binary: ")
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 10; i++ {
		result := etf.Tuple{atoms2K, long}
		node1gs1.Send(node2gs2.Self(), result)
		if err := waitForResultWithValueReturnError(t, gs2.v, result); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("OK")

	fmt.Printf("case 3: sending 2.1K UTF-8 long atoms: ")
	s = strings.Repeat("🚀", 252)
	for i := range atoms2K {
		atoms2K[i] = etf.Atom(fmt.Sprintf("%s%d", s, i/10))
	}
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 10; i++ {
		result := atoms2K
		node1gs1.Send(node2gs2.Self(), result)
		if err := waitForResultWithValueReturnError(t, gs2.v, result); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("OK")
	fmt.Printf("case 4: sending a tuple with 2.1K UTF-8 long atoms and 66K binary: ")
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 10; i++ {
		result := etf.Tuple{atoms2K, long}
		node1gs1.Send(node2gs2.Self(), result)
		if err := waitForResultWithValueReturnError(t, gs2.v, result); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("OK")
}

func TestAtomCacheLess255UniqViaProxyWithEncryption(t *testing.T) {
	fmt.Printf("\n=== Test Atom Cache (less 255 uniq) via Proxy connection with Encryption\n")
	opts1 := node.Options{}
	protoOptions := node.DefaultProtoOptions()
	protoOptions.NumHandlers = 2
	opts1.Proto = dist.CreateProto(protoOptions)

	opts1.Proxy.Flags = node.DefaultProxyFlags()
	opts1.Proxy.Flags.EnableEncryption = true

	fmt.Printf("Starting node: nodeAtomCache1Less255ViaProxyEnc@localhost with NumHandlers = 2: ")
	node1, e := ergo.StartNode("nodeAtomCache1Less255ViaProxyEnc@localhost", "cookie", opts1)
	if e != nil {
		t.Fatal(e)
	}
	fmt.Println("OK")

	fmt.Printf("Starting node: nodeAtomCacheTLess255ViaProxyEnc@localhost with NubHandlers = 2: ")
	optsT := node.Options{}
	optsT.Proxy.Transit = true
	optsT.Proto = dist.CreateProto(protoOptions)
	nodeT, e := ergo.StartNode("nodeAtomCacheTLess255ViaProxyEnc@localhost", "cookie", optsT)
	if e != nil {
		t.Fatal(e)
	}
	fmt.Println("OK")

	fmt.Printf("Starting node: nodeAtomCache2Less255ViaProxyEnc@localhost with NubHandlers = 2: ")
	opts2 := node.Options{}
	opts2.Proxy.Accept = true
	opts2.Proto = dist.CreateProto(protoOptions)
	node2, e := ergo.StartNode("nodeAtomCache2Less255ViaProxyEnc@localhost", "cookie", opts2)
	if e != nil {
		t.Fatal(e)
	}
	fmt.Println("OK")
	defer node1.Stop()
	defer node2.Stop()
	defer nodeT.Stop()

	fmt.Printf("    connect %s with %s via proxy %s: ", node1.Name(), node2.Name(), nodeT.Name())
	route := node.ProxyRoute{
		Name:  node2.Name(),
		Proxy: nodeT.Name(),
	}
	node1.AddProxyRoute(route)

	if err := node1.Connect(node2.Name()); err != nil {
		t.Fatal(err)
	}

	indirectNodes := node2.NodesIndirect()
	if len(indirectNodes) != 1 || indirectNodes[0] != node1.Name() {
		t.Fatal("wrong result:", indirectNodes)
	}
	fmt.Println("OK")

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

	fmt.Printf("case 1: sending 2.1K atoms: ")
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 10; i++ {
		result := atoms2K
		node1gs1.Send(node2gs2.Self(), result)
		if err := waitForResultWithValueReturnError(t, gs2.v, result); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("OK")
	fmt.Printf("case 2: sending a tuple with 2.1K atoms and 66K binary: ")
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 10; i++ {
		result := etf.Tuple{atoms2K, long}
		node1gs1.Send(node2gs2.Self(), result)
		if err := waitForResultWithValueReturnError(t, gs2.v, result); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("OK")

	fmt.Printf("case 3: sending 2.1K UTF-8 long atoms: ")
	s = strings.Repeat("🚀", 252)
	for i := range atoms2K {
		atoms2K[i] = etf.Atom(fmt.Sprintf("%s%d", s, i/10))
	}
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 10; i++ {
		result := atoms2K
		node1gs1.Send(node2gs2.Self(), result)
		if err := waitForResultWithValueReturnError(t, gs2.v, result); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("OK")
	fmt.Printf("case 4: sending a tuple with 2.1K UTF-8 long atoms and 66K binary: ")
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 10; i++ {
		result := etf.Tuple{atoms2K, long}
		node1gs1.Send(node2gs2.Self(), result)
		if err := waitForResultWithValueReturnError(t, gs2.v, result); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("OK")
}

func TestAtomCacheMore255UniqViaProxyWithEncryption(t *testing.T) {
	fmt.Printf("\n=== Test Atom Cache (more 255 uniq) via Proxy connection with Encriptin \n")
	opts1 := node.Options{}
	protoOptions := node.DefaultProtoOptions()
	protoOptions.NumHandlers = 2
	opts1.Proto = dist.CreateProto(protoOptions)

	opts1.Proxy.Flags = node.DefaultProxyFlags()
	opts1.Proxy.Flags.EnableEncryption = true

	fmt.Printf("Starting node: nodeAtomCache1More255ViaProxyEnc@localhost with NumHandlers = 2: ")
	node1, e := ergo.StartNode("nodeAtomCache1More255ViaProxyEnc@localhost", "cookie", opts1)
	if e != nil {
		t.Fatal(e)
	}
	fmt.Println("OK")

	fmt.Printf("Starting node: nodeAtomCacheTMore255ViaProxyEnc@localhost with NubHandlers = 2: ")
	optsT := node.Options{}
	optsT.Proxy.Transit = true
	optsT.Proto = dist.CreateProto(protoOptions)
	nodeT, e := ergo.StartNode("nodeAtomCacheTMore255ViaProxyEnc@localhost", "cookie", optsT)
	if e != nil {
		t.Fatal(e)
	}
	fmt.Println("OK")

	fmt.Printf("Starting node: nodeAtomCache2More255ViaProxyEnc@localhost with NubHandlers = 2: ")
	opts2 := node.Options{}
	opts2.Proxy.Accept = true
	opts2.Proto = dist.CreateProto(protoOptions)
	node2, e := ergo.StartNode("nodeAtomCache2More255ViaProxyEnc@localhost", "cookie", opts2)
	if e != nil {
		t.Fatal(e)
	}
	fmt.Println("OK")
	defer node1.Stop()
	defer node2.Stop()
	defer nodeT.Stop()

	fmt.Printf("    connect %s with %s via proxy %s: ", node1.Name(), node2.Name(), nodeT.Name())
	route := node.ProxyRoute{
		Name:  node2.Name(),
		Proxy: nodeT.Name(),
	}
	node1.AddProxyRoute(route)

	if err := node1.Connect(node2.Name()); err != nil {
		t.Fatal(err)
	}

	indirectNodes := node2.NodesIndirect()
	if len(indirectNodes) != 1 || indirectNodes[0] != node1.Name() {
		t.Fatal("wrong result:", indirectNodes)
	}
	fmt.Println("OK")

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

	fmt.Printf("case 1: sending 2.1K atoms: ")
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 10; i++ {
		result := atoms2K
		node1gs1.Send(node2gs2.Self(), result)
		if err := waitForResultWithValueReturnError(t, gs2.v, result); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("OK")
	fmt.Printf("case 2: sending a tuple with 2.1K atoms and 66K binary: ")
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 10; i++ {
		result := etf.Tuple{atoms2K, long}
		node1gs1.Send(node2gs2.Self(), result)
		if err := waitForResultWithValueReturnError(t, gs2.v, result); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("OK")

	fmt.Printf("case 3: sending 2.1K UTF-8 long atoms: ")
	s = strings.Repeat("🚀", 252)
	for i := range atoms2K {
		atoms2K[i] = etf.Atom(fmt.Sprintf("%s%d", s, i/10))
	}
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 10; i++ {
		result := atoms2K
		node1gs1.Send(node2gs2.Self(), result)
		if err := waitForResultWithValueReturnError(t, gs2.v, result); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("OK")
	fmt.Printf("case 4: sending a tuple with 2.1K UTF-8 long atoms and 66K binary: ")
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 10; i++ {
		result := etf.Tuple{atoms2K, long}
		node1gs1.Send(node2gs2.Self(), result)
		if err := waitForResultWithValueReturnError(t, gs2.v, result); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("OK")
}

func TestAtomCacheLess255UniqViaProxyWithEncryptionCompression(t *testing.T) {
	fmt.Printf("\n=== Test Atom Cache (less 255 uniq) via Proxy connection with Encryption and Compression\n")
	opts1 := node.Options{}
	protoOptions := node.DefaultProtoOptions()
	protoOptions.NumHandlers = 2
	opts1.Proto = dist.CreateProto(protoOptions)

	opts1.Proxy.Flags = node.DefaultProxyFlags()
	opts1.Proxy.Flags.EnableEncryption = true

	fmt.Printf("Starting node: nodeAtomCache1Less255ViaProxyEncComp@localhost with NumHandlers = 2: ")
	node1, e := ergo.StartNode("nodeAtomCache1Less255ViaProxyEncComp@localhost", "cookie", opts1)
	if e != nil {
		t.Fatal(e)
	}
	fmt.Println("OK")

	fmt.Printf("Starting node: nodeAtomCacheTLess255ViaProxyEncComp@localhost with NubHandlers = 2: ")
	optsT := node.Options{}
	optsT.Proxy.Transit = true
	optsT.Proto = dist.CreateProto(protoOptions)
	nodeT, e := ergo.StartNode("nodeAtomCacheTLess255ViaProxyEncComp@localhost", "cookie", optsT)
	if e != nil {
		t.Fatal(e)
	}
	fmt.Println("OK")

	fmt.Printf("Starting node: nodeAtomCache2Less255ViaProxyEncComp@localhost with NubHandlers = 2: ")
	opts2 := node.Options{}
	opts2.Proxy.Accept = true
	opts2.Proto = dist.CreateProto(protoOptions)
	node2, e := ergo.StartNode("nodeAtomCache2Less255ViaProxyEncComp@localhost", "cookie", opts2)
	if e != nil {
		t.Fatal(e)
	}
	fmt.Println("OK")
	defer node1.Stop()
	defer node2.Stop()
	defer nodeT.Stop()

	fmt.Printf("    connect %s with %s via proxy %s: ", node1.Name(), node2.Name(), nodeT.Name())
	route := node.ProxyRoute{
		Name:  node2.Name(),
		Proxy: nodeT.Name(),
	}
	node1.AddProxyRoute(route)

	if err := node1.Connect(node2.Name()); err != nil {
		t.Fatal(err)
	}

	indirectNodes := node2.NodesIndirect()
	if len(indirectNodes) != 1 || indirectNodes[0] != node1.Name() {
		t.Fatal("wrong result:", indirectNodes)
	}
	fmt.Println("OK")

	gs1 := &testMonitor{
		v: make(chan interface{}, 2),
	}
	gs2 := &testMonitor{
		v: make(chan interface{}, 2),
	}

	fmt.Printf("    wait for start of gs1 on %#v: ", node1.Name())
	node1gs1, _ := node1.Spawn("gs1", gen.ProcessOptions{}, gs1, nil)
	waitForResultWithValue(t, gs1.v, node1gs1.Self())

	node1gs1.SetCompression(true)

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

	fmt.Printf("case 1: sending 2.1K atoms: ")
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 10; i++ {
		result := atoms2K
		node1gs1.Send(node2gs2.Self(), result)
		if err := waitForResultWithValueReturnError(t, gs2.v, result); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("OK")
	fmt.Printf("case 2: sending a tuple with 2.1K atoms and 66K binary: ")
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 10; i++ {
		result := etf.Tuple{atoms2K, long}
		node1gs1.Send(node2gs2.Self(), result)
		if err := waitForResultWithValueReturnError(t, gs2.v, result); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("OK")

	fmt.Printf("case 3: sending 2.1K UTF-8 long atoms: ")
	s = strings.Repeat("🚀", 252)
	for i := range atoms2K {
		atoms2K[i] = etf.Atom(fmt.Sprintf("%s%d", s, i/10))
	}
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 10; i++ {
		result := atoms2K
		node1gs1.Send(node2gs2.Self(), result)
		if err := waitForResultWithValueReturnError(t, gs2.v, result); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("OK")
	fmt.Printf("case 4: sending a tuple with 2.1K UTF-8 long atoms and 66K binary: ")
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 10; i++ {
		result := etf.Tuple{atoms2K, long}
		node1gs1.Send(node2gs2.Self(), result)
		if err := waitForResultWithValueReturnError(t, gs2.v, result); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("OK")
}

func TestAtomCacheMore255UniqViaProxyWithEncryptionCompression(t *testing.T) {
	fmt.Printf("\n=== Test Atom Cache (more 255 uniq) via Proxy connection with Encription and Compression \n")
	opts1 := node.Options{}
	protoOptions := node.DefaultProtoOptions()
	protoOptions.NumHandlers = 2
	opts1.Proto = dist.CreateProto(protoOptions)

	opts1.Proxy.Flags = node.DefaultProxyFlags()
	opts1.Proxy.Flags.EnableEncryption = true

	fmt.Printf("Starting node: nodeAtomCache1More255ViaProxyEncComp@localhost with NumHandlers = 2: ")
	node1, e := ergo.StartNode("nodeAtomCache1More255ViaProxyEncComp@localhost", "cookie", opts1)
	if e != nil {
		t.Fatal(e)
	}
	fmt.Println("OK")

	fmt.Printf("Starting node: nodeAtomCacheTMore255ViaProxyEncComp@localhost with NubHandlers = 2: ")
	optsT := node.Options{}
	optsT.Proxy.Transit = true
	optsT.Proto = dist.CreateProto(protoOptions)
	nodeT, e := ergo.StartNode("nodeAtomCacheTMore255ViaProxyEncComp@localhost", "cookie", optsT)
	if e != nil {
		t.Fatal(e)
	}
	fmt.Println("OK")

	fmt.Printf("Starting node: nodeAtomCache2More255ViaProxyEncComp@localhost with NubHandlers = 2: ")
	opts2 := node.Options{}
	opts2.Proxy.Accept = true
	opts2.Proto = dist.CreateProto(protoOptions)
	node2, e := ergo.StartNode("nodeAtomCache2More255ViaProxyEncComp@localhost", "cookie", opts2)
	if e != nil {
		t.Fatal(e)
	}
	fmt.Println("OK")
	defer node1.Stop()
	defer node2.Stop()
	defer nodeT.Stop()

	fmt.Printf("    connect %s with %s via proxy %s: ", node1.Name(), node2.Name(), nodeT.Name())
	route := node.ProxyRoute{
		Name:  node2.Name(),
		Proxy: nodeT.Name(),
	}
	node1.AddProxyRoute(route)

	if err := node1.Connect(node2.Name()); err != nil {
		t.Fatal(err)
	}

	indirectNodes := node2.NodesIndirect()
	if len(indirectNodes) != 1 || indirectNodes[0] != node1.Name() {
		t.Fatal("wrong result:", indirectNodes)
	}
	fmt.Println("OK")

	gs1 := &testMonitor{
		v: make(chan interface{}, 2),
	}
	gs2 := &testMonitor{
		v: make(chan interface{}, 2),
	}

	fmt.Printf("    wait for start of gs1 on %#v: ", node1.Name())
	node1gs1, _ := node1.Spawn("gs1", gen.ProcessOptions{}, gs1, nil)
	waitForResultWithValue(t, gs1.v, node1gs1.Self())

	node1gs1.SetCompression(true)

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

	fmt.Printf("case 1: sending 2.1K atoms: ")
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 10; i++ {
		result := atoms2K
		node1gs1.Send(node2gs2.Self(), result)
		if err := waitForResultWithValueReturnError(t, gs2.v, result); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("OK")
	fmt.Printf("case 2: sending a tuple with 2.1K atoms and 66K binary: ")
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 10; i++ {
		result := etf.Tuple{atoms2K, long}
		node1gs1.Send(node2gs2.Self(), result)
		if err := waitForResultWithValueReturnError(t, gs2.v, result); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("OK")

	fmt.Printf("case 3: sending 2.1K UTF-8 long atoms: ")
	s = strings.Repeat("🚀", 252)
	for i := range atoms2K {
		atoms2K[i] = etf.Atom(fmt.Sprintf("%s%d", s, i/10))
	}
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 10; i++ {
		result := atoms2K
		node1gs1.Send(node2gs2.Self(), result)
		if err := waitForResultWithValueReturnError(t, gs2.v, result); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("OK")
	fmt.Printf("case 4: sending a tuple with 2.1K UTF-8 long atoms and 66K binary: ")
	rand.Shuffle(len(atoms2K), func(i, j int) {
		atoms2K[i], atoms2K[j] = atoms2K[j], atoms2K[i]
	})
	for i := 0; i < 10; i++ {
		result := etf.Tuple{atoms2K, long}
		node1gs1.Send(node2gs2.Self(), result)
		if err := waitForResultWithValueReturnError(t, gs2.v, result); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("OK")
}
