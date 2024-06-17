package inspect

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"slices"
)

func factory_network() gen.ProcessBehavior {
	return &network{}
}

type network struct {
	act.Actor
	token gen.Ref

	generating bool
}

func (in *network) Init(args ...any) error {
	in.Log().SetLogger("default")
	in.Log().Debug("network inspector started")
	// RegisterEvent is not allowed here
	in.Send(in.PID(), register{})
	return nil
}

func (in *network) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case generate:
		if in.generating == false {
			in.Log().Debug("generating canceled")
			break // cancelled
		}
		in.Log().Debug("generating event")

		info, err := in.Node().Network().Info()
		slices.Sort(info.Nodes)
		ev := MessageInspectNetwork{
			Node:    in.Node().Name(),
			Stopped: err == gen.ErrNetworkStopped,
			Info:    info,
		}

		if err := in.SendEvent(inspectNetwork, in.token, ev); err != nil {
			in.Log().Error("unable to send event %q: %s", inspectNetwork, err)
			return gen.TerminateReasonNormal
		}

		in.SendAfter(in.PID(), generate{}, inspectNetworkPeriod)

	case requestInspect:
		info, err := in.Node().Network().Info()
		response := ResponseInspectNetwork{
			Event: gen.Event{
				Name: inspectNetwork,
				Node: in.Node().Name(),
			},
			Stopped: err == gen.ErrNetworkStopped,
			Info:    info,
		}
		in.SendResponse(m.pid, m.ref, response)
		in.Log().Debug("sent response for the inspect network request to: %s", m.pid)

	case register:
		eopts := gen.EventOptions{
			Notify: true,
			Buffer: 1, // keep the last event
		}
		token, err := in.RegisterEvent(inspectNetwork, eopts)
		if err != nil {
			in.Log().Error("unable to register network event: %s", err)
			return err
		}
		in.Log().Info("registered event %s", inspectNetwork)

		in.token = token
		in.SendAfter(in.PID(), shutdown{}, inspectNetworkIdlePeriod)

	case shutdown:
		if in.generating {
			in.Log().Debug("ignore shutdown. generating is active")
			break // ignore.
		}
		return gen.TerminateReasonNormal

	case gen.MessageEventStart: // got first subscriber
		in.Log().Debug("got first subscriber. start generating events...")
		in.Send(in.PID(), generate{})
		in.generating = true

	case gen.MessageEventStop: // no subscribers
		in.Log().Debug("no subscribers. stop generating")
		if in.generating {
			in.generating = false
			in.SendAfter(in.PID(), shutdown{}, inspectNetworkIdlePeriod)
		}

	default:
		in.Log().Error("unknown message (ignored) %#v", message)
	}

	return nil
}

func (in *network) Terminate(reason error) {
	in.Log().Debug("network inspector terminated: %s", reason)
}
