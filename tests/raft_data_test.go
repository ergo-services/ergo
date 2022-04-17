//go:build !manual

package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/ergo-services/ergo/gen"
)

func TestRaftData(t *testing.T) {
	nodes, rafts, leaderSerial := startRaftCluster(6, gen.RaftQuorumState5)

	ref, err := rafts[0].Append("asdfkey", "asdfvalue")
	if err != nil {
		t.Fatal(err)
	}
	q := rafts[0].Quorum()
	peers := rafts[0].Peers()
	fmt.Println("quorum", q)
	fmt.Println(rafts[0].Self(), "peers", peers)
	fmt.Println("send append", ref, "current serial", leaderSerial)

	time.Sleep(10 * time.Second)
	for _, node := range nodes {
		node.Stop()
	}
}
