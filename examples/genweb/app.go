package main

import (
	"fmt"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
)

type webApp struct {
	gen.Application
}

func (wa *webApp) Load(args ...etf.Term) (gen.ApplicationSpec, error) {
	return gen.ApplicationSpec{
		Name:        "webApp",
		Description: "Demo Web Applicatoin",
		Version:     "v.1.0",
		Children: []gen.ApplicationChildSpec{
			gen.ApplicationChildSpec{
				Child: &webServer{},
				Name:  "web",
			},
			gen.ApplicationChildSpec{
				Child: &timeServer{},
				Name:  "time",
			},
		},
	}, nil
}

func (wa *webApp) Start(process gen.Process, args ...etf.Term) {
	fmt.Println("Application started!")
}
