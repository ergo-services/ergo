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

	demoNode, err := ergo.StartNode("sup@localhost", "cookie", node.Options{})
	if err != nil {
		panic(err)
	}

	demoSup := createDemoSup()
	sup, err := demoNode.Spawn("demoSup", gen.ProcessOptions{}, demoSup)
	if err != nil {
		panic(err)
	}
	fmt.Println("Started supervisor process", sup.Self())
	demoNode.Wait()
}
