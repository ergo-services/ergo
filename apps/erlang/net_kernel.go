package erlang

// https://github.com/erlang/otp/blob/master/lib/kernel/src/net_kernel.erl

import (
	"context"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/lib"
)

func CreateApp() gen.ApplicationBehavior {
	return &kernelApp{}
}

// KernelApp
type kernelApp struct {
	gen.Application
}

// Load
func (nka *kernelApp) Load(args ...etf.Term) (gen.ApplicationSpec, error) {
	return gen.ApplicationSpec{
		Name:        "erlang",
		Description: "Erlang support app",
		Version:     "v.1.0",
		Children: []gen.ApplicationChildSpec{
			{
				Child: &netKernelSup{},
				Name:  "net_kernel_sup",
			},
		},
	}, nil
}

// Start
func (nka *kernelApp) Start(p gen.Process, args ...etf.Term) {}

type netKernelSup struct {
	gen.Supervisor
}

// Init
func (nks *netKernelSup) Init(args ...etf.Term) (gen.SupervisorSpec, error) {
	return gen.SupervisorSpec{
		Children: []gen.SupervisorChildSpec{
			{
				Name:  "net_kernel",
				Child: &netKernel{},
			},
			{
				Name:  "global_name_server",
				Child: &globalNameServer{},
			},
			{
				Name:  "erlang",
				Child: &erlang{},
			},
		},
		Strategy: gen.SupervisorStrategy{
			Type:      gen.SupervisorStrategyOneForOne,
			Intensity: 10,
			Period:    5,
			Restart:   gen.SupervisorStrategyRestartPermanent,
		},
	}, nil
}

type netKernel struct {
	gen.Server
	routinesCtx map[etf.Pid]context.CancelFunc
}

// Init
func (nk *netKernel) Init(process *gen.ServerProcess, args ...etf.Term) error {
	lib.Log("NET_KERNEL: Init: %#v", args)
	nk.routinesCtx = make(map[etf.Pid]context.CancelFunc)
	return nil
}

// HandleCall
func (nk *netKernel) HandleCall(process *gen.ServerProcess, from gen.ServerFrom, message etf.Term) (reply etf.Term, status gen.ServerStatus) {
	lib.Log("NET_KERNEL: HandleCall: %#v, From: %#v", message, from)
	return
}

// HandleInfo
func (nk *netKernel) HandleInfo(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	lib.Log("NET_KERNEL: HandleInfo: %#v", message)
	return gen.ServerStatusOK
}
