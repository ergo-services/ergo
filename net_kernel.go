package node

import (
	erl "github.com/goerlang/etf/types"
)

type netKernel struct {
	GenServerImpl
}

func (nk *netKernel) Behaviour() (Behaviour, map[string]interface{}) {
	return nk, nk.Options()
}

func (nk *netKernel) Init(args ...interface{}) {
	nLog("NET_KERNEL: Init: %#v", args)
	nk.node.Register(erl.Atom("net_kernel"), nk.self)
}

func (nk *netKernel) HandleCast(message *erl.Term) {
	nLog("NET_KERNEL: HandleCast: %#v", *message)
}

func (nk *netKernel) HandleCall(message *erl.Term, from *erl.Tuple) (reply *erl.Term) {
	nLog("NET_KERNEL: HandleCall: %#v, From: %#v", *message, *from)
	switch t := (*message).(type) {
	case erl.Tuple:
		if len(t) == 2 {
			switch tag := t[0].(type) {
			case erl.Atom:
				if string(tag) == "is_auth" {
					nLog("NET_KERNEL: is_auth: %#v", t[1])
					replyTerm := erl.Term(erl.Atom("yes"))
					reply = &replyTerm
				}
			}
		}
	}
	return
}

func (nk *netKernel) HandleInfo(message *erl.Term) {
	nLog("NET_KERNEL: HandleInfo: %#v", *message)
}

func (nk *netKernel) Terminate(reason interface{}) {
	nLog("NET_KERNEL: Terminate: %#v", reason.(int))
}
