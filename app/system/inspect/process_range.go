package inspect

import (
	"fmt"
	"slices"
	"strings"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

func factory_process_range() gen.ProcessBehavior {
	return &process_range{}
}

type process_range struct {
	act.Actor
	token gen.Ref

	name        string
	behavior    string
	application string
	state       string
	minMailbox  uint64
	limit       int
	hash        string

	generating bool
	loopID     uint64
	event      gen.Atom
}

func (ipr *process_range) Init(args ...any) error {
	ipr.name = args[0].(string)
	ipr.behavior = args[1].(string)
	ipr.application = args[2].(string)
	ipr.state = args[3].(string)
	ipr.minMailbox = args[4].(uint64)
	ipr.limit = args[5].(int)
	ipr.hash = args[6].(string)

	ipr.Log().SetLogger("default")
	ipr.Log().Debug("process range inspector started. name=%q behavior=%q app=%q state=%q mailbox>=%d limit=%d",
		ipr.name, ipr.behavior, ipr.application, ipr.state, ipr.minMailbox, ipr.limit)
	ipr.Send(ipr.PID(), register{})
	ipr.SetCompression(true)
	return nil
}

func (ipr *process_range) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case generate:
		if m.id != ipr.loopID || ipr.generating == false {
			break
		}

		var list []gen.ProcessShortInfo
		nameLower := strings.ToLower(ipr.name)
		behaviorLower := strings.ToLower(ipr.behavior)
		appLower := strings.ToLower(ipr.application)

		ipr.Node().ProcessRangeShortInfo(func(info gen.ProcessShortInfo) bool {
			// apply filters
			if nameLower != "" {
				if strings.Contains(strings.ToLower(string(info.Name)), nameLower) == false {
					return true // skip, continue
				}
			}
			if behaviorLower != "" {
				if strings.Contains(strings.ToLower(info.Behavior), behaviorLower) == false {
					return true
				}
			}
			if appLower != "" {
				if strings.Contains(strings.ToLower(string(info.Application)), appLower) == false {
					return true
				}
			}
			if ipr.state != "" {
				if strings.EqualFold(info.State.String(), ipr.state) == false {
					return true
				}
			}
			if ipr.minMailbox > 0 {
				if info.MessagesMailbox < ipr.minMailbox {
					return true
				}
			}

			list = append(list, info)

			if ipr.limit > 0 && len(list) >= ipr.limit {
				return false // stop iteration
			}
			return true
		})

		slices.SortStableFunc(list, func(a, b gen.ProcessShortInfo) int {
			return int(a.PID.ID - b.PID.ID)
		})

		// reuse MessageInspectProcessList — same payload format
		ev := MessageInspectProcessList{
			Node:      ipr.Node().Name(),
			Processes: list,
		}

		if err := ipr.SendEvent(ipr.event, ipr.token, ev); err != nil {
			ipr.Log().Error("unable to send event %q: %s", ipr.event, err)
			return gen.TerminateReasonNormal
		}

		ipr.SendAfter(ipr.PID(), generate{id: ipr.loopID}, inspectProcessRangePeriod)

	case requestInspect:
		response := ResponseInspectProcessRange{
			Event: gen.Event{
				Name: ipr.event,
				Node: ipr.Node().Name(),
			},
		}
		ipr.SendResponse(m.pid, m.ref, response)

	case register:
		eopts := gen.EventOptions{
			Notify: true,
			Buffer: 1,
		}
		ipr.event = gen.Atom(fmt.Sprintf("%s_%s", inspectProcessRange, ipr.hash))
		token, err := ipr.RegisterEvent(ipr.event, eopts)
		if err != nil {
			ipr.Log().Error("unable to register event: %s", err)
			return err
		}
		ipr.Log().Info("registered event %s", ipr.event)
		ipr.token = token
		ipr.SendAfter(ipr.PID(), shutdown{}, inspectProcessRangeIdlePeriod)

	case shutdown:
		if ipr.generating {
			break
		}
		return gen.TerminateReasonNormal

	case gen.MessageEventStart:
		ipr.Log().Debug("got first subscriber. start generating events...")
		ipr.loopID++
		ipr.Send(ipr.PID(), generate{id: ipr.loopID})
		ipr.generating = true

	case gen.MessageEventStop:
		ipr.Log().Debug("no subscribers. stop generating")
		if ipr.generating {
			ipr.generating = false
			ipr.SendAfter(ipr.PID(), shutdown{}, inspectProcessRangeIdlePeriod)
		}

	default:
		ipr.Log().Error("unknown message (ignored) %#v", message)
	}

	return nil
}

func (ipr *process_range) Terminate(reason error) {
	ipr.Log().Debug("process range inspector terminated: %s", reason)
}

// filterHash builds a short deterministic suffix from filter fields
func filterHash(name, behavior, application, state string, minMailbox uint64, limit int) string {
	return fmt.Sprintf("%x", hashStr(fmt.Sprintf("%s|%s|%s|%s|%d|%d",
		name, behavior, application, state, minMailbox, limit)))
}

// eventListHash builds a short deterministic suffix from event list filter fields
func eventListHash(name string, notify, buffered int, minSubscribers int64, limit int) string {
	return fmt.Sprintf("%x", hashStr(fmt.Sprintf("%s|%d|%d|%d|%d",
		name, notify, buffered, minSubscribers, limit)))
}

func connectionListHash(name string, limit int) string {
	return fmt.Sprintf("%x", hashStr(fmt.Sprintf("%s|%d", name, limit)))
}

func hashStr(s string) uint32 {
	h := uint32(2166136261)
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}
