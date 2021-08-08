package main

import (
	"fmt"

	"github.com/halturin/ergo"
	"github.com/halturin/ergo/etf"
)

type App struct {
	ergo.Application
}

var (
	handler_sup = &HandlerSup{}
)

func (a *App) Load(args ...etf.Term) (ergo.ApplicationSpec, error) {
	return ergo.ApplicationSpec{
		Name:        "WebApp",
		Description: "Demo Web Application",
		Version:     "v.1.0",
		Environment: map[string]interface{}{},
		Children: []ergo.ApplicationChildSpec{
			ergo.ApplicationChildSpec{
				Child: handler_sup,
				Name:  "handler_sup",
			},
		},
	}, nil
}

func (a *App) Start(process *ergo.Process, args ...etf.Term) {
	fmt.Println("Application started!")
}
