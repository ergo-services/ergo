package inspect

import (
	"fmt"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

func factory_event() gen.ProcessBehavior {
	return &event{}
}

type eventArgs struct {
	Name gen.Atom
	Hash string
}

// event polls EventInfo(target) once per second and publishes stats. It does
// not monitor the target, so it never starts or holds the producer.
type event struct {
	act.Actor
	token  gen.Ref
	event  gen.Atom  // own published event
	target gen.Event // observed event

	generating bool
	loopID     uint64
}

func (ie *event) Init(args ...any) error {
	a := args[0].(eventArgs)

	ie.target = gen.Event{Name: a.Name, Node: ie.Node().Name()}
	ie.Log().SetLogger("default")
	ie.SetProcessKind(gen.ProcessKindMonitor)
	ie.SetCompression(true)

	eopts := gen.EventOptions{Notify: true, Buffer: 1}
	evname := gen.Atom(fmt.Sprintf("%s_%s", inspectEvent, a.Hash))
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
		info, err := ie.Node().EventInfo(ie.target)
		if err != nil {
			ie.sendClosed(err)
			return gen.TerminateReasonNormal
		}
		if err := ie.SendEvent(ie.event, ie.token, MessageInspectEvent{Node: ie.Node().Name(), Info: info}); err != nil {
			return gen.TerminateReasonNormal
		}
		ie.SendAfter(ie.PID(), flushEvent{id: ie.loopID}, inspectEventPeriod)

	case requestInspect:
		response := ResponseInspectEvent{
			Event: gen.Event{Name: ie.event, Node: ie.Node().Name()},
		}
		if info, err := ie.Node().EventInfo(ie.target); err == nil {
			response.Info = info
		}
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

	case gen.MessageEventStop:
		ie.generating = false
		ie.SendAfter(ie.PID(), shutdown{}, inspectEventIdlePeriod)

	default:
		ie.Log().Error("unknown message (ignored) %#v", message)
	}

	return nil
}

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
	ie.Log().Debug("event info inspector terminated: %s", reason)
}
