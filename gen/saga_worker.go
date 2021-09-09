package gen

import (
	"fmt"

	"github.com/halturin/ergo/etf"
)

type SagaWorkerBehavior interface {
	ProcessBehavior
	// Mandatory callbacks
	HandleStartJob(process *SagaWorkerProcess) error
	HandleCancelJob(process *SagaWorkerProcess)

	// Optional callbacks
	HandleCommitJob(process *SagaWorkerProcess)

	HandleWorkerInfo(process *SagaWorkerProcess, message etf.Term) string
	HandleWorkerCast(process *SagaWorkerProcess, message etf.Term) string
	HandleWorkerCall(process *SagaWorkerProcess, from ServerFrom, message etf.Term) (string, interface{})
	HandleWorkerDirect(process *SagaWorkerProcess, message interface{}) (interface{}, error)
}

type SagaWorker struct {
	Server
}

type SagaWorkerProcess struct {
	ServerProcess
	Job SagaJob
}

type SagaJobID etf.Ref

type SagaJob struct {
	ID    SagaJobID
	Value interface{}
	saga  etf.Pid
}

type messageSagaWorkerJobStart struct{}
type messageSagaWorkerJobCancel struct{}
type messageSagaWorkerJobCommit struct{}
type messageSagaWorkerJobInterim struct {
	id      SagaJobID
	interim interface{}
}
type messageSagaWorkerJobResult struct {
	id     SagaJobID
	result interface{}
}

func (w *SagaWorker) SendResult(process Process, job SagaJob, result interface{}) {
	message := messageSagaWorkerJobResult{
		id:     job.ID,
		result: result,
	}
	process.Cast(job.saga, message)
}

func (w *SagaWorker) SendInterim(process Process, job SagaJob, interim interface{}) {
	message := messageSagaWorkerJobInterim{
		id:      job.ID,
		interim: interim,
	}
	process.Cast(job.saga, message)
}

func (w *SagaWorker) Init(process *ServerProcess, args ...etf.Term) error {
	job, ok := args[0].(SagaJob)
	if !ok {
		return fmt.Errorf("not a gen.SagaJob")
	}
	workerProcess := &SagaWorkerProcess{
		ServerProcess: *process,
		Job:           job,
	}
	process.State = workerProcess
	process.Send(process.Self(), messageSagaWorkerJobStart{})
	return nil
}

func (w *SagaWorker) HandleInfo(process *ServerProcess, message etf.Term) string {
	p := process.State.(*SagaWorkerProcess)
	switch message.(type) {
	case messageSagaWorkerJobStart:
		err := process.Behavior().(SagaWorkerBehavior).HandleStartJob(p)
		switch err {
		case nil:
			return "noreply"
		case ErrStop:
			return "stop"
		default:
			return err.Error()
		}
	case messageSagaWorkerJobCommit:
		process.Behavior().(SagaWorkerBehavior).HandleCommitJob(p)
		return "stop"
	case messageSagaWorkerJobCancel:
		process.Behavior().(SagaWorkerBehavior).HandleCancelJob(p)
		return "stop"
	default:
		return process.Behavior().(SagaWorkerBehavior).HandleWorkerInfo(p, message)
	}
}

func (w *SagaWorker) HandleCall(process *ServerProcess, from ServerFrom, message etf.Term) (string, etf.Term) {
	p := process.State.(*SagaWorkerProcess)
	return process.Behavior().(SagaWorkerBehavior).HandleWorkerCall(p, from, message)
}

func (w *SagaWorker) HandleDirect(process *ServerProcess, message interface{}) (interface{}, error) {
	p := process.State.(*SagaWorkerProcess)
	return process.Behavior().(SagaWorkerBehavior).HandleWorkerDirect(p, message)
}
func (w *SagaWorker) HandleCast(process *ServerProcess, message etf.Term) string {
	p := process.State.(*SagaWorkerProcess)
	return process.Behavior().(SagaWorkerBehavior).HandleWorkerCast(p, message)
}

// default callbacks
func (w *SagaWorker) HandleCommitJob(process *SagaWorkerProcess) {
	return
}
func (w *SagaWorker) HandleWorkerInfo(process *SagaWorkerProcess, message etf.Term) string {
	fmt.Printf("HandleWorkerInfo: unhandled message %#v\n", message)
	return "noreply"
}
func (w *SagaWorker) HandleWorkerCast(process *SagaWorkerProcess, message etf.Term) string {
	fmt.Printf("HandleWorkerCast: unhandled message %#v\n", message)
	return "noreply"
}
func (w *SagaWorker) HandleWorkerCall(process *SagaWorkerProcess, from ServerFrom, message etf.Term) (string, interface{}) {
	fmt.Printf("HandleWorkerCall: unhandled message (from %#v) %#v\n", from, message)
	return "reply", etf.Atom("ok")
}
func (w *SagaWorker) HandleWorkerDirect(process *SagaWorkerProcess, message interface{}) (interface{}, error) {
	fmt.Printf("HandleWorkerDirect: unhandled message %#v\n", message)
	return nil, nil
}
