package inspect

import (
	"fmt"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

func factory_process() gen.ProcessBehavior {
	return &process{}
}

type process struct {
	act.Actor
	token gen.Ref

	event      gen.Atom
	pid        gen.PID
	generating bool
	loopID     uint64
}

func (ip *process) Init(args ...any) error {
	ip.pid = args[0].(gen.PID)
	ip.Log().SetLogger("default")
	ip.Log().Debug("process inspector started. pid %s", ip.pid)
	// RegisterEvent is not allowed here
	ip.Send(ip.PID(), register{})
	return nil
}

func (ip *process) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case generate:
		if m.id != ip.loopID || ip.generating == false {
			ip.Log().Debug("generating canceled")
			break // cancelled
		}
		ip.Log().Debug("generating event")

		ev := MessageInspectProcess{
			Node: ip.Node().Name(),
			Info: gen.ProcessInfo{
				PID:   ip.pid,
				State: gen.ProcessStateTerminated,
			},
		}

		info, err := ip.Node().ProcessInfo(ip.pid)
		switch err {
		case nil:
			break
		case gen.ErrNodeTerminated:
			return err

		case gen.ErrProcessUnknown:
			if err := ip.SendEvent(ip.event, ip.token, ev); err != nil {
				ip.Log().Error("unable to send event %q: %s", ip.event, err)
			}
			return gen.TerminateReasonNormal

		default:
			ip.Log().Error("unable to inspect process %s: %s", ip.pid, err)
			// will try next time (seems to be busy)
			ip.SendAfter(ip.PID(), generate{id: ip.loopID}, inspectProcessPeriod)
			return nil
		}

		for k, v := range info.Env {
			info.Env[k] = fmt.Sprintf("%#v", v)
		}

		ev.Info = info

		if err := ip.SendEvent(ip.event, ip.token, ev); err != nil {
			ip.Log().Error("unable to send event %q: %s", ip.event, err)
			return gen.TerminateReasonNormal
		}

		ip.SendAfter(ip.PID(), generate{id: ip.loopID}, inspectProcessPeriod)

	case requestInspect:
		response := ResponseInspectProcess{
			Event: gen.Event{
				Name: ip.event,
				Node: ip.Node().Name(),
			},
		}
		ip.SendResponse(m.pid, m.ref, response)
		ip.Log().Debug("sent response for the inspect process request to: %s", m.pid)

	case register:
		eopts := gen.EventOptions{
			Notify: true,
			Buffer: 1, // keep the last event
		}
		evname := gen.Atom(fmt.Sprintf("%s_%s", inspectProcess, ip.pid))
		token, err := ip.RegisterEvent(evname, eopts)
		if err != nil {
			ip.Log().Error("unable to register event: %s", err)
			return err
		}
		ip.Log().Info("registered event %s", evname)
		ip.event = evname

		ip.token = token
		ip.SendAfter(ip.PID(), shutdown{}, inspectProcessIdlePeriod)

	case shutdown:
		if ip.generating {
			ip.Log().Debug("ignore shutdown. generating is active")
			break // ignore.
		}
		return gen.TerminateReasonNormal

	case gen.MessageEventStart: // got first subscriber
		ip.Log().Debug("got first subscriber. start generating events...")
		ip.loopID++
		ip.Send(ip.PID(), generate{id: ip.loopID})
		ip.generating = true

	case gen.MessageEventStop: // no subscribers
		ip.Log().Debug("no subscribers. stop generating")
		if ip.generating {
			ip.generating = false
			ip.SendAfter(ip.PID(), shutdown{}, inspectProcessIdlePeriod)
		}

	default:
		ip.Log().Error("unknown message (ignored) %#v", message)
	}

	return nil
}

func (ip *process) Terminate(reason error) {
	ip.Log().Debug("process inspector terminated: %s", reason)
}
