package main

import (
	"fmt"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
)

func createDemoApp() gen.ApplicationBehavior {
	return &demoApp{}
}

type demoApp struct {
	gen.Application
}

func (da *demoApp) Load(args ...etf.Term) (gen.ApplicationSpec, error) {
	return gen.ApplicationSpec{
		Name:        "demoApp",
		Description: "Demo Applicatoin",
		Version:     "v.1.0",
		Children: []gen.ApplicationChildSpec{
			gen.ApplicationChildSpec{
				Child: createDemoSup(),
				Name:  "demoSup",
			},
			gen.ApplicationChildSpec{
				Child: createDemoServer(),
				Name:  "demoServer",
			},
		},
	}, nil
}

func (da *demoApp) Start(process gen.Process, args ...etf.Term) {
	fmt.Printf("Application started with Pid %s!\n", process.Self())
}
