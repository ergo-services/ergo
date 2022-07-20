package main

import (
	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
)

func createDemoSup() gen.SupervisorBehavior {
	return &demoSup{}
}

type demoSup struct {
	gen.Supervisor
}

func (ds *demoSup) Init(args ...etf.Term) (gen.SupervisorSpec, error) {
	spec := gen.SupervisorSpec{
		Name: "demoAppSup",
		Children: []gen.SupervisorChildSpec{
			gen.SupervisorChildSpec{
				Name:  "demoServer01",
				Child: createDemoServer(),
			},
			gen.SupervisorChildSpec{
				Name:  "demoServer02",
				Child: createDemoServer(),
				Args:  []etf.Term{12345},
			},
			gen.SupervisorChildSpec{
				Name:  "demoServer03",
				Child: createDemoServer(),
				Args:  []etf.Term{"abc", 67890},
			},
		},
		Strategy: gen.SupervisorStrategy{
			Type:      gen.SupervisorStrategyOneForAll,
			Intensity: 2,
			Period:    5,
			Restart:   gen.SupervisorStrategyRestartTemporary,
		},
	}
	return spec, nil
}
