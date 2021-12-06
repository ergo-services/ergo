package main

import (
	"flag"
	"fmt"

	"github.com/ergo-services/ergo"
	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/node"
)

// GenServer implementation structure
type demoGenServ struct {
	gen.Server
}

var (
	GenServerName string
	NodeName      string
	Cookie        string
	err           error

	EnableRPC bool
)

func (dgs *demoGenServ) HandleCast(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	fmt.Printf("HandleCast: %#v\n", message)
	switch message {
	case etf.Atom("stop"):
		return gen.ServerStatusStopWithReason("stop they said")
	case "test":
		node := process.Env(node.EnvKeyNode).(node.Node)
		n := node.Nodes()
		fmt.Println("nodes: ", n)
		if err := node.Disconnect(n[0]); err != nil {
			fmt.Println("Cant disconnect", err)
		}
		if err := node.Connect(n[0]); err != nil {
			fmt.Println("Cant connect", err)
		}
	}
	return gen.ServerStatusOK
}

func (dgs *demoGenServ) HandleCall(process *gen.ServerProcess, from gen.ServerFrom, message etf.Term) (etf.Term, gen.ServerStatus) {
	fmt.Printf("HandleCall: %#v, From: %#v\n", message, from)

	switch message {
	case etf.Atom("hello"):
		process.Cast(process.Self(), "test")
		return etf.Term("hi"), gen.ServerStatusOK
	}
	reply := etf.Tuple{etf.Atom("error"), etf.Atom("unknown_request")}
	return reply, gen.ServerStatusOK
}

func init() {
	flag.StringVar(&GenServerName, "gen_server_name", "example", "gen_server name")
	flag.StringVar(&NodeName, "name", "demo@127.0.0.1", "node name")
	flag.StringVar(&Cookie, "cookie", "123", "cookie for interaction with erlang cluster")
}

func main() {
	flag.Parse()

	opts := node.Options{
		// enables TLS encryption with self-signed certificate
		TLS: node.TLS{Enable: true},
	}

	// Initialize new node with given name, cookie, listening port range and epmd port
	nodeTLS, _ := ergo.StartNode(NodeName, Cookie, opts)

	// Spawn process with one arguments
	process, _ := nodeTLS.Spawn(GenServerName, gen.ProcessOptions{}, &demoGenServ{})
	fmt.Println("Run erl shell:")
	fmt.Printf("erl -proto_dist inet_tls -ssl_dist_opt server_certfile example.crt -ssl_dist_opt server_keyfile example.key -name %s -setcookie %s\n", "erl-"+nodeTLS.Name(), Cookie)

	fmt.Println("-----Examples that can be tried from 'erl'-shell")
	fmt.Printf("gen_server:cast({%s,'%s'}, stop).\n", GenServerName, NodeName)
	fmt.Printf("gen_server:call({%s,'%s'}, hello).\n", GenServerName, NodeName)

	process.Wait()
	nodeTLS.Stop()
}
