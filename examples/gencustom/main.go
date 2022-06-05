package main

import (
	"fmt"
	"time"

	"github.com/ergo-services/ergo"
	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/node"
)

type MyCustom struct {
	Custom
}

func (md *MyCustom) InitCustom(process *CustomProcess, args ...etf.Term) error {
	fmt.Printf("Started instance of MyCustom with PID %s and args %v\n", process.Self(), args)
	return nil
}

func (md *MyCustom) HandleHello(process *CustomProcess) CustomStatus {
	fmt.Println("got Hello")
	return CustomStatusOK
}

func (md *MyCustom) HandleCustomDirect(process *CustomProcess, message interface{}) (interface{}, error) {

	fmt.Println("Say hi to increase counter twice")
	process.Hi()
	return nil, nil
}

func main() {

	// Initialize new node with given name, cookie, listening port range and epmd port
	node, e := ergo.StartNode("mydemo@127.0.0.1", "cookie", node.Options{})
	if e != nil {
		fmt.Println("error", e)
		return
	}

	demo := &MyCustom{}
	// Spawn a new process with arguments
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

	fmt.Println("make direct request")
	process.Direct(nil)
	fmt.Println("How big is the counter now: ", demo.Stat(process))

	fmt.Println("send a simple message (no handler) and call Exit")
	process.Send(process.Self(), "simple message")

	// add some delay to see "unhandled message" warning
	time.Sleep(100 * time.Millisecond)

	fmt.Println("Exiting")
	process.Exit("normal")
	process.Wait()
	node.Stop()
	node.Wait()
	return
}
