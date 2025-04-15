package inspect

import (
	"fmt"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

func factory_meta() gen.ProcessBehavior {
	return &meta{}
}

type meta struct {
	act.Actor
	token gen.Ref

	event      gen.Atom
	generating bool
	meta       gen.Alias
}

func (im *meta) Init(args ...any) error {
	im.meta = args[0].(gen.Alias)
	im.Log().SetLogger("default")
	im.Log().Debug("meta process inspector started. pid %s", im.meta)
	// RegisterEvent is not allowed here
	im.Send(im.PID(), register{})
	return nil
}

func (im *meta) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case generate:
		if im.generating == false {
			im.Log().Debug("generating canceled")
			break // cancelled
		}
		im.Log().Debug("generating event")
		ev := MessageInspectMeta{
			Node:       im.Node().Name(),
			Terminated: true,
		}

		info, err := im.MetaInfo(im.meta)
		switch err {
		case nil:
			break
		case gen.ErrNodeTerminated:
			return err
		case gen.ErrProcessUnknown, gen.ErrMetaUnknown:
			if err := im.SendEvent(im.event, im.token, ev); err != nil {
				im.Log().Error("unable to send event %q: %s", im.event, err)
			}
			return gen.TerminateReasonNormal
		default:
			im.Log().Error("unable to inspect meta process %s: %s", im.meta, err)
			// will try next time
			im.SendAfter(im.PID(), generate{}, inspectMetaPeriod)
			return nil

		}

		ev.Terminated = false
		ev.Info = info

		if err := im.SendEvent(im.event, im.token, ev); err != nil {
			im.Log().Error("unable to send event %q: %s", im.event, err)
			return gen.TerminateReasonNormal
		}

		im.SendAfter(im.PID(), generate{}, inspectMetaPeriod)

	case requestInspect:
		response := ResponseInspectMeta{
			Event: gen.Event{
				Name: im.event,
				Node: im.Node().Name(),
			},
		}
		im.SendResponse(m.pid, m.ref, response)
		im.Log().Debug("sent response for the inspect meta request to: %s", m.pid)

	case register:
		eopts := gen.EventOptions{
			Notify: true,
			Buffer: 1, // keep the last event
		}
		evname := gen.Atom(fmt.Sprintf("%s_%s", inspectMeta, im.meta))
		token, err := im.RegisterEvent(evname, eopts)
		if err != nil {
			im.Log().Error("unable to register meta process event: %s", err)
			return err
		}
		im.Log().Info("registered event %s", evname)
		im.event = evname

		im.token = token
		im.SendAfter(im.PID(), shutdown{}, inspectMetaIdlePeriod)

	case shutdown:
		if im.generating {
			im.Log().Debug("ignore shutdown. generating is active")
			break // ignore.
		}
		return gen.TerminateReasonNormal

	case gen.MessageEventStart: // got first subscriber
		im.Log().Debug("got first subscriber. start generating events...")
		im.Send(im.PID(), generate{})
		im.generating = true

	case gen.MessageEventStop: // no subscribers
		im.Log().Debug("no subscribers. stop generating")
		if im.generating {
			im.generating = false
			im.SendAfter(im.PID(), shutdown{}, inspectMetaIdlePeriod)
		}

	default:
		im.Log().Error("unknown message (ignored) %#v", message)
	}

	return nil
}

func (im *meta) Terminate(reason error) {
	im.Log().Debug("meta %s inspector terminated: %s", im.meta, reason)
}
