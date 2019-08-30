package ergonode

import (
	"github.com/halturin/ergonode/etf"
	"github.com/halturin/ergonode/lib"
)

type rpcFunction func(etf.List) etf.Term

type modFun struct {
	module   string
	function string
}

type rpc struct {
	GenServer
	process Process
	methods map[modFun]rpcFunction
}

func (n *Node) RpcProvide(modName string, funName string, fun rpcFunction) (err error) {
	lib.Log("Provide: %s:%s %#v", modName, funName, fun)
	n.system.rpc.methods[modFun{modName, funName}] = fun
	return
}

func (n *Node) RpcRevoke(modName, funName string) {
	lib.Log("Revoke: %s:%s", modName, funName)
}

// Init initializes process state using arbitrary arguments
// Init(...) -> state
func (r *rpc) Init(p Process, args ...interface{}) interface{} {
	lib.Log("RPC: Init: %#v", args)
	r.process = p
	r.methods = make(map[modFun]rpcFunction, 0)
	return nil
}

// HandleCast -> ("noreply", state) - noreply
//		         ("stop", reason) - stop with reason
func (r *rpc) HandleCast(message etf.Term, state interface{}) (string, interface{}) {
	lib.Log("RPC: HandleCast: %#v", message)
	return "noreply", state
}

// HandleCall serves incoming messages sending via gen_server:call
// HandleCall -> ("reply", message, state) - reply
//				 ("noreply", _, state) - noreply
//		         ("stop", reason, _) - normal stop
func (r *rpc) HandleCall(from etf.Tuple, message etf.Term, state interface{}) (string, etf.Term, interface{}) {
	lib.Log("RPC: HandleCall: %#v, From: %#v", message, from)
	var reply etf.Term
	valid := false
	switch req := (message).(type) {
	case etf.Tuple:
		if len(req) > 0 {
			switch act := req[0].(type) {
			case etf.Atom:
				if string(act) == "call" {
					valid = true
					if fun, ok := r.methods[modFun{string(req[1].(etf.Atom)), string(req[2].(etf.Atom))}]; ok {
						reply = fun(req[3].(etf.List))
					} else {
						reply = etf.Term(etf.Tuple{etf.Atom("badrpc"), etf.Tuple{etf.Atom("EXIT"), etf.Tuple{etf.Atom("undef"), etf.List{etf.Tuple{req[1], req[2], req[3], etf.List{}}}}}})
					}
				}
			}
		}
	}
	if !valid {
		reply = etf.Term(etf.Tuple{etf.Atom("badrpc"), etf.Atom("unknown")})
	}
	return "reply", reply, state
}

// HandleInfo serves all another incoming messages (Pid ! message)
// HandleInfo -> ("noreply", state) - noreply
//		         ("stop", reason) - normal stop
func (r *rpc) HandleInfo(message etf.Term, state interface{}) (string, interface{}) {
	lib.Log("RPC: HandleInfo: %#v", message)
	return "noreply", state
}

// Terminate called when process died
func (r *rpc) Terminate(reason string, state interface{}) {
	lib.Log("RPC: Terminate: %#v", reason)
}
