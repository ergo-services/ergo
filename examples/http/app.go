package main

import (
	"fmt"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
)

type App struct {
	gen.Application
}

var (
	handler_sup = &HandlerSup{}
)

func (a *App) Load(args ...etf.Term) (gen.ApplicationSpec, error) {
	return gen.ApplicationSpec{
		Name:        "WebApp",
		Description: "Demo Web Application",
		Version:     "v.1.0",
		Environment: map[string]interface{}{},
		Children: []gen.ApplicationChildSpec{
			gen.ApplicationChildSpec{
				Child: handler_sup,
				Name:  "handler_sup",
			},
		},
	}, nil
}

func (a *App) Start(process gen.Process, args ...etf.Term) {
	fmt.Println("Application started!")
}
