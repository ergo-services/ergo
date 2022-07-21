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

	fmt.Println("")
	fmt.Println("to stop press Ctrl-C")
	fmt.Println("")

	// create nodes for producer and consumers
	fmt.Println("Starting nodes 'node_abc@localhost' and 'node_def@localhost'")
	node_abc, _ := ergo.StartNode("node_abc@localhost", "cookies", node.Options{})
	node_def, _ := ergo.StartNode("node_def@localhost", "cookies", node.Options{})

	// create producer and consumer objects
	producer := &Producer{}
	consumer := &Consumer{}

	fmt.Println("Spawn producer on 'node_abc@localhost'")
	_, errP := node_abc.Spawn("producer", gen.ProcessOptions{}, producer, nil)
	if errP != nil {
		panic(errP)
	}
	fmt.Println("Spawn 2 consumers on 'node_def@localhost'")
	_, errC1 := node_def.Spawn("even", gen.ProcessOptions{}, consumer, true)
	if errC1 != nil {
		panic(errC1)
	}
	_, errC2 := node_def.Spawn("odd", gen.ProcessOptions{}, consumer, false)
	if errC2 != nil {
		panic(errC2)
	}

	node_abc.Wait()

}
