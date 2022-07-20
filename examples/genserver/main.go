package main

import (
	"flag"
	"fmt"

	"github.com/ergo-services/ergo"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/node"
)

func main() {
	flag.Parse()

	fmt.Println("Start node mynode@localhost")
	myNode, _ := ergo.StartNode("mynode@localhost", "cookies", node.Options{})

	process, _ := myNode.Spawn("simple", gen.ProcessOptions{}, &simple{})
	fmt.Printf("Started process %s with name %q\n", process.Self(), process.Name())

	// send a message to itself
	fmt.Println("Send message 100 to itself")
	process.Send(process.Self(), 100)

	// wait for the prokcess termination.
	process.Wait()
	fmt.Println("Stop node", myNode.Name())
	myNode.Stop()
}
