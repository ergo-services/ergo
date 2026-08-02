package inspect

import (
	"fmt"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

func factory_meta_state() gen.ProcessBehavior {
	return &meta_state{}
}

type meta_state struct {
	act.Actor
	token gen.Ref

	event      gen.Atom
	generating bool
	loopID     uint64
	meta       gen.Alias
	items      []string
}

func (ims *meta_state) Init(args ...any) error {
	ims.meta = args[0].(gen.Alias)
	ims.items = args[1].([]string)
	ims.Log().SetLogger("default")
	ims.SetProcessKind(gen.ProcessKindMonitor)
	ims.Log().Debug("meta state inspector started. id %s", ims.meta)

	eopts := gen.EventOptions{
		Notify: true,
		Buffer: 1, // keep the last event
	}
	evname := gen.Atom(fmt.Sprintf("%s_%s_%s", inspectMetaState, ims.meta, itemsHash(ims.items)))
	token, err := ims.RegisterEvent(evname, eopts)
	if err != nil {
		ims.Log().Error("unable to register meta state event: %s", err)
		return err
	}
	ims.Log().Info("registered event %s", evname)
	ims.event = evname
	ims.token = token
	ims.SendAfter(ims.PID(), shutdown{}, inspectMetaStateIdlePeriod)

	return nil
}

func (ims *meta_state) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case generate:
		if m.id != ims.loopID || ims.generating == false {
			ims.Log().Debug("generating canceled")
			break // cancelled
		}
		ims.Log().Debug("generating event")
		state, err := ims.InspectMeta(ims.meta, ims.items...)
		if err != nil {
			if err == gen.ErrMetaUnknown {
				return gen.TerminateReasonNormal
			}
			ims.Log().Error("unable to inspect meta state %s: %s", ims.meta, err)
			// will try next time
			ims.SendAfter(ims.PID(), generate{id: ims.loopID}, inspectMetaStatePeriod)
			return nil
		}
		if state == nil {
			state = map[string]string{}
		}

		ev := MessageInspectMetaState{
			Node:  ims.Node().Name(),
			Meta:  ims.meta,
			State: state,
		}

		if err := ims.SendEvent(ims.event, ims.token, ev); err != nil {
			ims.Log().Error("unable to send event %q: %s", ims.event, err)
			return gen.TerminateReasonNormal
		}

		ims.SendAfter(ims.PID(), generate{id: ims.loopID}, inspectMetaStatePeriod)

	case requestInspect:
		response := ResponseInspectMetaState{
			Event: gen.Event{
				Name: ims.event,
				Node: ims.Node().Name(),
			},
		}
		ims.SendResponse(m.pid, m.ref, response)
		ims.Log().Debug("sent response for the inspect meta state %s request to: %s", ims.meta, m.pid)

	case shutdown:
		if ims.generating {
			ims.Log().Debug("ignore shutdown. generating is active")
			break // ignore.
		}
		return gen.TerminateReasonNormal

	case gen.MessageEventStart: // got first subscriber
		ims.Log().Debug("got first subscriber. start generating events...")
		ims.loopID++
		ims.Send(ims.PID(), generate{id: ims.loopID})
		ims.generating = true

	case gen.MessageEventStop: // no subscribers
		ims.Log().Debug("no subscribers. stop generating")
		if ims.generating {
			ims.generating = false
			ims.SendAfter(ims.PID(), shutdown{}, inspectMetaStateIdlePeriod)
		}

	default:
		ims.Log().Error("unknown message (ignored) %#v", message)
	}

	return nil
}

func (ims *meta_state) Terminate(reason error) {
	ims.Log().Debug("meta state %s inspector terminated: %s", ims.meta, reason)
}
