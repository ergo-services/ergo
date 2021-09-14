package main

import (
	"fmt"

	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/gen"
)

type Demo struct {
	gen.Server
}

type DemoProcess struct {
	gen.ServerProcess
	counter int
}

// DemoBehavior interface
type DemoBehavior interface {
	//
	// Mandatory callbacks
	//

	// InitDemo
	InitDemo(process *DemoProcess, args ...etf.Term) error

	// HandleHello invoked on a 'hello'
	HandleHello(process *DemoProcess) DemoStatus

	//
	// Optional callbacks
	//

	// HandleDemoCall this callback is invoked on Process.Call. This method is optional
	// for the implementation
	HandleDemoCall(process *DemoProcess, from gen.ServerFrom, message etf.Term) (etf.Term, gen.ServerStatus)
	// HandleDemoDirect this callback is invoked on Process.Direct. This method is optional
	// for the implementation
	HandleDemoDirect(process *DemoProcess, message interface{}) (interface{}, error)
	// HandleDemoCast this callback is invoked on Process.Cast. This method is optional
	// for the implementation
	HandleDemoCast(process *DemoProcess, message etf.Term) gen.ServerStatus
	// HandleDemoInfo this callback is invoked on Process.Send. This method is optional
	// for the implementation
	HandleDemoInfo(process *DemoProcess, message etf.Term) gen.ServerStatus
}

type DemoStatus error

var (
	DemoStatusOK   DemoStatus = nil
	DemoStatusStop DemoStatus = fmt.Errorf("stop")
)

// default Demo callbacks

func (gd *Demo) HandleDemoCall(process *DemoProcess, from gen.ServerFrom, message etf.Term) (etf.Term, gen.ServerStatus) {
	fmt.Printf("HandleDemoCall: unhandled message (from %#v) %#v\n", from, message)
	return etf.Atom("ok"), gen.ServerStatusOK
}

func (gd *Demo) HandleDemoCast(process *DemoProcess, message etf.Term) gen.ServerStatus {
	fmt.Printf("HandleDemoCast: unhandled message %#v\n", message)
	return gen.ServerStatusOK
}
func (gd *Demo) HandleDemoInfo(process *DemoProcess, message etf.Term) gen.ServerStatus {
	fmt.Printf("HandleDemoInfo: unhandled message %#v\n", message)
	return gen.ServerStatusOK
}

//
// API
//

type messageHello struct{}

func (d *Demo) Hello(process gen.Process) error {
	_, err := process.Direct(messageHello{})
	return err
}

type messageGetStat struct{}

func (d *Demo) Stat(process gen.Process) int {
	counter, _ := process.Direct(messageGetStat{})
	return counter.(int)
}

//
// Demo methods. Available to use inside actors only.
//

func (dp *DemoProcess) Hi() DemoStatus {
	dp.counter = dp.counter * 2
	return DemoStatusOK
}

//
// gen.Server callbacks
//
func (d *Demo) Init(process *gen.ServerProcess, args ...etf.Term) error {
	demo := &DemoProcess{
		ServerProcess: *process,
	}
	// do not inherite parent State
	demo.State = nil

	if err := process.Behavior().(DemoBehavior).InitDemo(demo, args...); err != nil {
		return err
	}
	process.State = demo
	return nil
}

func (gd *Demo) HandleCall(process *gen.ServerProcess, from gen.ServerFrom, message etf.Term) (etf.Term, gen.ServerStatus) {
	demo := process.State.(*DemoProcess)
	return process.Behavior().(DemoBehavior).HandleDemoCall(demo, from, message)
}

func (gd *Demo) HandleDirect(process *gen.ServerProcess, message interface{}) (interface{}, error) {
	demo := process.State.(*DemoProcess)
	switch message.(type) {
	case messageGetStat:
		return demo.counter, nil
	case messageHello:
		process.Behavior().(DemoBehavior).HandleHello(demo)
		demo.counter++
		return nil, nil
	default:
		return process.Behavior().(DemoBehavior).HandleDemoDirect(demo, message)
	}

}

func (gd *Demo) HandleCast(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	demo := process.State.(*DemoProcess)
	return process.Behavior().(DemoBehavior).HandleDemoCast(demo, message)
}

func (gd *Demo) HandleInfo(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	demo := process.State.(*DemoProcess)
	return process.Behavior().(DemoBehavior).HandleDemoInfo(demo, message)
}
