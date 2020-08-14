package ergo

import (
	"fmt"
	"github.com/halturin/ergo/etf"
	//"github.com/halturin/ergo/lib"
)

type GenStageSubscriptionMode string
type GenStageSubscriptionCancel string
type GenStageDemandMode string
type GenStageType int

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

	dispatcher GenStageDispatcherBehaviour
}

const (
	// GenStageSubscriptionModeAuto means the stage implementation will take care
	// of automatically sending demand to producers. This is the default
	GenStageSubscriptionModeAuto GenStageSubscriptionMode = "automatic"
	// GenStageSubscriptionModeManual means that demand must be sent to producers
	// explicitly via GenServer.Ask
	GenStageSubscriptionModeManual GenStageSubscriptionMode = "manual"
	GenStageSubscriptionModeStop   GenStageSubscriptionMode = "stop"

	// GenStageSubscriptionCancelPermanent the consumer exits when the producer cancels or exits.
	GenStageSubscriptionCancelPermanent GenStageSubscriptionCancel = "permanent"
	// GenStageSubscriptionCancelTransient the consumer exits only if reason is not "normal",
	// "shutdown", or {"shutdown", _}
	GenStageSubscriptionCancelTransient GenStageSubscriptionCancel = "transient"
	// GenStageSubscriptionCancelTemporary the consumer never exits
	GenStageSubscriptionCancelTemporary GenStageSubscriptionCancel = "temporary"

	GenStageDemandModeForward    GenStageDemandMode = "forward"
	GenStageDemandModeAccumulate GenStageDemandMode = "accumulate"

	GenStageTypeProducer         GenStageType = 1
	GenStageTypeConsumer         GenStageType = 2
	GenStageTypeProducerConsumer GenStageType = 3
)

type GenStageBehaviour interface {

	// InitStage(...) -> (GenStageOptions, state)
	InitStage(process *Process, args ...interface{}) (GenStageOptions, interface{})

	// HandleCancel
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
	HandleSubscribe(stageType GenStageType, options etf.List, state interface{}) (GenStageSubscriptionMode, interface{})
}

type GenStageSubscription struct {
	to      etf.Term
	self    *Process
	id      etf.Ref
	options GenStageSubscriptionOptions
}

type GenStageSubscriptionOptions struct {
	MinDemand uint
	MaxDemand uint
	Mode      GenStageSubscriptionMode
	Cancel    GenStageSubscriptionCancel
}

type GenStage struct {
	GenServer
}

type stateGenStage struct {
	p        *Process
	internal interface{}
	options  GenStageOptions
}

type stageRequestFrom struct {
	pid etf.Pid
	ref etf.Ref
}

type stageRequestCommand struct {
	cmd   etf.Atom
	value etf.Term
}

type stageMessage struct {
	request etf.Atom
	from    stageRequestFrom
	command stageRequestCommand
}

func (gs *GenStage) Init(p *Process, args ...interface{}) interface{} {
	//var stageOptions GenStageOptions
	var state stateGenStage

	state.p = p
	state.options, state.internal = p.object.(GenStageBehaviour).InitStage(p, args)

	return state
}

func (gs *GenStage) HandleCall(from etf.Tuple, message etf.Term, state interface{}) (string, etf.Term, interface{}) {
	var r stageMessage
	if err := etf.TermIntoStruct(message, &r); err != nil {
		return "reply", "error", state
	}

	if err := handleRequest(r, state.(stateGenStage).options); err != nil {
		return "reply", "error", state
	}

	return "reply", "ok", state
}

func (gs *GenStage) HandleCast(message etf.Term, state interface{}) (string, interface{}) {
	return "noreply", state
}

func (gs *GenStage) HandleInfo(message etf.Term, state interface{}) (string, interface{}) {
	return "noreply", state
}

func (gs *GenStage) Terminate(reason string, state interface{}) {
	return
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

func (gst *GenStage) Subscribe(p *Process, to etf.Term, opts GenStageSubscriptionOptions) (GenStageSubscription, error) {
	var subscription GenStageSubscription
	if p == nil {
		return subscription, fmt.Errorf("Subscription error. Process can not be nil")
	}

	subscription.id = p.MonitorProcess(to)
	subscription.self = p
	subscription.options = opts

	msg := etf.Tuple{
		"$gen_producer",
		etf.Tuple{subscription.id, p.Self()},
		etf.Tuple{etf.Atom("subscribe"), nil, opts},
	}
	p.Call(to, msg)

	return subscription, nil
}

func (gst *GenStage) Ask(subscription GenStageSubscription) error {
	return nil
}

func (gst *GenStage) Cancel(subscription GenStageSubscription) error {
	return nil
}

func handleRequest(m stageMessage, opts GenStageOptions) error {

	switch m.request {
	case "$gen_consumer":
		handleConsumer(m.from, m.command)
	case "$gen_producer":
		handleProducer(m.from, m.command)
	case "$demand":
	case "$subscribe":
	case "$info":
	}

	return fmt.Errorf("unknownRequest")
}

func handleConsumer(from stageRequestFrom, cmd stageRequestCommand) error {
	return nil
}

func handleProducer(from stageRequestFrom, cmd stageRequestCommand) error {
	return nil
}
