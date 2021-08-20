package main

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/halturin/ergo"
	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/gen"
	"github.com/halturin/ergo/node"
)

func main() {
	// create nodes for producer and consumers
	fmt.Println("Starting nodes 'node_abc@localhost' and 'node_def@localhost'")
	node_abc, _ := ergo.StartNode("node_abc@localhost", "cookies", node.Options{})
	node_def, _ := ergo.StartNode("node_def@localhost", "cookies", node.Options{})

	// create producer and consumer objects
	producer := &Producer{}
	consumer := &Consumer{}

	fmt.Println("Spawn producer on 'node_abc@localhost'")
	p1, errP := node_abc.Spawn("producer", gen.ProcessOptions{}, producer, nil)
	if errP != nil {
		panic(errP)
	}
	fmt.Println("Spawn 2 consumers on 'node_def@localhost'")
	c1, errC1 := node_def.Spawn("even", gen.ProcessOptions{}, consumer, nil)
	if errC1 != nil {
		panic(errC1)
	}
	c2, errC2 := node_def.Spawn("odd", gen.ProcessOptions{}, consumer, nil)
	if errC2 != nil {
		panic(errC2)
	}

	fmt.Println("Subscribe consumer 'even' with min events = 1 and max events 2 (even numbers only)")
	c1_sub_opts := gen.StageSubscribeOptions{
		MinDemand: 1,
		MaxDemand: 2,
		Partition: 0,
	}
	consumer.Subscribe(c1, etf.Tuple{"producer", "node_abc@localhost"}, c1_sub_opts)

	fmt.Println("Subscribe consumer 'odd' with min events = 2 and max events 4 (odd numbers only)")
	c2_sub_opts := gen.StageSubscribeOptions{
		MinDemand: 2,
		MaxDemand: 4,
		Partition: 1,
	}
	consumer.Subscribe(c2, etf.Tuple{"producer", "node_abc@localhost"}, c2_sub_opts)

	for {
		n := rand.Intn(9) + 1
		numbers := generateNumbers(n)
		fmt.Println("Producer. Generate random numbers and send them to consumers...", numbers)
		producer.SendEvents(p1, numbers)
		time.Sleep(1 * time.Second)
	}

}

func generateNumbers(n int) etf.List {
	l := etf.List{}
	for n > 0 {
		l = append(l, rand.Intn(100))
		n--
	}
	return l
}
