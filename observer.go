package ergonode

import (
	"github.com/halturin/ergonode/etf"
	"github.com/halturin/ergonode/lib"
)

type observer struct {
	GenServer
}

func (gns *observer) Init(args ...interface{}) (state interface{}) {
	lib.Log("OBSERVER: Init: %#v", args)
	gns.Node.Register("observer", gns.Self)
	return nil
}

func (gns *observer) HandleCast(message *etf.Term, state interface{}) (code int, stateout interface{}) {
	lib.Log("OBSERVER: HandleCast: %#v", *message)
	stateout = state
	code = 0
	return
}

func (gns *observer) HandleCall(from *etf.Tuple, message *etf.Term, state interface{}) (code int, reply *etf.Term, stateout interface{}) {
	lib.Log("OBSERVER: HandleCall: %#v, From: %#v", *message, *from)
	stateout = state
	code = 1
	replyTerm := etf.Term("reply")
	reply = &replyTerm
	return
}

func (gns *observer) HandleInfo(message *etf.Term, state interface{}) (code int, stateout interface{}) {
	lib.Log("OBSERVER: HandleInfo: %#v", *message)
	stateout = state
	code = 0
	return
}

func (gns *observer) Terminate(reason int, state interface{}) {
	lib.Log("OBSERVER: Terminate: %#v", reason)
}
