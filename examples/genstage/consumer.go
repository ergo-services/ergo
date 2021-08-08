package main

import (
	"fmt"

	"github.com/halturin/ergo"
	"github.com/halturin/ergo/etf"
)

type Consumer struct {
	ergo.GenStage
}

func (g *Consumer) InitStage(state *ergo.GenStageState, args ...etf.Term) error {
	return nil
}
func (g *Consumer) HandleEvents(state *ergo.GenStageState, subscription ergo.GenStageSubscription, events etf.List) error {
	fmt.Printf("Consumer '%s' got events: %v\n", state.Process.Name(), events)
	return nil
}
