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

	apps := []gen.ApplicationBehavior{
		createDemoApp(),
	}
	opts := node.Options{
		Applications: apps,
	}
	demoNode, err := ergo.StartNode("app@localhost", "cookie", opts)
	if err != nil {
		panic(err)
	}
	demoNode.Wait()
}
