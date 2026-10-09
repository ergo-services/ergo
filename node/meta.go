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

	requests atomic.Pointer[requests]

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

func (m *meta) SendRequest(to any, request any) (gen.Ref, error) {
	return m.sendRequest(to, request, gen.RequestOptions{
		Priority: gen.MessagePriority(m.priority.Load()),
	})
}

func (m *meta) SendRequestImportant(to any, request any) (gen.Ref, error) {
	return m.sendRequest(to, request, gen.RequestOptions{
		Priority:  gen.MessagePriority(m.priority.Load()),
		Important: true,
	})
}

func (m *meta) SendRequestWithTimeout(to any, request any, timeout int) (gen.Ref, error) {
	return m.sendRequest(to, request, gen.RequestOptions{
		Timeout:  timeout,
		Priority: gen.MessagePriority(m.priority.Load()),
	})
}

func (m *meta) SendRequestWithLabel(to any, request any, label any) (gen.Ref, error) {
	return m.sendRequest(to, request, gen.RequestOptions{
		Label:    label,
		Priority: gen.MessagePriority(m.priority.Load()),
	})
}

func (m *meta) SendRequestWithOptions(to any, request any, options gen.RequestOptions) (gen.Ref, error) {
	if options.Priority == 0 {
		options.Priority = gen.MessagePriority(m.priority.Load())
	}
	return m.sendRequest(to, request, options)
}

func (m *meta) sendRequest(to any, request any, ro gen.RequestOptions) (gen.Ref, error) {
	if gen.MetaState(atomic.LoadInt32(&m.state)) == gen.MetaStateTerminated {
		return gen.Ref{}, gen.ErrNotAllowed
	}

	switch t := to.(type) {
	case gen.PID:
		if t == m.p.pid {
			return gen.Ref{}, gen.ErrNotAllowed
		}
	case gen.Alias:
		if t == m.id {
			return gen.Ref{}, gen.ErrNotAllowed
		}
	case gen.ProcessID:
		if t.Node == "" {
			t.Node = m.p.node.name
			to = t
		}
	case gen.Atom:
		to = gen.ProcessID{Name: t, Node: m.p.node.name}
	default:
		return gen.Ref{}, gen.ErrUnsupported
	}

	if ro.Timeout < 1 {
		ro.Timeout = gen.DefaultRequestTimeout
	}

	ref, err := m.p.node.MakeRefWithDeadline(time.Now().Unix() + int64(ro.Timeout))
	if err != nil {
		return gen.Ref{}, err
	}
	ref.ID[1] = m.id.ID[0]

	compression := *m.p.compression.Load()
	compression.Enable = m.compression.Load()

	options := gen.MessageOptions{
		Ref:               ref,
		Priority:          ro.Priority,
		Compression:       compression,
		KeepNetworkOrder:  m.p.keeporder.Load(),
		ImportantDelivery: ro.Important,
	}

	pending := &pendingRequest{
		label:    ro.Label,
		priority: ro.Priority,
	}
	r := requestsOf(&m.requests)
	r.mu.Lock()
	pending.timer = time.AfterFunc(time.Duration(ro.Timeout)*time.Second, func() {
		m.expireRequest(ref)
	})
	r.pending[ref] = pending
	r.mu.Unlock()

	if err := m.routeRequest(to, options, request); err != nil {
		if pending := r.take(ref); pending != nil {
			pending.timer.Stop()
		}
		return gen.Ref{}, err
	}

	atomic.AddUint64(&m.messagesOut, 1)
	return ref, nil
}

func (m *meta) routeRequest(to any, options gen.MessageOptions, request any) error {
	switch t := to.(type) {
	case gen.PID:
		return m.p.core.RouteCallPID(m.p.pid, t, options, request)
	case gen.ProcessID:
		return m.p.core.RouteCallProcessID(m.p.pid, t, options, request)
	case gen.Alias:
		return m.p.core.RouteCallAlias(m.p.pid, t, options, request)
	}
	return gen.ErrUnsupported
}

func (m *meta) CancelRequest(ref gen.Ref) error {
	pending := m.takeRequest(ref)
	if pending == nil {
		return gen.ErrUnknown
	}
	pending.timer.Stop()
	return nil
}

func (m *meta) takeRequest(ref gen.Ref) *pendingRequest {
	r := m.requests.Load()
	if r == nil {
		return nil
	}
	return r.take(ref)
}

func (m *meta) expireRequest(ref gen.Ref) {
	if m.alive() == false {
		return
	}
	pending := m.takeRequest(ref)
	if pending == nil {
		return
	}
	m.pushResponse(gen.PID{}, ref, pending, nil, gen.ErrTimeout)
}

func (m *meta) deliverResponse(from gen.PID, options gen.MessageOptions, result any, rerr error) error {
	pending := m.takeRequest(options.Ref)
	if pending == nil {
		return gen.ErrResponseIgnored
	}
	pending.timer.Stop()

	if m.pushResponse(from, options.Ref, pending, result, rerr) == false {
		return gen.ErrMetaMailboxFull
	}
	if options.ImportantDelivery {
		m.p.core.RouteSendAck(m.p.pid, from, gen.MessageOptions{Ref: options.Ref}, nil)
	}
	return nil
}

func (m *meta) pushResponse(from gen.PID, ref gen.Ref, pending *pendingRequest, result any, rerr error) bool {
	qm := gen.TakeMailboxMessage()
	qm.From = from
	qm.Ref = ref
	qm.Type = gen.MailboxMessageTypeResponse
	qm.Message = gen.MessageResponse{
		From:   from,
		Ref:    ref,
		Label:  pending.label,
		Result: result,
		Error:  rerr,
	}

	queue := m.main
	switch pending.priority {
	case gen.MessagePriorityHigh, gen.MessagePriorityMax:
		queue = m.system
	}
	if queue.Push(qm) == false {
		gen.ReleaseMailboxMessage(qm)
		return false
	}

	atomic.AddUint64(&m.messagesIn, 1)
	m.handle()
	return true
}

func (m *meta) cancelRequests() {
	r := m.requests.Load()
	if r == nil {
		return
	}
	r.cancel()
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
	// next tick deadline; touched by the timer callback only, which never runs twice at once
	var next time.Time
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
		if stopped.Load() {
			return
		}
		// fixed rate: re-arm from the deadline, not from now, so lateness doesn't accumulate
		now := time.Now()
		next = next.Add(d)
		if next.After(now) == false {
			// more than a period late: drop the missed ticks, like time.Ticker
			next = next.Add((now.Sub(next)/d + 1) * d)
		}
		t.Reset(next.Sub(now))
	})
	next = time.Now().Add(d)
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
