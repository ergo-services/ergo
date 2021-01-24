package main

import (
	"fmt"
	"time"

	"github.com/halturin/ergo"
	"github.com/halturin/ergo/etf"
)

// ExampleGenServer simple implementation of GenServer
type ExampleGenServer struct {
	ergo.GenServer
	process *ergo.Process
}

type State struct {
	value int
}

// Init initializes process state using arbitrary arguments
// Init -> state
func (egs *ExampleGenServer) Init(p *ergo.Process, args ...interface{}) (state interface{}) {
	fmt.Printf("Init: args %v \n", args)
	egs.process = p
	InitialState := &State{
		value: args[0].(int), // 100
	}
	return InitialState
}

// HandleCast -> ("noreply", state) - noreply
//		         ("stop", reason) - stop with reason
func (egs *ExampleGenServer) HandleCast(message etf.Term, state interface{}) (string, interface{}) {
	fmt.Printf("HandleCast: %#v (state value %d) \n", message, state.(*State).value)
	time.Sleep(1 * time.Second)
	state.(*State).value++

	if state.(*State).value > 103 {
		egs.process.Send(egs.process.Self(), "hello")
	} else {
		egs.process.Cast(egs.process.Self(), "hi")
	}

	return "noreply", state
}

// HandleCall serves incoming messages sending via gen_server:call
// HandleCall -> ("reply", message, state) - reply
//				 ("noreply", _, state) - noreply
//		         ("stop", reason, _) - normal stop
func (egs *ExampleGenServer) HandleCall(from etf.Tuple, message etf.Term, state interface{}) (string, etf.Term, interface{}) {
	fmt.Printf("HandleCall: %#v, From: %#v\n", message, from)
	return "reply", message, state
}

// HandleInfo serves all another incoming messages (Pid ! message)
// HandleInfo -> ("noreply", state) - noreply
//		         ("stop", reason) - normal stop
func (egs *ExampleGenServer) HandleInfo(message etf.Term, state interface{}) (string, interface{}) {
	fmt.Printf("HandleInfo: %#v (state value %d) \n", message, state.(*State).value)
	time.Sleep(1 * time.Second)
	state.(*State).value++
	if state.(*State).value > 106 {
		return "stop", "normal"
	} else {
		egs.process.Send(egs.process.Self(), "hello")
	}
	return "noreply", state
}

// Terminate called when process died
func (egs *ExampleGenServer) Terminate(reason string, state interface{}) {
	fmt.Printf("Terminate: %#v \n", reason)
}

func main() {
	// create a new node
	node := ergo.CreateNode("node@localhost", "cookies", ergo.NodeOptions{})
	gs1 := &ExampleGenServer{}

	// spawn new process of genserver
	process, _ := node.Spawn("gs1", ergo.ProcessOptions{}, gs1, 100)

	// self casting
	process.Cast(process.Self(), "hey")

	// waiting for the process termination.
	process.Wait()
	fmt.Println("exited")
}
