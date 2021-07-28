package ergo

import (
	"fmt"

	"github.com/halturin/ergo/etf"
)

type GenSagaWorkerBehavior interface {
	// Mandatory callbacks
	HandleStartJob(state *GenSagaWorkerState) error
	HandleCancelJob(state *GenSagaWorkerState)

	// Optional callbacks
	HandleCommitJob(state *GenSagaWorkerState)

	HandleWorkerInfo(state *GenSagaWorkerState, message etf.Term) string
	HandleWorkerCast(state *GenSagaWorkerState, message etf.Term) string
	HandleWorkerCall(state *GenSagaWorkerState, from GenServerFrom, message etf.Term) (string, interface{})
	HandleWorkerDirect(state *GenSagaWorkerState, message interface{}) (interface{}, error)
}

type GenSagaWorker struct {
	GenServer
}

type GenSagaWorkerState struct {
	GenServerState
	Job GenSagaWorkerJob
}

type GenSagaWorkerJob struct {
	Ref   etf.Ref
	Value interface{}
	saga  etf.Pid
}

type messageSagaWorkerJobStart struct{}
type messageSagaWorkerJobCancel struct{}
type messageSagaWorkerJobCommit struct{}
type messageSagaWorkerJobInterim struct {
	ref     etf.Ref
	interim interface{}
}
type messageSagaWorkerJobResult struct {
	ref    etf.Ref
	result interface{}
}

func (w *GenSagaWorker) SendResult(process *Process, job GenSagaWorkerJob, result interface{}) {
	message := messageSagaWorkerJobResult{
		ref:    job.Ref,
		result: result,
	}
	process.Cast(job.saga, message)
}

func (w *GenSagaWorker) SendInterim(process *Process, job GenSagaWorkerJob, interim interface{}) {
	message := messageSagaWorkerJobInterim{
		ref:     job.Ref,
		interim: interim,
	}
	process.Cast(job.saga, message)
}

func (w *GenSagaWorker) Init(state *GenServerState, args ...interface{}) error {
	job := args[0].(GenSagaWorkerJob)
	workerState := &GenSagaWorkerState{
		GenServerState: *state,
		Job:            job,
	}
	state.State = workerState
	state.Process.Send(state.Process.Self(), messageSagaWorkerJobStart{})
	return nil
}

func (w *GenSagaWorker) HandleInfo(state *GenServerState, message etf.Term) string {
	st := state.State.(*GenSagaWorkerState)
	switch message.(type) {
	case messageSagaWorkerJobStart:
		err := state.Process.GetObject().(GenSagaWorkerBehavior).HandleStartJob(st)
		switch err {
		case nil:
			return "noreply"
		case ErrStop:
			return "stop"
		default:
			return err.Error()
		}
	case messageSagaWorkerJobCommit:
		state.Process.GetObject().(GenSagaWorkerBehavior).HandleCommitJob(st)
		return "stop"
	case messageSagaWorkerJobCancel:
		state.Process.GetObject().(GenSagaWorkerBehavior).HandleCancelJob(st)
		return "stop"
	default:
		return state.Process.GetObject().(GenSagaWorkerBehavior).HandleWorkerInfo(st, message)
	}
}

func (w *GenSagaWorker) HandleCall(state *GenServerState, from GenServerFrom, message etf.Term) (string, etf.Term) {
	st := state.State.(*GenSagaWorkerState)
	return state.Process.GetObject().(GenSagaWorkerBehavior).HandleWorkerCall(st, from, message)
}

func (w *GenSagaWorker) HandleDirect(state *GenServerState, message interface{}) (interface{}, error) {
	st := state.State.(*GenSagaWorkerState)
	return state.Process.GetObject().(GenSagaWorkerBehavior).HandleWorkerDirect(st, message)
}
func (w *GenSagaWorker) HandleCast(state *GenServerState, message etf.Term) string {
	st := state.State.(*GenSagaWorkerState)
	return state.Process.GetObject().(GenSagaWorkerBehavior).HandleWorkerCast(st, message)
}

// default callbacks
func (w *GenSagaWorker) HandleCommitJob(state *GenSagaWorkerState) {
	return
}
func (w *GenSagaWorker) HandleWorkerInfo(state *GenSagaWorkerState, message etf.Term) string {
	fmt.Printf("HandleWorkerInfo: unhandled message %#v\n", message)
	return "noreply"
}
func (w *GenSagaWorker) HandleWorkerCast(state *GenSagaWorkerState, message etf.Term) string {
	fmt.Printf("HandleWorkerCast: unhandled message %#v\n", message)
	return "noreply"
}
func (w *GenSagaWorker) HandleWorkerCall(state *GenSagaWorkerState, from GenServerFrom, message etf.Term) (string, interface{}) {
	fmt.Printf("HandleWorkerCall: unhandled message (from %#v) %#v\n", from, message)
	return "reply", etf.Atom("ok")
}
func (w *GenSagaWorker) HandleWorkerDirect(state *GenSagaWorkerState, message interface{}) (interface{}, error) {

	fmt.Printf("HandleWorkerDirect: unhandled message %#v\n", message)
	return nil, nil
}
