package main

import (
	"fmt"

	"github.com/ergo-services/ergo"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/node"
)

func main() {

	fmt.Printf("Starting node: node1@localhost and Raft1 process...")
	node1, err := ergo.StartNode("node1@localhost", "cookies", node.Options{})
	if err != nil {
		panic(err)
	}
	defer node1.Stop()
	raft1 := &Raft1{}
	raft1_process, err := node1.Spawn("raft1", gen.ProcessOptions{}, raft1)
	if err != nil {
		panic(err)
	}
	fmt.Println("OK")

	fmt.Printf("Starting node: node2@localhost and Raft2 process...")
	node2, err := ergo.StartNode("node2@localhost", "cookies", node.Options{})
	if err != nil {
		panic(err)
	}
	defer node2.Stop()
	raft2 := &Raft2{}
	raft2_process, err := node2.Spawn("raft2", gen.ProcessOptions{}, raft2)
	if err != nil {
		panic(err)
	}
	fmt.Println("OK")

	fmt.Printf("Starting node: node3@localhost and Raft3 process...")
	node3, err := ergo.StartNode("node3@localhost", "cookies", node.Options{})
	if err != nil {
		panic(err)
	}
	defer node3.Stop()
	raft3 := &Raft3{}
	raft3_process, err := node3.Spawn("raft3", gen.ProcessOptions{}, raft3)
	if err != nil {
		panic(err)
	}
	fmt.Println("OK")

	fmt.Printf("Starting node: node4@localhost and Raft4 process...")
	node4, err := ergo.StartNode("node4@localhost", "cookies", node.Options{})
	if err != nil {
		panic(err)
	}
	defer node4.Stop()
	raft4 := &Raft4{}
	raft4_process, err := node4.Spawn("raft4", gen.ProcessOptions{}, raft4)
	if err != nil {
		panic(err)
	}
	fmt.Println("OK")

	fmt.Println(raft1_process.Self(), raft2_process.Self(), raft3_process.Self(), raft4_process.Self())

}
