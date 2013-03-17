package node

import (
	"github.com/goerlang/etf"
)

type rpcFunction func(etf.List) etf.Term

type modFun struct {
	module   string
	function string
}

type rpcRex struct {
	GenServerImpl
	callMap map[modFun]rpcFunction
}

func (currNode *Node) RpcProvide(modName string, funName string, fun rpcFunction) (err error) {
	nLog("Provide: %s:%s %#v", modName, funName, fun)
	currNode.sysProcs.rpcRex.callMap[modFun{modName, funName}] = fun
	return
}

func (currNode *Node) RpcRevoke(modName, funName string) {
	nLog("Revoke: %s:%s", modName, funName)
}

func (rpcs *rpcRex) Init(args ...interface{}) {
	nLog("REX: Init: %#v", args)
	rpcs.Node.Register(etf.Atom("rex"), rpcs.Self)
	rpcs.callMap = make(map[modFun]rpcFunction, 0)
}

func (rpcs *rpcRex) HandleCast(message *etf.Term) {
	nLog("REX: HandleCast: %#v", *message)
}

func (rpcs *rpcRex) HandleCall(message *etf.Term, from *etf.Tuple) (reply *etf.Term) {
	nLog("REX: HandleCall: %#v, From: %#v", *message, *from)
	var replyTerm etf.Term
	valid := false
	switch req := (*message).(type) {
	case etf.Tuple:
		if len(req) > 0 {
			switch act := req[0].(type) {
			case etf.Atom:
				if string(act) == "call" {
					valid = true
					if fun, ok := rpcs.callMap[modFun{string(req[1].(etf.Atom)), string(req[2].(etf.Atom))}]; ok {
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
	reply = &replyTerm
	return
}

func (rpcs *rpcRex) HandleInfo(message *etf.Term) {
	nLog("REX: HandleInfo: %#v", *message)
}

func (rpcs *rpcRex) Terminate(reason interface{}) {
	nLog("REX: Terminate: %#v", reason.(int))
}
