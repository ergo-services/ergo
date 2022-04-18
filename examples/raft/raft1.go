package main

import (
	"fmt"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
)

//
// Raft1
//
type Raft1 struct {
	gen.Raft
}

func (r *Raft1) InitRaft(process *gen.RaftProcess, args ...etf.Term) (gen.RaftOptions, error) {
	opts := gen.RaftOptions{
		Serial: 1,
	}
	fmt.Printf("Serial: %d PID: %s ...", opts.Serial, process.Self())

	return opts, nil
}
func (r *Raft1) HandleQuorum(process *gen.RaftProcess, quorum *gen.RaftQuorum) gen.RaftStatus {
	fmt.Println(process.Self(), "Quorum built - State:", quorum.State, "Quorum member:", quorum.Member)
	return gen.RaftStatusOK
}

func (r *Raft1) HandleLeader(process *gen.RaftProcess, leader *gen.RaftLeader) gen.RaftStatus {
	if leader != nil && leader.Leader == process.Self() {
		fmt.Println(process.Self(), "I'm a leader of this quorum")
		return gen.RaftStatusOK
	}

	if leader != nil {
		fmt.Println(process.Self(), "Leader elected:", leader.Leader, "with serial", leader.Serial)
	}
	return gen.RaftStatusOK
}

func (r *Raft1) HandleAppend(process *gen.RaftProcess, ref etf.Ref, serial uint64, key string, value etf.Term) gen.RaftStatus {
	return gen.RaftStatusOK
}
func (r *Raft1) HandleGet(process *gen.RaftProcess, serial uint64) (string, etf.Term, gen.RaftStatus) {
	var key string
	var value etf.Term

	return key, value, gen.RaftStatusOK
}

func (r *Raft1) HandleRaftInfo(process *gen.RaftProcess, message etf.Term) gen.ServerStatus {
	return gen.ServerStatusOK
}
