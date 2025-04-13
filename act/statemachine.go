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

	spec StateMachineSpec[D]

	currentState   gen.Atom
	data           D
	stateCallbacks map[gen.Atom]map[string]interface{}
}

type StateCallback[D any, M any] func(*StateMachine[D], M) error

type StateMachineSpec[D any] struct {
	initialState   gen.Atom
	stateCallbacks map[gen.Atom]map[string]interface{}
}

type AddCallback[D any] func(*StateMachineSpec[D])

func NewStateMachineSpec[D any](initialState gen.Atom, callbacks ...AddCallback[D]) StateMachineSpec[D] {
	spec := StateMachineSpec[D]{
		initialState:   initialState,
		stateCallbacks: make(map[gen.Atom]map[string]interface{}),
	}
	for _, cb := range callbacks {
		cb(&spec)
	}
	return spec
}

func WithStateCallback[D any, M any](state gen.Atom, callback func(*StateMachine[D], M) error) AddCallback[D] {
	typeName := reflect.TypeOf((*M)(nil)).Elem().String()
	return func(s *StateMachineSpec[D]) {
		if _, exists := s.stateCallbacks[state]; exists == false {
			s.stateCallbacks[state] = make(map[string]interface{})
		}
		s.stateCallbacks[state][typeName] = callback
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

	// set up callbacks
	sm.currentState = spec.initialState
	sm.stateCallbacks = spec.stateCallbacks

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
			typeName := typeName(message)
			if callbackInterface, ok := sm.lookupCallback(typeName); ok == true {
				callbackValue := reflect.ValueOf(callbackInterface)
				smValue := reflect.ValueOf(sm)
				msgValue := reflect.ValueOf(message.Message)

				results := callbackValue.Call([]reflect.Value{smValue, msgValue})

				if len(results) > 0 && !results[0].IsNil() {
					return results[0].Interface().(error)
				}
				return nil
			}
			return fmt.Errorf("Unsupported message %s for state %s", typeName, sm.currentState)

		case gen.MailboxMessageTypeRequest:
			var reason error
			var result gen.Atom = sm.CurrentState()

			// check if there is a handler for the call in the current state
			typeName := typeName(message)
			if callbackInterface, ok := sm.lookupCallback(typeName); ok == true {
				callbackValue := reflect.ValueOf(callbackInterface)
				smValue := reflect.ValueOf(sm)
				msgValue := reflect.ValueOf(message.Message)

				results := callbackValue.Call([]reflect.Value{smValue, msgValue})

				if len(results) > 0 && !results[0].IsNil() {
					reason = results[0].Interface().(error)
				}
				result = sm.CurrentState()
			} else {
				reason = fmt.Errorf("Unsupported call %s for state %s", typeName, sm.currentState)
			}

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

func (sm *StateMachine[D]) lookupCallback(messageType string) (interface{}, bool) {
	if stateCallbacks, exists := sm.stateCallbacks[sm.currentState]; exists == true {
		if callback, exists := stateCallbacks[messageType]; exists == true {
			return callback, true
		}
	}
	return nil, false
}
