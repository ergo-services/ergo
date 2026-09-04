//go:build pprof

package node

import (
	"context"
	"runtime/pprof"
	"sync/atomic"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

func (m *meta) start() {
	labels := pprof.Labels("meta", m.id.String(), "role", "reader")
	pprof.Do(context.Background(), labels, func(context.Context) {
		defer m.p.metas.Delete(m.id)

		if lib.Recover() {
			defer func() {
				if rcv := recover(); rcv != nil {
					m.log.Panic("meta process %s terminated - %#v at %s", m.id,
						rcv, lib.PanicOrigin())
					old := atomic.SwapInt32(&m.state, int32(gen.MetaStateTerminated))
					if old != int32(gen.MetaStateTerminated) {
						m.p.node.aliases.Delete(m.id)
						atomic.StoreInt32(&m.state, int32(gen.MetaStateTerminated))
						reason := gen.TerminateReasonPanic
						m.p.core.RouteTerminateAlias(m.id, reason)
						m.behavior.Terminate(reason)
					}
				}
			}()
		}

		// start meta process
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
			m.p.core.RouteTerminateAlias(m.id, reason)
			m.behavior.Terminate(reason)
		}
	})
}

func (m *meta) handle() {
	var reason error
	var result any

	if atomic.CompareAndSwapInt32(&m.state, int32(gen.MetaStateSleep), int32(gen.MetaStateRunning)) == false {
		// running or terminated
		return
	}

	go func() {
		labels := pprof.Labels("meta", m.id.String(), "role", "handler")
		pprof.Do(context.Background(), labels, func(context.Context) {
			var message *gen.MailboxMessage

			if lib.Recover() {
				defer func() {
					if rcv := recover(); rcv != nil {
						m.log.Panic("meta process %s terminated - %#v at %s", m.id,
							rcv, lib.PanicOrigin())

						old := atomic.SwapInt32(&m.state, int32(gen.MetaStateTerminated))
						if old != int32(gen.MetaStateTerminated) {
							m.p.node.aliases.Delete(m.id)
							reason = gen.TerminateReasonPanic
							m.p.core.RouteTerminateAlias(m.id, reason)
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
						Priority:         gen.MessagePriority(m.p.priority.Load()),
						Compression:      *m.p.compression.Load(),
						KeepNetworkOrder: m.p.keeporder.Load(),
					}
					if reason == nil {
						if result != nil {
							m.p.core.RouteSendResponse(m.p.pid, message.From, options, result)
						}
						continue
					}
					if reason == gen.TerminateReasonNormal && result != nil {
						m.p.core.RouteSendResponse(m.p.pid, message.From, options, result)
					}
				case gen.MailboxMessageTypeInspect:
					result := m.behavior.HandleInspect(message.From, message.Message.([]string)...)
					options := gen.MessageOptions{
						Ref:              message.Ref,
						Priority:         gen.MessagePriority(m.p.priority.Load()),
						Compression:      *m.p.compression.Load(),
						KeepNetworkOrder: m.p.keeporder.Load(),
					}
					m.p.core.RouteSendResponse(m.p.pid, message.From, options, result)
					atomic.AddUint64(&m.messagesOut, 1)
					atomic.AddUint64(&m.p.messagesOut, 1)
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
					m.p.core.RouteTerminateAlias(m.id, reason)
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
		})
	}()
}
