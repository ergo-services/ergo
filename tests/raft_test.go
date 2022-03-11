package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/ergo-services/ergo"
	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/node"
)

type testRaft struct {
	gen.Raft
}

func (tr *testRaft) InitRaft(process *gen.RaftProcess, args ...etf.Term) (gen.RaftOptions, error) {
	var options gen.RaftOptions
	if len(args) > 0 {
		options.Peer = args[0].(gen.ProcessID)
	}

	return options, gen.RaftStatusOK
}

func TestRaft(t *testing.T) {
	fmt.Printf("\n=== Test GenRaft\n")
	fmt.Printf("Starting node: nodeGenRaft01@localhost...")

	node1, err := ergo.StartNode("nodeGenRaft01@localhost", "cookies", node.Options{})
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")
	defer node1.Stop()

	rgs1, err := node1.Spawn("raft01", gen.ProcessOptions{}, &testRaft{})
	if err != nil {
		t.Fatal(err)
	}

	fmt.Printf("Starting node: nodeGenRaft02@localhost...")
	node2, err := ergo.StartNode("nodeGenRaft02@localhost", "cookies", node.Options{})
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")
	defer node2.Stop()

	peer := gen.ProcessID{Node: node1.Name(), Name: rgs1.Name()}
	rgs2, err := node2.Spawn("raft02", gen.ProcessOptions{}, &testRaft{}, peer)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Printf("Starting node: nodeGenRaft03@localhost...")
	node3, err := ergo.StartNode("nodeGenRaft03@localhost", "cookies", node.Options{})
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("OK")
	defer node3.Stop()

	rgs3, err := node3.Spawn("raft03", gen.ProcessOptions{}, &testRaft{}, peer)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("peer", rgs1.Name(), rgs1.Self())
	fmt.Println("peer", rgs2.Name(), rgs2.Self())
	fmt.Println("peer", rgs3.Name(), rgs3.Self())
	time.Sleep(3 * time.Second)
	rgs2.Kill()
	time.Sleep(3 * time.Second)

}
