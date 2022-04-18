package main

import (
	"fmt"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
)

//
// Raft4
//
type Raft4 struct {
	gen.Raft
}

func (r *Raft4) InitRaft(process *gen.RaftProcess, args ...etf.Term) (gen.RaftOptions, error) {
	opts := gen.RaftOptions{
		Peers: []gen.ProcessID{
			gen.ProcessID{Name: "raft1", Node: "node1@localhost"},
		},
	}
	fmt.Printf("Serial: %d PID: %s ...", opts.Serial, process.Self())

	return opts, nil
}
func (r *Raft4) HandleQuorum(process *gen.RaftProcess, quorum *gen.RaftQuorum) gen.RaftStatus {
	fmt.Println(process.Self(), "Quorum built - State:", quorum.State, "Quorum member:", quorum.Member)
	if quorum.Member == false {
		fmt.Println(process.Self(), "Since I'm not a quorum member, I won't receive any information about elected leader")
	}
	return gen.RaftStatusOK
}

func (r *Raft4) HandleLeader(process *gen.RaftProcess, leader *gen.RaftLeader) gen.RaftStatus {
	if leader != nil && leader.Leader == process.Self() {
		fmt.Println(process.Self(), "I'm a leader of this quorum")
		return gen.RaftStatusOK
	}

	if leader != nil {
		fmt.Println(process.Self(), "Leader elected:", leader.Leader, "with serial", leader.Serial)
	}

	s := process.Serial()
	if s < leader.Serial {
		fmt.Println(process.Self(), "Missing serials:", s+1, "..", leader.Serial)
		for i := s; i < leader.Serial; i++ {
			req, e := process.Get(i)
			if e != nil {
				panic(e)
			}
			fmt.Println(process.Self(), "    requested missing serial to the cluster:", i, " id:", req)
		}
	}
	return gen.RaftStatusOK
}

func (r *Raft4) HandleAppend(process *gen.RaftProcess, ref etf.Ref, serial uint64, key string, value etf.Term) gen.RaftStatus {

	return gen.RaftStatusOK
}
func (r *Raft4) HandleGet(process *gen.RaftProcess, serial uint64) (string, etf.Term, gen.RaftStatus) {
	var key string
	var value etf.Term
	fmt.Println(process.Self(), "Received request for serial", serial)

	return key, value, gen.RaftStatusOK
}

func (r *Raft4) HandleRaftInfo(process *gen.RaftProcess, message etf.Term) gen.ServerStatus {
	return gen.ServerStatusOK
}

func (r *Raft4) HandleSerial(process *gen.RaftProcess, ref etf.Ref, serial uint64, key string, value etf.Term) gen.RaftStatus {
	fmt.Println(process.Self(), "Received requested serial - ", ref, "serial", serial, "with key", key, " and value", value)
	return gen.RaftStatusOK
}
