package gen

import (
	"fmt"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/lib"
)

type PoolBehavior interface {
	ServerBehavior

	InitPool(process *PoolProcess, args ...etf.Term) (PoolOptions, error)
}

type PoolProcess struct {
	ServerProcess
	options  PoolOptions
	workers  []etf.Pid
	monitors map[etf.Ref]int
	i        int
}

type Pool struct {
	Server
}

type PoolOptions struct {
	NumWorkers    int
	Worker        PoolWorkerBehavior
	WorkerOptions ProcessOptions
	WorkerArgs    []etf.Term
}

func (p *Pool) Init(process *ServerProcess, args ...etf.Term) error {
	behavior, ok := process.Behavior().(PoolBehavior)
	if !ok {
		return fmt.Errorf("Pool: not a PoolBehavior")
	}

	pool := &PoolProcess{
		ServerProcess: *process,
		monitors:      make(map[etf.Ref]int),
	}

	// do not inherit parent State
	pool.State = nil
	poolOptions, err := behavior.InitPool(pool, args...)
	if err != nil {
		return err
	}

	poolOptions.WorkerOptions.Context = process.Context()
	pool.options = poolOptions
	process.State = pool

	for i := 0; i < poolOptions.NumWorkers; i++ {
		w, err := process.Spawn("", poolOptions.WorkerOptions, poolOptions.Worker,
			poolOptions.WorkerArgs...)
		if err != nil {
			return err
		}

		pool.workers = append(pool.workers, w.Self())
		ref := process.MonitorProcess(w.Self())
		pool.monitors[ref] = i
	}

	return nil
}

func (p *Pool) HandleCall(process *ServerProcess, from ServerFrom, message etf.Term) (etf.Term, ServerStatus) {
	pool := process.State.(*PoolProcess)
	msg := workerCallMessage{
		from:    from,
		message: message,
	}
	if err := p.send(pool, msg); err != nil {
		lib.Warning("Pool (HandleCall): all workers are busy. Message dropped")
	}
	return nil, ServerStatusIgnore
}
func (p *Pool) HandleCast(process *ServerProcess, message etf.Term) ServerStatus {
	pool := process.State.(*PoolProcess)
	msg := workerCastMessage{
		message: message,
	}
	if err := p.send(pool, msg); err != nil {
		lib.Warning("Pool (HandleCast): all workers are busy. Message dropped")
	}
	return ServerStatusOK
}
func (p *Pool) HandleInfo(process *ServerProcess, message etf.Term) ServerStatus {
	pool := process.State.(*PoolProcess)
	switch m := message.(type) {
	case MessageDown:
		// worker terminated. restart it

		i, exist := pool.monitors[m.Ref]
		if exist == false {
			break
		}
		delete(pool.monitors, m.Ref)
		w, err := process.Spawn("", pool.options.WorkerOptions, pool.options.Worker,
			pool.options.WorkerArgs...)
		if err != nil {
			panicMessage := fmt.Sprintf("Pool: can't restart worker - %s", err)
			panic(panicMessage)
		}
		pool.workers[i] = w.Self()
		return ServerStatusOK
	}

	if err := p.send(pool, message); err != nil {
		lib.Warning("Pool (HandleInfo): all workers are busy. Message dropped")
	}

	return ServerStatusOK
}

func (p *Pool) send(pool *PoolProcess, message etf.Term) error {
	for retry := 0; retry < pool.options.NumWorkers; retry++ {
		pool.i++
		worker := pool.workers[pool.i%pool.options.NumWorkers]
		if err := pool.Send(worker, message); err == nil {
			return ServerStatusOK
		}
	}

	return fmt.Errorf("error")
}
