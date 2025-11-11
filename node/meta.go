package node

import (
	"runtime"
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

	priority    gen.MessagePriority
	compression bool

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
	return m.priority
}

func (m *meta) SetSendPriority(priority gen.MessagePriority) error {
	m.priority = priority
	return nil
}

func (m *meta) Send(to any, message any) error {
	if err := m.send(to, message); err != nil {
		return err
	}
	return nil
}

func (m *meta) SendWithPriority(to any, message any, priority gen.MessagePriority) error {
	var prev gen.MessagePriority
	prev, m.priority = m.priority, priority
	err := m.send(to, message)
	m.priority = prev
	return err
}

func (m *meta) SendResponse(to gen.PID, ref gen.Ref, message any) error {
	state := atomic.LoadInt32(&m.state)
	if gen.MetaState(state) != gen.MetaStateRunning {
		return gen.ErrNotAllowed
	}

	compression := m.p.compression
	compression.Enable = m.compression

	options := gen.MessageOptions{
		Priority:         m.priority,
		Compression:      compression,
		KeepNetworkOrder: m.p.keeporder,
	}
	if err := m.p.node.RouteSendResponse(m.p.pid, to, options, message); err != nil {
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

	compression := m.p.compression
	compression.Enable = m.compression

	options := gen.MessageOptions{
		Ref:              ref,
		Priority:         m.priority,
		Compression:      compression,
		KeepNetworkOrder: m.p.keeporder,
	}
	if rerr := m.p.node.RouteSendResponse(m.p.pid, to, options, err); rerr != nil {
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
	return m.compression
}

func (m *meta) SetCompression(enabled bool) error {
	state := atomic.LoadInt32(&m.state)
	if gen.MetaState(state) != gen.MetaStateRunning {
		return gen.ErrNotAllowed
	}
	m.compression = enabled
	return nil
}

func (m *meta) send(to any, message any) error {
	compression := m.p.compression
	compression.Enable = m.compression

	options := gen.MessageOptions{
		Priority:         m.priority,
		Compression:      compression,
		KeepNetworkOrder: m.p.keeporder,
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
			switch m.priority {
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

		if err := m.p.node.RouteSendPID(m.p.pid, t, options, message); err != nil {
			return err
		}
	case gen.Atom:
		if err := m.p.node.RouteSendProcessID(m.p.pid, gen.ProcessID{Name: t}, options, message); err != nil {
			return err
		}
	case gen.ProcessID:
		if err := m.p.node.RouteSendProcessID(m.p.pid, t, options, message); err != nil {
			return err
		}
	case gen.Alias:
		if err := m.p.node.RouteSendAlias(m.p.pid, t, options, message); err != nil {
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
				pc, fn, line, _ := runtime.Caller(2)
				m.log.Panic("init meta %s failed - %#v at %s[%s:%d]", m.id,
					rcv, runtime.FuncForPC(pc).Name(), fn, line)
				r = gen.TerminateReasonPanic
			}
		}()
	}
	return m.behavior.Init(m)
}

func (m *meta) start() {
	defer m.p.metas.Delete(m.id)

	if lib.Recover() {
		defer func() {
			if rcv := recover(); rcv != nil {
				pc, fn, line, _ := runtime.Caller(2)
				m.log.Panic("meta process %s terminated - %#v at %s[%s:%d]", m.id,
					rcv, runtime.FuncForPC(pc).Name(), fn, line)
				old := atomic.SwapInt32(&m.state, int32(gen.MetaStateTerminated))
				if old != int32(gen.MetaStateTerminated) {
					m.p.node.aliases.Delete(m.id)
					atomic.StoreInt32(&m.state, int32(gen.MetaStateTerminated))
					reason := gen.TerminateReasonPanic
					m.p.node.RouteTerminateAlias(m.id, reason)
					m.behavior.Terminate(reason)
				}
			}
		}()
	}

	// start meta process
	m.creation = time.Now().Unix()

	atomic.StoreInt32(&m.state, int32(gen.MetaStateSleep))

	// handle mailbox
	go m.handle()

	reason := m.behavior.Start()
	// meta process terminated
	old := atomic.SwapInt32(&m.state, int32(gen.MetaStateTerminated))
	if old != int32(gen.MetaStateTerminated) {
		m.p.node.aliases.Delete(m.id)
		if reason == nil {
			reason = gen.TerminateReasonNormal
		}
		m.p.node.RouteTerminateAlias(m.id, reason)
		m.behavior.Terminate(reason)
	}
}

func (m *meta) handle() {
	var reason error
	var result any

	if atomic.CompareAndSwapInt32(&m.state, int32(gen.MetaStateSleep), int32(gen.MetaStateRunning)) == false {
		// running or terminated
		return
	}

	go func() {
		var message *gen.MailboxMessage

		if lib.Recover() {
			defer func() {
				if rcv := recover(); rcv != nil {
					pc, fn, line, _ := runtime.Caller(2)
					m.log.Panic("meta process %s terminated - %#v at %s[%s:%d]", m.id,
						rcv, runtime.FuncForPC(pc).Name(), fn, line)

					old := atomic.SwapInt32(&m.state, int32(gen.MetaStateTerminated))
					if old != int32(gen.MetaStateTerminated) {
						m.p.node.aliases.Delete(m.id)
						reason = gen.TerminateReasonPanic
						m.p.node.RouteTerminateAlias(m.id, reason)
						m.behavior.Terminate(reason)
					}
				}
			}()
		}

	next:
		for {
			reason = nil
			result = nil

			if gen.MetaState(atomic.LoadInt32(&m.state)) != gen.MetaStateRunning {
				// terminated
				break
			}
			msg, ok := m.system.Pop()
			if ok == false {
				msg, ok = m.main.Pop()
				if ok == false {
					// no messages
					break
				}
			}

			if message != nil {
				// release previously handled mailbox message
				gen.ReleaseMailboxMessage(message)
				message = nil
			}

			if message, ok = msg.(*gen.MailboxMessage); ok == false {
				m.log.Error("got unknown mailbox message. ignored")
				continue
			}

			switch message.Type {
			case gen.MailboxMessageTypeRegular:
				reason = m.behavior.HandleMessage(message.From, message.Message)
				if reason == nil {
					continue
				}

			case gen.MailboxMessageTypeRequest:
				result, reason = m.behavior.HandleCall(message.From, message.Ref, message.Message)
				options := gen.MessageOptions{
					Ref:              message.Ref,
					Priority:         m.p.priority,
					Compression:      m.p.compression,
					KeepNetworkOrder: m.p.keeporder,
				}
				if reason == nil {
					if result != nil {
						m.p.node.RouteSendResponse(m.p.pid, message.From, options, result)
					}
					continue
				}
				if reason == gen.TerminateReasonNormal && result != nil {
					m.p.node.RouteSendResponse(m.p.pid, message.From, options, result)
				}
			case gen.MailboxMessageTypeInspect:
				result := m.behavior.HandleInspect(message.From, message.Message.([]string)...)
				options := gen.MessageOptions{
					Ref:              message.Ref,
					Priority:         m.p.priority,
					Compression:      m.p.compression,
					KeepNetworkOrder: m.p.keeporder,
				}
				m.p.node.RouteSendResponse(m.p.pid, message.From, options, result)
				atomic.AddUint64(&m.messagesOut, 1)
				continue

			case gen.MailboxMessageTypeExit:
				if err, ok := message.Message.(error); ok {
					reason = err
					break
				}
				m.p.log.Error("got incorrect exit-message from %s. ignored", message.From)
				continue
			default:

				m.p.log.Error("got unknown mailbox message type %#v. ignored", message.Type)
				continue
			}

			// terminated
			old := atomic.SwapInt32(&m.state, int32(gen.MetaStateTerminated))
			if old != int32(gen.MetaStateTerminated) {
				m.p.node.aliases.Delete(m.id)
				m.p.node.RouteTerminateAlias(m.id, reason)
				m.behavior.Terminate(reason)
			}
			return
		}

		if atomic.CompareAndSwapInt32(&m.state, int32(gen.MetaStateRunning), int32(gen.MetaStateSleep)) == false {
			// terminated. seems the main loop is stopped. do nothing.
			return
		}

		// check if we got a new message
		if m.system.Item() == nil {
			if m.main.Item() == nil {
				// no messages
				return
			}
		}

		// got some... try to use this goroutine
		if atomic.CompareAndSwapInt32(&m.state, int32(gen.MetaStateSleep), int32(gen.MetaStateRunning)) == false {
			// another goroutine is already running
			return
		}
		goto next
	}()
}
