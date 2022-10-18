package main

import (
	"fmt"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
)

type consumer struct {
	gen.Server
}

func (c *consumer) Init(process *gen.ServerProcess, args ...etf.Term) error {
	if err := process.MonitorEvent(simpleEvent); err != nil {
		return err
	}
	return nil
}

func (c *consumer) HandleInfo(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	switch message.(type) {
	case messageSimpleEvent:
		fmt.Println("consumer got event: ", message)
	case gen.MessageEventDown:
		fmt.Println("producer has terminated")
		return gen.ServerStatusStop
	default:
		fmt.Println("unknown message", message)
	}
	return gen.ServerStatusOK
}
