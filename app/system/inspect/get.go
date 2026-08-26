package inspect

import (
	"slices"
	"strings"

	"ergo.services/ergo/gen"
)

const getProcessLimit = 1000
const getProcessStart = 1000

type processFilter struct {
	Name        string
	Behavior    string
	Application string
	State       string
	MinMailbox  uint64
}

func (f processFilter) set() bool {
	return f.Name != "" || f.Behavior != "" || f.Application != "" ||
		f.State != "" || f.MinMailbox > 0
}

func (f processFilter) match(info gen.ProcessShortInfo) bool {
	if f.Name != "" &&
		strings.Contains(strings.ToLower(string(info.Name)), strings.ToLower(f.Name)) == false {
		return false
	}
	if f.Behavior != "" &&
		strings.Contains(strings.ToLower(info.Behavior), strings.ToLower(f.Behavior)) == false {
		return false
	}
	if f.Application != "" &&
		strings.Contains(strings.ToLower(string(info.Application)), strings.ToLower(f.Application)) == false {
		return false
	}
	if f.State != "" && strings.EqualFold(info.State.String(), f.State) == false {
		return false
	}
	if f.MinMailbox > 0 && info.MessagesMailbox < f.MinMailbox {
		return false
	}
	return true
}

type eventFilter struct {
	Name           string
	Notify         int
	Buffered       int
	Open           int
	MinSubscribers int64
}

func (f eventFilter) match(info gen.EventInfo) bool {
	if f.Name != "" &&
		strings.Contains(strings.ToLower(string(info.Event.Name)), strings.ToLower(f.Name)) == false {
		return false
	}
	if f.Notify == 1 && info.Notify == false {
		return false
	}
	if f.Notify == -1 && info.Notify == true {
		return false
	}
	if f.Buffered == 1 && info.BufferSize == 0 {
		return false
	}
	if f.Buffered == -1 && info.BufferSize > 0 {
		return false
	}
	if f.Open == 1 && info.Open == false {
		return false
	}
	if f.Open == -1 && info.Open == true {
		return false
	}
	if f.MinSubscribers > 0 && info.Subscribers < f.MinSubscribers {
		return false
	}
	return true
}

func (i *inspect) responseNode() ResponseGetNode {
	info, err := i.Node().Info()
	return ResponseGetNode{Node: i.Node().Name(), Info: info, Error: err}
}

func (i *inspect) responseNetwork() ResponseGetNetwork {
	info, err := i.Node().Network().Info()
	return ResponseGetNetwork{Node: i.Node().Name(), Info: info, Error: err}
}

func (i *inspect) responseConnection(r RequestGetConnection) ResponseGetConnection {
	out := ResponseGetConnection{Node: i.Node().Name()}
	remote, err := i.Node().Network().Node(r.RemoteNode)
	if err != nil {
		out.Error = err
		return out
	}
	out.Info = remote.Info()
	return out
}

func (i *inspect) responseConnectionList(r RequestGetConnectionList) ResponseGetConnectionList {
	out := ResponseGetConnectionList{Node: i.Node().Name()}

	network, err := i.Node().Network().Info()
	if err != nil {
		out.Error = err
		return out
	}

	name := strings.ToLower(r.Name)
	nodes := slices.Clone(network.Nodes)
	slices.Sort(nodes)

	for _, n := range nodes {
		if name != "" && strings.Contains(strings.ToLower(string(n)), name) == false {
			continue
		}
		remote, err := i.Node().Network().Node(n)
		if err != nil {
			continue
		}
		out.Connections = append(out.Connections, remote.Info())
		if r.Limit > 0 && len(out.Connections) >= r.Limit {
			break
		}
	}
	return out
}

func (i *inspect) responseProcessList(r RequestGetProcessList) ResponseGetProcessList {
	out := ResponseGetProcessList{Node: i.Node().Name()}

	start, limit := r.Start, r.Limit
	if start == 0 {
		start = getProcessStart
	}
	if limit == 0 {
		limit = getProcessLimit
	}

	filter := processFilter{
		Name: r.Name, Behavior: r.Behavior, Application: r.Application,
		State: r.State, MinMailbox: r.MinMailbox,
	}

	var predicate []func(gen.ProcessShortInfo) bool
	if filter.set() {
		predicate = append(predicate, filter.match)
	}

	list, err := i.Node().ProcessListShortInfo(start, limit+1, predicate...)
	if err != nil {
		out.Error = err
		return out
	}
	if len(list) > limit {
		out.Truncated = true
		list = list[:limit]
	}

	slices.SortStableFunc(list, func(a, b gen.ProcessShortInfo) int {
		return int(a.PID.ID - b.PID.ID)
	})
	out.Processes = list
	return out
}

func (i *inspect) responseProcessRange(r RequestGetProcessRange) ResponseGetProcessRange {
	out := ResponseGetProcessRange{Node: i.Node().Name()}

	limit := r.Limit
	if limit == 0 {
		limit = getProcessLimit
	}

	filter := processFilter{
		Name: r.Name, Behavior: r.Behavior, Application: r.Application,
		State: r.State, MinMailbox: r.MinMailbox,
	}

	list := []gen.ProcessShortInfo{}
	err := i.Node().ProcessRangeShortInfo(func(info gen.ProcessShortInfo) bool {
		if filter.match(info) == false {
			return true
		}
		if len(list) == limit {
			out.Truncated = true
			return false
		}
		list = append(list, info)
		return true
	})
	if err != nil {
		out.Error = err
		return out
	}

	slices.SortStableFunc(list, func(a, b gen.ProcessShortInfo) int {
		return int(a.PID.ID - b.PID.ID)
	})
	out.Processes = list
	return out
}

func (i *inspect) responseProcess(r RequestGetProcess) ResponseGetProcess {
	info, err := i.Node().ProcessInfo(r.PID)
	return ResponseGetProcess{Node: i.Node().Name(), Info: info, Error: err}
}

func (i *inspect) responseMeta(r RequestGetMeta) ResponseGetMeta {
	info, err := i.MetaInfo(r.Meta)
	return ResponseGetMeta{Node: i.Node().Name(), Info: info, Error: err}
}

func (i *inspect) responseTypes(r RequestGetTypes) ResponseGetTypes {
	registered := i.Node().Network().RegisteredTypes()

	types := registered
	if r.Name != "" || r.Kind != "" {
		types = []gen.RegisteredTypeInfo{}
		for _, t := range registered {
			if r.Name != "" &&
				strings.Contains(strings.ToLower(t.Name), strings.ToLower(r.Name)) == false {
				continue
			}
			if r.Kind != "" && strings.EqualFold(t.Kind, r.Kind) == false {
				continue
			}
			types = append(types, t)
		}
	}

	out := ResponseGetTypes{Types: types}
	if r.Limit > 0 && len(types) > r.Limit {
		out.Truncated = len(types) - r.Limit
		out.Types = types[:r.Limit]
	}
	return out
}

func (i *inspect) responseApplicationList() ResponseGetApplicationList {
	out := ResponseGetApplicationList{
		Node:         i.Node().Name(),
		Applications: map[gen.Atom]gen.ApplicationInfo{},
	}
	for _, name := range i.Node().Applications() {
		info, err := i.Node().ApplicationInfo(name)
		if err != nil {
			continue
		}
		out.Applications[name] = info
	}
	return out
}

func (i *inspect) responseEventList(r RequestGetEventList) ResponseGetEventList {
	out := ResponseGetEventList{Node: i.Node().Name()}

	limit := r.Limit
	if limit == 0 {
		limit = getProcessLimit
	}

	filter := eventFilter{
		Name: r.Name, Notify: r.Notify, Buffered: r.Buffered,
		Open: r.Open, MinSubscribers: r.MinSubscribers,
	}

	events, err := i.Node().EventListInfo(r.Timestamp, limit, filter.match)
	if err != nil {
		out.Error = err
		return out
	}
	out.Events = events
	return out
}

func (i *inspect) responseEvent(r RequestGetEvent) ResponseGetEvent {
	event := gen.Event{Name: r.Name, Node: i.Node().Name()}
	info, err := i.Node().EventInfo(event)
	return ResponseGetEvent{Node: i.Node().Name(), Info: info, Error: err}
}
