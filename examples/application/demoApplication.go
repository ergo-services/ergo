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

type demoApp struct {
	ergo.Application
}

func (da *demoApp) Load(args ...interface{}) (ergo.ApplicationSpec, error) {
	return ergo.ApplicationSpec{
		Name:        "demoApp",
		Description: "Demo Applicatoin",
		Version:     "v.1.0",
		Environment: map[string]interface{}{
			"envName1": 123,
			"envName2": "Hello world",
		},
		Children: []ergo.ApplicationChildSpec{
			ergo.ApplicationChildSpec{
				Child: &demoSup{},
				Name:  "demoSup",
			},
			ergo.ApplicationChildSpec{
				Child: &demoGenServ{},
				Name:  "justDemoGS",
			},
		},
	}, nil
}

func (da *demoApp) Start(process *ergo.Process, args ...interface{}) {
	fmt.Println("Application started!")
}

type demoSup struct {
	ergo.Supervisor
}

func (ds *demoSup) Init(args ...interface{}) ergo.SupervisorSpec {
	return ergo.SupervisorSpec{
		Name: "demoAppSup",
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
	fmt.Printf("HandleCall (%s): %#v, From: %#v\n", state.Process.Name(), message, from)

	reply := etf.Term(etf.Tuple{etf.Atom("error"), etf.Atom("unknown_request")})

	switch message {
	case etf.Atom("hello"):
		reply = etf.Term(etf.Atom("hi"))
	}
	return "reply", reply
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

	// start application
	if err := node.ApplicationLoad(&demoApp{}); err != nil {
		panic(err)
	}

	process, _ := node.ApplicationStart("demoApp")
	fmt.Println("Run erl shell:")
	fmt.Printf("erl -name %s -setcookie %s\n", "erl-"+node.FullName, Cookie)

	fmt.Println("-----Examples that can be tried from 'erl'-shell")
	fmt.Printf("gen_server:cast({%s,'%s'}, stop).\n", "demoServer01", NodeName)
	fmt.Printf("gen_server:call({%s,'%s'}, hello).\n", "demoServer01", NodeName)

	process.Wait()
	node.Stop()
}
