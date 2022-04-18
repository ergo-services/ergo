package main

import (
	"fmt"
	"time"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
)

//
// Raft3
//
type Raft3 struct {
	gen.Raft
}

func (r *Raft3) InitRaft(process *gen.RaftProcess, args ...etf.Term) (gen.RaftOptions, error) {
	opts := gen.RaftOptions{
		Peers: []gen.ProcessID{
			gen.ProcessID{Name: "raft1", Node: "node1@localhost"},
		},
		Serial: 4,
	}

	fmt.Printf("Serial: %d PID: %s ...", opts.Serial, process.Self())

	return opts, nil
}
func (r *Raft3) HandleQuorum(process *gen.RaftProcess, quorum *gen.RaftQuorum) gen.RaftStatus {
	fmt.Println(process.Self(), "Quorum built - State:", quorum.State, "Quorum member:", quorum.Member)

	if quorum != nil && quorum.Member == false {
		process.CastAfter(process.Self(), messageAppend{}, 300*time.Millisecond)
	}
	return gen.RaftStatusOK
}
func (r *Raft3) HandleLeader(process *gen.RaftProcess, leader *gen.RaftLeader) gen.RaftStatus {
	if leader != nil && leader.Leader == process.Self() {
		fmt.Println(process.Self(), "I'm a leader of this quorum")
		return gen.RaftStatusOK
	}

	if leader != nil {
		fmt.Println(process.Self(), "Leader elected:", leader.Leader, "with serial", leader.Serial)
	}
	return gen.RaftStatusOK
}

func (r *Raft3) HandleAppend(process *gen.RaftProcess, ref etf.Ref, serial uint64, key string, value etf.Term) gen.RaftStatus {
	return gen.RaftStatusOK
}
func (r *Raft3) HandleGet(process *gen.RaftProcess, serial uint64) (string, etf.Term, gen.RaftStatus) {
	var key string
	var value etf.Term

	return key, value, gen.RaftStatusOK
}
func (r *Raft3) HandleRaftInfo(process *gen.RaftProcess, message etf.Term) gen.ServerStatus {
	return gen.ServerStatusOK
}
