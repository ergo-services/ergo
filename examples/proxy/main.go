package main

import (
	"fmt"

	"github.com/ergo-services/ergo"
	"github.com/ergo-services/ergo/node"
)

func main() {
	fmt.Printf("Starting node: node1@localhost and p1 process (cluster 1)...")
	node1, err := ergo.StartNode("node1@localhost", "cluster1", node.Options{})
	if err != nil {
		panic(err)
	}
	//	saga1 := &Saga1{}
	//	p1, err := node1.Spawn("saga1", gen.ProcessOptions{MailboxSize: 10000}, saga1)
	//	if err != nil {
	//		panic(err)
	//	}
	fmt.Println("OK")

	fmt.Printf("Starting node: node2@localhost (cluster 1)...")
	node2, err := ergo.StartNode("node2@localhost", "cluster1", node.Options{})
	if err != nil {
		panic(err)
	}
	//	saga2 := &Saga2{}
	//	_, err = node2.Spawn("saga2", gen.ProcessOptions{MailboxSize: 10000}, saga2)
	//	if err != nil {
	//		panic(err)
	//	}
	fmt.Println("OK")

	fmt.Printf("Starting node: node3@localhost (cluster 2)...")
	node3, err := ergo.StartNode("node3@localhost", "cluster2", node.Options{})
	if err != nil {
		panic(err)
	}
	//	saga3 := &Saga3{}
	//	_, err = node3.Spawn("saga3", gen.ProcessOptions{MailboxSize: 10000}, saga3)
	//	if err != nil {
	//		panic(err)
	//	}
	fmt.Println("OK")

	fmt.Printf("Starting node: node4@localhost and p4 process (cluster 2)...")
	node4, err := ergo.StartNode("node4@localhost", "cluster2", node.Options{})
	if err != nil {
		panic(err)
	}
	//	saga4 := &Saga4{}
	//	_, err = node4.Spawn("saga4", gen.ProcessOptions{MailboxSize: 10000}, saga4)
	//	if err != nil {
	//		panic(err)
	//	}
	fmt.Println("OK")

	routeOptions := node.RouteOptions{
		Cookie: "cluster2",
	}
	if err := node2.AddStaticRouteOptions(node3.Name(), routeOptions); err != nil {
		fmt.Println("ERR add ", err)

	}
	if err := node2.Connect(node3.Name()); err != nil {
		fmt.Println("ERR", err)
	}

	node1.Stop()
	node2.Stop()
	node3.Stop()
	node4.Stop()
}
