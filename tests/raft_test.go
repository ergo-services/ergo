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

	data = map[string]dataValueSerial{
		"key0": dataValueSerial{"value0", 0},
		"key1": dataValueSerial{"value1", 1},
		"key2": dataValueSerial{"value2", 2},
		"key3": dataValueSerial{"value3", 3},
		"key4": dataValueSerial{"value4", 4},
		"key5": dataValueSerial{"value5", 5},
		"key6": dataValueSerial{"value6", 6},
		"key7": dataValueSerial{"value7", 7},
		"key8": dataValueSerial{"value8", 8},
		"key9": dataValueSerial{"value9", 9},
	}
	keySerials = []string{
		"key0",
		"key1",
		"key2",
		"key3",
		"key4",
		"key5",
		"key6",
		"key7",
		"key8",
		"key9",
	}
)

type dataValueSerial struct {
	value  string
	serial uint64
}

type testRaft struct {
	gen.Raft
	n      int
	qstate gen.RaftQuorumState
	p      chan *gen.RaftProcess // Init
	q      chan *gen.RaftQuorum  // HandleQuorum
	l      chan *gen.RaftLeader  // HandleLeader
	a      chan raftResult       // HandleAppend
	s      chan raftResult       // HandleSerial
}

type raftResult struct {
	process *gen.RaftProcess
	ref     etf.Ref
	serial  uint64
	key     string
	value   etf.Term
}

type raftArgs struct {
	peers  []gen.ProcessID
	serial uint64
}
type raftState struct {
	data    map[string]dataValueSerial
	serials []string
}

func (tr *testRaft) InitRaft(process *gen.RaftProcess, args ...etf.Term) (gen.RaftOptions, error) {
	var options gen.RaftOptions
	ra := args[0].(raftArgs)
	options.Peers = ra.peers
	options.Serial = ra.serial

	state := &raftState{
		data: make(map[string]dataValueSerial),
	}
	for i := 0; i < int(ra.serial)+1; i++ {
		key := keySerials[i]
		state.data[key] = data[key]
		state.serials = append(state.serials, key)
	}
	process.State = state
	tr.p <- process

	return options, gen.RaftStatusOK
}

func (tr *testRaft) HandleQuorum(process *gen.RaftProcess, quorum *gen.RaftQuorum) gen.RaftStatus {
	//fmt.Println(process.Self(), "QQQ", quorum)
	if quorum != nil {
		tr.q <- quorum
	}
	return gen.RaftStatusOK
}

func (tr *testRaft) HandleLeader(process *gen.RaftProcess, leader *gen.RaftLeader) gen.RaftStatus {
	//fmt.Println(process.Self(), "LLL", leader)
	// leader elected within a quorum
	q := process.Quorum()
	if q == nil {
		return gen.RaftStatusOK
	}
	if leader != nil && q.State == tr.qstate {
		tr.l <- leader
	}

	return gen.RaftStatusOK
}

func (tr *testRaft) HandleAppend(process *gen.RaftProcess, ref etf.Ref, serial uint64, key string, value etf.Term) gen.RaftStatus {
	//fmt.Println(process.Self(), "HANDLE APPEND member:", process.Quorum().Member, "append", ref, serial, key, value)

	result := raftResult{
		process: process,
		ref:     ref,
		serial:  serial,
		key:     key,
		value:   value,
	}
	tr.a <- result
	return gen.RaftStatusOK
}

func (tr *testRaft) HandleGet(process *gen.RaftProcess, serial uint64) (string, etf.Term, gen.RaftStatus) {
	var key string
	//fmt.Println(process.Self(), "HANDLE GET member:", process.Quorum().Member, "get", serial)

	state := process.State.(*raftState)
	if len(state.serials) < int(serial) {
		//	fmt.Println(process.Self(), "NO DATA for", serial)
		return key, nil, gen.RaftStatusOK
	}
	key = state.serials[int(serial)]
	data := state.data[key]
	return key, data.value, gen.RaftStatusOK
}

func (tr *testRaft) HandleSerial(process *gen.RaftProcess, ref etf.Ref, serial uint64, key string, value etf.Term) gen.RaftStatus {
	//fmt.Println(process.Self(), "HANDLE SERIAL member:", process.Quorum().Member, "append", ref, serial, key, value)
	result := raftResult{
		process: process,
		ref:     ref,
		serial:  serial,
		key:     key,
		value:   value,
	}
	s := process.Serial()
	if s != serial {
		fmt.Println(process.Self(), "ERROR: disordered serial request")
		tr.s <- raftResult{}
		return gen.RaftStatusOK
	}
	state := process.State.(*raftState)
	state.serials = append(state.serials, key)
	state.data[key] = dataValueSerial{
		value:  value.(string),
		serial: serial,
	}
	tr.s <- result
	return gen.RaftStatusOK
}
func (tr *testRaft) HandleCancel(process *gen.RaftProcess, ref etf.Ref, reason string) gen.RaftStatus {
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

		server := &testRaft{
			n:      c.n,
			qstate: c.state,
		}
		// start distributed raft processes and wait until
		// they build a quorum and elect their leader
		nodes, rafts, leaderSerial := startRaftCluster("append", server)
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

func startRaftCluster(name string, server *testRaft) ([]node.Node, []*gen.RaftProcess, uint64) {
	nodes := make([]node.Node, server.n)
	for i := range nodes {
		name := fmt.Sprintf("nodeGenRaft-%s-cluster-%02dNode%02d@localhost", name, server.n, i)
		node, err := ergo.StartNode(name, "cookies", node.Options{})
		if err != nil {
			panic(err)
		}
		nodes[i] = node
	}

	processes := make([]gen.Process, server.n)
	server.p = make(chan *gen.RaftProcess, 1000)
	server.q = make(chan *gen.RaftQuorum, 1000)
	server.l = make(chan *gen.RaftLeader, 1000)
	server.a = make(chan raftResult, 1000)
	server.s = make(chan raftResult, 1000)
	leaderSerial := uint64(0)
	var peer gen.ProcessID
	for i := range processes {
		name := fmt.Sprintf("raft%02d", i+1)
		args := raftArgs{
			serial: uint64(rand.Intn(9)),
		}
		if args.serial > leaderSerial {
			leaderSerial = args.serial
		}
		if i > 0 {
			peer.Node = nodes[i-1].Name()
			peer.Name = processes[i-1].Name()
			args.peers = []gen.ProcessID{peer}
		}
		p, err := nodes[i].Spawn(name, gen.ProcessOptions{}, server, args)
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
	for {
		select {
		case p := <-server.p:
			rafts = append(rafts, p)

			if len(rafts) < server.n {
				continue
			}

			if server.n == 2 {
				// no leader, no quorum
				return nodes, rafts, leaderSerial
			}
			continue

		case q := <-server.q:

			if q.State != server.qstate {
				continue
			}

			resultsQ++
			if resultsQ < int(server.qstate) {
				continue
			}

			if resultsL < int(server.qstate) {
				continue
			}
			// all quorum members are received leader election result
			return nodes, rafts, leaderSerial

		case l := <-server.l:
			if l.State != server.qstate {
				continue
			}

			resultsL++
			if resultsL < int(server.qstate) {
				continue
			}

			if resultsQ < server.n {
				continue
			}

			// all quorum members are received leader election result
			return nodes, rafts, leaderSerial

		case <-time.After(30 * time.Second):
			panic("can't start raft cluster")
		}
	}
}
