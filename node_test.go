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

	node := CreateNode("node@localhost", "cookies", opts)

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

	p, e := node.Spawn("", ProcessOptions{}, &GenServer{})

	if e != nil {
		t.Fatal(e)
	}
	// empty GenServer{} should die immediately (via panic and recovering)
	if node.IsProcessAlive(p.Self()) {
		t.Fatal("IsProcessAlive: expect 'false', but got 'true'")
	}

	gs1 := &testGenServer{
		err: make(chan error, 2),
	}
	p, e = node.Spawn("", ProcessOptions{}, gs1)
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

func (f *testFragmentationGS) Init(p *Process, args ...interface{}) interface{} {
	return nil
}

func (f *testFragmentationGS) HandleCall(from etf.Tuple, message etf.Term, state interface{}) (string, etf.Term, interface{}) {
	md5original := message.(etf.Tuple)[0].(string)
	blob := message.(etf.Tuple)[1].([]byte)

	result := etf.Atom("ok")
	md5 := fmt.Sprint(md5.Sum(blob))
	if !reflect.DeepEqual(md5original, md5) {
		result = etf.Atom("mismatch")
	}

	return "reply", result, state
}

func (f *testFragmentationGS) HandleCast(message etf.Term, state interface{}) (string, interface{}) {
	return "noreply", state
}

func (f *testFragmentationGS) HandleInfo(message etf.Term, state interface{}) (string, interface{}) {
	return "noreply", state
}

func (f *testFragmentationGS) Terminate(reason string, state interface{}) {

}

func TestNodeFragmentation(t *testing.T) {
	var wg sync.WaitGroup

	blob := make([]byte, 1024*1024)
	rand.Read(blob)
	md5 := fmt.Sprint(md5.Sum(blob))
	message := etf.Tuple{md5, blob}

	node1 := CreateNode("nodeT1Fragmentation@localhost", "secret", NodeOptions{})
	node2 := CreateNode("nodeT2Fragmentation@localhost", "secret", NodeOptions{})

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

	node1 := CreateNode("nodeT1AtomCache@localhost", "secret", NodeOptions{})
	node2 := CreateNode("nodeT2AtomCache@localhost", "secret", NodeOptions{})

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

type benchGS struct {
	GenServer
}

func (b *benchGS) Init(p *Process, args ...interface{}) interface{} {
	return nil
}

func (b *benchGS) HandleCall(from etf.Tuple, message etf.Term, state interface{}) (string, etf.Term, interface{}) {
	return "reply", etf.Atom("ok"), state
}

func (b *benchGS) HandleCast(message etf.Term, state interface{}) (string, interface{}) {
	return "noreply", state
}

func (b *benchGS) HandleInfo(message etf.Term, state interface{}) (string, interface{}) {
	return "noreply", state
}

func (b *benchGS) Terminate(reason string, state interface{}) {

}

func BenchmarkNodeSequential(b *testing.B) {

	node1name := fmt.Sprintf("nodeB1_%d@localhost", b.N)
	node2name := fmt.Sprintf("nodeB2_%d@localhost", b.N)
	node1 := CreateNode(node1name, "bench", NodeOptions{DisableHeaderAtomCache: false})
	node2 := CreateNode(node2name, "bench", NodeOptions{})

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
	node1 := CreateNode(node1name, "bench", NodeOptions{DisableHeaderAtomCache: true})

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
	node1 := CreateNode(node1name, "bench", NodeOptions{DisableHeaderAtomCache: false})
	node2 := CreateNode(node2name, "bench", NodeOptions{})

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
	node1 := CreateNode(node1name, "bench", NodeOptions{DisableHeaderAtomCache: true})

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
		benchCase{"tuple (PID)", etf.Pid{Node: "node@localhost", ID: 1, Serial: 1000, Creation: byte(0)}},
		benchCase{"binary 1MB", make([]byte, 1024*1024)},
	}
}
