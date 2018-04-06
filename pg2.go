package ergonode

import (
	"github.com/halturin/ergonode/etf"
)

// pg2Server allows node-to-node connectivity with Elixir Phoenix PG2 PubSub adapter.
// https://hexdocs.pm/phoenix/1.1.0/Phoenix.PubSub.PG2.html
//
// Without this subsystem, the communication between ergonode and most Phoenix-based
// nodes will catastrophically hang, due to a perpetual channel blocking inside the
// "func (n *Node) route(...)" function, when pg2 is not found in the registered list of servers.
type pg2Server struct {
	GenServer
}

func (pg2 *pg2Server) Init(args ...interface{}) (state interface{}) {
	nLog("PG2_PUBSUB_SERVER: Init: %#v", args)
	pg2.Node.Register(etf.Atom("pg2"), pg2.Self)
	return nil
}

func (pg2 *pg2Server) HandleCast(message *etf.Term, state interface{}) (code int, stateout interface{}) {
	nLog("PG2_PUBSUB_SERVER: HandleCast: %#v", *message)
	stateout = state
	code = 0
	return
}

func (pg2 *pg2Server) HandleCall(from *etf.Tuple, message *etf.Term, state interface{}) (code int, reply *etf.Term, stateout interface{}) {
	nLog("PG2_PUBSUB_SERVER: HandleCall: %#v, From: %#v", *message, *from)
	stateout = state
	code = 1
	replyTerm := etf.Term(etf.Atom("reply"))
	reply = &replyTerm
	return
}

func (pg2 *pg2Server) HandleInfo(message *etf.Term, state interface{}) (code int, stateout interface{}) {
	nLog("PG2_PUBSUB_SERVER: HandleInfo: %#v", *message)
	stateout = state
	code = 0
	return
}

func (pg2 *pg2Server) Terminate(reason int, state interface{}) {
	nLog("PG2_PUBSUB_SERVER: Terminate: %#v", reason)
}
