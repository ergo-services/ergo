package ergo

import (
	"fmt"
	"time"

	"github.com/halturin/ergo/etf"
	//"github.com/halturin/ergo/lib"
)

type GenStageCancelMode uint

// GenStageOptions defines the GenStage' configuration using Init callback.
// Some options are specific to the chosen stage mode while others are
// shared across all types.
type GenStageOptions struct {

	// If this stage acts as a consumer you can to define producers
	// this stage should subscribe to.
	// SubscribeTo is a list of GenStageSubscribeTo. Each element represents
	// a producer (etf.Pid or registered name) and subscription options.
	SubscribeTo []GenStageSubscribeTo

	// Options below are for the stage that acts as a producer.

	// DisableForwarding. the demand is always forwarded to the HandleDemand callback.
	// When this options is set to 'true', demands are accumulated until mode is
	// set back to 'false' using DisableDemandAccumulating method
	DisableForwarding bool

	// BufferSize the size of the buffer to store events without demand.
	// default value = defaultDispatcherBufferSize
	BufferSize uint

	// BufferKeepLast defines whether the first or last entries should be
	// kept on the buffer in case the buffer size is exceeded.
	BufferKeepLast bool

	Dispatcher GenStageDispatcherBehavior
}

const (
	GenStageCancelPermanent GenStageCancelMode = 0
	GenStageCancelTransient GenStageCancelMode = 1
	GenStageCancelTemporary GenStageCancelMode = 2

	defaultDispatcherBufferSize = 10000
	defaultAutoDemandCount      = 3
)

var (
	ErrNotAProducer = fmt.Errorf("not a producer")
)

// GenStageBehavior interface for the GenStage inmplementation
type GenStageBehavior interface {

	// InitStage
	InitStage(state *GenStageState, args ...interface{}) error

	// HandleDemand this callback is invoked on a producer stage
	// The producer that implements this callback must either store the demand, or return the amount of requested events.
	// Use `ergo.ErrStop` as an error for the normal shutdown this process. Any other error values
	// will be used as a reason for the abnornal shutdown process.
	HandleDemand(state *GenStageState, subscription GenStageSubscription, count uint) (error, etf.List)

	// HandleEvents this callback is invoked on a consumer stage.
	// Use `ergo.ErrStop` as an error for the normal shutdown this process. Any other error values
	// will be used as a reason for the abnornal shutdown process.
	HandleEvents(state *GenStageState, subscription GenStageSubscription, events etf.List) error

	// HandleSubscribe This callback is invoked on a producer stage.
	// Use `ergo.ErrStop` as an error for the normal shutdown this process. Any other error values
	// will be used as a reason for the abnornal shutdown process.
	HandleSubscribe(state *GenStageState, subscription GenStageSubscription, options GenStageSubscribeOptions) error

	// HandleSubscribed this callback is invoked as a confirmation for the subscription request
	// Returning false means that demand must be sent to producers explicitly using Ask method.
	// Returning true means the stage implementation will take care of automatically sending.
	// Use `ergo.ErrStop` as an error for the normal shutdown this process. Any other error values
	HandleSubscribed(state *GenStageState, subscription GenStageSubscription, opts GenStageSubscribeOptions) (error, bool)

	// HandleCancel
	// Invoked when a consumer is no longer subscribed to a producer (invoked on a producer stage)
	// The cancelReason will be a {Cancel: "cancel", Reason: _} if the reason for cancellation
	// was a GenStage.Cancel call. Any other value means the cancellation reason was
	// due to an EXIT.
	// Use `ergo.ErrStop` as an error for the normal shutdown this process. Any other error values
	// will be used as a reason for the abnornal shutdown process.
	HandleCancel(state *GenStageState, subscription GenStageSubscription, reason string) error

	// HandleCanceled
	// Invoked when a consumer is no longer subscribed to a producer (invoked on a consumer stage)
	// Termination this stage depends on a cancel mode for the given subscription. For the cancel mode
	// GenStageCancelPermanent - this stage will be terminated right after this callback invoking.
	// For the cancel mode GenStageCancelTransient - it depends on a reason of subscription canceling.
	// Cancel mode GenStageCancelTemporary keeps this stage alive whether the reason could be.
	HandleCanceled(state *GenStageState, subscription GenStageSubscription, reason string) error

	// HandleGenStageCall this callback is invoked on Process.Call. This method is optional
	// for the implementation
	HandleGenStageCall(state *GenStageState, from GenServerFrom, message etf.Term) (string, etf.Term)
	// HandleGenStageCast this callback is invoked on Process.Cast. This method is optional
	// for the implementation
	HandleGenStageCast(state *GenStageState, message etf.Term) string
	// HandleGenStageInfo this callback is invoked on Process.Send. This method is optional
	// for the implementation
	HandleGenStageInfo(state *GenStageState, message etf.Term) string
}

type GenStageSubscription struct {
	Pid etf.Pid
	Ref etf.Ref
}

type subscriptionInternal struct {
	Producer     etf.Term
	Subscription GenStageSubscription
	Options      GenStageSubscribeOptions
	Monitor      etf.Ref
	// number of event requests (demands) made as a consumer.
	count int
}

type GenStageSubscribeTo struct {
	Producer etf.Term
	Options  GenStageSubscribeOptions
}

type GenStageSubscribeOptions struct {
	MinDemand uint `etf:"min_demand"`
	MaxDemand uint `etf:"max_demand"`
	// The stage implementation will take care of automatically sending
	// demand to producer (as a default behavior). You can disable it
	// setting ManualDemand to true
	ManualDemand bool `etf:"manual"`
	// What should happened with consumer if producer has terminated
	// GenStageCancelPermanent the consumer exits when the producer cancels or exits.
	// GenStageCancelTransient the consumer exits only if reason is not "normal",
	// "shutdown", or {"shutdown", _}
	// GenStageCancelTemporary the consumer never exits
	Cancel GenStageCancelMode `etf:"cancel"`

	// Partition is defined the number of partition this subscription should belongs to.
	// This option uses in the DispatcherPartition
	Partition uint `etf:"partition"`

	// Extra is intended to be a custom set of options for the custom implementation
	// of GenStageDispatcherBehavior
	Extra etf.Term `etf:"extra"`
}

type GenStageCancelReason struct {
	Cancel string
	Reason string
}

type GenStage struct {
	GenServer
}

type GenStageState struct {
	GenServerState

	Options         GenStageOptions
	demandBuffer    []demandRequest
	dispatcherState interface{}
	// keep our subscriptions
	producers map[etf.Ref]*subscriptionInternal
	// keep our subscribers
	consumers map[etf.Pid]*subscriptionInternal
}

type stageRequestCommand struct {
	Cmd  etf.Atom
	Opt1 interface{}
	Opt2 interface{}
}

type stageMessage struct {
	Request      etf.Atom
	Subscription GenStageSubscription
	Command      interface{}
}

type setManualDemand struct {
	subscription GenStageSubscription
	enable       bool
}

type setCancelMode struct {
	subscription GenStageSubscription
	cancel       GenStageCancelMode
}

type doSubscribe struct {
	to      etf.Term
	options GenStageSubscribeOptions
}

type setForwardDemand struct {
	forward bool
}

type demandRequest struct {
	subscription GenStageSubscription
	count        uint
}

type cancelSubscription struct {
	subscription GenStageSubscription
	reason       string
}

type sendEvents struct {
	events etf.List
}

// GenStage methods

// DisableAutoDemand means that demand must be sent to producers explicitly using Ask method. This
// mode can be used when a special behavior is desired.
func (gst *GenStage) DisableAutoDemand(p *Process, subscription GenStageSubscription) error {
	message := setManualDemand{
		subscription: subscription,
		enable:       false,
	}
	_, err := p.Direct(message)
	return err
}

// EnableAutoDemand enables auto demand mode (this is default mode for the consumer).
func (gst *GenStage) EnableAutoDemand(p *Process, subscription GenStageSubscription) error {
	if p == nil {
		return fmt.Errorf("Subscription error. Process can not be nil")
	}
	message := setManualDemand{
		subscription: subscription,
		enable:       false,
	}
	_, err := p.Direct(message)
	return err
}

// EnableForwardDemand enables forwarding messages to the HandleDemand on a producer stage.
// This is default mode for the producer.
func (gst *GenStage) EnableForwardDemand(p *Process) error {
	message := setForwardDemand{
		forward: true,
	}
	_, err := p.Direct(message)
	return err
}

// DisableForwardDemand disables forwarding messages to the HandleDemand on a producer stage.
// This is useful as a synchronization mechanism, where the demand is accumulated until
// all consumers are subscribed.
func (gst *GenStage) DisableForwardDemand(p *Process) error {
	message := setForwardDemand{
		forward: false,
	}
	_, err := p.Direct(message)
	return err
}

// SendEvents sends events for the subscribers
func (gst *GenStage) SendEvents(p *Process, events etf.List) error {
	message := sendEvents{
		events: events,
	}
	_, err := p.Direct(message)
	return err
}

// SetCancelMode defines how consumer will handle termination of the producer. There are 3 modes:
// GenStageCancelPermanent (default) - consumer exits when the producer cancels or exits
// GenStageCancelTransient - consumer exits only if reason is not normal, shutdown, or {shutdown, reason}
// GenStageCancelTemporary - never exits
func (gst *GenStage) SetCancelMode(p *Process, subscription GenStageSubscription, cancel GenStageCancelMode) error {
	message := setCancelMode{
		subscription: subscription,
		cancel:       cancel,
	}

	_, err := p.Direct(message)
	return err
}

// Subscribe subscribes to the given producer. HandleSubscribed callback will be invoked
// on a consumer stage once a request for the subscription is sent. If something went wrong
// on a producer side the callback HandleCancel will be invoked with a reason of cancelation.
func (gst *GenStage) Subscribe(p *Process, producer etf.Term, opts GenStageSubscribeOptions) GenStageSubscription {
	var subscription GenStageSubscription

	subscription_id := p.MonitorProcess(producer)
	subscription.Pid = p.Self()
	subscription.Ref = subscription_id

	subscribe_opts := etf.List{
		etf.Tuple{
			etf.Atom("min_demand"),
			opts.MinDemand,
		},
		etf.Tuple{
			etf.Atom("max_demand"),
			opts.MaxDemand,
		},
		etf.Tuple{
			etf.Atom("cancel"),
			int(opts.Cancel), // custom types couldn't be handled by etf.Encode
		},
		etf.Tuple{
			etf.Atom("manual"),
			opts.ManualDemand,
		},
		etf.Tuple{
			etf.Atom("partition"),
			opts.Partition,
		},
	}

	// In order to get rid of race condition we should send this message
	// before we send 'subscribe' to the producer process. Just
	// to make sure if we registered this subscription before the 'DOWN'
	// or 'EXIT' message arrived in case of something went wrong.
	msg := etf.Tuple{
		etf.Atom("$gen_consumer"),
		etf.Tuple{p.Self(), subscription_id},
		etf.Tuple{etf.Atom("subscribed"), producer, subscribe_opts},
	}
	p.Send(p.Self(), msg)

	msg = etf.Tuple{
		etf.Atom("$gen_producer"),
		etf.Tuple{p.Self(), subscription_id},
		etf.Tuple{etf.Atom("subscribe"), nil, subscribe_opts},
	}
	p.Send(producer, msg)

	return subscription
}

// Ask makes a demand request for the given subscription. This function must only be
// used in the cases when a consumer sets a subscription to manual mode using DisableAutoDemand
func (gst *GenStage) Ask(p *Process, subscription GenStageSubscription, count uint) error {
	message := demandRequest{
		subscription: subscription,
		count:        count,
	}
	_, err := p.Direct(message)
	return err
}

// Cancel
func (gst *GenStage) Cancel(p *Process, subscription GenStageSubscription, reason string) error {
	message := cancelSubscription{
		subscription: subscription,
		reason:       reason,
	}
	_, err := p.Direct(message)
	return err
}

//
// GenServer callbacks
//
func (gst *GenStage) Init(state *GenServerState, args ...interface{}) error {
	//var stageOptions GenStageOptions

	stageState := &GenStageState{
		GenServerState: *state,
		producers:      make(map[etf.Ref]*subscriptionInternal),
		consumers:      make(map[etf.Pid]*subscriptionInternal),
	}

	state.State = stageState

	if err := state.Process.GetObject().(GenStageBehavior).InitStage(stageState, args); err != nil {
		return err
	}

	if stageState.Options.BufferSize == 0 {
		stageState.Options.BufferSize = defaultDispatcherBufferSize
	}

	// if dispatcher wasn't specified create a default one GenStageDispatcherDemand
	if stageState.Options.Dispatcher == nil {
		stageState.Options.Dispatcher = CreateGenStageDispatcherDemand()
	}

	stageState.dispatcherState = stageState.Options.Dispatcher.Init(stageState.Options)
	if len(stageState.Options.SubscribeTo) > 0 {
		for _, s := range stageState.Options.SubscribeTo {
			gst.Subscribe(state.Process, s.Producer, s.Options)
		}
	}

	return nil
}

func (gst *GenStage) HandleCall(state *GenServerState, from GenServerFrom, message etf.Term) (string, etf.Term) {
	st := state.State.(*GenStageState)
	return state.Process.GetObject().(GenStageBehavior).HandleGenStageCall(st, from, message)
}

func (gst *GenStage) HandleDirect(state *GenServerState, message interface{}) (interface{}, error) {
	st := state.State.(*GenStageState)
	switch m := message.(type) {
	case setManualDemand:
		subInternal, ok := st.producers[m.subscription.Ref]
		if !ok {
			return nil, fmt.Errorf("unknown subscription")
		}
		subInternal.Options.ManualDemand = m.enable
		if subInternal.count < defaultAutoDemandCount && !subInternal.Options.ManualDemand {
			sendDemand(state.Process, subInternal.Producer, m.subscription, defaultAutoDemandCount)
			subInternal.count += defaultAutoDemandCount
		}
		return nil, nil

	case setCancelMode:
		subInternal, ok := st.producers[m.subscription.Ref]
		if !ok {
			return nil, fmt.Errorf("unknown subscription")
		}
		subInternal.Options.Cancel = m.cancel
		return nil, nil

	case setForwardDemand:
		st.Options.DisableForwarding = !m.forward
		if !m.forward {
			return nil, nil
		}

		// create demand with count = 0, which will be ignored but start
		// the processing of the buffered demands
		msg := etf.Tuple{
			etf.Atom("$gen_producer"),
			etf.Tuple{etf.Pid{}, etf.Ref{}},
			etf.Tuple{etf.Atom("ask"), 0},
		}
		state.Process.Send(state.Process.Self(), msg)

		return nil, nil

	case sendEvents:
		var deliver []GenStageDispatchItem
		// dispatch to the subscribers
		deliver = st.Options.Dispatcher.Dispatch(st.dispatcherState, m.events)
		if len(deliver) == 0 {
			return nil, nil
		}
		for d := range deliver {
			msg := etf.Tuple{
				etf.Atom("$gen_consumer"),
				etf.Tuple{deliver[d].subscription.Pid, deliver[d].subscription.Ref},
				deliver[d].events,
			}
			state.Process.Send(deliver[d].subscription.Pid, msg)
		}
		return nil, nil

	case demandRequest:
		subInternal, ok := st.producers[m.subscription.Ref]
		if !ok {
			return nil, fmt.Errorf("unknown subscription")
		}
		if !subInternal.Options.ManualDemand {
			return nil, fmt.Errorf("auto demand")
		}

		sendDemand(state.Process, subInternal.Producer, m.subscription, m.count)
		subInternal.count += int(m.count)
		return nil, nil

	case cancelSubscription:
		// if we act as a consumer with this subscription
		if subInternal, ok := st.producers[m.subscription.Ref]; ok {
			msg := etf.Tuple{
				etf.Atom("$gen_producer"),
				etf.Tuple{m.subscription.Pid, m.subscription.Ref},
				etf.Tuple{etf.Atom("cancel"), m.reason},
			}
			state.Process.Send(subInternal.Producer, msg)
			cmd := stageRequestCommand{
				Cmd:  etf.Atom("cancel"),
				Opt1: "normal",
			}
			if _, err := handleConsumer(st, subInternal.Subscription, cmd); err != nil {
				return nil, err
			}
			return nil, nil
		}
		// if we act as a producer within this subscription
		if subInternal, ok := st.consumers[m.subscription.Pid]; ok {
			msg := etf.Tuple{
				etf.Atom("$gen_consumer"),
				etf.Tuple{m.subscription.Pid, m.subscription.Ref},
				etf.Tuple{etf.Atom("cancel"), m.reason},
			}
			state.Process.Send(m.subscription.Pid, msg)
			state.Process.DemonitorProcess(subInternal.Monitor)
			cmd := stageRequestCommand{
				Cmd:  etf.Atom("cancel"),
				Opt1: "normal",
			}
			if _, err := handleProducer(st, subInternal.Subscription, cmd); err != nil {
				return nil, err
			}
			return nil, nil
		}
		return nil, fmt.Errorf("unknown subscription")

	default:
		return nil, ErrUnsupportedRequest
	}

}

func (gst *GenStage) HandleCast(state *GenServerState, message etf.Term) string {
	st := state.State.(*GenStageState)
	return state.Process.GetObject().(GenStageBehavior).HandleGenStageCast(st, message)
}

func (gst *GenStage) HandleInfo(state *GenServerState, message etf.Term) string {
	var r stageMessage

	st := state.State.(*GenStageState)

	// check if we got a 'DOWN' message
	// {DOWN, Ref, process, PidOrName, Reason}
	if isDown, d := IsDownMessage(message); isDown {
		if err := handleStageDown(st, d); err != nil {
			return err.Error()
		}
		return "noreply"
	}

	if err := etf.TermIntoStruct(message, &r); err != nil {
		reply := state.Process.GetObject().(GenStageBehavior).HandleGenStageInfo(st, message)
		return reply
	}

	_, err := handleStageRequest(st, r)

	switch err {
	case nil:
		return "noreply"
	case ErrStop:
		return "stop"
	case ErrUnsupportedRequest:
		reply := state.Process.GetObject().(GenStageBehavior).HandleGenStageInfo(st, message)
		return reply
	default:
		return err.Error()
	}
}

// default callbacks

func (gst *GenStage) InitStage(state *GenStageState, args ...interface{}) error {
	return nil
}

func (gst *GenStage) HandleGenStageCall(state *GenStageState, from GenServerFrom, message etf.Term) (string, etf.Term) {
	// default callback if it wasn't implemented
	fmt.Printf("HandleGenStageCall: unhandled message (from %#v) %#v\n", from, message)
	return "reply", etf.Atom("ok")
}

func (gst *GenStage) HandleGenStageCast(state *GenStageState, message etf.Term) string {
	// default callback if it wasn't implemented
	fmt.Printf("HandleGenStageCast: unhandled message %#v\n", message)
	return "noreply"
}
func (gst *GenStage) HandleGenStageInfo(state *GenStageState, message etf.Term) string {
	// default callback if it wasn't implemnted
	fmt.Printf("HandleGenStageInfo: unhandled message %#v\n", message)
	return "noreply"
}

func (gst *GenStage) HandleSubscribe(state *GenStageState, subscription GenStageSubscription, options GenStageSubscribeOptions) error {
	return ErrNotAProducer
}

func (gst *GenStage) HandleSubscribed(state *GenStageState, subscription GenStageSubscription, opts GenStageSubscribeOptions) (error, bool) {
	return nil, opts.ManualDemand
}

func (gst *GenStage) HandleCancel(state *GenStageState, subscription GenStageSubscription, reason string) error {
	// default callback if it wasn't implemented
	return nil
}

func (gst *GenStage) HandleCanceled(state *GenStageState, subscription GenStageSubscription, reason string) error {
	// default callback if it wasn't implemented
	return nil
}

func (gst *GenStage) HandleEvents(state *GenStageState, subscription GenStageSubscription, events etf.List) error {
	fmt.Printf("GenStage HandleEvents: unhandled subscription (%#v) events %#v\n", subscription, events)
	return nil
}

func (gst *GenStage) HandleDemand(state *GenStageState, subscription GenStageSubscription, count uint) (error, etf.List) {
	fmt.Printf("GenStage HandleDemand: unhandled subscription (%#v) demand %#v\n", subscription, count)
	return nil, nil
}

// private functions

func handleStageRequest(state *GenStageState, m stageMessage) (etf.Term, error) {
	var command stageRequestCommand
	switch m.Request {
	case "$gen_consumer":
		// I wish i had {events, [...]} for the events message (in
		// fashion of the other messages), but the original autors
		// made this way, so i have to use this little hack in order
		// to use the same handler
		if cmd, ok := m.Command.(etf.List); ok {
			command.Cmd = etf.Atom("events")
			command.Opt1 = cmd
			return handleConsumer(state, m.Subscription, command)
		}
		if err := etf.TermIntoStruct(m.Command, &command); err != nil {
			return nil, ErrUnsupportedRequest
		}
		return handleConsumer(state, m.Subscription, command)
	case "$gen_producer":
		if err := etf.TermIntoStruct(m.Command, &command); err != nil {
			return nil, ErrUnsupportedRequest
		}
		return handleProducer(state, m.Subscription, command)
	}
	return nil, ErrUnsupportedRequest
}

func handleConsumer(state *GenStageState, subscription GenStageSubscription, cmd stageRequestCommand) (etf.Term, error) {
	var subscriptionOpts GenStageSubscribeOptions
	var err error
	var manualDemand bool

	switch cmd.Cmd {
	case etf.Atom("events"):
		events := cmd.Opt1.(etf.List)
		object := state.Process.GetObject()
		numEvents := len(events)

		subInternal, ok := state.producers[subscription.Ref]
		if !ok {
			fmt.Printf("Warning! got %d events for unknown subscription %#v\n", numEvents, subscription)
			return etf.Atom("ok"), nil
		}
		subInternal.count--
		if subInternal.count < 0 {
			return nil, fmt.Errorf("got %d events which haven't bin requested", numEvents)
		}
		if numEvents < int(subInternal.Options.MinDemand) {
			return nil, fmt.Errorf("got %d events which is less than min %d", numEvents, subInternal.Options.MinDemand)
		}
		if numEvents > int(subInternal.Options.MaxDemand) {
			return nil, fmt.Errorf("got %d events which is more than max %d", numEvents, subInternal.Options.MaxDemand)
		}

		err = object.(GenStageBehavior).HandleEvents(state, subscription, events)
		if err != nil {
			return nil, err
		}

		// if subscription has auto demand we should request yet another
		// bunch of events
		if subInternal.count < defaultAutoDemandCount && !subInternal.Options.ManualDemand {
			sendDemand(state.Process, subInternal.Producer, subscription, defaultAutoDemandCount)
			subInternal.count += defaultAutoDemandCount
		}
		return etf.Atom("ok"), nil

	case etf.Atom("subscribed"):
		if err := etf.TermProplistIntoStruct(cmd.Opt2, &subscriptionOpts); err != nil {
			return nil, err
		}

		object := state.Process.GetObject()
		err, manualDemand = object.(GenStageBehavior).HandleSubscribed(state, subscription, subscriptionOpts)

		if err != nil {
			return nil, err
		}
		subscriptionOpts.ManualDemand = manualDemand

		producer := cmd.Opt1
		subInternal := &subscriptionInternal{
			Subscription: subscription,
			Producer:     producer,
			Options:      subscriptionOpts,
		}
		state.producers[subscription.Ref] = subInternal

		if !manualDemand {
			sendDemand(state.Process, producer, subscription, defaultAutoDemandCount)
			subInternal.count = defaultAutoDemandCount
		}

		return etf.Atom("ok"), nil

	case etf.Atom("retry-cancel"):
		// if "subscribed" message hasn't still arrived then just ignore it
		if _, ok := state.producers[subscription.Ref]; !ok {
			return etf.Atom("ok"), nil
		}
		fallthrough
	case etf.Atom("cancel"):
		// the subscription was canceled
		reason, ok := cmd.Opt1.(string)
		if !ok {
			return nil, fmt.Errorf("Cancel reason is not a string")
		}

		subInternal, ok := state.producers[subscription.Ref]
		if !ok {
			// There might be a case when "cancel" message arrives before
			// the "subscribed" message due to async nature of messaging,
			// so we should wait a bit and try to handle it one more time
			// using "retry-cancel" message.
			// I got this problem with GOMAXPROCS=1
			msg := etf.Tuple{
				etf.Atom("$gen_consumer"),
				etf.Tuple{subscription.Pid, subscription.Ref},
				etf.Tuple{etf.Atom("retry-cancel"), reason},
			}
			// handle it in a second
			state.Process.SendAfter(state.Process.Self(), msg, 200*time.Millisecond)
			return etf.Atom("ok"), nil
		}

		state.Process.DemonitorProcess(subscription.Ref)

		object := state.Process.GetObject()
		err = object.(GenStageBehavior).HandleCanceled(state, subscription, reason)
		if err != nil {
			return nil, err
		}

		delete(state.producers, subscription.Ref)

		switch subInternal.Options.Cancel {
		case GenStageCancelTemporary:
			return etf.Atom("ok"), nil
		case GenStageCancelTransient:
			if reason == "normal" || reason == "shutdown" {
				return etf.Atom("ok"), nil
			}
			return nil, fmt.Errorf(reason)
		default:
			// GenStageCancelPermanent
			return nil, fmt.Errorf(reason)
		}
	}

	return nil, fmt.Errorf("unknown GenStage command (HandleCast)")
}

func handleProducer(state *GenStageState, subscription GenStageSubscription, cmd stageRequestCommand) (etf.Term, error) {
	var subscriptionOpts GenStageSubscribeOptions
	var err error

	switch cmd.Cmd {
	case etf.Atom("subscribe"):
		// {subscribe, Cancel, Opts}
		if err = etf.TermProplistIntoStruct(cmd.Opt2, &subscriptionOpts); err != nil {
			return nil, err
		}

		if subscriptionOpts.MinDemand > subscriptionOpts.MaxDemand {
			msg := etf.Tuple{
				etf.Atom("$gen_consumer"),
				etf.Tuple{subscription.Pid, subscription.Ref},
				etf.Tuple{etf.Atom("cancel"), fmt.Errorf("MinDemand greater MaxDemand")},
			}
			state.Process.Send(subscription.Pid, msg)
			return etf.Atom("ok"), nil
		}

		object := state.Process.GetObject()
		err = object.(GenStageBehavior).HandleSubscribe(state, subscription, subscriptionOpts)

		switch err {
		case nil:
			// cancel current subscription if this consumer has been already subscribed
			if s, ok := state.consumers[subscription.Pid]; ok {
				msg := etf.Tuple{
					etf.Atom("$gen_consumer"),
					etf.Tuple{subscription.Pid, s.Subscription.Ref},
					etf.Tuple{etf.Atom("cancel"), "resubscribed"},
				}
				state.Process.Send(subscription.Pid, msg)
				// notify dispatcher about cancelation the previous subscription
				canceledSubscription := GenStageSubscription{
					Pid: subscription.Pid,
					Ref: s.Subscription.Ref,
				}
				// cancel current demands
				state.Options.Dispatcher.Cancel(state.dispatcherState, canceledSubscription)
				// notify dispatcher about the new subscription
				if err := state.Options.Dispatcher.Subscribe(state.dispatcherState, subscription, subscriptionOpts); err != nil {
					// dispatcher can't handle this subscription
					msg := etf.Tuple{
						etf.Atom("$gen_consumer"),
						etf.Tuple{subscription.Pid, s.Subscription.Ref},
						etf.Tuple{etf.Atom("cancel"), err.Error()},
					}
					state.Process.Send(subscription.Pid, msg)
					return etf.Atom("ok"), nil
				}

				s.Subscription = subscription
				return etf.Atom("ok"), nil
			}

			if err := state.Options.Dispatcher.Subscribe(state.dispatcherState, subscription, subscriptionOpts); err != nil {
				// dispatcher can't handle this subscription
				msg := etf.Tuple{
					etf.Atom("$gen_consumer"),
					etf.Tuple{subscription.Pid, subscription.Ref},
					etf.Tuple{etf.Atom("cancel"), err.Error()},
				}
				state.Process.Send(subscription.Pid, msg)
				return etf.Atom("ok"), nil
			}

			// monitor subscriber in order to remove this subscription
			// if it terminated unexpectedly
			m := state.Process.MonitorProcess(subscription.Pid)
			s := &subscriptionInternal{
				Subscription: subscription,
				Monitor:      m,
				Options:      subscriptionOpts,
			}
			state.consumers[subscription.Pid] = s
			return etf.Atom("ok"), nil

		case ErrNotAProducer:
			// if it wasnt overloaded - send 'cancel' to the consumer
			msg := etf.Tuple{
				etf.Atom("$gen_consumer"),
				etf.Tuple{subscription.Pid, subscription.Ref},
				etf.Tuple{etf.Atom("cancel"), err.Error()},
			}
			state.Process.Send(subscription.Pid, msg)
			return etf.Atom("ok"), nil

		default:
			// any other error should terminate this stage
			return nil, err
		}
	case etf.Atom("retry-ask"):
		// if "subscribe" message hasn't still arrived, send a cancelation message
		// to the consumer
		if _, ok := state.consumers[subscription.Pid]; !ok {
			msg := etf.Tuple{
				etf.Atom("$gen_consumer"),
				etf.Tuple{subscription.Pid, subscription.Ref},
				etf.Tuple{etf.Atom("cancel"), "not subscribed"},
			}
			state.Process.Send(subscription.Pid, msg)
			return etf.Atom("ok"), nil
		}
		fallthrough

	case etf.Atom("ask"):
		var events etf.List
		var deliver []GenStageDispatchItem
		var count uint
		switch c := cmd.Opt1.(type) {
		case int:
			count = uint(c)
		case uint:
			count = c
		default:
			return nil, fmt.Errorf("Demand has wrong value %#v. Expected positive integer", cmd.Opt1)
		}

		// handle buffered demand on exit this function
		defer func() {
			if state.Options.DisableForwarding {
				return
			}
			if len(state.demandBuffer) == 0 {
				return
			}
			d := state.demandBuffer[0]
			msg := etf.Tuple{
				etf.Atom("$gen_producer"),
				etf.Tuple{d.subscription.Pid, d.subscription.Ref},
				etf.Tuple{etf.Atom("ask"), d.count},
			}
			state.Process.Send(state.Process.Self(), msg)
			state.demandBuffer = state.demandBuffer[1:]
		}()

		if count == 0 {
			// just ignore it
			return etf.Atom("ok"), nil
		}

		if _, ok := state.consumers[subscription.Pid]; !ok {
			// there might be a case when "ask" message arrives before
			// the "subscribe" message due to async nature of messaging,
			// so we should wait a bit and try to handle it one more time
			// using "retry-ask" message
			msg := etf.Tuple{
				etf.Atom("$gen_producer"),
				etf.Tuple{subscription.Pid, subscription.Ref},
				etf.Tuple{etf.Atom("retry-ask"), count},
			}
			// handle it in a second
			state.Process.SendAfter(state.Process.Self(), msg, 1*time.Second)
			return etf.Atom("ok"), nil
		}

		if state.Options.DisableForwarding {
			d := demandRequest{
				subscription: subscription,
				count:        count,
			}
			// FIXME it would be more effective to use sync.Pool with
			// preallocated array behind the slice.
			// see how it was made in lib.TakeBuffer
			state.demandBuffer = append(state.demandBuffer, d)
			return etf.Atom("ok"), nil
		}

		object := state.Process.GetObject()
		_, events = object.(GenStageBehavior).HandleDemand(state, subscription, count)

		// register this demand and trying to dispatch having events
		dispatcher := state.Options.Dispatcher
		dispatcher.Ask(state.dispatcherState, subscription, count)
		deliver = dispatcher.Dispatch(state.dispatcherState, events)
		if len(deliver) == 0 {
			return etf.Atom("ok"), nil
		}

		for d := range deliver {
			msg := etf.Tuple{
				etf.Atom("$gen_consumer"),
				etf.Tuple{deliver[d].subscription.Pid, deliver[d].subscription.Ref},
				deliver[d].events,
			}
			state.Process.Send(deliver[d].subscription.Pid, msg)
		}

		return etf.Atom("ok"), nil

	case etf.Atom("cancel"):
		var e error
		// handle this cancelation in the dispatcher
		dispatcher := state.Options.Dispatcher
		dispatcher.Cancel(state.dispatcherState, subscription)
		object := state.Process.GetObject()
		reason := cmd.Opt1.(string)
		// handle it in a GenStage callback
		e = object.(GenStageBehavior).HandleCancel(state, subscription, reason)
		delete(state.consumers, subscription.Pid)
		return etf.Atom("ok"), e
	}

	return nil, fmt.Errorf("unknown GenStage command (HandleCall)")
}

func handleStageDown(state *GenStageState, down DownMessage) error {
	// remove subscription for producer and consumer. corner case - two
	// processes have subscribed to each other.

	// checking for subscribers (if we act as a producer).
	// we monitor them by Pid only
	if Pid, isPid := down.From.(etf.Pid); isPid {
		if subInternal, ok := state.consumers[Pid]; ok {
			// producer monitors consumer by the Pid and stores monitor reference
			// in the subInternal struct
			state.Process.DemonitorProcess(subInternal.Monitor)
			cmd := stageRequestCommand{
				Cmd:  etf.Atom("cancel"),
				Opt1: down.Reason,
			}
			if _, err := handleProducer(state, subInternal.Subscription, cmd); err != nil {
				return err
			}
		}
	}

	// checking for producers (if we act as a consumer)
	if subInternal, ok := state.producers[down.Ref]; ok {
		cmd := stageRequestCommand{
			Cmd:  etf.Atom("cancel"),
			Opt1: down.Reason,
		}

		if _, err := handleConsumer(state, subInternal.Subscription, cmd); err != nil {
			return err
		}
	}

	return nil
}

// for the consumer side only
func sendDemand(p *Process, producer etf.Term, subscription GenStageSubscription, count uint) {
	msg := etf.Tuple{
		etf.Atom("$gen_producer"),
		etf.Tuple{subscription.Pid, subscription.Ref},
		etf.Tuple{etf.Atom("ask"), count},
	}
	p.Send(producer, msg)
}
