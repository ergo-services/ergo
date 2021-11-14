package main

import (
	"fmt"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
)

type Consumer struct {
	gen.Stage
}

func (c *Consumer) InitStage(process *gen.StageProcess, args ...etf.Term) (gen.StageOptions, error) {
	var opts gen.StageSubscribeOptions
	even := args[0].(bool)
	if even {
		opts = gen.StageSubscribeOptions{
			MinDemand: 1,
			MaxDemand: 2,
			Partition: 0,
		}
	} else {
		opts = gen.StageSubscribeOptions{
			MinDemand: 2,
			MaxDemand: 4,
			Partition: 1,
		}
	}
	fmt.Println("Subscribe consumer", process.Name(), "[", process.Self(), "]",
		"with min events =", opts.MinDemand,
		"and max events", opts.MaxDemand)
	process.Subscribe(gen.ProcessID{Name: "producer", Node: "node_abc@localhost"}, opts)
	return gen.StageOptions{}, nil
}
func (c *Consumer) HandleEvents(process *gen.StageProcess, subscription gen.StageSubscription, events etf.List) gen.StageStatus {
	fmt.Printf("Consumer '%s' got events: %v\n", process.Name(), events)
	return gen.StageStatusOK
}
