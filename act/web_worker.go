package act

import (
	"fmt"
	"net/http"
	"reflect"
	"runtime"
	"strings"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
	"ergo.services/ergo/meta"
)

// WebWorker interface

type WebWorkerBehavior interface {
	gen.ProcessBehavior

	// Init invoked on a spawn WebWorker for the initializing.
	Init(args ...any) error

	// HandleMessage invoked if WebWorker received a message sent with gen.Process.Send(...).
	// Non-nil value of the returning error will cause termination of this process.
	// To stop this process normally, return gen.TerminateReasonNormal
	// or any other for abnormal termination.
	HandleMessage(from gen.PID, message any) error

	// HandleCall invoked if WebWorker got a synchronous request made with gen.Process.Call(...).
	// Return nil as a result to handle this request asynchronously and
	// to provide the result later using the gen.Process.SendResponse(...) method.
	HandleCall(from gen.PID, ref gen.Ref, request any) (any, error)

	// Terminate invoked on a termination process
	Terminate(reason error)

	// HandleEvent invoked on an event message if this process got subscribed on
	// this event using gen.Process.LinkEvent or gen.Process.MonitorEvent
	HandleEvent(message gen.MessageEvent) error

	// HandleInspect invoked on the request made with gen.Process.Inspect(...)
	HandleInspect(from gen.PID, item ...string) map[string]string

	// HandleGet invoked on a GET request
	HandleGet(from gen.PID, writer http.ResponseWriter, request *http.Request) error
	// HandlePOST invoked on a POST request
	HandlePost(from gen.PID, writer http.ResponseWriter, request *http.Request) error
	// HandlePut invoked on a PUT request
	HandlePut(from gen.PID, writer http.ResponseWriter, request *http.Request) error
	// HandlePatch invoked on a PATCH request
	HandlePatch(from gen.PID, writer http.ResponseWriter, request *http.Request) error
	// HandleDelete invoked on a DELETE request
	HandleDelete(from gen.PID, writer http.ResponseWriter, request *http.Request) error
	// HandleHead invoked on a HEAD request
	HandleHead(from gen.PID, writer http.ResponseWriter, request *http.Request) error
	// HandleOptions invoked on an OPTIONS request
	HandleOptions(from gen.PID, writer http.ResponseWriter, request *http.Request) error
}

type WebWorker struct {
	gen.Process

	behavior WebWorkerBehavior
	mailbox  gen.ProcessMailbox
}

func (w *WebWorker) ProcessInit(process gen.Process, args ...any) (rr error) {
	var ok bool
	if w.behavior, ok = process.Behavior().(WebWorkerBehavior); ok == false {
		unknown := strings.TrimPrefix(reflect.TypeOf(process.Behavior()).String(), "*")
		return fmt.Errorf("ProcessInit: not a WebWorkerBehavior %s", unknown)
	}
	w.Process = process
	w.mailbox = process.Mailbox()

	if lib.Recover() {
		defer func() {
			if r := recover(); r != nil {
				pc, fn, line, _ := runtime.Caller(2)
				w.Log().Panic("WebWorker initialization failed. Panic reason: %#v at %s[%s:%d]",
					r, runtime.FuncForPC(pc).Name(), fn, line)
				rr = gen.TerminateReasonPanic
			}
		}()
	}

	return w.behavior.Init(args...)
}

func (w *WebWorker) ProcessRun() (rr error) {
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
			if r, ok := message.Message.(meta.MessageWebRequest); ok {
				var reason error
				switch r.Request.Method {
				case "GET":
					reason = w.behavior.HandleGet(message.From, r.Response, r.Request)
				case "POST":
					reason = w.behavior.HandlePost(message.From, r.Response, r.Request)
				case "PUT":
					reason = w.behavior.HandlePut(message.From, r.Response, r.Request)
				case "PATCH":
					reason = w.behavior.HandlePatch(message.From, r.Response, r.Request)
				case "DELETE":
					reason = w.behavior.HandleDelete(message.From, r.Response, r.Request)
				case "HEAD":
					reason = w.behavior.HandleHead(message.From, r.Response, r.Request)
				case "OPTIONS":
					reason = w.behavior.HandleOptions(message.From, r.Response, r.Request)
				default:
					http.Error(r.Response,
						"unknown request type: "+r.Request.Method,
						http.StatusNotImplemented)
				}
				r.Done()
				if reason != nil {
					return reason
				}
				continue
			}

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
			result := w.behavior.HandleInspect(message.From, message.Message.([]string)...)
			w.SendResponse(message.From, message.Ref, result)
		}

	}
}

func (w *WebWorker) ProcessTerminate(reason error) {
	w.behavior.Terminate(reason)
}

// default callbacks for WebWorkerBehavior interface
func (w *WebWorker) Init(args ...any) error {
	return nil
}
func (w *WebWorker) HandleMessage(from gen.PID, message any) error {
	w.Log().Warning("WebWorker.HandleMessage: unhandled message from %s", from)
	return nil
}
func (w *WebWorker) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	w.Log().Warning("WebWorker.HandleCall: unhandled request from %s", from)
	return nil, nil
}

func (w *WebWorker) HandleEvent(message gen.MessageEvent) error {
	w.Log().Warning("WebWorker.HandleEvent: unhandled event message %#v", message)
	return nil
}

func (w *WebWorker) Terminate(reason error) {}
func (w *WebWorker) HandleInspect(from gen.PID, item ...string) map[string]string {
	return nil
}
func (w *WebWorker) HandleGet(from gen.PID, writer http.ResponseWriter, request *http.Request) error {
	w.Log().Warning("WebWorker.HandleGet: unhandled request from %s with URI: %s", from, request.RequestURI)
	http.Error(writer, "unhandled request", http.StatusNotImplemented)
	return nil
}
func (w *WebWorker) HandlePost(from gen.PID, writer http.ResponseWriter, request *http.Request) error {
	w.Log().Warning("WebWorker.HandlePost: unhandled request from %s with URI: %s", from, request.RequestURI)
	http.Error(writer, "unhandled request", http.StatusNotImplemented)
	return nil
}
func (w *WebWorker) HandlePut(from gen.PID, writer http.ResponseWriter, request *http.Request) error {
	w.Log().Warning("WebWorker.HandlePut: unhandled request from %s with URI: %s", from, request.RequestURI)
	http.Error(writer, "unhandled request", http.StatusNotImplemented)
	return nil
}
func (w *WebWorker) HandlePatch(from gen.PID, writer http.ResponseWriter, request *http.Request) error {
	w.Log().Warning("WebWorker.HandlePatch: unhandled request from %s with URI: %s", from, request.RequestURI)
	http.Error(writer, "unhandled request", http.StatusNotImplemented)
	return nil
}
func (w *WebWorker) HandleDelete(from gen.PID, writer http.ResponseWriter, request *http.Request) error {
	w.Log().Warning("WebWorker.HandleDelete: unhandled request from %s with URI: %s", from, request.RequestURI)
	http.Error(writer, "unhandled request", http.StatusNotImplemented)
	return nil
}
func (w *WebWorker) HandleHead(from gen.PID, writer http.ResponseWriter, request *http.Request) error {
	w.Log().Warning("WebWorker.HandleHead: unhandled request from %s with URI: %s", from, request.RequestURI)
	http.Error(writer, "unhandled request", http.StatusNotImplemented)
	return nil
}
func (w *WebWorker) HandleOptions(from gen.PID, writer http.ResponseWriter, request *http.Request) error {
	w.Log().Warning("WebWorker.HandleOptions: unhandled request from %s for: %s", from, request.RequestURI)
	http.Error(writer, "unhandled request", http.StatusNotImplemented)
	return nil
}
