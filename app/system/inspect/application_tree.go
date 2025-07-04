package inspect

import (
	"fmt"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

func factory_application_tree() gen.ProcessBehavior {
	return &application_tree{}
}

type application_tree struct {
	act.Actor
	token gen.Ref

	application gen.Atom
	limit       int
	generating  bool
	event       gen.Atom
}

func (iat *application_tree) Init(args ...any) error {
	iat.application = args[0].(gen.Atom)
	iat.limit = args[1].(int)
	iat.Log().SetLogger("default")
	iat.Log().Debug("application tree inspector started for %s with limit %d", iat.application, iat.limit)
	// RegisterEvent is not allowed here
	iat.Send(iat.PID(), register{})
	return nil
}

func (iat *application_tree) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case generate:
		if iat.generating == false {
			iat.Log().Debug("generating canceled")
			break // cancelled
		}
		iat.Log().Debug("generating event for application %s", iat.application)

		list, err := iat.Node().ApplicationProcessListShortInfo(iat.application, iat.limit)
		if err != nil {
			return err
		}

		ev := MessageInspectApplicationTree{
			Node:        iat.Node().Name(),
			Application: iat.application,
			Processes:   list,
		}

		if err := iat.SendEvent(iat.event, iat.token, ev); err != nil {
			iat.Log().Error("unable to send event %q: %s", iat.event, err)
			return gen.TerminateReasonNormal
		}

		iat.SendAfter(iat.PID(), generate{}, inspectApplicationTreePeriod)

	case requestInspect:
		response := ResponseInspectApplicationTree{
			Event: gen.Event{
				Name: iat.event,
				Node: iat.Node().Name(),
			},
		}
		iat.SendResponse(m.pid, m.ref, response)
		iat.Log().Debug("sent response for the inspect application tree request to: %s", m.pid)

	case register:
		eopts := gen.EventOptions{
			Notify: true,
			Buffer: 1, // keep the last event
		}
		evname := gen.Atom(fmt.Sprintf("%s_%s_%d", inspectApplicationTree, iat.application, iat.limit))
		token, err := iat.RegisterEvent(evname, eopts)
		if err != nil {
			iat.Log().Error("unable to register event: %s", err)
			return err
		}
		iat.Log().Info("registered event %s", evname)
		iat.event = evname

		iat.token = token
		iat.SendAfter(iat.PID(), shutdown{}, inspectApplicationTreeIdlePeriod)

	case shutdown:
		if iat.generating {
			iat.Log().Debug("ignore shutdown. generating is active")
			break // ignore.
		}
		return gen.TerminateReasonNormal

	case gen.MessageEventStart: // got first subscriber
		iat.Log().Debug("got first subscriber. start generating events...")
		iat.Send(iat.PID(), generate{})
		iat.generating = true

	case gen.MessageEventStop: // no subscribers
		iat.Log().Debug("no subscribers. stop generating")
		if iat.generating {
			iat.generating = false
			iat.SendAfter(iat.PID(), shutdown{}, inspectApplicationTreeIdlePeriod)
		}

	default:
		iat.Log().Error("unknown message (ignored) %#v", message)
	}

	return nil
}

func (iat *application_tree) Terminate(reason error) {
	iat.Log().Debug("application tree inspector terminated: %s", reason)
}
