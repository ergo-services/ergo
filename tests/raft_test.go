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

	fmt.Printf("Starting 13 nodes: nodeGenRaftXX@localhost...")

	var nodes [13]node.Node
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

	var rafts [13]gen.Process
	var results [13]chan interface{}
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
		rafts[i] = raft
	}

	time.Sleep(10 * time.Second)

}
