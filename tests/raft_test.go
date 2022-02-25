package tests

import (
	"fmt"
	"testing"

	"github.com/ergo-services/ergo"
	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/node"
)

type testRaft struct {
	gen.Raft
}

type testRaftState struct{}

func (tr *testRaft) InitRaft(process *gen.RaftProcess, args ...etf.Term) (gen.RaftOptions, error) {
	var options gen.RaftOptions
	process.State = &testRaftState{}

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

}
