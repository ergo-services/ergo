package main

import (
	"flag"
	"fmt"

	"github.com/ergo-services/ergo"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/node"
)

const (
	simpleEvent gen.Event = "simple"
)

type messageSimpleEvent struct {
	e string
}

func main() {
	flag.Parse()

	fmt.Println("Start node eventsnode@localhost")
	myNode, _ := ergo.StartNode("eventsnode@localhost", "cookies", node.Options{})

	prod, _ := myNode.Spawn("producer", gen.ProcessOptions{}, &producer{})
	fmt.Printf("Started process %s with name %q\n", prod.Self(), prod.Name())

	cons, _ := myNode.Spawn("consumer", gen.ProcessOptions{}, &consumer{})
	fmt.Printf("Started process %s with name %q\n", cons.Self(), cons.Name())

	cons.Wait()
	fmt.Println("Stop node", myNode.Name())
	myNode.Stop()
}
