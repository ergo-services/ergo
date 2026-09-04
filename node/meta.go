package node

import (
	"sync/atomic"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

type meta struct {
	// fields were reordered to have small memory footprint
	behavior gen.MetaBehavior

	main   lib.QueueMPSC
	system lib.QueueMPSC

	p   *process
	log *log

	sbehavior string
	id        gen.Alias

	messagesIn  uint64
	messagesOut uint64

	priority    atomic.Int32 // gen.MessagePriority; mutable from node-level setter
	compression atomic.Bool  // mutable via SetCompression concurrently with senders

	creation int64 // used for the meta process Uptime method only
	state    int32
}

func (m *meta) ID() gen.Alias {
	return m.id
}

func (m *meta) Parent() gen.PID {
	return m.p.pid
}

func (m *meta) SendPriority() gen.MessagePriority {
	return gen.MessagePriority(m.priority.Load())
}

func (m *meta) SetSendPriority(priority gen.MessagePriority) error {
	state := atomic.LoadInt32(&m.state)
	if gen.MetaState(state) != gen.MetaStateRunning {
		return gen.ErrNotAllowed
	}
	m.priority.Store(int32(priority))
	return nil
}

func (m *meta) Send(to any, message any) error {
	if err := m.send(to, message, gen.MessagePriority(m.priority.Load())); err != nil {
		return err
	}
	return nil
}

func (m *meta) SendWithPriority(to any, message any, priority gen.MessagePriority) error {
	return m.send(to, message, priority)
}

func (m *meta) SendAfter(to any, message any, after time.Duration) (gen.CancelFunc, error) {
	return m.sendDeferred(to, message, gen.MessagePriority(m.priority.Load()), after, false)
}

func (m *meta) SendWithPriorityAfter(
	to any,
	message any,
	priority gen.MessagePriority,
	after time.Duration,
) (gen.CancelFunc, error) {
	return m.sendDeferred(to, message, priority, after, false)
}

func (m *meta) SendEvery(to any, message any, period time.Duration) (gen.CancelFunc, error) {
	return m.sendDeferred(to, message, gen.MessagePriority(m.priority.Load()), period, true)
}

func (m *meta) SendWithPriorityEvery(
	to any,
	message any,
	priority gen.MessagePriority,
	period time.Duration,
) (gen.CancelFunc, error) {
	return m.sendDeferred(to, message, priority, period, true)
}

func (m *meta) SendResponse(to gen.PID, ref gen.Ref, message any) error {
	state := atomic.LoadInt32(&m.state)
	if gen.MetaState(state) != gen.MetaStateRunning {
		return gen.ErrNotAllowed
	}

	compression := *m.p.compression.Load()
	compression.Enable = m.compression.Load()

	options := gen.MessageOptions{
		Ref:              ref,
		Priority:         gen.MessagePriority(m.priority.Load()),
		Compression:      compression,
		KeepNetworkOrder: m.p.keeporder.Load(),
	}
	if err := m.p.core.RouteSendResponse(m.p.pid, to, options, message); err != nil {
		return err
	}
	atomic.AddUint64(&m.messagesOut, 1)
	return nil
}

func (m *meta) SendResponseError(to gen.PID, ref gen.Ref, err error) error {
	state := atomic.LoadInt32(&m.state)
	if gen.MetaState(state) != gen.MetaStateRunning {
		return gen.ErrNotAllowed
	}

	compression := *m.p.compression.Load()
	compression.Enable = m.compression.Load()

	options := gen.MessageOptions{
		Ref:              ref,
		Priority:         gen.MessagePriority(m.priority.Load()),
		Compression:      compression,
		KeepNetworkOrder: m.p.keeporder.Load(),
	}
	if rerr := m.p.core.RouteSendResponseError(m.p.pid, to, options, err); rerr != nil {
		return rerr
	}
	atomic.AddUint64(&m.messagesOut, 1)
	return nil
}

func (m *meta) Spawn(behavior gen.MetaBehavior, options gen.MetaOptions) (gen.Alias, error) {
	var alias gen.Alias
	state := atomic.LoadInt32(&m.state)

	if state == int32(gen.MetaStateTerminated) {
		return alias, gen.ErrNotAllowed
	}

	return m.p.spawnMeta(behavior, options)
}

func (m *meta) Env(name gen.Env) (any, bool) {
	return m.p.Env(name)
}

func (m *meta) EnvList() map[gen.Env]any {
	return m.p.EnvList()
}

func (m *meta) EnvDefault(name gen.Env, def any) any {
	if val, ok := m.p.Env(name); ok {
		return val
	}
	return def
}

func (m *meta) Log() gen.Log {
	return m.log
}

func (m *meta) Compression() bool {
	return m.compression.Load()
}

func (m *meta) SetCompression(enabled bool) error {
	state := atomic.LoadInt32(&m.state)
	if gen.MetaState(state) != gen.MetaStateRunning {
		return gen.ErrNotAllowed
	}
	m.compression.Store(enabled)
	return nil
}

// sendDeferred schedules a delayed send: once after d (repeat=false) or every d (repeat=true).
func (m *meta) sendDeferred(to any, message any, priority gen.MessagePriority, d time.Duration, repeat bool) (gen.CancelFunc, error) {
	if gen.MetaState(atomic.LoadInt32(&m.state)) == gen.MetaStateTerminated {
		return nil, gen.ErrNotAllowed
	}
	if repeat && d <= 0 {
		return nil, gen.ErrIncorrect
	}
	switch to.(type) {
	case gen.PID, gen.ProcessID, gen.Alias, gen.Atom:
	default:
		return nil, gen.ErrIncorrect
	}

	if repeat == false {
		return time.AfterFunc(d, func() {
			if m.alive() == false {
				return
			}
			if lib.Verbose() {
				m.log.Trace("send after %s to %s (priority %s)", d, to, priority)
			}
			m.send(to, message, priority)
		}).Stop, nil
	}

	var stopped atomic.Bool
	var t *time.Timer
	// armed far out first so the callback can't read t before it is set
	t = time.AfterFunc(time.Hour, func() {
		if stopped.Load() || m.alive() == false {
			t.Stop()
			return
		}
		if lib.Verbose() {
			m.log.Trace("send every %s to %s (priority %s)", d, to, priority)
		}
		m.send(to, message, priority)
		if stopped.Load() == false {
			t.Reset(d)
		}
	})
	t.Reset(d)
	return func() bool {
		t.Stop()
		return stopped.Swap(true) == false
	}, nil
}

func (m *meta) alive() bool {
	if gen.MetaState(atomic.LoadInt32(&m.state)) == gen.MetaStateTerminated {
		return false
	}
	return m.p.isAlive()
}

func (m *meta) send(to any, message any, priority gen.MessagePriority) error {
	compression := *m.p.compression.Load()
	compression.Enable = m.compression.Load()

	options := gen.MessageOptions{
		Priority:         priority,
		Compression:      compression,
		KeepNetworkOrder: m.p.keeporder.Load(),
	}

	switch t := to.(type) {
	case gen.PID:
		if t == m.p.pid {
			// sending to itself
			qm := gen.TakeMailboxMessage()
			qm.From = m.p.pid
			qm.Type = gen.MailboxMessageTypeRegular
			qm.Target = to
			qm.Message = message

			var queue lib.QueueMPSC
			switch priority {
			case gen.MessagePriorityHigh:
				queue = m.p.mailbox.System
			case gen.MessagePriorityMax:
				queue = m.p.mailbox.Urgent
			default:
				queue = m.p.mailbox.Main
			}

			if ok := queue.Push(qm); ok == false {
				return gen.ErrProcessMailboxFull
			}

			// manualy routed message to itself
			// so we need to increase messagesIn counter there
			// and run the process
			atomic.AddUint64(&m.p.messagesIn, 1)
			m.p.run()

			atomic.AddUint64(&m.messagesOut, 1)
			return nil
		}

		if err := m.p.core.RouteSendPID(m.p.pid, t, options, message); err != nil {
			return err
		}
	case gen.Atom:
		if err := m.p.core.RouteSendProcessID(m.p.pid, gen.ProcessID{Name: t}, options, message); err != nil {
			return err
		}
	case gen.ProcessID:
		if err := m.p.core.RouteSendProcessID(m.p.pid, t, options, message); err != nil {
			return err
		}
	case gen.Alias:
		if t == m.id {
			// self-send to own alias: skip the node-level alias lookup
			// (mirrors the gen.PID self-send fast path above)
			qm := gen.TakeMailboxMessage()
			qm.From = m.p.pid
			qm.Type = gen.MailboxMessageTypeRegular
			qm.Target = to
			qm.Message = message

			if ok := m.main.Push(qm); ok == false {
				return gen.ErrMetaMailboxFull
			}
			atomic.AddUint64(&m.messagesIn, 1)
			m.handle()

			atomic.AddUint64(&m.messagesOut, 1)
			return nil
		}

		if err := m.p.core.RouteSendAlias(m.p.pid, t, options, message); err != nil {
			return err
		}
	default:
		return gen.ErrIncorrect
	}

	atomic.AddUint64(&m.messagesOut, 1)
	return nil
}

func (m *meta) init() (r error) {
	if lib.Recover() {
		defer func() {
			if rcv := recover(); rcv != nil {
				m.log.Panic("init meta %s failed - %#v at %s", m.id,
					rcv, lib.PanicOrigin())
				r = gen.TerminateReasonPanic
			}
		}()
	}
	return m.behavior.Init(m)
}
