package inspect

import (
	"cmp"
	"fmt"
	"runtime"
	"slices"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

func factory_node() gen.ProcessBehavior {
	return &node{}
}

type node struct {
	act.Actor
	token gen.Ref

	generating bool
	loopID     uint64
}

func (in *node) Init(args ...any) error {
	in.Log().SetLogger("default")
	in.Log().Debug("node inspector started")

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

	return nil
}

func (in *node) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case generate:
		if m.id != in.loopID || in.generating == false {
			in.Log().Debug("generating canceled")
			break // cancelled
		}
		in.Log().Debug("generating event")

		info, err := in.Node().Info()
		if err != nil {
			return err
		}

		for k, v := range info.Env {
			info.Env[k] = fmt.Sprintf("%#v", v)
		}
		slices.SortStableFunc(info.Loggers, func(a, b gen.LoggerInfo) int {
			return cmp.Compare(a.Name, b.Name)
		})

		ev := MessageInspectNode{
			Node: in.Node().Name(),
			Info: info,
		}

		if err := in.SendEvent(inspectNode, in.token, ev); err != nil {
			in.Log().Error("unable to send event %q: %s", inspectNode, err)
			return gen.TerminateReasonNormal
		}

		in.SendAfter(in.PID(), generate{id: in.loopID}, inspectNodePeriod)

	case requestInspect:
		response := ResponseInspectNode{
			Event: gen.Event{
				Name: inspectNode,
				Node: in.Node().Name(),
			},

			Arch:      runtime.GOARCH,
			OS:        runtime.GOOS,
			Cores:     runtime.NumCPU(),
			GoVersion: runtime.Version(),
			Timezone: func() string {
				now := time.Now()
				name, _ := now.Zone()
				loc := now.Location().String()
				if loc == "Local" {
					return name // e.g. "MSK", "CET"
				}
				return loc // e.g. "Europe/Moscow"
			}(),
			Version:  in.Node().Version(),
			Creation: in.Node().Creation(),
			CRC32:    in.Node().Name().CRC32(),
		}
		in.SendResponse(m.pid, m.ref, response)
		in.Log().Debug("sent response for the inspect node request to: %s", m.pid)

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
			in.SendAfter(in.PID(), shutdown{}, inspectNodeIdlePeriod)
		}

	default:
		in.Log().Error("unknown message (ignored) %#v", message)
	}

	return nil
}

func (in *node) Terminate(reason error) {
	in.Log().Debug("node inspector terminated: %s", reason)
}
