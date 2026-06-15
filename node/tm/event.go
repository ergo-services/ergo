package tm

import (
	"sync/atomic"
	"time"

	"ergo.services/ergo/gen"
)

// seq lets snapshot readers detect slots overwritten mid-read.
type bufferedMsg struct {
	seq uint64
	msg gen.MessageEvent
}

// Lock-free fixed-size ring; concurrent publishers claim slots via atomic seq.
type eventRingBuffer struct {
	size  uint64
	seq   atomic.Uint64
	slots []atomic.Pointer[bufferedMsg]
}

func newEventRingBuffer(size int) *eventRingBuffer {
	return &eventRingBuffer{
		size:  uint64(size),
		slots: make([]atomic.Pointer[bufferedMsg], size),
	}
}

func (rb *eventRingBuffer) push(msg gen.MessageEvent) {
	next := rb.seq.Add(1)
	rb.slots[(next-1)%rb.size].Store(&bufferedMsg{seq: next, msg: msg})
}

func (rb *eventRingBuffer) snapshot() []gen.MessageEvent {
	end := rb.seq.Load()
	if end == 0 {
		return make([]gen.MessageEvent, 0)
	}
	var start uint64
	if end > rb.size {
		start = end - rb.size
	}
	out := make([]gen.MessageEvent, 0, end-start)
	for i := start; i < end; i++ {
		p := rb.slots[i%rb.size].Load()
		if p == nil || p.seq != i+1 {
			continue
		}
		out = append(out, p.msg)
	}
	return out
}

func (rb *eventRingBuffer) length() int {
	end := rb.seq.Load()
	if end >= rb.size {
		return int(rb.size)
	}
	return int(end)
}

// Subscribers live in Storage (target = event). Fields above buffer are
// immutable after RegisterEvent; buffer is lock-free, so no mutex needed.
type eventEntry struct {
	id        uint64
	createdAt int64
	event     gen.Event
	producer  gen.PID
	token     gen.Ref
	notify    bool
	open      bool

	buffer *eventRingBuffer

	subscriberCount atomic.Int64

	messagesPublished  atomic.Int64
	messagesLocalSent  atomic.Int64
	messagesRemoteSent atomic.Int64
	lastPublishedAt    atomic.Int64
}

func (e *eventEntry) snapshotBuffer() []gen.MessageEvent {
	if e.buffer == nil {
		return nil
	}
	return e.buffer.snapshot()
}

func (m *Manager) buildEventInfo(e *eventEntry) gen.EventInfo {
	var bufSize, bufLen int
	if e.buffer != nil {
		bufSize = int(e.buffer.size)
		bufLen = e.buffer.length()
	}
	return gen.EventInfo{
		CreatedAt:          e.createdAt,
		Event:              e.event,
		Producer:           e.producer,
		BufferSize:         bufSize,
		CurrentBuffer:      bufLen,
		Notify:             e.notify,
		Open:               e.open,
		Subscribers:        e.subscriberCount.Load(),
		MessagesPublished:  e.messagesPublished.Load(),
		MessagesLocalSent:  e.messagesLocalSent.Load(),
		MessagesRemoteSent: e.messagesRemoteSent.Load(),
		LastPublishedAt:    e.lastPublishedAt.Load(),
	}
}

func (m *Manager) RegisterEvent(producer gen.PID, name gen.Atom, options gen.EventOptions) (gen.Ref, error) {
	event := gen.Event{Node: m.core.Name(), Name: name}

	notify := options.Notify
	if producer == m.core.PID() {
		// Node-level event: no producer process to notify.
		notify = false
	}

	id := m.eventSeq.Add(1)
	entry := &eventEntry{
		id:        id,
		createdAt: time.Now().UnixNano(),
		event:     event,
		producer:  producer,
		token:     m.core.MakeRef(),
		notify:    notify,
		open:      options.Open,
	}
	if options.Buffer > 0 {
		entry.buffer = newEventRingBuffer(options.Buffer)
	}

	if _, loaded := m.events.LoadOrStore(event, entry); loaded {
		return gen.Ref{}, gen.ErrTaken
	}
	m.eventsByID.Store(id, entry)
	m.eventsCount.Add(1)
	return entry.token, nil
}

func (m *Manager) UnregisterEvent(producer gen.PID, name gen.Atom) error {
	event := gen.Event{Node: m.core.Name(), Name: name}
	v, ok := m.events.Load(event)
	if ok == false {
		return gen.ErrEventUnknown
	}
	entry := v.(*eventEntry)
	if entry.producer != producer {
		return gen.ErrEventOwner
	}
	if m.events.CompareAndDelete(event, entry) == false {
		return gen.ErrEventUnknown
	}
	m.eventsByID.Delete(entry.id)
	m.eventsCount.Add(-1)

	relations := m.storage.RemoveTarget(event)
	m.dispatchTargetTerminate(event, relations, gen.ErrUnregistered)
	return nil
}

// LinkEvent boundary against PublishEvent is best-effort: a boundary
// message may arrive once (via buffer or live walk) or duplicated via both.
// Serialization would need a per-event lock on the publish hot-path.
func (m *Manager) LinkEvent(consumer gen.PID, event gen.Event) ([]gen.MessageEvent, error) {
	if event.Node == m.core.Name() {
		return m.linkEventLocal(consumer, event, KindLink)
	}
	return m.linkEventRemote(consumer, event, KindLink)
}

func (m *Manager) MonitorEvent(consumer gen.PID, event gen.Event) ([]gen.MessageEvent, error) {
	if event.Node == m.core.Name() {
		return m.linkEventLocal(consumer, event, KindMonitor)
	}
	return m.linkEventRemote(consumer, event, KindMonitor)
}

func (m *Manager) linkEventLocal(consumer gen.PID, event gen.Event, kind Kind) ([]gen.MessageEvent, error) {
	v, ok := m.events.Load(event)
	if ok == false {
		return nil, gen.ErrEventUnknown
	}
	entry := v.(*eventEntry)

	if m.storage.Register(event, consumer, kind) == false {
		if consumer.Node != m.core.Name() {
			// Idempotent for retrying remote peers.
			return entry.snapshotBuffer(), nil
		}
		return nil, gen.ErrTargetExist
	}

	// UnregisterEvent may have raced between events.Load and the Register
	// above; the Register would then leak a fresh target entry. Roll back.
	if _, stillThere := m.events.Load(event); stillThere == false {
		m.storage.Unregister(event, consumer, kind)
		m.storage.RemoveTarget(event)
		return nil, gen.ErrEventUnknown
	}

	if entry.subscriberCount.Add(1) == 1 && entry.notify {
		if _, stillThere := m.events.Load(event); stillThere {
			m.core.RouteSendPID(
				m.core.PID(),
				entry.producer,
				gen.MessageOptions{Priority: gen.MessagePriorityHigh},
				gen.MessageEventStart{Name: event.Name},
			)
		}
	}
	return entry.snapshotBuffer(), nil
}

func (m *Manager) linkEventRemote(consumer gen.PID, event gen.Event, kind Kind) ([]gen.MessageEvent, error) {
	if m.storage.Register(event, consumer, kind) == false {
		if consumer.Node != m.core.Name() {
			// Idempotent for retrying remote peers.
			return nil, nil
		}
		return nil, gen.ErrTargetExist
	}
	wp := m.wireFor(event, kind)
	if consumer.Node == m.core.Name() {
		wp.localCount.Add(1)
	}
	buffer, err := m.ensureRemoteLinkEvent(wp, event, kind)
	if err != nil {
		if consumer.Node == m.core.Name() {
			wp.localCount.Add(-1)
		}
		m.storage.Unregister(event, consumer, kind)
		return nil, err
	}
	return buffer, nil
}

func (m *Manager) UnlinkEvent(consumer gen.PID, event gen.Event) error {
	if event.Node == m.core.Name() {
		return m.unlinkEventLocal(consumer, event, KindLink)
	}
	return m.unlinkEventRemote(consumer, event, KindLink)
}

func (m *Manager) DemonitorEvent(consumer gen.PID, event gen.Event) error {
	if event.Node == m.core.Name() {
		return m.unlinkEventLocal(consumer, event, KindMonitor)
	}
	return m.unlinkEventRemote(consumer, event, KindMonitor)
}

func (m *Manager) unlinkEventLocal(consumer gen.PID, event gen.Event, kind Kind) error {
	v, ok := m.events.Load(event)
	if ok == false {
		return gen.ErrEventUnknown
	}
	entry := v.(*eventEntry)

	if m.storage.Unregister(event, consumer, kind) == false {
		return gen.ErrTargetUnknown
	}

	if _, stillThere := m.events.Load(event); stillThere == false {
		return nil
	}

	if entry.subscriberCount.Add(-1) == 0 && entry.notify {
		m.core.RouteSendPID(
			m.core.PID(),
			entry.producer,
			gen.MessageOptions{Priority: gen.MessagePriorityHigh},
			gen.MessageEventStop{Name: event.Name},
		)
	}
	return nil
}

func (m *Manager) unlinkEventRemote(consumer gen.PID, event gen.Event, kind Kind) error {
	if m.storage.Unregister(event, consumer, kind) == false {
		return gen.ErrTargetUnknown
	}
	wp := m.wireForExisting(event, kind)
	if wp != nil && consumer.Node == m.core.Name() {
		wp.localCount.Add(-1)
	}
	m.ensureRemoteUnlink(wp, event.Node, func(conn gen.Connection) {
		if kind == KindLink {
			conn.UnlinkEvent(m.core.PID(), event)
			return
		}
		conn.DemonitorEvent(m.core.PID(), event)
	})
	return nil
}

func (m *Manager) PublishEvent(from gen.PID, token gen.Ref, options gen.MessageOptions, message gen.MessageEvent) error {
	if from.Node == m.core.Name() {
		return m.publishLocalProducer(from, token, options, message)
	}
	return m.publishRemoteProducer(from, options, message)
}

func (m *Manager) publishLocalProducer(from gen.PID, token gen.Ref, options gen.MessageOptions, message gen.MessageEvent) error {
	v, ok := m.events.Load(message.Event)
	if ok == false {
		return gen.ErrEventUnknown
	}
	entry := v.(*eventEntry)

	if entry.open == false && entry.token != token {
		return gen.ErrEventOwner
	}

	if entry.buffer != nil {
		entry.buffer.push(message)
	}

	entry.messagesPublished.Add(1)
	entry.lastPublishedAt.Store(time.Now().UnixNano())

	var localConsumers []gen.PID
	remoteNodes := make(map[gen.Atom]struct{})
	m.storage.Walk(message.Event, func(p gen.PID, _ Kind) {
		if p.Node == m.core.Name() {
			localConsumers = append(localConsumers, p)
			return
		}
		remoteNodes[p.Node] = struct{}{}
	})

	m.eventsPublished.Add(1)
	if len(localConsumers) > 0 {
		m.core.RouteSendEventMessages(from, localConsumers, options, message)
		n := int64(len(localConsumers))
		entry.messagesLocalSent.Add(n)
		m.eventsLocalSent.Add(n)
	}
	for node := range remoteNodes {
		conn, err := m.core.GetConnection(node)
		if err != nil {
			continue
		}
		if err := conn.SendEvent(from, options, message); err != nil {
			continue
		}
		entry.messagesRemoteSent.Add(1)
		m.eventsRemoteSent.Add(1)
	}
	return nil
}

func (m *Manager) publishRemoteProducer(from gen.PID, options gen.MessageOptions, message gen.MessageEvent) error {
	var localConsumers []gen.PID
	m.storage.Walk(message.Event, func(p gen.PID, _ Kind) {
		if p.Node != m.core.Name() {
			return
		}
		localConsumers = append(localConsumers, p)
	})
	m.eventsReceived.Add(1)
	if len(localConsumers) > 0 {
		m.core.RouteSendEventMessages(from, localConsumers, options, message)
		m.eventsLocalSent.Add(int64(len(localConsumers)))
	}
	return nil
}

func (m *Manager) EventInfo(event gen.Event) (gen.EventInfo, error) {
	v, ok := m.events.Load(event)
	if ok == false {
		return gen.EventInfo{}, gen.ErrEventUnknown
	}
	return m.buildEventInfo(v.(*eventEntry)), nil
}

func (m *Manager) EventRangeInfo(fn func(gen.EventInfo) bool) error {
	m.events.Range(func(_, v any) bool {
		return fn(m.buildEventInfo(v.(*eventEntry)))
	})
	return nil
}

func (m *Manager) EventListInfo(timestamp int64, limit int, filter ...func(gen.EventInfo) bool) ([]gen.EventInfo, error) {
	maxID := m.eventSeq.Load()
	if maxID == 0 {
		return nil, nil
	}

	absLimit := limit
	if absLimit < 0 {
		absLimit = -absLimit
	}
	if absLimit == 0 {
		return nil, nil
	}

	var fn func(gen.EventInfo) bool
	if len(filter) > 0 {
		fn = filter[0]
	}

	result := make([]gen.EventInfo, 0, absLimit)
	accept := func(e *eventEntry) bool {
		if timestamp > 0 {
			if limit >= 0 && e.createdAt < timestamp {
				return false
			}
			if limit < 0 && e.createdAt > timestamp {
				return false
			}
		}
		if fn != nil {
			return fn(m.buildEventInfo(e))
		}
		return true
	}

	if timestamp == -1 || limit < 0 {
		for id := maxID; id >= 1 && len(result) < absLimit; id-- {
			v, ok := m.eventsByID.Load(id)
			if ok == false {
				continue
			}
			e := v.(*eventEntry)
			if accept(e) {
				result = append(result, m.buildEventInfo(e))
			}
		}
		return result, nil
	}

	for id := uint64(1); id <= maxID && len(result) < absLimit; id++ {
		v, ok := m.eventsByID.Load(id)
		if ok == false {
			continue
		}
		e := v.(*eventEntry)
		if accept(e) {
			result = append(result, m.buildEventInfo(e))
		}
	}
	return result, nil
}
