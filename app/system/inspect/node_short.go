package inspect

import (
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

func factory_node_short() gen.ProcessBehavior {
	return &nodeShort{}
}

type nodeShort struct {
	act.Actor
	token gen.Ref

	event  gen.Atom
	period time.Duration

	generating bool
	loopID     uint64
}

func (in *nodeShort) Init(args ...any) error {
	in.event = args[0].(gen.Atom)
	in.period = args[1].(time.Duration)

	in.Log().SetLogger("default")
	in.SetProcessKind(gen.ProcessKindMonitor)
	in.SetCompression(true)
	in.Log().Debug("node short inspector started")

	eopts := gen.EventOptions{
		Notify: true,
		Buffer: 1, // keep the last event
	}
	token, err := in.RegisterEvent(in.event, eopts)
	if err != nil {
		in.Log().Error("unable to register event: %s", err)
		return err
	}
	in.Log().Info("registered event %s", in.event)
	in.token = token
	in.SendAfter(in.PID(), shutdown{}, inspectNodeShortIdlePeriod)

	return nil
}

func (in *nodeShort) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case generate:
		if m.id != in.loopID || in.generating == false {
			in.Log().Debug("generating canceled")
			break // cancelled
		}
		in.Log().Debug("generating event")

		info, err := in.Node().ShortInfo()
		if err != nil {
			return err
		}

		ev := MessageInspectNodeShort{
			Node: in.Node().Name(),
			Info: info,
		}

		if err := in.SendEvent(in.event, in.token, ev); err != nil {
			in.Log().Error("unable to send event %q: %s", in.event, err)
			return gen.TerminateReasonNormal
		}

		in.SendAfter(in.PID(), generate{id: in.loopID}, in.period)

	case requestInspect:
		response := ResponseInspectNodeShort{
			Event: gen.Event{
				Name: in.event,
				Node: in.Node().Name(),
			},
		}

		info, err := in.Node().ShortInfo()
		if err != nil {
			response.Error = err
		} else {
			response.Info = info
		}

		in.SendResponse(m.pid, m.ref, response)
		in.Log().Debug("sent response for the inspect node short request to: %s", m.pid)

	case shutdown:
		if in.generating {
			in.Log().Debug("ignore shutdown. generating is active")
			break // ignore.
		}
		return gen.TerminateReasonNormal

	case gen.MessageEventStart: // got first subscriber
		in.Log().Debug("got first subscriber. start generating events...")
		in.loopID++
		in.Send(in.PID(), generate{id: in.loopID})
		in.generating = true

	case gen.MessageEventStop: // no subscribers
		in.Log().Debug("no subscribers. stop generating")
		if in.generating {
			in.generating = false
			in.SendAfter(in.PID(), shutdown{}, inspectNodeShortIdlePeriod)
		}

	default:
		in.Log().Error("unknown message (ignored) %#v", message)
	}

	return nil
}

func (in *nodeShort) Terminate(reason error) {
	in.Log().Debug("node short inspector terminated: %s", reason)
}
