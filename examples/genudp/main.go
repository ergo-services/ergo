package main

import (
	"flag"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/ergo-services/ergo"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/lib"
	"github.com/ergo-services/ergo/node"
)

var (
	UDPListenPort int
	UDPListenHost string
)

func init() {
	flag.IntVar(&UDPListenPort, "port", 5533, "listen port number")
	flag.StringVar(&UDPListenHost, "host", "localhost", "listen on host")
}

func main() {
	flag.Parse()

	opts := node.Options{
		Applications: []gen.ApplicationBehavior{
			&udpApp{}, // app.go
		},
	}

	fmt.Println("Start node", "udp@127.0.0.1")
	udpNode, err := ergo.StartNode("udp@127.0.0.1", "secret", opts)
	if err != nil {
		panic(err)
	}

	hostPort := net.JoinHostPort(UDPListenHost, strconv.Itoa(UDPListenPort))
	c, err := net.Dial("udp", hostPort)
	if err != nil {
		return
	}
	defer c.Close()
	for i := 0; i < 5; i++ {
		str := lib.RandomString(16)

		fmt.Printf("send string %q to %q\n", str, c.RemoteAddr().String())
		c.Write([]byte(str))
		time.Sleep(time.Second)
	}

	fmt.Println("stop node", udpNode.Name())
	udpNode.Stop()
}
