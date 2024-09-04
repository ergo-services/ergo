package inspect

import (
	"fmt"
	"slices"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

func factory_process_list() gen.ProcessBehavior {
	return &process_list{}
}

type process_list struct {
	act.Actor
	token gen.Ref

	start      int
	limit      int
	generating bool
	event      gen.Atom
}

func (ipl *process_list) Init(args ...any) error {
	ipl.start = args[0].(int)
	ipl.limit = args[1].(int)
	ipl.Log().SetLogger("default")
	ipl.Log().Debug("process list inspector started. %d...%d", ipl.start, ipl.start+ipl.limit-1)
	// RegisterEvent is not allowed here
	ipl.Send(ipl.PID(), register{})
	return nil
}
func (ipl *process_list) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case generate:
		if ipl.generating == false {
			ipl.Log().Debug("generating canceled")
			break // cancelled
		}
		ipl.Log().Debug("generating event")

		list, err := ipl.Node().ProcessListShortInfo(ipl.start, ipl.limit)
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

		ipl.SendAfter(ipl.PID(), generate{}, inspectProcessListPeriod)

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
			Buffer: 1, // keep the last event
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
			break // ignore.
		}
		return gen.TerminateReasonNormal

	case gen.MessageEventStart: // got first subscriber
		ipl.Log().Debug("got first subscriber. start generating events...")
		ipl.Send(ipl.PID(), generate{})
		ipl.generating = true

	case gen.MessageEventStop: // no subscribers
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
