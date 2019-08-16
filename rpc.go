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

func (r *rpc) Init(args ...interface{}) interface{} {
	lib.Log("RPC: Init: %#v", args)
	r.Node.Register(etf.Atom("rpc"), r.self)
	r.methods = make(map[modFun]rpcFunction, 0)
	return nil
}

func (r *rpc) HandleCast(message *etf.Term, state interface{}) (string, interface{}) {
	lib.Log("RPC: HandleCast: %#v", *message)
	return "noreply", state
}

func (r *rpc) HandleCall(from *etf.Tuple, message *etf.Term, state interface{}) (string, *etf.Term, interface{}) {
	lib.Log("RPC: HandleCall: %#v, From: %#v", *message, *from)
	var replyTerm etf.Term
	valid := false
	switch req := (*message).(type) {
	case etf.Tuple:
		if len(req) > 0 {
			switch act := req[0].(type) {
			case etf.Atom:
				if string(act) == "call" {
					valid = true
					if fun, ok := r.methods[modFun{string(req[1].(etf.Atom)), string(req[2].(etf.Atom))}]; ok {
						replyTerm = fun(req[3].(etf.List))
					} else {
						replyTerm = etf.Term(etf.Tuple{etf.Atom("badrpc"), etf.Tuple{etf.Atom("EXIT"), etf.Tuple{etf.Atom("undef"), etf.List{etf.Tuple{req[1], req[2], req[3], etf.List{}}}}}})
					}
				}
			}
		}
	}
	if !valid {
		replyTerm = etf.Term(etf.Tuple{etf.Atom("badrpc"), etf.Atom("unknown")})
	}
	message = &replyTerm
	return "reply", message, state
}

func (r *rpc) HandleInfo(message *etf.Term, state interface{}) (string, interface{}) {
	lib.Log("RPC: HandleInfo: %#v", *message)
	return "noreply", state
}

func (r *rpc) Terminate(reason int, state interface{}) {
	lib.Log("RPC: Terminate: %#v", reason)
}
