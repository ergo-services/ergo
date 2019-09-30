package main

import (
	"flag"
	"fmt"

	ergo "github.com/halturin/ergonode"
	"github.com/halturin/ergonode/etf"
)

// GenServer implementation structure
type demoGenServ struct {
	ergo.GenServer
	process ergo.Process
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
func (dgs *demoGenServ) Init(p ergo.Process, args ...interface{}) interface{} {
	dgs.process = p
	return state{i: 12345}
}

// HandleCast serves incoming messages sending via gen_server:cast
// HandleCast -> ("noreply", state) - noreply
//		         ("stop", reason) - stop with reason
func (dgs *demoGenServ) HandleCast(message etf.Term, state interface{}) (string, interface{}) {
	fmt.Printf("HandleCast: %#v\n", message)
	// Check type of message
	switch req := (message).(type) {
	case etf.Tuple:
		if len(req) == 2 {
			switch act := req[0].(type) {
			case etf.Atom:
				if string(act) == "ping" {
					to := req.Element(2).(etf.Pid)
					self := dgs.process.Self()
					rep := etf.Term(etf.Tuple{etf.Atom("pong"), self})
					dgs.process.Send(to, rep)
				}
			}
		}
	case etf.Atom:
		if string(req) == "stop" {
			return "stop", "reason: they asked"
		}
	}
	return "ok", state
}

// HandleCall serves incoming messages sending via gen_server:call
// HandleCall -> ("reply", message, state) - reply
//				 ("noreply", _, state) - noreply
//		         ("stop", reason, _) - normal stop
func (dgs *demoGenServ) HandleCall(from etf.Tuple, message etf.Term, state interface{}) (string, etf.Term, interface{}) {
	fmt.Printf("HandleCall: %#v, From: %#v\n", message, from)

	code := "reply"
	reply := etf.Term(etf.Tuple{etf.Atom("error"), etf.Atom("unknown_request")})

	switch req := (message).(type) {
	case etf.Atom:
		switch string(req) {
		case "pid":
			reply = etf.Term(dgs.process.Self())
		case "stop":
			return "stop", "they said stop", state
		case "monitor":
			dgs.process.MonitorProcess(from.Element(1).(etf.Pid))
			reply = etf.Term(etf.Atom("ok"))
		}

	case etf.Tuple:
		var to, msg etf.Term
		// {testcall, { {name, node}, message  }}
		// {testcast, { {name, node}, message  }}
		if len(req) == 2 {
			act := req[0].(etf.Atom)
			c := req[1].(etf.Tuple)

			switch c[0].(type) {
			case etf.Tuple:
				switch ct := c[0].(type) {
				case etf.Tuple:
					if ct[0].(etf.Atom) == ct[1].(etf.Atom) {
					}
					to = etf.Term(c[0])
				default:
					return code, reply, state
				}
			case etf.Pid:
				to = etf.Term(c[0])
			default:
				return code, reply, state
			}

			msg = c.Element(2)

			if string(act) == "testcall" {
				fmt.Printf("doing test call... %#v : %#v\n", to, msg)
				if reply, err = dgs.process.Call(to, msg); err != nil {
					fmt.Println(err.Error())
				}
			} else if string(act) == "testcast" {
				fmt.Println("doing test cast...")
				dgs.process.Cast(to, msg)
			}

		}
	}
	return code, reply, state
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
	}

	// Initialize new node with given name, cookie, listening port range and epmd port
	node := ergo.CreateNode(NodeName, Cookie, opts)

	// Initialize new instance of demoGenServ structure which implements Process behaviour
	demoGS := new(demoGenServ)

	// Spawn process with one arguments
	process, _ := node.Spawn(GenServerName, ergo.ProcessOptions{}, demoGS)
	fmt.Println("Run erl shell:")
	fmt.Printf("erl -name %s -setcookie %s\n", "erl-"+node.FullName, Cookie)

	fmt.Println("Allowed commands...")
	fmt.Printf("gen_server:cast({%s,'%s'}, stop).\n", GenServerName, NodeName)
	fmt.Printf("gen_server:call({%s,'%s'}, pid).\n", GenServerName, NodeName)
	fmt.Printf("gen_server:cast({%s,'%s'}, {ping, self()}), flush().\n", GenServerName, NodeName)
	fmt.Println("make remote call by golang node...")
	fmt.Printf("gen_server:call({%s,'%s'}, {testcall, {Pid, Message}}).\n", GenServerName, NodeName)
	fmt.Printf("gen_server:call({%s,'%s'}, {testcall, {{pname, remotenode}, Message}}).\n", GenServerName, NodeName)

	// Ctrl+C to stop it
	select {
	case <-process.Context.Done():

	}
	node.Stop()
}
