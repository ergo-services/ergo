package gen

import (
	"fmt"
	"time"

	"github.com/halturin/ergo/etf"
	//"github.com/halturin/ergo/lib"
)

type StageCancelMode uint

// StageOptions defines the Stage' configuration using Init callback.
// Some options are specific to the chosen stage mode while others are
// shared across all types.
type StageOptions struct {

	// If this stage acts as a consumer you can to define producers
	// this stage should subscribe to.
	// SubscribeTo is a list of StageSubscribeTo. Each element represents
	// a producer (etf.Pid or registered name) and subscription options.
	SubscribeTo []StageSubscribeTo

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

	Dispatcher StageDispatcherBehavior
}

const (
	StageCancelPermanent StageCancelMode = 0
	StageCancelTransient StageCancelMode = 1
	StageCancelTemporary StageCancelMode = 2

	defaultDispatcherBufferSize = 10000
	defaultAutoDemandCount      = 3
)

var (
	ErrNotAProducer = fmt.Errorf("not a producer")
)

// StageBehavior interface for the Stage inmplementation
type StageBehavior interface {

	// InitStage
	InitStage(process *StageProcess, args ...etf.Term) error

	// HandleDemand this callback is invoked on a producer stage
	// The producer that implements this callback must either store the demand, or return the amount of requested events.
	// Use `ergo.ErrStop` as an error for the normal shutdown this process. Any other error values
	// will be used as a reason for the abnornal shutdown process.
	HandleDemand(process *StageProcess, subscription StageSubscription, count uint) (error, etf.List)

	// HandleEvents this callback is invoked on a consumer stage.
	// Use `ergo.ErrStop` as an error for the normal shutdown this process. Any other error values
	// will be used as a reason for the abnornal shutdown process.
	HandleEvents(process *StageProcess, subscription StageSubscription, events etf.List) error

	// HandleSubscribe This callback is invoked on a producer stage.
	// Use `ergo.ErrStop` as an error for the normal shutdown this process. Any other error values
	// will be used as a reason for the abnornal shutdown process.
	HandleSubscribe(process *StageProcess, subscription StageSubscription, options StageSubscribeOptions) error

	// HandleSubscribed this callback is invoked as a confirmation for the subscription request
	// Returning false means that demand must be sent to producers explicitly using Ask method.
	// Returning true means the stage implementation will take care of automatically sending.
	// Use `ergo.ErrStop` as an error for the normal shutdown this process. Any other error values
	HandleSubscribed(process *StageProcess, subscription StageSubscription, opts StageSubscribeOptions) (error, bool)

	// HandleCancel
	// Invoked when a consumer is no longer subscribed to a producer (invoked on a producer stage)
	// The cancelReason will be a {Cancel: "cancel", Reason: _} if the reason for cancellation
	// was a Stage.Cancel call. Any other value means the cancellation reason was
	// due to an EXIT.
	// Use `ergo.ErrStop` as an error for the normal shutdown this process. Any other error values
	// will be used as a reason for the abnornal shutdown process.
	HandleCancel(process *StageProcess, subscription StageSubscription, reason string) error

	// HandleCanceled
	// Invoked when a consumer is no longer subscribed to a producer (invoked on a consumer stage)
	// Termination this stage depends on a cancel mode for the given subscription. For the cancel mode
	// StageCancelPermanent - this stage will be terminated right after this callback invoking.
	// For the cancel mode StageCancelTransient - it depends on a reason of subscription canceling.
	// Cancel mode StageCancelTemporary keeps this stage alive whether the reason could be.
	HandleCanceled(process *StageProcess, subscription StageSubscription, reason string) error

	// HandleStageCall this callback is invoked on Process.Call. This method is optional
	// for the implementation
	HandleStageCall(process *StageProcess, from ServerFrom, message etf.Term) (string, etf.Term)
	// HandleStageCast this callback is invoked on Process.Cast. This method is optional
	// for the implementation
	HandleStageCast(process *StageProcess, message etf.Term) string
	// HandleStageInfo this callback is invoked on Process.Send. This method is optional
	// for the implementation
	HandleStageInfo(process *StageProcess, message etf.Term) string
}

type StageSubscription struct {
	Pid etf.Pid
	Ref etf.Ref
}

type subscriptionInternal struct {
	Producer     etf.Term
	Subscription StageSubscription
	Options      StageSubscribeOptions
	Monitor      etf.Ref
	// number of event requests (demands) made as a consumer.
	count int
}

type StageSubscribeTo struct {
	Producer etf.Term
	Options  StageSubscribeOptions
}

type StageSubscribeOptions struct {
	MinDemand uint `etf:"min_demand"`
	MaxDemand uint `etf:"max_demand"`
	// The stage implementation will take care of automatically sending
	// demand to producer (as a default behavior). You can disable it
	// setting ManualDemand to true
	ManualDemand bool `etf:"manual"`
	// What should happened with consumer if producer has terminated
	// StageCancelPermanent the consumer exits when the producer cancels or exits.
	// StageCancelTransient the consumer exits only if reason is not "normal",
	// "shutdown", or {"shutdown", _}
	// StageCancelTemporary the consumer never exits
	Cancel StageCancelMode `etf:"cancel"`

	// Partition is defined the number of partition this subscription should belongs to.
	// This option uses in the DispatcherPartition
	Partition uint `etf:"partition"`

	// Extra is intended to be a custom set of options for the custom implementation
	// of StageDispatcherBehavior
	Extra etf.Term `etf:"extra"`
}

type StageCancelReason struct {
	Cancel string
	Reason string
}

type Stage struct {
	Server
}

type StageProcess struct {
	ServerProcess

	Options         StageOptions
	demandBuffer    []demandRequest
	dispatcherState interface{}
	// keep our subscriptions
	producers map[etf.Ref]*subscriptionInternal
	// keep our subscribers
	consumers map[etf.Pid]*subscriptionInternal
	//
	behavior StageBehavior
}

type stageRequestCommand struct {
	Cmd  etf.Atom
	Opt1 interface{}
	Opt2 interface{}
}

type stageMessage struct {
	Request      etf.Atom
	Subscription StageSubscription
	Command      interface{}
}

type setManualDemand struct {
	subscription StageSubscription
	enable       bool
}

type setCancelMode struct {
	subscription StageSubscription
	cancel       StageCancelMode
}

type doSubscribe struct {
	to      etf.Term
	options StageSubscribeOptions
}

type setForwardDemand struct {
	forward bool
}

type demandRequest struct {
	subscription StageSubscription
	count        uint
}

type cancelSubscription struct {
	subscription StageSubscription
	reason       string
}

type sendEvents struct {
	events etf.List
}

// Stage methods

// DisableAutoDemand means that demand must be sent to producers explicitly using Ask method. This
// mode can be used when a special behavior is desired.
func (s *Stage) DisableAutoDemand(p Process, subscription StageSubscription) error {
	message := setManualDemand{
		subscription: subscription,
		enable:       false,
	}
	_, err := p.Direct(message)
	return err
}

// EnableAutoDemand enables auto demand mode (this is default mode for the consumer).
func (s *Stage) EnableAutoDemand(p Process, subscription StageSubscription) error {
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
func (s *Stage) EnableForwardDemand(p Process) error {
	message := setForwardDemand{
		forward: true,
	}
	_, err := p.Direct(message)
	return err
}

// DisableForwardDemand disables forwarding messages to the HandleDemand on a producer stage.
// This is useful as a synchronization mechanism, where the demand is accumulated until
// all consumers are subscribed.
func (s *Stage) DisableForwardDemand(p Process) error {
	message := setForwardDemand{
		forward: false,
	}
	_, err := p.Direct(message)
	return err
}

// SendEvents sends events for the subscribers
func (s *Stage) SendEvents(p Process, events etf.List) error {
	message := sendEvents{
		events: events,
	}
	_, err := p.Direct(message)
	return err
}

// SetCancelMode defines how consumer will handle termination of the producer. There are 3 modes:
// StageCancelPermanent (default) - consumer exits when the producer cancels or exits
// StageCancelTransient - consumer exits only if reason is not normal, shutdown, or {shutdown, reason}
// StageCancelTemporary - never exits
func (s *Stage) SetCancelMode(p Process, subscription StageSubscription, cancel StageCancelMode) error {
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
func (s *Stage) Subscribe(p Process, producer etf.Term, opts StageSubscribeOptions) StageSubscription {
	var subscription StageSubscription

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
func (s *Stage) Ask(p Process, subscription StageSubscription, count uint) error {
	message := demandRequest{
		subscription: subscription,
		count:        count,
	}
	_, err := p.Direct(message)
	return err
}

// Cancel
func (s *Stage) Cancel(p Process, subscription StageSubscription, reason string) error {
	message := cancelSubscription{
		subscription: subscription,
		reason:       reason,
	}
	_, err := p.Direct(message)
	return err
}

//
// gen.Server callbacks
//
func (gst *Stage) Init(process *ServerProcess, args ...etf.Term) error {
	//var stageOptions StageOptions

	stageProcess := &StageProcess{
		ServerProcess: *process,
		producers:     make(map[etf.Ref]*subscriptionInternal),
		consumers:     make(map[etf.Pid]*subscriptionInternal),
	}

	behavior, ok := process.ProcessBehavior().(StageBehavior)
	if !ok {
		return fmt.Errorf("Stage: not a StageBehavior")
	}
	stageProcess.behavior = behavior

	if err := behavior.InitStage(stageProcess, args); err != nil {
		return err
	}

	if stageProcess.Options.BufferSize == 0 {
		stageProcess.Options.BufferSize = defaultDispatcherBufferSize
	}

	// if dispatcher wasn't specified create a default one StageDispatcherDemand
	if stageProcess.Options.Dispatcher == nil {
		stageProcess.Options.Dispatcher = CreateStageDispatcherDemand()
	}

	stageProcess.dispatcherState = stageProcess.Options.Dispatcher.Init(stageProcess.Options)
	if len(stageProcess.Options.SubscribeTo) > 0 {
		for _, s := range stageProcess.Options.SubscribeTo {
			gst.Subscribe(process, s.Producer, s.Options)
		}
	}

	process.State = stageProcess
	return nil
}

func (gst *Stage) HandleCall(process *ServerProcess, from ServerFrom, message etf.Term) (string, etf.Term) {
	stageProcess := process.State.(*StageProcess)
	return stageProcess.behavior.HandleStageCall(stageProcess, from, message)
}

func (gst *Stage) HandleDirect(process *ServerProcess, message interface{}) (interface{}, error) {
	stageProcess := process.State.(*StageProcess)
	switch m := message.(type) {
	case setManualDemand:
		subInternal, ok := stageProcess.producers[m.subscription.Ref]
		if !ok {
			return nil, fmt.Errorf("unknown subscription")
		}
		subInternal.Options.ManualDemand = m.enable
		if subInternal.count < defaultAutoDemandCount && !subInternal.Options.ManualDemand {
			sendDemand(process, subInternal.Producer, m.subscription, defaultAutoDemandCount)
			subInternal.count += defaultAutoDemandCount
		}
		return nil, nil

	case setCancelMode:
		subInternal, ok := stageProcess.producers[m.subscription.Ref]
		if !ok {
			return nil, fmt.Errorf("unknown subscription")
		}
		subInternal.Options.Cancel = m.cancel
		return nil, nil

	case setForwardDemand:
		stageProcess.Options.DisableForwarding = !m.forward
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
		process.Send(process.Self(), msg)

		return nil, nil

	case sendEvents:
		var deliver []StageDispatchItem
		// dispatch to the subscribers
		deliver = stageProcess.Options.Dispatcher.Dispatch(stageProcess.dispatcherState, m.events)
		if len(deliver) == 0 {
			return nil, nil
		}
		for d := range deliver {
			msg := etf.Tuple{
				etf.Atom("$gen_consumer"),
				etf.Tuple{deliver[d].subscription.Pid, deliver[d].subscription.Ref},
				deliver[d].events,
			}
			process.Send(deliver[d].subscription.Pid, msg)
		}
		return nil, nil

	case demandRequest:
		subInternal, ok := stageProcess.producers[m.subscription.Ref]
		if !ok {
			return nil, fmt.Errorf("unknown subscription")
		}
		if !subInternal.Options.ManualDemand {
			return nil, fmt.Errorf("auto demand")
		}

		sendDemand(process, subInternal.Producer, m.subscription, m.count)
		subInternal.count += int(m.count)
		return nil, nil

	case cancelSubscription:
		// if we act as a consumer with this subscription
		if subInternal, ok := stageProcess.producers[m.subscription.Ref]; ok {
			msg := etf.Tuple{
				etf.Atom("$gen_producer"),
				etf.Tuple{m.subscription.Pid, m.subscription.Ref},
				etf.Tuple{etf.Atom("cancel"), m.reason},
			}
			process.Send(subInternal.Producer, msg)
			cmd := stageRequestCommand{
				Cmd:  etf.Atom("cancel"),
				Opt1: "normal",
			}
			if _, err := handleConsumer(stageProcess, subInternal.Subscription, cmd); err != nil {
				return nil, err
			}
			return nil, nil
		}
		// if we act as a producer within this subscription
		if subInternal, ok := stageProcess.consumers[m.subscription.Pid]; ok {
			msg := etf.Tuple{
				etf.Atom("$gen_consumer"),
				etf.Tuple{m.subscription.Pid, m.subscription.Ref},
				etf.Tuple{etf.Atom("cancel"), m.reason},
			}
			process.Send(m.subscription.Pid, msg)
			process.DemonitorProcess(subInternal.Monitor)
			cmd := stageRequestCommand{
				Cmd:  etf.Atom("cancel"),
				Opt1: "normal",
			}
			if _, err := handleProducer(stageProcess, subInternal.Subscription, cmd); err != nil {
				return nil, err
			}
			return nil, nil
		}
		return nil, fmt.Errorf("unknown subscription")

	default:
		return nil, ErrUnsupportedRequest
	}

}

func (gst *Stage) HandleCast(process *ServerProcess, message etf.Term) string {
	stageProcess := process.State.(*StageProcess)
	return stageProcess.behavior.HandleStageCast(stageProcess, message)
}

func (gst *Stage) HandleInfo(process *ServerProcess, message etf.Term) string {
	var r stageMessage

	stageProcess := process.State.(*StageProcess)

	// check if we got a 'DOWN' message
	// {DOWN, Ref, process, PidOrName, Reason}
	if isDown, d := IsDownMessage(message); isDown {
		if err := handleStageDown(stageProcess, d); err != nil {
			return err.Error()
		}
		return "noreply"
	}

	if err := etf.TermIntoStruct(message, &r); err != nil {
		reply := stageProcess.behavior.HandleStageInfo(stageProcess, message)
		return reply
	}

	_, err := handleStageRequest(stageProcess, r)

	switch err {
	case nil:
		return "noreply"
	case ErrStop:
		return "stop"
	case ErrUnsupportedRequest:
		reply := stageProcess.behavior.HandleStageInfo(stageProcess, message)
		return reply
	default:
		return err.Error()
	}
}

// default callbacks

func (gst *Stage) InitStage(process *StageProcess, args ...etf.Term) error {
	return nil
}

func (gst *Stage) HandleStageCall(process *StageProcess, from ServerFrom, message etf.Term) (string, etf.Term) {
	// default callback if it wasn't implemented
	fmt.Printf("HandleStageCall: unhandled message (from %#v) %#v\n", from, message)
	return "reply", etf.Atom("ok")
}

func (gst *Stage) HandleStageCast(process *StageProcess, message etf.Term) string {
	// default callback if it wasn't implemented
	fmt.Printf("HandleStageCast: unhandled message %#v\n", message)
	return "noreply"
}
func (gst *Stage) HandleStageInfo(process *StageProcess, message etf.Term) string {
	// default callback if it wasn't implemnted
	fmt.Printf("HandleStageInfo: unhandled message %#v\n", message)
	return "noreply"
}

func (gst *Stage) HandleSubscribe(process *StageProcess, subscription StageSubscription, options StageSubscribeOptions) error {
	return ErrNotAProducer
}

func (gst *Stage) HandleSubscribed(process *StageProcess, subscription StageSubscription, opts StageSubscribeOptions) (error, bool) {
	return nil, opts.ManualDemand
}

func (gst *Stage) HandleCancel(process *StageProcess, subscription StageSubscription, reason string) error {
	// default callback if it wasn't implemented
	return nil
}

func (gst *Stage) HandleCanceled(process *StageProcess, subscription StageSubscription, reason string) error {
	// default callback if it wasn't implemented
	return nil
}

func (gst *Stage) HandleEvents(process *StageProcess, subscription StageSubscription, events etf.List) error {
	fmt.Printf("Stage HandleEvents: unhandled subscription (%#v) events %#v\n", subscription, events)
	return nil
}

func (gst *Stage) HandleDemand(process *StageProcess, subscription StageSubscription, count uint) (error, etf.List) {
	fmt.Printf("Stage HandleDemand: unhandled subscription (%#v) demand %#v\n", subscription, count)
	return nil, nil
}

// private functions

func handleStageRequest(process *StageProcess, m stageMessage) (etf.Term, error) {
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
			return handleConsumer(process, m.Subscription, command)
		}
		if err := etf.TermIntoStruct(m.Command, &command); err != nil {
			return nil, ErrUnsupportedRequest
		}
		return handleConsumer(process, m.Subscription, command)
	case "$gen_producer":
		if err := etf.TermIntoStruct(m.Command, &command); err != nil {
			return nil, ErrUnsupportedRequest
		}
		return handleProducer(process, m.Subscription, command)
	}
	return nil, ErrUnsupportedRequest
}

func handleConsumer(process *StageProcess, subscription StageSubscription, cmd stageRequestCommand) (etf.Term, error) {
	var subscriptionOpts StageSubscribeOptions
	var err error
	var manualDemand bool

	switch cmd.Cmd {
	case etf.Atom("events"):
		events := cmd.Opt1.(etf.List)
		numEvents := len(events)

		subInternal, ok := process.producers[subscription.Ref]
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

		err = process.behavior.HandleEvents(process, subscription, events)
		if err != nil {
			return nil, err
		}

		// if subscription has auto demand we should request yet another
		// bunch of events
		if subInternal.count < defaultAutoDemandCount && !subInternal.Options.ManualDemand {
			sendDemand(process, subInternal.Producer, subscription, defaultAutoDemandCount)
			subInternal.count += defaultAutoDemandCount
		}
		return etf.Atom("ok"), nil

	case etf.Atom("subscribed"):
		if err := etf.TermProplistIntoStruct(cmd.Opt2, &subscriptionOpts); err != nil {
			return nil, err
		}

		err, manualDemand = process.behavior.HandleSubscribed(process, subscription, subscriptionOpts)

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
		process.producers[subscription.Ref] = subInternal

		if !manualDemand {
			sendDemand(process, producer, subscription, defaultAutoDemandCount)
			subInternal.count = defaultAutoDemandCount
		}

		return etf.Atom("ok"), nil

	case etf.Atom("retry-cancel"):
		// if "subscribed" message hasn't still arrived then just ignore it
		if _, ok := process.producers[subscription.Ref]; !ok {
			return etf.Atom("ok"), nil
		}
		fallthrough
	case etf.Atom("cancel"):
		// the subscription was canceled
		reason, ok := cmd.Opt1.(string)
		if !ok {
			return nil, fmt.Errorf("Cancel reason is not a string")
		}

		subInternal, ok := process.producers[subscription.Ref]
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
			process.SendAfter(process.Self(), msg, 200*time.Millisecond)
			return etf.Atom("ok"), nil
		}

		process.DemonitorProcess(subscription.Ref)

		err = process.behavior.HandleCanceled(process, subscription, reason)
		if err != nil {
			return nil, err
		}

		delete(process.producers, subscription.Ref)

		switch subInternal.Options.Cancel {
		case StageCancelTemporary:
			return etf.Atom("ok"), nil
		case StageCancelTransient:
			if reason == "normal" || reason == "shutdown" {
				return etf.Atom("ok"), nil
			}
			return nil, fmt.Errorf(reason)
		default:
			// StageCancelPermanent
			return nil, fmt.Errorf(reason)
		}
	}

	return nil, fmt.Errorf("unknown Stage command (HandleCast)")
}

func handleProducer(process *StageProcess, subscription StageSubscription, cmd stageRequestCommand) (etf.Term, error) {
	var subscriptionOpts StageSubscribeOptions
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
			process.Send(subscription.Pid, msg)
			return etf.Atom("ok"), nil
		}

		err = process.behavior.HandleSubscribe(process, subscription, subscriptionOpts)

		switch err {
		case nil:
			// cancel current subscription if this consumer has been already subscribed
			if s, ok := process.consumers[subscription.Pid]; ok {
				msg := etf.Tuple{
					etf.Atom("$gen_consumer"),
					etf.Tuple{subscription.Pid, s.Subscription.Ref},
					etf.Tuple{etf.Atom("cancel"), "resubscribed"},
				}
				process.Send(subscription.Pid, msg)
				// notify dispatcher about cancelation the previous subscription
				canceledSubscription := StageSubscription{
					Pid: subscription.Pid,
					Ref: s.Subscription.Ref,
				}
				// cancel current demands
				process.Options.Dispatcher.Cancel(process.dispatcherState, canceledSubscription)
				// notify dispatcher about the new subscription
				if err := process.Options.Dispatcher.Subscribe(process.dispatcherState, subscription, subscriptionOpts); err != nil {
					// dispatcher can't handle this subscription
					msg := etf.Tuple{
						etf.Atom("$gen_consumer"),
						etf.Tuple{subscription.Pid, s.Subscription.Ref},
						etf.Tuple{etf.Atom("cancel"), err.Error()},
					}
					process.Send(subscription.Pid, msg)
					return etf.Atom("ok"), nil
				}

				s.Subscription = subscription
				return etf.Atom("ok"), nil
			}

			if err := process.Options.Dispatcher.Subscribe(process.dispatcherState, subscription, subscriptionOpts); err != nil {
				// dispatcher can't handle this subscription
				msg := etf.Tuple{
					etf.Atom("$gen_consumer"),
					etf.Tuple{subscription.Pid, subscription.Ref},
					etf.Tuple{etf.Atom("cancel"), err.Error()},
				}
				process.Send(subscription.Pid, msg)
				return etf.Atom("ok"), nil
			}

			// monitor subscriber in order to remove this subscription
			// if it terminated unexpectedly
			m := process.MonitorProcess(subscription.Pid)
			s := &subscriptionInternal{
				Subscription: subscription,
				Monitor:      m,
				Options:      subscriptionOpts,
			}
			process.consumers[subscription.Pid] = s
			return etf.Atom("ok"), nil

		case ErrNotAProducer:
			// if it wasnt overloaded - send 'cancel' to the consumer
			msg := etf.Tuple{
				etf.Atom("$gen_consumer"),
				etf.Tuple{subscription.Pid, subscription.Ref},
				etf.Tuple{etf.Atom("cancel"), err.Error()},
			}
			process.Send(subscription.Pid, msg)
			return etf.Atom("ok"), nil

		default:
			// any other error should terminate this stage
			return nil, err
		}
	case etf.Atom("retry-ask"):
		// if "subscribe" message hasn't still arrived, send a cancelation message
		// to the consumer
		if _, ok := process.consumers[subscription.Pid]; !ok {
			msg := etf.Tuple{
				etf.Atom("$gen_consumer"),
				etf.Tuple{subscription.Pid, subscription.Ref},
				etf.Tuple{etf.Atom("cancel"), "not subscribed"},
			}
			process.Send(subscription.Pid, msg)
			return etf.Atom("ok"), nil
		}
		fallthrough

	case etf.Atom("ask"):
		var events etf.List
		var deliver []StageDispatchItem
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
			if process.Options.DisableForwarding {
				return
			}
			if len(process.demandBuffer) == 0 {
				return
			}
			d := process.demandBuffer[0]
			msg := etf.Tuple{
				etf.Atom("$gen_producer"),
				etf.Tuple{d.subscription.Pid, d.subscription.Ref},
				etf.Tuple{etf.Atom("ask"), d.count},
			}
			process.Send(process.Self(), msg)
			process.demandBuffer = process.demandBuffer[1:]
		}()

		if count == 0 {
			// just ignore it
			return etf.Atom("ok"), nil
		}

		if _, ok := process.consumers[subscription.Pid]; !ok {
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
			process.SendAfter(process.Self(), msg, 1*time.Second)
			return etf.Atom("ok"), nil
		}

		if process.Options.DisableForwarding {
			d := demandRequest{
				subscription: subscription,
				count:        count,
			}
			// FIXME it would be more effective to use sync.Pool with
			// preallocated array behind the slice.
			// see how it was made in lib.TakeBuffer
			process.demandBuffer = append(process.demandBuffer, d)
			return etf.Atom("ok"), nil
		}

		_, events = process.behavior.HandleDemand(process, subscription, count)

		// register this demand and trying to dispatch having events
		dispatcher := process.Options.Dispatcher
		dispatcher.Ask(process.dispatcherState, subscription, count)
		deliver = dispatcher.Dispatch(process.dispatcherState, events)
		if len(deliver) == 0 {
			return etf.Atom("ok"), nil
		}

		for d := range deliver {
			msg := etf.Tuple{
				etf.Atom("$gen_consumer"),
				etf.Tuple{deliver[d].subscription.Pid, deliver[d].subscription.Ref},
				deliver[d].events,
			}
			process.Send(deliver[d].subscription.Pid, msg)
		}

		return etf.Atom("ok"), nil

	case etf.Atom("cancel"):
		var e error
		// handle this cancelation in the dispatcher
		dispatcher := process.Options.Dispatcher
		dispatcher.Cancel(process.dispatcherState, subscription)
		reason := cmd.Opt1.(string)
		// handle it in a Stage callback
		e = process.behavior.HandleCancel(process, subscription, reason)
		delete(process.consumers, subscription.Pid)
		return etf.Atom("ok"), e
	}

	return nil, fmt.Errorf("unknown Stage command (HandleCall)")
}

func handleStageDown(process *StageProcess, down DownMessage) error {
	// remove subscription for producer and consumer. corner case - two
	// processes have subscribed to each other.

	// checking for subscribers (if we act as a producer).
	// we monitor them by Pid only
	if Pid, isPid := down.From.(etf.Pid); isPid {
		if subInternal, ok := process.consumers[Pid]; ok {
			// producer monitors consumer by the Pid and stores monitor reference
			// in the subInternal struct
			process.DemonitorProcess(subInternal.Monitor)
			cmd := stageRequestCommand{
				Cmd:  etf.Atom("cancel"),
				Opt1: down.Reason,
			}
			if _, err := handleProducer(process, subInternal.Subscription, cmd); err != nil {
				return err
			}
		}
	}

	// checking for producers (if we act as a consumer)
	if subInternal, ok := process.producers[down.Ref]; ok {
		cmd := stageRequestCommand{
			Cmd:  etf.Atom("cancel"),
			Opt1: down.Reason,
		}

		if _, err := handleConsumer(process, subInternal.Subscription, cmd); err != nil {
			return err
		}
	}

	return nil
}

// for the consumer side only
func sendDemand(p Process, producer etf.Term, subscription StageSubscription, count uint) {
	msg := etf.Tuple{
		etf.Atom("$gen_producer"),
		etf.Tuple{subscription.Pid, subscription.Ref},
		etf.Tuple{etf.Atom("ask"), count},
	}
	p.Send(producer, msg)
}
