package gen

import (
	"fmt"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/lib"
)

type workerCastMessage struct {
	message etf.Term
}
type workerCallMessage struct {
	from    ServerFrom
	message etf.Term
}

type PoolWorkerBehavior interface {
	ServerBehavior
	InitPoolWorker(process *PoolWorkerProcess, args ...etf.Term) error
	HandleWorkerInfo(process *PoolWorkerProcess, message etf.Term)
	HandleWorkerCast(process *PoolWorkerProcess, message etf.Term)
	HandleWorkerCall(process *PoolWorkerProcess, message etf.Term) etf.Term
}

type PoolWorkerProcess struct {
	ServerProcess
}

type PoolWorker struct {
	Server
}

func (pw *PoolWorker) Init(process *ServerProcess, args ...etf.Term) error {
	behavior, ok := process.Behavior().(PoolWorkerBehavior)

	if !ok {
		return fmt.Errorf("Pool: not a PoolWorkerBehavior")
	}

	worker := &PoolWorkerProcess{
		ServerProcess: *process,
	}

	worker.State = nil

	if err := behavior.InitPoolWorker(worker, args...); err != nil {
		return err
	}

	process.State = worker
	return nil
}

func (pw *PoolWorker) HandleInfo(process *ServerProcess, message etf.Term) ServerStatus {
	worker := process.State.(*PoolWorkerProcess)
	behavior := worker.Behavior().(PoolWorkerBehavior)
	switch m := message.(type) {
	case workerCallMessage:
		result := behavior.HandleWorkerCall(worker, m.message)
		process.SendReply(m.from, result)
	case workerCastMessage:
		behavior.HandleWorkerCast(worker, m.message)
	default:
		behavior.HandleWorkerInfo(worker, message)
	}
	return ServerStatusOK
}

// HandleWorkerInfo
func (pw *PoolWorker) HandleWorkerInfo(process *PoolWorkerProcess, message etf.Term) {
	lib.Warning("HandleWorkerInfo: unhandled message %#v", message)
}

// HandleWorkerCast
func (pw *PoolWorker) HandleWorkerCast(process *PoolWorkerProcess, message etf.Term) {
	lib.Warning("HandleWorkerCast: unhandled message %#v", message)
}

// HandleWorkerCall
func (pw *PoolWorker) HandleWorkerCall(process *PoolWorkerProcess, message etf.Term) etf.Term {
	lib.Warning("HandleWorkerCall: unhandled message %#v", message)
	return nil
}
