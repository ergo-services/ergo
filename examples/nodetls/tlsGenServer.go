package main

import (
	"flag"
	"fmt"

	"github.com/halturin/ergo"
	"github.com/halturin/ergo/etf"
)

// GenServer implementation structure
type demoGenServ struct {
	ergo.GenServer
	process *ergo.Process
}

type state struct {
	i int
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

// Init initializes process state using arbitrary arguments
// Init(...) -> state
func (dgs *demoGenServ) Init(p *ergo.Process, args ...interface{}) interface{} {
	fmt.Printf("Init: args %v \n", args)
	dgs.process = p
	return state{i: 12345}
}

// HandleCast serves incoming messages sending via gen_server:cast
// HandleCast -> ("noreply", state) - noreply
//		         ("stop", reason) - stop with reason
func (dgs *demoGenServ) HandleCast(message etf.Term, state interface{}) (string, interface{}) {
	fmt.Printf("HandleCast: %#v\n", message)
	switch message {
	case etf.Atom("stop"):
		return "stop", "they said"
	}
	return "noreply", state
}

// HandleCall serves incoming messages sending via gen_server:call
// HandleCall -> ("reply", message, state) - reply
//				 ("noreply", _, state) - noreply
//		         ("stop", reason, _) - normal stop
func (dgs *demoGenServ) HandleCall(from etf.Tuple, message etf.Term, state interface{}) (string, etf.Term, interface{}) {
	fmt.Printf("HandleCall: %#v, From: %#v\n", message, from)

	reply := etf.Term(etf.Tuple{etf.Atom("error"), etf.Atom("unknown_request")})
	switch message {
	case etf.Atom("hello"):
		reply = etf.Term("hi")
	}
	return "reply", reply, state
}

// HandleInfo serves all another incoming messages (Pid ! message)
// HandleInfo -> ("noreply", state) - noreply
//		         ("stop", reason) - normal stop
func (dgs *demoGenServ) HandleInfo(message etf.Term, state interface{}) (string, interface{}) {
	fmt.Printf("HandleInfo: %#v\n", message)
	return "noreply", state
}

// Terminate called when process died
func (dgs *demoGenServ) Terminate(reason string, state interface{}) {
	fmt.Printf("Terminate: %#v\n", reason)
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

	opts := ergo.NodeOptions{
		ListenRangeBegin: uint16(ListenRangeBegin),
		ListenRangeEnd:   uint16(ListenRangeEnd),
		EPMDPort:         uint16(ListenEPMD),

		// enables TLS encryption with self-signed certificate
		TLSmode: ergo.TLSmodeAuto,

		// set TLSmode to TLSmodeStrict to use custom certificate
		// TLSmode: ergo.TLSmodeStrict,
		// TLScrtServer: "example.crt",
		// TLSkeyServer: "example.key",
		// TLScrtClient: "example.crt",
		// TLSkeyClient: "example.key",
	}

	// Initialize new node with given name, cookie, listening port range and epmd port
	node := ergo.CreateNode(NodeName, Cookie, opts)

	// Initialize new instance of demoGenServ structure which implements Process behaviour
	demoGS := new(demoGenServ)

	// Spawn process with one arguments
	process, _ := node.Spawn(GenServerName, ergo.ProcessOptions{}, demoGS)
	fmt.Println("Run erl shell:")
	fmt.Printf("erl -proto_dist inet_tls -ssl_dist_opt server_certfile example.crt -ssl_dist_opt server_keyfile example.key -name %s -setcookie %s\n", "erl-"+node.FullName, Cookie)

	fmt.Println("-----Examples that can be tried from 'erl'-shell")
	fmt.Printf("gen_server:cast({%s,'%s'}, stop).\n", GenServerName, NodeName)
	fmt.Printf("gen_server:call({%s,'%s'}, hello).\n", GenServerName, NodeName)

	process.Wait()
	node.Stop()
}
