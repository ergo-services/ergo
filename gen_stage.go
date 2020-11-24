package ergo

import (
	//"fmt"
	"github.com/halturin/ergo/etf"
	"github.com/halturin/ergo/lib"
	"sync"
)

type GenStageSubscriptionMode string
type GenStageDemandMode string
type GenStageType int

type GenStageSubscriptionOptions struct {
	to        etf.Term
	minDemand int
	maxDemand int
}

// GenStageOptions defines the GenStage' configuration via Init callback.
// Some options are specific to the chosen stage mode while others are
// shared across all types.
type GenStageOptions struct {
	// mode defines the way of acting
	stageType GenStageType

	// ---- options for GenStageModeProducer

	// when set to GenStageDemandModeForward, the demand is always forwarded
	// to the HandleDemand callback. When this options is set to
	// GenStageDemandModeAccumulate, demands are accumulated until mode is
	// set back to GenStageDemandModeForward via Demand method
	demand GenStageDemandMode

	// ---- options for GenStageModeProducer and GenStageModeProducerConsumer

	// bufferSize the size of the buffer to store events without demand.
	// default is 10000
	bufferSize uint

	// bufferKeepFirst defines whether the first or last entries should be
	// kept on the buffer in case the buffer size is exceeded.
	bufferKeepFirst bool

	// ---- options for GenStageModeConsumer and GenStageModeProducerConsumer

	// subscribeTo a list of producers to subscribe to. Each element represents
	// a tuple with process of Producer (atom, tuple with registered name and node,
	// etf.Pid) and the subscription options.
	subscribeTo []etf.Term
}

const (
	GenStageSubscriptionModeAuto   GenStageSubscriptionMode = "automatic"
	GenStageSubscriptionModeManual GenStageSubscriptionMode = "manual"
	GenStageSubscriptionModeStop   GenStageSubscriptionMode = "stop"

	GenStageDemandModeForward    GenStageDemandMode = "forward"
	GenStageDemandModeAccumulate GenStageDemandMode = "accumulate"

	GenStageTypeProducer         GenStageType = 1
	GenStageTypeConsumer         GenStageType = 2
	GenStageTypeProducerConsumer GenStageType = 3
)

type GenStageBehavior interface {

	// Init(...) -> (GenStageOptions, state)
	Init(process *Process, args ...interface{}) (GenStageOptions, interface{})

	// HandleSubscribe
	// HandleSubscribe -> (SubscriptionAuto | SubscriptionManual, State)

	// Invoked when a consumer is no longer subscribed to a producer.
	// HandleCancel -> ("noreply", subscription, state)
	//                 ("stop", reason, _)
	// where cancelReason is
	// 				etf.Tuple{"cancel", etf.Term(reason)}
	//              etf.Tuple{"down", etf.Term(reason)}
	// The cancelReason will be a {"cancel", _} tuple if the reason for cancellation
	// was a GenStage.cancel call. Any other value means the cancellation reason was
	// due to an EXIT.
	// Return values are the same as HandleCast.
	HandleCancel(cancelReason etf.Term, from etf.Term, state interface{}) (string, etf.Term, interface{})

	// HandleDemand
	// HandleDemand -> ("noreply", event, state)
	//                 ("stop", reason, _)
	// invoked on GenStageTypeProducer
	HandleDemand(demand uint64, state interface{}) (string, etf.Term, interface{})

	// HandleEvents
	HandleEvents(events interface{}, from etf.Term, state interface{}) (string, interface{})

	// HandleSubscribe This callback is invoked in both producers and consumers.
	// stageType will be GenStageTypeProducer if this callback is invoked on a consumer
	// and GenStageTypeConsumer if when this callback is invoked on producers a consumer subscribed to.
	// For consumers,  successful subscriptions must return one of:
	//   - (GenStageSubscriptionModeAuto, state)
	//      means the stage implementation will take care of automatically sending
	//      demand to producers. This is the default
	//   - (GenStageSubscriptionModeManual, state)
	//     means that demand must be sent to producers explicitly via Ask method.
	//     this kind of subscription must be canceled when HandleCancel is called
	// For producers, successful subscriptions must always return GenStageSubscriptionAuto.
	// Manual mode is not supported.
	HandleSubscribe(stageType GenStageType, options etf.List) (GenStageSubscriptionMode, interface{})
}

type GenStage struct{
	GenServer
}

type state {
	p *Process
}

func (gs *GenStage) Init(p *Process, args ...interface{}) interface{} {
	//var stageOptions GenStageOptions

	//etf.Atom("$info"):
	//etf.Atom("$demand"):
	//etf.Atom("$subscribe"):
	//etf.Atom("$gen_consumer"):
	//etf.Atom("$gen_producer"):

	return state{p: p}
}

func (gs *GenStage) HandleCall(from etf.Tuple, message *etf.Term, state interface{}) (string, etf.Term, interface{}) {
	return "reply", "ok", state
}

func (gs *GenStage) HandleCast(message *etf.Term, state interface{}) (string, interface{}) {
	return "noreply", state
}

func (gs *GenStage) HandleInfo(message *etf.Term, state interface{}) (string, interface{}) {
	return "noreply", state
}

func (gs *GenStage) Terminate(reason string, state interface{}) {
	
}

func (gst *GenStage) loop(p *Process, object interface{}, args ...interface{}) string {

}

func (gst *GenStage) GetDemandMode() GenStageDemandMode {
	return GenStageDemandModeForward //FIXME
}

// SetDemandMode Sets the demand mode for a producer
// When DemandModeForward (by default), the demand is always forwarded to the handle_demand callback.
// When DemandModeAccumulate, demand is accumulated until its mode is set to DemandModeForward.
// This is useful as a synchronization mechanism, where the demand is accumulated until
// all consumers are subscribed.
func (gst *GenStage) SetDemandMode(mode GenStageDemandMode) {
}

func (gst *GenStage) handleDirect(m directMessage) {

	if m.reply != nil {
		m.err = ErrUnsupportedRequest
		m.reply <- m
	}
}
