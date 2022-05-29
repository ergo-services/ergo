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
	flag.IntVar(&WebListenPort, "port", 0, "listen port number. Default port 8080, for TLS 8443")
	flag.StringVar(&WebListenHost, "host", "", "listen on host")
	flag.BoolVar(&WebEnableTLS, "tls", false, "enable TLS")
}

func main() {
	flag.Parse()

	opts := node.Options{}

	// Initialize new node with given name, cookie, listening port range and epmd port
	node, err := ergo.StartNode("web@127.0.0.1", "secret", opts)
	if err != nil {
		panic(err)
	}

	p, err := node.Spawn("", gen.ProcessOptions{}, &web{})
	if err != nil {
		panic(err)
	}

	fmt.Println("Started Web Process with PID:", p.Self(), "TLS:", WebEnableTLS)
	//time.Sleep(5 * time.Second)
	//fmt.Println("Stoping process")
	//p.Kill()

	node.Wait()
}
