package inspect

import (
	"fmt"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

func factory_event_list() gen.ProcessBehavior {
	return &event_list{}
}

type event_list struct {
	act.Actor
	token gen.Ref

	timestamp      int64
	name           string
	notify         int
	buffered       int
	open           int
	minSubscribers int64
	limit          int
	hash           string

	generating bool
	loopID     uint64
	event      gen.Atom
}

func (iel *event_list) Init(args ...any) error {
	iel.timestamp = args[0].(int64)
	iel.name = args[1].(string)
	iel.notify = args[2].(int)
	iel.buffered = args[3].(int)
	iel.open = args[4].(int)
	iel.minSubscribers = args[5].(int64)
	iel.limit = args[6].(int)
	iel.hash = args[7].(string)

	iel.Log().SetLogger("default")
	iel.SetProcessKind(gen.ProcessKindMonitor)
	iel.Log().Debug("event list inspector started. timestamp=%d name=%q notify=%d buffered=%d open=%d minSubs=%d limit=%d",
		iel.timestamp, iel.name, iel.notify, iel.buffered, iel.open, iel.minSubscribers, iel.limit)
	iel.SetCompression(true)

	eopts := gen.EventOptions{
		Notify: true,
		Buffer: 1,
	}
	iel.event = gen.Atom(fmt.Sprintf("%s_%s", inspectEventList, iel.hash))
	token, err := iel.RegisterEvent(iel.event, eopts)
	if err != nil {
		iel.Log().Error("unable to register event: %s", err)
		return err
	}
	iel.Log().Info("registered event %s", iel.event)
	iel.token = token
	iel.SendAfter(iel.PID(), shutdown{}, inspectEventListIdlePeriod)

	return nil
}

func (iel *event_list) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case generate:
		if m.id != iel.loopID || iel.generating == false {
			iel.Log().Debug("generating canceled")
			break
		}
		iel.Log().Debug("generating event")

		events, _ := iel.Node().EventListInfo(iel.timestamp, iel.limit, iel.filterEvent)

		ev := MessageInspectEventList{
			Node:   iel.Node().Name(),
			Events: events,
		}

		if err := iel.SendEvent(iel.event, iel.token, ev); err != nil {
			iel.Log().Error("unable to send event %q: %s", iel.event, err)
			return gen.TerminateReasonNormal
		}

		iel.SendAfter(iel.PID(), generate{id: iel.loopID}, inspectEventListPeriod)

	case requestInspect:
		response := ResponseInspectEventList{
			Event: gen.Event{
				Name: iel.event,
				Node: iel.Node().Name(),
			},
		}
		iel.SendResponse(m.pid, m.ref, response)
		iel.Log().Debug("sent response for the inspect event list request to: %s", m.pid)

	case shutdown:
		if iel.generating {
			iel.Log().Debug("ignore shutdown. generating is active")
			break
		}
		return gen.TerminateReasonNormal

	case gen.MessageEventStart:
		iel.Log().Debug("got first subscriber. start generating events...")
		iel.loopID++
		iel.Send(iel.PID(), generate{id: iel.loopID})
		iel.generating = true

	case gen.MessageEventStop:
		iel.Log().Debug("no subscribers. stop generating")
		if iel.generating {
			iel.generating = false
			iel.SendAfter(iel.PID(), shutdown{}, inspectEventListIdlePeriod)
		}

	default:
		iel.Log().Error("unknown message (ignored) %#v", message)
	}

	return nil
}

func (iel *event_list) filterEvent(info gen.EventInfo) bool {
	return eventFilter{
		Name: iel.name, Notify: iel.notify, Buffered: iel.buffered,
		Open: iel.open, MinSubscribers: iel.minSubscribers,
	}.match(info)
}

func (iel *event_list) Terminate(reason error) {
	iel.Log().Debug("event list inspector terminated: %s", reason)
}
