package inspect

import (
	"fmt"
	"strings"
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

	levels         []gen.LogLevel
	limit          int
	messagePattern string // lower-cased for fast matching
	messageExclude bool
	generating     bool
	loopID         uint64

	// ring buffer
	ring     []InspectLogEntry
	pos      int
	full     bool
	received int64
}

const logFlushInterval = time.Second

func (il *log) Init(args ...any) error {
	il.levels = args[0].([]gen.LogLevel)
	il.limit = args[1].(int)
	if len(args) > 3 {
		il.messagePattern = strings.ToLower(args[2].(string))
		il.messageExclude = args[3].(bool)
	}
	il.ring = make([]InspectLogEntry, il.limit)
	il.Log().SetLogger("default")
	il.Log().Debug("log inspector started (limit: %d)", il.limit)
	il.SetCompression(true)

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
		if il.received == 0 {
			il.SendAfter(il.PID(), flushLog{id: il.loopID}, logFlushInterval)
			break
		}

		// collect entries from ring buffer in correct order
		var entries []InspectLogEntry
		if il.full {
			// ring wrapped: oldest at pos, newest at pos-1
			entries = make([]InspectLogEntry, il.limit)
			copy(entries, il.ring[il.pos:])
			copy(entries[il.limit-il.pos:], il.ring[:il.pos])
		} else {
			entries = make([]InspectLogEntry, il.pos)
			copy(entries, il.ring[:il.pos])
		}

		suppressed := il.received - int64(len(entries))
		if suppressed < 0 {
			suppressed = 0
		}

		ev := MessageInspectLog{
			Node:       il.Node().Name(),
			Entries:    entries,
			Suppressed: suppressed,
		}

		// reset ring
		il.pos = 0
		il.full = false
		il.received = 0

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

	case shutdown:
		if il.generating {
			break // ignore
		}
		return gen.TerminateReasonNormal

	case gen.MessageEventStart: // got first subscriber
		il.Log().Debug("add this process as a logger")
		il.Node().LoggerAddPID(il.PID(), il.PID().String(), il.levels...)
		il.loopID++
		il.generating = true
		il.SendAfter(il.PID(), flushLog{id: il.loopID}, logFlushInterval)

	case gen.MessageEventStop: // no subscribers
		il.Node().LoggerDeletePID(il.PID())
		il.Log().Debug("removed this process as a logger")
		il.generating = false
		il.pos = 0
		il.full = false
		il.received = 0
		il.SendAfter(il.PID(), shutdown{}, inspectLogIdlePeriod)
	}

	return nil
}

func (il *log) HandleLog(message gen.MessageLog) error {
	msg := fmt.Sprintf(message.Format, message.Args...)

	if il.messagePattern != "" {
		contains := strings.Contains(strings.ToLower(msg), il.messagePattern)
		if il.messageExclude == contains {
			return nil
		}
	}

	entry := InspectLogEntry{
		Timestamp: message.Time.UnixNano(),
		Level:     message.Level,
		Message:   msg,
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
	case gen.MessageLogApplication:
		entry.Source = "application"
		entry.Name = m.Name
		entry.Behavior = m.Behavior
	}

	il.ring[il.pos] = entry
	il.pos++
	if il.pos >= il.limit {
		il.pos = 0
		il.full = true
	}
	il.received++

	return nil
}

func (il *log) Terminate(reason error) {
	il.Log().Debug("log inspector terminated: %s", reason)
}
