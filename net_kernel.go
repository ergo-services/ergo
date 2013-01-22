package node

import (
	"erlang/term"
)

type netKernel struct {
	node *Node
}

func (nk *netKernel) Behaviour() (Behaviour, map[string]interface{}) {
	gsi := &GenServerImpl{}
	return gsi, gsi.Options()
}

func (nk *netKernel) Init(args ...interface{}) {
	nLog("NET_KERNEL: Init: %#v", args)
	nk.node = args[0].(*Node)
}

func (nk *netKernel) HandleCast(message *term.Term) {
	nLog("NET_KERNEL: HandleCast: %#v", *message)
}

func (nk *netKernel) HandleCall(message *term.Term, from *term.Tuple) (reply *term.Term) {
	nLog("NET_KERNEL: HandleCall: %#v, From: %#v", *message, *from)
	switch t := (*message).(type) {
	case term.Tuple:
		if len(t) == 2 {
			switch tag := t[0].(type) {
			case term.Atom:
				if string(tag) == "is_auth" {
					nLog("NET_KERNEL: is_auth: %#v", t[1])
					replyTerm := term.Term(term.Atom("yes"))
					reply = &replyTerm
				}
			}
		}
	}
	return
}

func (nk *netKernel) HandleInfo(message *term.Term) {
	nLog("NET_KERNEL: HandleInfo: %#v", *message)
}

func (nk *netKernel) Terminate(reason interface{}) {
	nLog("NET_KERNEL: Terminate: %#v", reason.(int))
}
