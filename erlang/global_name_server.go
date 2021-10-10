package erlang

import (
	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
)

// TODO: https://github.com/erlang/otp/blob/master/lib/kernel/src/global.erl

type globalNameServer struct {
	gen.Server
}

func (gns *globalNameServer) HandleCast(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	return gen.ServerStatusOK
}
