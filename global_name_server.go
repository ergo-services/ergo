package ergo

import "github.com/halturin/ergo/etf"

// TODO: https://github.com/erlang/otp/blob/master/lib/kernel/src/global.erl

type globalNameServer struct {
	GenServer
}

func (gns *globalNameServer) HandleCast(message etf.Term, state GenServerState) string {
	return "noreply"
}
