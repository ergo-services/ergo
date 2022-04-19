//go:build manual
// +build manual

// to run this test:
//   go test -run TestRaft -ergo.norecover -tags manual
//
// enable debug printing in the gen/raft.go
//   quorum building debuging: %s/\/\/ QUODBG //
//   leader election debuging: %s/\/\/ LDRDBG //
//   heartbeat debuging:       %s/\/\/ HRTDBG //

package tests

import (
	"fmt"
	"math/rand"
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
		options.Peers = args[0].([]gen.ProcessID)
		options.Serial = uint64(rand.Intn(10))
	}

	fmt.Println(process.Self(), process.Name(), " ----------", options.Serial)
	return options, gen.RaftStatusOK
}

func (tr *testRaft) HandleQuorum(process *gen.RaftProcess, q *gen.RaftQuorum) gen.RaftStatus {
	if q == nil {
		fmt.Println("QQQ quorum", process.Name(), "state: NONE")
		return gen.RaftStatusOK
	} else {
		fmt.Println("QQQ quorum", process.Name(), "state:", q.State, q.Member, q.Peers)
	}
	if sent, _ := process.State.(int); sent != 1 {
		process.SendAfter(process.Self(), "ok", 7*time.Second)
		process.State = 1
	}
	//tr.res <- qs
	return gen.RaftStatusOK
}

func (tr *testRaft) HandleLeader(process *gen.RaftProcess, leader *gen.RaftLeader) gen.RaftStatus {
	fmt.Println("LLL leader", process.Name(), leader)
	return gen.RaftStatusOK
}

func (tr *testRaft) HandleAppend(process *gen.RaftProcess, ref etf.Ref, serial uint64, key string, value etf.Term) gen.RaftStatus {
	fmt.Println("AAA append", ref, serial, value)
	return gen.RaftStatusOK
}

func (tr *testRaft) HandleGet(process *gen.RaftProcess, serial uint64) (string, etf.Term, gen.RaftStatus) {
	fmt.Println("GGG get", process.Name(), serial)
	return "", nil, gen.RaftStatusOK
}

func (tr *testRaft) HandleRaftInfo(process *gen.RaftProcess, message etf.Term) gen.ServerStatus {
	q := process.Quorum()
	if q == nil {
		fmt.Println("III info", process.Name(), "state: NONE", "message:", message)
	} else {
		fmt.Println("III info", process.Name(), "Q:", q.State, q.Member, "", process.Leader(), "message:", message)
	}
	if l := process.Leader(); l != nil && l.Leader == process.Self() {
		fmt.Println("III i'm leader. freeze", process.Self())
		time.Sleep(35 * time.Second)
	}
	process.State = 0
	return gen.ServerStatusOK
}

func TestRaftLeader(t *testing.T) {
	fmt.Printf("\n=== Test GenRaft\n")
	var N int = 4

	fmt.Printf("Starting %d nodes: nodeGenRaftXX@localhost...", N)

	nodes := make([]node.Node, N)
	for i := range nodes {
		name := fmt.Sprintf("nodeGenRaft%02d@localhost", i)
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
		name := fmt.Sprintf("raft%02d", i+1)
		if i == 0 {
			args = nil
		} else {
			peer.Node = nodes[i-1].Name()
			peer.Name = rafts[i-1].Name()
			peers := []gen.ProcessID{peer}
			args = []etf.Term{peers}
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
		//time.Sleep(300 * time.Millisecond)
	}

	time.Sleep(50 * time.Second)

}
