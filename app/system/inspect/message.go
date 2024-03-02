package inspect

import "ergo.services/ergo/gen"

type RequestInspectNode struct{}
type ResponseInspectNode struct {
	CRC32    string
	Event    gen.Event
	OS       string
	Arch     string
	Cores    int
	Version  gen.Version
	Creation int64
}

type MessageInspectNode struct {
	Node gen.Atom
	Info gen.NodeInfo
}

// network

type RequestInspectNetwork struct{}
type ResponseInspectNetwork struct {
	Event   gen.Event
	Stopped bool
	Info    gen.NetworkInfo
}

type MessageInspectNetwork struct {
	Node    gen.Atom
	Stopped bool
	Info    gen.NetworkInfo
}

type RequestInspectConnection struct {
	RemoteNode gen.Atom
}
type ResponseInspectConnection struct {
	Event        gen.Event
	Disconnected bool
	Info         gen.RemoteNodeInfo
}

type MessageInspectConnection struct {
	Node         gen.Atom
	Disconnected bool
	Info         gen.RemoteNodeInfo
}

// process list

type RequestInspectProcessList struct {
	Start int
	Limit int
}
type ResponseInspectProcessList struct {
	Event gen.Event
}

type MessageInspectProcessList struct {
	Node      gen.Atom
	Processes []gen.ProcessShortInfo
}

// node logs

type RequestInspectLog struct {
	Levels []gen.LogLevel
}
type ResponseInspectLog struct {
	Event gen.Event
}

type MessageInspectLogNode struct {
	Node      gen.Atom
	Creation  int64
	Timestamp int64
	Level     gen.LogLevel
	Message   string
}

type MessageInspectLogProcess struct {
	Node      gen.Atom
	Name      gen.Atom
	PID       gen.PID
	Timestamp int64
	Level     gen.LogLevel
	Message   string
}

type MessageInspectLogNetwork struct {
	Node      gen.Atom
	Peer      gen.Atom
	Timestamp int64
	Level     gen.LogLevel
	Message   string
}

type MessageInspectLogMeta struct {
	Node      gen.Atom
	Parent    gen.PID
	Meta      gen.Alias
	Timestamp int64
	Level     gen.LogLevel
	Message   string
}

// process

type RequestInspectProcess struct {
	PID gen.PID
}
type ResponseInspectProcess struct {
	Event gen.Event
}

type MessageInspectProcess struct {
	Node       gen.Atom
	Info       gen.ProcessInfo
	Terminated bool
}

// process state
type RequestInspectProcessState struct {
	PID gen.PID
}
type ResponseInspectProcessState struct {
	Event gen.Event
}

type MessageInspectProcessState struct {
	Node  gen.Atom
	PID   gen.PID
	State map[string]string
}

// meta

type RequestInspectMeta struct {
	Meta gen.Alias
}
type ResponseInspectMeta struct {
	Event gen.Event
}

type MessageInspectMeta struct {
	Node       gen.Atom
	Info       gen.MetaInfo
	Terminated bool
}

// meta state
type RequestInspectMetaState struct {
	Meta gen.Alias
}
type ResponseInspectMetaState struct {
	Event gen.Event
}

type MessageInspectMetaState struct {
	Node  gen.Atom
	Meta  gen.Alias
	State map[string]string
}

// do send

type RequestDoSend struct {
	PID     gen.PID
	Message any
}
type ResponseDoSend struct {
	Error error
}

type RequestDoSendMeta struct {
	Meta    gen.Alias
	Message any
}
type ResponseDoSendMeta struct {
	Error error
}

// do send exit

type RequestDoSendExit struct {
	PID    gen.PID
	Reason error
}
type ResponseDoSendExit struct {
	Error error
}

type RequestDoSendExitMeta struct {
	Meta   gen.Alias
	Reason error
}
type ResponseDoSendExitMeta struct {
	Error error
}

// do kill

type RequestDoKill struct {
	PID gen.PID
}
type ResponseDoKill struct {
	Error error
}

// do set log level

// node
type RequestDoSetLogLevel struct {
	Level gen.LogLevel
}
type ResponseDoSetLogLevel struct {
	Error error
}

// process
type RequestDoSetLogLevelProcess struct {
	PID   gen.PID
	Level gen.LogLevel
}

// meta
type RequestDoSetLogLevelMeta struct {
	Meta  gen.Alias
	Level gen.LogLevel
}
