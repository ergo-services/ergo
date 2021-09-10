package gen

import (
	"fmt"

	"github.com/halturin/ergo/etf"
)

type SagaWorkerBehavior interface {
	ServerBehavior
	// Mandatory callbacks
	HandleStartJob(process *SagaWorkerProcess, job SagaJob) error
	HandleCancelJob(process *SagaWorkerProcess)

	// Optional callbacks
	HandleCommitJob(process *SagaWorkerProcess)

	HandleWorkerInfo(process *SagaWorkerProcess, message etf.Term) (SagaWorkerState, interface{})
	HandleWorkerCast(process *SagaWorkerProcess, message etf.Term) (SagaWorkerState, interface{})
	HandleWorkerCall(process *SagaWorkerProcess, from ServerFrom, message etf.Term) (SagaWorkerState, interface{})
	HandleWorkerDirect(process *SagaWorkerProcess, message interface{}) (interface{}, error)
}

type SagaWorkerState string

const (
	SagaWorkerStateOK    SagaWorkerState = "ok"
	SagaWorkerStateError SagaWorkerState = "error"
	SagaWorkerStateReply SagaWorkerState = "reply"
)

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
	id      SagaJobID
	interim interface{}
}
type messageSagaJobResult struct {
	id     SagaJobID
	result interface{}
}

//
// SagaWorkerProcess methods
//

// SendResult
func (wp *SagaWorkerProcess) SendResult(result interface{}) {
	message := messageSagaJobResult{
		id:     wp.job.ID,
		result: result,
	}
	wp.Cast(wp.job.saga, message)
	wp.job.done = true
}

// SendInterim
func (wp *SagaWorkerProcess) SendInterim(interim interface{}) {
	message := messageSagaJobInterim{
		id:      wp.job.ID,
		interim: interim,
	}
	wp.Cast(wp.job.saga, message)

}

// Server callbacks

func (w *SagaWorker) Init(process *ServerProcess, args ...etf.Term) error {
	workerProcess := &SagaWorkerProcess{
		ServerProcess: *process,
	}
	process.State = workerProcess
	return nil
}

func (w *SagaWorker) HandleCast(process *ServerProcess, message etf.Term) string {
	p := process.State.(*SagaWorkerProcess)
	switch m := message.(type) {
	case messageSagaJobStart:
		p.job = m.job
		err := process.Behavior().(SagaWorkerBehavior).HandleStartJob(p, p.job)

		switch err {
		case nil:
			// if job is done and commit shouldn't be awaited
			// stop this worker with 'normal' as a reason
			if p.job.done && !p.job.commit {
				return "stop"
			}
			return "noreply"
		default:
			return err.Error()
		}
	case messageSagaJobCommit:
		process.Behavior().(SagaWorkerBehavior).HandleCommitJob(p)
		return "stop"
	case messageSagaJobCancel:
		process.Behavior().(SagaWorkerBehavior).HandleCancelJob(p)
		return "stop"
	default:
		return process.Behavior().(SagaWorkerBehavior).HandleWorkerCast(p, message)
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
func (w *SagaWorker) HandleInfo(process *ServerProcess, message etf.Term) string {
	p := process.State.(*SagaWorkerProcess)
	return process.Behavior().(SagaWorkerBehavior).HandleWorkerInfo(p, message)
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
