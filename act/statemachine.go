package act

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"

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
}

// Type alias for MessageHandler callbacks.
// D is the type of the data associated with the StateMachine.
// M is the type of the message this handler accepts.
type StateMessageHandler[D any, M any] func(gen.Atom, D, M, gen.Process) (gen.Atom, D, error)

// Type alias for CallHandler callbacks.
// D is the type of the data associated with the StateMachine.
// M is the type of the message this handler accepts.
// R is the type of the result value.
type StateCallHandler[D any, M any, R any] func(gen.Atom, D, M, gen.Process) (gen.Atom, D, R, error)

type StateMachineSpec[D any] struct {
	initialState         gen.Atom
	data                 D
	stateMessageHandlers map[gen.Atom]map[string]any
	stateCallHandlers    map[gen.Atom]map[string]any
}

type Option[D any] func(*StateMachineSpec[D])

func NewStateMachineSpec[D any](initialState gen.Atom, options ...Option[D]) StateMachineSpec[D] {
	spec := StateMachineSpec[D]{
		initialState:         initialState,
		stateMessageHandlers: make(map[gen.Atom]map[string]any),
		stateCallHandlers:    make(map[gen.Atom]map[string]any),
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

func (s *StateMachine[D]) CurrentState() gen.Atom {
	return s.currentState
}

func (s *StateMachine[D]) SetCurrentState(state gen.Atom) {
	s.currentState = state
}

func (s *StateMachine[D]) Data() D {
	return s.data
}

func (s *StateMachine[D]) SetData(data D) {
	s.data = data
}

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
			// check if there is a handler for the message in the current state
			messageType := typeName(message)
			handler, ok := sm.lookupMessageHandler(messageType)
			if ok == false {
				return fmt.Errorf("No handler for message %s in state %s", messageType, sm.currentState)
			}
			return sm.invokeMessageHandler(handler, message)

		case gen.MailboxMessageTypeRequest:
			var reason error
			var result any

			// check if there is a handler for the call in the current state
			messageType := typeName(message)
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
			if reason := sm.behavior.HandleEvent(message.Message.(gen.MessageEvent)); reason != nil {
				return reason
			}

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

func typeName(message *gen.MailboxMessage) string {
	return reflect.TypeOf(message.Message).String()
}

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
			"error when invoking call handler for %v", typeName(message))
	}
	if !results[2].IsNil() {
		return results[2].Interface().(error)
	}
	//TODO: panic if new state or new data is not provided
	setCurrentStateMethod := reflect.ValueOf(sm).MethodByName("SetCurrentState")
	setCurrentStateMethod.Call([]reflect.Value{results[0]})
	setDataMethod := reflect.ValueOf(sm).MethodByName("SetData")
	setDataMethod.Call([]reflect.Value{results[1]})

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
			"error when invoking call handler for %v", typeName(message))
	}

	if !results[3].IsNil() {
		err := results[1].Interface().(error)
		return nil, err
	}
	//TODO: panic if new state or new data is not provided
	setCurrentStateMethod := reflect.ValueOf(sm).MethodByName("SetCurrentState")
	setCurrentStateMethod.Call([]reflect.Value{results[0]})
	setDataMethod := reflect.ValueOf(sm).MethodByName("SetData")
	setDataMethod.Call([]reflect.Value{results[1]})

	result := results[2].Interface()
	return result, nil
}
