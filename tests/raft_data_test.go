//go:build !manual

package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/ergo-services/ergo/gen"
)

// cases (append)
// F - follower
// M - quorum member
// L - leader
// Q - quorum (all members)
// 1. F -> M -> L -> Q ... broadcast
// 2. F -> L -> Q ... broadcast
// 3. M -> L - Q ... broadcast
// 4. L -> Q ... broadcast

func TestRaftData(t *testing.T) {
	nodes, rafts, leaderSerial := startRaftCluster(6, gen.RaftQuorumState5)

	fmt.Println("leaderSerial", leaderSerial)
	// case 1. F -> M
	for _, raft := range rafts {
		q := raft.Quorum()
		if q.Member == true {
			continue
		}
		fmt.Println(raft.Self(), "!!!! peers", raft.Peers())
		fmt.Println(raft.Self(), "!!!! quorum", raft.Quorum())
		ref, err := raft.Append("asdfkey", "asdfvalue")
		fmt.Println(raft.Self(), "!!!! sent append", ref, "current serial", leaderSerial)
		if err != nil {
			t.Fatal(err)
		}
		break
	}

	time.Sleep(15 * time.Second)
	fmt.Println("!!!stop nodes")
	for _, node := range nodes {
		node.Stop()
	}
}
