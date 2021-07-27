package main

import (
	"fmt"

	"github.com/halturin/ergo"
	"github.com/halturin/ergo/etf"
)

type GenDemo struct {
	ergo.GenServer
}

type GenDemoOptions struct {
	A int
	B string
}

type GenDemoState struct {
	ergo.GenServerState
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
	InitDemo(state *GenDemoState, args ...interface{}) error

	// HandleHello invoked on a 'hello' request where 'n' is how many times it was received
	HandleHello(state *GenDemoState, n int) error

	//
	// Optional callbacks
	//

	HandleHi(state *GenDemoState) error

	// HandleGenDemoCall this callback is invoked on Process.Call. This method is optional
	// for the implementation
	HandleGenDemoCall(state *GenDemoState, from ergo.GenServerFrom, message etf.Term) (string, etf.Term)
	// HandleGenDemoCast this callback is invoked on Process.Cast. This method is optional
	// for the implementation
	HandleGenDemoCast(state *GenDemoState, message etf.Term) string
	// HandleGenDemoInfo this callback is invoked on Process.Send. This method is optional
	// for the implementation
	HandleGenDemoInfo(state *GenDemoState, message etf.Term) string
}

// default GenDemo callbacks

func (gd *GenDemo) HandleHi(state *GenDemoState) error {
	fmt.Printf("HandleHi: unhandled message %#v\n", tx)
	return nil
}

func (gd *GenDemo) HandleGenDemoCall(state *GenDemoState, from ergo.GenServerFrom, message etf.Term) (string, etf.Term) {
	fmt.Printf("HandleGenDemoCall: unhandled message (from %#v) %#v\n", from, message)
	return "reply", etf.Atom("ok")
}

func (gd *GenDemo) HandleGenDemoCast(state *GenDemoState, message etf.Term) string {
	fmt.Printf("HandleGenDemoCast: unhandled message %#v\n", message)
	return "noreply"
}
func (gd *GenDemo) HandleGenDemoInfo(state *GenDemoState, message etf.Term) string {
	fmt.Printf("HandleGenDemoInfo: unhandled message %#v\n", message)
	return "noreply"
}

// API

func (gd *GenDemo) SetCounter(process *ergo.Process, c int) error {
	_, err := process.Direct(setCounter{c: c})
	return err
}

func (gd *GenDemo) GetCounter(process *ergo.Process) (int, error) {
	return process.Direct(getCounter{})

}

//
// GenServer callbacks
//
func (gd *GenDemo) Init(state *ergo.GenServerState, args ...interface{}) error {
	demoState := &GenDemoState{
		ergo.GenServerState: *state,
	}
	if err := state.Process.GetObject().(GenDemoBehavior).InitDemo(demoState, args...); err != nil {
		return err
	}
	state.State = demoState
	return nil
}

func (gd *GenDemo) HandleCall(state *ergo.GenServerState, from ergo.GenServerFrom, message etf.Term) (string, etf.Term) {
	st := state.State.(*GenDemoState)
	return state.Process.GetObject().(GenDemoBehavior).HandleGenDemoCall(st, from, message)
}

func (gd *GenDemo) HandleDirect(state *ergo.GenServerState, message interface{}) (interface{}, error) {
	st := state.State.(*GenDemoState)
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

func (gd *GenDemo) HandleCast(state *ergo.GenServerState, message etf.Term) string {
	st := state.State.(*GenDemoState)
	return state.Process.GetObject().(GenDemoBehavior).HandleGenDemoCast(st, message)
}

func (gd *GenDemo) HandleInfo(state *ergo.GenServerState, message etf.Term) string {
	var d DownMessage
	var m demoMessage

	st := state.State.(*GenDemoState)
	// check if we got a 'DOWN' message
	// {DOWN, Ref, process, PidOrName, Reason}
	if isDown, d := IsDownMessage(message); isDown {
		if err := handleDemoDown(st, d); err != nil {
			return err.Error()
		}
		return "noreply"
	}

	if err := etf.TermIntoStruct(message, &m); err != nil {
		reply := state.Process.GetObject().(GenDemoBehavior).HandleGenDemoInfo(st, message)
		return reply
	}

	if err := handleDemoRequest(st, m); err != nil {
		// stop with reason
		return err.Error()
	}
	return "noreply"
}

func handleDemoRequest(state *GenDemoState, m demoMessage) error {
	return nil
}
func handleDemoDown(state *GenDemoState, down DownMessage) error {
	return nil
}
