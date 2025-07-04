package act

import (
	"fmt"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

const (
	defaultRestartIntensity uint16 = 5
	defaultRestartPeriod    uint16 = 5
)

type SupervisorBehavior interface {
	gen.ProcessBehavior

	// Init invoked on a spawn Supervisor process. This is a mandatory callback for the implementation
	Init(args ...any) (SupervisorSpec, error)

	// HandleChildStart invoked on a successful child process starting if option EnableHandleChild
	// was enabled in act.SupervisorSpec
	HandleChildStart(name gen.Atom, pid gen.PID) error

	// HandleChildTerminate invoked on a child process termination if option EnableHandleChild
	// was enabled in act.SupervisorSpec
	HandleChildTerminate(name gen.Atom, pid gen.PID, reason error) error

	// HandleMessage invoked if Supervisor received a message sent with gen.Process.Send(...).
	// Non-nil value of the returning error will cause termination of this process.
	// To stop this process normally, return gen.TerminateReasonNormal or
	// gen.TerminateReasonShutdown. Any other - for abnormal termination.
	HandleMessage(from gen.PID, message any) error

	// HandleCall invoked if Supervisor got a synchronous request made with gen.Process.Call(...).
	// Return nil as a result to handle this request asynchronously and
	// to provide the result later using the gen.Process.SendResponse(...) method.
	HandleCall(from gen.PID, ref gen.Ref, request any) (any, error)

	// Terminate invoked on a termination supervisor process
	Terminate(reason error)

	// HandleEvent invoked on an event message if this process got subscribed on
	// this event using gen.Process.LinkEvent or gen.Process.MonitorEvent
	HandleEvent(message gen.MessageEvent) error

	// HandleInspect invoked on the request made with gen.Process.Inspect(...)
	HandleInspect(from gen.PID, item ...string) map[string]string
}

type Supervisor struct {
	gen.Process

	behavior SupervisorBehavior
	mailbox  gen.ProcessMailbox

	sup supBehavior

	handleChild bool
	children    map[gen.PID]gen.Atom
	state       supState
}

// SupervisorType
type SupervisorType int

func (s SupervisorType) String() string {
	switch s {
	case SupervisorTypeOneForOne:
		return "One For One"
	case SupervisorTypeAllForOne:
		return "All For One"
	case SupervisorTypeRestForOne:
		return "Rest For One"
	case SupervisorTypeSimpleOneForOne:
		return "Simple One For One"
	}
	return "Bug: unknown supervisor type"
}

const (

	// SupervisorTypeOneForOne If one child process terminates and is to be restarted, only
	// that child process is affected. This is the default restart strategy.
	SupervisorTypeOneForOne SupervisorType = 0

	// SupervisorTypeAllForOne If one child process terminates and is to be restarted, all other
	// child processes are terminated and then all child processes are restarted.
	SupervisorTypeAllForOne SupervisorType = 1

	// SupervisorTypeRestForOne If one child process terminates and is to be restarted,
	// the 'rest' of the child processes (that is, the child
	// processes after the terminated child process in the start order)
	// are terminated. Then the terminated child process and all
	// child processes after it are restarted
	SupervisorTypeRestForOne SupervisorType = 2

	// SupervisorTypeSimpleOneForOne A simplified one_for_one supervisor, where all
	// child processes are dynamically added instances
	// of the same process type, that is, running the same code.
	SupervisorTypeSimpleOneForOne SupervisorType = 3
)

// SupervisorStrategy defines restart strategy for the children processes
type SupervisorStrategy int

func (s SupervisorStrategy) String() string {
	switch s {
	case SupervisorStrategyTransient:
		return "Transient"
	case SupervisorStrategyTemporary:
		return "Temporary"
	case SupervisorStrategyPermanent:
		return "Permanent"
	}
	return "Bug: unknown supervisor strategy type"
}

const (
	// SupervisorStrategyTransient child process is restarted only if
	// it terminates abnormally, that is, with an exit reason other
	// than TerminateReasonNormal, TerminateReasonShutdown.
	// This is default strategy.
	SupervisorStrategyTransient SupervisorStrategy = 0

	// SupervisorStrategyTemporary child process is never restarted
	// (not even when the supervisor restart strategy is rest_for_one
	// or one_for_all and a sibling death causes the temporary process
	// to be terminated)
	SupervisorStrategyTemporary SupervisorStrategy = 1

	// SupervisorStrategyPermanent child process is always restarted
	SupervisorStrategyPermanent SupervisorStrategy = 2
)

// SupervisorSpec
type SupervisorSpec struct {
	Children []SupervisorChildSpec
	Type     SupervisorType
	Restart  SupervisorRestart

	// EnableHandleChild enables HandleChildStart/HandleChildTerminate callback
	// invoking on starting/stopping child processes. These callbacks are invoked
	// after the restart strategy finishes its work with all children.
	EnableHandleChild bool

	// DisableAutoShutdown makes the supervisor not shutdown if there is no one
	// running child process left (it happens if all child processes have been
	// terminated normally - by itself or the child spec was disabled).
	// This options is ignored for SupervisorTypeSimpleOneForOne
	// or if used restart strategy SupervisorStrategyPermanent
	DisableAutoShutdown bool
}

type SupervisorRestart struct {
	Strategy  SupervisorStrategy
	Intensity uint16
	Period    uint16
	KeepOrder bool // ignored for SupervisorTypeSimpleOneForOne and SupervisorTypeOneForOne
}

// SupervisorChildSpec
type SupervisorChildSpec struct {
	Name        gen.Atom
	Significant bool // ignored for SupervisorTypeSimpleOneForOne or if used restart strategy SupervisorStrategyPermanent
	Factory     gen.ProcessFactory
	Options     gen.ProcessOptions
	Args        []any
}

type SupervisorChild struct {
	Spec        gen.Atom
	Name        gen.Atom
	PID         gen.PID
	Significant bool
	Disabled    bool
}

// Children returns a list of supervisor children processes
func (s *Supervisor) Children() []SupervisorChild {
	return s.sup.children()
}

// StartChild starts new child process defined in the supervisor spec.
func (s *Supervisor) StartChild(name gen.Atom, args ...any) error {
	if s.State() != gen.ProcessStateRunning {
		return gen.ErrNotAllowed
	}

	action, err := s.sup.childSpec(name)
	if err != nil {
		return err
	}
	if len(args) > 0 {
		action.spec.Args = args
	}

	return s.handleAction(action)
}

// AddChild allows add a new child to the supervisor. Returns error if
// spawning child process is failed.
func (s *Supervisor) AddChild(child SupervisorChildSpec) error {
	if s.State() != gen.ProcessStateRunning {
		return gen.ErrNotAllowed
	}

	action, err := s.sup.childAddSpec(child)
	if err != nil {
		return err
	}
	return s.handleAction(action)
}

// EnableChild enables the child process in the supervisor spec and attempts to
// start it. Returns error if spawning child process is failed.
func (s *Supervisor) EnableChild(name gen.Atom) error {
	if s.State() != gen.ProcessStateRunning {
		return gen.ErrNotAllowed
	}

	action, err := s.sup.childEnable(name)
	if err != nil {
		return err
	}
	return s.handleAction(action)
}

// DisableChild stops the child process with gen.TerminateReasonShutdown
// and disables it in the supervisor spec.
func (s *Supervisor) DisableChild(name gen.Atom) error {
	if s.State() != gen.ProcessStateRunning {
		return gen.ErrNotAllowed
	}

	action, err := s.sup.childDisable(name)
	if err != nil {
		return err
	}
	return s.handleAction(action)
}

//
// ProcessBehavior implementation
//

// ProcessInit
func (s *Supervisor) ProcessInit(process gen.Process, args ...any) (rr error) {
	var ok bool

	if s.behavior, ok = process.Behavior().(SupervisorBehavior); ok == false {
		unknown := strings.TrimPrefix(reflect.TypeOf(process.Behavior()).String(), "*")
		return fmt.Errorf("ProcessInit: not a SupervisorBehavior %s", unknown)
	}

	s.Process = process
	s.mailbox = process.Mailbox()

	if lib.Recover() {
		defer func() {
			if r := recover(); r != nil {
				pc, fn, line, _ := runtime.Caller(2)
				s.Log().Panic("Supervisor initialization failed. Panic reason: %#v at %s[%s:%d]",
					r, runtime.FuncForPC(pc).Name(), fn, line)
				rr = gen.TerminateReasonPanic
			}
		}()
	}

	spec, err := s.behavior.Init(args...)
	if err != nil {
		return err
	}

	// validate restart strategy
	switch spec.Restart.Strategy {
	case SupervisorStrategyTransient, SupervisorStrategyTemporary,
		SupervisorStrategyPermanent:
		break
	default:
		return fmt.Errorf("unknown supervisor restart strategy")
	}

	// validate restart options
	if spec.Restart.Intensity == 0 {
		spec.Restart.Intensity = defaultRestartIntensity
	}
	if spec.Restart.Period == 0 {
		spec.Restart.Period = defaultRestartPeriod
	}

	// validate child spec list
	if len(spec.Children) == 0 {
		return fmt.Errorf("children list can not be empty")
	}

	s.handleChild = spec.EnableHandleChild

	duplicate := make(map[gen.Atom]bool)
	for _, s := range spec.Children {
		if err := validateChildSpec(s); err != nil {
			return err
		}
		_, dup := duplicate[s.Name]
		if dup {
			return ErrSupervisorChildDuplicate
		}
		duplicate[s.Name] = true
	}

	// create supervisor
	switch spec.Type {
	case SupervisorTypeOneForOne:
		s.sup = createSupOneForOne()
	case SupervisorTypeRestForOne:
		s.sup = createSupAllRestForOne()
	case SupervisorTypeAllForOne:
		s.sup = createSupAllRestForOne()
	case SupervisorTypeSimpleOneForOne:
		s.sup = createSupSimpleOneForOne()
	default:
		return fmt.Errorf("unknown supervisor type")
	}

	action, err := s.sup.init(spec)
	if err != nil {
		return err
	}

	s.children = make(map[gen.PID]gen.Atom)
	err = s.handleAction(action)
	if err != nil {
		return err
	}
	return nil
}

func (s *Supervisor) ProcessRun() (rr error) {
	var message *gen.MailboxMessage

	if lib.Recover() {
		defer func() {
			if r := recover(); r != nil {
				pc, fn, line, _ := runtime.Caller(2)

				s.Log().Panic("Supervisor got panic. Shutting down with reason: %#v at %s[%s:%d]",
					r, runtime.FuncForPC(pc).Name(), fn, line)

				action := s.sup.childTerminated(s.Name(), s.PID(), gen.TerminateReasonPanic)
				rr = s.handleAction(action)
			}
		}()
	}

	for {
		if s.State() != gen.ProcessStateRunning {
			// process was killed.
			return gen.TerminateReasonKill
		}

		if message != nil {
			gen.ReleaseMailboxMessage(message)
			message = nil
		}

		for {

			// check queues
			msg, ok := s.mailbox.Urgent.Pop()
			if ok {
				// got new urgent message. handle it
				message = msg.(*gen.MailboxMessage)
				break
			}

			// exit-signals are comming into the Urgent queue, if supervisor is
			// in supStateStrategy state we check this queue only
			// and skip the others.
			if s.state != supStateNormal {
				return nil
			}

			msg, ok = s.mailbox.System.Pop()
			if ok {
				// got new system message. handle it
				message = msg.(*gen.MailboxMessage)
				break
			}

			msg, ok = s.mailbox.Main.Pop()
			if ok {
				// got new regular message. handle it
				message = msg.(*gen.MailboxMessage)
				break
			}

			if _, ok := s.mailbox.Log.Pop(); ok {
				panic("supervisor process can not be a logger")
			}

			// no messages in the mailbox
			return nil
		}

		switch message.Type {

		case gen.MailboxMessageTypeRegular:
			var reason error
			if s.handleChild {
				switch m := message.Message.(type) {
				case supMessageChildStart:
					reason = s.behavior.HandleChildStart(m.name, m.pid)
				case supMessageChildTerminate:
					reason = s.behavior.HandleChildTerminate(m.name, m.pid, m.reason)
				default:
					reason = s.behavior.HandleMessage(message.From, message.Message)
				}
			} else {
				reason = s.behavior.HandleMessage(message.From, message.Message)
			}

			if reason != nil {
				action := s.sup.childTerminated(s.Name(), s.PID(), reason)
				if err := s.handleAction(action); err != nil {
					return err
				}
			}

		case gen.MailboxMessageTypeRequest:
			result, reason := s.behavior.HandleCall(message.From, message.Ref, message.Message)
			if result != nil {
				s.SendResponse(message.From, message.Ref, result)
			}
			if reason != nil {
				action := s.sup.childTerminated(s.Name(), s.PID(), reason)
				if err := s.handleAction(action); err != nil {
					return err
				}
			}

		case gen.MailboxMessageTypeEvent:
			if reason := s.behavior.HandleEvent(message.Message.(gen.MessageEvent)); reason != nil {
				return reason
			}

		case gen.MailboxMessageTypeExit:
			switch exit := message.Message.(type) {
			case gen.MessageExitPID:
				name, found := s.children[exit.PID]
				if found {
					delete(s.children, exit.PID)
					if s.handleChild {
						s.Send(s.PID(), supMessageChildTerminate{name, exit.PID, exit.Reason})
					}
				}
				action := s.sup.childTerminated(name, exit.PID, exit.Reason)
				if err := s.handleAction(action); err != nil {
					return err
				}

			case gen.MessageExitProcessID:
				reason := fmt.Errorf("%s: %w", exit.ProcessID, exit.Reason)
				action := s.sup.childTerminated(s.Name(), s.PID(), reason)
				if err := s.handleAction(action); err != nil {
					return err
				}

			case gen.MessageExitAlias:
				reason := fmt.Errorf("%s: %w", exit.Alias, exit.Reason)
				action := s.sup.childTerminated(s.Name(), s.PID(), reason)
				if err := s.handleAction(action); err != nil {
					return err
				}

			case gen.MessageExitEvent:
				reason := fmt.Errorf("%s: %w", exit.Event, exit.Reason)
				action := s.sup.childTerminated(s.Name(), s.PID(), reason)
				if err := s.handleAction(action); err != nil {
					return err
				}
			case gen.MessageExitNode:
				reason := fmt.Errorf("%s: %w", exit.Name, gen.ErrNoConnection)
				action := s.sup.childTerminated(s.Name(), s.PID(), reason)
				if err := s.handleAction(action); err != nil {
					return err
				}

			default:
				panic(fmt.Sprintf("unknown exit message: %#v", exit))
			}

		case gen.MailboxMessageTypeInspect:
			result := s.behavior.HandleInspect(message.From, message.Message.([]string)...)
			s.SendResponse(message.From, message.Ref, result)
		}
	}
}

//
// SupervisorBehavior default callbacks
//

func (s *Supervisor) HandleChildStart(name gen.Atom, pid gen.PID) error {
	s.Log().Warning("Supervisor.HandleChildStart: unhandled message")
	return nil
}

func (s *Supervisor) HandleChildTerminate(name gen.Atom, pid gen.PID, reason error) error {
	s.Log().Warning("Supervisor.HandleChildTerminate: unhandled message")
	return nil
}

func (s *Supervisor) HandleMessage(from gen.PID, message any) error {
	s.Log().Warning("Supervisor.HandleMessage: unhandled message from %s", from)
	return nil
}

func (s *Supervisor) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	s.Log().Warning("Supervisor.HandleCall: unhandled request from %s", from)
	return nil, nil
}
func (s *Supervisor) HandleEvent(message gen.MessageEvent) error {
	s.Log().Warning("Supervisor.HandleEvent: unhandled event message %#v", message)
	return nil
}

func (s *Supervisor) HandleInspect(from gen.PID, item ...string) map[string]string {
	return nil
}

func (s *Supervisor) Terminate(reason error) {}

//
//
//

func (s *Supervisor) handleAction(action supAction) error {
	for {
		switch action.do {
		case supActionDoNothing:
			s.state = supStateNormal
			break

		case supActionStartChild:
			s.state = supStateStrategy
			var pid gen.PID
			var err error

			action.spec.Options.LinkChild = true
			action.spec.Options.LinkParent = true

			if action.spec.register {
				pid, err = s.SpawnRegister(
					action.spec.Name,
					action.spec.Factory,
					action.spec.Options,
					action.spec.Args...)
			} else {
				pid, err = s.Spawn(action.spec.Factory, action.spec.Options, action.spec.Args...)
			}

			if err != nil {
				s.state = supStateNormal
				return err
			}

			if s.handleChild {
				s.Send(s.PID(), supMessageChildStart{action.spec.Name, pid})
			}

			s.children[pid] = action.spec.Name
			action = s.sup.childStarted(action.spec, pid)
			continue

		case supActionTerminateChildren:
			if len(action.terminate) == 0 {
				// no children. terminate this process
				return action.reason
			}
			// on disabling child spec
			s.state = supStateStrategy
			for _, pid := range action.terminate {
				// TODO
				// Child is disabling if exceeded restart limits. basically we dont need
				// to send exit again.
				// Current workaround: SendExit return error if its terminated already,
				// so dont log misleading message
				if err := s.SendExit(pid, action.reason); err == nil {
					s.Log().Info("Supervisor: terminate children %s", pid)
				}
			}
			s.state = supStateNormal
			return nil

		case supActionTerminate:
			return action.reason

		default:
			panic("unknown supAction")
		}

		break
	}
	return nil
}

func (s *Supervisor) ProcessTerminate(reason error) {
	s.behavior.Terminate(reason)
}

//
// internals
//

type supBehavior interface {
	init(spec SupervisorSpec) (supAction, error)

	childAddSpec(spec SupervisorChildSpec) (supAction, error)

	childSpec(name gen.Atom) (supAction, error)
	childStarted(spec supChildSpec, pid gen.PID) supAction
	childTerminated(name gen.Atom, pid gen.PID, reason error) supAction

	childEnable(name gen.Atom) (supAction, error)
	childDisable(name gen.Atom) (supAction, error)

	children() []SupervisorChild
}

type supState int

const (
	supStateNormal   = 0
	supStateStrategy = 1
)

type supActionType int

const (
	// just relax
	supActionDoNothing supActionType = 0
	//	start child
	supActionStartChild supActionType = 1
	// just stop children (on disabling child spec)
	supActionTerminateChildren supActionType = 2
	// stop children due to restart strategy activated
	supActionTerminateChildrenStrategy supActionType = 3
	supActionTerminate                 supActionType = 4
)

type supAction struct {
	do supActionType

	// for supActionStartChild
	spec supChildSpec

	// for supActionTerminateChildren
	terminate []gen.PID
	reason    error
}

type supMessageChildStart struct {
	name gen.Atom
	pid  gen.PID
}

type supMessageChildTerminate struct {
	name   gen.Atom
	pid    gen.PID
	reason error
}

// checkRestartIntensity returns true if exceeded
func supCheckRestartIntensity(restarts []int64, period int, intensity int) ([]int64, bool) {
	restarts = append(restarts, time.Now().Unix())
	if len(restarts) <= intensity {
		return restarts, false
	}

	p := int(time.Now().Unix() - restarts[0])
	if p > period {
		restarts = restarts[1:]
		return restarts, false
	}
	return restarts, true
}

func validateChildSpec(s SupervisorChildSpec) error {
	if s.Name == "" {
		return fmt.Errorf("invalid child spec Name")
	}

	if s.Factory == nil {
		return fmt.Errorf("child spec Factory is nil")
	}

	return nil
}

type supChild struct {
	pid  gen.PID
	spec supChildSpec
}

type supChildSpec struct {
	SupervisorChildSpec
	register bool
	disabled bool
	i        int
	pid      gen.PID
}

func sortSupChild(c []supChild) []SupervisorChild {
	var children []SupervisorChild

	if len(c) == 0 {
		return children
	}

	sort.Slice(c, func(i, j int) bool {
		if c[i].spec.i == c[j].spec.i {
			return c[i].pid.ID < c[j].pid.ID
		}
		return c[i].spec.i < c[j].spec.i
	})

	for _, v := range c {
		child := SupervisorChild{
			Spec:        v.spec.Name,
			PID:         v.pid,
			Significant: v.spec.Significant,
			Disabled:    v.spec.disabled,
		}
		if v.spec.register {
			child.Name = v.spec.Name
		}
		children = append(children, child)
	}
	return children
}
