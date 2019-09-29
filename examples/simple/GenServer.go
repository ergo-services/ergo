package main

import (
	"fmt"
	"time"

	ergo "github.com/halturin/ergonode"
	"github.com/halturin/ergonode/etf"
)

type ExampleGenServer struct {
	ergo.GenServer
	process ergo.Process
}

type State struct {
	value int
}

func (egs *ExampleGenServer) Init(p ergo.Process, args ...interface{}) (state interface{}) {
	fmt.Printf("Init: args %v \n", args)
	egs.process = p
	InitialState := &State{
		value: args[0].(int), // 100
	}
	return InitialState
}

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

func (egs *ExampleGenServer) HandleCall(from etf.Tuple, message etf.Term, state interface{}) (string, etf.Term, interface{}) {
	fmt.Printf("HandleCall: %#v, From: %#v\n", message, from)
	return "reply", message, state
}

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
func (egs *ExampleGenServer) Terminate(reason string, state interface{}) {
	fmt.Printf("Terminate: %s \n", reason)
}

func main() {

	node := ergo.CreateNode("node@localhost", "cookies", ergo.NodeOptions{})
	gs1 := &ExampleGenServer{}
	process, _ := node.Spawn("gs1", ergo.ProcessOptions{}, gs1, 100)

	process.Cast(process.Self(), "hey")

	select {
	case <-process.Context.Done():
		fmt.Println("exited")
	}
}
