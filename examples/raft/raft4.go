package main

import (
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
	opts := gen.RaftOptions{}

	return opts, nil
}

func (r *Raft4) HandleAppend(process *gen.RaftProcess, ref etf.Ref, serial uint64, key string, value etf.Term) gen.RaftStatus {
	return gen.RaftStatusOK
}
func (r *Raft4) HandleGet(process *gen.RaftProcess, serial uint64) (string, etf.Term, gen.RaftStatus) {
	var key string
	var value etf.Term

	return key, value, gen.RaftStatusOK
}
