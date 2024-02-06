package act

import (
	"errors"
	"fmt"
	"net/http"
	"runtime"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
	"ergo.services/ergo/meta"
)

// WebBehavior interface

type WebBehavior interface {
	gen.ProcessBehavior

	// Init invoked on a spawn Web for the initializing.
	Init(args ...any) (WebOptions, error)

	// HandleMessage invoked if Web received a message sent with gen.Process.Send(...).
	// Non-nil value of the returning error will cause termination of this process.
	// To stop this process normally, return gen.TerminateReasonNormal
	// or any other for abnormal termination.
	HandleMessage(from gen.PID, message any) error

	// HandleCall invoked if Web got a synchronous request made with gen.Process.Call(...).
	// Return nil as a result to handle this request asynchronously and
	// to provide the result later using the gen.Process.SendResponse(...) method.
	HandleCall(from gen.PID, ref gen.Ref, request any) (any, error)

	// Terminate invoked on a termination process
	Terminate(reason error)

	// HandleEvent invoked on an event message if this process got subscribed on
	// this event using gen.Process.LinkEvent or gen.Process.MonitorEvent
	HandleEvent(message gen.MessageEvent) error

	// HandleInspect invoked on the request made with gen.Process.Inspect(...)
	HandleInspect(from gen.PID) map[string]string
}

type Web struct {
	gen.Process

	behavior WebBehavior
	mailbox  gen.ProcessMailbox

	handler http.Handler
}

type WebOptions struct {
	Host        string
	Port        uint16
	CertManager gen.CertManager
	Handler     http.Handler
}

func (w *Web) ProcessInit(process gen.Process, args ...any) (rr error) {
	var ok bool

	if w.behavior, ok = process.Behavior().(WebBehavior); ok == false {
		return errors.New("ProcessInit: not a WebBehavior")
	}
	w.Process = process
	w.mailbox = process.Mailbox()

	if lib.Recover() {
		defer func() {
			if r := recover(); r != nil {
				pc, fn, line, _ := runtime.Caller(2)
				w.Log().Panic("Web initialization failed. Panic reason: %#v at %s[%s:%d]",
					r, runtime.FuncForPC(pc).Name(), fn, line)
				rr = gen.TerminateReasonPanic
			}
		}()
	}

	options, err := w.behavior.Init(args...)
	if err != nil {
		return err
	}

	mwo := meta.WebOptions{
		Host:        options.Host,
		Port:        options.Port,
		CertManager: options.CertManager,
		Handler:     options.Handler,
	}
	web, err := meta.CreateWeb(mwo)
	if err != nil {
		return err
	}

	if _, err := w.SpawnMeta(web, gen.MetaOptions{}); err != nil {
		return err
	}

	return nil
}

func (w *Web) ProcessRun() (rr error) {
	var message *gen.MailboxMessage

	if lib.Recover() {
		defer func() {
			if r := recover(); r != nil {
				pc, fn, line, _ := runtime.Caller(2)
				w.Log().Panic("Web terminated. Panic reason: %#v at %s[%s:%d]",
					r, runtime.FuncForPC(pc).Name(), fn, line)
				rr = gen.TerminateReasonPanic
			}
		}()
	}

	for {
		if w.State() != gen.ProcessStateRunning {
			// process was killed by the node.
			return gen.TerminateReasonKill
		}

		if message != nil {
			gen.ReleaseMailboxMessage(message)
			message = nil
		}

		for {
			// check queues
			msg, ok := w.mailbox.Urgent.Pop()
			if ok {
				// got new urgent message. handle it
				message = msg.(*gen.MailboxMessage)
				break
			}

			msg, ok = w.mailbox.System.Pop()
			if ok {
				// got new system message. handle it
				message = msg.(*gen.MailboxMessage)
				break
			}

			msg, ok = w.mailbox.Main.Pop()
			if ok {
				// got new regular message. handle it
				message = msg.(*gen.MailboxMessage)
				break
			}

			if _, ok := w.mailbox.Log.Pop(); ok {
				panic("web process can not be a logger")
			}

			// no messages in the mailbox
			return nil
		}

		switch message.Type {
		case gen.MailboxMessageTypeRegular:
			if reason := w.behavior.HandleMessage(message.From, message.Message); reason != nil {
				return reason
			}

		case gen.MailboxMessageTypeRequest:
			var reason error
			var result any

			result, reason = w.behavior.HandleCall(message.From, message.Ref, message.Message)

			if reason != nil {
				// if reason is "normal" and we got response - send it before termination
				if reason == gen.TerminateReasonNormal && result != nil {
					w.SendResponse(message.From, message.Ref, result)
				}
				return reason
			}

			if result == nil {
				// async handling of sync request. response could be sent
				// later, even by the other process
				continue
			}

			w.SendResponse(message.From, message.Ref, result)

		case gen.MailboxMessageTypeEvent:
			if reason := w.behavior.HandleEvent(message.Message.(gen.MessageEvent)); reason != nil {
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
			result := w.behavior.HandleInspect(message.From)
			w.SendResponse(message.From, message.Ref, result)
		}

	}
}

func (w *Web) ProcessTerminate(reason error) {
	w.behavior.Terminate(reason)
}

//
// default callbacks for WebBehavior interface
//

func (w *Web) HandleMessage(from gen.PID, message any) error {
	w.Log().Warning("Web.HandleMessage: unhandled message from %s", from)
	return nil
}
func (w *Web) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	w.Log().Warning("Web.HandleCall: unhandled request from %s", from)
	return nil, nil
}

func (w *Web) HandleEvent(message gen.MessageEvent) error {
	w.Log().Warning("Web.HandleEvent: unhandled event message %#v", message)
	return nil
}

func (w *Web) Terminate(reason error) {}
func (w *Web) HandleInspect(from gen.PID) map[string]string {
	return nil
}
