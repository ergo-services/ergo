package ergonode

import (
	"github.com/halturin/ergonode/etf"
	"github.com/halturin/ergonode/lib"
)

type rpcFunction func(...etf.Term) etf.Term

type modFun struct {
	module   etf.Atom
	function etf.Atom
}

type rpc struct {
	GenServer
	process Process
	methods map[modFun]rpcFunction
}

func (n *Node) RpcProvide(module string, function string, fun rpcFunction) {
	lib.Log("Provide: %s:%s %#v", module, function, fun)
	mf := modFun{
		module:   etf.Atom(module),
		function: etf.Atom(function),
	}
	n.system.rpc.methods[mf] = fun
}

func (n *Node) RpcRevoke(module, function string) {
	lib.Log("Revoke: %s:%s", module, function)
	mf := modFun{
		module:   etf.Atom(module),
		function: etf.Atom(function),
	}
	delete(n.system.rpc.methods, mf)
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
	switch req := (message).(type) {
	case etf.Tuple:
		switch act := req.Element(1).(type) {
		case etf.Atom:
			if act == etf.Atom("call") {
				module := req.Element(2).(etf.Atom)
				function := req.Element(3).(etf.Atom)
				args := req.Element(4).(etf.List)

				mf := modFun{
					module:   module,
					function: function,
				}

				if function, ok := r.methods[mf]; ok {
					reply = function(args...)
					return "reply", reply, state
				}

				// unknown request. return error
				reply = etf.Tuple{
					etf.Atom("badrpc"),
					etf.Tuple{
						etf.Atom("EXIT"),
						etf.Tuple{
							etf.Atom("undef"),
							etf.List{
								etf.Tuple{
									req.Element(2),
									req.Element(3),
									req.Element(4),
									etf.List{},
								},
							},
						},
					},
				}

				return "reply", reply, state
			}
		}
	}

	reply = etf.Term(etf.Tuple{etf.Atom("badrpc"), etf.Atom("unknown")})
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
