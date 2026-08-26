package inspect

import (
	"fmt"
	"math"
	"sort"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

const (
	Name gen.Atom = "system_inspect"

	inspectNode           = "inspect_node"
	inspectNodePeriod     = time.Second
	inspectNodeIdlePeriod = 5 * time.Second

	inspectNodeShort           = "inspect_node_short"
	inspectNodeShortPeriod     = 3 * time.Second
	inspectNodeShortMinPeriod  = 100 * time.Millisecond
	inspectNodeShortIdlePeriod = 10 * time.Second

	inspectProcessList           = "inspect_process_list"
	inspectProcessListPeriod     = time.Second
	inspectProcessListIdlePeriod = 5 * time.Second

	inspectProcess           = "inspect_process"
	inspectProcessPeriod     = time.Second
	inspectProcessIdlePeriod = 5 * time.Second

	inspectMeta           = "inspect_meta"
	inspectMetaPeriod     = time.Second
	inspectMetaIdlePeriod = 5 * time.Second

	inspectNetwork           = "inspect_network"
	inspectNetworkPeriod     = time.Second
	inspectNetworkIdlePeriod = 5 * time.Second

	inspectConnection           = "inspect_connection"
	inspectConnectionPeriod     = time.Second
	inspectConnectionIdlePeriod = 5 * time.Second

	inspectLog           = "inspect_log"
	inspectLogIdlePeriod = 10 * time.Second

	inspectTracing = "inspect_tracing"

	inspectApplicationList           = "inspect_application_list"
	inspectApplicationListPeriod     = time.Second
	inspectApplicationListIdlePeriod = 5 * time.Second

	inspectEventList           = "inspect_event_list"
	inspectEventListPeriod     = time.Second
	inspectEventListIdlePeriod = 5 * time.Second

	inspectEvent           = "inspect_event"
	inspectEventStream     = "inspect_event_stream"
	inspectEventPeriod     = time.Second
	inspectEventIdlePeriod = 10 * time.Second

	inspectProcessRange           = "inspect_process_range"
	inspectProcessRangePeriod     = time.Second
	inspectProcessRangeIdlePeriod = 5 * time.Second

	inspectConnectionList           = "inspect_connection_list"
	inspectConnectionListPeriod     = time.Second
	inspectConnectionListIdlePeriod = 5 * time.Second

	inspectHeap           = "inspect_heap"
	inspectHeapPeriod     = time.Second
	inspectHeapIdlePeriod = 5 * time.Second
)

var (
	inspectLogFilter = []gen.LogLevel{
		gen.LogLevelDebug,
		gen.LogLevelInfo,
		gen.LogLevelWarning,
		gen.LogLevelError,
		gen.LogLevelPanic,
	}
)

func Factory() gen.ProcessBehavior {
	return &inspectPool{}
}

// Types returns the inspector wire-format types for use in
// gen.ApplicationSpec.Network.RegisterTypes.
func Types() []any {
	return []any{
		RequestInspectNode{}, ResponseInspectNode{}, MessageInspectNode{},
		RequestInspectNodeShort{}, ResponseInspectNodeShort{}, MessageInspectNodeShort{},
		RequestInspectNetwork{}, ResponseInspectNetwork{}, MessageInspectNetwork{},
		RequestInspectConnection{}, ResponseInspectConnection{}, MessageInspectConnection{},
		RequestInspectConnectionList{}, ResponseInspectConnectionList{}, MessageInspectConnectionList{},
		RequestInspectProcessList{}, ResponseInspectProcessList{}, MessageInspectProcessList{},
		RequestInspectProcessRange{}, ResponseInspectProcessRange{},
		RequestInspectEventList{}, ResponseInspectEventList{}, MessageInspectEventList{},
		RequestInspectEvent{}, ResponseInspectEvent{}, InspectEventEntry{}, MessageInspectEvent{},
		RequestInspectEventStream{}, ResponseInspectEventStream{},
		RequestInspectLog{}, ResponseInspectLog{}, InspectLogEntry{}, MessageInspectLog{},
		RequestInspectProcess{}, ResponseInspectProcess{}, MessageInspectProcess{},
		RequestInspectMeta{}, ResponseInspectMeta{}, MessageInspectMeta{},
		RequestInspectApplicationList{}, ResponseInspectApplicationList{}, MessageInspectApplicationList{},
		RequestInspectHeap{}, ResponseInspectHeap{}, MessageInspectHeap{},
		RequestInspectTracing{}, ResponseInspectTracing{}, MessageInspectTracing{},

		RequestGetCapabilities{}, ResponseGetCapabilities{},
		RequestGetAppTree{}, ResponseGetAppTree{},
		RequestGetSubtree{}, ResponseGetSubtree{},
		RequestGetProcessState{}, ResponseGetProcessState{},
		RequestGetProcessLookup{}, ResponseGetProcessLookup{},
		RequestGetCronInfo{}, ResponseGetCronInfo{},
		RequestGetCronSchedule{}, ResponseGetCronSchedule{},
		RequestGetRegistrarNodes{}, ResponseGetRegistrarNodes{},
		RequestGetRegistrarRoutes{}, ResponseGetRegistrarRoutes{},
		RequestGetRegistrarProxyRoutes{}, ResponseGetRegistrarProxyRoutes{},
		RequestGetRegistrarApplicationRoutes{}, ResponseGetRegistrarApplicationRoutes{},
		RequestGetMetaState{}, ResponseGetMetaState{},
		RequestGetGoroutines{}, GoroutineGroup{}, ResponseGetGoroutines{},
		RequestGetHeapProfile{}, HeapRecord{}, ResponseGetHeapProfile{},
		RequestGetTypes{}, ResponseGetTypes{},

		RequestGetNode{}, ResponseGetNode{},
		RequestGetNetwork{}, ResponseGetNetwork{},
		RequestGetConnection{}, ResponseGetConnection{},
		RequestGetConnectionList{}, ResponseGetConnectionList{},
		RequestGetProcessList{}, ResponseGetProcessList{},
		RequestGetProcessRange{}, ResponseGetProcessRange{},
		RequestGetProcess{}, ResponseGetProcess{},
		RequestGetMeta{}, ResponseGetMeta{},
		RequestGetApplicationList{}, ResponseGetApplicationList{},
		RequestGetEventList{}, ResponseGetEventList{},
		RequestGetEvent{}, ResponseGetEvent{},
	}
}

func workerFactory() gen.ProcessBehavior {
	return &inspect{}
}

type inspectPool struct {
	act.Pool
}

func (p *inspectPool) Init(args ...any) (act.PoolOptions, error) {
	return act.PoolOptions{
		PoolSize:      15,
		WorkerFactory: workerFactory,
	}, nil
}

type inspect struct {
	act.Actor

	caps          ResponseGetCapabilities
	manageProcess gen.Atom
	manageCaps    []string
}

type requestInspect struct {
	pid gen.PID
	ref gen.Ref
}

type register struct{}
type shutdown struct{}
type generate struct{ id uint64 }
type flushLog struct{ id uint64 }
type flushEvent struct{ id uint64 }

func (i *inspect) Init(args ...any) error {
	i.Log().SetLogger("default")
	i.Log().Debug("%s started", i.Name())
	i.SetCompression(true)
	i.caps = i.capabilities()
	i.manageProcess, i.manageCaps = i.manageCapabilities()
	return nil
}

func (i *inspect) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	switch r := request.(type) {
	case RequestInspectNode:
		// try to spawn node inspector process
		opts := gen.ProcessOptions{
			LinkParent: true,
		}
		_, err := i.SpawnRegister(inspectNode, factory_node, opts)
		if err != nil && err != gen.ErrTaken {
			return err, nil
		}
		// forward this request
		forward := requestInspect{
			pid: from,
			ref: ref,
		}
		i.Send(inspectNode, forward)
		return nil, nil // no reply

	case RequestInspectNodeShort:
		opts := gen.ProcessOptions{
			LinkParent: true,
		}

		period := r.Period
		if period == 0 {
			period = inspectNodeShortPeriod
		}
		if period < inspectNodeShortMinPeriod {
			period = inspectNodeShortMinPeriod
		}

		// the period is part of the identity: consumers wanting different rates
		// must not share an inspector, nor its event
		pname := gen.Atom(fmt.Sprintf("%s_%d", inspectNodeShort, period.Milliseconds()))
		_, err := i.SpawnRegister(pname, factory_node_short, opts, pname, period)
		if err != nil && err != gen.ErrTaken {
			return err, nil
		}
		// forward this request
		forward := requestInspect{
			pid: from,
			ref: ref,
		}
		i.Send(pname, forward)
		return nil, nil // no reply

	case RequestInspectNetwork:
		opts := gen.ProcessOptions{
			LinkParent: true,
		}
		_, err := i.SpawnRegister(inspectNetwork, factory_network, opts)
		if err != nil && err != gen.ErrTaken {
			return err, nil
		}
		// forward this request
		forward := requestInspect{
			pid: from,
			ref: ref,
		}
		i.Send(inspectNetwork, forward)
		return nil, nil // no reply

	case RequestInspectConnection:
		opts := gen.ProcessOptions{
			LinkParent: true,
		}
		pname := gen.Atom(fmt.Sprintf("%s_%s", inspectConnection, r.RemoteNode))
		_, err := i.SpawnRegister(pname, factory_connection, opts, r.RemoteNode)
		if err != nil && err != gen.ErrTaken {
			return err, nil
		}
		// forward this request
		forward := requestInspect{
			pid: from,
			ref: ref,
		}
		i.Send(pname, forward)
		return nil, nil // no reply

	case RequestInspectProcessList:
		opts := gen.ProcessOptions{
			LinkParent: true,
		}
		if r.Start >= 0 && r.Start < 1000 {
			r.Start = 1000
		}
		if r.Limit < 1 {
			r.Limit = 1000
		}
		hash := filterHash(r.Name, r.Behavior, r.Application, r.State, r.MinMailbox, r.Limit)
		pname := gen.Atom(fmt.Sprintf("%s_%d_%s", inspectProcessList, r.Start, hash))
		_, err := i.SpawnRegister(pname, factory_process_list, opts,
			r.Start, r.Limit, r.Name, r.Behavior, r.Application, r.State, r.MinMailbox)
		if err != nil && err != gen.ErrTaken {
			return err, nil
		}
		// forward this request
		forward := requestInspect{
			pid: from,
			ref: ref,
		}
		i.Send(pname, forward)
		return nil, nil // no reply

	case RequestInspectProcessRange:
		opts := gen.ProcessOptions{
			LinkParent: true,
		}
		if r.Limit < 1 {
			r.Limit = 10000
		}
		hash := filterHash(r.Name, r.Behavior, r.Application, r.State, r.MinMailbox, r.Limit)
		pname := gen.Atom(fmt.Sprintf("%s_%s", inspectProcessRange, hash))
		_, err := i.SpawnRegister(pname, factory_process_range, opts,
			r.Name, r.Behavior, r.Application, r.State, r.MinMailbox, r.Limit, hash)
		if err != nil && err != gen.ErrTaken {
			return err, nil
		}
		forward := requestInspect{
			pid: from,
			ref: ref,
		}
		i.Send(pname, forward)
		return nil, nil // no reply

	case RequestInspectProcess:
		opts := gen.ProcessOptions{
			LinkParent: true,
		}
		pname := gen.Atom(fmt.Sprintf("%s_%s", inspectProcess, r.PID))
		_, err := i.SpawnRegister(pname, factory_process, opts, r.PID)
		if err != nil && err != gen.ErrTaken {
			return err, nil
		}
		// forward this request
		forward := requestInspect{
			pid: from,
			ref: ref,
		}
		i.Send(pname, forward)
		return nil, nil // no reply

	case RequestInspectMeta:
		opts := gen.ProcessOptions{
			LinkParent: true,
		}
		pname := gen.Atom(fmt.Sprintf("%s_%s", inspectMeta, r.Meta))
		_, err := i.SpawnRegister(pname, factory_meta, opts, r.Meta)
		if err != nil && err != gen.ErrTaken {
			return err, nil
		}
		// forward this request
		forward := requestInspect{
			pid: from,
			ref: ref,
		}
		i.Send(pname, forward)
		return nil, nil // no reply

	case RequestInspectLog:
		// try to spawn node inspector process
		opts := gen.ProcessOptions{
			LinkParent: true,
			Compression: gen.Compression{
				Enable: true,
				Type:   gen.CompressionTypeGZIP,
				Level:  gen.CompressionBestSpeed,
			},
		}

		levels := r.Levels
		if len(r.Levels) > 0 {
			sort.Slice(levels, func(i, j int) bool { return levels[i] < levels[j] })
		} else {
			levels = inspectLogFilter
		}

		limit := r.Limit
		if limit < 1 {
			limit = 500
		}

		hash := fmt.Sprintf("%x", hashStr(fmt.Sprintf("%v|%d|%s|%v", levels, limit, r.MessagePattern, r.MessageExclude)))
		pname := gen.Atom(fmt.Sprintf("%s_%s", inspectLog, hash))
		_, err := i.SpawnRegister(pname, factory_log, opts, levels, limit, r.MessagePattern, r.MessageExclude)
		if err != nil && err != gen.ErrTaken {
			return err, nil
		}
		// forward this request
		forward := requestInspect{
			pid: from,
			ref: ref,
		}
		i.Send(pname, forward)
		return nil, nil // no reply

	case RequestInspectTracing:
		opts := gen.ProcessOptions{
			LinkParent: true,
			Compression: gen.Compression{
				Enable: true,
				Type:   gen.CompressionTypeGZIP,
				Level:  gen.CompressionBestSpeed,
			},
		}

		limit := r.Limit
		if limit < 1 {
			limit = 500
		}

		hash := fmt.Sprintf("%x", hashStr(fmt.Sprintf("%v|%d|%d|%d|%s|%v", r.Flags, limit, r.Kinds, r.Points, r.MessagePattern, r.MessageExclude)))
		pname := gen.Atom(fmt.Sprintf("%s_%s", inspectTracing, hash))
		_, err := i.SpawnRegister(pname, factory_tracing, opts, r.Flags, limit, r.Kinds, r.Points, r.MessagePattern, r.MessageExclude)
		if err != nil && err != gen.ErrTaken {
			return err, nil
		}
		forward := requestInspect{
			pid: from,
			ref: ref,
		}
		i.Send(pname, forward)
		return nil, nil

	case RequestInspectEventList:
		opts := gen.ProcessOptions{
			LinkParent: true,
		}
		if r.Limit < 1 {
			r.Limit = 500
		}
		hash := eventListHash(r.Timestamp, r.Name, r.Notify, r.Buffered, r.Open, r.MinSubscribers, r.Limit)
		pname := gen.Atom(fmt.Sprintf("%s_%s", inspectEventList, hash))
		_, err := i.SpawnRegister(pname, factory_event_list, opts,
			r.Timestamp, r.Name, r.Notify, r.Buffered, r.Open, r.MinSubscribers, r.Limit, hash)
		if err != nil && err != gen.ErrTaken {
			return err, nil
		}
		forward := requestInspect{
			pid: from,
			ref: ref,
		}
		i.Send(pname, forward)
		return nil, nil

	case RequestInspectEvent:
		opts := gen.ProcessOptions{
			LinkParent: true,
			Compression: gen.Compression{
				Enable: true,
				Type:   gen.CompressionTypeGZIP,
				Level:  gen.CompressionBestSpeed,
			},
		}
		hash := fmt.Sprintf("%x", hashStr(string(r.Name)))
		pname := gen.Atom(fmt.Sprintf("%s_%s", inspectEvent, hash))
		_, err := i.SpawnRegister(pname, factory_event, opts, eventArgs{Name: r.Name, Hash: hash})
		if err != nil && err != gen.ErrTaken {
			return err, nil
		}
		forward := requestInspect{
			pid: from,
			ref: ref,
		}
		i.Send(pname, forward)
		return nil, nil

	case RequestInspectEventStream:
		opts := gen.ProcessOptions{
			LinkParent: true,
			Compression: gen.Compression{
				Enable: true,
				Type:   gen.CompressionTypeGZIP,
				Level:  gen.CompressionBestSpeed,
			},
		}

		limit := r.Limit
		if limit < 1 {
			limit = 500
		}

		hash := fmt.Sprintf("%x", hashStr(fmt.Sprintf("%s|%d|%s|%s|%v|%v|%v", r.Name, limit, r.TypePattern, r.MessagePattern, r.MessageExclude, r.Force, r.Verbose)))
		pname := gen.Atom(fmt.Sprintf("%s_%s", inspectEventStream, hash))
		_, err := i.SpawnRegister(pname, factory_event_stream, opts, eventStreamArgs{
			Name:           r.Name,
			Limit:          limit,
			TypePattern:    r.TypePattern,
			MessagePattern: r.MessagePattern,
			MessageExclude: r.MessageExclude,
			Hash:           hash,
			Force:          r.Force,
			Verbose:        r.Verbose,
		})
		if err != nil && err != gen.ErrTaken {
			return err, nil
		}
		forward := requestInspect{
			pid: from,
			ref: ref,
		}
		i.Send(pname, forward)
		return nil, nil

	case RequestInspectConnectionList:
		opts := gen.ProcessOptions{
			LinkParent: true,
		}
		if r.Limit < 1 {
			r.Limit = 100
		}
		hash := connectionListHash(r.Name, r.Limit)
		pname := gen.Atom(fmt.Sprintf("%s_%s", inspectConnectionList, hash))
		_, err := i.SpawnRegister(pname, factory_connection_list, opts, r.Name, r.Limit, hash)
		if err != nil && err != gen.ErrTaken {
			return err, nil
		}
		forward := requestInspect{
			pid: from,
			ref: ref,
		}
		i.Send(pname, forward)
		return nil, nil

	case RequestInspectApplicationList:
		opts := gen.ProcessOptions{
			LinkParent: true,
		}
		_, err := i.SpawnRegister(inspectApplicationList, factory_application_list, opts)
		if err != nil && err != gen.ErrTaken {
			return err, nil
		}
		// forward this request
		forward := requestInspect{
			pid: from,
			ref: ref,
		}
		i.Send(inspectApplicationList, forward)
		return nil, nil // no reply

	case RequestInspectHeap:
		opts := gen.ProcessOptions{LinkParent: true}
		if r.Limit < 1 {
			r.Limit = 100
		}
		hash := filterHash(r.Name, "", "", "", 0, r.Limit)
		pname := gen.Atom(fmt.Sprintf("%s_%s", inspectHeap, hash))
		_, err := i.SpawnRegister(pname, factory_heap, opts, r.Limit, r.Name)
		if err != nil && err != gen.ErrTaken {
			return err, nil
		}
		forward := requestInspect{pid: from, ref: ref}
		i.Send(pname, forward)
		return nil, nil

	// one-shot reads

	case RequestGetCapabilities:
		return i.responseCapabilities(), nil

	case RequestGetAppTree:
		limit := r.Limit
		if limit < 1 {
			limit = 1000
		}
		list, omitted, err := i.Node().ApplicationProcessListShortInfo(r.Application, limit)
		return ResponseGetAppTree{
			Node:        i.Node().Name(),
			Application: r.Application,
			Processes:   list,
			Truncated:   omitted,
			Error:       err,
		}, nil

	case RequestGetSubtree:
		limit := r.Limit
		if limit < 1 {
			limit = 1000
		}
		list, truncated, err := i.subtree(r.PID, limit)
		return ResponseGetSubtree{
			Node:      i.Node().Name(),
			PID:       r.PID,
			Processes: list,
			Truncated: truncated,
			Error:     err,
		}, nil

	case RequestGetProcessState:
		if r.PID == i.PID() {
			return ResponseGetProcessState{State: i.HandleInspect(i.PID(), r.Items...)}, nil
		}
		state, err := i.Inspect(r.PID, r.Items...)
		return ResponseGetProcessState{State: state, Error: err}, nil

	case RequestGetMetaState:
		state, err := i.InspectMeta(r.Meta, r.Items...)
		return ResponseGetMetaState{State: state, Error: err}, nil

	case RequestGetProcessLookup:
		return i.responseProcessLookup(r), nil

	case RequestGetCronInfo:
		return i.responseCronInfo(r), nil

	case RequestGetCronSchedule:
		return i.responseCronSchedule(r), nil

	case RequestGetRegistrarNodes:
		return i.responseRegistrarNodes(), nil

	case RequestGetRegistrarRoutes:
		return i.responseRegistrarRoutes(r), nil

	case RequestGetRegistrarProxyRoutes:
		return i.responseRegistrarProxyRoutes(r), nil

	case RequestGetRegistrarApplicationRoutes:
		return i.responseRegistrarApplicationRoutes(r), nil

	case RequestGetGoroutines:
		return captureGoroutines(r), nil

	case RequestGetHeapProfile:
		return captureHeapProfile(r), nil

	case RequestGetTypes:
		return i.responseTypes(r), nil

	case RequestGetNode:
		return i.responseNode(), nil

	case RequestGetNetwork:
		return i.responseNetwork(), nil

	case RequestGetConnection:
		return i.responseConnection(r), nil

	case RequestGetConnectionList:
		return i.responseConnectionList(r), nil

	case RequestGetProcessList:
		return i.responseProcessList(r), nil

	case RequestGetProcessRange:
		return i.responseProcessRange(r), nil

	case RequestGetProcess:
		return i.responseProcess(r), nil

	case RequestGetMeta:
		return i.responseMeta(r), nil

	case RequestGetApplicationList:
		return i.responseApplicationList(), nil

	case RequestGetEventList:
		return i.responseEventList(r), nil

	case RequestGetEvent:
		return i.responseEvent(r), nil
	}

	i.Log().Error("unsupported request: %#v", request)
	return gen.ErrUnsupported, nil
}

// subtree returns the process tree rooted at pid (the process itself plus all
// descendants), capped at limit. It relies on the same id-ordering invariant as
// appTree: starting from pid's id and walking ids upward, a process belongs to
// the subtree iff its parent is already known to belong (the root, or a process
// added earlier in the walk). Connectivity holds under truncation.
func (i *inspect) subtree(pid gen.PID, limit int) ([]gen.ProcessShortInfo, int, error) {
	inSub := map[gen.PID]bool{pid: true}
	matched := 0
	list, err := i.Node().ProcessListShortInfo(int(pid.ID), math.MaxInt, func(p gen.ProcessShortInfo) bool {
		if p.PID != pid && inSub[p.Parent] == false {
			return false
		}
		inSub[p.PID] = true
		matched++
		return matched <= limit
	})
	if err != nil {
		return nil, 0, err
	}
	if len(list) == 0 {
		return nil, 0, gen.ErrProcessUnknown
	}
	return list, matched - len(list), nil
}
