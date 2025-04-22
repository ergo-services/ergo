package act

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

type StateMachineBehavior[D any] interface {
	gen.ProcessBehavior

	Init(args ...any) (StateMachineSpec[D], error)

	HandleMessage(from gen.PID, message any) error

	HandleCall(from gen.PID, ref gen.Ref, message any) (any, error)

	HandleEvent(event gen.MessageEvent) error

	HandleInspect(from gen.PID, item ...string) map[string]string

	Terminate(reason error)

	CurrentState() gen.Atom

	SetCurrentState(gen.Atom)

	Data() D

	SetData(data D)
}

type StateMachine[D any] struct {
	gen.Process

	behavior StateMachineBehavior[D]
	mailbox  gen.ProcessMailbox

	// The specification for the StateMachine
	spec StateMachineSpec[D]

	// The state the StateMachine is currently in
	currentState gen.Atom

	// The data associated with the StateMachine
	data D

	// stateMessageHandlers maps states to the (asynchronous) handlers for the state.
	// Key: State (gen.Atom) - The state for which the handler is registered.
	// Value: Map of message type to the handler for that message.
	//   Key: The type of the message received (String).
	//   Value: The message handler (any). There is a compile-time guarantee
	//          that the handler is of type StateMessageHandler[D, M].
	stateMessageHandlers map[gen.Atom]map[string]any

	// stateCallHandlers maps states to the (synchronous) handlers for the state.
	// Key: State (gen.Atom) - The state for which the handler is registered.
	// Value: Map of message type to the handler for that message.
	//   Key: The type of the message received (String).
	//   Value: The message handler (any). There is a compile-time guarantee
	//          that the handler is of type StateCallHandler[D, M, R].
	stateCallHandlers map[gen.Atom]map[string]any

	// eventHandlers maps events to the handler for the event.
	// Key: Event name (gen.Atom) - The name of the event
	// Value: The event handler (any). There is a compile-time guarantee that
	//        the handler is of type EventHandler[D, E]
	eventHandlers map[gen.Event]any

	// Callback that is invoked immediately after every state change. If no
	// callback is registered stateEnterCallback is nil.
	stateEnterCallback StateEnterCallback[D]
}

type Action interface {
	isAction()
}

type StateTimeout[M any] struct {
	Duration time.Duration
	message  M
}

func (StateTimeout[M]) IsAction() {}

// state_timeout
// timeout

// Type alias for MessageHandler callbacks.
// D is the type of the data associated with the StateMachine.
// M is the type of the message this handler accepts.
type StateMessageHandler[D any, M any] func(gen.Atom, D, M, gen.Process) (gen.Atom, D, error)

// new version with actions
//type StateMessageHandler[D any, M any] func(gen.Atom, D, M, gen.Process) (gen.Atom, D, []Action, error)

// Type alias for CallHandler callbacks.
// D is the type of the data associated with the StateMachine.
// M is the type of the message this handler accepts.
// R is the type of the result value.
type StateCallHandler[D any, M any, R any] func(gen.Atom, D, M, gen.Process) (gen.Atom, D, R, error)

// Type alias for event handler callbacks.
// D is the type of the data associated with the StateMachine.
// E is the type of the event.
type EventHandler[D any, E any] func(gen.Atom, D, E, gen.Process) (gen.Atom, D, error)

// Type alias for StateEnter callback.
// D is the type of the data associated with the StateMachine.
type StateEnterCallback[D any] func(gen.Atom, gen.Atom, D, gen.Process) (gen.Atom, D, error)

type StateMachineSpec[D any] struct {
	initialState         gen.Atom
	data                 D
	stateMessageHandlers map[gen.Atom]map[string]any
	stateCallHandlers    map[gen.Atom]map[string]any
	eventHandlers        map[gen.Event]any
	stateEnterCallback   StateEnterCallback[D]
}

type Option[D any] func(*StateMachineSpec[D])

func NewStateMachineSpec[D any](initialState gen.Atom, options ...Option[D]) StateMachineSpec[D] {
	spec := StateMachineSpec[D]{
		initialState:         initialState,
		stateMessageHandlers: make(map[gen.Atom]map[string]any),
		stateCallHandlers:    make(map[gen.Atom]map[string]any),
		eventHandlers:        make(map[gen.Event]any),
	}
	for _, opt := range options {
		opt(&spec)
	}
	return spec
}

func WithData[D any](data D) Option[D] {
	return func(s *StateMachineSpec[D]) {
		s.data = data
	}
}

func WithStateMessageHandler[D any, M any](state gen.Atom, handler StateMessageHandler[D, M]) Option[D] {
	messageType := reflect.TypeOf((*M)(nil)).Elem().String()
	return func(s *StateMachineSpec[D]) {
		if _, exists := s.stateMessageHandlers[state]; exists == false {
			s.stateMessageHandlers[state] = make(map[string]any)
		}
		s.stateMessageHandlers[state][messageType] = handler
	}
}

func WithStateCallHandler[D any, M any, R any](state gen.Atom, handler StateCallHandler[D, M, R]) Option[D] {
	messageType := reflect.TypeOf((*M)(nil)).Elem().String()
	return func(s *StateMachineSpec[D]) {
		if _, exists := s.stateCallHandlers[state]; exists == false {
			s.stateCallHandlers[state] = make(map[string]any)
		}
		s.stateCallHandlers[state][messageType] = handler
	}
}

func WithStateEnterCallback[D any](callback StateEnterCallback[D]) Option[D] {
	return func(s *StateMachineSpec[D]) {
		s.stateEnterCallback = callback
	}
}

func WithEventHandler[D any, E any](event gen.Event, handler EventHandler[D, E]) Option[D] {
	return func(s *StateMachineSpec[D]) {
		s.eventHandlers[event] = handler
	}
}

func (s *StateMachine[D]) CurrentState() gen.Atom {
	return s.currentState
}

func (s *StateMachine[D]) SetCurrentState(state gen.Atom) {
	if state != s.currentState {
		s.Log().Info("setting current state to %v", state)
		oldState := s.currentState
		s.currentState = state

		// Execute state enter callback until no new transition is triggered.
		if s.stateEnterCallback != nil {
			newState, newData, err := s.stateEnterCallback(oldState, state, s.data, s)
			if err != nil {
				panic(fmt.Sprintf("error in StateEnterCallback for state %s", state))
			}
			s.SetData(newData)
			s.SetCurrentState(newState)
		}
	}
}

func (s *StateMachine[D]) Data() D {
	return s.data
}

func (s *StateMachine[D]) SetData(data D) {
	s.data = data
}

type startMonitoringEvents struct{}

//
// ProcessBehavior implementation
//

func (sm *StateMachine[D]) ProcessInit(process gen.Process, args ...any) (rr error) {
	var ok bool

	if sm.behavior, ok = process.Behavior().(StateMachineBehavior[D]); ok == false {
		unknown := strings.TrimPrefix(reflect.TypeOf(process.Behavior()).String(), "*")
		return fmt.Errorf("ProcessInit: not a StateMachineBehavior %s", unknown)
	}

	sm.Process = process
	sm.mailbox = process.Mailbox()

	if lib.Recover() {
		defer func() {
			if r := recover(); r != nil {
				pc, fn, line, _ := runtime.Caller(2)
				sm.Log().Panic("StateMachine initialization failed. Panic reason: %#v at %s[%s:%d]",
					r, runtime.FuncForPC(pc).Name(), fn, line)
				rr = gen.TerminateReasonPanic
			}
		}()
	}

	spec, err := sm.behavior.Init(args...)
	if err != nil {
		return err
	}

	sm.currentState = spec.initialState
	sm.data = spec.data
	sm.stateMessageHandlers = spec.stateMessageHandlers
	sm.stateCallHandlers = spec.stateCallHandlers
	sm.eventHandlers = spec.eventHandlers
	sm.stateEnterCallback = spec.stateEnterCallback

	// if we have event handlers we need to start listening for events
	if len(sm.eventHandlers) > 0 {
		sm.Send(sm.PID(), startMonitoringEvents{})
	}

	return nil
}

func (sm *StateMachine[D]) ProcessRun() (rr error) {
	var message *gen.MailboxMessage

	if lib.Recover() {
		defer func() {
			if r := recover(); r != nil {
				pc, fn, line, _ := runtime.Caller(2)
				sm.Log().Panic("StateMachine terminated. Panic reason: %#v at %s[%s:%d]",
					r, runtime.FuncForPC(pc).Name(), fn, line)
				rr = gen.TerminateReasonPanic
			}
		}()
	}

	for {
		if sm.State() != gen.ProcessStateRunning {
			// process was killed by the node.
			return gen.TerminateReasonKill
		}

		if message != nil {
			gen.ReleaseMailboxMessage(message)
			message = nil
		}

		for {
			// check queues
			msg, ok := sm.mailbox.Urgent.Pop()
			if ok {
				// got new urgent message. handle it
				message = msg.(*gen.MailboxMessage)
				break
			}

			msg, ok = sm.mailbox.System.Pop()
			if ok {
				// got new system message. handle it
				message = msg.(*gen.MailboxMessage)
				break
			}

			msg, ok = sm.mailbox.Main.Pop()
			if ok {
				// got new regular message. handle it
				message = msg.(*gen.MailboxMessage)
				break
			}

			if _, ok := sm.mailbox.Log.Pop(); ok {
				panic("statemachne process can not be a logger")
			}

			// no messages in the mailbox
			return nil
		}

		switch message.Type {
		case gen.MailboxMessageTypeRegular:
			switch message.Message.(type) {
			case startMonitoringEvents:
				// start monitoring
				for event := range sm.eventHandlers {
					if _, err := sm.MonitorEvent(event); err != nil {
						panic(fmt.Sprintf("Error monitoring event: %v.", err))
					}
				}
				sm.Log().Info("StateMachine %s is now monitoring events", sm.PID())
				return nil

			default:
				// check if there is a handler for the message in the current state
				messageType := reflect.TypeOf(message.Message).String()
				handler, ok := sm.lookupMessageHandler(messageType)
				if ok == false {
					return fmt.Errorf("No handler for message %s in state %s", messageType, sm.currentState)
				}
				return sm.invokeMessageHandler(handler, message)
			}

		case gen.MailboxMessageTypeRequest:
			var reason error
			var result any

			// check if there is a handler for the call in the current state
			messageType := reflect.TypeOf(message.Message).String()
			handler, ok := sm.lookupCallHandler(messageType)
			if ok == false {
				return fmt.Errorf("No handler for message %s in state %s", messageType, sm.currentState)
			}
			result, reason = sm.invokeCallHandler(handler, message)

			if reason != nil {
				// if reason is "normal" and we got response - send it before termination
				if reason == gen.TerminateReasonNormal {
					sm.SendResponse(message.From, message.Ref, result)
				}
				return reason
			}
			// Note: we do not support async handling of sync request at the moment
			sm.SendResponse(message.From, message.Ref, result)

		case gen.MailboxMessageTypeEvent:
			event := message.Message.(gen.MessageEvent)
			handler, exists := sm.eventHandlers[event.Event]
			if exists == false {
				return fmt.Errorf("No handler for event %v", event)
			}
			return sm.invokeEventHandler(handler, &event)

		case gen.MailboxMessageTypeExit:
			switch exit := message.Message.(type) {
			case gen.MessageExitPID:
				return fmt.Errorf("%s: %w", exit.PID, exit.Reason)

			case gen.MessageExitProcessID:
				return fmt.Errorf("%s: %w", exit.ProcessID, exit.Reason)

			case gen.MessageExitAlias:
				return fmt.Errorf("%s: %w", exit.Alias, exit.Reason)

			case gen.MessageExitEvent:
				return fmt.Errorf("%s: %w", exit.Event, exit.Reason)

			case gen.MessageExitNode:
				return fmt.Errorf("%s: %w", exit.Name, gen.ErrNoConnection)

			default:
				panic(fmt.Sprintf("unknown exit message: %#v", exit))
			}

		case gen.MailboxMessageTypeInspect:
			result := sm.behavior.HandleInspect(message.From, message.Message.([]string)...)
			sm.SendResponse(message.From, message.Ref, result)
		}
	}
}

func (sm *StateMachine[D]) ProcessTerminate(reason error) {
	sm.behavior.Terminate(reason)
}

//
// StateMachineBehavior default callbacks
//

func (s *StateMachine[D]) HandleMessage(from gen.PID, message any) error {
	s.Log().Warning("StateMachine.HandleMessage: unhandled message from %s", from)
	return nil
}

func (s *StateMachine[D]) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	s.Log().Warning("StateMachine.HandleCall: unhandled request from %s", from)
	return nil, nil
}
func (s *StateMachine[D]) HandleEvent(message gen.MessageEvent) error {
	s.Log().Warning("StateMachine.HandleEvent: unhandled event message %#v", message)
	return nil
}

func (s *StateMachine[D]) HandleInspect(from gen.PID, item ...string) map[string]string {
	return nil
}

func (s *StateMachine[D]) Terminate(reason error) {}

//
// Internals
//

func (sm *StateMachine[D]) lookupMessageHandler(messageType string) (any, bool) {
	if stateMessageHandlers, exists := sm.stateMessageHandlers[sm.currentState]; exists == true {
		if callback, exists := stateMessageHandlers[messageType]; exists == true {
			return callback, true
		}
	}
	return nil, false
}

func (sm *StateMachine[D]) invokeMessageHandler(handler any, message *gen.MailboxMessage) error {
	callbackValue := reflect.ValueOf(handler)
	stateValue := reflect.ValueOf(sm.currentState)
	dataValue := reflect.ValueOf(sm.Data())
	msgValue := reflect.ValueOf(message.Message)
	procValue := reflect.ValueOf(sm)

	results := callbackValue.Call([]reflect.Value{stateValue, dataValue, msgValue, procValue})

	if len(results) != 3 {
		sm.Log().Panic("StateMachine terminated. Panic reason: unexpected "+
			"error when invoking call handler for %s", reflect.TypeOf(message.Message))
		return gen.TerminateReasonPanic
	}
	if !results[2].IsNil() {
		return results[2].Interface().(error)
	}

	setDataMethod := reflect.ValueOf(sm).MethodByName("SetData")
	setDataMethod.Call([]reflect.Value{results[1]})
	// It is important that we set the state last as this can potentially trigger
	// a state enter callback
	setCurrentStateMethod := reflect.ValueOf(sm).MethodByName("SetCurrentState")
	setCurrentStateMethod.Call([]reflect.Value{results[0]})

	return nil
}

func (sm *StateMachine[D]) lookupCallHandler(messageType string) (any, bool) {
	if stateCallHandlers, exists := sm.stateCallHandlers[sm.currentState]; exists == true {
		if callback, exists := stateCallHandlers[messageType]; exists == true {
			return callback, true
		}
	}
	return nil, false
}

func (sm *StateMachine[D]) invokeCallHandler(handler any, message *gen.MailboxMessage) (any, error) {
	callbackValue := reflect.ValueOf(handler)
	stateValue := reflect.ValueOf(sm.currentState)
	dataValue := reflect.ValueOf(sm.Data())
	msgValue := reflect.ValueOf(message.Message)
	procValue := reflect.ValueOf(sm)

	results := callbackValue.Call([]reflect.Value{stateValue, dataValue, msgValue, procValue})

	if len(results) != 4 {
		sm.Log().Panic("StateMachine terminated. Panic reason: unexpected "+
			"error when invoking call handler for %s", reflect.TypeOf(message.Message))
		return nil, gen.TerminateReasonPanic
	}

	if !results[3].IsNil() {
		err := results[3].Interface().(error)
		return nil, err
	}

	setDataMethod := reflect.ValueOf(sm).MethodByName("SetData")
	setDataMethod.Call([]reflect.Value{results[1]})
	// It is important that we set the state last as this can potentially trigger
	// a state enter callback
	setCurrentStateMethod := reflect.ValueOf(sm).MethodByName("SetCurrentState")
	setCurrentStateMethod.Call([]reflect.Value{results[0]})

	result := results[2].Interface()

	return result, nil
}

func (sm *StateMachine[D]) invokeEventHandler(handler any, message *gen.MessageEvent) error {
	callbackValue := reflect.ValueOf(handler)
	stateValue := reflect.ValueOf(sm.currentState)
	dataValue := reflect.ValueOf(sm.Data())
	msgValue := reflect.ValueOf(message.Message)
	procValue := reflect.ValueOf(sm)

	results := callbackValue.Call([]reflect.Value{stateValue, dataValue, msgValue, procValue})

	if len(results) != 3 {
		sm.Log().Panic("StateMachine terminated. Panic reason: unexpected "+
			"error when invoking call handler for %s", reflect.TypeOf(message.Message))
		return gen.TerminateReasonPanic
	}

	if !results[2].IsNil() {
		err := results[2].Interface().(error)
		return err
	}

	setDataMethod := reflect.ValueOf(sm).MethodByName("SetData")
	setDataMethod.Call([]reflect.Value{results[1]})
	// It is important that we set the state last as this can potentially trigger
	// a state enter callback
	setCurrentStateMethod := reflect.ValueOf(sm).MethodByName("SetCurrentState")
	setCurrentStateMethod.Call([]reflect.Value{results[0]})

	return nil
}
