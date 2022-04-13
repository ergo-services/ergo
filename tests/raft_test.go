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
		testCaseRaft{n: 8, name: qlf, state: gen.RaftQuorumState7},
		testCaseRaft{n: 9, name: ql, state: gen.RaftQuorumState9},
		testCaseRaft{n: 10, name: qlf, state: gen.RaftQuorumState9},
		testCaseRaft{n: 11, name: ql, state: gen.RaftQuorumState11},
		testCaseRaft{n: 12, name: qlf, state: gen.RaftQuorumState11},
		testCaseRaft{n: 15, name: qlf, state: gen.RaftQuorumState11},
		//testCaseRaft{n: 25, name: qlf, state: gen.RaftQuorumState11},
	}
)

type testRaft struct {
	gen.Raft
	peers  []gen.ProcessID
	serial uint64
	qstate gen.RaftQuorumState
	res    chan *gen.RaftProcess
}

func (tr *testRaft) InitRaft(process *gen.RaftProcess, args ...etf.Term) (gen.RaftOptions, error) {
	var options gen.RaftOptions
	options.Peers = tr.peers
	options.Serial = tr.serial
	tr.res <- process

	return options, gen.RaftStatusOK
}

func (tr *testRaft) HandleQuorum(process *gen.RaftProcess, quorum *gen.RaftQuorum) gen.RaftStatus {
	//fmt.Println(process.Self(), "QQQ", quorum)
	return gen.RaftStatusOK
}

func (tr *testRaft) HandleLeader(process *gen.RaftProcess, leader *gen.RaftLeader) gen.RaftStatus {
	//fmt.Println(process.Self(), "LLL", leader)
	if leader != nil && leader.Leader == process.Self() {
		// leader elected
		if process.Quorum().State == tr.qstate {
			tr.res <- nil
		}

	}
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
		nodes, rafts, leaderSerial := startCluster(c.n, c.state)
		fmt.Println("OK", len(nodes), len(rafts), leaderSerial)

		// stop cluster
		for _, node := range nodes {
			node.Stop()
		}
	}

}

func startCluster(n int, state gen.RaftQuorumState) ([]node.Node, []*gen.RaftProcess, uint64) {
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
	result := make(chan *gen.RaftProcess, 1000)
	leaderSerial := uint64(0)
	var peer gen.ProcessID
	for i := range processes {
		name := fmt.Sprintf("raft%02d", i+1)
		tr := &testRaft{
			res:    result,
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
wait:
	select {
	case r := <-result:
		if r != nil {
			rafts = append(rafts, r)
		}
		if len(rafts) < n {
			goto wait
		}

		if n == 2 {
			// no leader, no quorum
			return nodes, rafts, leaderSerial
		}

		if r == nil {
			// leader elected
			return nodes, rafts, leaderSerial
		}
		goto wait

	case <-time.After(30 * time.Second):
		panic("can't start raft cluster")
	}
}
