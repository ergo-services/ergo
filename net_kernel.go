package node

import (
	"github.com/goerlang/etf"
)

type netKernel struct {
	GenServerImpl
}

func (nk *netKernel) Init(args ...interface{}) {
	nLog("NET_KERNEL: Init: %#v", args)
	nk.Node.Register(etf.Atom("net_kernel"), nk.Self)
}

func (nk *netKernel) HandleCast(message *etf.Term) {
	nLog("NET_KERNEL: HandleCast: %#v", *message)
}

func (nk *netKernel) HandleCall(message *etf.Term, from *etf.Tuple) (reply *etf.Term) {
	nLog("NET_KERNEL: HandleCall: %#v, From: %#v", *message, *from)
	switch t := (*message).(type) {
	case etf.Tuple:
		if len(t) == 2 {
			switch tag := t[0].(type) {
			case etf.Atom:
				if string(tag) == "is_auth" {
					nLog("NET_KERNEL: is_auth: %#v", t[1])
					replyTerm := etf.Term(etf.Atom("yes"))
					reply = &replyTerm
				}
			}
		}
	}
	return
}

func (nk *netKernel) HandleInfo(message *etf.Term) {
	nLog("NET_KERNEL: HandleInfo: %#v", *message)
}

func (nk *netKernel) Terminate(reason interface{}) {
	nLog("NET_KERNEL: Terminate: %#v", reason.(int))
}
