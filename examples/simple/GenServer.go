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

func (egs *ExampleGenServer) HandleInfo(message etf.Term, state ergo.GenServerState) string {
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
	node := ergo.CreateNode("node@localhost", "cookies", ergo.NodeOptions{})

	// spawn new process of genserver
	process, _ := node.Spawn("gs1", ergo.ProcessOptions{}, &ExampleGenServer{})

	// sending message to itself
	process.Send(process.Self(), 100)

	// waiting for the process termination.
	process.Wait()
	fmt.Println("exited")
	node.Stop()
}
