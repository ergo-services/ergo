package inspect

import (
	"runtime"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

func factory_inode() gen.ProcessBehavior {
	return &inode{}
}

type inode struct {
	act.Actor
	token gen.Ref

	generating bool
}

func (in *inode) Init(args ...any) error {
	in.Log().SetLogger("default")
	in.Log().Debug("node inspector started")
	// RegisterEvent is not allowed here
	in.Send(in.PID(), register{})
	return nil
}

func (in *inode) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case generate:
		if in.generating == false {
			in.Log().Debug("generating canceled")
			break // cancelled
		}
		in.Log().Debug("generating event")

		info, err := in.Node().Info()
		if err != nil {
			return err
		}

		ev := MessageInspectNode{
			Node: in.Node().Name(),
			Info: info,
		}

		if err := in.SendEvent(inspectNode, in.token, ev); err != nil {
			in.Log().Error("unable to send event %q: %s", inspectNode, err)
			return gen.TerminateReasonNormal
		}

		in.SendAfter(in.PID(), generate{}, inspectNodePeriod)

	case requestInspect:
		response := ResponseInspectNode{
			Event: gen.Event{
				Name: inspectNode,
				Node: in.Node().Name(),
			},

			Arch:     runtime.GOARCH,
			OS:       runtime.GOOS,
			Cores:    runtime.NumCPU(),
			Version:  in.Node().Version(),
			Creation: in.Node().Creation(),
			CRC32:    in.Node().Name().CRC32(),
		}
		in.SendResponse(m.pid, m.ref, response)
		in.Log().Debug("sent response for the inspect node request to: %s", m.pid)

	case register:
		eopts := gen.EventOptions{
			Notify: true,
			Buffer: 1, // keep the last event
		}
		token, err := in.RegisterEvent(inspectNode, eopts)
		if err != nil {
			in.Log().Error("unable to register event: %s", err)
			return err
		}
		in.Log().Info("registered event %s", inspectNode)

		in.token = token
		in.SendAfter(in.PID(), shutdown{}, inspectNodeIdlePeriod)

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
			// wait 10 seconds and terminate this process
			in.SendAfter(in.PID(), shutdown{}, inspectNodeIdlePeriod)
		}

	default:
		in.Log().Error("unknown message (ignored) %#v", message)
	}

	return nil
}

func (in *inode) Terminate(reason error) {
	in.Log().Debug("node inspector terminated: %s", reason)
}
