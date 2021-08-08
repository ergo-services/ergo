package main

import (
	"github.com/halturin/ergo"
	"github.com/halturin/ergo/etf"
)

type HandlerSup struct {
	ergo.Supervisor
}

func (hs *HandlerSup) Init(args ...etf.Term) ergo.SupervisorSpec {
	return ergo.SupervisorSpec{
		Name: "handler_sup",
		Children: []ergo.SupervisorChildSpec{
			ergo.SupervisorChildSpec{
				Name:    "handler",
				Child:   &Handler{},
				Restart: ergo.SupervisorChildRestartTemporary,
			},
		},
		Strategy: ergo.SupervisorStrategy{
			Type:      ergo.SupervisorStrategySimpleOneForOne,
			Intensity: 5,
			Period:    5,
		},
	}
}
