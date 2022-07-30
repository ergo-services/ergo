package main

import (
	"fmt"
	"net/http"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
)

type timeHandler struct {
	gen.WebHandler
}

type processState struct {
	asyncRequests map[etf.Ref]gen.WebMessageRequest
}

func (th *timeHandler) HandleRequest(process *gen.WebHandlerProcess, request gen.WebMessageRequest) gen.WebHandlerStatus {
	state, ok := process.State.(*processState)
	if !ok {
		state = &processState{
			asyncRequests: make(map[etf.Ref]gen.WebMessageRequest),
		}
		process.State = state
	}
	mt := messageTimeServerRequest{
		ref:  request.Ref,
		from: process.Self(),
	}
	if err := process.Cast("time", mt); err == nil {
		state.asyncRequests[mt.ref] = request
		return gen.WebHandlerStatusWait
	}

	request.Response.WriteHeader(http.StatusServiceUnavailable) // 503
	return gen.WebHandlerStatusDone
}

func (th *timeHandler) HandleWebHandlerCast(process *gen.WebHandlerProcess, message etf.Term) gen.ServerStatus {
	state, ok := process.State.(*processState)
	if !ok {
		return gen.ServerStatusOK
	}

	switch m := message.(type) {
	case messageTimeServerReply:
		request, ok := state.asyncRequests[m.ref]
		if !ok {
			return gen.ServerStatusOK
		}
		delete(state.asyncRequests, m.ref)
		timeResult := fmt.Sprintf("time: %s", m.time)
		request.Response.Write([]byte(timeResult))
		process.Reply(m.ref, nil, nil)

	}
	return gen.ServerStatusOK
}
