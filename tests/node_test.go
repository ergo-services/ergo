package test

import (
	"crypto/md5"
	"fmt"
	"math/rand"
	"net"
	"reflect"
	"runtime"
	"sync"
	"testing"

	"github.com/halturin/ergo"
	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/gen"
	"github.com/halturin/ergo/node"
	"github.com/halturin/ergo/node/dist"
)

type benchCase struct {
	name  string
	value etf.Term
}

func TestNode(t *testing.T) {
	opts := node.Options{
		ListenRangeBegin: 25001,
		ListenRangeEnd:   25001,
		EPMDPort:         24999,
	}

	node1, _ := ergo.StartNode("node@localhost", "cookies", opts)

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

	gs1 := &testServer{
		res: make(chan interface{}, 2),
	}
	p, e := node1.Spawn("", gen.ProcessOptions{}, gs1)
	if e != nil {
		t.Fatal(e)
	}

	if !node1.IsProcessAlive(p) {
		t.Fatal("IsProcessAlive: expect 'true', but got 'false'")
	}

	_, ee := node1.ProcessInfo(p.Self())
	if ee != nil {
		t.Fatal(ee)
	}

	node1.Stop()
}

type testFragmentationGS struct {
	gen.Server
}

func (f *testFragmentationGS) HandleCall(process *gen.ServerProcess, from gen.ServerFrom, message etf.Term) (etf.Term, gen.ServerStatus) {
	md5original := message.(etf.Tuple)[0].(string)
	blob := message.(etf.Tuple)[1].([]byte)

	result := etf.Atom("ok")
	md5 := fmt.Sprint(md5.Sum(blob))
	if !reflect.DeepEqual(md5original, md5) {
		result = etf.Atom("mismatch")
	}

	return result, gen.ServerStatusOK
}

func TestNodeFragmentation(t *testing.T) {
	var wg sync.WaitGroup

	blob := make([]byte, 1024*1024)
	rand.Read(blob)
	md5 := fmt.Sprint(md5.Sum(blob))
	message := etf.Tuple{md5, blob}

	node1, _ := ergo.StartNode("nodeT1Fragmentation@localhost", "secret", node.Options{})
	node2, _ := ergo.StartNode("nodeT2Fragmentation@localhost", "secret", node.Options{})

	tgs := &testFragmentationGS{}
	p1, e1 := node1.Spawn("", gen.ProcessOptions{}, tgs)
	p2, e2 := node2.Spawn("", gen.ProcessOptions{}, tgs)

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
			p1, _ := node1.Spawn("", gen.ProcessOptions{}, tgs)
			p2, _ := node2.Spawn("", gen.ProcessOptions{}, tgs)
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

	node1, _ := ergo.StartNode("nodeT1AtomCache@localhost", "secret", node.Options{})
	node2, _ := ergo.StartNode("nodeT2AtomCache@localhost", "secret", node.Options{})

	tgs := &benchGS{}
	p1, e1 := node1.Spawn("", gen.ProcessOptions{}, tgs)
	p2, e2 := node2.Spawn("", gen.ProcessOptions{}, tgs)

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

	node1, _ := ergo.StartNode(nodeName, "secret", node.Options{})
	nr, err := node1.Resolve(nodeName)
	if err != nil {
		t.Fatal("Can't resolve port number for ", nodeName)
	}

	e := node1.AddStaticRoute(nodeName, uint16(nodeStaticPort))
	if e != nil {
		t.Fatal(e)
	}
	// should be overrided by the new value of nodeStaticPort
	if nr, err := node1.Resolve(nodeName); err != nil || nr.Port != nodeStaticPort {
		t.Fatal("Wrong port number after adding static route. Got", nr.Port, "Expected", nodeStaticPort)
	}

	node1.RemoveStaticRoute(nodeName)

	// should be resolved into the original port number
	if nr2, err := node1.Resolve(nodeName); err != nil || nr.Port != nr2.Port {
		t.Fatal("Wrong port number after removing static route")
	}
}

type handshakeGenServer struct {
	gen.Server
}

func (h *handshakeGenServer) Init(process *gen.ServerProcess, args ...etf.Term) error {
	return nil
}

func (h *handshakeGenServer) HandleCall(process *gen.ServerProcess, from gen.ServerFrom, message etf.Term) (etf.Term, gen.ServerStatus) {
	return "pass", gen.ServerStatusOK
}

func TestNodeDistHandshake(t *testing.T) {
	fmt.Printf("\n=== Test Node Handshake versions\n")

	nodeOptions5 := node.Options{
		HandshakeVersion: dist.ProtoHandshake5,
	}
	nodeOptions6 := node.Options{
		HandshakeVersion: dist.ProtoHandshake6,
	}
	nodeOptions5WithTLS := node.Options{
		HandshakeVersion: dist.ProtoHandshake5,
		TLSMode:          node.TLSModeAuto,
	}
	nodeOptions6WithTLS := node.Options{
		HandshakeVersion: dist.ProtoHandshake6,
		TLSMode:          node.TLSModeAuto,
	}
	hgs := &handshakeGenServer{}

	type Pair struct {
		name  string
		nodeA node.Node
		nodeB node.Node
	}
	node1, e1 := ergo.StartNode("node1Handshake5@localhost", "secret", nodeOptions5)
	if e1 != nil {
		t.Fatal(e1)
	}
	node2, e2 := ergo.StartNode("node2Handshake5@localhost", "secret", nodeOptions5)
	if e2 != nil {
		t.Fatal(e2)
	}
	node3, e3 := ergo.StartNode("node3Handshake5@localhost", "secret", nodeOptions5)
	if e3 != nil {
		t.Fatal(e3)
	}
	node4, e4 := ergo.StartNode("node4Handshake6@localhost", "secret", nodeOptions6)
	if e4 != nil {
		t.Fatal(e4)
	}
	// node5, _ := ergo.StartNode("node5Handshake6@localhost", "secret", nodeOptions6)
	// node6, _ := ergo.StartNode("node6Handshake5@localhost", "secret", nodeOptions5)
	node7, e7 := ergo.StartNode("node7Handshake6@localhost", "secret", nodeOptions6)
	if e7 != nil {
		t.Fatal(e7)
	}
	node8, e8 := ergo.StartNode("node8Handshake6@localhost", "secret", nodeOptions6)
	if e8 != nil {
		t.Fatal(e8)
	}
	node9, e9 := ergo.StartNode("node9Handshake5@localhost", "secret", nodeOptions5WithTLS)
	if e9 != nil {
		t.Fatal(e9)
	}
	node10, e10 := ergo.StartNode("node10Handshake5@localhost", "secret", nodeOptions5WithTLS)
	if e10 != nil {
		t.Fatal(e10)
	}
	node11, e11 := ergo.StartNode("node11Handshake5@localhost", "secret", nodeOptions5WithTLS)
	if e11 != nil {
		t.Fatal(e11)
	}
	node12, e12 := ergo.StartNode("node12Handshake6@localhost", "secret", nodeOptions6WithTLS)
	if e12 != nil {
		t.Fatal(e12)
	}
	// node13, _ := ergo.StartNode("node13Handshake6@localhost", "secret", nodeOptions6WithTLS)
	// node14, _ := ergo.StartNode("node14Handshake5@localhost", "secret", nodeOptions5WithTLS)
	node15, e15 := ergo.StartNode("node15Handshake6@localhost", "secret", nodeOptions6WithTLS)
	if e15 != nil {
		t.Fatal(e15)
	}
	node16, e16 := ergo.StartNode("node16Handshake6@localhost", "secret", nodeOptions6WithTLS)
	if e16 != nil {
		t.Fatal(e16)
	}

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

	var pA, pB gen.Process
	var e error
	var result etf.Term
	for i := range nodes {
		pair := nodes[i]
		fmt.Printf("    %s: ", pair.name)
		pA, e = pair.nodeA.Spawn("", gen.ProcessOptions{}, hgs)
		if e != nil {
			t.Fatal(e)
		}
		pB, e = pair.nodeB.Spawn("", gen.ProcessOptions{}, hgs)
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
	node1, _ := ergo.StartNode("node1remoteSpawn@localhost", "secret", node.Options{})
	node2, _ := ergo.StartNode("node2remoteSpawn@localhost", "secret", node.Options{})
	defer node1.Stop()
	defer node2.Stop()

	node2.ProvideRemoteSpawn("remote", &handshakeGenServer{})
	process, err := node1.Spawn("gs1", gen.ProcessOptions{}, &handshakeGenServer{})
	if err != nil {
		t.Fatal(err)
	}

	opts := gen.RemoteSpawnOptions{
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
	if err != node.ErrTaken {
		t.Fatal(err)
	}
	fmt.Println("OK")
}

type benchGS struct {
	gen.Server
}

func (b *benchGS) HandleCall(process *gen.ServerProcess, from gen.ServerFrom, message etf.Term) (etf.Term, gen.ServerStatus) {
	return etf.Atom("ok"), gen.ServerStatusOK
}

func BenchmarkNodeSequential(b *testing.B) {

	node1name := fmt.Sprintf("nodeB1_%d@localhost", b.N)
	node2name := fmt.Sprintf("nodeB2_%d@localhost", b.N)
	node1, _ := ergo.StartNode(node1name, "bench", node.Options{DisableHeaderAtomCache: false})
	node2, _ := ergo.StartNode(node2name, "bench", node.Options{})

	bgs := &benchGS{}

	p1, e1 := node1.Spawn("", gen.ProcessOptions{}, bgs)
	p2, e2 := node2.Spawn("", gen.ProcessOptions{}, bgs)

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
	node1, _ := ergo.StartNode(node1name, "bench", node.Options{DisableHeaderAtomCache: true})

	bgs := &benchGS{}

	p1, e1 := node1.Spawn("", gen.ProcessOptions{}, bgs)
	p2, e2 := node1.Spawn("", gen.ProcessOptions{}, bgs)

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
	node1, _ := ergo.StartNode(node1name, "bench", node.Options{DisableHeaderAtomCache: false})
	node2, _ := ergo.StartNode(node2name, "bench", node.Options{})

	bgs := &benchGS{}

	p1, e1 := node1.Spawn("", gen.ProcessOptions{}, bgs)
	if e1 != nil {
		b.Fatal(e1)
	}
	p2, e2 := node2.Spawn("", gen.ProcessOptions{}, bgs)
	if e2 != nil {
		b.Fatal(e2)
	}

	if _, e := p1.Call(p2.Self(), "hi"); e != nil {
		b.Fatal("single ping", e)
	}

	b.SetParallelism(15)
	b.RunParallel(func(pb *testing.PB) {
		p1, e1 := node1.Spawn("", gen.ProcessOptions{}, bgs)
		if e1 != nil {
			b.Fatal(e1)
		}
		p2, e2 := node2.Spawn("", gen.ProcessOptions{}, bgs)
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
	node1, _ := ergo.StartNode(node1name, "bench", node.Options{DisableHeaderAtomCache: true})

	bgs := &benchGS{}

	p1, e1 := node1.Spawn("", gen.ProcessOptions{}, bgs)
	if e1 != nil {
		b.Fatal(e1)
	}
	p2, e2 := node1.Spawn("", gen.ProcessOptions{}, bgs)
	if e2 != nil {
		b.Fatal(e2)
	}

	if _, e := p1.Call(p2.Self(), "hi"); e != nil {
		b.Fatal("single ping", e)
	}
	b.SetParallelism(15)
	b.RunParallel(func(pb *testing.PB) {
		p1, e1 := node1.Spawn("", gen.ProcessOptions{}, bgs)
		if e1 != nil {
			b.Fatal(e1)
		}
		p2, e2 := node1.Spawn("", gen.ProcessOptions{}, bgs)
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
