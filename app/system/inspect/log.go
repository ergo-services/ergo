package inspect

import (
	"fmt"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

func factory_ilog() gen.ProcessBehavior {
	return &ilog{}
}

type ilog struct {
	act.Actor
	token gen.Ref

	generating bool
}

func (il *ilog) Init(args ...any) error {
	il.Log().SetLogger("default")
	il.Log().Debug("log inspector started")
	// RegisterEvent is not allowed here
	il.Send(il.PID(), register{})
	return nil
}

// as soon this process registered as a logger it is not able to use Log()
// method anymore

func (il *ilog) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case requestInspect:
		response := ResponseInspectLog{
			Event: gen.Event{
				Name: inspectLog,
				Node: il.Node().Name(),
			},
		}
		il.SendResponse(m.pid, m.ref, response)

	case register:
		eopts := gen.EventOptions{
			Notify: true,
		}
		token, err := il.RegisterEvent(inspectLog, eopts)
		if err != nil {
			return err
		}

		il.token = token
		il.SendAfter(il.PID(), shutdown{}, inspectLogIdlePeriod)

	case shutdown:
		if il.generating {
			break // ignore.
		}
		return gen.TerminateReasonNormal

	case gen.MessageEventStart: // got first subscriber
		// register this process as a logger
		il.Log().Debug("add this process as a logger")
		il.Node().LoggerAddPID(il.PID(), il.PID().String(), inspectLogFilter...)
		// we cant use Log() method while this process registered as a logger
		il.generating = true

	case gen.MessageEventStop: // no subscribers
		// unregister this process as a logger
		il.Node().LoggerDeletePID(il.PID())
		// now we can use Log() method
		il.Log().Debug("removed this process as a logger")
		il.generating = false
		il.SendAfter(il.PID(), shutdown{}, inspectLogIdlePeriod)
	}

	return nil
}

func (il *ilog) HandleLog(message gen.MessageLog) error {
	switch m := message.Source.(type) {
	case gen.MessageLogNode:
		// handle message
		ev := MessageInspectLogNode{
			Node:      m.Node,
			Creation:  m.Creation,
			Timestamp: message.Time.UnixNano(),
			Level:     message.Level,
			Message:   fmt.Sprintf(message.Format, message.Args...),
		}
		if err := il.SendEvent(inspectLog, il.token, ev); err != nil {
			return gen.TerminateReasonNormal
		}
	case gen.MessageLogProcess:
		// handle message
		ev := MessageInspectLogProcess{
			Node:      m.Node,
			Name:      m.Name,
			PID:       m.PID,
			Timestamp: message.Time.UnixNano(),
			Level:     message.Level,
			Message:   fmt.Sprintf(message.Format, message.Args...),
		}
		if err := il.SendEvent(inspectLog, il.token, ev); err != nil {
			return gen.TerminateReasonNormal
		}

	case gen.MessageLogMeta:
		// handle message
		ev := MessageInspectLogMeta{
			Node:      m.Node,
			Parent:    m.Parent,
			Meta:      m.Meta,
			Timestamp: message.Time.UnixNano(),
			Level:     message.Level,
			Message:   fmt.Sprintf(message.Format, message.Args...),
		}

		if err := il.SendEvent(inspectLog, il.token, ev); err != nil {
			return gen.TerminateReasonNormal
		}
	case gen.MessageLogNetwork:
		ev := MessageInspectLogNetwork{
			Node:      m.Node,
			Peer:      m.Peer,
			Timestamp: message.Time.UnixNano(),
			Level:     message.Level,
			Message:   fmt.Sprintf(message.Format, message.Args...),
		}
		if err := il.SendEvent(inspectLog, il.token, ev); err != nil {
			return gen.TerminateReasonNormal
		}
	}
	// ignore any other log messages
	// TODO should we handle them?
	return nil
}

func (il *ilog) Terminate(reason error) {
	// since this process is already unregistered
	// it is also unregistered as a logger
	// so we can use Log() here
	il.Log().Debug("log inspector terminated: %s", reason)
}
