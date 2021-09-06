package main

import (
	"fmt"

	"github.com/halturin/ergo"
	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/gen"
	"github.com/halturin/ergo/node"
)

type MyDemo struct {
	GenDemo
}

func (md *MyDemo) InitDemo(process *GenDemoProcess, args ...etf.Term) error {
	fmt.Printf("Started instance of MyDemo with PID %s and args %v\n", process.Self(), args)
	return nil
}

func (md *MyDemo) HandleHello(process *GenDemoProcess) {
	fmt.Println("got Hello")
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

	fmt.Println("call Hi method (no handler)")
	demo.Hi(process)

	fmt.Println("make simple call (no handler)")
	process.Call(process.Self(), "simple message")

	fmt.Println("Exiting")
	process.Exit("normal")
	process.Wait()
	node.Stop()
	node.Wait()
	return
}
