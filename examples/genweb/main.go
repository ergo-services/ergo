package main

import (
	"flag"
	"fmt"

	"github.com/ergo-services/ergo"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/node"
)

var (
	WebListenPort int
	WebListenHost string
	WebEnableTLS  bool
)

func init() {
	flag.IntVar(&WebListenPort, "port", 8080, "listen port number. Default port 8080, for TLS 8443")
	flag.StringVar(&WebListenHost, "host", "localhost", "listen on host")
	flag.BoolVar(&WebEnableTLS, "tls", false, "enable TLS")
}

func main() {
	fmt.Println("")
	fmt.Println("to stop press Ctrl-C")
	fmt.Println("")

	flag.Parse()

	opts := node.Options{
		Applications: []gen.ApplicationBehavior{
			&webApp{}, // app.go
		},
	}

	webNode, err := ergo.StartNode("web@127.0.0.1", "secret", opts)
	if err != nil {
		panic(err)
	}

	webNode.Wait()
}
