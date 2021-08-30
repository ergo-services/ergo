package main

import (
	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/gen"
)

type HandlerSup struct {
	gen.Supervisor
}

func (hs *HandlerSup) Init(args ...etf.Term) (gen.SupervisorSpec, error) {
	return gen.SupervisorSpec{
		Name: "handler_sup",
		Children: []gen.SupervisorChildSpec{
			gen.SupervisorChildSpec{
				Name:    "handler",
				Child:   &Handler{},
				Restart: gen.SupervisorChildRestartTemporary,
			},
		},
		Strategy: gen.SupervisorStrategy{
			Type:      gen.SupervisorStrategySimpleOneForOne,
			Intensity: 5,
			Period:    5,
		},
	}, nil
}
