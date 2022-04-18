package main

import (
	"fmt"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
)

//
// Raft2
//
type Raft2 struct {
	gen.Raft
}

func (r *Raft2) InitRaft(process *gen.RaftProcess, args ...etf.Term) (gen.RaftOptions, error) {
	opts := gen.RaftOptions{
		Peers: []gen.ProcessID{
			gen.ProcessID{Name: "raft1", Node: "node1@localhost"},
		},
	}

	return opts, nil
}
func (r *Raft2) HandleQuorum(process *gen.RaftProcess, quorum *gen.RaftQuorum) gen.RaftStatus {
	fmt.Println(process.Name(), "quorum built. quorum member:", quorum.Member, "state:", quorum.State)
	return gen.RaftStatusOK
}

func (r *Raft2) HandleAppend(process *gen.RaftProcess, ref etf.Ref, serial uint64, key string, value etf.Term) gen.RaftStatus {
	return gen.RaftStatusOK
}
func (r *Raft2) HandleGet(process *gen.RaftProcess, serial uint64) (string, etf.Term, gen.RaftStatus) {
	var key string
	var value etf.Term

	return key, value, gen.RaftStatusOK
}
func (r *Raft2) HandleRaftInfo(process *gen.RaftProcess, message etf.Term) gen.ServerStatus {
	return gen.ServerStatusOK
}
