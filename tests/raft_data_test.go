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
//
// append cases:
// 1. F -> M -> L -> Q ... broadcast
// 2. F -> L -> Q ... broadcast
// 3. M -> L - Q ... broadcast
// 4. L -> Q ... broadcast
//

func TestRaftAppend(t *testing.T) {
	fmt.Printf("\n=== Test GenRaft - append data\n")
	server := &testRaft{
		n:      6,
		qstate: gen.RaftQuorumState5,
	}
	nodes, rafts, leaderSerial := startRaftCluster("append", server)

	fmt.Printf("    append on a follower (send to the quorum member and forward to the leader: ")
	for _, raft := range rafts {
		q := raft.Quorum()
		// find the follower
		if q.Member == true {
			continue
		}

		// cases 1 and 2 - send to the quorum member.
		// the follower isn't able to send it to the leader (case 2)
		// since it has no info about the quorum leader (quorum member forwards it
		// to the leader under the hood)
		ref, err := raft.Append("asdfkey", "asdfvalue")
		if err != nil {
			t.Fatal(err)
		}
		leaderSerial = checkAppend(t, server, ref, rafts, leaderSerial)
		break
	}
	fmt.Println("OK")
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
		leaderSerial = checkAppend(t, server, ref, rafts, leaderSerial)
		break
	}
	fmt.Println("OK")
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
		leaderSerial = checkAppend(t, server, ref, rafts, leaderSerial)
		break
	}
	fmt.Println("OK")

	for _, node := range nodes {
		node.Stop()
	}
}

// get serial case: run through the raft process and get all the data to achieve
// the same serial
func TestRaftGet(t *testing.T) {
	fmt.Printf("\n=== Test GenRaft - get data\n")
	server := &testRaft{
		n:      6,
		qstate: gen.RaftQuorumState5,
	}
	nodes, rafts, leaderSerial := startRaftCluster("get", server)
	for _, raft := range rafts {
		fmt.Println("    started raft process", raft.Self(), "with serial", raft.Serial())
	}

	for _, raft := range rafts {
		_, err := raft.Get(12341234)
		if err != gen.ErrRaftNoSerial {
			t.Fatal("must be ErrRaftNoSerial here")
		}

		if raft.Serial() == leaderSerial {
			fmt.Println("    raft process", raft.Self(), "has already latest serial. skip it")
			continue
		}

		fmt.Printf("    get serials on %s to reach the leader's serial (from %d to %d): ", raft.Self(), raft.Serial()+1, leaderSerial)
		serial := raft.Serial()
		gotFrom := []etf.Pid{}
		for i := serial; i < leaderSerial; i++ {
			ref, err := raft.Get(i + 1)
			if err != nil {
				t.Fatal(err)
			}
			result := waitGetRef(t, server, ref)

			// compare with the original
			original := data[result.key]
			if original.serial != result.serial {
				t.Fatal("wrong serial")
			}
			if original.value != result.value {
				t.Fatal("wrong value")
			}
			// check internal data
			internal := raft.State.(*raftState)
			if internal.serials[result.serial] != result.key {
				t.Fatal("serial mismatch the result key")
			}
			d, exist := internal.data[result.key]
			if exist == false {
				t.Fatal("internal data hasn't been updated")
			}
			if d.serial != result.serial {
				t.Fatal("intenal data serial mismatch")
			}
			if d.value != result.value {
				t.Fatal("intarnal data value mismatch")
			}
			gotFrom = append(gotFrom, result.process.Self())
		}
		fmt.Println("OK")
		fmt.Println("         got from:", gotFrom, ")")
	}

	//fmt.Println("-----------")
	//for _, raft := range rafts {
	//	s := raft.State.(*raftState)
	//	fmt.Println(raft.Self(), "INTERNAL", s)
	//}

	for _, node := range nodes {
		node.Stop()
	}
}

func waitGetRef(t *testing.T, server *testRaft, ref etf.Ref) raftResult {
	var result raftResult
	select {
	case result = <-server.s:
		if result.ref != ref {
			t.Fatal("wrong ref")
		}
		return result
	case <-time.After(30 * time.Second):
		t.Fatal("get timeout")
	}
	return result
}

func checkAppend(t *testing.T, server *testRaft, ref etf.Ref, rafts []*gen.RaftProcess, serial uint64) uint64 {
	appends := 0
	for {
		select {
		case result := <-server.a:
			if result.serial != serial+1 {
				t.Fatalf("wrong serial %d (must be %d)", result.serial, serial+1)
			}
			appends++
			//fmt.Println("got append on ", result.process.Self(), "total appends", appends)
			if appends != len(rafts) {
				continue
			}
			// check serials
			for _, r := range rafts {
				s := r.Serial()
				if s != serial+1 {
					t.Fatalf("wrong serial %d on %s", s, r.Self())
				}
			}
			return serial + 1
		case <-time.After(30 * time.Second):
			t.Fatal("append timeout")

		}
	}

}
