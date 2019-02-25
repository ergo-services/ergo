package ergonode

import (
	"github.com/halturin/ergonode/etf"
	"github.com/halturin/ergonode/lib"
)

type netKernel struct {
	GenServer
}

func (nk *netKernel) Init(args ...interface{}) (state interface{}) {
	lib.Log("NET_KERNEL: Init: %#v", args)
	nk.Node.Register(etf.Atom("net_kernel"), nk.Self)
	return nil
}

func (nk *netKernel) HandleCast(message *etf.Term, state interface{}) (code int, stateout interface{}) {
	lib.Log("NET_KERNEL: HandleCast: %#v", *message)
	stateout = state
	code = 0
	return
}

func (nk *netKernel) HandleCall(from *etf.Tuple, message *etf.Term, state interface{}) (code int, reply *etf.Term, stateout interface{}) {
	lib.Log("NET_KERNEL: HandleCall: %#v, From: %#v", *message, *from)
	stateout = state
	code = 1
	switch t := (*message).(type) {
	case etf.Tuple:
		if len(t) == 2 {
			switch tag := t[0].(type) {
			case etf.Atom:
				if string(tag) == "is_auth" {
					lib.Log("NET_KERNEL: is_auth: %#v", t[1])
					replyTerm := etf.Term(etf.Atom("yes"))
					reply = &replyTerm
				}
			}
		}
	}
	return
}

func (nk *netKernel) HandleInfo(message *etf.Term, state interface{}) (code int, stateout interface{}) {
	lib.Log("NET_KERNEL: HandleInfo: %#v", *message)
	stateout = state
	code = 0
	return
}

func (nk *netKernel) Terminate(reason int, state interface{}) {
	lib.Log("NET_KERNEL: Terminate: %#v", reason)
}
