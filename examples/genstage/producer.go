package main

import (
	"fmt"

	"github.com/halturin/ergo"
	"github.com/halturin/ergo/etf"
)

type Producer struct {
	ergo.GenStage
	dispatcher ergo.GenStageDispatcherBehavior
}

func (g *Producer) InitStage(process *ergo.Process, args ...interface{}) (ergo.GenStageOptions, interface{}) {
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

	options := ergo.GenStageOptions{
		Dispatcher: ergo.CreateGenStageDispatcherPartition(3, hash),
	}
	return options, nil
}
func (g *Producer) HandleDemand(subscription ergo.GenStageSubscription, count uint, state interface{}) (error, etf.List) {
	fmt.Println("Producer: just got demand for", count, "pack of events from", subscription.Pid)
	return nil, nil
}

func (g *Producer) HandleSubscribe(subscription ergo.GenStageSubscription, options ergo.GenStageSubscribeOptions, state interface{}) error {
	fmt.Println("New subscription from:", subscription.Pid, "with min:", options.MinDemand, "and max:", options.MaxDemand)
	return nil
}
