package tm

import (
	"sync"
	"sync/atomic"
	"time"

	"ergo.services/ergo/gen"
)

const defaultNumShards = 16

type shard struct {
	mutex sync.RWMutex

	linkRelations    map[relationKey]struct{}
	monitorRelations map[relationKey]struct{}
	targetIndex      map[any]*targetEntry

	// Events that hash to this shard
	events         map[gen.Event]*eventEntry
	producerEvents map[gen.PID]map[gen.Event]struct{}
}

// targetManager implements gen.TargetManager interface
type targetManager struct {
	core      gen.CoreTargetManager
	shards    []shard
	numShards uint64

	// Event ordering index: sequential ID -> *eventEntry
	eventSeq   atomic.Uint64
	eventIndex sync.Map // uint64 -> *eventEntry

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
	if rb.len >= rb.size {
		rb.data[rb.head] = msg
		rb.head = (rb.head + 1) % rb.size
		return
	}

	idx := (rb.head + rb.len) % rb.size
	rb.data[idx] = msg
	rb.len++
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
	id        uint64
	createdAt int64
	event     gen.Event

	producer gen.PID
	token    gen.Ref
	notify   bool
	open     bool

	// Ring buffer (nil if unbuffered, protected by bufferMutex)
	bufferMutex sync.Mutex
	buffer      *eventRingBuffer

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
	n := uint64(defaultNumShards)

	tm := &targetManager{
		core:      core,
		shards:    make([]shard, n),
		numShards: n,
	}

	for i := uint64(0); i < n; i++ {
		tm.shards[i] = shard{
			linkRelations:    make(map[relationKey]struct{}),
			monitorRelations: make(map[relationKey]struct{}),
			targetIndex:      make(map[any]*targetEntry),
			events:           make(map[gen.Event]*eventEntry),
			producerEvents:   make(map[gen.PID]map[gen.Event]struct{}),
		}
	}

	return tm
}

func (tm *targetManager) Info() gen.TargetManagerInfo {
	var links, monitors, events int64

	for i := range tm.shards {
		s := &tm.shards[i]
		s.mutex.RLock()
		links += int64(len(s.linkRelations))
		monitors += int64(len(s.monitorRelations))
		events += int64(len(s.events))
		s.mutex.RUnlock()
	}

	return gen.TargetManagerInfo{
		Links:                 links,
		Monitors:              monitors,
		Events:                events,
		ExitSignalsProduced:   tm.exitSignalsProduced.Load(),
		ExitSignalsDelivered:  tm.exitSignalsDelivered.Load(),
		DownMessagesProduced:  tm.downMessagesProduced.Load(),
		DownMessagesDelivered: tm.downMessagesDelivered.Load(),
		EventsPublished:       tm.eventsPublished.Load(),
		EventsReceived:        tm.eventsReceived.Load(),
		EventsLocalSent:       tm.eventsLocalSent.Load(),
		EventsRemoteSent:      tm.eventsRemoteSent.Load(),
	}
}

// shardFor returns the shard responsible for the given target.
// Uses bit masking (numShards must be power of 2).
func (tm *targetManager) shardFor(target any) *shard {
	var idx uint64
	switch t := target.(type) {
	case gen.PID:
		idx = t.ID
	case gen.Alias:
		idx = t.ID[1]
	case gen.ProcessID:
		idx = fnv1aString(string(t.Name))
	case gen.Event:
		idx = fnv1aString(string(t.Name))
	case gen.Atom:
		idx = fnv1aString(string(t))
	}
	return &tm.shards[idx&(tm.numShards-1)]
}

// fnv1aString is an inline FNV-1a hash for strings (no allocation).
func fnv1aString(s string) uint64 {
	h := uint64(14695981039346656037)
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return h
}

// getEventBuffer returns a snapshot of the event buffer, or nil.
func getEventBuffer(entry *eventEntry) []gen.MessageEvent {
	if entry.buffer == nil {
		return nil
	}
	return entry.buffer.snapshot()
}

// generateToken creates a unique event token.
func (tm *targetManager) generateToken() gen.Ref {
	return gen.Ref{
		Node:     tm.core.Name(),
		Creation: tm.core.PID().Creation,
		ID:       [3]uint64{uint64(time.Now().UnixNano()), 0, 0},
	}
}
