package main

/*
import (
	"fmt"

	"github.com/halturin/ergo"
	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/gen"
)

type GenDemo struct {
	gen.Server
}

type GenDemoOptions struct {
	A int
	B string
}

type GenDemoProcess struct {
	Options GenDemoOptions
	counter int
}

type demoMessage struct {
	request string
}

type setCounter struct {
	c int
}
type getCounter struct{}

// GenDemoBehavior interface
type GenDemoBehavior interface {
	//
	// Mandatory callbacks
	//

	// InitDemo
	InitDemo(process *GenDemoProcess, args ...etf.Term) error

	// HandleHello invoked on a 'hello' request where 'n' is how many times it was received
	HandleHello(process *GenDemoProcess, n int) error

	//
	// Optional callbacks
	//

	HandleHi(process *GenDemoProcess) error

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

func (gd *GenDemo) HandleHi(process *GenDemoProcess) error {
	fmt.Println("HandleHi: unhandled message")
	return nil
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

func (gd *GenDemo) SetCounter(process gen.Process, c int) error {
	_, err := process.Direct(setCounter{c: c})
	return err
}

func (gd *GenDemo) GetCounter(process gen.Process) (int, error) {
	return process.Direct(getCounter{})

}

//
// gen.Server callbacks
//
func (gd *GenDemo) Init(process *gen.ServerProcess, args ...etf.Term) error {
	demoState := &GenDemoProcess{}
	if err := state.Process.GetObject().(GenDemoBehavior).InitDemo(demoState, args...); err != nil {
		return err
	}
	process.State = demoState
	return nil
}

func (gd *GenDemo) HandleCall(process *gen.ServerProcess, from gen.ServerFrom, message etf.Term) (string, etf.Term) {
	st := process.State.(*GenDemoProcess)
	return process.GetObject().(GenDemoBehavior).HandleGenDemoCall(st, from, message)
}

func (gd *GenDemo) HandleDirect(process *gen.ServerProcess, message interface{}) (interface{}, error) {
	st := process.State.(*GenDemoProcess)
	switch m := message.(type) {
	case setCounter:
		st.counter = m.c
		return nil, nil
	case getCounter:
		return st.counter, nil
	default:
		return nil, ergo.ErrUnsupportedRequest
	}

}

func (gd *GenDemo) HandleCast(process *gen.ServerProcess, message etf.Term) string {
	st := process.State.(*GenDemoProcess)
	return process.GetObject().(GenDemoBehavior).HandleGenDemoCast(st, message)
}

func (gd *GenDemo) HandleInfo(process *gen.ServerProcess, message etf.Term) string {
	var d DownMessage
	var m demoMessage

	st := process.State.(*GenDemoProcess)
	// check if we got a 'DOWN' message
	// {DOWN, Ref, process, PidOrName, Reason}
	if isDown, d := IsDownMessage(message); isDown {
		if err := handleDemoDown(st, d); err != nil {
			return err.Error()
		}
		return "noreply"
	}

	if err := etf.TermIntoStruct(message, &m); err != nil {
		reply := process.GetObject().(GenDemoBehavior).HandleGenDemoInfo(st, message)
		return reply
	}

	if err := handleDemoRequest(st, m); err != nil {
		// stop with reason
		return err.Error()
	}
	return "noreply"
}

func handleDemoRequest(state *GenDemoProcess, m demoMessage) error {
	return nil
}
func handleDemoDown(state *GenDemoProcess, down DownMessage) error {
	return nil
}
*/
