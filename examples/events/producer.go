package main

import (
	"fmt"
	"time"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/lib"
)

type producer struct {
	gen.Server
}

func (p *producer) Init(process *gen.ServerProcess, args ...etf.Term) error {

	if err := process.RegisterEvent(simpleEvent, messageSimpleEvent{}); err != nil {
		lib.Warning("can't register event %q: %s", simpleEvent, err)
	}
	fmt.Printf("process %s registered event %s\n", process.Self(), simpleEvent)
	process.SendAfter(process.Self(), 1, time.Second)
	return nil
}

func (p *producer) HandleInfo(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	n := message.(int)
	if n > 5 {
		return gen.ServerStatusStop
	}
	// sending message with delay 1 second
	process.SendAfter(process.Self(), n+1, time.Second)
	event := messageSimpleEvent{
		e: fmt.Sprintf("EVNT %d", n),
	}

	fmt.Printf("... producing event: %s\n", event)
	if err := process.SendEventMessage(simpleEvent, event); err != nil {
		fmt.Println("can't send event:", err)
	}
	return gen.ServerStatusOK
}
