package node

import (
	"erlang/term"
)

func net_kernel(n *Node, msg term.Term, state interface{}) (newState interface{}, ts *term.Term) {
	nLog("NET_KERNEL message: %#v", msg)
	newState = state
	return
}
