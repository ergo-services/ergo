package ergonode

import (
	"github.com/halturin/ergonode/etf"
	"github.com/halturin/ergonode/lib"
)

type observer struct {
	GenServer
}

func (o *observer) Init(args ...interface{}) (state interface{}) {
	lib.Log("OBSERVER: Init: %#v", args)
	return nil
}

func (o *observer) HandleCast(message *etf.Term, state interface{}) (string, interface{}) {
	lib.Log("OBSERVER: HandleCast: %#v", *message)
	return "noreply", state
}

func (o *observer) HandleCall(from *etf.Tuple, message *etf.Term, state interface{}) (string, *etf.Term, interface{}) {
	lib.Log("OBSERVER: HandleCall: %#v, From: %#v", *message, *from)
	replyTerm := etf.Term("reply")
	message = &replyTerm
	return "reply", message, state
}

func (o *observer) HandleInfo(message *etf.Term, state interface{}) (string, interface{}) {
	lib.Log("OBSERVER: HandleInfo: %#v", *message)
	return "noreply", state
}

func (o *observer) Terminate(reason string, state interface{}) {
	lib.Log("OBSERVER: Terminate: %#v", reason)
}
