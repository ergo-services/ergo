package main

import (
	"fmt"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
)

func createDemoServer() gen.ServerBehavior {
	return &demoServer{}
}

type demoServer struct {
	gen.Server
}

func (ds *demoServer) Init(process *gen.ServerProcess, args ...etf.Term) error {
	fmt.Printf("Started new process\n\tPid: %s\n\tName: %q\n\tParent: %s\n\tArgs:%#v\n",
		process.Self(),
		process.Name(),
		process.Parent().Self(),
		args)
	return nil
}
