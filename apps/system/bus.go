package system

import (
	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/lib"
)

type systemBus struct {
	gen.Server
}

type systemBusState struct{}

func (sb *systemBus) Init(process *gen.ServerProcess, args ...etf.Term) error {
	lib.Log("SYSTEM_BUS: Init: %#v", args)

	process.State = &systemBusState{}
	return nil
}

func (sb *systemBus) HandleCast(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	lib.Log("SYSTEM_BUS: HandleCast: %#v", message)
	return gen.ServerStatusOK
}

func (sb *systemBus) HandleInfo(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	lib.Log("SYSTEM_BUS: HandleInfo: %#v", message)
	return gen.ServerStatusOK
}

func (sb *systemBus) Terminate(process *gen.ServerProcess, reason string) {
	lib.Log("SYSTEM_BUS: Terminate with reason %q", reason)
}
