package main

import (
	"flag"
	"fmt"

	"github.com/ergo-services/ergo"
	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/node"
)

var (
	NodeName string
	Cookie   string
	err      error

	EnableRPC bool
)

type demoSup struct {
	gen.Supervisor
}

func (ds *demoSup) Init(args ...etf.Term) (gen.SupervisorSpec, error) {
	return gen.SupervisorSpec{
		Name: "demoSupervisorSup",
		Children: []gen.SupervisorChildSpec{
			gen.SupervisorChildSpec{
				Name:  "demoServer01",
				Child: &demoGenServ{},
			},
			gen.SupervisorChildSpec{
				Name:  "demoServer02",
				Child: &demoGenServ{},
				Args:  []etf.Term{12345},
			},
			gen.SupervisorChildSpec{
				Name:  "demoServer03",
				Child: &demoGenServ{},
				Args:  []etf.Term{"abc", 67890},
			},
		},
		Strategy: gen.SupervisorStrategy{
			Type: gen.SupervisorStrategyOneForAll,
			// Type:      gen.SupervisorStrategyRestForOne,
			// Type:      gen.SupervisorStrategyOneForOne,
			Intensity: 2,
			Period:    5,
			// Restart:   gen.SupervisorStrategyRestartTemporary,
			// Restart: gen.SupervisorStrategyRestartTransient,
			Restart: gen.SupervisorStrategyRestartPermanent,
		},
	}, nil
}

// GenServer implementation structure
type demoGenServ struct {
	gen.Server
}

func (dgs *demoGenServ) HandleCast(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	fmt.Printf("HandleCast (%s): %#v\n", process.Name(), message)
	switch message {
	case etf.Atom("stop"):
		return gen.ServerStatusStopWithReason("stop they said")
	}
	return gen.ServerStatusOK
}

func (dgs *demoGenServ) HandleCall(process *gen.ServerProcess, from gen.ServerFrom, message etf.Term) (etf.Term, gen.ServerStatus) {

	if message == etf.Atom("hello") {
		return etf.Atom("hi"), gen.ServerStatusOK
	}
	return etf.Tuple{etf.Atom("error"), etf.Atom("unknown_request")}, gen.ServerStatusOK
}

func (dgs *demoGenServ) HandleInfo(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	fmt.Printf("HandleInfo (%s): %#v\n", process.Name(), message)
	return gen.ServerStatusOK
}

func (dgs *demoGenServ) Terminate(process *gen.ServerProcess, reason string) {
	fmt.Printf("Terminate (%s): %#v\n", process.Name(), reason)
}

func init() {
	flag.StringVar(&NodeName, "name", "demo@127.0.0.1", "node name")
	flag.StringVar(&Cookie, "cookie", "123", "cookie for interaction with erlang cluster")
}

func main() {
	flag.Parse()

	// Initialize new node with given name, cookie, listening port range and epmd port
	node, _ := ergo.StartNode(NodeName, Cookie, node.Options{})

	// Spawn supervisor process
	process, _ := node.Spawn("demo_sup", gen.ProcessOptions{}, &demoSup{})

	fmt.Println("Run erl shell:")
	fmt.Printf("erl -name %s -setcookie %s\n", "erl-"+node.Name(), Cookie)

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
