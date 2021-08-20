package erlang

import (
	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/gen"
)

// TODO: https://github.com/erlang/otp/blob/master/lib/kernel/src/global.erl

type GlobalNameServer struct {
	gen.Server
}

func (gns *GlobalNameServer) HandleCast(process *gen.ServerProcess, message etf.Term) string {
	return "noreply"
}
