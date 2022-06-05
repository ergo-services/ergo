package main

import (
	"flag"
	"fmt"

	"github.com/ergo-services/ergo"
	"github.com/ergo-services/ergo/node"
)

var (
	WebListenPort int
	WebListenHost string
	WebEnableTLS  bool
)

func init() {
	flag.IntVar(&WebListenPort, "port", 0, "listen port number. Default port 8080, for TLS 8443")
	flag.StringVar(&WebListenHost, "host", "", "listen on host")
	flag.BoolVar(&WebEnableTLS, "tls", false, "enable TLS")
}

func main() {
	flag.Parse()
	fmt.Println("")
	fmt.Println("to stop press Ctrl-C")
	fmt.Println("")

	opts := node.Options{}

	// Initialize new node with given name, cookie, listening port range and epmd port
	webNode, err := ergo.StartNode("web@127.0.0.1", "secret", opts)
	if err != nil {
		panic(err)
	}

	if _, err := webNode.ApplicationLoad(&webApp{}); err != nil {
		panic(err)
	}

	if _, err := webNode.ApplicationStart("webApp"); err != nil {
		panic(err)
	}

	webNode.Wait()
}
