package inspect

import (
	"errors"
	"fmt"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

const (
	Name gen.Atom = "system_inspect"

	inspectNode           = "inspect_node"
	inspectNodePeriod     = 15 * time.Second
	inspectNodeIdlePeriod = 10 * time.Second

	inspectProcessList           = "inspect_process_list"
	inspectProcessListPeriod     = 15 * time.Second
	inspectProcessListIdlePeriod = 10 * time.Second

	inspectProcess           = "inspect_process"
	inspectProcessPeriod     = 15 * time.Second
	inspectProcessIdlePeriod = 10 * time.Second

	inspectProcessState           = "inspect_process_state"
	inspectProcessStatePeriod     = 15 * time.Second
	inspectProcessStateIdlePeriod = 10 * time.Second

	inspectMeta           = "inspect_meta"
	inspectMetaPeriod     = 15 * time.Second
	inspectMetaIdlePeriod = 10 * time.Second

	inspectMetaState           = "inspect_meta_state"
	inspectMetaStatePeriod     = 15 * time.Second
	inspectMetaStateIdlePeriod = 10 * time.Second

	inspectNetwork           = "inspect_network"
	inspectNetworkPeriod     = 15 * time.Second
	inspectNetworkIdlePeriod = 10 * time.Second

	inspectConnection           = "inspect_connection"
	inspectConnectionPeriod     = 15 * time.Second
	inspectConnectionIdlePeriod = 10 * time.Second

	inspectLog           = "inspect_log"
	inspectLogIdlePeriod = 10 * time.Second
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
	return &inspect{}
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
type generate struct{}

func (i *inspect) Init(args ...any) error {
	i.Log().SetLogger("default")
	i.Log().Debug("%s started", i.Name())
	return nil
}

func (i *inspect) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	switch r := request.(type) {
	case RequestInspectNode:
		// try to spawn node inspector process
		opts := gen.ProcessOptions{
			LinkParent: true,
		}
		_, err := i.SpawnRegister(inspectNode, factory_inode, opts)
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
		_, err := i.SpawnRegister(inspectNetwork, factory_inetwork, opts)
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
		_, err := i.SpawnRegister(pname, factory_iconnection, opts, r.RemoteNode)
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
		if r.Start < 1000 {
			r.Start = 1000
		}
		if r.Limit < 1 {
			r.Limit = 1000
		}
		pname := gen.Atom(fmt.Sprintf("%s_%d..+%d", inspectProcessList, r.Start, r.Limit))
		_, err := i.SpawnRegister(pname, factory_iprocess_list, opts, r.Start, r.Limit)
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

	case RequestInspectProcess:
		opts := gen.ProcessOptions{
			LinkParent: true,
		}
		pname := gen.Atom(fmt.Sprintf("%s_%s", inspectProcess, r.PID))
		_, err := i.SpawnRegister(pname, factory_iprocess, opts, r.PID)
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
		_, err := i.SpawnRegister(pname, factory_iprocess_state, opts, r.PID)
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
		_, err := i.SpawnRegister(pname, factory_imeta, opts, r.Meta)
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
		}
		_, err := i.SpawnRegister(inspectLog, factory_ilog, opts)
		if err != nil && err != gen.ErrTaken {
			return err, nil
		}
		// forward this request
		forward := requestInspect{
			pid: from,
			ref: ref,
		}
		i.Send(inspectLog, forward)
		return nil, nil // no reply

	// do commands

	case RequestDoSend:
		response := ResponseDoSend{
			Error: i.SendPID(r.PID, r.Message),
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

	case RequestDoSetLogLevelProcess:
		response := ResponseDoSetLogLevel{
			Error: i.Node().SetLogLevelProcess(r.PID, r.Level),
		}
		return response, nil

	case RequestDoSetLogLevelMeta:
		response := ResponseDoSetLogLevel{
			Error: i.Node().SetLogLevelMeta(r.Meta, r.Level),
		}
		return response, nil
	}

	i.Log().Error("unsupported request: %#v", request)
	return gen.ErrUnsupported, nil
}
