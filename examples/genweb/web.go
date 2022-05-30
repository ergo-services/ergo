package main

import (
	"fmt"
	"net/http"
	"time"

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

	mux := http.NewServeMux()
	root := process.StartWebHandler(&rootHandler{}, gen.WebHandlerOptions{})
	whOptions := gen.WebHandlerOptions{
		NumHandlers: 4,
	}
	user := process.StartWebHandler(&userHandler{}, whOptions)
	mux.Handle("/", root)
	mux.Handle("/root", root)
	mux.Handle("/user/", user)
	options.Handler = mux

	return options, nil
}

type userHandler struct {
	gen.WebHandler
}

func (u *userHandler) HandleRequest(process *gen.WebHandlerProcess, request gen.WebMessageRequest) gen.WebHandlerStatus {
	fmt.Println("user handle request", process.Self())
	time.Sleep(10 * time.Second)
	return gen.WebHandlerStatusOK
}

type rootHandler struct {
	gen.WebHandler
}

func (r *rootHandler) HandleRequest(process *gen.WebHandlerProcess, request gen.WebMessageRequest) gen.WebHandlerStatus {
	fmt.Println("root handle request", process.Self())
	request.Response.WriteHeader(http.StatusNotFound)
	return gen.WebHandlerStatusOK
}
