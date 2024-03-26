package node

import (
	"runtime"
	"sync/atomic"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

type meta struct {
	id        gen.Alias
	behavior  gen.MetaBehavior
	sbehavior string

	main        lib.QueueMPSC
	system      lib.QueueMPSC
	messagesIn  uint64
	messagesOut uint64

	p        *process
	priority gen.MessagePriority
	log      *log
	state    int32

	// used for the meta process Uptime method only
	creation int64
}

func (m *meta) ID() gen.Alias {
	return m.id
}

func (m *meta) Parent() gen.PID {
	return m.p.pid
}

func (m *meta) Send(to any, message any) error {
	atomic.AddUint64(&m.messagesOut, 1)
	return m.p.Send(to, message)
}

func (m *meta) Spawn(behavior gen.MetaBehavior, options gen.MetaOptions) (gen.Alias, error) {
	return m.p.SpawnMeta(behavior, options)
}

func (m *meta) Env(name gen.Env) (any, bool) {
	return m.p.Env(name)
}

func (m *meta) EnvList() map[gen.Env]any {
	return m.p.EnvList()
}

func (m *meta) Log() gen.Log {
	return m.log
}

func (m *meta) init() (r error) {
	if lib.Recover() {
		defer func() {
			if rcv := recover(); rcv != nil {
				pc, fn, line, _ := runtime.Caller(2)
				m.log.Panic("init meta %s failed - %#v at %s[%s:%d]", m.id,
					rcv, runtime.FuncForPC(pc).Name(), fn, line)
				r = gen.TerminateMetaPanic
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
					reason := gen.TerminateMetaPanic
					m.p.node.RouteTerminateAlias(m.id, reason)
					m.behavior.Terminate(reason)
				}
			}
		}()
	}

	// start meta process
	m.creation = time.Now().Unix()
	reason := m.behavior.Start()
	// meta process terminated
	old := atomic.SwapInt32(&m.state, int32(gen.MetaStateTerminated))
	if old != int32(gen.MetaStateTerminated) {
		m.p.node.aliases.Delete(m.id)
		if reason == nil {
			reason = gen.TerminateMetaNormal
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
						reason = gen.TerminateMetaPanic
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
					Priority:         m.p.priority,
					Compression:      m.p.compression,
					KeepNetworkOrder: m.p.keeporder,
				}
				if reason == nil {
					if result != nil {
						m.p.node.RouteSendResponse(m.p.pid, message.From, message.Ref, options, result)
					}
					continue
				}
				if reason == gen.TerminateMetaNormal && result != nil {
					m.p.node.RouteSendResponse(m.p.pid, message.From, message.Ref, options, result)
				}
			case gen.MailboxMessageTypeInspect:
				result := m.behavior.HandleInspect(message.From, message.Message.([]string)...)
				options := gen.MessageOptions{
					Priority:         m.p.priority,
					Compression:      m.p.compression,
					KeepNetworkOrder: m.p.keeporder,
				}
				m.p.node.RouteSendResponse(m.p.pid, message.From, message.Ref, options, result)
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
