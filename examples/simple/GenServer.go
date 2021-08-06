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
}

func (egs *ExampleGenServer) HandleDirect(state *ergo.GenServerState, message interface{}) (interface{}, error) {
	state.Process.Send(state.Process.Self(), message)
	return nil, nil
}

func (egs *ExampleGenServer) HandleInfo(state *ergo.GenServerState, message etf.Term) string {
	value := message.(int)
	fmt.Printf("HandleInfo: %#v \n", message)
	if value > 104 {
		return "stop"
	}
	// sending message with delay
	state.Process.SendAfter(state.Process.Self(), value+1, time.Duration(1*time.Second))
	return "noreply"
}

func main() {
	// create a new node
	node, _ := ergo.CreateNode("node@localhost", "cookies", ergo.NodeOptions{})

	// spawn new process of genserver
	process, _ := node.Spawn("gs1", ergo.ProcessOptions{}, &ExampleGenServer{})

	// send a direct message to itself
	process.Direct(100)

	// wait for the process termination.
	process.Wait()
	fmt.Println("exited")
	node.Stop()
}
