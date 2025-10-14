package unit

import (
	"fmt"
	"time"

	"ergo.services/ergo/gen"
)

// Event represents something that happened during a test
type Event interface {
	Type() string
	String() string
}

// SendEvent captures a Send operation
type SendEvent struct {
	From      gen.PID
	To        any
	Message   any
	Priority  gen.MessagePriority
	Important bool
	Ref       gen.Ref       // for responses
	After     time.Duration // for SendAfter
}

func (e SendEvent) Type() string {
	return "send"
}

func (e SendEvent) String() string {
	return "Send"
}

// SendResponseEvent captures a SendResponse operation
type SendResponseEvent struct {
	From     gen.PID
	To       gen.PID
	Response any
	Ref      gen.Ref
	Priority gen.MessagePriority
}

func (e SendResponseEvent) Type() string {
	return "send_response"
}

func (e SendResponseEvent) String() string {
	return fmt.Sprintf("SendResponse(to=%s, ref=%s)", e.To, e.Ref)
}

// SendResponseErrorEvent captures a SendResponseError operation
type SendResponseErrorEvent struct {
	From     gen.PID
	To       gen.PID
	Error    error
	Ref      gen.Ref
	Priority gen.MessagePriority
}

func (e SendResponseErrorEvent) Type() string {
	return "send_response_error"
}

func (e SendResponseErrorEvent) String() string {
	return fmt.Sprintf("SendResponseError(to=%s, ref=%s, error=%s)", e.To, e.Ref, e.Error)
}

// SpawnEvent captures a Spawn operation
type SpawnEvent struct {
	Factory gen.ProcessFactory
	Options gen.ProcessOptions
	Args    []any
	Result  gen.PID
}

func (e SpawnEvent) Type() string {
	return "spawn"
}

func (e SpawnEvent) String() string {
	return "Spawn"
}

// SpawnMetaEvent captures a SpawnMeta operation
type SpawnMetaEvent struct {
	Behavior gen.MetaBehavior
	Options  gen.MetaOptions
	Result   gen.Alias
}

func (e SpawnMetaEvent) Type() string {
	return "spawn_meta"
}

func (e SpawnMetaEvent) String() string {
	return "SpawnMeta"
}

// CallEvent captures a Call operation
type CallEvent struct {
	From      gen.PID
	To        any
	Request   any
	Response  any
	Error     error
	Timeout   int
	Priority  gen.MessagePriority
	Important bool
}

func (e CallEvent) Type() string {
	return "call"
}

func (e CallEvent) String() string {
	return "Call"
}

// LogEvent captures a Log operation
type LogEvent struct {
	Level   gen.LogLevel
	Message string
	Args    []any
}

func (e LogEvent) Type() string {
	return "log"
}

func (e LogEvent) String() string {
	return "Log"
}

// ExitEvent captures a SendExit operation
type ExitEvent struct {
	To     gen.PID
	Reason error
	After  time.Duration // for SendExitAfter
}

func (e ExitEvent) Type() string {
	return "exit"
}

func (e ExitEvent) String() string {
	return "SendExit"
}

// ExitMetaEvent captures a SendExitMeta operation
type ExitMetaEvent struct {
	Meta   gen.Alias
	Reason error
	After  time.Duration // for SendExitMetaAfter
}

func (e ExitMetaEvent) Type() string {
	return "exit_meta"
}

func (e ExitMetaEvent) String() string {
	return "SendExitMeta"
}

// MonitorEvent captures a Monitor operation
type MonitorEvent struct {
	Target any
}

func (e MonitorEvent) Type() string {
	return "monitor"
}

func (e MonitorEvent) String() string {
	return "Monitor"
}

// DemonitorEvent captures a Demonitor operation
type DemonitorEvent struct {
	Target any
}

func (e DemonitorEvent) Type() string {
	return "demonitor"
}

func (e DemonitorEvent) String() string {
	return "Demonitor"
}

// LinkEvent captures a Link operation
type LinkEvent struct {
	Target any
}

func (e LinkEvent) Type() string {
	return "link"
}

func (e LinkEvent) String() string {
	return "Link"
}

// UnlinkEvent captures an Unlink operation
type UnlinkEvent struct {
	Target any
}

func (e UnlinkEvent) Type() string {
	return "unlink"
}

func (e UnlinkEvent) String() string {
	return "Unlink"
}

// RegisterEvent captures a RegisterEvent operation
type RegisterEvent struct {
	Name    gen.Atom
	Options gen.EventOptions
	Result  gen.Ref
}

func (e RegisterEvent) Type() string {
	return "register_event"
}

func (e RegisterEvent) String() string {
	return "RegisterEvent"
}

// SendEventEvent captures a SendEvent operation
type SendEventEvent struct {
	Name    gen.Atom
	Token   gen.Ref
	Message any
	Options gen.MessageOptions
}

func (e SendEventEvent) Type() string {
	return "send_event"
}

func (e SendEventEvent) String() string {
	return "SendEvent"
}

// RegisterNameEvent captures a RegisterName operation
type RegisterNameEvent struct {
	Name gen.Atom
	PID  gen.PID
}

func (e RegisterNameEvent) Type() string {
	return "register_name"
}

func (e RegisterNameEvent) String() string {
	return "RegisterName"
}

// UnregisterNameEvent captures an UnregisterName operation
type UnregisterNameEvent struct {
	Name gen.Atom
}

func (e UnregisterNameEvent) Type() string {
	return "unregister_name"
}

func (e UnregisterNameEvent) String() string {
	return "UnregisterName"
}

// AliasEvent captures a CreateAlias operation
type AliasEvent struct {
	Result gen.Alias
}

func (e AliasEvent) Type() string {
	return "create_alias"
}

func (e AliasEvent) String() string {
	return "CreateAlias"
}

// RemoteSpawnEvent captures remote spawn operations
type RemoteSpawnEvent struct {
	Node     gen.Atom
	Name     gen.Atom
	Register gen.Atom // empty if not using SpawnRegister
	Options  gen.ProcessOptions
	Args     []any
	Result   gen.PID
	Error    error
}

// RemoteApplicationStartEvent captures remote application start operations
type RemoteApplicationStartEvent struct {
	Node    gen.Atom
	AppName gen.Atom
	Mode    gen.ApplicationMode // 0 for default, 1 for temporary, 2 for transient, 3 for permanent
	Options gen.ApplicationOptions
	Error   error
}

// NodeConnectionEvent captures node connection/disconnection events
type NodeConnectionEvent struct {
	Node      gen.Atom
	Connected bool
	Action    string // "connect", "disconnect", "create"
}

// NetworkRouteEvent captures route addition/removal
type NetworkRouteEvent struct {
	Pattern string
	Route   gen.NetworkRoute
	Weight  int
	Action  string // "add", "remove"
}

// ProxyRouteEvent captures proxy route operations
type ProxyRouteEvent struct {
	Pattern string
	Route   gen.NetworkProxyRoute
	Weight  int
	Action  string // "add", "remove"
}

func (e RemoteSpawnEvent) Type() string {
	return "remote_spawn"
}

func (e RemoteSpawnEvent) String() string {
	if e.Register != "" {
		return fmt.Sprintf("RemoteSpawnRegister(node=%s, name=%s, register=%s)", e.Node, e.Name, e.Register)
	}
	return fmt.Sprintf("RemoteSpawn(node=%s, name=%s)", e.Node, e.Name)
}

func (e RemoteApplicationStartEvent) Type() string {
	return "remote_application_start"
}

func (e RemoteApplicationStartEvent) String() string {
	return fmt.Sprintf("RemoteApplicationStart(node=%s, app=%s, mode=%d)", e.Node, e.AppName, e.Mode)
}

func (e NodeConnectionEvent) Type() string {
	return "node_connection"
}

func (e NodeConnectionEvent) String() string {
	return fmt.Sprintf("NodeConnection(node=%s, action=%s, connected=%t)", e.Node, e.Action, e.Connected)
}

func (e NetworkRouteEvent) Type() string {
	return "network_route"
}

func (e NetworkRouteEvent) String() string {
	return fmt.Sprintf("NetworkRoute(pattern=%s, action=%s)", e.Pattern, e.Action)
}

func (e ProxyRouteEvent) Type() string {
	return "proxy_route"
}

func (e ProxyRouteEvent) String() string {
	return fmt.Sprintf("ProxyRoute(pattern=%s, action=%s)", e.Pattern, e.Action)
}

// CronJobAddEvent captures cron job additions
type CronJobAddEvent struct {
	Job gen.CronJob
}

func (e CronJobAddEvent) Type() string {
	return "cron_job_add"
}

func (e CronJobAddEvent) String() string {
	return fmt.Sprintf("CronJobAdd(name=%s, spec=%s)", e.Job.Name, e.Job.Spec)
}

// CronJobRemoveEvent captures cron job removals
type CronJobRemoveEvent struct {
	Name gen.Atom
}

func (e CronJobRemoveEvent) Type() string {
	return "cron_job_remove"
}

func (e CronJobRemoveEvent) String() string {
	return fmt.Sprintf("CronJobRemove(name=%s)", e.Name)
}

// CronJobEnableEvent captures cron job enable operations
type CronJobEnableEvent struct {
	Name gen.Atom
}

func (e CronJobEnableEvent) Type() string {
	return "cron_job_enable"
}

func (e CronJobEnableEvent) String() string {
	return fmt.Sprintf("CronJobEnable(name=%s)", e.Name)
}

// CronJobDisableEvent captures cron job disable operations
type CronJobDisableEvent struct {
	Name gen.Atom
}

func (e CronJobDisableEvent) Type() string {
	return "cron_job_disable"
}

func (e CronJobDisableEvent) String() string {
	return fmt.Sprintf("CronJobDisable(name=%s)", e.Name)
}

// CronJobExecutionEvent captures cron job executions
type CronJobExecutionEvent struct {
	Name       gen.Atom
	Time       time.Time
	ActionInfo string
	Error      error
}

func (e CronJobExecutionEvent) Type() string {
	return "cron_job_execution"
}

func (e CronJobExecutionEvent) String() string {
	if e.Error != nil {
		return fmt.Sprintf("CronJobExecution(name=%s, time=%s, error=%s)", e.Name, e.Time, e.Error)
	}
	return fmt.Sprintf("CronJobExecution(name=%s, time=%s)", e.Name, e.Time)
}

// TerminateEvent captures when an actor terminates due to an error
type TerminateEvent struct {
	PID    gen.PID
	Reason error
}

func (e TerminateEvent) Type() string {
	return "terminate"
}

func (e TerminateEvent) String() string {
	return fmt.Sprintf("Terminate(pid=%s, reason=%s)", e.PID, e.Reason)
}
