package main

import (
	"fmt"
	"time"

	"github.com/ergo-services/ergo"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/node"
)

func main() {
	fmt.Printf("Starting node: node1@localhost and Saga1 process...")
	node1, err := ergo.StartNode("node1@localhost", "cookies", node.Options{})
	if err != nil {
		panic(err)
	}
	saga1 := &Saga1{}
	saga1_process, err := node1.Spawn("saga1", gen.ProcessOptions{MailboxSize: 10000}, saga1)
	if err != nil {
		panic(err)
	}
	fmt.Println("OK")

	fmt.Printf("Starting node: node2@localhost and Saga2 process...")
	node2, err := ergo.StartNode("node2@localhost", "cookies", node.Options{})
	if err != nil {
		panic(err)
	}
	saga2 := &Saga2{}
	_, err = node2.Spawn("saga2", gen.ProcessOptions{MailboxSize: 10000}, saga2)
	if err != nil {
		panic(err)
	}
	fmt.Println("OK")

	fmt.Printf("Starting node: node3@localhost and Saga3 process...")
	node3, err := ergo.StartNode("node3@localhost", "cookies", node.Options{})
	if err != nil {
		panic(err)
	}
	saga3 := &Saga3{}
	_, err = node3.Spawn("saga3", gen.ProcessOptions{MailboxSize: 10000}, saga3)
	if err != nil {
		panic(err)
	}
	fmt.Println("OK")

	fmt.Printf("Starting node: node4@localhost and Saga4 process...")
	node4, err := ergo.StartNode("node4@localhost", "cookies", node.Options{})
	if err != nil {
		panic(err)
	}
	saga4 := &Saga4{}
	_, err = node4.Spawn("saga4", gen.ProcessOptions{MailboxSize: 10000}, saga4)
	if err != nil {
		panic(err)
	}
	fmt.Println("OK")

	args := startTX{
		done: make(chan bool),
	}
	saga1_process.Send(saga1_process.Self(), args)

	<-args.done
	time.Sleep(300 * time.Millisecond)

	node1.Stop()
	node2.Stop()
	node3.Stop()
	node4.Stop()
}
