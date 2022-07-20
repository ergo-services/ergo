package main

import (
	"fmt"
	"time"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
)

// simple implementation of Server
type simple struct {
	gen.Server
}

func (s *simple) HandleInfo(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	value := message.(int)
	fmt.Printf("HandleInfo: %#v \n", message)
	if value > 104 {
		return gen.ServerStatusStop
	}
	// sending message with delay 1 second
	fmt.Println("increase this value by 1 and send it to itself again")
	process.SendAfter(process.Self(), value+1, time.Second)
	return gen.ServerStatusOK
}
