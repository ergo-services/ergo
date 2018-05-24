# Node #

Implementation of Erlang/OTP node in Go

Features:

 * Publish listen port via EPMD
 * Handle incoming connection from other node using Erlang Distribution Protocol
 * Spawn Erlang-like processes
 * Register and unregister processes with simple atom
 * Send sync and async messages like `erlang:gen_call` and `erlang:gen_cast`
 * Create own process with `GenServer` behaviour (like `gen_server` in Erlang/OTP)
 * Atomic 'state' of GenServer
 * Initiate connection to other node
 * RPC callbacks
 * Monitor processes
 * Monitor nodes
 * Support Erlang 20.*

Requirement: Go 1.9 and above

## Usage ##

```golang

type goGenServ struct {
    ergonode.GenServer
    completeChan chan bool
}

Node := ergonode.Create("examplenode@127.0.0.1", 21234, "SecretCookie")
completeChan := make(chan bool)
gs := new(goGenServ)

n.Spawn(gs, completeChan)

message := etf.Term(etf.Atom("hello"))

// gen_server:call({pname, 'node@address'} , hello) with default timeout 5 seconds
to := etf.Tuple{etf.Atom("pname"), etf.Atom("node@address")}

answer, err := gs.Call(to, message)
fmt.Printf("Got response: %v\n", answer)

// specify the custom calling timeout
// gen_server:call({pname, 'node@address'} , hello, 8)
answer, err := gs.Call(Pid, message, 12)


// it's also possible to call using Pid (etf.Pid)
answer, err := gs.Call(Pid, message)

// gen_server:cast({pname, 'node@address'} , hello)
to := etf.Tuple{etf.Atom("pname"), etf.Atom("node@address")}
gs.Cast(to, message)

// the same way using Pid
gs.Cast(Pid, message)

// simple sending message 'Pid ! hello'
gs.Send(Pid, message)

// to get pid like it does erlang:self()
gs.Self()

// set monitor. this gen_server will recieve the message (via HandleInfo) like
// {'DOWN',#Ref<0.0.13893633.237772>,process,<26194.4.1>, Reason})
// in case of remote process went down by some reason
gs.Monitor(Pid)


// *** http://erlang.org/doc/man/erlang.html#monitor_node-2
// *** Making several calls to monitor_node(Node, true) for the same Node is not an error;
// *** it results in as many independent monitoring instances.
// seting up node monitor (will recieve {nodedown, Nodename})
gs.MonitorNode(etf.Atom("node@address"), true)
// removing monitor
gs.MonitorNode(etf.Atom("node@address"), false)

/*
 *  Simple example how are handling incoming messages.
 *  Interface implementation
 */

// Init initializes process state using arbitrary arguments
func (gs *goGenServ) Init(args ...interface{}) (state interface{}) {
    // Self-registration with name SrvName
    gs.Node.Register(etf.Atom(SrvName), gs.Self)
    return nil
}


// HandleCast serves incoming messages sending via gen_server:cast
// HandleCast -> (0, state) - noreply
//               (-1, state) - normal stop (-2, -3 .... custom reasons to stop)
func (gs *goGenServ) HandleCast(message *etf.Term, state interface{}) (code int, stateout interface{}) {
    return 0, state
}

// HandleCall serves incoming messages sending via gen_server:call
// HandleCall -> (1, reply, state) - reply
//               (0, _, state) - noreply
//               (-1, _, state) - normal stop (-2, -3 .... custom reasons to stop)
func (gs *goGenServ) HandleCall(from *etf.Tuple, message *etf.Term) (code int, reply *etf.Term, stateout interface{}) {
    reply = etf.Term(etf.Atom("ok"))
    return 1, &reply, state
}

// HandleInfo serves all another incoming messages (Pid ! message)
// HandleInfo -> (0, state) - noreply
//               (-1, state) - normal stop (-2, -3 .... custom reasons to stop)
func (gs *goGenServ) HandleInfo(message *etf.Term, state interface{}) (code int, stateout interface{}) {
    fmt.Printf("HandleInfo: %#v\n", *message)
    return 0, state
}

// Terminate called when process died
func (gs *goGenServ) Terminate(reason int, state interface{}) {
    fmt.Printf("Terminate: %#v\n", reason)
}


```

## Example ##

See `examples/` for simple implementation of node and `GenServer` process
