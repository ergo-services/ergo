package inspect

import (
	"fmt"
	"strings"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

func factory_event_stream() gen.ProcessBehavior {
	return &eventStream{}
}

type eventStreamArgs struct {
	Name           gen.Atom
	Limit          int
	TypePattern    string
	MessagePattern string
	MessageExclude bool
	Hash           string
	Force          bool
	Verbose        bool
}

// eventStream monitors the target (gated): a ring serves the backlog to every
// subscriber, per-tick deltas stream live with a suppressed count.
type eventStream struct {
	act.Actor
	token  gen.Ref
	event  gen.Atom
	target gen.Event

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

	ring       []InspectEventEntry
	pos        int
	full       bool
	received   int64
	flushed    int64
	suppressed int64
}

func (ie *eventStream) Init(args ...any) error {
	a := args[0].(eventStreamArgs)
	ie.limit = a.Limit
	ie.typePattern = strings.ToLower(a.TypePattern)
	ie.messagePattern = strings.ToLower(a.MessagePattern)
	ie.messageExclude = a.MessageExclude
	ie.forced = a.Force
	ie.verbose = a.Verbose

	ie.target = gen.Event{Name: a.Name, Node: ie.Node().Name()}
	ie.watchReason = "idle_gated"
	ie.Log().SetLogger("default")
	ie.SetProcessKind(gen.ProcessKindMonitor)
	ie.SetCompression(true)

	// ring = scope (limit): the retention window. Target backlog is capped to it.
	size := ie.limit
	if size < 1 {
		size = 1
	}
	ie.ring = make([]InspectEventEntry, size)

	eopts := gen.EventOptions{Notify: true}
	evname := gen.Atom(fmt.Sprintf("%s_%s", inspectEventStream, a.Hash))
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

func (ie *eventStream) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case flushEvent:
		if m.id != ie.loopID || ie.generating == false {
			break
		}
		info, err := ie.Node().EventInfo(ie.target)
		if err != nil {
			ie.sendClosed(err)
			return gen.TerminateReasonNormal
		}
		prevWatching, prevReason := ie.watching, ie.watchReason
		ie.evaluate(info)
		ie.flush()
		if ie.watching != prevWatching || ie.watchReason != prevReason {
			ie.publishStatus()
		}
		ie.SendAfter(ie.PID(), flushEvent{id: ie.loopID}, inspectEventPeriod)

	case requestInspect:
		if info, err := ie.Node().EventInfo(ie.target); err == nil {
			ie.evaluate(info)
		}
		ie.SendResponse(m.pid, m.ref, ResponseInspectEventStream{
			Event:       gen.Event{Name: ie.event, Node: ie.Node().Name()},
			Target:      ie.target,
			Buffer:      ie.ringSnapshot(),
			Watching:    ie.watching,
			WatchReason: ie.watchReason,
		})

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

func (ie *eventStream) HandleEvent(message gen.MessageEvent) error {
	if message.Event != ie.target {
		return nil
	}
	if e, ok := ie.toEntry(message); ok {
		ie.capture(e)
	}
	return nil
}

func (ie *eventStream) capture(e InspectEventEntry) {
	ie.ring[ie.pos] = e
	ie.pos++
	if ie.pos >= len(ie.ring) {
		ie.pos = 0
		ie.full = true
	}
	ie.received++
}

func (ie *eventStream) ringSnapshot() []InspectEventEntry {
	if ie.full == false && ie.pos == 0 {
		return nil
	}
	if ie.full {
		out := make([]InspectEventEntry, 0, len(ie.ring))
		out = append(out, ie.ring[ie.pos:]...)
		out = append(out, ie.ring[:ie.pos]...)
		return out
	}
	out := make([]InspectEventEntry, ie.pos)
	copy(out, ie.ring[:ie.pos])
	return out
}

// flush sends the entries captured since the last flush as one batch, capping to the ring and counting the rest as suppressed.
func (ie *eventStream) flush() {
	delta := ie.received - ie.flushed
	if delta <= 0 {
		return
	}
	ie.flushed = ie.received

	snap := ie.ringSnapshot()
	deliver := int64(len(snap))
	if delta < deliver {
		deliver = delta
	}
	if delta > deliver {
		ie.suppressed += delta - deliver
	}
	entries := snap[int64(len(snap))-deliver:]

	ie.SendEvent(ie.event, ie.token, MessageInspectEvent{
		Node:        ie.Node().Name(),
		Info:        gen.EventInfo{Event: ie.target},
		Entries:     entries,
		Suppressed:  ie.suppressed,
		Watching:    ie.watching,
		WatchReason: ie.watchReason,
	})
}

// evaluate gates monitoring so the inspector never starts or holds a notify-gated producer.
func (ie *eventStream) evaluate(info gen.EventInfo) {
	others := info.Subscribers
	if ie.watching {
		others--
	}

	// passive: a rising publish count at subs==0 means the producer ignores Notify
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
			return
		}
		ie.watching = true
		ie.passiveSeen = false
		ie.watchReason = ie.reasonFor(info, others)
		for _, em := range buf {
			if e, ok := ie.toEntry(em); ok {
				ie.capture(e)
			}
		}
		ie.flushed = ie.received // backlog goes out via requestInspect, not the flush delta
		return
	}

	if want == false && ie.watching {
		ie.DemonitorEvent(ie.target) // let the producer get its Stop
		ie.watching = false
		ie.passiveSeen = false
		ie.watchReason = "idle_gated"
		return
	}

	if ie.watching {
		ie.watchReason = ie.reasonFor(info, others)
	} else {
		ie.watchReason = "idle_gated"
	}
}

// reasonFor keeps "forced" last so it shows only when force is the sole reason.
func (ie *eventStream) reasonFor(info gen.EventInfo, others int64) string {
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

func (ie *eventStream) toEntry(em gen.MessageEvent) (InspectEventEntry, bool) {
	typ := fmt.Sprintf("%T", em.Message)
	readable := fmt.Sprintf("%+v", em.Message)
	verbose := fmt.Sprintf("%#v", em.Message)

	if ie.typePattern != "" {
		if strings.Contains(strings.ToLower(typ), ie.typePattern) == false {
			return InspectEventEntry{}, false
		}
	}
	if ie.messagePattern != "" {
		contains := strings.Contains(strings.ToLower(readable), ie.messagePattern) ||
			strings.Contains(strings.ToLower(verbose), ie.messagePattern)
		if ie.messageExclude == contains {
			return InspectEventEntry{}, false
		}
	}

	entry := InspectEventEntry{Timestamp: em.Timestamp, Type: typ, Message: readable}
	if ie.verbose && verbose != readable {
		entry.Verbose = verbose
	}
	return entry, true
}

func (ie *eventStream) publishStatus() {
	ie.SendEvent(ie.event, ie.token, MessageInspectEvent{
		Node:        ie.Node().Name(),
		Info:        gen.EventInfo{Event: ie.target},
		Suppressed:  ie.suppressed,
		Watching:    ie.watching,
		WatchReason: ie.watchReason,
	})
}

func (ie *eventStream) sendClosed(reason error) {
	rs := ""
	if reason != nil {
		rs = reason.Error()
	}
	ie.SetSendPriority(gen.MessagePriorityMax)
	ie.SendEvent(ie.event, ie.token, MessageInspectEvent{
		Node:   ie.Node().Name(),
		Info:   gen.EventInfo{Event: ie.target},
		Closed: true,
		Reason: rs,
	})
}

func (ie *eventStream) Terminate(reason error) {
	ie.Log().Debug("event stream inspector terminated: %s", reason)
}
