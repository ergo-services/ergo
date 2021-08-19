package main

import (
	"flag"
	"fmt"

	"github.com/halturin/ergo"
	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/gen"
	"github.com/halturin/ergo/node"
)

// GenServer implementation structure
type demoGenServ struct {
	gen.GenServer
}

var (
	GenServerName    string
	NodeName         string
	Cookie           string
	err              error
	ListenRangeBegin int
	ListenRangeEnd   int = 35000
	Listen           string
	ListenEPMD       int

	EnableRPC bool
)

func (dgs *demoGenServ) Init(process *gen.GenServerProcess, args ...etf.Term) error {
	fmt.Printf("[%s] Init: args %v \n", process.Name(), args)
	return nil
}

func (dgs *demoGenServ) HandleCast(process *gen.GenServerProcess, message etf.Term) string {
	fmt.Printf("[%s] HandleCast: %#v\n", process.Name(), message)
	if pid, ok := message.(etf.Pid); ok {
		process.Send(pid, etf.Atom("hahaha"))
		return "noreply"
	}
	switch message {
	case etf.Atom("stop"):
		return "stop they said"
	}
	return "noreply"
}

func (dgs *demoGenServ) HandleCall(process *gen.GenServerProcess, from gen.GenServerFrom, message etf.Term) (string, etf.Term) {
	fmt.Printf("[%s] HandleCall: %#v, From: %#v\n", process.Name(), message, from)

	reply := etf.Term(etf.Tuple{etf.Atom("error"), etf.Atom("unknown_request")})
	switch message {
	case etf.Atom("hello"):
		reply = etf.Term("hi")
	}
	return "reply", reply
}

func init() {
	flag.IntVar(&ListenRangeBegin, "listen_begin", 15151, "listen port range")
	flag.IntVar(&ListenRangeEnd, "listen_end", 25151, "listen port range")
	flag.StringVar(&GenServerName, "gen_server_name", "example", "gen_server name")
	flag.StringVar(&NodeName, "name", "demo@127.0.0.1", "node name")
	flag.IntVar(&ListenEPMD, "epmd", 4369, "EPMD port")
	flag.StringVar(&Cookie, "cookie", "123", "cookie for interaction with erlang cluster")
}

func main() {
	flag.Parse()

	opts := node.Options{
		ListenRangeBegin: uint16(ListenRangeBegin),
		ListenRangeEnd:   uint16(ListenRangeEnd),
		EPMDPort:         uint16(ListenEPMD),
	}

	// Initialize new node with given name, cookie, listening port range and epmd port
	node, e := ergo.StartNode(NodeName, Cookie, opts)
	if e != nil {
		fmt.Println("error", e)
		return
	}

	// Initialize new instance of demoGenServ structure which implements Process behavior
	demoGS := &demoGenServ{}

	// Spawn process with one arguments
	process, e := node.Spawn(GenServerName, gen.ProcessOptions{}, demoGS)
	if e != nil {
		fmt.Println("error", e)
		return
	}

	// Print how it can be used along with the Erlang node
	fmt.Println("Run erl shell:")
	fmt.Printf("erl -name %s -setcookie %s\n", "erl-"+node.Name(), Cookie)

	fmt.Println("-----Examples that can be tried from 'erl'-shell")
	fmt.Printf("gen_server:cast({%s,'%s'}, stop).\n", GenServerName, NodeName)
	fmt.Printf("gen_server:call({%s,'%s'}, hello).\n", GenServerName, NodeName)

	process.Wait()
	node.Stop()
	node.Wait()
}
