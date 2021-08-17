package ergo

import (
	"crypto/md5"
	"fmt"
	"math/rand"
	"net"
	"reflect"
	"runtime"
	"sync"
	"testing"

	"github.com/halturin/ergo/dist"
	"github.com/halturin/ergo/etf"
)

type benchCase struct {
	name  string
	value etf.Term
}

func TestNode(t *testing.T) {
	opts := NodeOptions{
		ListenRangeBegin: 25001,
		ListenRangeEnd:   25001,
		EPMDPort:         24999,
	}

	node, _ := CreateNode("node@localhost", "cookies", opts)

	if conn, err := net.Dial("tcp", ":25001"); err != nil {
		fmt.Println("Connect to the node' listening port FAILED")
		t.Fatal(err)
	} else {
		defer conn.Close()
	}

	if conn, err := net.Dial("tcp", ":24999"); err != nil {
		fmt.Println("Connect to the node' listening EPMD port FAILED")
		t.Fatal(err)
	} else {
		defer conn.Close()
	}

	gs1 := &testGenServer{
		err: make(chan error, 2),
	}
	p, e := node.Spawn("", ProcessOptions{}, gs1)
	if e != nil {
		t.Fatal(e)
	}

	if !node.IsProcessAlive(p.Self()) {
		t.Fatal("IsProcessAlive: expect 'true', but got 'false'")
	}

	_, ee := node.ProcessInfo(p.Self())
	if ee != nil {
		t.Fatal(ee)
	}

	node.Stop()
}

type testFragmentationGS struct {
	GenServer
}

func (f *testFragmentationGS) HandleCall(state *GenServerState, from GenServerFrom, message etf.Term) (string, etf.Term) {
	md5original := message.(etf.Tuple)[0].(string)
	blob := message.(etf.Tuple)[1].([]byte)

	result := etf.Atom("ok")
	md5 := fmt.Sprint(md5.Sum(blob))
	if !reflect.DeepEqual(md5original, md5) {
		result = etf.Atom("mismatch")
	}

	return "reply", result
}

func TestNodeFragmentation(t *testing.T) {
	var wg sync.WaitGroup

	blob := make([]byte, 1024*1024)
	rand.Read(blob)
	md5 := fmt.Sprint(md5.Sum(blob))
	message := etf.Tuple{md5, blob}

	node1, _ := CreateNode("nodeT1Fragmentation@localhost", "secret", NodeOptions{})
	node2, _ := CreateNode("nodeT2Fragmentation@localhost", "secret", NodeOptions{})

	tgs := &testFragmentationGS{}
	p1, e1 := node1.Spawn("", ProcessOptions{}, tgs)
	p2, e2 := node2.Spawn("", ProcessOptions{}, tgs)

	if e1 != nil {
		t.Fatal(e1)
	}
	if e2 != nil {
		t.Fatal(e2)
	}

	// check single call
	check, e := p1.Call(p2.Self(), message)
	if e != nil {
		t.Fatal(e)
	}
	if check != etf.Atom("ok") {
		t.Fatal("md5sum mismatch")
	}

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			p1, _ := node1.Spawn("", ProcessOptions{}, tgs)
			p2, _ := node2.Spawn("", ProcessOptions{}, tgs)
			defer wg.Done()
			for k := 0; k < 100; k++ {
				check, e := p1.Call(p2.Self(), message)
				if e != nil {
					t.Fatal(e)
				}
				if check != etf.Atom("ok") {
					t.Fatal("md5sum mismatch")
				}
			}

		}()
	}
	wg.Wait()
}

func TestNodeAtomCache(t *testing.T) {

	node1, _ := CreateNode("nodeT1AtomCache@localhost", "secret", NodeOptions{})
	node2, _ := CreateNode("nodeT2AtomCache@localhost", "secret", NodeOptions{})

	tgs := &benchGS{}
	p1, e1 := node1.Spawn("", ProcessOptions{}, tgs)
	p2, e2 := node2.Spawn("", ProcessOptions{}, tgs)

	if e1 != nil {
		t.Fatal(e1)
	}
	if e2 != nil {
		t.Fatal(e2)
	}

	message := etf.Tuple{
		etf.Atom("a1"),
		etf.Atom("a2"),
		etf.Atom("a3"),
		etf.Atom("a4"),
		etf.Atom("a5"),
	}
	for i := 0; i < 2*runtime.GOMAXPROCS(-1); i++ {
		if _, e := p1.Call(p2.Self(), message); e != nil {
			t.Fatal(e)
		}
	}
}

func TestNodeStaticRoute(t *testing.T) {
	nodeName := "nodeT1StaticRoute@localhost"
	nodeStaticPort := 9876

	node1, _ := CreateNode(nodeName, "secret", NodeOptions{})
	port1 := node1.ResolvePort(nodeName)
	if port1 == -1 {
		t.Fatal("Can't resolve port number for ", nodeName)
	}

	e := node1.AddStaticRoute(nodeName, uint16(nodeStaticPort))
	if e != nil {
		t.Fatal(e)
	}
	port2 := node1.ResolvePort(nodeName)
	// should be overrided by the new value of nodeStaticPort
	if port2 != nodeStaticPort {
		t.Fatal("Wrong port number after adding static route. Got", port2, "Expected", nodeStaticPort)
	}

	node1.RemoveStaticRoute(nodeName)
	port2 = node1.ResolvePort(nodeName)
	// should be resolved into the original port number
	if port1 != port2 {
		t.Fatal("Wrong port number after removing static route")
	}
}

type handshakeGenServer struct {
	GenServer
}

func (h *handshakeGenServer) Init(state *GenServerState, args ...etf.Term) error {
	return nil
}

func (h *handshakeGenServer) HandleCall(state *GenServerState, from GenServerFrom, message etf.Term) (string, etf.Term) {
	return "reply", "pass"
}

func TestNodeDistHandshake(t *testing.T) {
	fmt.Printf("\n=== Test Node Handshake versions\n")

	nodeOptions5 := NodeOptions{
		HandshakeVersion: dist.ProtoHandshake5,
	}
	nodeOptions6 := NodeOptions{
		HandshakeVersion: dist.ProtoHandshake6,
	}
	nodeOptions5WithTLS := NodeOptions{
		HandshakeVersion: dist.ProtoHandshake5,
		TLSmode:          TLSmodeAuto,
	}
	nodeOptions6WithTLS := NodeOptions{
		HandshakeVersion: dist.ProtoHandshake6,
		TLSmode:          TLSmodeAuto,
	}
	hgs := &handshakeGenServer{}

	type Pair struct {
		name  string
		nodeA *Node
		nodeB *Node
	}
	node1, _ := CreateNode("node1Handshake5", "secret", nodeOptions5)
	node2, _ := CreateNode("node2Handshake5", "secret", nodeOptions5)
	node3, _ := CreateNode("node3Handshake5", "secret", nodeOptions5)
	node4, _ := CreateNode("node4Handshake6", "secret", nodeOptions6)
	// node5, _ := CreateNode("node5Handshake6", "secret", nodeOptions6)
	// node6, _ := CreateNode("node6Handshake5", "secret", nodeOptions5)
	node7, _ := CreateNode("node7Handshake6", "secret", nodeOptions6)
	node8, _ := CreateNode("node8Handshake6", "secret", nodeOptions6)
	node9, _ := CreateNode("node9Handshake5", "secret", nodeOptions5WithTLS)
	node10, _ := CreateNode("node10Handshake5", "secret", nodeOptions5WithTLS)
	node11, _ := CreateNode("node11Handshake5", "secret", nodeOptions5WithTLS)
	node12, _ := CreateNode("node12Handshake6", "secret", nodeOptions6WithTLS)
	// node13, _ := CreateNode("node13Handshake6", "secret", nodeOptions6WithTLS)
	// node14, _ := CreateNode("node14Handshake5", "secret", nodeOptions5WithTLS)
	node15, _ := CreateNode("node15Handshake6", "secret", nodeOptions6WithTLS)
	node16, _ := CreateNode("node16Handshake6", "secret", nodeOptions6WithTLS)
	nodes := []Pair{
		Pair{"No TLS. version 5 -> version 5", node1, node2},
		Pair{"No TLS. version 5 -> version 6", node3, node4},
		//Pair{ "No TLS. version 6 -> version 5", node5, node6 },
		Pair{"No TLS. version 6 -> version 6", node7, node8},
		Pair{"With TLS. version 5 -> version 5", node9, node10},
		Pair{"With TLS. version 5 -> version 6", node11, node12},
		//Pair{ "With TLS. version 6 -> version 5", node13, node14 },
		Pair{"With TLS. version 6 -> version 6", node15, node16},
	}

	defer func(nodes []Pair) {
		for i := range nodes {
			nodes[i].nodeA.Stop()
			nodes[i].nodeB.Stop()
		}
	}(nodes)

	var pA, pB *Process
	var e error
	var result etf.Term
	for i := range nodes {
		pair := nodes[i]
		fmt.Printf("    %s: ", pair.name)
		pA, e = pair.nodeA.Spawn("", ProcessOptions{}, hgs)
		if e != nil {
			t.Fatal(e)
		}
		pB, e = pair.nodeB.Spawn("", ProcessOptions{}, hgs)
		if e != nil {
			t.Fatal(e)
		}

		result, e = pA.Call(pB.Self(), "test")
		if e != nil {
			t.Fatal(e)
		}
		if r, ok := result.(string); !ok || r != "pass" {
			t.Fatal("wrong result")
		}
		fmt.Println("OK")
	}
}

func TestNodeRemoteSpawn(t *testing.T) {
	fmt.Printf("\n=== Test Node Remote Spawn\n")
	node1, _ := CreateNode("node1remoteSpawn", "secret", NodeOptions{})
	node2, _ := CreateNode("node2remoteSpawn", "secret", NodeOptions{})
	defer node1.Stop()
	defer node2.Stop()

	node2.ProvideRemoteSpawn("remote", &handshakeGenServer{})
	process, err := node1.Spawn("gs1", ProcessOptions{}, &handshakeGenServer{})
	if err != nil {
		t.Fatal(err)
	}

	opts := RemoteSpawnOptions{
		RegisterName: "remote",
	}
	fmt.Printf("    process gs1@node1 requests spawn new process on node2 and register this process with name 'remote': ")
	_, err = process.RemoteSpawn(node2.Name(), "remote", opts, 1, 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")
	fmt.Printf("    process gs1@node1 requests spawn new process on node2 with the same name (must be failed): ")

	_, err = process.RemoteSpawn(node2.Name(), "remote", opts, 1, 2, 3)
	if err != ErrTaken {
		t.Fatal(err)
	}
	fmt.Println("OK")
}

type benchGS struct {
	GenServer
}

func (b *benchGS) HandleCall(state *GenServerState, from GenServerFrom, message etf.Term) (string, etf.Term) {
	return "reply", etf.Atom("ok")
}

func BenchmarkNodeSequential(b *testing.B) {

	node1name := fmt.Sprintf("nodeB1_%d@localhost", b.N)
	node2name := fmt.Sprintf("nodeB2_%d@localhost", b.N)
	node1, _ := CreateNode(node1name, "bench", NodeOptions{DisableHeaderAtomCache: false})
	node2, _ := CreateNode(node2name, "bench", NodeOptions{})

	bgs := &benchGS{}

	p1, e1 := node1.Spawn("", ProcessOptions{}, bgs)
	p2, e2 := node2.Spawn("", ProcessOptions{}, bgs)

	if e1 != nil {
		b.Fatal(e1)
	}
	if e2 != nil {
		b.Fatal(e2)
	}

	if _, e := p1.Call(p2.Self(), 1); e != nil {
		b.Fatal("single ping", e)
	}

	b.ResetTimer()
	for _, c := range benchCases() {
		b.Run(c.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, e := p1.Call(p2.Self(), c.value)
				if e != nil {
					b.Fatal(e, i)
				}
			}
		})
	}
}

func BenchmarkNodeSequentialSingleNode(b *testing.B) {

	node1name := fmt.Sprintf("nodeB1Local_%d@localhost", b.N)
	node1, _ := CreateNode(node1name, "bench", NodeOptions{DisableHeaderAtomCache: true})

	bgs := &benchGS{}

	p1, e1 := node1.Spawn("", ProcessOptions{}, bgs)
	p2, e2 := node1.Spawn("", ProcessOptions{}, bgs)

	if e1 != nil {
		b.Fatal(e1)
	}
	if e2 != nil {
		b.Fatal(e2)
	}

	if _, e := p1.Call(p2.Self(), 1); e != nil {
		b.Fatal("single ping", e)
	}

	b.ResetTimer()
	for _, c := range benchCases() {
		b.Run(c.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, e := p1.Call(p2.Self(), c.value)
				if e != nil {
					b.Fatal(e, i)
				}
			}
		})
	}
}

func BenchmarkNodeParallel(b *testing.B) {

	node1name := fmt.Sprintf("nodeB1Parallel_%d@localhost", b.N)
	node2name := fmt.Sprintf("nodeB2Parallel_%d@localhost", b.N)
	node1, _ := CreateNode(node1name, "bench", NodeOptions{DisableHeaderAtomCache: false})
	node2, _ := CreateNode(node2name, "bench", NodeOptions{})

	bgs := &benchGS{}

	p1, e1 := node1.Spawn("", ProcessOptions{}, bgs)
	if e1 != nil {
		b.Fatal(e1)
	}
	p2, e2 := node2.Spawn("", ProcessOptions{}, bgs)
	if e2 != nil {
		b.Fatal(e2)
	}

	if _, e := p1.Call(p2.Self(), "hi"); e != nil {
		b.Fatal("single ping", e)
	}

	b.SetParallelism(15)
	b.RunParallel(func(pb *testing.PB) {
		p1, e1 := node1.Spawn("", ProcessOptions{}, bgs)
		if e1 != nil {
			b.Fatal(e1)
		}
		p2, e2 := node2.Spawn("", ProcessOptions{}, bgs)
		if e2 != nil {
			b.Fatal(e2)
		}
		b.ResetTimer()
		for pb.Next() {
			_, e := p1.Call(p2.Self(), etf.Atom("ping"))
			if e != nil {
				b.Fatal(e)
			}
		}

	})
}

func BenchmarkNodeParallelSingleNode(b *testing.B) {

	node1name := fmt.Sprintf("nodeB1ParallelLocal_%d@localhost", b.N)
	node1, _ := CreateNode(node1name, "bench", NodeOptions{DisableHeaderAtomCache: true})

	bgs := &benchGS{}

	p1, e1 := node1.Spawn("", ProcessOptions{}, bgs)
	if e1 != nil {
		b.Fatal(e1)
	}
	p2, e2 := node1.Spawn("", ProcessOptions{}, bgs)
	if e2 != nil {
		b.Fatal(e2)
	}

	if _, e := p1.Call(p2.Self(), "hi"); e != nil {
		b.Fatal("single ping", e)
	}
	b.SetParallelism(15)
	b.RunParallel(func(pb *testing.PB) {
		p1, e1 := node1.Spawn("", ProcessOptions{}, bgs)
		if e1 != nil {
			b.Fatal(e1)
		}
		p2, e2 := node1.Spawn("", ProcessOptions{}, bgs)
		if e2 != nil {
			b.Fatal(e2)
		}
		b.ResetTimer()
		for pb.Next() {
			_, e := p1.Call(p2.Self(), etf.Atom("ping"))
			if e != nil {
				b.Fatal(e)
			}
		}

	})
}
func benchCases() []benchCase {
	return []benchCase{
		benchCase{"number", 12345},
		benchCase{"string", "hello world"},
		benchCase{"tuple (PID)",
			etf.Pid{
				Node:     "node@localhost",
				ID:       1000,
				Creation: 1,
			},
		},
		benchCase{"binary 1MB", make([]byte, 1024*1024)},
	}
}
