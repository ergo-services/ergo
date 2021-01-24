package main

import (
	"flag"
	"fmt"

	"github.com/halturin/ergo"
	"github.com/halturin/ergo/etf"
)

var (
	NodeName         string
	Cookie           string
	err              error
	ListenRangeBegin int
	ListenRangeEnd   int = 35000
	Listen           string
	ListenEPMD       int

	EnableRPC bool
)

type demoSup struct {
	ergo.Supervisor
}

func (ds *demoSup) Init(args ...interface{}) ergo.SupervisorSpec {
	return ergo.SupervisorSpec{
		Name: "demoSupervisorSup",
		Children: []ergo.SupervisorChildSpec{
			ergo.SupervisorChildSpec{
				Name:    "demoServer01",
				Child:   &demoGenServ{},
				Restart: ergo.SupervisorChildRestartTemporary,
				// Restart: ergo.SupervisorChildRestartTransient,
				// Restart: ergo.SupervisorChildRestartPermanent,
			},
			ergo.SupervisorChildSpec{
				Name:    "demoServer02",
				Child:   &demoGenServ{},
				Restart: ergo.SupervisorChildRestartPermanent,
				Args:    []interface{}{12345},
			},
			ergo.SupervisorChildSpec{
				Name:    "demoServer03",
				Child:   &demoGenServ{},
				Restart: ergo.SupervisorChildRestartPermanent,
				Args:    []interface{}{"abc", 67890},
			},
		},
		Strategy: ergo.SupervisorStrategy{
			Type: ergo.SupervisorStrategyOneForAll,
			// Type:      ergo.SupervisorStrategyRestForOne,
			// Type:      ergo.SupervisorStrategyOneForOne,
			Intensity: 2,
			Period:    5,
		},
	}
}

// GenServer implementation structure
type demoGenServ struct {
	ergo.GenServer
	process *ergo.Process
}

type state struct {
	i int
}

// Init initializes process state using arbitrary arguments
// Init(...) -> state
func (dgs *demoGenServ) Init(p *ergo.Process, args ...interface{}) interface{} {
	fmt.Printf("Init (%s): args %v \n", p.Name(), args)
	dgs.process = p
	return state{i: 12345}
}

// HandleCast serves incoming messages sending via gen_server:cast
// HandleCast -> ("noreply", state) - noreply
//		         ("stop", reason) - stop with reason
func (dgs *demoGenServ) HandleCast(message etf.Term, state interface{}) (string, interface{}) {
	fmt.Printf("HandleCast (%s): %#v\n", dgs.process.Name(), message)
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
	fmt.Printf("HandleCall (%s): %#v, From: %#v\n", dgs.process.Name(), message, from)

	reply := etf.Term(etf.Tuple{etf.Atom("error"), etf.Atom("unknown_request")})

	switch message {
	case etf.Atom("hello"):
		reply = etf.Term(etf.Atom("hi"))
	}
	return "reply", reply, state
}

// HandleInfo serves all another incoming messages (Pid ! message)
// HandleInfo -> ("noreply", state) - noreply
//		         ("stop", reason) - normal stop
func (dgs *demoGenServ) HandleInfo(message etf.Term, state interface{}) (string, interface{}) {
	fmt.Printf("HandleInfo (%s): %#v\n", dgs.process.Name(), message)
	return "noreply", state
}

// Terminate called when process died
func (dgs *demoGenServ) Terminate(reason string, state interface{}) {
	fmt.Printf("Terminate (%s): %#v\n", dgs.process.Name(), reason)
}

func init() {
	flag.IntVar(&ListenRangeBegin, "listen_begin", 15151, "listen port range")
	flag.IntVar(&ListenRangeEnd, "listen_end", 25151, "listen port range")
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

	// Spawn supervisor process
	process, _ := node.Spawn("demo_sup", ergo.ProcessOptions{}, &demoSup{})

	fmt.Println("Run erl shell:")
	fmt.Printf("erl -name %s -setcookie %s\n", "erl-"+node.FullName, Cookie)

	fmt.Println("-----Examples that can be tried from 'erl'-shell")
	fmt.Printf("gen_server:cast({%s,'%s'}, stop).\n", "demoServer01", NodeName)
	fmt.Printf("gen_server:call({%s,'%s'}, hello).\n", "demoServer01", NodeName)
	fmt.Println("or...")
	fmt.Printf("gen_server:cast({%s,'%s'}, stop).\n", "demoServer02", NodeName)
	fmt.Printf("gen_server:call({%s,'%s'}, hello).\n", "demoServer02", NodeName)
	fmt.Println("or...")
	fmt.Printf("gen_server:cast({%s,'%s'}, stop).\n", "demoServer03", NodeName)
	fmt.Printf("gen_server:call({%s,'%s'}, hello).\n", "demoServer03", NodeName)

	process.Wait()
	node.Stop()
	node.Wait()
}
