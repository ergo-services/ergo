package inspect

import (
	"fmt"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

func factory_connection() gen.ProcessBehavior {
	return &connection{}
}

type connection struct {
	act.Actor
	token gen.Ref

	event      gen.Atom
	generating bool
	remote     gen.Atom
}

func (ic *connection) Init(args ...any) error {
	ic.remote = args[0].(gen.Atom)
	ic.Log().SetLogger("default")
	ic.Log().Debug("connection inspector started")
	// RegisterEvent is not allowed here
	ic.Send(ic.PID(), register{})
	return nil
}

func (ic *connection) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case generate:
		if ic.generating == false {
			ic.Log().Debug("generating canceled")
			break // cancelled
		}
		ic.Log().Debug("generating event")

		ev := MessageInspectConnection{
			Node:         ic.Node().Name(),
			Disconnected: true,
		}

		remote, err := ic.Node().Network().Node(ic.remote)
		if err != nil {
			ic.SendEvent(ic.event, ic.token, ev)
			return gen.TerminateReasonNormal
		}

		ev.Disconnected = false
		ev.Info = remote.Info()

		if err := ic.SendEvent(ic.event, ic.token, ev); err != nil {
			ic.Log().Error("unable to send event %q: %s", inspectNetwork, err)
			return gen.TerminateReasonNormal
		}

		if ev.Disconnected {
			return gen.TerminateReasonNormal
		}
		ic.SendAfter(ic.PID(), generate{}, inspectNetworkPeriod)

	case requestInspect:
		response := ResponseInspectConnection{
			Event: gen.Event{
				Name: ic.event,
				Node: ic.Node().Name(),
			},
			Disconnected: true,
		}
		if remote, err := ic.Node().Network().Node(ic.remote); err == nil {
			response.Disconnected = false
			response.Info = remote.Info()
		}
		ic.SendResponse(m.pid, m.ref, response)
		ic.Log().Debug("sent response for the inspect connection request to: %s", m.pid)
		if response.Disconnected {
			return gen.TerminateReasonNormal
		}

	case register:
		eopts := gen.EventOptions{
			Notify: true,
			Buffer: 1, // keep the last event
		}
		evname := gen.Atom(fmt.Sprintf("%s_%s", inspectConnection, ic.remote))
		token, err := ic.RegisterEvent(evname, eopts)
		if err != nil {
			ic.Log().Error("unable to register connection event: %s", err)
			return err
		}
		ic.Log().Info("registered event %s", inspectNetwork)
		ic.event = evname

		ic.token = token
		ic.SendAfter(ic.PID(), shutdown{}, inspectNetworkIdlePeriod)

	case shutdown:
		if ic.generating {
			ic.Log().Debug("ignore shutdown. generating is active")
			break // ignore.
		}
		return gen.TerminateReasonNormal

	case gen.MessageEventStart: // got first subscriber
		ic.Log().Debug("got first subscriber. start generating events...")
		ic.Send(ic.PID(), generate{})
		ic.generating = true

	case gen.MessageEventStop: // no subscribers
		ic.Log().Debug("no subscribers. stop generating")
		if ic.generating {
			ic.generating = false
			ic.SendAfter(ic.PID(), shutdown{}, inspectNetworkIdlePeriod)
		}

	default:
		ic.Log().Error("unknown message (ignored) %#v", message)
	}

	return nil
}

func (ic *connection) Terminate(reason error) {
	ic.Log().Debug("network inspector terminated: %s", reason)
}
