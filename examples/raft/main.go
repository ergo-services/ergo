package main

import (
	"fmt"
	"time"

	"github.com/ergo-services/ergo"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/node"
)

func main() {

	fmt.Println("")
	fmt.Println("to stop press Ctrl-C or wait 10 seconds...")
	fmt.Println("")

	fmt.Printf("Starting node: node1@localhost and raft1 process - ")
	node1, err := ergo.StartNode("node1@localhost", "cookies", node.Options{})
	if err != nil {
		panic(err)
	}
	defer node1.Stop()
	raft1 := &Raft{startSerial: 1}
	_, err = node1.Spawn("raft1", gen.ProcessOptions{}, raft1)
	if err != nil {
		panic(err)
	}
	fmt.Println("OK")

	fmt.Printf("Starting node: node2@localhost and raft2 process - ")
	node2, err := ergo.StartNode("node2@localhost", "cookies", node.Options{})
	if err != nil {
		panic(err)
	}
	defer node2.Stop()
	raft2 := &Raft{
		startSerial: 4,
		startPeers:  []gen.ProcessID{gen.ProcessID{Name: "raft1", Node: "node1@localhost"}},
	}
	_, err = node2.Spawn("raft2", gen.ProcessOptions{}, raft2)
	if err != nil {
		panic(err)
	}
	fmt.Println("OK")

	fmt.Printf("Starting node: node3@localhost and raft3 process - ")
	node3, err := ergo.StartNode("node3@localhost", "cookies", node.Options{})
	if err != nil {
		panic(err)
	}
	defer node3.Stop()
	raft3 := &Raft{
		startSerial: 4,
		startPeers:  []gen.ProcessID{gen.ProcessID{Name: "raft1", Node: "node1@localhost"}},
	}
	_, err = node3.Spawn("raft3", gen.ProcessOptions{}, raft3)
	if err != nil {
		panic(err)
	}
	fmt.Println("OK")

	fmt.Printf("Starting node: node4@localhost and raft4 process - ")
	node4, err := ergo.StartNode("node4@localhost", "cookies", node.Options{})
	if err != nil {
		panic(err)
	}
	defer node4.Stop()
	raft4 := &Raft{
		startSerial: 0,
		startPeers:  []gen.ProcessID{gen.ProcessID{Name: "raft1", Node: "node1@localhost"}},
	}
	_, err = node4.Spawn("raft4", gen.ProcessOptions{}, raft4)
	if err != nil {
		panic(err)
	}
	fmt.Println("OK")
	time.Sleep(10 * time.Second)
}
