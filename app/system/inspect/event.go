package inspect

import (
	"fmt"
	"strings"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

func factory_event() gen.ProcessBehavior {
	return &event{}
}

type event struct {
	act.Actor
	token  gen.Ref
	event  gen.Atom  // own published event
	target gen.Event // watched event

	limit          int
	typePattern    string
	messagePattern string
	messageExclude bool
	forced         bool
	verbose        bool

	generating bool
	loopID     uint64

	watching            bool
	watchReason         string
	publishesRegardless bool
	passiveSeen         bool
	passivePublished    int64

	ring     []InspectEventEntry
	pos      int
	full     bool
	received int64
}

func (ie *event) Init(args ...any) error {
	name := args[0].(gen.Atom)
	ie.limit = args[1].(int)
	ie.typePattern = strings.ToLower(args[2].(string))
	ie.messagePattern = strings.ToLower(args[3].(string))
	ie.messageExclude = args[4].(bool)
	hash := args[5].(string)
	ie.forced = args[6].(bool)
	ie.verbose = args[7].(bool)

	ie.ring = make([]InspectEventEntry, ie.limit)
	ie.target = gen.Event{Name: name, Node: ie.Node().Name()}
	ie.watchReason = "idle_gated"
	ie.Log().SetLogger("default")
	ie.SetProcessKind(gen.ProcessKindMonitor)
	ie.SetCompression(true)

	eopts := gen.EventOptions{Notify: true, Buffer: 1}
	evname := gen.Atom(fmt.Sprintf("%s_%s", inspectEvent, hash))
	token, err := ie.RegisterEvent(evname, eopts)
	if err != nil {
		ie.Log().Error("unable to register event: %s", err)
		return err
	}
	ie.event = evname
	ie.token = token
	ie.SendAfter(ie.PID(), shutdown{}, inspectEventIdlePeriod)

	return nil
}

func (ie *event) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case flushEvent:
		if m.id != ie.loopID || ie.generating == false {
			break
		}
		if reason := ie.flush(); reason != nil {
			return reason
		}
		ie.SendAfter(ie.PID(), flushEvent{id: ie.loopID}, inspectEventPeriod)

	case requestInspect:
		response := ResponseInspectEvent{
			Event: gen.Event{Name: ie.event, Node: ie.Node().Name()},
		}
		if info, err := ie.Node().EventInfo(ie.target); err == nil {
			response.Info = info
			if buf, subscribed := ie.evaluate(info); subscribed {
				response.Buffer = buf
			}
		}
		response.Watching = ie.watching
		response.WatchReason = ie.watchReason
		ie.SendResponse(m.pid, m.ref, response)

	case shutdown:
		if ie.generating {
			break
		}
		return gen.TerminateReasonNormal

	case gen.MessageEventStart:
		ie.loopID++
		ie.generating = true
		ie.Send(ie.PID(), flushEvent{id: ie.loopID})

	case gen.MessageEventStop: // release the target so we never hold the producer
		ie.generating = false
		if ie.watching {
			ie.DemonitorEvent(ie.target)
			ie.watching = false
			ie.passiveSeen = false
		}
		ie.SendAfter(ie.PID(), shutdown{}, inspectEventIdlePeriod)

	case gen.MessageDownEvent: // target unregistered or producer gone
		if m.Event != ie.target {
			break
		}
		ie.sendClosed(m.Reason)
		return gen.TerminateReasonNormal

	default:
		ie.Log().Error("unknown message (ignored) %#v", message)
	}

	return nil
}

func (ie *event) HandleEvent(message gen.MessageEvent) error {
	if message.Event != ie.target {
		return nil
	}
	ie.capture(message)
	return nil
}

// evaluate subscribes/unsubscribes so the inspector never starts or holds a
// notify-gated producer; returns the buffer snapshot if it subscribed this call.
func (ie *event) evaluate(info gen.EventInfo) ([]InspectEventEntry, bool) {
	others := info.Subscribers
	if ie.watching {
		others-- // discount our own monitor
	}

	// while passive, a rising publish count at subs==0 means the producer ignores Notify
	if ie.watching == false && info.Notify && others <= 0 {
		if ie.passiveSeen == false {
			ie.passivePublished = info.MessagesPublished
			ie.passiveSeen = true
		} else if info.MessagesPublished > ie.passivePublished {
			ie.publishesRegardless = true
		}
	}

	want := ie.forced || info.Notify == false || others > 0 || ie.publishesRegardless

	if want && ie.watching == false {
		buf, err := ie.MonitorEvent(ie.target)
		if err != nil {
			ie.watchReason = "idle_gated"
			return nil, false
		}
		ie.watching = true
		ie.passiveSeen = false
		ie.watchReason = ie.reasonFor(info, others)
		entries := make([]InspectEventEntry, 0, len(buf))
		for _, em := range buf {
			if e, ok := ie.toEntry(em); ok {
				entries = append(entries, e)
			}
		}
		return entries, true
	}

	if want == false && ie.watching {
		ie.DemonitorEvent(ie.target) // let the producer get its Stop
		ie.watching = false
		ie.passiveSeen = false
		ie.watchReason = "idle_gated"
		return nil, false
	}

	if ie.watching {
		ie.watchReason = ie.reasonFor(info, others)
	} else {
		ie.watchReason = "idle_gated"
	}
	return nil, false
}

// reasonFor keeps "forced" last so it only shows when force is the sole reason.
func (ie *event) reasonFor(info gen.EventInfo, others int64) string {
	switch {
	case info.Notify == false:
		return "notify_off"
	case others > 0:
		return "other_subscribers"
	case ie.publishesRegardless:
		return "publishes_regardless"
	case ie.forced:
		return "forced"
	}
	return "other_subscribers"
}

func (ie *event) toEntry(em gen.MessageEvent) (InspectEventEntry, bool) {
	typ := fmt.Sprintf("%T", em.Message)
	readable := fmt.Sprintf("%+v", em.Message) // honors Stringer/error on every level
	verbose := fmt.Sprintf("%#v", em.Message)  // full Go-syntax; used for filtering and (optionally) sent

	if ie.typePattern != "" {
		if strings.Contains(strings.ToLower(typ), ie.typePattern) == false {
			return InspectEventEntry{}, false
		}
	}
	if ie.messagePattern != "" {
		// match against both forms so a search finds Stringer output as well as struct internals
		contains := strings.Contains(strings.ToLower(readable), ie.messagePattern) ||
			strings.Contains(strings.ToLower(verbose), ie.messagePattern)
		if ie.messageExclude == contains {
			return InspectEventEntry{}, false
		}
	}

	entry := InspectEventEntry{Timestamp: em.Timestamp, Type: typ, Message: readable}
	// only carry the verbose form when requested and it actually adds information,
	// so plain values (where %+v == %#v) never double the payload
	if ie.verbose && verbose != readable {
		entry.Verbose = verbose
	}
	return entry, true
}

func (ie *event) pushEntry(e InspectEventEntry) {
	ie.ring[ie.pos] = e
	ie.pos++
	if ie.pos >= ie.limit {
		ie.pos = 0
		ie.full = true
	}
	ie.received++
}

func (ie *event) capture(em gen.MessageEvent) {
	if e, ok := ie.toEntry(em); ok {
		ie.pushEntry(e)
	}
}

func (ie *event) flush() error {
	info, err := ie.Node().EventInfo(ie.target)
	if err != nil {
		ie.sendClosed(err)
		return gen.TerminateReasonNormal
	}

	if buf, subscribed := ie.evaluate(info); subscribed {
		for _, e := range buf {
			ie.pushEntry(e)
		}
	}

	var entries []InspectEventEntry
	if ie.full {
		entries = make([]InspectEventEntry, ie.limit)
		copy(entries, ie.ring[ie.pos:])
		copy(entries[ie.limit-ie.pos:], ie.ring[:ie.pos])
	} else if ie.pos > 0 {
		entries = make([]InspectEventEntry, ie.pos)
		copy(entries, ie.ring[:ie.pos])
	}

	suppressed := ie.received - int64(len(entries))
	if suppressed < 0 {
		suppressed = 0
	}

	ev := MessageInspectEvent{
		Node:        ie.Node().Name(),
		Info:        info,
		Entries:     entries,
		Suppressed:  suppressed,
		Watching:    ie.watching,
		WatchReason: ie.watchReason,
	}

	ie.pos = 0
	ie.full = false
	ie.received = 0

	if err := ie.SendEvent(ie.event, ie.token, ev); err != nil {
		return gen.TerminateReasonNormal
	}

	return nil
}

// sendClosed delivers the final closed batch then the caller terminates. Max priority
// puts this batch ahead of our own event-Down (high) that fires at termination, so the
// session shows "closed" before the subscription is dropped.
func (ie *event) sendClosed(reason error) {
	rs := ""
	if reason != nil {
		rs = reason.Error()
	}
	ev := MessageInspectEvent{
		Node:   ie.Node().Name(),
		Info:   gen.EventInfo{Event: ie.target},
		Closed: true,
		Reason: rs,
	}
	ie.SetSendPriority(gen.MessagePriorityMax)
	ie.SendEvent(ie.event, ie.token, ev)
}

func (ie *event) Terminate(reason error) {
	ie.Log().Debug("event inspector terminated: %s", reason)
}
