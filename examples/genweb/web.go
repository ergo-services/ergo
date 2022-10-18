package main

import (
	"crypto/tls"
	"fmt"
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
	proto := "http"
	if WebEnableTLS {
		cert, err := lib.GenerateSelfSignedCert("gen.Web demo")
		if err != nil {
			return options, err
		}
		options.TLS = &tls.Config{
			Certificates: []tls.Certificate{cert},
		}
		proto = "https"
	}

	mux := http.NewServeMux()
	whOptions := gen.WebHandlerOptions{
		NumHandlers:    50,
		IdleTimeout:    10,
		RequestTimeout: 20,
	}
	webRoot := process.StartWebHandler(&rootHandler{}, whOptions)
	webTime := process.StartWebHandler(&timeHandler{}, gen.WebHandlerOptions{})
	mux.Handle("/", webRoot)
	mux.Handle("/time/", webTime)
	options.Handler = mux

	fmt.Printf("Start Web server on %s://%s:%d/\n", proto, WebListenHost, WebListenPort)

	return options, nil
}
