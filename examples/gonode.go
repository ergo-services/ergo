package main

import (
	"flag"
	"fmt"
	"github.com/ergolang/etf"
	"github.com/halturin/node"
)

// GenServer implementation structure
type gonodeSrv struct {
	node.GenServerImpl
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
func (gs *gonodeSrv) Init(args ...interface{}) {
	fmt.Printf("Init: %#v", args)

	// Self-registration with name go_srv
	gs.Node.Register(etf.Atom(SrvName), gs.Self)

	// Store first argument as channel
	gs.completeChan = args[0].(chan bool)
}

// HandleCast
// Call `gen_server:cast({go_srv, gonode@localhost}, stop)` at Erlang node to stop this Go-node
func (gs *gonodeSrv) HandleCast(message *etf.Term) {
	fmt.Printf("HandleCast: %#v", *message)

	// Check type of message
	switch req := (*message).(type) {
	case etf.Tuple:
		if len(req) == 2 {
			switch act := req[0].(type) {
			case etf.Atom:
				if string(act) == "ping" {
					var self_pid etf.Pid = gs.Self

					gs.Node.Send(req[1].(etf.Pid), etf.Tuple{etf.Atom("pong"), etf.Pid(self_pid)})

				}
			}
		}
	case etf.Atom:
		// If message is atom 'stop', we should say it to main process
		if string(req) == "stop" {
			gs.completeChan <- true
		}
	}
}

// HandleCall handles incoming messages from `gen_server:call/2`, if returns non-nil term,
// then calling process have reply
// Call `gen_server:call({go_srv, gonode@localhost}, Message)` at Erlang node
func (gs *gonodeSrv) HandleCall(message *etf.Term, from *etf.Tuple) (reply *etf.Term) {
	// fmt.Printf("HandleCall: %#v, From: %#v\n", *message, *from)

	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Call recovered: %#v\n", r)
		}
	}()

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

			cmess = req[1]

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

			if string(act) == "testcall" {
				fmt.Println("testcall...")
				reply = gs.Call(cto, &cmess)
			} else if string(act) == "testcast" {
				fmt.Println("testcast...")
				gs.Cast(cto, &cmess)
				replyTerm = etf.Term(etf.Atom("ok"))
				reply = &replyTerm
			} else {
				return
			}

		}
	}
	return
}

// HandleInfo handles all another incoming messages
func (gs *gonodeSrv) HandleInfo(message *etf.Term) {
	fmt.Printf("HandleInfo: %#v\n", *message)
}

// Terminate called when process died
func (gs *gonodeSrv) Terminate(reason interface{}) {
	fmt.Printf("Terminate: %#v\n", reason.(int))
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
	enode := node.NewNode(NodeName, Cookie)

	// Allow node be available on EpmdPort port
	err = enode.Publish(EpmdPort)
	if err != nil {
		panic(fmt.Sprintf("PANIC: Cannot publish: %s", err))
	}

	// Create channel to receive message when main process should be stopped
	completeChan := make(chan bool)

	// Initialize new instance of gonodeSrv structure which implements Process behaviour
	eSrv := new(gonodeSrv)

	// Spawn process with one arguments
	enode.Spawn(eSrv, completeChan)

	// RPC
	// Create closure
	rpc := func(terms etf.List) (r etf.Term) {
		r = etf.Term(etf.Tuple{etf.Atom(NodeName), etf.Atom("reply"), len(terms)})
		return
	}

	// Provide it to call via RPC with `rpc:call(gonode@localhost, rpc, call, [as, qwe])`
	err = enode.RpcProvide("rpc", "call", rpc)
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
