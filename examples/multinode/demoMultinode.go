package main

import (
	"context"
	"fmt"
	"sync"

	"github.com/halturin/ergo"
	"github.com/halturin/ergo/etf"
)

// GenServer implementation structure
type demoGenServ struct {
	ergo.GenServer
	wg     *sync.WaitGroup
	bridge chan interface{}
}

type state struct {
	i int
}

var (
	GenServerName string
)

// Init initializes process state using arbitrary arguments
// Init(...) -> state
func (dgs *demoGenServ) Init(p *ergo.Process, args ...interface{}) (interface{}, error) {
	// fmt.Printf("Init: args %v \n", args)
	dgs.wg = args[0].(*sync.WaitGroup)
	dgs.bridge = args[2].(chan interface{})

	go func() {
		ctx, cancel := context.WithCancel(p.Context)
		defer cancel()
		for {
			select {
			case msg := <-args[1].(chan interface{}):
				p.Send(p.Self(), etf.Tuple{"forwarded", msg})

			case <-ctx.Done():
				return
			}
		}
	}()

	return state{i: 12345}, nil
}

func (dgs *demoGenServ) HandleCast(message etf.Term, state ergo.GenServerState) string {
	fmt.Printf("[%s] HandleCast: %#v\n", state.Process.Node.FullName, message)
	switch message {
	case etf.Atom("stop"):
		return "stop they said"
	case etf.Atom("forward"):
		dgs.bridge <- fmt.Sprintf("Hi from %v", state.Process.Self())
	}
	return "noreply"
}

func (dgs *demoGenServ) HandleCall(from etf.Tuple, message etf.Term, state ergo.GenServerState) (string, etf.Term) {
	fmt.Printf("[%s] HandleCall: %#v, From: %#v\n", state.Process.Node.FullName, message, from)

	reply := etf.Term(etf.Tuple{etf.Atom("error"), etf.Atom("unknown_request")})

	switch message {
	case etf.Atom("hello"):
		reply = etf.Term(etf.Atom("hi"))
	}
	return "reply", reply
}

func (dgs *demoGenServ) HandleInfo(message etf.Term, state ergo.GenServerState) string {
	fmt.Printf("[%s] HandleInfo: %#v\n", state.Process.Node.FullName, message)
	return "noreply"
}

func (dgs *demoGenServ) Terminate(reason string, state ergo.GenServerState) {
	fmt.Printf("[%s] Terminate: %#v\n", state.Process.Node.FullName, reason)
	dgs.wg.Done()
}

func main() {
	var wg sync.WaitGroup
	// Initialize new node with EPMD port 7878
	optsNode01 := ergo.NodeOptions{
		EPMDPort: 7878,
	}
	node01 := ergo.CreateNode("demoNode7878@127.0.0.1", "cookie123", optsNode01)
	fmt.Println("Started ergo node: demoNode7878@127.0.0.1 on port 7878")
	optsNode02 := ergo.NodeOptions{
		EPMDPort: 8787,
	}
	node02 := ergo.CreateNode("demoNode8787@127.0.0.1", "cookie456", optsNode02)
	fmt.Println("Started ergo node: demoNode8787@127.0.0.1 on port 8787")

	// Spawn process with one arguments
	wg.Add(1)
	bridgeToNode01 := make(chan interface{}, 10)
	bridgeToNode02 := make(chan interface{}, 10)
	p1, _ := node01.Spawn("example", ergo.ProcessOptions{}, &demoGenServ{}, &wg, bridgeToNode01, bridgeToNode02)
	fmt.Println("Started 'example' GenServer at demoNode7878@127.0.0.1 with PID", p1.Self())

	wg.Add(1)
	p2, _ := node02.Spawn("example", ergo.ProcessOptions{}, &demoGenServ{}, &wg, bridgeToNode02, bridgeToNode01)
	fmt.Println("Started 'example' GenServer at demoNode8787@127.0.0.1 with PID", p2.Self())

	fmt.Println("")

	fmt.Println("Run erl shell (cluster with cookie123):")
	fmt.Printf("ERL_EPMD_PORT=7878 erl -name %s -setcookie cookie123\n", "erl-demoNode-cookie123@127.0.0.1")

	fmt.Println("\n-----Examples that can be tried from 'erl'-shell")
	fmt.Println("gen_server:cast({example,'demoNode7878@127.0.0.1'}, stop).")
	fmt.Println("gen_server:call({example,'demoNode7878@127.0.0.1'}, hello).")
	fmt.Println("gen_server:cast({example,'demoNode7878@127.0.0.1'}, forward).")
	fmt.Println("")

	fmt.Println("Run erl shell (cluster with cookie456):")
	fmt.Printf("ERL_EPMD_PORT=8787 erl -name %s -setcookie cookie456\n", "erl-demoNode-cookie456@127.0.0.1")
	fmt.Println("\n-----Examples that can be tried from 'erl'-shell")
	fmt.Println("gen_server:cast({example,'demoNode8787@127.0.0.1'}, stop).")
	fmt.Println("gen_server:call({example,'demoNode8787@127.0.0.1'}, hello).")
	fmt.Println("gen_server:cast({example,'demoNode8787@127.0.0.1'}, forward).")

	wg.Wait()
	node01.Stop()
	node02.Stop()
}
