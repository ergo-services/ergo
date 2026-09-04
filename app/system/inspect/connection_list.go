package inspect

import (
	"fmt"
	"slices"
	"strings"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

func factory_connection_list() gen.ProcessBehavior {
	return &connection_list{}
}

type connection_list struct {
	act.Actor
	token gen.Ref

	name       string
	limit      int
	hash       string
	generating bool
	loopID     uint64
	event      gen.Atom
}

func (icl *connection_list) Init(args ...any) error {
	icl.name = args[0].(string)
	icl.limit = args[1].(int)
	icl.hash = args[2].(string)

	icl.Log().SetLogger("default")
	icl.SetProcessKind(gen.ProcessKindMonitor)
	icl.Log().Debug("connection list inspector started. name=%q limit=%d", icl.name, icl.limit)
	icl.SetCompression(true)

	eopts := gen.EventOptions{
		Notify: true,
		Buffer: 1,
	}
	icl.event = gen.Atom(fmt.Sprintf("%s_%s", inspectConnectionList, icl.hash))
	token, err := icl.RegisterEvent(icl.event, eopts)
	if err != nil {
		icl.Log().Error("unable to register event: %s", err)
		return err
	}
	icl.Log().Info("registered event %s", icl.event)
	icl.token = token
	icl.SendAfter(icl.PID(), shutdown{}, inspectConnectionListIdlePeriod)

	return nil
}

func (icl *connection_list) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case generate:
		if m.id != icl.loopID || icl.generating == false {
			break
		}

		networkInfo, err := icl.Node().Network().Info()
		if err != nil {
			return err
		}

		nameLower := strings.ToLower(icl.name)
		var connections []gen.RemoteNodeInfo

		// sort node names for stable output
		slices.Sort(networkInfo.Nodes)

		for _, n := range networkInfo.Nodes {
			if nameLower != "" {
				if strings.Contains(strings.ToLower(string(n)), nameLower) == false {
					continue
				}
			}

			remote, rerr := icl.Node().Network().Node(n)
			if rerr != nil {
				continue
			}

			connections = append(connections, remote.Info())

			if icl.limit > 0 && len(connections) >= icl.limit {
				break
			}
		}

		ev := MessageInspectConnectionList{
			Node:        icl.Node().Name(),
			Connections: connections,
		}

		if err := icl.SendEvent(icl.event, icl.token, ev); err != nil {
			icl.Log().Error("unable to send event %q: %s", icl.event, err)
			return gen.TerminateReasonNormal
		}

		icl.SendAfter(icl.PID(), generate{id: icl.loopID}, inspectConnectionListPeriod)

	case requestInspect:
		response := ResponseInspectConnectionList{
			Event: gen.Event{
				Name: icl.event,
				Node: icl.Node().Name(),
			},
		}
		icl.SendResponse(m.pid, m.ref, response)

	case shutdown:
		if icl.generating {
			break
		}
		return gen.TerminateReasonNormal

	case gen.MessageEventStart:
		icl.Log().Debug("got first subscriber. start generating events...")
		icl.loopID++
		icl.Send(icl.PID(), generate{id: icl.loopID})
		icl.generating = true

	case gen.MessageEventStop:
		icl.Log().Debug("no subscribers. stop generating")
		if icl.generating {
			icl.generating = false
			icl.SendAfter(icl.PID(), shutdown{}, inspectConnectionListIdlePeriod)
		}

	default:
		icl.Log().Error("unknown message (ignored) %#v", message)
	}

	return nil
}

func (icl *connection_list) Terminate(reason error) {
	icl.Log().Debug("connection list inspector terminated: %s", reason)
}
