package inspect

import (
	"errors"
	"fmt"
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

	inspectProcessList           = "inspect_process_list"
	inspectProcessListPeriod     = time.Second
	inspectProcessListIdlePeriod = 5 * time.Second

	inspectProcess           = "inspect_process"
	inspectProcessPeriod     = time.Second
	inspectProcessIdlePeriod = 5 * time.Second

	inspectProcessState           = "inspect_process_state"
	inspectProcessStatePeriod     = time.Second
	inspectProcessStateIdlePeriod = 5 * time.Second

	inspectMeta           = "inspect_meta"
	inspectMetaPeriod     = time.Second
	inspectMetaIdlePeriod = 5 * time.Second

	inspectMetaState           = "inspect_meta_state"
	inspectMetaStatePeriod     = time.Second
	inspectMetaStateIdlePeriod = 5 * time.Second

	inspectNetwork           = "inspect_network"
	inspectNetworkPeriod     = time.Second
	inspectNetworkIdlePeriod = 5 * time.Second

	inspectConnection           = "inspect_connection"
	inspectConnectionPeriod     = time.Second
	inspectConnectionIdlePeriod = 5 * time.Second

	inspectLog           = "inspect_log"
	inspectLogIdlePeriod = 10 * time.Second

	inspectApplicationList           = "inspect_application_list"
	inspectApplicationListPeriod     = time.Second
	inspectApplicationListIdlePeriod = 5 * time.Second

	inspectApplicationTree           = "inspect_application_tree"
	inspectApplicationTreePeriod     = time.Second
	inspectApplicationTreeIdlePeriod = 5 * time.Second

	inspectEventList           = "inspect_event_list"
	inspectEventListPeriod     = time.Second
	inspectEventListIdlePeriod = 5 * time.Second

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

func workerFactory() gen.ProcessBehavior {
	return &inspect{}
}

type inspectPool struct {
	act.Pool
}

func (p *inspectPool) Init(args ...any) (act.PoolOptions, error) {
	return act.PoolOptions{
		PoolSize:      5,
		WorkerFactory: workerFactory,
	}, nil
}

type inspect struct {
	act.Actor
}

type requestInspect struct {
	pid gen.PID
	ref gen.Ref
}

type register struct{}
type shutdown struct{}
type generate struct{ id uint64 }
type flushLog struct{ id uint64 }

func (i *inspect) Init(args ...any) error {
	i.Log().SetLogger("default")
	i.Log().Debug("%s started", i.Name())
	i.SetCompression(true)
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

	case RequestInspectProcessState:
		if r.PID == i.PID() {
			return errors.New("unable to inspect the state of itself"), nil
		}
		opts := gen.ProcessOptions{
			LinkParent: true,
		}
		pname := gen.Atom(fmt.Sprintf("%s_%s", inspectProcessState, r.PID))
		_, err := i.SpawnRegister(pname, factory_process_state, opts, r.PID)
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

	case RequestInspectMetaState:
		opts := gen.ProcessOptions{
			LinkParent: true,
		}
		pname := gen.Atom(fmt.Sprintf("%s_%s", inspectMetaState, r.Meta))
		_, err := i.SpawnRegister(pname, factory_meta_state, opts, r.Meta)
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

	case RequestInspectEventList:
		opts := gen.ProcessOptions{
			LinkParent: true,
		}
		if r.Limit < 1 {
			r.Limit = 500
		}
		hash := eventListHash(r.Name, r.Notify, r.Buffered, r.MinSubscribers, r.Limit)
		pname := gen.Atom(fmt.Sprintf("%s_%s", inspectEventList, hash))
		_, err := i.SpawnRegister(pname, factory_event_list, opts,
			r.Name, r.Notify, r.Buffered, r.MinSubscribers, r.Limit, hash)
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

	case RequestInspectApplicationTree:
		opts := gen.ProcessOptions{
			LinkParent: true,
		}
		if r.Limit < 1 {
			r.Limit = 1000
		}
		pname := gen.Atom(fmt.Sprintf("%s_%s_%d", inspectApplicationTree, r.Application, r.Limit))
		_, err := i.SpawnRegister(pname, factory_application_tree, opts, r.Application, r.Limit)
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

	// do commands

	case RequestDoSend:
		response := ResponseDoSend{
			Error: i.SendWithPriority(r.PID, r.Message, r.Priority),
		}
		return response, nil

	case RequestDoSendMeta:
		response := ResponseDoSendMeta{
			Error: i.SendAlias(r.Meta, r.Message),
		}
		return response, nil

	case RequestDoSendExit:
		response := ResponseDoSendExit{
			Error: i.SendExit(r.PID, r.Reason),
		}
		return response, nil

	case RequestDoSendExitMeta:
		response := ResponseDoSendExit{
			Error: i.SendExitMeta(r.Meta, r.Reason),
		}
		return response, nil

	case RequestDoKill:
		response := ResponseDoKill{
			Error: i.Node().Kill(r.PID),
		}
		return response, nil

	case RequestDoSetLogLevel:
		response := ResponseDoSetLogLevel{
			Error: i.Node().Log().SetLevel(r.Level),
		}
		return response, nil

	case RequestDoSetProcessLogLevel:
		response := ResponseDoSetLogLevel{
			Error: i.Node().SetProcessLogLevel(r.PID, r.Level),
		}
		return response, nil

	case RequestDoSetMetaLogLevel:
		response := ResponseDoSetLogLevel{
			Error: i.Node().SetMetaLogLevel(r.Meta, r.Level),
		}
		return response, nil

	// process settings

	case RequestDoSetProcessSendPriority:
		return ResponseDoSet{Error: i.Node().SetProcessSendPriority(r.PID, r.Priority)}, nil

	case RequestDoSetProcessCompression:
		return ResponseDoSet{Error: i.Node().SetProcessCompression(r.PID, r.Enabled)}, nil

	case RequestDoSetProcessCompressionType:
		return ResponseDoSet{Error: i.Node().SetProcessCompressionType(r.PID, r.Type)}, nil

	case RequestDoSetProcessCompressionLevel:
		return ResponseDoSet{Error: i.Node().SetProcessCompressionLevel(r.PID, r.Level)}, nil

	case RequestDoSetProcessCompressionThreshold:
		return ResponseDoSet{Error: i.Node().SetProcessCompressionThreshold(r.PID, r.Threshold)}, nil

	case RequestDoSetProcessKeepNetworkOrder:
		return ResponseDoSet{Error: i.Node().SetProcessKeepNetworkOrder(r.PID, r.Order)}, nil

	case RequestDoSetProcessImportantDelivery:
		return ResponseDoSet{Error: i.Node().SetProcessImportantDelivery(r.PID, r.Important)}, nil

	// meta settings

	case RequestDoSetMetaSendPriority:
		return ResponseDoSet{Error: i.Node().SetMetaSendPriority(r.Meta, r.Priority)}, nil

	// app lifecycle

	case RequestDoAppStart:
		opts := gen.ApplicationOptions{}
		var err error
		switch r.Mode {
		case gen.ApplicationModeTemporary:
			err = i.Node().ApplicationStartTemporary(r.Name, opts)
		case gen.ApplicationModeTransient:
			err = i.Node().ApplicationStartTransient(r.Name, opts)
		case gen.ApplicationModePermanent:
			err = i.Node().ApplicationStartPermanent(r.Name, opts)
		default:
			err = i.Node().ApplicationStart(r.Name, opts)
		}
		return ResponseDoAppStart{Error: err}, nil

	case RequestDoAppStop:
		var err error
		if r.Force {
			err = i.Node().ApplicationStopForce(r.Name)
		} else {
			err = i.Node().ApplicationStop(r.Name)
		}
		return ResponseDoAppStop{Error: err}, nil

	case RequestDoAppUnload:
		return ResponseDoAppUnload{Error: i.Node().ApplicationUnload(r.Name)}, nil

	// one-shot inspect

	case RequestDoInspect:
		state, err := i.Inspect(r.PID)
		return ResponseDoInspect{State: state, Error: err}, nil

	case RequestDoGoroutines:
		return captureGoroutines(r), nil

	case RequestDoHeapProfile:
		return captureHeapProfile(r), nil
	}

	i.Log().Error("unsupported request: %#v", request)
	return gen.ErrUnsupported, nil
}
