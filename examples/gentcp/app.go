package main

import (
	"fmt"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
)

type tcpApp struct {
	gen.Application
}

func (ta *tcpApp) Load(args ...etf.Term) (gen.ApplicationSpec, error) {
	return gen.ApplicationSpec{
		Name:        "tcpApp",
		Description: "Demo TCP Applicatoin",
		Version:     "v.1.0",
		Children: []gen.ApplicationChildSpec{
			gen.ApplicationChildSpec{
				Child: &tcpServer{}, // tcp_server.go
				Name:  "tcp",
			},
		},
	}, nil
}

func (ta *tcpApp) Start(process gen.Process, args ...etf.Term) {
	fmt.Println("Application started!")
}
