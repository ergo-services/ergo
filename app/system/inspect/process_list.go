package inspect

import (
	"fmt"
	"slices"
	"strings"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

func factory_process_list() gen.ProcessBehavior {
	return &process_list{}
}

type process_list struct {
	act.Actor
	token gen.Ref

	start       int
	limit       int
	name        string
	behavior    string
	application string
	state       string
	minMailbox  uint64

	generating bool
	loopID     uint64
	event      gen.Atom
}

func (ipl *process_list) Init(args ...any) error {
	ipl.start = args[0].(int)
	ipl.limit = args[1].(int)
	ipl.name = args[2].(string)
	ipl.behavior = args[3].(string)
	ipl.application = args[4].(string)
	ipl.state = args[5].(string)
	ipl.minMailbox = args[6].(uint64)

	ipl.Log().SetLogger("default")
	ipl.Log().Debug("process list inspector started. %d...%d", ipl.start, ipl.start+ipl.limit-1)
	ipl.Send(ipl.PID(), register{})
	ipl.SetCompression(true)
	return nil
}

func (ipl *process_list) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case generate:
		if m.id != ipl.loopID || ipl.generating == false {
			ipl.Log().Debug("generating canceled")
			break
		}
		ipl.Log().Debug("generating event")

		var filter []func(gen.ProcessShortInfo) bool
		if ipl.hasFilters() {
			filter = append(filter, ipl.matchFilter)
		}

		list, err := ipl.Node().ProcessListShortInfo(ipl.start, ipl.limit, filter...)
		if err != nil {
			return err
		}

		slices.SortStableFunc(list, func(a, b gen.ProcessShortInfo) int {
			return int(a.PID.ID - b.PID.ID)
		})

		ev := MessageInspectProcessList{
			Node:      ipl.Node().Name(),
			Processes: list,
		}

		if err := ipl.SendEvent(ipl.event, ipl.token, ev); err != nil {
			ipl.Log().Error("unable to send event %q: %s", ipl.event, err)
			return gen.TerminateReasonNormal
		}

		ipl.SendAfter(ipl.PID(), generate{id: ipl.loopID}, inspectProcessListPeriod)

	case requestInspect:
		response := ResponseInspectProcessList{
			Event: gen.Event{
				Name: ipl.event,
				Node: ipl.Node().Name(),
			},
		}
		ipl.SendResponse(m.pid, m.ref, response)
		ipl.Log().Debug("sent response for the inspect process list request to: %s", m.pid)

	case register:
		eopts := gen.EventOptions{
			Notify: true,
			Buffer: 1,
		}
		evname := gen.Atom(fmt.Sprintf("%s_%d_%d", inspectProcessList, ipl.start, ipl.start+ipl.limit-1))
		token, err := ipl.RegisterEvent(evname, eopts)
		if err != nil {
			ipl.Log().Error("unable to register event: %s", err)
			return err
		}
		ipl.Log().Info("registered event %s", evname)
		ipl.event = evname
		ipl.token = token
		ipl.SendAfter(ipl.PID(), shutdown{}, inspectProcessListIdlePeriod)

	case shutdown:
		if ipl.generating {
			ipl.Log().Debug("ignore shutdown. generating is active")
			break
		}
		return gen.TerminateReasonNormal

	case gen.MessageEventStart:
		ipl.Log().Debug("got first subscriber. start generating events...")
		ipl.loopID++
		ipl.Send(ipl.PID(), generate{id: ipl.loopID})
		ipl.generating = true

	case gen.MessageEventStop:
		ipl.Log().Debug("no subscribers. stop generating")
		if ipl.generating {
			ipl.generating = false
			ipl.SendAfter(ipl.PID(), shutdown{}, inspectProcessListIdlePeriod)
		}

	default:
		ipl.Log().Error("unknown message (ignored) %#v", message)
	}

	return nil
}

func (ipl *process_list) Terminate(reason error) {
	ipl.Log().Debug("process list inspector terminated: %s", reason)
}

func (ipl *process_list) hasFilters() bool {
	return ipl.name != "" || ipl.behavior != "" || ipl.application != "" || ipl.state != "" || ipl.minMailbox > 0
}

func (ipl *process_list) matchFilter(info gen.ProcessShortInfo) bool {
	if ipl.name != "" && strings.Contains(strings.ToLower(string(info.Name)), strings.ToLower(ipl.name)) == false {
		return false
	}
	if ipl.behavior != "" && strings.Contains(strings.ToLower(info.Behavior), strings.ToLower(ipl.behavior)) == false {
		return false
	}
	if ipl.application != "" && strings.Contains(strings.ToLower(string(info.Application)), strings.ToLower(ipl.application)) == false {
		return false
	}
	if ipl.state != "" && strings.EqualFold(info.State.String(), ipl.state) == false {
		return false
	}
	if ipl.minMailbox > 0 && info.MessagesMailbox < ipl.minMailbox {
		return false
	}
	return true
}
