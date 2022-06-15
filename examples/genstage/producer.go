package main

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
)

type Producer struct {
	gen.Stage
	dispatcher gen.StageDispatcherBehavior
}

func (p *Producer) InitStage(process *gen.StageProcess, args ...etf.Term) (gen.StageOptions, error) {
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

	options := gen.StageOptions{
		Dispatcher: gen.CreateStageDispatcherPartition(3, hash),
	}
	return options, nil
}
func (p *Producer) HandleDemand(process *gen.StageProcess, subscription gen.StageSubscription, count uint) (etf.List, gen.StageStatus) {
	fmt.Println("Producer: just got demand for", count, "event(s) from", subscription.Pid)
	numbers := generateNumbers(int(count) + 3)
	fmt.Println("Producer. Generate random numbers and send them to consumers...", numbers)
	process.SendEvents(numbers)
	time.Sleep(500 * time.Millisecond)
	return nil, gen.StageStatusOK
}

func (p *Producer) HandleSubscribe(process *gen.StageProcess, subscription gen.StageSubscription, options gen.StageSubscribeOptions) gen.StageStatus {
	fmt.Println("New subscription from:", subscription.Pid, "with min:", options.MinDemand, "and max:", options.MaxDemand)
	return gen.StageStatusOK
}

func generateNumbers(n int) etf.List {
	l := etf.List{}
	for n > 0 {
		l = append(l, rand.Intn(100))
		n--
	}
	return l
}
