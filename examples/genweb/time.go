package main

import (
	"time"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/lib"
)

type timeServer struct {
	gen.Server
}

type messageTimeServerRequest struct {
	from etf.Pid
	ref  etf.Ref
}

type messageTimeServerReply struct {
	ref  etf.Ref
	time time.Time
}

func (ts *timeServer) HandleCast(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	switch m := message.(type) {
	case messageTimeServerRequest:
		reply := messageTimeServerReply{
			ref:  m.ref,
			time: time.Now(),
		}
		process.Cast(m.from, reply)
	default:
		lib.Warning("got unknown message %#v", m)
	}
	return gen.ServerStatusOK
}
