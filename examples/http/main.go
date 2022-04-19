package main

import (
	"flag"
	"fmt"
	"net/http"

	"github.com/ergo-services/ergo"
	"github.com/ergo-services/ergo/node"
)

var (
	NodeName         string
	Cookie           string
	ListenRangeBegin int
	ListenRangeEnd   int = 35000
	Listen           string
	ListenEPMD       int
)

func init() {
	flag.StringVar(&NodeName, "name", "web@127.0.0.1", "node name")
	flag.StringVar(&Cookie, "cookie", "123", "cookie for interaction with erlang cluster")
}

func main() {
	flag.Parse()

	opts := node.Options{}

	// Initialize new node with given name, cookie, listening port range and epmd port
	nodeHTTP, _ := ergo.StartNode(NodeName, Cookie, opts)

	// start application
	if _, err := nodeHTTP.ApplicationLoad(&App{}); err != nil {
		panic(err)
	}

	process, _ := nodeHTTP.ApplicationStart("WebApp")

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		p := process.ProcessByName("handler_sup")
		if p == nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if handlerProcess, err := handler_sup.StartChild(p, "handler", r); err == nil {
			process.Send(handlerProcess.Self(), w)
			handlerProcess.Wait()
			return
		}

		w.WriteHeader(http.StatusInternalServerError)
	})

	go http.ListenAndServe(":8080", nil)
	fmt.Println("HTTP is listening on http://127.0.0.1:8080")

	process.Wait()
	nodeHTTP.Stop()
}
