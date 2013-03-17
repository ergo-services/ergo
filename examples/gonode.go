package main

import (
	"github.com/goerlang/etf"
	"github.com/goerlang/node"
	"log"
)

// GenServer implementation structure
type gonodeSrv struct {
	node.GenServerImpl
	completeChan chan bool
}

// Init initializes process state using arbitrary arguments
func (gs *gonodeSrv) Init(args ...interface{}) {
	log.Printf("GO_SRV: Init: %#v", args)

	// Self-registration with name go_srv
	gs.Node.Register(etf.Atom("go_srv"), gs.Self)

	// Store first argument as channel
	gs.completeChan = args[0].(chan bool)
}

// HandleCast handles incoming messages from `gen_server:cast/2`
// Call `gen_server:cast({go_srv, gonode@localhost}, stop)` at Erlang node to stop this Go-node
func (gs *gonodeSrv) HandleCast(message *etf.Term) {
	log.Printf("GO_SRV: HandleCast: %#v", *message)

	// Check type of message
	switch t := (*message).(type) {
	case etf.Atom:
		// If message is atom 'stop', we should say it to main process
		if string(t) == "stop" {
			gs.completeChan <- true
		}
	}
}

// HandleCall handles incoming messages from `gen_server:call/2`, if returns non-nil term,
// then calling process have reply
// Call `gen_server:call({go_srv, gonode@localhost}, Message)` at Erlang node
func (gs *gonodeSrv) HandleCall(message *etf.Term, from *etf.Tuple) (reply *etf.Term) {
	log.Printf("GO_SRV: HandleCall: %#v, From: %#v", *message, *from)

	// Just create new term tuple where first element is atom 'ok', second 'go_reply' and third is original message
	replyTerm := etf.Term(etf.Tuple{etf.Atom("ok"), etf.Atom("go_reply"), *message})
	reply = &replyTerm
	return
}

// HandleInfo handles all another incoming messages
func (gs *gonodeSrv) HandleInfo(message *etf.Term) {
	log.Printf("GO_SRV: HandleInfo: %#v", *message)
}

// Terminate called when process died
func (gs *gonodeSrv) Terminate(reason interface{}) {
	log.Printf("GO_SRV: Terminate: %#v", reason.(int))
}

func main() {
	// Initialize new node with given name and cookie
	enode := node.NewNode("gonode@localhost", "123")

	// Allow node be available on 5588 port
	err := enode.Publish(5588)
	if err != nil {
		log.Fatalf("Cannot publish: %s", err)
	}

	// Create channel to receive message when main process should be stopped
	completeChan := make(chan bool)

	// Initialize new instance of gonodeSrv structure which implements Process behaviour
	eSrv := new(gonodeSrv)

	// Spawn process with one arguments
	enode.Spawn(eSrv, completeChan)

	// RPC
	// Create closure
	eClos := func(terms etf.List) (r etf.Term) {
		r = etf.Term(etf.Tuple{etf.Atom("gonode"), etf.Atom("reply"), len(terms)})
		return
	}

	// Provide it to call via RPC with `rpc:call(gonode@localhost, go_rpc, call, [as, qwe])`
	err = enode.RpcProvide("go_rpc", "call", eClos)
	if err != nil {
		log.Printf("Cannot provide function to RPC: %s", err)
	}


	// Wait to stop
	<-completeChan

	return
}
