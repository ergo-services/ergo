package inspect

import (
	"fmt"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

func factory_process_state() gen.ProcessBehavior {
	return &process_state{}
}

type process_state struct {
	act.Actor
	token gen.Ref

	event      gen.Atom
	generating bool
	pid        gen.PID
}

func (ips *process_state) Init(args ...any) error {
	ips.pid = args[0].(gen.PID)
	ips.Log().SetLogger("default")
	ips.Log().Debug("process state inspector started. pid %s", ips.pid)
	// RegisterEvent is not allowed here
	ips.Send(ips.PID(), register{})
	return nil
}

func (ips *process_state) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case generate:
		if ips.generating == false {
			ips.Log().Debug("generating canceled")
			break // cancelled
		}
		ips.Log().Debug("generating event")
		state, err := ips.Inspect(ips.pid)
		if err != nil {
			if err == gen.ErrProcessUnknown {
				return gen.TerminateReasonNormal
			}
			ips.Log().Error("unable to inspect process state %s: %s", ips.pid, err)
			// will try next time
			ips.SendAfter(ips.PID(), generate{}, inspectProcessStatePeriod)
			return nil
		}

		ev := MessageInspectProcessState{
			Node:  ips.Node().Name(),
			PID:   ips.pid,
			State: state,
		}

		if err := ips.SendEvent(ips.event, ips.token, ev); err != nil {
			ips.Log().Error("unable to send event %q: %s", ips.event, err)
			return gen.TerminateReasonNormal
		}

		ips.SendAfter(ips.PID(), generate{}, inspectProcessStatePeriod)

	case requestInspect:
		response := ResponseInspectProcessState{
			Event: gen.Event{
				Name: ips.event,
				Node: ips.Node().Name(),
			},
		}
		ips.SendResponse(m.pid, m.ref, response)
		ips.Log().Debug("sent response for the inspect process state %s request to: %s", ips.pid, m.pid)

	case register:
		eopts := gen.EventOptions{
			Notify: true,
			Buffer: 1, // keep the last event
		}
		evname := gen.Atom(fmt.Sprintf("%s_%s", inspectProcessState, ips.pid))
		token, err := ips.RegisterEvent(evname, eopts)
		if err != nil {
			ips.Log().Error("unable to register process state event: %s", err)
			return err
		}
		ips.Log().Info("registered event %s", evname)
		ips.event = evname

		ips.token = token
		ips.SendAfter(ips.PID(), shutdown{}, inspectProcessStateIdlePeriod)

	case shutdown:
		if ips.generating {
			ips.Log().Debug("ignore shutdown. generating is active")
			break // ignore.
		}
		return gen.TerminateReasonNormal

	case gen.MessageEventStart: // got first subscriber
		ips.Log().Debug("got first subscriber. start generating events...")
		ips.Send(ips.PID(), generate{})
		ips.generating = true

	case gen.MessageEventStop: // no subscribers
		ips.Log().Debug("no subscribers. stop generating")
		if ips.generating {
			ips.generating = false
			ips.SendAfter(ips.PID(), shutdown{}, inspectProcessStateIdlePeriod)
		}

	default:
		ips.Log().Error("unknown message (ignored) %#v", message)
	}

	return nil
}

func (ips *process_state) Terminate(reason error) {
	ips.Log().Debug("process state %s inspector terminated: %s", ips.pid, reason)
}
