package main

import (
	"flag"
	"github.com/ergolang/etf"
	"github.com/halturin/node"
	"log"
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
	log.Printf("Init: %#v", args)

	// Self-registration with name go_srv
	gs.Node.Register(etf.Atom(SrvName), gs.Self)

	// Store first argument as channel
	gs.completeChan = args[0].(chan bool)
}

// HandleCast
// Call `gen_server:cast({go_srv, gonode@localhost}, stop)` at Erlang node to stop this Go-node
func (gs *gonodeSrv) HandleCast(message *etf.Term) {
	log.Printf("HandleCast: %#v", *message)

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
	// log.Printf("HandleCall: %#v, From: %#v\n", *message, *from)

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
	}

	return
}

// HandleInfo handles all another incoming messages
func (gs *gonodeSrv) HandleInfo(message *etf.Term) {
	log.Printf("HandleInfo: %#v", *message)
}

// Terminate called when process died
func (gs *gonodeSrv) Terminate(reason interface{}) {
	log.Printf("Terminate: %#v", reason.(int))
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
		log.Println("PANIC: Cannot publish: %s", err)
		return
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
		log.Printf("Cannot provide function to RPC: %s", err)
	}

	log.Println("Allowed commands...")
	log.Printf("gen_server:cast({%s,'%s'}, stop).", SrvName, NodeName)
	log.Printf("gen_server:call({%s,'%s'}, pid).", SrvName, NodeName)
	log.Printf("gen_server:cast({%s,'%s'}, {ping, self()}), flush().", SrvName, NodeName)

	// Wait to stop
	<-completeChan

	return
}
