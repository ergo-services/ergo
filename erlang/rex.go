package erlang

// https://github.com/erlang/otp/blob/master/lib/kernel/src/rpc.erl

import (
	"fmt"

	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/gen"
	"github.com/halturin/ergo/lib"
	"github.com/halturin/ergo/node"
)

type modFun struct {
	module   string
	function string
}

var (
	allowedModFun = []string{
		"observer_backend",
	}
)

type rex struct {
	gen.Server
	// Keep methods in the object. Won't be lost if the process restarted for some reason
	methods map[modFun]gen.RPC
}

func (r *rex) Init(process *gen.ServerProcess, args ...etf.Term) error {
	lib.Log("REX: Init: %#v", args)
	// Do not overwrite existing methods if this process restarted
	if r.methods == nil {
		r.methods = make(map[modFun]gen.RPC, 0)
	}

	for i := range allowedModFun {
		mf := modFun{
			allowedModFun[i],
			"*",
		}
		r.methods[mf] = nil
	}
	node := process.Env("ergo:node").(node.Node)
	node.ProvideRemoteSpawn("erpc", &erpc{})
	return nil
}

func (r *rex) HandleCall(process *gen.ServerProcess, from gen.ServerFrom, message etf.Term) (etf.Term, gen.ServerStatus) {
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
				return reply, gen.ServerStatusOK
			}

			to := gen.ProcessID{string(module), process.NodeName()}
			m := etf.Tuple{m.Element(3), m.Element(4)}
			reply, err := process.Call(to, m)
			if err != nil {
				reply = etf.Term(etf.Tuple{etf.Atom("error"), err})
			}
			return reply, gen.ServerStatusOK

		}
	}

	reply := etf.Term(etf.Tuple{etf.Atom("badrpc"), etf.Atom("unknown")})
	return reply, gen.ServerStatusOK
}

func (r *rex) HandleInfo(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	// add this handler to suppres any messages from erlang
	return gen.ServerStatusOK
}

func (r *rex) HandleDirect(process *gen.ServerProcess, message interface{}) (interface{}, error) {
	switch m := message.(type) {
	case gen.MessageManageRPC:
		mf := modFun{
			module:   m.Module,
			function: m.Function,
		}
		// provide RPC
		if m.Provide {
			if _, ok := r.methods[mf]; ok {
				return nil, node.ErrTaken
			}
			r.methods[mf] = m.Fun
			return nil, nil
		}

		// revoke RPC
		if _, ok := r.methods[mf]; ok {
			delete(r.methods, mf)
			return nil, nil
		}
		return nil, fmt.Errorf("unknown RPC name")

	default:
		return nil, gen.ErrUnsupportedRequest
	}
}

func (r *rex) handleRPC(process *gen.ServerProcess, module, function etf.Atom, args etf.List) (reply interface{}) {
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

type erpc struct {
	gen.Server
}

func (e *erpc) Init(process *gen.ServerProcess, args ...etf.Term) error {
	lib.Log("ERPC [%v]: Init: %#v", process.Self(), args)
	return nil
}

func (e *erpc) HandleCall(process *gen.ServerProcess, from gen.ServerFrom, message etf.Term) (etf.Term, gen.ServerStatus) {
	lib.Log("ERPC [%v]: HandleCall: %#v, From: %#v", process.Self(), message, from)

	return etf.Atom("ok"), gen.ServerStatusOK
}

func (e *erpc) HandleCast(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	lib.Log("ERPC [%v]: HandleCast: %#v", process.Self(), message)
	return gen.ServerStatusOK
}

func (e *erpc) HandleInfo(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	lib.Log("ERPC [%v]: HandleInfo: %#v", process.Self(), message)
	return gen.ServerStatusOK
}
