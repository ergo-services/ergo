package main

import (
	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/lib"
)

type web struct {
	gen.Web
}

func (w *web) InitWeb(process *gen.WebProcess, args ...etf.Term) (gen.WebOptions, error) {
	var options gen.WebOptions

	options.Port = uint16(WebListenPort)
	options.Host = WebListenHost
	if WebEnableTLS {
		cert, err := lib.GenerateSelfSignedCert("gen.Web demo")
		if err != nil {
			return options, err
		}
		options.Cert = cert
	}

	return options, nil
}
