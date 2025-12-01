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
	eventsSent            atomic.Int64
}

type relationKey struct {
	consumer gen.PID
	target   any
}

type targetEntry struct {
	allowAlwaysFirst bool
	consumers        map[gen.PID]struct{}
}

type eventEntry struct {
	producer gen.PID
	token    gen.Ref
	notify   bool

	// Buffer (simple slice - protected by mutex)
	buffer     []gen.MessageEvent
	bufferSize int

	// Subscribers (links and monitors separately)
	// Slice for fast iteration, map for O(1) lookup/delete
	linkSubscribers      []gen.PID
	linkSubscribersIndex map[gen.PID]int

	monitorSubscribers      []gen.PID
	monitorSubscribersIndex map[gen.PID]int

	subscriberCount int64
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
		EventsSent:            tm.eventsSent.Load(),
	}
}
