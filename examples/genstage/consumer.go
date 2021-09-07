package main

import (
	"fmt"

	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/gen"
)

type Consumer struct {
	gen.Stage
}

func (c *Consumer) InitStage(process *gen.StageProcess, args ...etf.Term) error {
	return nil
}
func (c *Consumer) HandleEvents(process *gen.StageProcess, subscription gen.StageSubscription, events etf.List) error {
	fmt.Printf("Consumer '%s' got events: %v\n", process.Name(), events)
	return nil
}
