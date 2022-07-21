package main

import (
	"crypto/tls"
	"fmt"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/lib"
)

type tcpServer struct {
	gen.TCP
}

func (ts *tcpServer) InitTCP(process *gen.TCPProcess, args ...etf.Term) (gen.TCPOptions, error) {
	options := gen.TCPOptions{
		Host:    TCPListenHost,
		Port:    uint16(TCPListenPort),
		Handler: &tcpHandler{},
	}

	if TCPEnableTLS {
		cert, _ := lib.GenerateSelfSignedCert("localhost")
		fmt.Println("TLS enabled. Generated self signed certificate. You may check it with command below:")
		fmt.Printf("   $ openssl s_client -connect %s:%d\n", TCPListenHost, TCPListenPort)
		options.TLS = &tls.Config{
			Certificates:       []tls.Certificate{cert},
			InsecureSkipVerify: true,
		}
	}

	return options, nil
}
