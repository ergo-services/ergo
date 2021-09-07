package main

import (
	"fmt"
	"time"

	"github.com/halturin/ergo"
	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/gen"
	"github.com/halturin/ergo/node"
)

// simple simple implementation of Server
type simple struct {
	gen.Server
}

func (s *simple) HandleInfo(process *gen.ServerProcess, message etf.Term) string {
	value := message.(int)
	fmt.Printf("HandleInfo: %#v \n", message)
	if value > 104 {
		return "stop"
	}
	// sending message with delay
	process.SendAfter(process.Self(), value+1, time.Duration(1*time.Second))
	return "noreply"
}

func main() {
	// create a new node
	node, _ := ergo.StartNode("node@localhost", "cookies", node.Options{})

	// spawn a new process of gen.Server
	process, _ := node.Spawn("gs1", gen.ProcessOptions{}, &simple{})

	// send a message to itself
	process.Send(process.Self(), 100)

	// wait for the process termination.
	process.Wait()
	fmt.Println("exited")
	node.Stop()
}
