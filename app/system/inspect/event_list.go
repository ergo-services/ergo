package inspect

import (
	"fmt"
	"strings"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

func factory_event_list() gen.ProcessBehavior {
	return &event_list{}
}

type event_list struct {
	act.Actor
	token gen.Ref

	name           string
	notify         int
	buffered       int
	minSubscribers int64
	limit          int
	hash           string

	generating bool
	loopID     uint64
	event      gen.Atom
}

func (iel *event_list) Init(args ...any) error {
	iel.name = args[0].(string)
	iel.notify = args[1].(int)
	iel.buffered = args[2].(int)
	iel.minSubscribers = args[3].(int64)
	iel.limit = args[4].(int)
	iel.hash = args[5].(string)

	iel.Log().SetLogger("default")
	iel.Log().Debug("event list inspector started. name=%q notify=%d buffered=%d minSubs=%d limit=%d",
		iel.name, iel.notify, iel.buffered, iel.minSubscribers, iel.limit)
	iel.Send(iel.PID(), register{})
	iel.SetCompression(true)
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

		var events []gen.EventInfo
		nameLower := strings.ToLower(iel.name)

		iel.Node().EventRangeInfo(func(info gen.EventInfo) bool {
			if nameLower != "" {
				if strings.Contains(strings.ToLower(string(info.Event.Name)), nameLower) == false {
					return true
				}
			}
			if iel.notify == 1 {
				if info.Notify == false {
					return true
				}
			}
			if iel.notify == -1 {
				if info.Notify == true {
					return true
				}
			}
			if iel.buffered == 1 {
				if info.BufferSize == 0 {
					return true
				}
			}
			if iel.buffered == -1 {
				if info.BufferSize > 0 {
					return true
				}
			}
			if iel.minSubscribers > 0 {
				if info.Subscribers < iel.minSubscribers {
					return true
				}
			}

			events = append(events, info)

			if iel.limit > 0 && len(events) >= iel.limit {
				return false
			}
			return true
		})

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

	case register:
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

func (iel *event_list) Terminate(reason error) {
	iel.Log().Debug("event list inspector terminated: %s", reason)
}
