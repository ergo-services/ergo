package main

import (
	"crypto/tls"
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
	TCPListenPort int
	TCPListenHost string
	TCPEnableTLS  bool
)

func init() {
	flag.IntVar(&TCPListenPort, "port", 8383, "listen port number")
	flag.StringVar(&TCPListenHost, "host", "", "listen on host")
	flag.BoolVar(&TCPEnableTLS, "tls", false, "enable TLS")
}

func main() {
	var connection net.Conn
	var err error

	flag.Parse()
	opts := node.Options{
		Applications: []gen.ApplicationBehavior{
			&tcpApp{}, // app.go
		},
	}

	fmt.Println("Start node", "tcp@127.0.0.1")
	tcpNode, err := ergo.StartNode("tcp@127.0.0.1", "secret", opts)
	if err != nil {
		panic(err)
	}

	hostPort := net.JoinHostPort(TCPListenHost, strconv.Itoa(TCPListenPort))
	dialer := net.Dialer{}

	if TCPEnableTLS {
		tlsdialer := tls.Dialer{
			NetDialer: &dialer,
			Config: &tls.Config{
				InsecureSkipVerify: true,
			},
		}
		connection, err = tlsdialer.Dial("tcp", hostPort)
	} else {
		connection, err = dialer.Dial("tcp", hostPort)
	}

	if err != nil {
		return
	}

	defer connection.Close()

	for i := 0; i < 5; i++ {
		str := lib.RandomString(16)

		fmt.Printf("send string %q to %q\n", str, connection.RemoteAddr().String())
		connection.Write([]byte(str))
		time.Sleep(time.Second)
	}

	fmt.Println("stop node", tcpNode.Name())
	tcpNode.Stop()
}
