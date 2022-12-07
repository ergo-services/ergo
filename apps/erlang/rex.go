package erlang

// https://github.com/erlang/otp/blob/master/lib/kernel/src/rpc.erl

import (
	"fmt"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/lib"
	"github.com/ergo-services/ergo/node"
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

// Init
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
	node := process.Env(node.EnvKeyNode).(node.Node)
	node.ProvideRemoteSpawn("erpc", &erpc{})
	return nil
}

// HandleCall
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

			to := gen.ProcessID{Name: string(module), Node: process.NodeName()}
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

// HandleCast
func (r *rex) HandleCast(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	lib.Log("REX: HandleCast: %#v", message)
	switch m := message.(type) {
	case etf.Tuple:
		//etf.Tuple{"cast", "observer_backend", "sys_info",
		//           etf.List{}, etf.Pid{Node:"erl-examplenode@127.0.0.1", Id:0x46, Serial:0x0, Creation:0x2}}
		switch m.Element(1) {
		case etf.Atom("cast"):
			module := m.Element(2).(etf.Atom)
			function := m.Element(3).(etf.Atom)
			args := m.Element(4).(etf.List)
			reply := r.handleRPC(process, module, function, args)
			if reply != nil {
				checkErrAndWarning(reply)
				return gen.ServerStatusOK
			}

			to := gen.ProcessID{Name: string(module), Node: process.NodeName()}
			m := etf.Tuple{m.Element(3), m.Element(4)}
			err := process.Cast(to, m)
			if err != nil {
				reply = etf.Term(etf.Tuple{etf.Atom("error"), err})
				checkErrAndWarning(reply)
			}
			return gen.ServerStatusOK

		}
	}

	reply := etf.Term(etf.Tuple{etf.Atom("badrpc"), etf.Atom("unknown")})
	checkErrAndWarning(reply)
	return gen.ServerStatusOK
}

func checkErrAndWarning(reply interface{}) {
	switch rep := reply.(type) {
	case etf.Tuple:
		switch rep.Element(1) {
		case etf.Atom("badrpc"):
			lib.Warning("badrpc: %s", rep)
		case etf.Atom("error"):
			lib.Warning("error: %s", rep)
		}
	}
}

// HandleInfo
func (r *rex) HandleInfo(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	// add this handler to suppres any messages from erlang
	return gen.ServerStatusOK
}

// HandleDirect
func (r *rex) HandleDirect(process *gen.ServerProcess, ref etf.Ref, message interface{}) (interface{}, gen.DirectStatus) {
	switch m := message.(type) {
	case gen.MessageManageRPC:
		mf := modFun{
			module:   m.Module,
			function: m.Function,
		}
		// provide RPC
		if m.Provide {
			if _, ok := r.methods[mf]; ok {
				return nil, lib.ErrTaken
			}
			r.methods[mf] = m.Fun
			return nil, gen.DirectStatusOK
		}

		// revoke RPC
		if _, ok := r.methods[mf]; ok {
			delete(r.methods, mf)
			return nil, gen.DirectStatusOK
		}
		return nil, fmt.Errorf("unknown RPC name")

	default:
		return nil, lib.ErrUnsupportedRequest
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

type erpcMFA struct {
	id etf.Ref
	m  etf.Atom
	f  etf.Atom
	a  etf.List
}
type erpc struct {
	gen.Server
}

// Init
func (e *erpc) Init(process *gen.ServerProcess, args ...etf.Term) error {
	lib.Log("ERPC [%v]: Init: %#v", process.Self(), args)
	mfa := erpcMFA{
		id: args[0].(etf.Ref),
		m:  args[1].(etf.Atom),
		f:  args[2].(etf.Atom),
		a:  args[3].(etf.List),
	}
	process.Cast(process.Self(), mfa)
	return nil

}

// HandleCast
func (e *erpc) HandleCast(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	lib.Log("ERPC [%v]: HandleCast: %#v", process.Self(), message)
	mfa := message.(erpcMFA)
	rsr := process.Env("ergo:RemoteSpawnRequest").(gen.RemoteSpawnRequest)

	call := etf.Tuple{etf.Atom("call"), mfa.m, mfa.f, mfa.a}
	value, _ := process.Call("rex", call)

	reply := etf.Tuple{
		etf.Atom("DOWN"),
		rsr.Ref,
		etf.Atom("process"),
		process.Self(),
		etf.Tuple{
			mfa.id,
			etf.Atom("return"),
			value,
		},
	}
	process.Send(rsr.From, reply)

	return gen.ServerStatusStop
}
