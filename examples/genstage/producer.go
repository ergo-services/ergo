package main

import (
	"fmt"

	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/gen"
)

type Producer struct {
	gen.Stage
	dispatcher gen.StageDispatcherBehavior
}

func (p *Producer) InitStage(process *gen.StageProcess, args ...etf.Term) error {
	// create a hash function for the dispatcher
	hash := func(t etf.Term) int {
		i, ok := t.(int)
		if !ok {
			// filtering out
			return -1
		}
		if i%2 == 0 {
			return 0
		}
		return 1
	}

	process.Options = gen.StageOptions{
		Dispatcher: gen.CreateStageDispatcherPartition(3, hash),
	}
	return nil
}
func (p *Producer) HandleDemand(process *gen.StageProcess, subscription gen.StageSubscription, count uint) (error, etf.List) {
	fmt.Println("Producer: just got demand for", count, "pack of events from", subscription.Pid)
	return nil, nil
}

func (p *Producer) HandleSubscribe(process *gen.StageProcess, subscription gen.StageSubscription, options gen.StageSubscribeOptions) error {
	fmt.Println("New subscription from:", subscription.Pid, "with min:", options.MinDemand, "and max:", options.MaxDemand)
	return nil
}
