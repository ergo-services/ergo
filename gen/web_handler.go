package gen

import (
	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/lib"
)

type WebHandlerBehavior interface {
	ServerBehavior

	// Optional callbacks
	HandleGet(process *WebHandlerProcess, get WebMessageRequest)
	HandlePost(process *WebHandlerProcess, post WebMessageRequest)
	HandleHead(process *WebHandlerProcess, head WebMessageRequest)
	HandlePut(process *WebHandlerProcess, put WebMessageRequest)
	HandleDelete(process *WebHandlerProcess, del WebMessageRequest)
	HandleOptions(process *WebHandlerProcess, options WebMessageRequest)
	HandlePath(process *WebHandlerProcess, path WebMessageRequest)

	HandleWebHandlerCall(process *WebHandlerProcess, from ServerFrom, message etf.Term) (etf.Term, ServerStatus)
	HandleWebHandlerCast(process *WebHandlerProcess, message etf.Term) ServerStatus
	HandleWebHandlerInfo(process *WebHandlerProcess, message etf.Term) ServerStatus
}

type WebHandler struct {
	Server
}

type WebHandlerProcess struct {
	ServerProcess
}

//
// default WebHandler callbacks
//

// HandleWebHandlerCall
func (wh *WebHandler) HandleWebHandlerCall(process *WebHandlerProcess, from ServerFrom, message etf.Term) (etf.Term, ServerStatus) {
	lib.Warning("HandleWebHandlerCall: unhandled message (from %#v) %#v", from, message)
	return etf.Atom("ok"), ServerStatusOK
}

// HandleWebHandlerCast
func (wh *WebHandler) HandleWebHandlerCast(process *WebHandlerProcess, message etf.Term) ServerStatus {
	lib.Warning("HandleWebHandlerCast: unhandled message %#v", message)
	return ServerStatusOK
}

// HandleWebHandlerInfo
func (wh *WebHandler) HandleWebHandlerInfo(process *WebHandlerProcess, message etf.Term) ServerStatus {
	lib.Warning("HandleWebHandlerInfo: unhandled message %#v", message)
	return ServerStatusOK
}

// HandleWebHandlerDirect
func (wh *WebHandler) HandleWebHandlerDirect(process *WebHandlerProcess, message interface{}) (interface{}, error) {
	return nil, ErrUnsupportedRequest
}

// HandleWebHandlerTerminate
func (wh *WebHandler) HandleWebHandlerTerminate(process *WebHandlerProcess, reason string) {
	return
}

// we should disable SetTrapExit for the WebHandlerProcess by overriding it.
func (whp *WebHandlerProcess) SetTrapExit(trap bool) {
	lib.Warning("[%s] method 'SetTrapExit' is disabled for WebHandlerProcess", whp.Self())
}
