package main

import (
	"flag"
	"fmt"
	"strconv"
	"strings"

	"github.com/halturin/ergonode"
	"github.com/halturin/ergonode/etf"
)

// GenServer implementation structure
type demoApplication struct {
	ergonode.GenServer
	process ergonode.Process
}

type state struct {
	i int
}

var (
	ServerName       string
	NodeName         string
	Cookie           string
	err              error
	ListenRangeBegin uint16
	ListenRangeEnd   uint16 = 35000
	Listen           string
	ListenEPMD       int

	EnableRPC bool
)

// Init initializes process state using arbitrary arguments
// Init(...) -> state
func (dgs *demoApplication) Init(p ergonode.Process, args ...interface{}) interface{} {
	dgs.process = p
	return state{i: 12345}
}

// HandleCast serves incoming messages sending via gen_server:cast
// HandleCast -> ("noreply", state) - noreply
//		         ("stop", reason) - stop with reason
func (dgs *demoApplication) HandleCast(message etf.Term, state interface{}) (string, interface{}) {
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
func (dgs *demoApplication) HandleCall(from etf.Tuple, message etf.Term, state interface{}) (string, etf.Term, interface{}) {
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
func (dgs *demoApplication) HandleInfo(message etf.Term, state interface{}) (string, interface{}) {
	fmt.Printf("HandleInfo: %#v\n", message)
	return "noreply", state
}

// Terminate called when process died
func (dgs *demoApplication) Terminate(reason string, state interface{}) {
	fmt.Printf("Terminate: %#v\n", reason)
}

func init() {
	flag.StringVar(&Listen, "listen", "15151-20151", "listen port range")
	flag.StringVar(&ServerName, "gen_server", "example", "gen_server name")
	flag.StringVar(&NodeName, "name", "examplenode@127.0.0.1", "node name")
	flag.IntVar(&ListenEPMD, "epmd", 4369, "EPMD port")
	flag.StringVar(&Cookie, "cookie", "123", "cookie for interaction with erlang cluster")
	flag.BoolVar(&EnableRPC, "rpc", false, "enable RPC")

}

func main() {

	opts := ergonode.NodeOptions{}

	flag.Parse()

	// parse listen range port
	l := strings.Split(Listen, "-")
	switch len(l) {
	case 1:
		if i, err := strconv.ParseUint(l[0], 10, 16); err != nil {
			panic(err)
		} else {
			opts.ListenRangeBegin = uint16(i)
		}
	case 2:
		if i, err := strconv.ParseUint(l[0], 10, 16); err != nil {
			panic(err)
		} else {
			opts.ListenRangeBegin = uint16(i)
		}
		if i, err := strconv.ParseUint(l[1], 10, 16); err != nil {
			panic(err)
		} else {
			opts.ListenRangeEnd = uint16(i)
		}
	default:
		panic("wrong port range arg")
	}

	opts.EPMDPort = uint16(ListenEPMD)

	// Initialize new node with given name, cookie, listening port range and epmd port
	node := ergonode.CreateNode(NodeName, Cookie, opts)

	// Initialize new instance of demoApplication structure which implements Process behaviour
	demoGS := new(demoApplication)

	// Spawn process with one arguments
	node.Spawn(ServerName, ergonode.ProcessOptions{}, demoGS)
	fmt.Println("Run erl shell:")
	fmt.Printf("erl -name %s -setcookie %s\n", "erl-"+node.FullName, Cookie)

	fmt.Println("-----Examples that can be tried from 'erl'-shell ...")
	fmt.Printf("gen_server:cast({%s,'%s'}, stop).\n", ServerName, NodeName)
	fmt.Printf("gen_server:call({%s,'%s'}, pid).\n", ServerName, NodeName)
	fmt.Printf("gen_server:cast({%s,'%s'}, {ping, self()}), flush().\n", ServerName, NodeName)
	fmt.Println("make remote call by golang node...")
	fmt.Printf("gen_server:call({%s,'%s'}, {testcall, {Pid, Message}}).\n", ServerName, NodeName)
	fmt.Printf("gen_server:call({%s,'%s'}, {testcall, {{pname, remotenode}, Message}}).\n", ServerName, NodeName)

	// Ctrl+C to stop it
	select {}

}
