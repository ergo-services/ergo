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
	NodeName         string
	Cookie           string
	err              error
	ListenRangeBegin int
	ListenRangeEnd   int = 35000
	Listen           string
	ListenEPMD       int

	EnableRPC bool
)

type demoApp struct {
	gen.Application
}

func (da *demoApp) Load(args ...etf.Term) (gen.ApplicationSpec, error) {
	return gen.ApplicationSpec{
		Name:        "demoApp",
		Description: "Demo Applicatoin",
		Version:     "v.1.0",
		Environment: map[string]interface{}{
			"envName1": 123,
			"envName2": "Hello world",
		},
		Children: []gen.ApplicationChildSpec{
			gen.ApplicationChildSpec{
				Child: &demoSup{},
				Name:  "demoSup",
			},
			gen.ApplicationChildSpec{
				Child: &demoGenServ{},
				Name:  "justDemoGS",
			},
		},
	}, nil
}

func (da *demoApp) Start(process gen.Process, args ...etf.Term) {
	fmt.Println("Application started!")
}

type demoSup struct {
	gen.Supervisor
}

func (ds *demoSup) Init(args ...etf.Term) (gen.SupervisorSpec, error) {
	spec := gen.SupervisorSpec{
		Name: "demoAppSup",
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
			Restart:   gen.SupervisorStrategyRestartTemporary,
			// Restart: gen.SupervisorStrategyRestartTransient,
			// Restart: gen.SupervisorStrategyRestartPermanent,
		},
	}
	return spec, nil
}

// gen.Server implementation structure
type demoGenServ struct {
	gen.Server
}

func (dgs *demoGenServ) HandleCast(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	fmt.Printf("HandleCast (%s): %v\n", process.Name(), message)
	switch message {
	case etf.Atom("stop"):
		return gen.ServerStatusStopWithReason("stop they said")
	}
	return gen.ServerStatusOK
}

func (dgs *demoGenServ) HandleCall(process *gen.ServerProcess, from gen.ServerFrom, message etf.Term) (etf.Term, gen.ServerStatus) {
	fmt.Printf("HandleCall (%s): %v, From: %v\n", process.Name(), message, from)

	switch message {
	case etf.Atom("hello"):
		return etf.Atom("hi"), gen.ServerStatusOK
	}

	reply := etf.Tuple{etf.Atom("error"), etf.Atom("unknown_request")}
	return reply, gen.ServerStatusOK
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

	opts := node.Options{
		ListenRangeBegin: uint16(ListenRangeBegin),
		ListenRangeEnd:   uint16(ListenRangeEnd),
		EPMDPort:         uint16(ListenEPMD),
	}

	// Initialize new node with given name, cookie, listening port range and epmd port
	demoNode, _ := ergo.StartNode(NodeName, Cookie, opts)

	// start application
	if _, err := demoNode.ApplicationLoad(&demoApp{}); err != nil {
		panic(err)
	}

	appProcess, _ := demoNode.ApplicationStart("demoApp")
	fmt.Println("Run erl shell:")
	fmt.Printf("erl -name %s -setcookie %s\n", "erl-"+demoNode.Name(), Cookie)

	fmt.Println("-----Examples that can be tried from 'erl'-shell")
	fmt.Printf("gen_server:cast({%s,'%s'}, stop).\n", "demoServer01", demoNode.Name())
	fmt.Printf("gen_server:call({%s,'%s'}, hello).\n", "demoServer01", demoNode.Name())

	appProcess.Wait()
	demoNode.Stop()
}
