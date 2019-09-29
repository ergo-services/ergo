[![GitHub release](https://img.shields.io/github/release/halturin/ergonode.svg)](https://github.com/halturin/ergonode/releases/latest)
[![Go Report Card](https://goreportcard.com/badge/github.com/halturin/ergonode)](https://goreportcard.com/report/github.com/halturin/ergonode)
[![GoDoc](https://godoc.org/code.gitea.io/gitea?status.svg)](https://godoc.org/github.com/halturin/ergonode)

# Ergo Framework #

Implementation of Erlang/OTP node in Go

### Purpose ###

The goal of this project is leverage Erlang/OTP experience with Golang performance. *Ergo Framework* implements OTP design patterns such as `GenServer`/`Supervisor`/`Application` and makes you able to create high performance and reliable application having native integration with Erlang infrastructure

### Features ###

 * Erlang node (run single/[multinode](#multinode))
 * [embedded EPMD](#epmd) (in order to get rid of erlang' dependencies)
 * Spawn Erlang-like processes
 * Register/unregister processes with simple atom
 * `GenServer` behavior support (with atomic state)
 * `Supervisor` behavior support (with all known restart strategies support)
 * `Application` behavior support
 * Connect to (accept connection from) any Erlang node within a cluster (or clusters, if running as multinode)
 * Making sync/async request in fashion of `gen_server:call` or `gen_server:cast`
 * Monitor processes/nodes
    - local -> local
    - local -> remote
    - remote -> local
 * Link processes
    - local <-> local
    - local <-> remote
    - remote <-> local
 * RPC callbacks support
 * basic [Observer support](#observer)
 * Support Erlang 21.*

### Requirements ###

 * Go 1.10 and above

### EPMD ###

*Ergo Framework* has embedded EPMD implementation in order to run your node without external epmd process needs. But it works as a client with erlang' epmd daemon or others ergo's nodes either.

The one thing that makes embedded EPMD different is behavior of handling connection hangs - if ergo' node is running as a epmd client and lost connection it tries to run its own embedded EPMD service or to restore lost connection.

As an extra option we provide EPMD service as a standalone application. There is a simple drop-in replacement of the original Erlang' epmd daemon.

`go get -u github.com/halturin/ergo/cmd/epmd`

### Multinode ###

 This feature allows create two or more nodes within a single running instance. The only needs is specify the different set of options for creating nodes (such as: node name, empd port number, secret cookie). You may also want to use this feature to create 'proxy'-node between some clusters. 
 See [Quick examples](#quick-examples) for more details
 

### Observer ###

 Allows you to see the most of metrics using standard tool of Erlang distribution. Example below shows this feature in action using one of examples:

 ... put here gif-ed video demonstrating it us

### Changelog ###

Here is the changes of latest release. For more details see the [ChangeLog](ChangeLog)

#### [1.0.0](https://github.com/halturin/ergo/releases/tag/1.0.0) - 2019-11-30 ####
 There is a bunch of changes we deliver with this release
 * We have changed the name - Ergo (or Ergo Framework). GitHub's repo has been renamed as well. We also have created cloned repo `ergonode` to support users of the old version of this project. So, its still available at [https://github.com/halturin/ergonode](https://github.com/halturin/ergonode). But it's strongly recommend to use the new one.
 * Completely reworked (almost from scratch) architecture whole project
 * Implemented linking process feature (in order to support Application/Supervisor behaviors)
 * Reworked Monitor-feature. Now it has full-featured support with remote process/nodes
 * Added multinode support
 * Added basic observer support
 * Improved code structure and readability
 * Among the new features we have added new bugs that are still uncovered :). So, any feedback/bugreport/contribution is highly appreciated

 ### Quick example ###

Code below a simple implementation of GenServer pattern

```golang
package main

import (
	"fmt"
	"time"

	"github.com/halturin/ergo"
	"github.com/halturin/ergo/etf"
)

type ExampleGenServer struct {
	ergo.GenServer
	process ergo.Process
}

type State struct {
	value int
}

func (egs *ExampleGenServer) Init(p ergo.Process, args ...interface{}) (state interface{}) {
	egs.process = p
	InitialState := &State{
		value: args[0].(int), // 100
	}
	return InitialState
}

func (egs *ExampleGenServer) HandleCast(message etf.Term, state interface{}) (string, interface{}) {
	fmt.Printf("ExampleGenServer HandleCast: %#v (state value %d) \n", message, state.(*State).value)
	time.Sleep(1 * time.Second)
	state.(*State).value++

	if state.(*State).value > 103 {
		egs.process.Send(egs.process.Self(), "hello")
	} else {
		egs.process.Cast(egs.process.Self(), "hi")
	}

	return "noreply", state
}

func (egs *ExampleGenServer) HandleCall(from etf.Tuple, message etf.Term, state interface{}) (string, etf.Term, interface{}) {
	fmt.Printf("ExampleGenServer HandleCall: %#v, From: %#v\n", message, from)
	return "reply", message, state
}

func (egs *ExampleGenServer) HandleInfo(message etf.Term, state interface{}) (string, interface{}) {
	fmt.Printf("ExampleGenServer HandleInfo: %#v (state value %d) \n", message, state.(*State).value)
	time.Sleep(1 * time.Second)
	state.(*State).value++
	if state.(*State).value > 106 {
		egs.process.Exit(egs.process.Self(), "normal")
	} else {
		egs.process.Send(egs.process.Self(), "hello")
	}
	return "noreply", state
}
func (egs *ExampleGenServer) Terminate(reason string, state interface{}) {
	fmt.Printf("ExampleGenServer Terminate: %s \n", reason)
}

func main() {

	node := ergo.CreateNode("nodeGS1@localhost", "cookies", ergo.NodeOptions{})
	gs1 := &ExampleGenServer{}
	process, _ := node.Spawn("gs1", ergo.ProcessOptions{}, gs1, 100)

	process.Cast(process.Self(), "hey")

	select {
	case <-process.Context.Done():
		fmt.Println("exited")
	}
}

```


See examples/ for more details

  * [demoGenServer](examples/genserver)
  * [demoSupervisor](examples/supervisor)
  * [demoApplication](examples/application)
  * [demoMultinode](examples/multinode)

### Elixir Phoenix Users ###

Users of the Elixir Phoenix framework might encounter timeouts when trying to connect a Phoenix node
to an ergo node. The reason is that, in addition to global_name_server and net_kernel,
Phoenix attempts to broadcast messages to the pg2 PubSub handler:
https://hexdocs.pm/phoenix/1.1.0/Phoenix.PubSub.PG2.html

To work with Phoenix nodes, you must create and register a dedicated pg2 GenServer, and
spawn it inside your node. Take inspiration from the global_name_server.go for the rest of
the GenServer methods, but the Spawn must have "pg2" as a process name:

```golang
type Pg2GenServer struct {
   GenServer
}

func main() {
   // ...
   pg2 := &Pg2GenServer{}
   node1 := CreateNode("node1@localhost", "cookies", NodeOptions{})
   process, _ := node1.Spawn("pg2", ProcessOptions{}, pg2, nil)
   // ...
}

```
