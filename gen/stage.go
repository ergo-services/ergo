package gen

import (
	"fmt"
	"time"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/lib"
	//"github.com/ergo-services/ergo/lib"
)

type StageCancelMode uint

// StageOptions defines the producer configuration using Init callback. It will be ignored
// if it acts as a consumer only.
type StageOptions struct {

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

type StageStatus error

const (
	StageCancelPermanent StageCancelMode = 0
	StageCancelTransient StageCancelMode = 1
	StageCancelTemporary StageCancelMode = 2

	defaultDispatcherBufferSize = 10000
	defaultAutoDemandCount      = 3
)

var (
	StageStatusOK           StageStatus = nil
	StageStatusStop         StageStatus = fmt.Errorf("stop")
	StageStatusUnsupported  StageStatus = fmt.Errorf("unsupported")
	StageStatusNotAProducer StageStatus = fmt.Errorf("not a producer")
)

// StageBehavior interface for the Stage inmplementation
type StageBehavior interface {

	// InitStage
	InitStage(process *StageProcess, args ...etf.Term) (StageOptions, error)

	// HandleDemand this callback is invoked on a producer stage
	// The producer that implements this callback must either store the demand, or return the amount of requested events.
	HandleDemand(process *StageProcess, subscription StageSubscription, count uint) (etf.List, StageStatus)

	// HandleEvents this callback is invoked on a consumer stage.
	HandleEvents(process *StageProcess, subscription StageSubscription, events etf.List) StageStatus

	// HandleSubscribe This callback is invoked on a producer stage.
	HandleSubscribe(process *StageProcess, subscription StageSubscription, options StageSubscribeOptions) StageStatus

	// HandleSubscribed this callback is invoked as a confirmation for the subscription request
	// Returning false means that demand must be sent to producers explicitly using Ask method.
	// Returning true means the stage implementation will take care of automatically sending.
	HandleSubscribed(process *StageProcess, subscription StageSubscription, opts StageSubscribeOptions) (bool, StageStatus)

	// HandleCancel
	// Invoked when a consumer is no longer subscribed to a producer (invoked on a producer stage)
	// The cancelReason will be a {Cancel: "cancel", Reason: _} if the reason for cancellation
	// was a Stage.Cancel call. Any other value means the cancellation reason was
	// due to an EXIT.
	HandleCancel(process *StageProcess, subscription StageSubscription, reason string) StageStatus

	// HandleCanceled
	// Invoked when a consumer is no longer subscribed to a producer (invoked on a consumer stage)
	// Termination this stage depends on a cancel mode for the given subscription. For the cancel mode
	// StageCancelPermanent - this stage will be terminated right after this callback invoking.
	// For the cancel mode StageCancelTransient - it depends on a reason of subscription canceling.
	// Cancel mode StageCancelTemporary keeps this stage alive whether the reason could be.
	HandleCanceled(process *StageProcess, subscription StageSubscription, reason string) StageStatus

	// HandleStageCall this callback is invoked on ServerProcess.Call. This method is optional
	// for the implementation
	HandleStageCall(process *StageProcess, from ServerFrom, message etf.Term) (etf.Term, ServerStatus)
	// HandleStageDirect this callback is invoked on Process.Direct. This method is optional
	// for the implementation
	HandleStageDirect(process *StageProcess, ref etf.Ref, message interface{}) (interface{}, DirectStatus)
	// HandleStageCast this callback is invoked on ServerProcess.Cast. This method is optional
	// for the implementation
	HandleStageCast(process *StageProcess, message etf.Term) ServerStatus
	// HandleStageInfo this callback is invoked on Process.Send. This method is optional
	// for the implementation
	HandleStageInfo(process *StageProcess, message etf.Term) ServerStatus
}

type StageSubscription struct {
	Pid etf.Pid
	ID  etf.Ref
}

type subscriptionInternal struct {
	Producer     etf.Term
	Subscription StageSubscription
	options      StageSubscribeOptions
	Monitor      etf.Ref
	// number of event requests (demands) made as a consumer.
	count int
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

	options         StageOptions
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

type setForwardDemand struct {
	forward bool
}

type demandRequest struct {
	subscription StageSubscription
	count        uint
}

// Stage API

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

//
// StageProcess methods
//

// Subscribe subscribes to the given producer. HandleSubscribed callback will be invoked
// on a consumer stage once a request for the subscription is sent. If something went wrong
// on a producer side the callback HandleCancel will be invoked with a reason of cancelation.
func (p *StageProcess) Subscribe(producer etf.Term, opts StageSubscribeOptions) (StageSubscription, error) {
	var subscription StageSubscription
	switch producer.(type) {
	case string:
	case etf.Pid:
	case ProcessID:
	default:
		return subscription, fmt.Errorf("allowed type for producer: etf.Pid, string, gen.ProcessID")
	}

	subscription_id := p.MonitorProcess(producer)
	subscription.Pid = p.Self()
	subscription.ID = subscription_id

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
	// to make sure if we registered this subscription before the MessageDown
	// or MessageExit message arrived in case of something went wrong.
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

	return subscription, nil
}

// SendEvents sends events to the subscribers
func (p *StageProcess) SendEvents(events etf.List) error {
	var deliver []StageDispatchItem
	// dispatch to the subscribers
	if len(p.consumers) == 0 {
		return fmt.Errorf("no subscribers")
	}
	deliver = p.options.Dispatcher.Dispatch(p.dispatcherState, events)
	if len(deliver) == 0 {
		return fmt.Errorf("no demand")
	}
	for d := range deliver {
		msg := etf.Tuple{
			etf.Atom("$gen_consumer"),
			etf.Tuple{deliver[d].subscription.Pid, deliver[d].subscription.ID},
			deliver[d].events,
		}
		p.Send(deliver[d].subscription.Pid, msg)
	}
	return nil
}

// Ask makes a demand request for the given subscription. This function must only be
// used in the cases when a consumer sets a subscription to manual mode using DisableAutoDemand
func (p *StageProcess) Ask(subscription StageSubscription, count uint) error {
	subInternal, ok := p.producers[subscription.ID]
	if !ok {
		return fmt.Errorf("unknown subscription")
	}
	if !subInternal.options.ManualDemand {
		return fmt.Errorf("auto demand")
	}

	sendDemand(p, subInternal.Producer, subInternal.Subscription, count)
	subInternal.count += int(count)
	return nil
}

// Cancel
func (p *StageProcess) Cancel(subscription StageSubscription, reason string) error {
	// if we act as a consumer with this subscription
	if subInternal, ok := p.producers[subscription.ID]; ok {
		msg := etf.Tuple{
			etf.Atom("$gen_producer"),
			etf.Tuple{subscription.Pid, subscription.ID},
			etf.Tuple{etf.Atom("cancel"), reason},
		}
		p.Send(subInternal.Producer, msg)
		cmd := stageRequestCommand{
			Cmd:  etf.Atom("cancel"),
			Opt1: "normal",
		}
		if _, err := handleConsumer(p, subInternal.Subscription, cmd); err != nil {
			return err
		}
		return nil
	}
	// if we act as a producer within this subscription
	if subInternal, ok := p.consumers[subscription.Pid]; ok {
		msg := etf.Tuple{
			etf.Atom("$gen_consumer"),
			etf.Tuple{subscription.Pid, subscription.ID},
			etf.Tuple{etf.Atom("cancel"), reason},
		}
		p.Send(subscription.Pid, msg)
		p.DemonitorProcess(subInternal.Monitor)
		cmd := stageRequestCommand{
			Cmd:  etf.Atom("cancel"),
			Opt1: "normal",
		}
		if _, err := handleProducer(p, subInternal.Subscription, cmd); err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("unknown subscription")

}

//
// gen.Server callbacks
//
func (gst *Stage) Init(process *ServerProcess, args ...etf.Term) error {
	stageProcess := &StageProcess{
		ServerProcess: *process,
		producers:     make(map[etf.Ref]*subscriptionInternal),
		consumers:     make(map[etf.Pid]*subscriptionInternal),
	}
	// do not inherit parent State
	stageProcess.State = nil

	behavior, ok := process.Behavior().(StageBehavior)
	if !ok {
		return fmt.Errorf("Stage: not a StageBehavior")
	}
	stageProcess.behavior = behavior

	stageOpts, err := behavior.InitStage(stageProcess, args...)
	if err != nil {
		return err
	}

	if stageOpts.BufferSize == 0 {
		stageOpts.BufferSize = defaultDispatcherBufferSize
	}

	// if dispatcher wasn't specified create a default one StageDispatcherDemand
	if stageOpts.Dispatcher == nil {
		stageOpts.Dispatcher = CreateStageDispatcherDemand()
	}

	stageProcess.dispatcherState = stageOpts.Dispatcher.Init(stageOpts)
	stageProcess.options = stageOpts

	process.State = stageProcess
	return nil
}

func (gst *Stage) HandleCall(process *ServerProcess, from ServerFrom, message etf.Term) (etf.Term, ServerStatus) {
	stageProcess := process.State.(*StageProcess)
	return stageProcess.behavior.HandleStageCall(stageProcess, from, message)
}

func (gst *Stage) HandleDirect(process *ServerProcess, ref etf.Ref, message interface{}) (interface{}, DirectStatus) {
	stageProcess := process.State.(*StageProcess)
	switch m := message.(type) {
	case setManualDemand:
		subInternal, ok := stageProcess.producers[m.subscription.ID]
		if !ok {
			return nil, fmt.Errorf("unknown subscription")
		}
		subInternal.options.ManualDemand = m.enable
		if subInternal.count < defaultAutoDemandCount && !subInternal.options.ManualDemand {
			sendDemand(process, subInternal.Producer, m.subscription, defaultAutoDemandCount)
			subInternal.count += defaultAutoDemandCount
		}
		return nil, DirectStatusOK

	case setCancelMode:
		subInternal, ok := stageProcess.producers[m.subscription.ID]
		if !ok {
			return nil, fmt.Errorf("unknown subscription")
		}
		subInternal.options.Cancel = m.cancel
		return nil, DirectStatusOK

	case setForwardDemand:
		stageProcess.options.DisableForwarding = !m.forward
		if !m.forward {
			return nil, DirectStatusOK
		}

		// create demand with count = 0, which will be ignored but start
		// the processing of the buffered demands
		msg := etf.Tuple{
			etf.Atom("$gen_producer"),
			etf.Tuple{etf.Pid{}, etf.Ref{}},
			etf.Tuple{etf.Atom("ask"), 0},
		}
		process.Send(process.Self(), msg)

		return nil, DirectStatusOK

	default:
		return stageProcess.behavior.HandleStageDirect(stageProcess, ref, message)
	}

}

func (gst *Stage) HandleCast(process *ServerProcess, message etf.Term) ServerStatus {
	stageProcess := process.State.(*StageProcess)
	return stageProcess.behavior.HandleStageCast(stageProcess, message)
}

func (gst *Stage) HandleInfo(process *ServerProcess, message etf.Term) ServerStatus {
	var r stageMessage

	stageProcess := process.State.(*StageProcess)

	// check if we got a MessageDown
	if d, isDown := IsMessageDown(message); isDown {
		if err := handleStageDown(stageProcess, d); err != nil {
			return err
		}
		return ServerStatusOK
	}

	if err := etf.TermIntoStruct(message, &r); err != nil {
		reply := stageProcess.behavior.HandleStageInfo(stageProcess, message)
		return reply
	}

	_, err := handleStageRequest(stageProcess, r)

	switch err {
	case nil:
		return ServerStatusOK
	case StageStatusStop:
		return ServerStatusStop
	case StageStatusUnsupported:
		status := stageProcess.behavior.HandleStageInfo(stageProcess, message)
		return status
	default:
		return err
	}
}

// default callbacks

// InitStage
func (gst *Stage) InitStage(process *StageProcess, args ...etf.Term) error {
	return nil
}

// HandleSagaCall
func (gst *Stage) HandleStageCall(process *StageProcess, from ServerFrom, message etf.Term) (etf.Term, ServerStatus) {
	// default callback if it wasn't implemented
	lib.Warning("HandleStageCall: unhandled message (from %#v) %#v", from, message)
	return etf.Atom("ok"), ServerStatusOK
}

// HandleStageDirect
func (gst *Stage) HandleStageDirect(process *StageProcess, message interface{}) (interface{}, error) {
	// default callback if it wasn't implemented
	return nil, lib.ErrUnsupportedRequest
}

// HandleStageCast
func (gst *Stage) HandleStageCast(process *StageProcess, message etf.Term) ServerStatus {
	// default callback if it wasn't implemented
	lib.Warning("HandleStageCast: unhandled message %#v", message)
	return ServerStatusOK
}

// HandleStageInfo
func (gst *Stage) HandleStageInfo(process *StageProcess, message etf.Term) ServerStatus {
	// default callback if it wasn't implemnted
	lib.Warning("HandleStageInfo: unhandled message %#v", message)
	return ServerStatusOK
}

// HandleSubscribe
func (gst *Stage) HandleSubscribe(process *StageProcess, subscription StageSubscription, options StageSubscribeOptions) StageStatus {
	return StageStatusNotAProducer
}

// HandleSubscribed
func (gst *Stage) HandleSubscribed(process *StageProcess, subscription StageSubscription, opts StageSubscribeOptions) (bool, StageStatus) {
	return opts.ManualDemand, StageStatusOK
}

// HandleCancel
func (gst *Stage) HandleCancel(process *StageProcess, subscription StageSubscription, reason string) StageStatus {
	// default callback if it wasn't implemented
	return StageStatusOK
}

// HandleCanceled
func (gst *Stage) HandleCanceled(process *StageProcess, subscription StageSubscription, reason string) StageStatus {
	// default callback if it wasn't implemented
	return StageStatusOK
}

// HanndleEvents
func (gst *Stage) HandleEvents(process *StageProcess, subscription StageSubscription, events etf.List) StageStatus {
	lib.Warning("Stage HandleEvents: unhandled subscription (%#v) events %#v", subscription, events)
	return StageStatusOK
}

// HandleDemand
func (gst *Stage) HandleDemand(process *StageProcess, subscription StageSubscription, count uint) (etf.List, StageStatus) {
	lib.Warning("Stage HandleDemand: unhandled subscription (%#v) demand %#v", subscription, count)
	return nil, StageStatusOK
}

// private functions

func handleStageRequest(process *StageProcess, m stageMessage) (etf.Term, StageStatus) {
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
			return nil, StageStatusUnsupported
		}
		return handleConsumer(process, m.Subscription, command)
	case "$gen_producer":
		if err := etf.TermIntoStruct(m.Command, &command); err != nil {
			return nil, StageStatusUnsupported
		}
		return handleProducer(process, m.Subscription, command)
	}
	return nil, StageStatusUnsupported
}

func handleConsumer(process *StageProcess, subscription StageSubscription, cmd stageRequestCommand) (etf.Term, error) {
	var subscriptionOpts StageSubscribeOptions
	var err error

	switch cmd.Cmd {
	case etf.Atom("events"):
		events := cmd.Opt1.(etf.List)
		numEvents := len(events)

		subInternal, ok := process.producers[subscription.ID]
		if !ok {
			lib.Warning("consumer got %d events for unknown subscription %#v", numEvents, subscription)
			return etf.Atom("ok"), nil
		}
		subInternal.count--
		if subInternal.count < 0 {
			return nil, fmt.Errorf("got %d events which haven't bin requested", numEvents)
		}
		if numEvents < int(subInternal.options.MinDemand) {
			return nil, fmt.Errorf("got %d events which is less than min %d", numEvents, subInternal.options.MinDemand)
		}
		if numEvents > int(subInternal.options.MaxDemand) {
			return nil, fmt.Errorf("got %d events which is more than max %d", numEvents, subInternal.options.MaxDemand)
		}

		err = process.behavior.HandleEvents(process, subscription, events)
		if err != nil {
			return nil, err
		}

		// if subscription has auto demand we should request yet another
		// bunch of events
		if subInternal.count < defaultAutoDemandCount && !subInternal.options.ManualDemand {
			sendDemand(process, subInternal.Producer, subscription, defaultAutoDemandCount)
			subInternal.count += defaultAutoDemandCount
		}
		return etf.Atom("ok"), nil

	case etf.Atom("subscribed"):
		if err := etf.TermProplistIntoStruct(cmd.Opt2, &subscriptionOpts); err != nil {
			return nil, err
		}

		manualDemand, status := process.behavior.HandleSubscribed(process, subscription, subscriptionOpts)

		if status != StageStatusOK {
			return nil, status
		}
		subscriptionOpts.ManualDemand = manualDemand

		producer := cmd.Opt1
		subInternal := &subscriptionInternal{
			Subscription: subscription,
			Producer:     producer,
			options:      subscriptionOpts,
		}
		process.producers[subscription.ID] = subInternal

		if !manualDemand {
			sendDemand(process, producer, subscription, defaultAutoDemandCount)
			subInternal.count = defaultAutoDemandCount
		}

		return etf.Atom("ok"), nil

	case etf.Atom("retry-cancel"):
		// if "subscribed" message hasn't still arrived then just ignore it
		if _, ok := process.producers[subscription.ID]; !ok {
			return etf.Atom("ok"), nil
		}
		fallthrough
	case etf.Atom("cancel"):
		// the subscription was canceled
		reason, ok := cmd.Opt1.(string)
		if !ok {
			return nil, fmt.Errorf("Cancel reason is not a string")
		}

		subInternal, ok := process.producers[subscription.ID]
		if !ok {
			// There might be a case when "cancel" message arrives before
			// the "subscribed" message due to async nature of messaging,
			// so we should wait a bit and try to handle it one more time
			// using "retry-cancel" message.
			// I got this problem with GOMAXPROCS=1
			msg := etf.Tuple{
				etf.Atom("$gen_consumer"),
				etf.Tuple{subscription.Pid, subscription.ID},
				etf.Tuple{etf.Atom("retry-cancel"), reason},
			}
			// handle it in a second
			process.SendAfter(process.Self(), msg, 200*time.Millisecond)
			return etf.Atom("ok"), nil
		}

		// if we already handle MessageDown skip it
		if reason != "noconnection" {
			process.DemonitorProcess(subscription.ID)
		}
		delete(process.producers, subscription.ID)

		err = process.behavior.HandleCanceled(process, subscription, reason)
		if err != nil {
			return nil, err
		}

		switch subInternal.options.Cancel {
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
				etf.Tuple{subscription.Pid, subscription.ID},
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
					etf.Tuple{subscription.Pid, s.Subscription.ID},
					etf.Tuple{etf.Atom("cancel"), "resubscribed"},
				}
				process.Send(subscription.Pid, msg)
				// notify dispatcher about cancelation the previous subscription
				canceledSubscription := StageSubscription{
					Pid: subscription.Pid,
					ID:  s.Subscription.ID,
				}
				// cancel current demands
				process.options.Dispatcher.Cancel(process.dispatcherState, canceledSubscription)
				// notify dispatcher about the new subscription
				if err := process.options.Dispatcher.Subscribe(process.dispatcherState, subscription, subscriptionOpts); err != nil {
					// dispatcher can't handle this subscription
					msg := etf.Tuple{
						etf.Atom("$gen_consumer"),
						etf.Tuple{subscription.Pid, s.Subscription.ID},
						etf.Tuple{etf.Atom("cancel"), err.Error()},
					}
					process.Send(subscription.Pid, msg)
					return etf.Atom("ok"), nil
				}

				s.Subscription = subscription
				return etf.Atom("ok"), nil
			}

			if err := process.options.Dispatcher.Subscribe(process.dispatcherState, subscription, subscriptionOpts); err != nil {
				// dispatcher can't handle this subscription
				msg := etf.Tuple{
					etf.Atom("$gen_consumer"),
					etf.Tuple{subscription.Pid, subscription.ID},
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
				options:      subscriptionOpts,
			}
			process.consumers[subscription.Pid] = s
			return etf.Atom("ok"), nil

		case StageStatusNotAProducer:
			// if it wasnt overloaded - send 'cancel' to the consumer
			msg := etf.Tuple{
				etf.Atom("$gen_consumer"),
				etf.Tuple{subscription.Pid, subscription.ID},
				etf.Tuple{etf.Atom("cancel"), err.Error()},
			}
			process.Send(subscription.Pid, msg)
			return etf.Atom("ok"), nil

		default:
			// any other error should terminate this stage
			return nil, err
		}
	case etf.Atom("retry-ask"):
		// if the "subscribe" message hasn't still arrived, send a cancelation message
		// to the consumer
		if _, ok := process.consumers[subscription.Pid]; !ok {
			msg := etf.Tuple{
				etf.Atom("$gen_consumer"),
				etf.Tuple{subscription.Pid, subscription.ID},
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
			if process.options.DisableForwarding {
				return
			}
			if len(process.demandBuffer) == 0 {
				return
			}
			d := process.demandBuffer[0]
			msg := etf.Tuple{
				etf.Atom("$gen_producer"),
				etf.Tuple{d.subscription.Pid, d.subscription.ID},
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
				etf.Tuple{subscription.Pid, subscription.ID},
				etf.Tuple{etf.Atom("retry-ask"), count},
			}
			// handle it in a second
			process.SendAfter(process.Self(), msg, 1*time.Second)
			return etf.Atom("ok"), nil
		}

		if process.options.DisableForwarding {
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

		events, _ = process.behavior.HandleDemand(process, subscription, count)

		// register this demand and trying to dispatch having events
		dispatcher := process.options.Dispatcher
		dispatcher.Ask(process.dispatcherState, subscription, count)
		deliver = dispatcher.Dispatch(process.dispatcherState, events)
		if len(deliver) == 0 {
			return etf.Atom("ok"), nil
		}

		for d := range deliver {
			msg := etf.Tuple{
				etf.Atom("$gen_consumer"),
				etf.Tuple{deliver[d].subscription.Pid, deliver[d].subscription.ID},
				deliver[d].events,
			}
			process.Send(deliver[d].subscription.Pid, msg)
		}

		return etf.Atom("ok"), nil

	case etf.Atom("cancel"):
		var e error
		// handle this cancelation in the dispatcher
		dispatcher := process.options.Dispatcher
		dispatcher.Cancel(process.dispatcherState, subscription)
		reason := cmd.Opt1.(string)
		// handle it in a Stage callback
		e = process.behavior.HandleCancel(process, subscription, reason)
		delete(process.consumers, subscription.Pid)
		return etf.Atom("ok"), e
	}

	return nil, fmt.Errorf("unknown Stage command (HandleCall)")
}

func handleStageDown(process *StageProcess, down MessageDown) error {
	// remove subscription for producer and consumer. corner case - two
	// processes have subscribed to each other.

	// checking for subscribers (if we act as a producer).
	// we monitor them by Pid only
	if subInternal, ok := process.consumers[down.Pid]; ok {
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
		etf.Tuple{subscription.Pid, subscription.ID},
		etf.Tuple{etf.Atom("ask"), count},
	}
	p.Send(producer, msg)
}
