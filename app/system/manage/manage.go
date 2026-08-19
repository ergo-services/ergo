package manage

import (
	"errors"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

const (
	Name gen.Atom = "system_manage"

	// EnvPoolSize carries the worker count from the application spec.
	EnvPoolSize gen.Env = "manage_pool_size"

	// DefaultPoolSize is larger than the inspect pool: a mutation can take seconds
	// and the confirmed response holds the worker on top of that.
	DefaultPoolSize int64 = 20

	// depth 1 spreads a burst across workers; an overflow is dropped, not queued
	workerMailboxSize int64 = 1
)

func Factory() gen.ProcessBehavior {
	return &pool{}
}

type pool struct {
	act.Pool
}

func (p *pool) Init(args ...any) (act.PoolOptions, error) {
	size := DefaultPoolSize
	if v, exist := p.Env(EnvPoolSize); exist {
		if n, ok := v.(int); ok && n > 0 {
			size = int64(n)
		}
	}

	return act.PoolOptions{
		PoolSize:          size,
		WorkerMailboxSize: workerMailboxSize,
		WorkerFactory:     workerFactory,
	}, nil
}

func workerFactory() gen.ProcessBehavior {
	return &manage{}
}

type manage struct {
	act.Actor
}

func (m *manage) Init(args ...any) error {
	m.Log().SetLogger("default")
	m.Log().Debug("%s started", m.Name())
	m.SetCompression(true)
	return nil
}

// HandleCall applies one mutation and answers with a confirmed response. The
// deadline is checked before and after applying; an answer that provably never
// arrived rolls the mutation back where that is possible.
func (m *manage) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	op, known := m.plan(request)
	if known == false {
		m.Log().Error("unsupported request: %#v", request)
		return gen.ErrUnsupported, nil
	}

	if ref.IsAlive() == false {
		m.Log().Warning("%s on %s dropped: caller %s is past its deadline, nothing applied",
			op.name, op.target, from)
		return nil, nil
	}

	response := op.apply()

	if ref.IsAlive() == false {
		m.rollback(op, "deadline expired while applying")
		return nil, nil
	}

	switch err := m.SendResponseImportant(from, ref, response); {
	case err == nil:

	case errors.Is(err, gen.ErrResponseIgnored):
		m.rollback(op, "caller is no longer waiting")

	default:
		// the response may have been delivered, so undoing would diverge from the caller
		m.Log().Error("%s on %s applied, response to %s unconfirmed (%s), change stands",
			op.name, op.target, from, err)
	}

	return nil, nil
}

// rollback undoes a mutation whose result never reached the caller.
func (m *manage) rollback(op operation, reason string) {
	if op.undo == nil {
		m.Log().Error("%s on %s applied and cannot be undone (%s), change stands",
			op.name, op.target, reason)
		return
	}

	if err := op.undo(); err != nil {
		m.Log().Error("%s on %s rollback failed: %s (%s), change stands",
			op.name, op.target, err, reason)
		return
	}

	m.Log().Warning("%s on %s rolled back: %s", op.name, op.target, reason)
}

func (m *manage) Terminate(reason error) {
	m.Log().Debug("%s terminated: %s", m.Name(), reason)
}

func (m *manage) HandleInspect(from gen.PID, item ...string) map[string]string {
	return map[string]string{
		"plane": "manage",
	}
}
