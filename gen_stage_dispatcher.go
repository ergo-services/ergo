package ergo

import (
	"fmt"
	"math/rand"

	"github.com/halturin/ergo/etf"
)

// GenStageDispatcherBehavior defined interface for the dispatcher
// implementation. To create a custom dispatcher you should implement this interface
// and use it in GenStageOptions as a Dispatcher
type GenStageDispatcherBehavior interface {
	// InitStageDispatcher(opts)
	Init(opts GenStageOptions) interface{}

	// Ask called every time a consumer sends demand
	Ask(subscription GenStageSubscription, count uint, state interface{})

	// Cancel called every time a subscription is cancelled or the consumer goes down.
	Cancel(subscription GenStageSubscription, state interface{})

	// Dispatch called every time a producer wants to dispatch an event.
	Dispatch(events etf.List, state interface{}) []GenStageDispatchItem

	// Subscribe called every time the producer gets a new subscriber
	Subscribe(subscription GenStageSubscription, opts GenStageSubscribeOptions, state interface{}) error
}

type GenStageDispatcher int
type dispatcherDemand struct{}
type dispatcherBroadcast struct{}
type dispatcherPartition struct {
	n    uint
	hash func(etf.Term) int
}

// CreateGenStageDispatcherDemand creates a dispatcher that sends batches
// to the highest demand. This is the default dispatcher used
// by GenStage. In order to avoid greedy consumers, it is recommended
// that all consumers have exactly the same maximum demand.
func CreateGenStageDispatcherDemand() GenStageDispatcherBehavior {
	return &dispatcherDemand{}
}

// CreateGenStageDispatcherBroadcast creates a dispatcher that accumulates
// demand from all consumers before broadcasting events to all of them.
// This dispatcher guarantees that events are dispatched to
// all consumers without exceeding the demand of any given consumer.
// The demand is only sent upstream once all consumers ask for data.
func CreateGenStageDispatcherBroadcast() GenStageDispatcherBehavior {
	return &dispatcherBroadcast{}
}

// CreateGenStageDispatcherPartition creates a dispatcher that sends
// events according to partitions. Number of partitions 'n' must be > 0.
// 'hash' should return number within range [0,n). Value outside of this range
// is discarding event.
// If 'hash' is nil the random partition will be used on every event.
func CreateGenStageDispatcherPartition(n uint, hash func(etf.Term) int) GenStageDispatcherBehavior {
	if hash == nil {
		hash = func(event etf.Term) int {
			p := rand.Intn(int(n) - 1)
			return p
		}
	}
	return &dispatcherPartition{
		n:    n,
		hash: hash,
	}
}

type GenStageDispatchItem struct {
	subscription GenStageSubscription
	events       etf.List
}

//
// Dispatcher Demand implementation
//

type demand struct {
	subscription GenStageSubscription
	minDemand    uint
	maxDemand    uint
	count        uint
	partition    uint
}

type demandState struct {
	demands map[etf.Pid]*demand
	order   []etf.Pid
	i       int
	// buffer of events
	events         chan etf.Term
	bufferSize     uint
	bufferKeepLast bool
}

type partitionState struct {
	demands map[etf.Pid]*demand
	// partitioned
	order  [][]etf.Pid
	i      []int
	events []chan etf.Term

	bufferSize     uint
	bufferKeepLast bool
}

type broadcastState struct {
	demands map[etf.Pid]*demand
	// maxDemand should be a min value of all MaxDemand
	maxDemand uint

	// minDemand should be a max value of all MinDemand
	minDemand uint

	// Number of broadcast iteration could be done.
	// Computes on every Ask/Cancel call as a minimum value
	// among the all demands.
	broadcasts uint

	events         chan etf.Term
	bufferSize     uint
	bufferKeepLast bool
}

func (dd *dispatcherDemand) Init(opts GenStageOptions) interface{} {
	state := &demandState{
		demands:        make(map[etf.Pid]*demand),
		i:              0,
		events:         make(chan etf.Term, opts.BufferSize),
		bufferSize:     opts.BufferSize,
		bufferKeepLast: opts.BufferKeepLast,
	}
	return state
}

func (dd *dispatcherDemand) Ask(subscription GenStageSubscription, count uint, state interface{}) {
	st := state.(*demandState)
	demand, ok := st.demands[subscription.Pid]
	if !ok {
		return
	}
	demand.count += count
	return
}

func (dd *dispatcherDemand) Cancel(subscription GenStageSubscription, state interface{}) {
	st := state.(*demandState)
	delete(st.demands, subscription.Pid)
	for i := range st.order {
		if st.order[i] != subscription.Pid {
			continue
		}
		st.order[i] = st.order[0]
		st.order = st.order[1:]
		break
	}
	return
}

func (dd *dispatcherDemand) Dispatch(events etf.List, state interface{}) []GenStageDispatchItem {
	st := state.(*demandState)
	// put events into the buffer before we start dispatching
	for e := range events {
		select {
		case st.events <- events[e]:
			continue
		default:
			// buffer is full
			if st.bufferKeepLast {
				<-st.events
				st.events <- events[e]
				continue
			}
		}
		// seems we dont have enough space to keep these events.
		break
	}

	// check out whether we have subscribers
	if len(st.order) == 0 {
		return nil
	}

	dispatchItems := []GenStageDispatchItem{}
	nextDemand := st.i
	for {
		countLeft := uint(0)
		for range st.order {
			if st.i > len(st.order)-1 {
				st.i = 0
			}
			if len(st.events) == 0 {
				// have nothing to dispatch
				break
			}

			pid := st.order[st.i]
			demand := st.demands[pid]
			st.i++

			if demand.count == 0 || len(st.events) < int(demand.minDemand) {
				continue
			}

			nextDemand = st.i
			item := makeDispatchItem(st.events, demand)
			demand.count--
			dispatchItems = append(dispatchItems, item)

			if len(st.events) < int(demand.minDemand) {
				continue
			}
			countLeft += demand.count
		}
		if countLeft > 0 && len(st.events) > 0 {
			continue
		}
		break
	}

	st.i = nextDemand

	return dispatchItems
}

func (dd *dispatcherDemand) Subscribe(subscription GenStageSubscription, opts GenStageSubscribeOptions, state interface{}) error {
	st := state.(*demandState)
	newDemand := &demand{
		subscription: subscription,
		minDemand:    opts.MinDemand,
		maxDemand:    opts.MaxDemand,
	}
	st.demands[subscription.Pid] = newDemand
	st.order = append(st.order, subscription.Pid)
	return nil
}

//
// Dispatcher Broadcast implementation
//

func (db *dispatcherBroadcast) Init(opts GenStageOptions) interface{} {
	state := &broadcastState{
		demands:        make(map[etf.Pid]*demand),
		events:         make(chan etf.Term, opts.BufferSize),
		bufferSize:     opts.BufferSize,
		bufferKeepLast: opts.BufferKeepLast,
	}
	return state
}

func (db *dispatcherBroadcast) Ask(subscription GenStageSubscription, count uint, state interface{}) {
	st := state.(*broadcastState)
	demand, ok := st.demands[subscription.Pid]
	if !ok {
		return
	}
	demand.count += count
	st.broadcasts = minCountDemand(st.demands)
	return
}

func (db *dispatcherBroadcast) Cancel(subscription GenStageSubscription, state interface{}) {
	st := state.(*broadcastState)
	delete(st.demands, subscription.Pid)
	st.broadcasts = minCountDemand(st.demands)
	return
}

func (db *dispatcherBroadcast) Dispatch(events etf.List, state interface{}) []GenStageDispatchItem {
	st := state.(*broadcastState)
	// put events into the buffer before we start dispatching
	for e := range events {
		select {
		case st.events <- events[e]:
			continue
		default:
			// buffer is full
			if st.bufferKeepLast {
				<-st.events
				st.events <- events[e]
				continue
			}
		}
		// seems we dont have enough space to keep these events.
		break
	}

	demand := &demand{
		minDemand: st.minDemand,
		maxDemand: st.maxDemand,
	}

	dispatchItems := []GenStageDispatchItem{}
	for {
		if st.broadcasts == 0 {
			break
		}
		if len(st.events) < int(st.minDemand) {
			break
		}

		broadcast_item := makeDispatchItem(st.events, demand)
		for _, d := range st.demands {
			item := GenStageDispatchItem{
				subscription: d.subscription,
				events:       broadcast_item.events,
			}
			dispatchItems = append(dispatchItems, item)
			d.count--
		}
		st.broadcasts--
	}
	return dispatchItems
}

func (db *dispatcherBroadcast) Subscribe(subscription GenStageSubscription, opts GenStageSubscribeOptions, state interface{}) error {
	st := state.(*broadcastState)
	newDemand := &demand{
		subscription: subscription,
		minDemand:    opts.MinDemand,
		maxDemand:    opts.MaxDemand,
	}
	if len(st.demands) == 0 {
		st.minDemand = opts.MinDemand
		st.maxDemand = opts.MaxDemand
		st.demands[subscription.Pid] = newDemand
		return nil
	}

	// check if min and max outside of the having range
	// defined by the previous subscriptions
	if opts.MaxDemand < st.minDemand {
		return fmt.Errorf("broadcast dispatcher: MaxDemand (%d) outside of the accepted range (%d..%d)", opts.MaxDemand, st.minDemand, st.maxDemand)
	}
	if opts.MinDemand > st.maxDemand {
		return fmt.Errorf("broadcast dispatcher: MinDemand (%d) outside of the accepted range (%d..%d)", opts.MinDemand, st.minDemand, st.maxDemand)
	}

	// adjust the range
	if opts.MaxDemand < st.maxDemand {
		st.maxDemand = opts.MaxDemand
	}
	if opts.MinDemand > st.minDemand {
		st.minDemand = opts.MinDemand
	}
	st.demands[subscription.Pid] = newDemand
	// we should stop broadcast events until this subscription make a demand
	st.broadcasts = 0
	return nil
}

//
// Dispatcher Partition implementation
//
func (dp *dispatcherPartition) Init(opts GenStageOptions) interface{} {
	state := &partitionState{
		demands:        make(map[etf.Pid]*demand),
		order:          make([][]etf.Pid, dp.n),
		i:              make([]int, dp.n),
		events:         make([]chan etf.Term, dp.n),
		bufferSize:     opts.BufferSize,
		bufferKeepLast: opts.BufferKeepLast,
	}
	for i := range state.events {
		state.events[i] = make(chan etf.Term, state.bufferSize)
	}
	return state
}

func (dp *dispatcherPartition) Ask(subscription GenStageSubscription, count uint, state interface{}) {
	st := state.(*partitionState)
	demand, ok := st.demands[subscription.Pid]
	if !ok {
		return
	}
	demand.count += count
	return
}

func (dp *dispatcherPartition) Cancel(subscription GenStageSubscription, state interface{}) {
	st := state.(*partitionState)
	demand, ok := st.demands[subscription.Pid]
	if !ok {
		return
	}
	delete(st.demands, subscription.Pid)
	for i := range st.order[demand.partition] {
		if st.order[demand.partition][i] != subscription.Pid {
			continue
		}
		st.order[demand.partition][i] = st.order[demand.partition][0]
		st.order[demand.partition] = st.order[demand.partition][1:]
		break
	}
	return
}

func (dp *dispatcherPartition) Dispatch(events etf.List, state interface{}) []GenStageDispatchItem {
	st := state.(*partitionState)
	// put events into the buffer before we start dispatching
	for e := range events {
		partition := dp.hash(events[e])
		if partition < 0 || partition > int(dp.n-1) {
			// discard this event. partition is out of range
			continue
		}
		select {
		case st.events[partition] <- events[e]:
			continue
		default:
			// buffer is full
			if st.bufferKeepLast {
				<-st.events[partition]
				st.events[partition] <- events[e]
				continue
			}
		}
		// seems we dont have enough space to keep these events. discard the rest of them.
		fmt.Println("Warning: dispatcherPartition. Event buffer is full. Discarding event: ", events[e])
		break
	}

	dispatchItems := []GenStageDispatchItem{}
	for partition := range st.events {
		// do we have anything to dispatch?
		if len(st.events[partition]) == 0 {
			continue
		}

		nextDemand := st.i[partition]
		for {
			countLeft := uint(0)
			for range st.order[partition] {
				order_index := st.i[partition]
				if order_index > len(st.order[partition])-1 {
					order_index = 0
				}
				if len(st.events[partition]) == 0 {
					// have nothing to dispatch
					break
				}

				pid := st.order[partition][order_index]
				demand := st.demands[pid]
				st.i[partition] = order_index + 1

				if demand.count == 0 || len(st.events[partition]) < int(demand.minDemand) {
					continue
				}

				nextDemand = st.i[partition]
				item := makeDispatchItem(st.events[partition], demand)
				demand.count--
				dispatchItems = append(dispatchItems, item)
				if len(st.events[partition]) < int(demand.minDemand) {
					continue
				}
				countLeft += demand.count
			}
			if countLeft > 0 && len(st.events[partition]) > 0 {
				continue
			}
			break
		}

		st.i[partition] = nextDemand
	}
	return dispatchItems
}

func (dp *dispatcherPartition) Subscribe(subscription GenStageSubscription, opts GenStageSubscribeOptions, state interface{}) error {
	st := state.(*partitionState)
	if opts.Partition > dp.n-1 {
		return fmt.Errorf("unknown partition")
	}
	newDemand := &demand{
		subscription: subscription,
		minDemand:    opts.MinDemand,
		maxDemand:    opts.MaxDemand,
	}
	st.demands[subscription.Pid] = newDemand
	st.order[opts.Partition] = append(st.order[opts.Partition], subscription.Pid)
	return nil
}

// helpers

func makeDispatchItem(events chan etf.Term, d *demand) GenStageDispatchItem {
	item := GenStageDispatchItem{
		subscription: d.subscription,
	}

	i := uint(0)
	for {
		select {
		case e := <-events:
			item.events = append(item.events, e)
			i++
			if i == d.maxDemand {
				break
			}
			continue
		default:
			// we dont have events in the buffer
		}
		break
	}
	return item
}

func minCountDemand(demands map[etf.Pid]*demand) uint {
	if len(demands) == 0 {
		return uint(0)
	}

	minCount := uint(100)

	for _, d := range demands {
		if d.count < minCount {
			minCount = d.count
		}
	}
	return minCount
}
