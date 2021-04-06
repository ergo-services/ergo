package main

import (
	"fmt"

	"github.com/halturin/ergo"
	"github.com/halturin/ergo/etf"
)

type Consumer struct {
	ergo.GenStage
}

type ConsumerState struct {
	p *ergo.Process
}

func (g *Consumer) InitStage(process *ergo.Process, args ...interface{}) (ergo.GenStageOptions, interface{}) {
	// create a hash function for the dispatcher
	state := &ConsumerState{
		p: process,
	}

	return ergo.GenStageOptions{}, state
}
func (g *Consumer) HandleEvents(subscription ergo.GenStageSubscription, events etf.List, state interface{}) error {
	st := state.(*ConsumerState)
	fmt.Printf("Consumer '%s' got events: %v\n", st.p.Name(), events)
	return nil
}
