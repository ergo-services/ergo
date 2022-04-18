//go:build !manual

package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
)

// F - follower
// M - quorum member
// L - leader
// Q - quorum (all members)
// cases:
// 1. F -> M -> L -> Q ... broadcast
// 2. F -> L -> Q ... broadcast
// 3. M -> L - Q ... broadcast
// 4. L -> Q ... broadcast

func TestRaftData(t *testing.T) {
	fmt.Printf("\n=== Test GenRaft - append/get data\n")
	server := &testRaft{
		n:      6,
		qstate: gen.RaftQuorumState5,
	}
	nodes, rafts, leaderSerial := startRaftCluster("get-data", server)

	fmt.Printf("    append on a follower (send to the quorum member and forward to the leader: ")
	fmt.Println("leaderSerial", leaderSerial)
	for _, raft := range rafts {
		q := raft.Quorum()
		// find the follower
		if q.Member == true {
			continue
		}

		// cases 1 and 2 - send to the quorum member.
		// the follower isn't able to send it to the leader (case 2)
		// since it has no info about the quorum leader
		ref, err := raft.Append("asdfkey", "asdfvalue")
		if err != nil {
			t.Fatal(err)
		}
		checkAppend(t, server, ref, rafts, leaderSerial)
		break
	}
	fmt.Printf("OK")
	fmt.Printf("    append on a quorum member (send to the leader): ")
	for _, raft := range rafts {
		q := raft.Quorum()
		// find the quorum member
		if q.Member == false {
			continue
		}

		// case 3 - quorum member sends append to the leader
		ref, err := raft.Append("asdfkey", "asdfvalue")
		if err != nil {
			t.Fatal(err)
		}
		checkAppend(t, server, ref, rafts, leaderSerial)
		break
	}
	fmt.Printf("OK")
	fmt.Printf("    append on a leader: ")
	for _, raft := range rafts {
		l := raft.Leader()
		// finde the quorum leader
		if l == nil || l.Leader != raft.Self() {
			continue
		}

		// case 4 - leader makes append
		ref, err := raft.Append("asdfkey", "asdfvalue")
		if err != nil {
			t.Fatal(err)
		}
		checkAppend(t, server, ref, rafts, leaderSerial)
		break
	}
	fmt.Printf("OK")

	time.Sleep(15 * time.Second)
	fmt.Println("!!!stop nodes")
	for _, node := range nodes {
		node.Stop()
	}
}

func checkAppend(t *testing.T, server *testRaft, ref etf.Ref, rafts []*gen.RaftProcess, serial uint64) {
	for {
		select {
		case result := <-server.a:
			if result.serial != serial+1 {
				t.Fatalf("wrong serial %d (must be %d)", result.serial, serial+1)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("append timeout")

		}
	}

}
