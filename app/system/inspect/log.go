package inspect

import (
	"fmt"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

func factory_log() gen.ProcessBehavior {
	return &log{}
}

type log struct {
	act.Actor
	token gen.Ref
	event gen.Atom

	levels     []gen.LogLevel
	generating bool
	loopID     uint64
	buffer     []InspectLogEntry
}

const logFlushInterval = time.Second

func (il *log) Init(args ...any) error {
	il.levels = args[0].([]gen.LogLevel)
	il.Log().SetLogger("default")
	il.Log().Debug("log inspector started")
	// RegisterEvent is not allowed here
	il.Send(il.PID(), register{})
	return nil
}

// as soon this process registered as a logger it is not able to use Log()
// method anymore

func (il *log) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case flushLog:
		if m.id != il.loopID || il.generating == false {
			break
		}
		if len(il.buffer) == 0 {
			il.SendAfter(il.PID(), flushLog{id: il.loopID}, logFlushInterval)
			break
		}

		ev := MessageInspectLog{
			Node:    il.Node().Name(),
			Entries: il.buffer,
		}
		il.buffer = nil

		if err := il.SendEvent(il.event, il.token, ev); err != nil {
			return gen.TerminateReasonNormal
		}

		il.SendAfter(il.PID(), flushLog{id: il.loopID}, logFlushInterval)

	case requestInspect:
		response := ResponseInspectLog{
			Event: gen.Event{
				Name: il.event,
				Node: il.Node().Name(),
			},
		}
		il.SendResponse(m.pid, m.ref, response)

	case register:
		eopts := gen.EventOptions{
			Notify: true,
		}
		evname := gen.Atom(fmt.Sprintf("%s_%s", string(il.Name()), il.PID()))
		token, err := il.RegisterEvent(evname, eopts)
		if err != nil {
			return err
		}

		il.event = evname
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
		il.Node().LoggerAddPID(il.PID(), il.PID().String(), il.levels...)
		// we cant use Log() method while this process registered as a logger
		il.loopID++
		il.generating = true
		il.SendAfter(il.PID(), flushLog{id: il.loopID}, logFlushInterval)

	case gen.MessageEventStop: // no subscribers
		// unregister this process as a logger
		il.Node().LoggerDeletePID(il.PID())
		// now we can use Log() method
		il.Log().Debug("removed this process as a logger")
		il.generating = false
		il.buffer = nil
		il.SendAfter(il.PID(), shutdown{}, inspectLogIdlePeriod)
	}

	return nil
}

func (il *log) HandleLog(message gen.MessageLog) error {
	entry := InspectLogEntry{
		Timestamp: message.Time.UnixNano(),
		Level:     message.Level,
		Message:   fmt.Sprintf(message.Format, message.Args...),
		Fields:    message.Fields,
	}

	switch m := message.Source.(type) {
	case gen.MessageLogNode:
		entry.Source = "node"
		entry.Creation = m.Creation
	case gen.MessageLogProcess:
		entry.Source = "process"
		entry.Name = m.Name
		entry.PID = m.PID
		entry.Behavior = m.Behavior
	case gen.MessageLogMeta:
		entry.Source = "meta"
		entry.Parent = m.Parent
		entry.Meta = m.Meta
		entry.Behavior = m.Behavior
	case gen.MessageLogNetwork:
		entry.Source = "network"
		entry.Peer = gen.Atom(m.Peer.CRC32())
	}

	il.buffer = append(il.buffer, entry)
	return nil
}

func (il *log) Terminate(reason error) {
	// since this process is already unregistered
	// it is also unregistered as a logger
	// so we can use Log() here
	il.Log().Debug("log inspector terminated: %s", reason)
}
