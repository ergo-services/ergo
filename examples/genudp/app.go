package main

import (
	"fmt"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
)

type udpApp struct {
	gen.Application
}

func (ua *udpApp) Load(args ...etf.Term) (gen.ApplicationSpec, error) {
	return gen.ApplicationSpec{
		Name:        "udpApp",
		Description: "Demo UDP Applicatoin",
		Version:     "v.1.0",
		Children: []gen.ApplicationChildSpec{
			gen.ApplicationChildSpec{
				Child: &udpServer{}, // udp_server.go
				Name:  "udp",
			},
		},
	}, nil
}

func (ua *udpApp) Start(process gen.Process, args ...etf.Term) {
	fmt.Println("Application started!")
}
