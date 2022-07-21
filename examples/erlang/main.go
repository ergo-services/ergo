package main

import (
	"flag"
	"fmt"

	"github.com/ergo-services/ergo"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/node"
)

var (
	ServerName string
	NodeName   string
	Cookie     string
)

func init() {
	flag.StringVar(&ServerName, "server", "example", "server process name")
	flag.StringVar(&NodeName, "name", "demo@127.0.0.1", "node name")
	flag.StringVar(&Cookie, "cookie", "123", "cookie for interaction with erlang cluster")
}

func main() {
	flag.Parse()

	fmt.Println("")
	fmt.Println("to stop press Ctrl-C")
	fmt.Println("")

	node, err := ergo.StartNode(NodeName, Cookie, node.Options{})
	if err != nil {
		panic(err)
	}

	_, err = node.Spawn(ServerName, gen.ProcessOptions{}, &demo{})
	if err != nil {
		panic(err)
	}

	fmt.Println("Start erlang node with the command below:")
	fmt.Printf("    $ erl -name %s -setcookie %s\n\n", "erl-"+node.Name(), Cookie)

	fmt.Println("----- to make call request from 'erl'-shell:")
	fmt.Printf("gen_server:call({%s,'%s'}, hi).\n", ServerName, NodeName)
	fmt.Printf("gen_server:call({%s,'%s'}, {echo, 1,2,3}).\n", ServerName, NodeName)
	fmt.Println("----- to send cast message from 'erl'-shell:")
	fmt.Printf("gen_server:cast({%s,'%s'}, {cast, 1,2,3}).\n", ServerName, NodeName)
	fmt.Println("----- send atom 'stop' to stop server :")
	fmt.Printf("gen_server:cast({%s,'%s'}, stop).\n", ServerName, NodeName)

	node.Wait()
}
