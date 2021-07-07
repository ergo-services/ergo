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

type ExampleState struct {
	value int
}

func (egs *ExampleGenServer) Init(p *ergo.Process, args ...interface{}) (interface{}, error) {
	fmt.Printf("Init: args %v \n", args)
	egs.process = p
	InitialState := &ExampleState{
		value: args[0].(int), // 100
	}
	return InitialState, nil
}

func (egs *ExampleGenServer) HandleCast(message etf.Term, state ergo.GenServerState) string {
	st := state.State.(*ExampleState)
	fmt.Printf("HandleCast: %#v (state value %d) \n", message, st.value)
	time.Sleep(1 * time.Second)
	state.State.(*ExampleState).value++

	if st.value > 103 {
		state.Process.Send(state.Process.Self(), "hello")
	} else {
		state.Process.Cast(state.Process.Self(), "hi")
	}

	return "noreply"
}

func (egs *ExampleGenServer) HandleCall(from etf.Tuple, message etf.Term, state ergo.GenServerState) (string, etf.Term) {
	fmt.Printf("HandleCall: %#v, From: %#v\n", message, from)
	return "reply", message
}

func (egs *ExampleGenServer) HandleInfo(message etf.Term, state ergo.GenServerState) string {
	st := state.State.(*ExampleState)
	fmt.Printf("HandleInfo: %#v (state value %d) \n", message, st.value)
	time.Sleep(1 * time.Second)
	st.value++
	if st.value > 106 {
		return "stop"
	} else {
		state.Process.Send(state.Process.Self(), "hello")
	}
	return "noreply"
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
