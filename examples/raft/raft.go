package main

import (
	"fmt"
	"time"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
)

//
// Raft
//
type Raft struct {
	gen.Raft
	storage *storage

	startSerial uint64
	startPeers  []gen.ProcessID
}

type messageAppend struct {
	key   string
	value string
}

func (r *Raft) InitRaft(process *gen.RaftProcess, args ...etf.Term) (gen.RaftOptions, error) {
	opts := gen.RaftOptions{
		Serial: r.startSerial,
		Peers:  r.startPeers,
	}

	r.storage = &storage{}
	r.storage.Init(int(opts.Serial))
	fmt.Printf("Serial: %d PID: %s ...", opts.Serial, process.Self())

	return opts, nil
}
func (r *Raft) HandleQuorum(process *gen.RaftProcess, quorum *gen.RaftQuorum) gen.RaftStatus {
	fmt.Println(process.Self(), "Quorum built - State:", quorum.State, "Quorum member:", quorum.Member)
	if quorum.Member == false {
		fmt.Println(process.Self(), "    since I'm not a quorum member, I won't receive any information about elected leader")
		message := messageAppend{key: "key100", value: "value100"}
		process.SendAfter(process.Self(), message, 500*time.Millisecond)
	}
	return gen.RaftStatusOK
}

func (r *Raft) HandleLeader(process *gen.RaftProcess, leader *gen.RaftLeader) gen.RaftStatus {
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
		for i := s + 1; i <= leader.Serial; i++ {
			req, e := process.Get(i)
			if e != nil {
				panic(e)
			}
			fmt.Println(process.Self(), "    requested missing serial to the cluster:", i, " id:", req)
		}
	}
	return gen.RaftStatusOK
}

func (r *Raft) HandleAppend(process *gen.RaftProcess, ref etf.Ref, serial uint64, key string, value etf.Term) gen.RaftStatus {
	fmt.Printf("%s Received append request with serial %d, key %q and value %q\n", process.Self(), serial, key, value)
	r.storage.Append(serial, key, value)
	// check if storage has missing items
	for i := uint64(1); i < serial; i++ {
		_, v := r.storage.Read(i)
		if v != nil {
			continue
		}
		req, e := process.Get(i)
		if e != nil {
			panic(e)
		}
		fmt.Println(process.Self(), "Missing serial:", i, " requested missing serial to the cluster. id", req)
	}
	return gen.RaftStatusOK
}
func (r *Raft) HandleGet(process *gen.RaftProcess, serial uint64) (string, etf.Term, gen.RaftStatus) {
	var key string
	var value etf.Term
	fmt.Println(process.Self(), "Received request for serial", serial)
	key, value = r.storage.Read(serial)
	return key, value, gen.RaftStatusOK
}

func (r *Raft) HandleRaftInfo(process *gen.RaftProcess, message etf.Term) gen.ServerStatus {
	messageAppend, ok := message.(messageAppend)
	if ok == false {
		return gen.ServerStatusOK
	}
	if _, err := process.Append(messageAppend.key, messageAppend.value); err != nil {
		fmt.Println("can't make append request", err)
		return gen.ServerStatusOK
	}
	return gen.ServerStatusOK
}

func (r *Raft) HandleSerial(process *gen.RaftProcess, ref etf.Ref, serial uint64, key string, value etf.Term) gen.RaftStatus {
	fmt.Println(process.Self(), "Received requested serial", serial, "with key", key, "and value", value)
	r.storage.Append(serial, key, value)
	return gen.RaftStatusOK
}
