package ergo

import (
	"github.com/halturin/ergo/etf"
)

// GenStageDispatcherBehaviour defined interface for the dispatcher
// implementation.
type GenStageDispatcherBehaviour interface {
	// InitStageDispatcher(opts)
	Init(opts GenStageOptions) interface{}

	// Ask called every time a consumer sends demand
	Ask(demand string, from etf.Term, state interface{}) interface{}

	// Cancel called every time a subscription is cancelled or the consumer goes down.
	Cancel(from etf.Term, state interface{}) interface{}

	// Dispatch called every time a producer wants to dispatch an event.
	Dispatch(events string, length int, state interface{}) interface{}

	// Subscribe called every time the producer gets a new subscriber
	Subscribe(opts string, from etf.Term, state interface{}) interface{}
}

type GenStageDispatcher int
type dispatcherDemand struct{}
type dispatcherBroadcast struct{}
type dispatcherPartition struct{}

const (
	GenStageDispatcherDemand    GenStageDispatcher = 0
	GenStageDispatcherBroadcast GenStageDispatcher = 1
	GenStageDispatcherPartition GenStageDispatcher = 2
)

// CreateGenStageDispatcher creates a new dispatcher with a given type.
// There are 3 type of dispatchers we have implemented
//		GenStageDispatcherDemand
//			A dispatcher that sends batches to the highest demand.
//			This is the default dispatcher used by GenStage. In
//			order to avoid greedy consumers, it is recommended
//			that all consumers have exactly the same maximum demand.
//		GenStageDispatcherBroadcast
//			A dispatcher that accumulates demand from all consumers
//			before broadcasting events to all of them.
//			This dispatcher guarantees that events are dispatched to
//			all consumers without exceeding the demand of any given consumer.
//		GenStageDispatcherPartition
//			A dispatcher that sends events according to partitions.
//			Keep in mind that, if partitions are not evenly distributed,
//			a backed-up partition will slow all other ones
//		To create a custome one you should implement GenStageDispatcherBehaviour interface
func CreateGenStageDispatcher(dispatcher GenStageDispatcher) GenStageDispatcherBehaviour {
	switch dispatcher {
	case GenStageDispatcherDemand:
		return &dispatcherDemand{}
	case GenStageDispatcherBroadcast:
		return &dispatcherBroadcast{}
	case GenStageDispatcherPartition:
		return &dispatcherPartition{}
	}

	return nil
}

// Dispatcher Demand implementation

func (dd *dispatcherDemand) Init(opts GenStageOptions) interface{} {
	return nil
}

func (dd *dispatcherDemand) Ask(demand string, from etf.Term, state interface{}) interface{} {
	return state
}

func (dd *dispatcherDemand) Cancel(from etf.Term, state interface{}) interface{} {
	return state
}

func (dd *dispatcherDemand) Dispatch(events string, length int, state interface{}) interface{} {
	return state
}

func (dd *dispatcherDemand) Subscribe(opts string, from etf.Term, state interface{}) interface{} {
	return state
}

// Dispatcher Broadcast implementation

func (db *dispatcherBroadcast) Init(opts GenStageOptions) interface{} {
	return nil
}

func (db *dispatcherBroadcast) Ask(demand string, from etf.Term, state interface{}) interface{} {
	return state
}

func (db *dispatcherBroadcast) Cancel(from etf.Term, state interface{}) interface{} {
	return state
}

func (db *dispatcherBroadcast) Dispatch(events string, length int, state interface{}) interface{} {
	return state
}

func (db *dispatcherBroadcast) Subscribe(opts string, from etf.Term, state interface{}) interface{} {
	return state
}

// Dispatcher Partition implementation

func (dp *dispatcherPartition) Init(opts GenStageOptions) interface{} {
	return nil
}

func (dp *dispatcherPartition) Ask(demand string, from etf.Term, state interface{}) interface{} {
	return state
}

func (dp *dispatcherPartition) Cancel(from etf.Term, state interface{}) interface{} {
	return state
}

func (dp *dispatcherPartition) Dispatch(events string, length int, state interface{}) interface{} {
	return state
}

func (dp *dispatcherPartition) Subscribe(opts string, from etf.Term, state interface{}) interface{} {
	return state
}
