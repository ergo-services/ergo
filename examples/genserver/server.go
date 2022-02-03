package main

import (
	"flag"
	"fmt"

	"github.com/ergo-services/ergo"
	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/node"
)

// Server implementation structure
type demo struct {
	gen.Server
}

var (
	ServerName  string
	NodeName    string
	Cookie      string
	err         error
	ListenBegin int
	ListenEnd   int = 35000

	EnableRPC bool
)

func (dgs *demo) Init(process *gen.ServerProcess, args ...etf.Term) error {
	fmt.Printf("[%s] Init: args %v \n", process.Name(), args)
	return nil
}

func (dgs *demo) HandleCast(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	fmt.Printf("[%s] HandleCast: %v\n", process.Name(), message)
	if pid, ok := message.(etf.Pid); ok {
		process.Send(pid, etf.Atom("hahaha"))
		return gen.ServerStatusOK
	}
	switch message {
	case etf.Atom("stop"):
		return gen.ServerStatusStopWithReason("stop they said")
	}
	return gen.ServerStatusOK
}

func (dgs *demo) HandleCall(process *gen.ServerProcess, from gen.ServerFrom, message etf.Term) (etf.Term, gen.ServerStatus) {
	fmt.Printf("[%s] HandleCall: %#v, From: %v\n", process.Name(), message, from)

	reply := etf.Term(etf.Tuple{etf.Atom("error"), etf.Atom("unknown_request")})
	switch message.(type) {
	case etf.Atom:
		return etf.Term("hi"), gen.ServerStatusOK

	case etf.List:
		return message, gen.ServerStatusOK
	}
	return reply, gen.ServerStatusOK
}

func init() {
	flag.IntVar(&ListenBegin, "listen_begin", 15151, "listen port range")
	flag.IntVar(&ListenEnd, "listen_end", 25151, "listen port range")
	flag.StringVar(&ServerName, "gen_server_name", "example", "gen_server name")
	flag.StringVar(&NodeName, "name", "demo@127.0.0.1", "node name")
	flag.StringVar(&Cookie, "cookie", "123", "cookie for interaction with erlang cluster")
}

func main() {
	flag.Parse()

	opts := node.Options{
		ListenBegin: uint16(ListenBegin),
		ListenEnd:   uint16(ListenEnd),
	}

	// Initialize new node with given name, cookie, listening port range and epmd port
	node, e := ergo.StartNode(NodeName, Cookie, opts)
	if e != nil {
		fmt.Println("error", e)
		return
	}

	// Initialize new instance of demo structure which implements Process behavior
	demoGS := &demo{}

	// Spawn process with one arguments
	process, e := node.Spawn(ServerName, gen.ProcessOptions{}, demoGS)
	if e != nil {
		fmt.Println("error", e)
		return
	}

	// Add RPC handle with MF "rpc" "request"
	fun := func(args ...etf.Term) etf.Term {
		if len(args) == 0 {
			return etf.Atom("ok")
		}
		return etf.Term(args)
	}
	err := node.ProvideRPC("rpc", "request", fun)
	if err != nil {
		fmt.Println("error", err)
		return
	}

	// Print how it can be used along with the Erlang node
	fmt.Println("Run erl shell:")
	fmt.Printf("erl -name %s -setcookie %s\n", "erl-"+node.Name(), Cookie)

	fmt.Println("----- Examples that can be tried from 'erl'-shell")
	fmt.Printf("gen_server:cast({%s,'%s'}, stop).\n", ServerName, NodeName)
	fmt.Printf("gen_server:call({%s,'%s'}, hello).\n", ServerName, NodeName)
	fmt.Println("----- you may also want to try RPC request two ways:	")
	fmt.Println("----- by the registered MF('rpc','request'). will be handled by the registered anonymous func")
	fmt.Printf("rpc:call('%s', rpc, request, [hello, 3.14]).\n", NodeName)
	fmt.Printf("----- by the process name. will be handled by %s process. function name will be ignored\n", ServerName)
	fmt.Printf("rpc:call('%s', %s, f, [hello, 3.14]).\n", NodeName, ServerName)

	process.Wait()
	node.Stop()
	node.Wait()
}
