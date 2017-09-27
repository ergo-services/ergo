package main

import (
	"flag"
	"fmt"
	"github.com/halturin/ergonode"
	"github.com/halturin/ergonode/etf"
)

// GenServer implementation structure
type goGenServ struct {
	ergonode.GenServer
	completeChan chan bool
}

var (
	SrvName   string
	NodeName  string
	Cookie    string
	err       error
	EpmdPort  int
	EnableRPC bool
)

// Init initializes process state using arbitrary arguments
func (gs *goGenServ) Init(args ...interface{}) interface{} {
	// Self-registration with name go_srv
	gs.Node.Register(etf.Atom(SrvName), gs.Self)

	// Store first argument as channel
	gs.completeChan = args[0].(chan bool)

	return nil
}

// HandleCast serves incoming messages sending via gen_server:cast
func (gs *goGenServ) HandleCast(message *etf.Term, state interface{}) (code int, stateout interface{}) {
	fmt.Printf("HandleCast: %#v", *message)
	stateout = state
	code = 0
	// Check type of message
	switch req := (*message).(type) {
	case etf.Tuple:
		if len(req) == 2 {
			switch act := req[0].(type) {
			case etf.Atom:
				if string(act) == "ping" {
					var self_pid etf.Pid = gs.Self
					rep := etf.Term(etf.Tuple{etf.Atom("pong"), etf.Pid(self_pid)})
					gs.Send(req[1].(etf.Pid), &rep)

				}
			}
		}
	case etf.Atom:
		// If message is atom 'stop', we should say it to main process
		if string(req) == "stop" {
			gs.completeChan <- true
		}
	}
	return
}

// HandleCall serves incoming messages sending via gen_server:call
func (gs *goGenServ) HandleCall(from *etf.Tuple, message *etf.Term, state interface{}) (code int, reply *etf.Term, stateout interface{}) {
	// fmt.Printf("HandleCall: %#v, From: %#v\n", *message, *from)

	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Call recovered: %#v\n", r)
		}
	}()

	stateout = state
	code = 1
	replyTerm := etf.Term(etf.Tuple{etf.Atom("error"), etf.Atom("unknown_request")})
	reply = &replyTerm

	switch req := (*message).(type) {
	case etf.Atom:
		// If message is atom 'stop', we should say it to main process
		switch string(req) {
		case "pid":
			replyTerm = etf.Term(etf.Pid(gs.Self))
			reply = &replyTerm
		}
	case etf.Tuple:
		var cto, cmess etf.Term
		// {testcall, { {name, node}, message  }}
		// {testcast, { {name, node}, message  }}
		if len(req) == 2 {
			act := req[0].(etf.Atom)
			c := req[1].(etf.Tuple)

			switch c[0].(type) {
			case etf.Tuple:
				switch ct := c[0].(type) {
				case etf.Tuple:
					if ct[0].(etf.Atom) == ct[1].(etf.Atom) {
					}
					cto = etf.Term(c[0])
				default:
					return
				}
			case etf.Pid:
				cto = etf.Term(c[0])
			default:
				return
			}

			cmess = c[1]

			if string(act) == "testcall" {
				fmt.Printf("!!!!!!!testcall... %#v : %#v\n", cto, cmess)
				if reply, err = gs.Call(cto, &cmess); err != nil {
					fmt.Println(err.Error())
				}
			} else if string(act) == "testcast" {
				fmt.Println("testcast...")
				gs.Cast(cto, &cmess)
				fmt.Println("testcast...2222")
				replyTerm = etf.Term(etf.Atom("ok"))
				reply = &replyTerm
				fmt.Println("testcast...3333")
			} else {
				return
			}

		}
	}
	return
}

// HandleInfo serves all another incoming messages (Pid ! message)
func (gs *goGenServ) HandleInfo(message *etf.Term, state interface{}) (code int, stateout interface{}) {
	fmt.Printf("HandleInfo: %#v\n", *message)
	stateout = state
	code = 0
	return
}

// Terminate called when process died
func (gs *goGenServ) Terminate(reason int, state interface{}) {
	fmt.Printf("Terminate: %#v\n", reason)
}

func init() {
	flag.StringVar(&SrvName, "gen_server", "examplegs", "gen_server name")
	flag.StringVar(&NodeName, "name", "examplenode@127.0.0.1", "node name")
	flag.StringVar(&Cookie, "cookie", "123", "cookie for interaction with erlang cluster")
	flag.IntVar(&EpmdPort, "epmd_port", 15151, "epmd port")
	flag.BoolVar(&EnableRPC, "rpc", false, "enable RPC")
}

func main() {
	flag.Parse()

	// Initialize new node with given name and cookie
	n := ergonode.Create(NodeName, uint16(EpmdPort), Cookie)

	// Create channel to receive message when main process should be stopped
	completeChan := make(chan bool)

	// Initialize new instance of goGenServ structure which implements Process behaviour
	gs := new(goGenServ)

	// Spawn process with one arguments
	n.Spawn(gs, completeChan)

	// RPC
	// Create closure
	rpc := func(terms etf.List) (r etf.Term) {
		r = etf.Term(etf.Tuple{etf.Atom(NodeName), etf.Atom("reply"), len(terms)})
		return
	}

	// Provide it to call via RPC with `rpc:call(gonode@localhost, rpc, call, [as, qwe])`
	err = n.RpcProvide("rpc", "call", rpc)
	if err != nil {
		fmt.Printf("Cannot provide function to RPC: %s\n", err)
	}

	fmt.Println("Allowed commands...")
	fmt.Printf("gen_server:cast({%s,'%s'}, stop).\n", SrvName, NodeName)
	fmt.Printf("gen_server:call({%s,'%s'}, pid).\n", SrvName, NodeName)
	fmt.Printf("gen_server:cast({%s,'%s'}, {ping, self()}), flush().\n", SrvName, NodeName)
	fmt.Println("make remote call by golang node...")
	fmt.Printf("gen_server:call({%s,'%s'}, {testcall, {Pid, Message}}).\n", SrvName, NodeName)
	fmt.Printf("gen_server:call({%s,'%s'}, {testcall, {{pname, remotenode}, Message}}).\n", SrvName, NodeName)

	// Wait to stop
	<-completeChan

	return
}
