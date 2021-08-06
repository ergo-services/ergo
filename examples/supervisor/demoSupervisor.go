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
}

func (dgs *demoGenServ) HandleCast(state *ergo.GenServerState, message etf.Term) string {
	fmt.Printf("HandleCast (%s): %#v\n", state.Process.Name(), message)
	switch message {
	case etf.Atom("stop"):
		return "stop they said"
	}
	return "noreply"
}

func (dgs *demoGenServ) HandleCall(state *ergo.GenServerState, from ergo.GenServerFrom, message etf.Term) (string, etf.Term) {

	if message == etf.Atom("hello") {
		return "reply", etf.Atom("hi")
	}
	return "reply", etf.Tuple{etf.Atom("error"), etf.Atom("unknown_request")}
}

func (dgs *demoGenServ) HandleInfo(state *ergo.GenServerState, message etf.Term) string {
	fmt.Printf("HandleInfo (%s): %#v\n", state.Process.Name(), message)
	return "noreply"
}

func (dgs *demoGenServ) Terminate(state *ergo.GenServerState, reason string) {
	fmt.Printf("Terminate (%s): %#v\n", state.Process.Name(), reason)
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
	node, _ := ergo.CreateNode(NodeName, Cookie, opts)

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
