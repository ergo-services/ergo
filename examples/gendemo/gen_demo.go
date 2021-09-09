package main

import (
	"fmt"

	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/gen"
)

type GenDemo struct {
	gen.Server
}

type GenDemoProcess struct {
	gen.ServerProcess
	counter int
}

type messageHello struct{}
type messageHi struct{}
type messageGetStat struct{}

// GenDemoBehavior interface
type GenDemoBehavior interface {
	//
	// Mandatory callbacks
	//

	// InitDemo
	InitDemo(process *GenDemoProcess, args ...etf.Term) error

	// HandleHello invoked on a 'hello'
	HandleHello(process *GenDemoProcess)

	//
	// Optional callbacks
	//

	HandleHi(process *GenDemoProcess)

	// HandleGenDemoCall this callback is invoked on Process.Call. This method is optional
	// for the implementation
	HandleGenDemoCall(process *GenDemoProcess, from gen.ServerFrom, message etf.Term) (string, etf.Term)
	// HandleGenDemoCast this callback is invoked on Process.Cast. This method is optional
	// for the implementation
	HandleGenDemoCast(process *GenDemoProcess, message etf.Term) string
	// HandleGenDemoInfo this callback is invoked on Process.Send. This method is optional
	// for the implementation
	HandleGenDemoInfo(process *GenDemoProcess, message etf.Term) string
}

// default GenDemo callbacks

func (gd *GenDemo) HandleHi(process *GenDemoProcess) {
	fmt.Println("HandleHi: unhandled message")
	return
}

func (gd *GenDemo) HandleGenDemoCall(process *GenDemoProcess, from gen.ServerFrom, message etf.Term) (string, etf.Term) {
	fmt.Printf("HandleGenDemoCall: unhandled message (from %#v) %#v\n", from, message)
	return "reply", etf.Atom("ok")
}

func (gd *GenDemo) HandleGenDemoCast(process *GenDemoProcess, message etf.Term) string {
	fmt.Printf("HandleGenDemoCast: unhandled message %#v\n", message)
	return "noreply"
}
func (gd *GenDemo) HandleGenDemoInfo(process *GenDemoProcess, message etf.Term) string {
	fmt.Printf("HandleGenDemoInfo: unhandled message %#v\n", message)
	return "noreply"
}

// API

func (gd *GenDemo) Hello(process gen.Process) error {
	_, err := process.Direct(messageHello{})
	return err
}
func (gd *GenDemo) Hi(process gen.Process) error {
	_, err := process.Direct(messageHi{})
	return err
}

func (gd *GenDemo) Stat(process gen.Process) int {
	counter, _ := process.Direct(messageGetStat{})
	return counter.(int)
}

//
// gen.Server callbacks
//
func (gd *GenDemo) Init(process *gen.ServerProcess, args ...etf.Term) error {
	demo := &GenDemoProcess{
		ServerProcess: *process,
	}
	// do not inherite parent State
	demo.State = nil

	if err := process.Behavior().(GenDemoBehavior).InitDemo(demo, args...); err != nil {
		return err
	}
	process.State = demo
	return nil
}

func (gd *GenDemo) HandleCall(process *gen.ServerProcess, from gen.ServerFrom, message etf.Term) (string, etf.Term) {
	demo := process.State.(*GenDemoProcess)
	return process.Behavior().(GenDemoBehavior).HandleGenDemoCall(demo, from, message)
}

func (gd *GenDemo) HandleDirect(process *gen.ServerProcess, message interface{}) (interface{}, error) {
	demo := process.State.(*GenDemoProcess)
	switch message.(type) {
	case messageGetStat:
		return demo.counter, nil
	case messageHello:
		process.Behavior().(GenDemoBehavior).HandleHello(demo)
		demo.counter++
		return nil, nil
	case messageHi:
		process.Behavior().(GenDemoBehavior).HandleHi(demo)
		return nil, nil
	default:
		return nil, gen.ErrUnsupportedRequest
	}

}

func (gd *GenDemo) HandleCast(process *gen.ServerProcess, message etf.Term) string {
	demo := process.State.(*GenDemoProcess)
	return process.Behavior().(GenDemoBehavior).HandleGenDemoCast(demo, message)
}

func (gd *GenDemo) HandleInfo(process *gen.ServerProcess, message etf.Term) string {
	demo := process.State.(*GenDemoProcess)
	return process.Behavior().(GenDemoBehavior).HandleGenDemoInfo(demo, message)
	return "noreply"
}
