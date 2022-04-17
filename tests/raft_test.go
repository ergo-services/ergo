//go:build !manual

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

type testCaseRaft struct {
	n     int
	state gen.RaftQuorumState
	name  string
}

var (
	ql    string = "quorum of %2d members with 1 leader: "
	qlf   string = "quorum of %2d members with 1 leader + %d follower(s): "
	cases        = []testCaseRaft{
		testCaseRaft{n: 2, name: "no quorum, no leader: "},
		testCaseRaft{n: 3, name: ql, state: gen.RaftQuorumState3},
		testCaseRaft{n: 4, name: qlf, state: gen.RaftQuorumState3},
		testCaseRaft{n: 5, name: ql, state: gen.RaftQuorumState5},
		testCaseRaft{n: 6, name: qlf, state: gen.RaftQuorumState5},
		testCaseRaft{n: 7, name: ql, state: gen.RaftQuorumState7},

		//
		// cases below are work well, but quorum building takes too long some time.
		//testCaseRaft{n: 8, name: qlf, state: gen.RaftQuorumState7},
		//testCaseRaft{n: 9, name: ql, state: gen.RaftQuorumState9},
		//testCaseRaft{n: 10, name: qlf, state: gen.RaftQuorumState9},
		//testCaseRaft{n: 11, name: ql, state: gen.RaftQuorumState11},
		//testCaseRaft{n: 12, name: qlf, state: gen.RaftQuorumState11},
		//testCaseRaft{n: 15, name: qlf, state: gen.RaftQuorumState11},
		//testCaseRaft{n: 25, name: qlf, state: gen.RaftQuorumState11},
	}
)

type testRaft struct {
	gen.Raft
	peers  []gen.ProcessID
	serial uint64
	qstate gen.RaftQuorumState
	p      chan *gen.RaftProcess
	q      chan *gen.RaftQuorum
	l      chan *gen.RaftLeader
}

func (tr *testRaft) InitRaft(process *gen.RaftProcess, args ...etf.Term) (gen.RaftOptions, error) {
	var options gen.RaftOptions
	options.Peers = tr.peers
	options.Serial = tr.serial
	tr.p <- process

	return options, gen.RaftStatusOK
}

func (tr *testRaft) HandleQuorum(process *gen.RaftProcess, quorum *gen.RaftQuorum) gen.RaftStatus {
	fmt.Println(process.Self(), "QQQ", quorum)
	if quorum != nil {
		tr.q <- quorum
	}
	return gen.RaftStatusOK
}

func (tr *testRaft) HandleLeader(process *gen.RaftProcess, leader *gen.RaftLeader) gen.RaftStatus {
	fmt.Println(process.Self(), "LLL", leader)
	// leader elected within a quorum
	q := process.Quorum()
	if q == nil {
		return gen.RaftStatusOK
	}
	if leader != nil && q.State == tr.qstate {
		fmt.Println(process.Self(), "LLL1")
		tr.l <- leader
	} else {
		fmt.Println(process.Self(), "LLL2")
	}

	return gen.RaftStatusOK
}

func (tr *testRaft) HandleAppend(process *gen.RaftProcess, ref etf.Ref, serial uint64, key string, value etf.Term) gen.RaftStatus {

	fmt.Println(process.Self(), "member:", process.Quorum().Member, "append", ref, serial, key, value)
	return gen.RaftStatusOK
}

func (tr *testRaft) HandleGet(process *gen.RaftProcess, serial uint64) (string, etf.Term, gen.RaftStatus) {
	fmt.Println("GGG get", process.Name(), serial)
	return "", nil, gen.RaftStatusOK
}

func (tr *testRaft) HandleCancel(process *gen.RaftProcess, ref etf.Ref, reason string) gen.RaftStatus {
	fmt.Println("CCC cancel", ref, reason)
	return gen.RaftStatusOK
}

func (tr *testRaft) HandleRaftInfo(process *gen.RaftProcess, message etf.Term) gen.ServerStatus {
	return gen.ServerStatusOK
}

func TestRaftLeader(t *testing.T) {
	fmt.Printf("\n=== Test GenRaft - build quorum, leader election\n")
	for _, c := range cases {
		fmt.Printf("    cluster with %2d distributed raft processes. ", c.n)
		if c.n == 2 {
			fmt.Printf(c.name)
		} else {
			f := c.n - int(c.state)
			if f == 0 {
				fmt.Printf(c.name, c.state)
			} else {
				fmt.Printf(c.name, c.state, c.n-int(c.state))
			}
		}
		// start distributed raft processes and wait until
		// they build a quorum and elect their leader
		nodes, rafts, leaderSerial := startRaftCluster(c.n, c.state)
		ok := true
		if c.n > 2 {
			ok = false
			for _, raft := range rafts {
				q := raft.Quorum()
				if q == nil {
					continue
				}
				if q.Member == false {
					continue
				}

				l := raft.Leader()
				if l == nil {
					continue
				}
				if l.Serial != leaderSerial {
					t.Fatal("wrong leader serial")
				}
				ok = true
				break
			}
		}
		if ok == false {
			t.Fatal("no quorum or leader found")
		}
		fmt.Println("OK")
		// stop cluster
		for _, node := range nodes {
			node.Stop()
		}
	}

}

func startRaftCluster(n int, state gen.RaftQuorumState) ([]node.Node, []*gen.RaftProcess, uint64) {
	nodes := make([]node.Node, n)
	for i := range nodes {
		name := fmt.Sprintf("nodeGenRaftCluster%02dNode%02d@localhost", n, i)
		node, err := ergo.StartNode(name, "cookies", node.Options{})
		if err != nil {
			panic(err)
		}
		nodes[i] = node
	}

	processes := make([]gen.Process, n)
	resultP := make(chan *gen.RaftProcess, 1000)
	resultQ := make(chan *gen.RaftQuorum, 1000)
	resultL := make(chan *gen.RaftLeader, 1000)
	leaderSerial := uint64(0)
	var peer gen.ProcessID
	for i := range processes {
		name := fmt.Sprintf("raft%02d", i+1)
		tr := &testRaft{
			p:      resultP,
			q:      resultQ,
			l:      resultL,
			serial: uint64(rand.Intn(10)),
			qstate: state,
		}
		if tr.serial > leaderSerial {
			leaderSerial = tr.serial
		}
		if i > 0 {
			peer.Node = nodes[i-1].Name()
			peer.Name = processes[i-1].Name()
			tr.peers = []gen.ProcessID{peer}
		}
		p, err := nodes[i].Spawn(name, gen.ProcessOptions{}, tr)
		if err != nil {
			panic(err)
		}
		processes[i] = p
	}

	rafts := []*gen.RaftProcess{}
	// how many results with 'leader' should be awaiting
	resultsL := 0
	// how many results with 'quorum' should be awaiting
	resultsQ := 0
wait:
	select {
	case p := <-resultP:
		rafts = append(rafts, p)

		if len(rafts) < n {
			goto wait
		}

		if n == 2 {
			// no leader, no quorum
			return nodes, rafts, leaderSerial
		}
		goto wait

	case q := <-resultQ:

		if q.State != state {
			goto wait
		}

		resultsQ++
		if resultsQ < int(state) {
			goto wait
		}

		if resultsL < int(state) {
			goto wait
		}
		// all quorum members are received leader election result
		return nodes, rafts, leaderSerial

	case l := <-resultL:
		if l.State != state {
			goto wait
		}

		resultsL++
		if resultsL < int(state) {
			goto wait
		}

		if resultsQ < n {
			goto wait
		}

		// all quorum members are received leader election result
		return nodes, rafts, leaderSerial

	case <-time.After(30 * time.Second):
		panic("can't start raft cluster")
	}

	return nil, nil, 0
}
