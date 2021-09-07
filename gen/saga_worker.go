package gen

import (
	"fmt"

	"github.com/halturin/ergo/etf"
)

type SagaWorkerBehavior interface {
	ProcessBehavior
	// Mandatory callbacks
	HandleStartJob(state *SagaWorkerState) error
	HandleCancelJob(state *SagaWorkerState)

	// Optional callbacks
	HandleCommitJob(state *SagaWorkerState)

	HandleWorkerInfo(state *SagaWorkerState, message etf.Term) string
	HandleWorkerCast(state *SagaWorkerState, message etf.Term) string
	HandleWorkerCall(state *SagaWorkerState, from ServerFrom, message etf.Term) (string, interface{})
	HandleWorkerDirect(state *SagaWorkerState, message interface{}) (interface{}, error)
}

type SagaWorker struct {
	Server
}

type SagaWorkerState struct {
	ServerState
	Job SagaJob
}

type SagaJob struct {
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

func (w *SagaWorker) SendResult(process *Process, job SagaJob, result interface{}) {
	message := messageSagaWorkerJobResult{
		ref:    job.Ref,
		result: result,
	}
	process.Cast(job.saga, message)
}

func (w *SagaWorker) SendInterim(process *Process, job SagaJob, interim interface{}) {
	message := messageSagaWorkerJobInterim{
		ref:     job.Ref,
		interim: interim,
	}
	process.Cast(job.saga, message)
}

func (w *SagaWorker) Init(state *ServerState, args ...etf.Term) error {
	job := args[0].(SagaJob)
	workerState := &SagaWorkerState{
		ServerState: *state,
		Job:         job,
	}
	state.State = workerState
	state.Process.Send(state.Process.Self(), messageSagaWorkerJobStart{})
	return nil
}

func (w *SagaWorker) HandleInfo(state *ServerState, message etf.Term) string {
	st := state.State.(*SagaWorkerState)
	switch message.(type) {
	case messageSagaWorkerJobStart:
		err := state.Process.Behavior().(SagaWorkerBehavior).HandleStartJob(st)
		switch err {
		case nil:
			return "noreply"
		case ErrStop:
			return "stop"
		default:
			return err.Error()
		}
	case messageSagaWorkerJobCommit:
		state.Process.Behavior().(SagaWorkerBehavior).HandleCommitJob(st)
		return "stop"
	case messageSagaWorkerJobCancel:
		state.Process.Behavior().(SagaWorkerBehavior).HandleCancelJob(st)
		return "stop"
	default:
		return state.Process.Behavior().(SagaWorkerBehavior).HandleWorkerInfo(st, message)
	}
}

func (w *SagaWorker) HandleCall(state *ServerState, from ServerFrom, message etf.Term) (string, etf.Term) {
	st := state.State.(*SagaWorkerState)
	return state.Process.Behavior().(SagaWorkerBehavior).HandleWorkerCall(st, from, message)
}

func (w *SagaWorker) HandleDirect(state *ServerState, message interface{}) (interface{}, error) {
	st := state.State.(*SagaWorkerState)
	return state.Process.Behavior().(SagaWorkerBehavior).HandleWorkerDirect(st, message)
}
func (w *SagaWorker) HandleCast(state *ServerState, message etf.Term) string {
	st := state.State.(*SagaWorkerState)
	return state.Process.Behavior().(SagaWorkerBehavior).HandleWorkerCast(st, message)
}

// default callbacks
func (w *SagaWorker) HandleCommitJob(state *SagaWorkerState) {
	return
}
func (w *SagaWorker) HandleWorkerInfo(state *SagaWorkerState, message etf.Term) string {
	fmt.Printf("HandleWorkerInfo: unhandled message %#v\n", message)
	return "noreply"
}
func (w *SagaWorker) HandleWorkerCast(state *SagaWorkerState, message etf.Term) string {
	fmt.Printf("HandleWorkerCast: unhandled message %#v\n", message)
	return "noreply"
}
func (w *SagaWorker) HandleWorkerCall(state *SagaWorkerState, from ServerFrom, message etf.Term) (string, interface{}) {
	fmt.Printf("HandleWorkerCall: unhandled message (from %#v) %#v\n", from, message)
	return "reply", etf.Atom("ok")
}
func (w *SagaWorker) HandleWorkerDirect(state *SagaWorkerState, message interface{}) (interface{}, error) {

	fmt.Printf("HandleWorkerDirect: unhandled message %#v\n", message)
	return nil, nil
}
