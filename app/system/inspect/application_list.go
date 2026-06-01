package inspect

import (
	"fmt"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

func factory_application_list() gen.ProcessBehavior {
	return &application_list{}
}

type application_list struct {
	act.Actor
	token gen.Ref

	generating bool
	loopID     uint64
	event      gen.Atom
}

func (ial *application_list) Init(args ...any) error {
	ial.Log().SetLogger("default")
	ial.SetProcessKind(gen.ProcessKindMonitor)
	ial.Log().Debug("application list inspector started")

	eopts := gen.EventOptions{
		Notify: true,
		Buffer: 1, // keep the last event
	}
	evname := gen.Atom(fmt.Sprintf("%s", inspectApplicationList))
	token, err := ial.RegisterEvent(evname, eopts)
	if err != nil {
		ial.Log().Error("unable to register event: %s", err)
		return err
	}
	ial.Log().Info("registered event %s", evname)
	ial.event = evname
	ial.token = token
	ial.SendAfter(ial.PID(), shutdown{}, inspectApplicationListIdlePeriod)

	return nil
}

func (ial *application_list) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case generate:
		if m.id != ial.loopID || ial.generating == false {
			ial.Log().Debug("generating canceled")
			break // cancelled
		}
		ial.Log().Debug("generating event")

		// Get all applications using gen.Node.Applications() method
		applications := ial.Node().Applications()
		applicationMap := make(map[gen.Atom]gen.ApplicationInfo)

		// Get detailed info for each application using gen.Node.ApplicationInfo() method
		for _, appName := range applications {
			appInfo, err := ial.Node().ApplicationInfo(appName)
			if err != nil {
				ial.Log().Warning("unable to get info for application %s: %s", appName, err)
				continue // skip this application but continue with others
			}
			applicationMap[appName] = appInfo
		}

		ev := MessageInspectApplicationList{
			Node:         ial.Node().Name(),
			Applications: applicationMap,
		}

		if err := ial.SendEvent(ial.event, ial.token, ev); err != nil {
			ial.Log().Error("unable to send event %q: %s", ial.event, err)
			return gen.TerminateReasonNormal
		}

		ial.SendAfter(ial.PID(), generate{id: ial.loopID}, inspectApplicationListPeriod)

	case requestInspect:
		response := ResponseInspectApplicationList{
			Event: gen.Event{
				Name: ial.event,
				Node: ial.Node().Name(),
			},
		}
		ial.SendResponse(m.pid, m.ref, response)
		ial.Log().Debug("sent response for the inspect application list request to: %s", m.pid)

	case shutdown:
		if ial.generating {
			ial.Log().Debug("ignore shutdown. generating is active")
			break // ignore.
		}
		return gen.TerminateReasonNormal

	case gen.MessageEventStart: // got first subscriber
		ial.Log().Debug("got first subscriber. start generating events...")
		ial.loopID++
		ial.Send(ial.PID(), generate{id: ial.loopID})
		ial.generating = true

	case gen.MessageEventStop: // no subscribers
		ial.Log().Debug("no subscribers. stop generating")
		if ial.generating {
			ial.generating = false
			ial.SendAfter(ial.PID(), shutdown{}, inspectApplicationListIdlePeriod)
		}

	default:
		ial.Log().Error("unknown message (ignored) %#v", message)
	}

	return nil
}

func (ial *application_list) Terminate(reason error) {
	ial.Log().Debug("application list inspector terminated: %s", reason)
}
