package inspect

import (
	"cmp"
	"fmt"
	"runtime"
	"runtime/debug"
	"slices"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

// readBuildInfo extracts the running binary's module/build details for the
// inspect node response. Returns zero values if build info is unavailable.
func readBuildInfo() (main, revision string, modified bool, settings, deps, replaces []string) {
	bi, ok := debug.ReadBuildInfo()
	if ok == false {
		return
	}
	main = bi.Main.Path
	if bi.Main.Version != "" {
		main = bi.Main.Path + "@" + bi.Main.Version
	}
	settings = make([]string, 0, len(bi.Settings))
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
		settings = append(settings, s.Key+"="+s.Value)
	}
	verOf := func(m *debug.Module) string {
		if m.Version != "" {
			return m.Path + "@" + m.Version
		}
		return m.Path
	}
	deps = make([]string, 0, len(bi.Deps))
	for _, d := range bi.Deps {
		// list the required module itself; the replacement (if any) is reported separately
		deps = append(deps, verOf(d))
		if d.Replace != nil {
			replaces = append(replaces, d.Path+" => "+verOf(d.Replace))
		}
	}
	slices.Sort(deps)
	slices.Sort(replaces)
	return
}

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
	in.SetProcessKind(gen.ProcessKindMonitor)
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
		response.BuildMain, response.BuildRevision,
			response.BuildModified, response.BuildSettings, response.BuildDeps,
			response.BuildReplaces = readBuildInfo()
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
