package act

import (
	"fmt"
	"reflect"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

// PoolBehavior interface
const (
	defaultPoolSize = 3
)

// PoolBehavior is the interface a Pool implementation must satisfy. Normal
// priority traffic is forwarded to a worker and reaches no callback here, high
// and max priority traffic is handled by the Pool itself.
type PoolBehavior interface {
	gen.ProcessBehavior

	// Init invoked on a spawn Pool for the initializing.
	Init(args ...any) (PoolOptions, error)

	// HandleMessage invoked for a message sent with MessagePriorityHigh or Max.
	// Non-nil value of the returning error will cause termination of this process.
	// To stop this process normally, return gen.TerminateReasonNormal
	// or any other for abnormal termination.
	HandleMessage(from gen.PID, message any) error

	// HandleCall invoked for a request made with MessagePriorityHigh or Max.
	// Return nil as a result to handle this request asynchronously and
	// to provide the result later using the gen.Process.SendResponse(...) method.
	HandleCall(from gen.PID, ref gen.Ref, request any) (any, error)

	// HandleForwardFailed invoked if no worker took the message. It is dropped
	// once this returns, a caller of Call is already answered with the reason.
	// Non-nil value of the returning error will cause termination of this process.
	HandleForwardFailed(from gen.PID, message any, reason error) error

	// Terminate invoked on a termination process
	Terminate(reason error)

	// HandleEvent invoked on an event message if this process got subscribed on
	// this event using gen.Process.LinkEvent or gen.Process.MonitorEvent
	HandleEvent(message gen.MessageEvent) error

	// HandleInspect invoked on the request made with gen.Process.Inspect(...).
	// The returning fields are merged into the pool stats, so implement it to
	// add the fields of your own. The pool stats use the reserved "ergo:"
	// prefix for its keys; a returning field with such a key overrides it.
	HandleInspect(from gen.PID, item ...string) map[string]string
}

type Pool struct {
	gen.Process

	behavior        PoolBehavior
	mailbox         gen.ProcessMailbox
	sWorkerBehavior string
	forwarded       uint64
	restarts        uint64
	unhandled       uint64

	options PoolOptions
	pool    lib.QueueMPSC

	spanStart int64 // handler-entry time for the Processed span interval
}

// ProcessKind reports this process as built on act.Pool.
func (p *Pool) ProcessKind() gen.ProcessKind {
	return gen.ProcessKindPool
}

type PoolOptions struct {
	WorkerMailboxSize int64
	PoolSize          int64
	WorkerFactory     gen.ProcessFactory
	WorkerArgs        []any
}

func (p *Pool) AddWorkers(n int) (int64, error) {
	if p.State() != gen.ProcessStateRunning {
		return 0, gen.ErrNotAllowed
	}

	wopt := gen.ProcessOptions{
		MailboxSize: p.options.WorkerMailboxSize,
		LinkParent:  true,
	}
	for i := 0; i < n; i++ {
		pid, err := p.Spawn(p.options.WorkerFactory, wopt, p.options.WorkerArgs...)
		if err != nil {
			return 0, err
		}
		p.pool.Push(pid)
	}

	return p.pool.Len(), nil
}

func (p *Pool) RemoveWorkers(n int) (int64, error) {
	if p.State() != gen.ProcessStateRunning {
		return 0, gen.ErrNotAllowed
	}
	for i := 0; i < n; i++ {
		v, ok := p.pool.Pop()
		if ok == false {
			return 0, ErrPoolEmpty
		}
		pid := v.(gen.PID)
		p.SendExit(pid, gen.TerminateReasonNormal)
	}

	return p.pool.Len(), nil
}

func (p *Pool) ProcessInit(process gen.Process, args ...any) (rr error) {
	var ok bool

	if p.behavior, ok = process.Behavior().(PoolBehavior); ok == false {
		return fmt.Errorf("ProcessInit: not a PoolBehavior %s", process.BehaviorName())
	}
	p.Process = process
	p.mailbox = process.Mailbox()

	if lib.Recover() {
		defer func() {
			if r := recover(); r != nil {
				p.Log().Panic("Pool initialization failed. Panic reason: %#v at %s",
					r, lib.PanicOrigin())
				rr = gen.TerminateReasonPanic
			}
		}()
	}

	options, err := p.behavior.Init(args...)
	if err != nil {
		return err
	}
	if options.PoolSize < 1 {
		options.PoolSize = defaultPoolSize
	}
	p.options = options

	p.pool = lib.NewQueueLimitMPSC(options.PoolSize * 100)
	wopt := gen.ProcessOptions{
		MailboxSize: options.WorkerMailboxSize,
		LinkParent:  true,
	}
	for i := int64(0); i < options.PoolSize; i++ {
		pid, err := p.Spawn(options.WorkerFactory, wopt, options.WorkerArgs...)
		if err != nil {
			return err
		}

		p.pool.Push(pid)
		if i == 0 {
			pi, _ := p.Node().ProcessInfo(pid)
			p.sWorkerBehavior = pi.Behavior
		}
	}

	return nil
}

func (p *Pool) ProcessRun() (rr error) {
	var message *gen.MailboxMessage

	if lib.Recover() {
		defer func() {
			if r := recover(); r != nil {
				p.Log().Panic("Pool terminated. Panic reason: %#v at %s",
					r, lib.PanicOrigin())
				rr = gen.TerminateReasonPanic
			}
		}()
	}

	for {
		if p.State() != gen.ProcessStateRunning {
			// process was killed by the node.
			return gen.TerminateReasonKill
		}

		if message != nil {
			gen.ReleaseMailboxMessage(message)
			message = nil
		}

		for {
			// check queues
			msg, ok := p.mailbox.Urgent.Pop()
			if ok {
				// got new urgent message. handle it
				message = msg.(*gen.MailboxMessage)
				break
			}

			msg, ok = p.mailbox.System.Pop()
			if ok {
				// got new system message. handle it
				message = msg.(*gen.MailboxMessage)
				break
			}

			msg, ok = p.mailbox.Main.Pop()
			if ok {
				// got new regular message. handle it
				message = msg.(*gen.MailboxMessage)
				if message.Type < gen.MailboxMessageTypeExit {
					// MailboxMessageTypeRegular, MailboxMessageTypeRequest, MailboxMessageTypeEvent, MailboxMessageTypeSpan
					ferr := p.forward(message)
					if ferr == nil {
						// it shouldn't be "released" back to the pool
						message = nil
						continue
					}
					p.unhandled++
					if message.Type == gen.MailboxMessageTypeRequest {
						p.SendResponseError(message.From, message.Ref, ferr)
					}
					reason := p.behavior.HandleForwardFailed(message.From, message.Message, ferr)
					gen.ReleaseMailboxMessage(message)
					message = nil
					if reason != nil {
						return reason
					}
					continue
				}

				break
			}

			if _, ok := p.mailbox.Log.Pop(); ok {
				panic("pool process can not be a logger")
			}

			// no messages in the mailbox
			return nil
		}

		switch message.Type {
		case gen.MailboxMessageTypeRegular:
			messageHasTracing := message.Tracing.ID != [2]uint64{}
			if messageHasTracing {
				p.SetPropagatingTrace(message.Tracing)
				p.spanStart = time.Now().UnixNano()
			}

			if reason := p.behavior.HandleMessage(message.From, message.Message); reason != nil {
				p.sendSpanProcessed(message, gen.TracingKindSend, reason.Error())
				return reason
			}
			p.sendSpanProcessed(message, gen.TracingKindSend, "")

			if messageHasTracing {
				p.SetPropagatingTrace(gen.Tracing{})
			}

		case gen.MailboxMessageTypeRequest:
			messageHasTracing := message.Tracing.ID != [2]uint64{}
			if messageHasTracing {
				p.SetPropagatingTrace(message.Tracing)
				p.spanStart = time.Now().UnixNano()
			}

			var reason error
			var result any

			result, reason = p.behavior.HandleCall(message.From, message.Ref, message.Message)

			if reason != nil {
				if reason == gen.TerminateReasonNormal && result != nil {
					p.sendSpanProcessed(message, gen.TracingKindRequest, "")
					p.SendResponse(message.From, message.Ref, result)
				} else {
					p.sendSpanProcessed(message, gen.TracingKindRequest, reason.Error())
				}
				return reason
			}

			if result == nil {
				p.sendSpanProcessed(message, gen.TracingKindRequest, "")
				if messageHasTracing {
					p.SetPropagatingTrace(gen.Tracing{})
				}
				continue
			}

			p.sendSpanProcessed(message, gen.TracingKindRequest, "")
			p.SendResponse(message.From, message.Ref, result)

			if messageHasTracing {
				p.SetPropagatingTrace(gen.Tracing{})
			}

		case gen.MailboxMessageTypeEvent:
			if reason := p.behavior.HandleEvent(message.Message.(gen.MessageEvent)); reason != nil {
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
			items := message.Message.([]string)
			// pool stats first, the behavior may override any of the fields
			result := p.inspect()
			for k, v := range p.behavior.HandleInspect(message.From, items...) {
				result[k] = v
			}
			p.SendResponse(message.From, message.Ref, result)

		}

	}
}

func (p *Pool) ProcessTerminate(reason error) {
	p.behavior.Terminate(reason)
}

//
// default callbacks for PoolBehavior interface
//

func (p *Pool) HandleMessage(from gen.PID, message any) error {
	p.Log().Warning("Pool.HandleMessage: unhandled message from %s", from)
	return nil
}

func (p *Pool) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	p.Log().Warning("Pool.HandleCall: unhandled request from %s", from)
	return nil, nil
}

func (p *Pool) HandleForwardFailed(from gen.PID, message any, reason error) error {
	p.Log().Error("no available worker process. ignored message from %s: %s", from, reason)
	return nil
}

func (p *Pool) Terminate(reason error) {}

func (p *Pool) HandleEvent(message gen.MessageEvent) error {
	p.Log().Warning("Pool.HandleEvent: unhandled event message %#v", message)
	return nil
}

func (p *Pool) sendSpanProcessed(message *gen.MailboxMessage, kind gen.TracingKind, errStr string) {
	if message.Tracing.ID == [2]uint64{} {
		return
	}
	var msgType string
	if message.Message != nil {
		msgType = reflect.TypeOf(message.Message).String()
	}
	p.SendTracingSpan(gen.TracingSpan{
		TraceID:      message.Tracing.ID,
		SpanID:       message.Tracing.SpanID,
		Point:        gen.TracingPointProcessed,
		Kind:         kind,
		Timestamp:    p.spanStart,
		EndTimestamp: time.Now().UnixNano(),
		Node:         p.Node().Name(),
		From:         message.From,
		To:           p.PID(),
		Ref:          message.Ref,
		Message:      msgType,
		Error:        errStr,
		Attributes:   p.TracingAttributes(),
	})
	p.CloseTracingSpans()
	p.ClearTracingSpanAttributes()
}

func (p *Pool) HandleInspect(from gen.PID, item ...string) map[string]string {
	return p.inspect()
}

// private

func (p *Pool) inspect() map[string]string {
	return map[string]string{
		"ergo:pool_size":           fmt.Sprintf("%d", p.options.PoolSize),
		"ergo:worker_behavior":     p.sWorkerBehavior,
		"ergo:worker_mailbox_size": fmt.Sprintf("%d", p.options.WorkerMailboxSize),
		"ergo:worker_restarts":     fmt.Sprintf("%d", p.restarts),
		"ergo:messages_forwarded":  fmt.Sprintf("%d", p.forwarded),
		"ergo:messages_unhandled":  fmt.Sprintf("%d", p.unhandled),
	}
}

// forward hands the message to a worker; on error the message is still ours.
func (p *Pool) forward(message *gen.MailboxMessage) error {
	l := p.pool.Len()
	if l == 0 {
		return gen.ErrProcessUnknown
	}
	var err error
	for i := int64(0); i < l; i++ {
		err = nil
		v, _ := p.pool.Pop()
		pid := v.(gen.PID)
		err = p.Forward(pid, message, gen.MessagePriorityNormal)
		if err == nil {
			// back to pool
			p.pool.Push(v)
			p.forwarded++
			return nil
		}
		if err == gen.ErrProcessUnknown || err == gen.ErrProcessTerminated {
			// restart
			wopt := gen.ProcessOptions{
				MailboxSize: p.options.WorkerMailboxSize,
				LinkParent:  true,
			}
			spawned, serr := p.Spawn(p.options.WorkerFactory, wopt, p.options.WorkerArgs...)
			if serr != nil {
				p.Log().Error("unable to spawn new worker process: %s", serr)
				err = serr
				continue
			}
			p.restarts++
			err = p.Forward(spawned, message, gen.MessagePriorityNormal)
			p.pool.Push(spawned)
			if err != nil {
				p.Log().Error("unable to forward to the respawned worker %s: %s", spawned, err)
				continue
			}
			p.forwarded++
			return nil
		}

		// mailbox is full. try next worker
		p.pool.Push(v)
	}
	return err
}
