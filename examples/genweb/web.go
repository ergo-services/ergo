package main

import (
	"net/http"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/lib"
)

type webServer struct {
	gen.Web
}

func (w *webServer) InitWeb(process *gen.WebProcess, args ...etf.Term) (gen.WebOptions, error) {
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

	mux := http.NewServeMux()
	whOptions := gen.WebHandlerOptions{
		MaxHandlers:    200,
		IdleTimeout:    10,
		RequestTimeout: 20,
	}
	webRoot := process.StartWebHandler(&rootHandler{}, whOptions)
	webTime := process.StartWebHandler(&timeHandler{}, whOptions)
	mux.Handle("/", webRoot)
	mux.Handle("/time/", webTime)
	options.Handler = mux

	return options, nil
}
