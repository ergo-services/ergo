package node

import (
	"erlang/term"
)

func global_name_server(n *Node, msg term.Term, state interface{}) (newState interface{}, ts *term.Term) {
	nLog("GLOBAL_NAME_SERVER message: %#v", msg)
	newState = state
	return
}
