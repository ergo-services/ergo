package ergo

import (
	"fmt"
	"github.com/halturin/ergo/etf"
	"time"
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

	Dispatcher GenStageDispatcherBehaviour
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

// GenStageBehaviour interface for the GenStage inmplementation
type GenStageBehaviour interface {

	// InitStage
	InitStage(process *Process, args ...interface{}) (GenStageOptions, interface{})

	// HandleDemand this callback is invoked on a producer stage
	// The producer that implements this callback must either store the demand, or return the amount of requested events.
	// Use `ergo.ErrStop` as an error for the normal shutdown this process. Any other error values
	// will be used as a reason for the abnornal shutdown process.
	HandleDemand(subscription GenStageSubscription, count uint, state interface{}) (error, etf.List)

	// HandleEvents this callback is invoked on a consumer stage.
	// Use `ergo.ErrStop` as an error for the normal shutdown this process. Any other error values
	// will be used as a reason for the abnornal shutdown process.
	HandleEvents(subscription GenStageSubscription, events etf.List, state interface{}) error

	// HandleSubscribe This callback is invoked on a producer stage.
	// Use `ergo.ErrStop` as an error for the normal shutdown this process. Any other error values
	// will be used as a reason for the abnornal shutdown process.
	HandleSubscribe(subscription GenStageSubscription, options GenStageSubscribeOptions, state interface{}) error

	// HandleSubscribed this callback is invoked as a confirmation for the subscription request
	// Returning false means that demand must be sent to producers explicitly using Ask method.
	// Returning true means the stage implementation will take care of automatically sending.
	// Use `ergo.ErrStop` as an error for the normal shutdown this process. Any other error values
	HandleSubscribed(subscription GenStageSubscription, opts GenStageSubscribeOptions, state interface{}) (error, bool)

	// HandleCancel
	// Invoked when a consumer is no longer subscribed to a producer (invoked on a producer stage)
	// The cancelReason will be a {Cancel: "cancel", Reason: _} if the reason for cancellation
	// was a GenStage.Cancel call. Any other value means the cancellation reason was
	// due to an EXIT.
	// Use `ergo.ErrStop` as an error for the normal shutdown this process. Any other error values
	// will be used as a reason for the abnornal shutdown process.
	HandleCancel(subscription GenStageSubscription, reason string, state interface{}) error

	// HandleCanceled
	// Invoked when a consumer is no longer subscribed to a producer (invoked on a consumer stage)
	// Termination this stage depends on a cancel mode for the given subscription. For the cancel mode
	// GenStageCancelPermanent - this stage will be terminated right after this callback invoking.
	// For the cancel mode GenStageCancelTransient - it depends on a reason of subscription canceling.
	// Cancel mode GenStageCancelTemporary keeps this stage alive whether the reason could be.
	HandleCanceled(subscription GenStageSubscription, reason string, state interface{}) error

	// HandleGenStageCall this callback is invoked on Process.Call. This method is optional
	// for the implementation
	HandleGenStageCall(from etf.Tuple, message etf.Term, state interface{}) (string, etf.Term)
	// HandleGenStageCast this callback is invoked on Process.Cast. This method is optional
	// for the implementation
	HandleGenStageCast(message etf.Term, state interface{}) string
	// HandleGenStageInfo this callback is invoked on Process.Send. This method is optional
	// for the implementation
	HandleGenStageInfo(message etf.Term, state interface{}) string
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
	// demand to producer (as a default behaviour). You can disable it
	// setting ManualDemand to true
	ManualDemand bool `etf:"manual"`
	// What should happend with consumer if producer has terminated
	// GenStageCancelPermanent the consumer exits when the producer cancels or exits.
	// GenStageCancelTransient the consumer exits only if reason is not "normal",
	// "shutdown", or {"shutdown", _}
	// GenStageCancelTemporary the consumer never exits
	Cancel GenStageCancelMode `etf:"cancel"`

	// Partition is defined the number of partition this subscription should belongs to.
	// This option uses in the DispatcherPartition
	Partition uint `etf:"partition"`

	// Extra is intended to be a custom set of options for the custom implementation
	// of GenStageDispatcherBehaviour
	Extra etf.Term `etf:"extra"`
}

type GenStageCancelReason struct {
	Cancel string
	Reason string
}

type GenStage struct {
	GenServer
}

type stateGenStage struct {
	p               *Process
	internal        interface{}
	options         GenStageOptions
	demandBuffer    []demandRequest
	dispatcherState interface{}
	// keep our subscriptions
	// key: string representation of etf.Ref (monitor)
	producers map[string]*subscriptionInternal
	// keep our subscribers
	consumers map[etf.Pid]*subscriptionInternal
}

type stageRequestCommandCancel struct {
	Subscription etf.Ref
	Reason       etf.Term
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

type downMessage struct {
	Down   etf.Atom // = etf.Atom("DOWN")
	Ref    etf.Ref  // a monitor reference
	Type   etf.Atom // = etf.Atom("process")
	From   etf.Term // Pid or Name. Depends on how MonitorProcess was called - by name or by pid
	Reason string
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
// mode can be used when a special behaviour is desired.
func (gst *GenStage) DisableAutoDemand(p *Process, subscription GenStageSubscription) error {
	message := setManualDemand{
		subscription: subscription,
		enable:       false,
	}
	val, err := p.Call(p.Self(), message)
	if err, ok := val.(error); ok {
		return err
	}
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
	val, err := p.Call(p.Self(), message)
	if err, ok := val.(error); ok {
		return err
	}
	return err
}

// EnableForwardDemand enables forwarding messages to the HandleDemand on a producer stage.
// This is default mode for the producer.
func (gst *GenStage) EnableForwardDemand(p *Process) error {
	message := setForwardDemand{
		forward: true,
	}
	val, err := p.Call(p.Self(), message)
	if err, ok := val.(error); ok {
		return err
	}
	return err
}

// DisableForwardDemand disables forwarding messages to the HandleDemand on a producer stage.
// This is useful as a synchronization mechanism, where the demand is accumulated until
// all consumers are subscribed.
func (gst *GenStage) DisableForwardDemand(p *Process) error {
	message := setForwardDemand{
		forward: false,
	}
	val, err := p.Call(p.Self(), message)
	if err, ok := val.(error); ok {
		return err
	}
	return err
}

// SendEvents sends events for the subscribers
func (gst *GenStage) SendEvents(p *Process, events etf.List) error {
	message := sendEvents{
		events: events,
	}
	val, err := p.Call(p.Self(), message)
	if err, ok := val.(error); ok {
		return err
	}
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

	val, err := p.Call(p.Self(), message)
	if err, ok := val.(error); ok {
		return err
	}
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
	val, err := p.Call(p.Self(), message)
	if err, ok := val.(error); ok {
		return err
	}
	return err
}

// Cancel
func (gst *GenStage) Cancel(p *Process, subscription GenStageSubscription, reason string) error {
	message := cancelSubscription{
		subscription: subscription,
		reason:       reason,
	}
	val, err := p.Call(p.Self(), message)
	if err, ok := val.(error); ok {
		return err
	}
	return err
}

//
// GenServer callbacks
//
func (gst *GenStage) Init(p *Process, args ...interface{}) interface{} {
	//var stageOptions GenStageOptions
	state := &stateGenStage{
		producers: make(map[string]*subscriptionInternal),
		consumers: make(map[etf.Pid]*subscriptionInternal),
	}

	state.p = p
	state.options, state.internal = p.object.(GenStageBehaviour).InitStage(p, args)
	if state.options.BufferSize == 0 {
		state.options.BufferSize = defaultDispatcherBufferSize
	}

	// if dispatcher wasn't specified create a default one GenStageDispatcherDemand
	if state.options.Dispatcher == nil {
		state.options.Dispatcher = CreateGenStageDispatcherDemand()
	}

	state.dispatcherState = state.options.Dispatcher.Init(state.options)
	if len(state.options.SubscribeTo) > 0 {
		for _, s := range state.options.SubscribeTo {
			gst.Subscribe(p, s.Producer, s.Options)
		}
	}

	return state
}

func (gst *GenStage) HandleCall(from etf.Tuple, message etf.Term, state interface{}) (string, etf.Term, interface{}) {
	st := state.(*stateGenStage)

	switch m := message.(type) {
	case setManualDemand:
		subInternal, ok := st.producers[m.subscription.Ref.String()]
		if !ok {
			return "reply", fmt.Errorf("unknown subscription"), state
		}
		subInternal.Options.ManualDemand = m.enable
		if subInternal.count < defaultAutoDemandCount && !subInternal.Options.ManualDemand {
			sendDemand(st.p, subInternal.Producer, m.subscription, defaultAutoDemandCount)
			subInternal.count += defaultAutoDemandCount
		}
		return "reply", "ok", state

	case setCancelMode:
		subInternal, ok := st.producers[m.subscription.Ref.String()]
		if !ok {
			return "reply", fmt.Errorf("unknown subscription"), state
		}
		subInternal.Options.Cancel = m.cancel
		return "reply", "ok", state

	case setForwardDemand:
		st.options.DisableForwarding = !m.forward
		if !m.forward {
			return "reply", "ok", state
		}

		// create demand with count = 0, which will be ignored but start
		// the processing of the buffered demands
		msg := etf.Tuple{
			etf.Atom("$gen_producer"),
			etf.Tuple{etf.Pid{}, etf.Ref{}},
			etf.Tuple{etf.Atom("ask"), 0},
		}
		st.p.Send(st.p.Self(), msg)

		return "reply", "ok", state

	case sendEvents:
		var deliver []GenStageDispatchItem
		// dispatch to the subscribers
		deliver = st.options.Dispatcher.Dispatch(m.events, st.dispatcherState)
		if len(deliver) == 0 {
			return "reply", "ok", state
		}
		for d := range deliver {
			msg := etf.Tuple{
				etf.Atom("$gen_consumer"),
				etf.Tuple{deliver[d].subscription.Pid, deliver[d].subscription.Ref},
				deliver[d].events,
			}
			st.p.Send(deliver[d].subscription.Pid, msg)
		}
		return "reply", "ok", state

	case demandRequest:
		subInternal, ok := st.producers[m.subscription.Ref.String()]
		if !ok {
			return "reply", fmt.Errorf("unknown subscription"), state
		}
		if !subInternal.Options.ManualDemand {
			return "reply", fmt.Errorf("auto demand"), state
		}

		sendDemand(st.p, subInternal.Producer, m.subscription, m.count)
		subInternal.count += int(m.count)
		return "reply", "ok", state

	case cancelSubscription:
		// if we act as a consumer with this subscription
		if subInternal, ok := st.producers[m.subscription.Ref.String()]; ok {
			msg := etf.Tuple{
				etf.Atom("$gen_producer"),
				etf.Tuple{m.subscription.Pid, m.subscription.Ref},
				etf.Tuple{etf.Atom("cancel"), m.reason},
			}
			st.p.Send(subInternal.Producer, msg)
			cmd := stageRequestCommand{
				Cmd:  etf.Atom("cancel"),
				Opt1: "normal",
			}
			if _, err := handleConsumer(subInternal.Subscription, cmd, st); err != nil {
				return "reply", err, state
			}
			return "reply", "ok", state
		}
		// if we act as a producer within this subscription
		if subInternal, ok := st.consumers[m.subscription.Pid]; ok {
			msg := etf.Tuple{
				etf.Atom("$gen_consumer"),
				etf.Tuple{m.subscription.Pid, m.subscription.Ref},
				etf.Tuple{etf.Atom("cancel"), m.reason},
			}
			st.p.Send(m.subscription.Pid, msg)
			st.p.DemonitorProcess(subInternal.Monitor)
			cmd := stageRequestCommand{
				Cmd:  etf.Atom("cancel"),
				Opt1: "normal",
			}
			if _, err := handleProducer(subInternal.Subscription, cmd, st); err != nil {
				return "reply", err, st
			}
			return "reply", "ok", state
		}
		return "reply", fmt.Errorf("unknown subscription"), state

	default:
		reply, term := st.p.object.(GenStageBehaviour).HandleGenStageCall(from, message, st.internal)
		return reply, term, state
	}

	return "reply", "ok", state
}

func (gst *GenStage) HandleCast(message etf.Term, state interface{}) (string, interface{}) {
	st := state.(*stateGenStage)
	reply := st.p.object.(GenStageBehaviour).HandleGenStageCast(message, st.internal)

	return reply, state
}

func (gst *GenStage) HandleInfo(message etf.Term, state interface{}) (string, interface{}) {
	var r stageMessage
	var d downMessage
	var err error

	st := state.(*stateGenStage)

	// check if we got a 'DOWN' message
	// {DOWN, Ref, process, PidOrName, Reason}
	if err := etf.TermIntoStruct(message, &d); err == nil && d.Down == etf.Atom("DOWN") {
		if err1 := handleDown(d, st); err1 != nil {
			return "stop", err1.Error()
		}
		return "noreply", state
	}

	if err := etf.TermIntoStruct(message, &r); err != nil {
		reply := st.p.object.(GenStageBehaviour).HandleGenStageInfo(message, st.internal)
		return reply, state
	}

	_, err = handleRequest(r, st)

	switch err {
	case nil:
		return "noreply", state
	case ErrStop:
		return "stop", "normal"
	case ErrUnsupportedRequest:
		reply := st.p.object.(GenStageBehaviour).HandleGenStageInfo(message, st.internal)
		return reply, state
	default:
		return "stop", err.Error()
	}

	return "noreply", state
}

func (gst *GenStage) Terminate(reason string, state interface{}) {
	return
}

// default callbacks

func (gst *GenStage) InitStage(process *Process, args ...interface{}) (GenStageOptions, interface{}) {
	// GenStage initialization with default options
	opts := GenStageOptions{}
	return opts, nil
}

func (gst *GenStage) HandleGenStageCall(from etf.Tuple, message etf.Term, state interface{}) (string, etf.Term) {
	// default callback if it wasn't implemented
	fmt.Printf("HandleGenStageCall: unhandled message (from %#v) %#v\n", from, message)
	return "reply", etf.Atom("ok")
}

func (gst *GenStage) HandleGenStageCast(message etf.Term, state interface{}) string {
	// default callback if it wasn't implemented
	fmt.Printf("HandleGenStageCast: unhandled message %#v\n", message)
	return "noreply"
}
func (gst *GenStage) HandleGenStageInfo(message etf.Term, state interface{}) string {
	// default callback if it wasn't implemnted
	fmt.Printf("HandleGenStageInfo: unhandled message %#v\n", message)
	return "noreply"
}

func (gst *GenStage) HandleSubscribe(subscription GenStageSubscription, options GenStageSubscribeOptions,
	state interface{}) error {
	return ErrNotAProducer
}

func (gst *GenStage) HandleSubscribed(subscription GenStageSubscription, opts GenStageSubscribeOptions, state interface{}) (error, bool) {
	return nil, opts.ManualDemand
}

func (gst *GenStage) HandleCancel(subscription GenStageSubscription, reason string, state interface{}) error {
	// default callback if it wasn't implemented
	return nil
}

func (gst *GenStage) HandleCanceled(subscription GenStageSubscription, reason string, state interface{}) error {
	// default callback if it wasn't implemented
	return nil
}

func (gst *GenStage) HandleEvents(subscription GenStageSubscription, events etf.List, state interface{}) error {
	fmt.Printf("GenStage HandleEvents: unhandled subscription (%#v) events %#v\n", subscription, events)
	return nil
}

func (gst *GenStage) HandleDemand(subscription GenStageSubscription, count uint, state interface{}) (error, etf.List) {
	fmt.Printf("GenStage HandleDemand: unhandled subscription (%#v) demand %#v\n", subscription, count)
	return nil, nil
}

// private functions

func handleRequest(m stageMessage, state *stateGenStage) (etf.Term, error) {
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
			return handleConsumer(m.Subscription, command, state)
		}
		if err := etf.TermIntoStruct(m.Command, &command); err != nil {
			return nil, ErrUnsupportedRequest
		}
		return handleConsumer(m.Subscription, command, state)
	case "$gen_producer":
		if err := etf.TermIntoStruct(m.Command, &command); err != nil {
			return nil, ErrUnsupportedRequest
		}
		return handleProducer(m.Subscription, command, state)
	}
	return nil, ErrUnsupportedRequest
}

func handleConsumer(subscription GenStageSubscription, cmd stageRequestCommand, state *stateGenStage) (etf.Term, error) {
	var subscriptionOpts GenStageSubscribeOptions
	var err error
	var manualDemand bool

	switch cmd.Cmd {
	case etf.Atom("events"):
		events := cmd.Opt1.(etf.List)
		object := state.p.object
		numEvents := len(events)

		subInternal, ok := state.producers[subscription.Ref.String()]
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

		err = object.(GenStageBehaviour).HandleEvents(subscription, events, state.internal)
		if err != nil {
			return nil, err
		}

		// if subscription has auto demand we should request yet another
		// bunch of events
		if subInternal.count < defaultAutoDemandCount && !subInternal.Options.ManualDemand {
			sendDemand(state.p, subInternal.Producer, subscription, defaultAutoDemandCount)
			subInternal.count += defaultAutoDemandCount
		}
		return etf.Atom("ok"), nil

	case etf.Atom("subscribed"):
		if err := etf.TermProplistIntoStruct(cmd.Opt2, &subscriptionOpts); err != nil {
			return nil, err
		}

		object := state.p.object
		err, manualDemand = object.(GenStageBehaviour).HandleSubscribed(subscription, subscriptionOpts, state.internal)

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
		state.producers[subscription.Ref.String()] = subInternal

		if !manualDemand {
			sendDemand(state.p, producer, subscription, defaultAutoDemandCount)
			subInternal.count = defaultAutoDemandCount
		}

		return etf.Atom("ok"), nil

	case etf.Atom("retry-cancel"):
		// if "subscribed" message hasn't still arrived then just ignore it
		if _, ok := state.producers[subscription.Ref.String()]; !ok {
			return etf.Atom("ok"), nil
		}
		fallthrough
	case etf.Atom("cancel"):
		// the subscription was canceled
		reason, ok := cmd.Opt1.(string)
		if !ok {
			return nil, fmt.Errorf("Cancel reason is not a string")
		}

		subInternal, ok := state.producers[subscription.Ref.String()]
		if !ok {
			// There might be a case when "cancel" message arrives before
			// the "subscribed" message due to async nature of messaging,
			// so we should wait a bit and try to handle it one more time
			// using "retry-cancel" message.
			// I got this with problem with GOMAXPROCS=1
			msg := etf.Tuple{
				etf.Atom("$gen_consumer"),
				etf.Tuple{subscription.Pid, subscription.Ref},
				etf.Tuple{etf.Atom("retry-cancel"), reason},
			}
			// handle it in a second
			state.p.SendAfter(state.p.Self(), msg, 200*time.Millisecond)
			return etf.Atom("ok"), nil
		}

		state.p.DemonitorProcess(subscription.Ref)

		object := state.p.object
		err = object.(GenStageBehaviour).HandleCanceled(subscription, reason, state.internal)
		if err != nil {
			return nil, err
		}

		delete(state.producers, subscription.Ref.String())

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

func handleProducer(subscription GenStageSubscription, cmd stageRequestCommand, state *stateGenStage) (etf.Term, error) {
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
			state.p.Send(subscription.Pid, msg)
			return etf.Atom("ok"), nil
		}

		object := state.p.object
		err = object.(GenStageBehaviour).HandleSubscribe(subscription, subscriptionOpts, state.internal)

		switch err {
		case nil:
			// cancel current subscription if this consumer has been already subscribed
			if s, ok := state.consumers[subscription.Pid]; ok {
				msg := etf.Tuple{
					etf.Atom("$gen_consumer"),
					etf.Tuple{subscription.Pid, s.Subscription.Ref},
					etf.Tuple{etf.Atom("cancel"), "resubscribed"},
				}
				state.p.Send(subscription.Pid, msg)
				// notify dispatcher about cancelation the previous subscription
				canceledSubscription := GenStageSubscription{
					Pid: subscription.Pid,
					Ref: s.Subscription.Ref,
				}
				// cancel current demands
				state.options.Dispatcher.Cancel(canceledSubscription, state.dispatcherState)
				// notify dispatcher about the new subscription
				if err := state.options.Dispatcher.Subscribe(subscription, subscriptionOpts, state.dispatcherState); err != nil {
					// dispatcher can't handle this subscription
					msg := etf.Tuple{
						etf.Atom("$gen_consumer"),
						etf.Tuple{subscription.Pid, s.Subscription.Ref},
						etf.Tuple{etf.Atom("cancel"), err.Error()},
					}
					state.p.Send(subscription.Pid, msg)
					return etf.Atom("ok"), nil
				}

				s.Subscription = subscription
				return etf.Atom("ok"), nil
			}

			if err := state.options.Dispatcher.Subscribe(subscription, subscriptionOpts, state.dispatcherState); err != nil {
				// dispatcher can't handle this subscription
				msg := etf.Tuple{
					etf.Atom("$gen_consumer"),
					etf.Tuple{subscription.Pid, subscription.Ref},
					etf.Tuple{etf.Atom("cancel"), err.Error()},
				}
				state.p.Send(subscription.Pid, msg)
				return etf.Atom("ok"), nil
			}

			// monitor subscriber in order to remove this subscription
			// if it terminated unexpectedly
			m := state.p.MonitorProcess(subscription.Pid)
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
			state.p.Send(subscription.Pid, msg)
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
			state.p.Send(subscription.Pid, msg)
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
			if state.options.DisableForwarding {
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
			state.p.Send(state.p.Self(), msg)
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
			state.p.SendAfter(state.p.Self(), msg, 1*time.Second)
			return etf.Atom("ok"), nil
		}

		if state.options.DisableForwarding {
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

		object := state.p.object
		_, events = object.(GenStageBehaviour).HandleDemand(subscription, count, state.internal)

		// register this demand and trying to dispatch having events
		dispatcher := state.options.Dispatcher
		dispatcher.Ask(subscription, count, state.dispatcherState)
		deliver = dispatcher.Dispatch(events, state.dispatcherState)
		if len(deliver) == 0 {
			return etf.Atom("ok"), nil
		}

		for d := range deliver {
			msg := etf.Tuple{
				etf.Atom("$gen_consumer"),
				etf.Tuple{deliver[d].subscription.Pid, deliver[d].subscription.Ref},
				deliver[d].events,
			}
			state.p.Send(deliver[d].subscription.Pid, msg)
		}

		return etf.Atom("ok"), nil

	case etf.Atom("cancel"):
		var e error
		// handle this cancelation in the dispatcher
		dispatcher := state.options.Dispatcher
		dispatcher.Cancel(subscription, state.dispatcherState)
		object := state.p.object
		reason := cmd.Opt1.(string)
		// handle it in a GenStage callback
		e = object.(GenStageBehaviour).HandleCancel(subscription, reason, state.internal)
		delete(state.consumers, subscription.Pid)
		return etf.Atom("ok"), e
	}

	return nil, fmt.Errorf("unknown GenStage command (HandleCall)")
}

func handleDown(down downMessage, state *stateGenStage) error {
	// remove subscription for producer and consumer. corner case - two
	// processes have subscribed to each other.

	// checking for subscribers (if we act as a producer).
	// we monitor them by Pid only
	if Pid, isPid := down.From.(etf.Pid); isPid {
		if subInternal, ok := state.consumers[Pid]; ok {
			// producer monitors consumer by the Pid and stores monitor reference
			// in the subInternal struct
			state.p.DemonitorProcess(subInternal.Monitor)
			cmd := stageRequestCommand{
				Cmd:  etf.Atom("cancel"),
				Opt1: down.Reason,
			}
			if _, err := handleProducer(subInternal.Subscription, cmd, state); err != nil {
				return err
			}
		}
	}

	// checking for producers (if we act as a consumer)
	if subInternal, ok := state.producers[down.Ref.String()]; ok {
		cmd := stageRequestCommand{
			Cmd:  etf.Atom("cancel"),
			Opt1: down.Reason,
		}

		if _, err := handleConsumer(subInternal.Subscription, cmd, state); err != nil {
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
