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
	res chan interface{}
}

func (tr *testRaft) InitRaft(process *gen.RaftProcess, args ...etf.Term) (gen.RaftOptions, error) {
	var options gen.RaftOptions
	if len(args) > 0 {
		options.Peer = args[0].(gen.ProcessID)
	}

	return options, gen.RaftStatusOK
}

func (tr *testRaft) HandleQuorumChange(process *gen.RaftProcess, qs gen.RaftQuorumState) gen.RaftStatus {
	fmt.Println("AAAA", process.Name(), qs)
	//tr.res <- qs
	return gen.RaftStatusOK
}

func TestRaft(t *testing.T) {
	fmt.Printf("\n=== Test GenRaft\n")
	var N int = 4

	fmt.Printf("Starting %d nodes: nodeGenRaftXX@localhost...", N)

	nodes := make([]node.Node, N)
	for i := range nodes {
		name := fmt.Sprintf("nodeGenRaft%0d@localhost", i)
		node, err := ergo.StartNode(name, "cookies", node.Options{})
		if err != nil {
			t.Fatal(err)
		}
		nodes[i] = node
	}

	defer func() {
		for i := range nodes {
			nodes[i].Stop()
		}
	}()
	fmt.Println("OK")

	rafts := make([]gen.Process, N)
	results := make([]chan interface{}, N)
	var args []etf.Term
	var peer gen.ProcessID
	for i := range rafts {
		name := fmt.Sprintf("raft%0d", i)
		if i == 0 {
			args = nil
		} else {
			peer.Node = nodes[i-1].Name()
			peer.Name = rafts[i-1].Name()
			args = []etf.Term{peer}
		}
		tr := &testRaft{
			res: make(chan interface{}, 2),
		}
		results[i] = tr.res
		raft, err := nodes[i].Spawn(name, gen.ProcessOptions{}, tr, args...)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Println(raft.Self(), raft.Name(), " - SSSSSSTARTED")
		rafts[i] = raft
	}

	time.Sleep(10 * time.Second)

}
