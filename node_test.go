package ergo

import (
	"fmt"
	"net"
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

func BenchmarkNode(b *testing.B) {

	node1name := fmt.Sprintf("nodeB1_%d@localhost", b.N)
	node2name := fmt.Sprintf("nodeB2_%d@localhost", b.N)
	node1 := CreateNode(node1name, "bench", NodeOptions{DisableHeaderAtomCache: true})
	node2 := CreateNode(node2name, "bench", NodeOptions{})

	bgs := &benchGS{}

	p1, e1 := node1.Spawn("", ProcessOptions{}, bgs)
	p2, e2 := node2.Spawn("", ProcessOptions{}, bgs)

	fmt.Println("pids: ", p1.Self(), p2.Self())

	if e1 != nil {
		b.Fatal(e1)
	}
	if e2 != nil {
		b.Fatal(e2)
	}

	for i := 0; i < 10000; i++ {
		if _, e := p1.Call(p2.Self(), "hi"); e != nil {
			b.Fatal("single loop", e, i)
		}
	}
	fmt.Println("ok")
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
	node1 := CreateNode(node1name, "bench", NodeOptions{DisableHeaderAtomCache: true})
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

func benchCases() []benchCase {
	return []benchCase{
		benchCase{"number", 12345},
		benchCase{"string", "hello world"},
		benchCase{"tuple (PID)", etf.Pid{Node: "node@localhost", ID: 1, Serial: 1000, Creation: byte(0)}},
		//		benchCase{"binary 1MB", make([]byte, 1024*1024)},
	}
}
