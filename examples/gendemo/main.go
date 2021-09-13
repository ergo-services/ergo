package main

import (
	"fmt"

	"github.com/halturin/ergo"
	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/gen"
	"github.com/halturin/ergo/node"
)

type MyDemo struct {
	Demo
}

func (md *MyDemo) InitDemo(process *DemoProcess, args ...etf.Term) error {
	fmt.Printf("Started instance of MyDemo with PID %s and args %v\n", process.Self(), args)
	return nil
}

func (md *MyDemo) HandleHello(process *DemoProcess) DemoStatus {
	fmt.Println("got Hello")
	return DemoStatusOK
}

func (md *MyDemo) HandleDemoDirect(process *DemoProcess, message interface{}) (interface{}, gen.ServerStatus) {

	fmt.Println("Say hi to increase counter twice")
	process.Hi()
	return nil, gen.ServerStatusOK
}

func main() {

	// Initialize new node with given name, cookie, listening port range and epmd port
	node, e := ergo.StartNode("mydemo@127.0.0.1", "cookie", node.Options{})
	if e != nil {
		fmt.Println("error", e)
		return
	}

	demo := &MyDemo{}
	// Spawn process with one arguments
	process, e := node.Spawn("demo", gen.ProcessOptions{}, demo, 1, 2, 3)
	if e != nil {
		fmt.Println("error", e)
		return
	}

	fmt.Println("call Hello method 3 times")
	demo.Hello(process)
	demo.Hello(process)
	demo.Hello(process)

	fmt.Println("How many times Hello was called: ", demo.Stat(process))

	fmt.Println("make simple cast (no handler)")
	process.Cast(process.Self(), "simple message")

	fmt.Println("make direct request")
	process.Direct(nil)
	fmt.Println("How big is the counter now: ", demo.Stat(process))

	fmt.Println("Exiting")
	process.Exit("normal")
	process.Wait()
	node.Stop()
	node.Wait()
	return
}
