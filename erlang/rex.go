package erlang

// https://github.com/erlang/otp/blob/master/lib/kernel/src/rpc.erl

import (
	"fmt"

	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/gen"
	"github.com/halturin/ergo/lib"
)

type FunctionRPC func(...etf.Term) etf.Term

type modFun struct {
	module   string
	function string
}

var (
	allowedModFun = []string{
		"observer_backend",
	}
)

type Rex struct {
	gen.Server
	methods map[modFun]FunctionRPC
}

func (r *Rex) Init(state *gen.ServerProcess, args ...etf.Term) error {
	lib.Log("REX: Init: %#v", args)
	r.methods = make(map[modFun]FunctionRPC, 0)

	for i := range allowedModFun {
		mf := modFun{
			allowedModFun[i],
			"*",
		}
		r.methods[mf] = nil
	}

	return nil
}

func (r *Rex) HandleCall(process *gen.ServerProcess, from gen.ServerFrom, message etf.Term) (string, etf.Term) {
	lib.Log("REX: HandleCall: %#v, From: %#v", message, from)
	switch m := message.(type) {
	case etf.Tuple:
		//etf.Tuple{"call", "observer_backend", "sys_info",
		//           etf.List{}, etf.Pid{Node:"erl-examplenode@127.0.0.1", Id:0x46, Serial:0x0, Creation:0x2}}
		switch m.Element(1) {
		case etf.Atom("call"):
			module := m.Element(2).(etf.Atom)
			function := m.Element(3).(etf.Atom)
			args := m.Element(4).(etf.List)
			reply := r.handleRPC(process, module, function, args)
			if reply != nil {
				return "reply", reply
			}

			to := etf.Tuple{string(module), process.NodeName()}
			m := etf.Tuple{m.Element(3), m.Element(4)}
			reply, err := process.Call(to, m)
			if err != nil {
				reply = etf.Term(etf.Tuple{etf.Atom("error"), err})
			}
			return "reply", reply

		case etf.Atom("$provide"):
			module := m.Element(2).(etf.Atom)
			function := m.Element(3).(etf.Atom)
			fun := m.Element(4).(FunctionRPC)
			mf := modFun{
				module:   string(module),
				function: string(function),
			}
			if _, ok := r.methods[mf]; ok {
				return "reply", etf.Atom("taken")
			}

			r.methods[mf] = fun
			return "reply", etf.Atom("ok")

		case etf.Atom("$revoke"):
			module := m.Element(2).(etf.Atom)
			function := m.Element(3).(etf.Atom)
			mf := modFun{
				module:   string(module),
				function: string(function),
			}

			if _, ok := r.methods[mf]; ok {
				delete(r.methods, mf)
				return "reply", etf.Atom("ok")
			}

			return "reply", etf.Atom("unknown")
		}

	}

	reply := etf.Term(etf.Tuple{etf.Atom("badrpc"), etf.Atom("unknown")})
	return "reply", reply
}

func (r *Rex) handleRPC(process *gen.ServerProcess, module, function etf.Atom, args etf.List) (reply interface{}) {
	defer func() {
		if x := recover(); x != nil {
			err := fmt.Sprintf("panic reason: %s", x)
			// recovered
			reply = etf.Tuple{
				etf.Atom("badrpc"),
				etf.Tuple{
					etf.Atom("EXIT"),
					etf.Tuple{
						etf.Atom("panic"),
						etf.List{
							etf.Tuple{module, function, args, etf.List{err}},
						},
					},
				},
			}
		}
	}()
	mf := modFun{
		module:   string(module),
		function: string(function),
	}
	// calling dynamically declared rpc method
	if function, ok := r.methods[mf]; ok {
		return function(args...)
	}

	// unknown request. return error
	badRPC := etf.Tuple{
		etf.Atom("badrpc"),
		etf.Tuple{
			etf.Atom("EXIT"),
			etf.Tuple{
				etf.Atom("undef"),
				etf.List{
					etf.Tuple{
						module,
						function,
						args,
						etf.List{},
					},
				},
			},
		},
	}
	// calling a local module if its been registered as a process)
	if process.ProcessByName(mf.module) == nil {
		return badRPC
	}

	if value, err := process.Call(mf.module, args); err != nil {
		return badRPC
	} else {
		return value
	}
}
