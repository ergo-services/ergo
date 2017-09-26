This project based on https://github.com/goerlang but everything has been merged
and refactored into this project and has no backward capability

# Node #

Implementation of Erlang/OTP node in Go

Supported features:

 * Publish listen port via EPMD
 * Handle incoming connection from other node using Erlang Distribution Protocol
 * Spawn Erlang-like processes
 * Register and unregister processes with simple atom
 * Send messages to registered (using atom) or not registered (using Pid) processes at Go-node or remote Erlang-node
 * Create own process with `GenServer` behaviour (like `gen_server` in Erlang/OTP)
 * Initiate connection to other node
 * RPC callbacks

Not supported:

 * Supervisors tree
 * Create own behaviours

## Usage ##
```golang

type goGenServ struct {
    ergonode.GenServerImpl
    completeChan chan bool
}

Node := ergonode.Create("examplenode@127.0.0.1", 21234, "SecretCookie")
completeChan := make(chan bool)
gs := new(goGenServ)

n.Spawn(gs, completeChan)

message := etf.Term(etf.Atom("hello"))

// gen_server:call({pname, 'node@address'} , hello)
to := etf.Tuple{etf.Atom("pname"), etf.Atom("node@address")}

answer := gs.Call(to, message)
fmt.Printf("Got response: %v\n", answer)

// it's also possible to call using Pid (etf.Pid)
answer := gs.Call(Pid, message)

// gen_server:cast({pname, 'node@address'} , hello)
to := etf.Tuple{etf.Atom("pname"), etf.Atom("node@address")}
gs.Cast(to, message)

// the same way using Pid
gs.Cast(Pid, message)

// simple sending message 'Pid ! hello'
gs.Send(Pid, message)

// to get pid of itself like it does erlang:self()
gs.Self()

/*
 *  Here is simple example how is serving incoming messages.
 *  Interface implementation
 */

// Init initializes process state using arbitrary arguments
func (gs *goGenServ) Init(args ...interface{}) {
    // Self-registration with name go_srv
    gs.Node.Register(etf.Atom(SrvName), gs.Self)
}


// HandleCast serves incoming messages sending via gen_server:cast
func (gs *goGenServ) HandleCast(message *etf.Term) {

}

// HandleCall serves incoming messages sending via gen_server:call
func (gs *goGenServ) HandleCall(from *etf.Tuple, message *etf.Term) (reply *etf.Term) {
    replyTerm = etf.Term(etf.Atom("ok"))
    reply = &replyTerm

    return
}

// HandleInfo serves all another incoming messages (Pid ! message)
func (gs *goGenServ) HandleInfo(message *etf.Term) {
    fmt.Printf("HandleInfo: %#v\n", *message)
}

// Terminate called when process died
func (gs *goGenServ) Terminate(reason interface{}) {
    fmt.Printf("Terminate: %#v\n", reason.(int))
}


```

## Example ##

See `examples/` to see simple implementation of node and `GenServer` process
