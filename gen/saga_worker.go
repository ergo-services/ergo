package gen

import (
	"fmt"

	"github.com/halturin/ergo/etf"
)

type SagaWorkerBehavior interface {
	ServerBehavior
	// Mandatory callbacks

	// HandleJobStart invoked on a worker start
	HandleJobStart(process *SagaWorkerProcess, job SagaJob) error
	// HandleJobCancel invoked if transaction was canceled
	HandleJobCancel(process *SagaWorkerProcess)

	// Optional callbacks

	// HandleJobCommit invoked if this job was a part of the transaction with 2PC
	HandleJobCommit(process *SagaWorkerProcess)

	// HandleWorkerInfo this callback is invoked on Process.Send. This method is optional
	// for the implementation
	HandleWorkerInfo(process *SagaWorkerProcess, message etf.Term) ServerStatus
	// HandleWorkerCast this callback is invoked on Process.Cast. This method is optional
	// for the implementation
	HandleWorkerCast(process *SagaWorkerProcess, message etf.Term) ServerStatus
	// HandleWorkerCall this callback is invoked on Process.Call. This method is optional
	// for the implementation
	HandleWorkerCall(process *SagaWorkerProcess, from ServerFrom, message etf.Term) (etf.Term, ServerStatus)
	// HandleWorkerDirect this callback is invoked on Process.Direct. This method is optional
	// for the implementation
	HandleWorkerDirect(process *SagaWorkerProcess, message interface{}) (interface{}, error)
}

type SagaWorker struct {
	Server
}

type SagaWorkerProcess struct {
	ServerProcess
	job  SagaJob
	done bool
}

type messageSagaJobStart struct {
	job SagaJob
}
type messageSagaJobCancel struct{}
type messageSagaJobCommit struct{}
type messageSagaJobInterim struct {
	pid     etf.Pid
	interim interface{}
}
type messageSagaJobResult struct {
	pid    etf.Pid
	result interface{}
}

//
// SagaWorkerProcess methods
//

// SendResult
func (wp *SagaWorkerProcess) SendResult(result interface{}) error {
	message := messageSagaJobResult{
		pid:    wp.Self(),
		result: result,
	}
	// must be a sync request to keep an order of sending messages
	// and to ensure the result has been delivered
	err := wp.Cast(wp.job.saga, message)
	if err != nil {
		return err
	}
	wp.done = true
	return nil
}

// SendInterim
func (wp *SagaWorkerProcess) SendInterim(interim interface{}) error {
	message := messageSagaJobInterim{
		pid:     wp.Self(),
		interim: interim,
	}
	// must be a sync request to keep an order of sending messages
	// and to ensure the result has been delivered
	return wp.Cast(wp.job.saga, message)
}

// Server callbacks

func (w *SagaWorker) Init(process *ServerProcess, args ...etf.Term) error {
	workerProcess := &SagaWorkerProcess{
		ServerProcess: *process,
	}
	process.State = workerProcess
	return nil
}

func (w *SagaWorker) HandleCast(process *ServerProcess, message etf.Term) ServerStatus {
	p := process.State.(*SagaWorkerProcess)
	switch m := message.(type) {
	case messageSagaJobStart:
		p.job = m.job
		err := process.Behavior().(SagaWorkerBehavior).HandleJobStart(p, p.job)
		if err != nil {
			return err
		}

		// if job is done and 2PC is disabled
		// stop this worker with 'normal' as a reason
		if p.done && !p.job.commit {
			return ServerStatusStop
		}
		return ServerStatusOK
	case messageSagaJobCommit:
		process.Behavior().(SagaWorkerBehavior).HandleJobCommit(p)
		return ServerStatusStop
	case messageSagaJobCancel:
		process.Behavior().(SagaWorkerBehavior).HandleJobCancel(p)
		return ServerStatusStop
	default:
		return process.Behavior().(SagaWorkerBehavior).HandleWorkerCast(p, message)
	}
}

func (w *SagaWorker) HandleCall(process *ServerProcess, from ServerFrom, message etf.Term) (etf.Term, ServerStatus) {
	p := process.State.(*SagaWorkerProcess)
	return process.Behavior().(SagaWorkerBehavior).HandleWorkerCall(p, from, message)
}

func (w *SagaWorker) HandleDirect(process *ServerProcess, message interface{}) (interface{}, error) {
	p := process.State.(*SagaWorkerProcess)
	return process.Behavior().(SagaWorkerBehavior).HandleWorkerDirect(p, message)
}
func (w *SagaWorker) HandleInfo(process *ServerProcess, message etf.Term) ServerStatus {
	p := process.State.(*SagaWorkerProcess)
	return process.Behavior().(SagaWorkerBehavior).HandleWorkerInfo(p, message)
}

// default callbacks
func (w *SagaWorker) HandleJobCommit(process *SagaWorkerProcess) {
	return
}
func (w *SagaWorker) HandleWorkerInfo(process *SagaWorkerProcess, message etf.Term) ServerStatus {
	fmt.Printf("HandleWorkerInfo: unhandled message %#v\n", message)
	return ServerStatusOK
}
func (w *SagaWorker) HandleWorkerCast(process *SagaWorkerProcess, message etf.Term) ServerStatus {
	fmt.Printf("HandleWorkerCast: unhandled message %#v\n", message)
	return ServerStatusOK
}
func (w *SagaWorker) HandleWorkerCall(process *SagaWorkerProcess, from ServerFrom, message etf.Term) (etf.Term, ServerStatus) {
	fmt.Printf("HandleWorkerCall: unhandled message (from %#v) %#v\n", from, message)
	return etf.Atom("ok"), ServerStatusOK
}
func (w *SagaWorker) HandleWorkerDirect(process *SagaWorkerProcess, message interface{}) (interface{}, error) {
	fmt.Printf("HandleWorkerDirect: unhandled message %#v\n", message)
	return nil, nil
}
