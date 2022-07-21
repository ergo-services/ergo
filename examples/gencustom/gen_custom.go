package main

import (
	"fmt"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
)

type Custom struct {
	gen.Server
}

type CustomProcess struct {
	gen.ServerProcess
	counter int
}

// CustomBehavior interface
type CustomBehavior interface {
	//
	// Mandatory callbacks
	//

	// InitCustom
	InitCustom(process *CustomProcess, args ...etf.Term) error

	// HandleHello invoked on a 'hello'
	HandleHello(process *CustomProcess) CustomStatus

	//
	// Optional callbacks
	//

	// HandleCustomCall this callback is invoked on ServerProcess.Call. This method is optional
	// for the implementation
	HandleCustomCall(process *CustomProcess, from gen.ServerFrom, message etf.Term) (etf.Term, gen.ServerStatus)
	// HandleCustomDirect this callback is invoked on Process.Direct. This method is optional
	// for the implementation
	HandleCustomDirect(process *CustomProcess, message interface{}) (interface{}, error)
	// HandleCustomCast this callback is invoked on ServerProcess.Cast. This method is optional
	// for the implementation
	HandleCustomCast(process *CustomProcess, message etf.Term) gen.ServerStatus
	// HandleCustomInfo this callback is invoked on Process.Send. This method is optional
	// for the implementation
	HandleCustomInfo(process *CustomProcess, message etf.Term) gen.ServerStatus
}

type CustomStatus error

var (
	CustomStatusOK   CustomStatus = nil
	CustomStatusStop CustomStatus = fmt.Errorf("stop")
)

// default Custom callbacks

func (gd *Custom) HandleCustomCall(process *CustomProcess, from gen.ServerFrom, message etf.Term) (etf.Term, gen.ServerStatus) {
	fmt.Printf("HandleCustomCall: unhandled message (from %#v) %#v\n", from, message)
	return etf.Atom("ok"), gen.ServerStatusOK
}

func (gd *Custom) HandleCustomCast(process *CustomProcess, message etf.Term) gen.ServerStatus {
	fmt.Printf("HandleCustomCast: unhandled message %#v\n", message)
	return gen.ServerStatusOK
}
func (gd *Custom) HandleCustomInfo(process *CustomProcess, message etf.Term) gen.ServerStatus {
	fmt.Printf("HandleCustomInfo: unhandled message %#v\n", message)
	return gen.ServerStatusOK
}

//
// API
//

type messageHello struct{}

func (d *Custom) Hello(process gen.Process) error {
	_, err := process.Direct(messageHello{})
	return err
}

type messageGetStat struct{}

func (d *Custom) Stat(process gen.Process) int {
	counter, _ := process.Direct(messageGetStat{})
	return counter.(int)
}

//
// Custom methods. Available to use inside actors only.
//

func (dp *CustomProcess) Hi() CustomStatus {
	dp.counter = dp.counter * 2
	return CustomStatusOK
}

//
// gen.Server callbacks
//
func (d *Custom) Init(process *gen.ServerProcess, args ...etf.Term) error {
	custom := &CustomProcess{
		ServerProcess: *process,
	}
	// do not inherit parent State
	custom.State = nil

	if err := process.Behavior().(CustomBehavior).InitCustom(custom, args...); err != nil {
		return err
	}
	process.State = custom
	return nil
}

func (gd *Custom) HandleCall(process *gen.ServerProcess, from gen.ServerFrom, message etf.Term) (etf.Term, gen.ServerStatus) {
	custom := process.State.(*CustomProcess)
	return process.Behavior().(CustomBehavior).HandleCustomCall(custom, from, message)
}

func (gd *Custom) HandleDirect(process *gen.ServerProcess, ref etf.Ref, message interface{}) (interface{}, gen.DirectStatus) {
	custom := process.State.(*CustomProcess)
	switch message.(type) {
	case messageGetStat:
		return custom.counter, gen.DirectStatusOK
	case messageHello:
		process.Behavior().(CustomBehavior).HandleHello(custom)
		custom.counter++
		return nil, gen.DirectStatusOK
	default:
		return process.Behavior().(CustomBehavior).HandleCustomDirect(custom, message)
	}

}

func (gd *Custom) HandleCast(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	custom := process.State.(*CustomProcess)
	return process.Behavior().(CustomBehavior).HandleCustomCast(custom, message)
}

func (gd *Custom) HandleInfo(process *gen.ServerProcess, message etf.Term) gen.ServerStatus {
	custom := process.State.(*CustomProcess)
	return process.Behavior().(CustomBehavior).HandleCustomInfo(custom, message)
}
