package tm

import (
	"sync"
	"sync/atomic"

	"ergo.services/ergo/gen"
)

// targetManager implements gen.TargetManager interface
type targetManager struct {
	mutex sync.RWMutex

	core gen.CoreTargetManager

	// Link/Monitor relationships
	linkRelations    map[relationKey]struct{}
	monitorRelations map[relationKey]struct{}
	targetIndex      map[any]*targetEntry

	// Event storage
	events         map[gen.Event]*eventEntry
	producerEvents map[gen.PID]map[gen.Event]struct{} // producer -> events index

	// Statistics
	exitSignalsProduced   atomic.Int64
	exitSignalsDelivered  atomic.Int64
	downMessagesProduced  atomic.Int64
	downMessagesDelivered atomic.Int64
	eventsPublished       atomic.Int64
	eventsReceived        atomic.Int64
	eventsLocalSent       atomic.Int64
	eventsRemoteSent      atomic.Int64
}

type relationKey struct {
	consumer gen.PID
	target   any
}

type targetEntry struct {
	allowAlwaysFirst bool
	consumers        map[gen.PID]struct{}
}

// eventRingBuffer is a fixed-size circular buffer for event messages.
// O(1) push, O(n) snapshot. No copy-shift on overflow.
type eventRingBuffer struct {
	data []gen.MessageEvent
	size int // capacity
	head int // index of oldest element
	len  int // current number of elements
}

func (rb *eventRingBuffer) push(msg gen.MessageEvent) {
	idx := (rb.head + rb.len) % rb.size
	if rb.len < rb.size {
		rb.data[idx] = msg
		rb.len++
	} else {
		// overwrite oldest
		rb.data[rb.head] = msg
		rb.head = (rb.head + 1) % rb.size
	}
}

func (rb *eventRingBuffer) snapshot() []gen.MessageEvent {
	if rb.len == 0 {
		return make([]gen.MessageEvent, 0)
	}
	result := make([]gen.MessageEvent, rb.len)
	for i := 0; i < rb.len; i++ {
		result[i] = rb.data[(rb.head+i)%rb.size]
	}
	return result
}

type eventEntry struct {
	producer gen.PID
	token    gen.Ref
	notify   bool

	// Ring buffer (nil if unbuffered, protected by mutex)
	buffer *eventRingBuffer

	// Subscribers (links and monitors separately)
	// Slice for fast iteration, map for O(1) lookup/delete
	linkSubscribers      []gen.PID
	linkSubscribersIndex map[gen.PID]int

	monitorSubscribers      []gen.PID
	monitorSubscribersIndex map[gen.PID]int

	subscriberCount int64

	// Per-event statistics
	messagesPublished  atomic.Int64
	messagesLocalSent  atomic.Int64
	messagesRemoteSent atomic.Int64
}

type Options struct{}

func Create(core gen.CoreTargetManager, options Options) gen.TargetManager {
	tm := &targetManager{
		core:             core,
		linkRelations:    make(map[relationKey]struct{}),
		monitorRelations: make(map[relationKey]struct{}),
		targetIndex:      make(map[any]*targetEntry),
		events:           make(map[gen.Event]*eventEntry),
		producerEvents:   make(map[gen.PID]map[gen.Event]struct{}),
	}

	return tm
}

func (tm *targetManager) Info() gen.TargetManagerInfo {
	tm.mutex.RLock()
	defer tm.mutex.RUnlock()

	return gen.TargetManagerInfo{
		Links:                 int64(len(tm.linkRelations)),
		Monitors:              int64(len(tm.monitorRelations)),
		Events:                int64(len(tm.events)),
		ExitSignalsProduced:   tm.exitSignalsProduced.Load(),
		ExitSignalsDelivered:  tm.exitSignalsDelivered.Load(),
		DownMessagesProduced:  tm.downMessagesProduced.Load(),
		DownMessagesDelivered: tm.downMessagesDelivered.Load(),
		EventsPublished:       tm.eventsPublished.Load(),
		EventsReceived:       tm.eventsReceived.Load(),
		EventsLocalSent:      tm.eventsLocalSent.Load(),
		EventsRemoteSent:     tm.eventsRemoteSent.Load(),
	}
}
