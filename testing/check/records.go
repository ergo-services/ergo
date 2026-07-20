package check

import (
	"fmt"
	"time"

	"ergo.services/ergo/gen"
)

// records

// Send is an outgoing message observed at the sender (egress). Error is the
// outcome of the send call (nil on success). Options is the effective
// gen.MessageOptions the send was issued with (priority, compression, keep-order,
// important delivery) - what the routing core receives.
type Send struct {
	From    gen.PID
	To      any
	Message any
	Options gen.MessageOptions
	Error   error
}

func (Send) Kind() string { return "sent" }
func (r Send) String() string {
	return fmt.Sprintf("Send(from=%s to=%v msg=%#v err=%v)", r.From, r.To, r.Message, r.Error)
}

// Call is an outgoing request observed at the caller (egress). Error is the
// outcome of the call (nil on success); Response is the value returned.
type Call struct {
	From     gen.PID
	To       any
	Request  any
	Response any
	Error    error
}

func (Call) Kind() string { return "called" }
func (r Call) String() string {
	return fmt.Sprintf("Call(from=%s to=%v req=%#v err=%v)", r.From, r.To, r.Request, r.Error)
}

// Spawn is a child process created (or attempted) by a process (egress). On
// failure Child is the zero PID and Error is set. Factory is the spawned factory
// (set by harnesses that know it, e.g. the unit mock); zero otherwise. Register is
// the name for a SpawnRegister (empty for an anonymous Spawn). Options is the
// gen.ProcessOptions the spawn was requested with.
type Spawn struct {
	Parent   gen.PID
	Child    gen.PID
	Register gen.Atom
	Factory  gen.ProcessFactory
	Options  gen.ProcessOptions
	Error    error
}

func (Spawn) Kind() string { return "spawned" }
func (r Spawn) String() string {
	return fmt.Sprintf("Spawn(parent=%s child=%s register=%s err=%v)", r.Parent, r.Child, r.Register, r.Error)
}

// RemoteSpawn is a process spawned (or attempted) on a remote node by name
// (egress). Node is the target node, Name the remote factory name, Register the
// name to register the child under (empty for a plain RemoteSpawn). On failure
// Child is the zero PID and Error is set.
type RemoteSpawn struct {
	Parent   gen.PID
	Node     gen.Atom
	Name     gen.Atom
	Register gen.Atom
	Child    gen.PID
	Options  gen.ProcessOptions
	Error    error
}

func (RemoteSpawn) Kind() string { return "remote_spawned" }
func (r RemoteSpawn) String() string {
	return fmt.Sprintf("RemoteSpawn(parent=%s node=%s name=%s register=%s child=%s err=%v)",
		r.Parent, r.Node, r.Name, r.Register, r.Child, r.Error)
}

// RemoteApplicationStart is an application started (or attempted) on a remote node by a
// process via RemoteNode.ApplicationStart* (egress). Node is the target node, Name the
// application, Mode the start variant (zero for the plain ApplicationStart).
type RemoteApplicationStart struct {
	From  gen.PID
	Node  gen.Atom
	Name  gen.Atom
	Mode  gen.ApplicationMode
	Error error
}

func (RemoteApplicationStart) Kind() string { return "remote_application_started" }
func (r RemoteApplicationStart) String() string {
	return fmt.Sprintf("RemoteApplicationStart(from=%s node=%s name=%s mode=%v err=%v)",
		r.From, r.Node, r.Name, r.Mode, r.Error)
}

// SpawnMeta is a meta process spawned (or attempted) by a process (egress). On
// failure Alias is the zero alias and Error is set.
type SpawnMeta struct {
	Parent gen.PID
	Alias  gen.Alias
	Error  error
}

func (SpawnMeta) Kind() string { return "meta_spawned" }
func (r SpawnMeta) String() string {
	return fmt.Sprintf("SpawnMeta(parent=%s alias=%s err=%v)", r.Parent, r.Alias, r.Error)
}

// CreateAlias is an alias created (or attempted) by a process via CreateAlias
// (egress). On failure Alias is the zero alias and Error is set.
type CreateAlias struct {
	PID   gen.PID
	Alias gen.Alias
	Error error
}

func (CreateAlias) Kind() string { return "alias_created" }
func (r CreateAlias) String() string {
	return fmt.Sprintf("CreateAlias(pid=%s alias=%s err=%v)", r.PID, r.Alias, r.Error)
}

// DeleteAlias is an alias removed (or attempted) by a process via DeleteAlias
// (egress).
type DeleteAlias struct {
	PID   gen.PID
	Alias gen.Alias
	Error error
}

func (DeleteAlias) Kind() string { return "alias_deleted" }
func (r DeleteAlias) String() string {
	return fmt.Sprintf("DeleteAlias(pid=%s alias=%s err=%v)", r.PID, r.Alias, r.Error)
}

// RegisterEvent is an event producer registered (or attempted) by a process via
// RegisterEvent (egress). Ref is the producer token returned on success.
type RegisterEvent struct {
	PID   gen.PID
	Name  gen.Atom
	Ref   gen.Ref
	Error error
}

func (RegisterEvent) Kind() string { return "event_registered" }
func (r RegisterEvent) String() string {
	return fmt.Sprintf("RegisterEvent(pid=%s name=%s err=%v)", r.PID, r.Name, r.Error)
}

// UnregisterEvent is an event producer removed (or attempted) by a process via
// UnregisterEvent (egress).
type UnregisterEvent struct {
	PID   gen.PID
	Name  gen.Atom
	Error error
}

func (UnregisterEvent) Kind() string { return "event_unregistered" }
func (r UnregisterEvent) String() string {
	return fmt.Sprintf("UnregisterEvent(pid=%s name=%s err=%v)", r.PID, r.Name, r.Error)
}

// Forward is a message handed (or attempted) to another process via Forward,
// observed at the forwarder (egress). Used by act.Pool (round-robin) and
// act.Router (by-name routing). By is the forwarder, To the target, From the
// original sender; Error is the outcome of the forward.
type Forward struct {
	By      gen.PID
	To      gen.PID
	From    gen.PID
	Message any
	Error   error
}

func (Forward) Kind() string { return "forwarded" }
func (r Forward) String() string {
	return fmt.Sprintf("Forward(by=%s to=%s from=%s msg=%#v err=%v)", r.By, r.To, r.From, r.Message, r.Error)
}

// Delivered is a message delivered into a local mailbox on this node (ingress).
// Down/Exit/Event signals have their own records (Down/Exit/Event), not Delivered.
// Producer notifications (gen.MessageEventStart / MessageEventStop) do surface here
// as Delivered, since they reach the producer as ordinary mailbox messages.
type Delivered struct {
	From    gen.PID
	To      any
	Message any
}

func (Delivered) Kind() string { return "delivered" }
func (r Delivered) String() string {
	return fmt.Sprintf("Delivered(from=%s to=%v msg=%#v)", r.From, r.To, r.Message)
}

// Down is a down notification delivered to a monitoring process (ingress).
// Message is one of gen.MessageDownPID / MessageDownProcessID / etc.
type Down struct {
	To      gen.PID
	Message any
}

func (Down) Kind() string     { return "down" }
func (r Down) String() string { return fmt.Sprintf("Down(to=%s msg=%#v)", r.To, r.Message) }

// Exit is an exit signal delivered to a linked process (ingress).
// Message is one of gen.MessageExitPID / MessageExitProcessID / etc.
type Exit struct {
	To      gen.PID
	Message any
}

func (Exit) Kind() string     { return "exit" }
func (r Exit) String() string { return fmt.Sprintf("Exit(to=%s msg=%#v)", r.To, r.Message) }

// Event is a pub/sub event delivered to a subscriber (ingress).
type Event struct {
	To        gen.PID
	Event     gen.Event
	Timestamp int64
	Message   any
}

func (Event) Kind() string { return "event" }
func (r Event) String() string {
	return fmt.Sprintf("Event(to=%s %s msg=%#v)", r.To, r.Event, r.Message)
}

// Monitor is a monitor set up (or attempted) by a process (egress).
type Monitor struct {
	From   gen.PID
	Target any
	Error  error
}

func (Monitor) Kind() string { return "monitored" }
func (r Monitor) String() string {
	return fmt.Sprintf("Monitor(from=%s target=%v err=%v)", r.From, r.Target, r.Error)
}

// Demonitor is a monitor removed (or attempted) by a process (egress).
type Demonitor struct {
	From   gen.PID
	Target any
	Error  error
}

func (Demonitor) Kind() string { return "demonitored" }
func (r Demonitor) String() string {
	return fmt.Sprintf("Demonitor(from=%s target=%v err=%v)", r.From, r.Target, r.Error)
}

// Link is a link set up (or attempted) by a process (egress).
type Link struct {
	From   gen.PID
	Target any
	Error  error
}

func (Link) Kind() string { return "linked" }
func (r Link) String() string {
	return fmt.Sprintf("Link(from=%s target=%v err=%v)", r.From, r.Target, r.Error)
}

// Unlink is a link removed (or attempted) by a process (egress).
type Unlink struct {
	From   gen.PID
	Target any
	Error  error
}

func (Unlink) Kind() string { return "unlinked" }
func (r Unlink) String() string {
	return fmt.Sprintf("Unlink(from=%s target=%v err=%v)", r.From, r.Target, r.Error)
}

// WireLink is a remote consumer's link arriving over the connection (ingress on
// the target node). The sender deduplicates, so one per remote node.
type WireLink struct {
	From   gen.PID
	Target any
}

func (WireLink) Kind() string { return "wire_link" }
func (r WireLink) String() string {
	return fmt.Sprintf("WireLink(from=%s target=%v)", r.From, r.Target)
}

// WireUnlink is a remote consumer's unlink arriving over the connection (sent only
// when its last local subscriber leaves).
type WireUnlink struct {
	From   gen.PID
	Target any
}

func (WireUnlink) Kind() string { return "wire_unlink" }
func (r WireUnlink) String() string {
	return fmt.Sprintf("WireUnlink(from=%s target=%v)", r.From, r.Target)
}

// WireMonitor is a remote consumer's monitor arriving over the connection.
type WireMonitor struct {
	From   gen.PID
	Target any
}

func (WireMonitor) Kind() string { return "wire_monitor" }
func (r WireMonitor) String() string {
	return fmt.Sprintf("WireMonitor(from=%s target=%v)", r.From, r.Target)
}

// WireDemonitor is a remote consumer's demonitor arriving over the connection.
type WireDemonitor struct {
	From   gen.PID
	Target any
}

func (WireDemonitor) Kind() string { return "wire_demonitor" }
func (r WireDemonitor) String() string {
	return fmt.Sprintf("WireDemonitor(from=%s target=%v)", r.From, r.Target)
}

// SendEvent is an event published by a process (egress). Error is the outcome of
// the publish (nil on success).
type SendEvent struct {
	From    gen.PID
	Name    gen.Atom
	Token   gen.Ref // the producer registration token returned by RegisterEvent
	Message any
	Options gen.MessageOptions
	Error   error
}

func (SendEvent) Kind() string { return "sent_event" }
func (r SendEvent) String() string {
	return fmt.Sprintf("SendEvent(from=%s name=%s token=%s msg=%#v err=%v)", r.From, r.Name, r.Token, r.Message, r.Error)
}

// SendResponse is a response a process sent back to a caller's request (egress).
// Error is the outcome of the send call (nil on success); for an error response
// (SendResponseError) the responded error is carried in Message.
type SendResponse struct {
	From    gen.PID
	To      gen.PID
	Ref     gen.Ref
	Message any
	Options gen.MessageOptions
	Error   error
}

func (SendResponse) Kind() string { return "sent_response" }
func (r SendResponse) String() string {
	return fmt.Sprintf("SendResponse(from=%s to=%s msg=%#v err=%v)", r.From, r.To, r.Message, r.Error)
}

// SendExit is an exit signal a process sent to a PID via SendExit (egress). Reason
// is the exit reason delivered; Error is whether the send itself failed.
type SendExit struct {
	From   gen.PID
	To     gen.PID
	Reason error
	Error  error
}

func (SendExit) Kind() string { return "sent_exit" }
func (r SendExit) String() string {
	return fmt.Sprintf("SendExit(from=%s to=%s reason=%v err=%v)", r.From, r.To, r.Reason, r.Error)
}

// SendExitMeta is an exit signal a process sent to a meta process by alias via
// SendExitMeta (egress). Reason is the exit reason delivered; Error is whether the
// send itself failed.
type SendExitMeta struct {
	From   gen.PID
	Meta   gen.Alias
	Reason error
	Error  error
}

func (SendExitMeta) Kind() string { return "sent_exit_meta" }
func (r SendExitMeta) String() string {
	return fmt.Sprintf("SendExitMeta(from=%s meta=%s reason=%v err=%v)", r.From, r.Meta, r.Reason, r.Error)
}

// Span is a business span a process opened with StartTracingSpan and then closed
// (egress). Name is the span name; Error is set when closed with EndError.
// TraceID/SpanID/ParentSpanID are populated by harnesses that observe the emitted
// trace (stage); the unit harness records the span on close and leaves them zero.
type Span struct {
	From         gen.PID
	Name         string
	TraceID      [2]uint64
	SpanID       uint64
	ParentSpanID uint64
	Timestamp    int64 // span open time (live harness); 0 in unit
	EndTimestamp int64 // span close time (live harness); 0 in unit
	Attributes   []gen.TracingAttribute
	Error        string
}

func (Span) Kind() string { return "span" }
func (r Span) String() string {
	return fmt.Sprintf("Span(from=%s name=%s span=%d parent=%d err=%q)", r.From, r.Name, r.SpanID, r.ParentSpanID, r.Error)
}

// Log is a log line emitted by a process (egress). Message is preformatted.
type Log struct {
	From    gen.PID
	Level   gen.LogLevel
	Message string
}

func (Log) Kind() string { return "logged" }
func (r Log) String() string {
	return fmt.Sprintf("Log(from=%s level=%v msg=%q)", r.From, r.Level, r.Message)
}

// AddCronJob is a cron job registered (or attempted) by a process via Cron().AddJob
// (egress). Spec is the crontab schedule. On a duplicate name Error is gen.ErrTaken.
type AddCronJob struct {
	From  gen.PID
	Name  gen.Atom
	Spec  string
	Error error
}

func (AddCronJob) Kind() string { return "add_cron_job" }
func (r AddCronJob) String() string {
	return fmt.Sprintf("AddCronJob(from=%s name=%s spec=%q err=%v)", r.From, r.Name, r.Spec, r.Error)
}

// RemoveCronJob is a cron job removed (or attempted) by a process via Cron().RemoveJob
// (egress). On an unknown name Error is gen.ErrUnknown.
type RemoveCronJob struct {
	From  gen.PID
	Name  gen.Atom
	Error error
}

func (RemoveCronJob) Kind() string { return "remove_cron_job" }
func (r RemoveCronJob) String() string {
	return fmt.Sprintf("RemoveCronJob(from=%s name=%s err=%v)", r.From, r.Name, r.Error)
}

// Terminated is the subject actor's own termination, observed directly by the
// in-process harness (unit). reason is wrapped in *gen.Error when PreserveMailbox.
type Terminated struct {
	PID    gen.PID
	Reason error
}

func (Terminated) Kind() string { return "terminated" }
func (r Terminated) String() string {
	return fmt.Sprintf("Terminated(pid=%s reason=%v)", r.PID, r.Reason)
}

// SendAfter is a delayed send a process scheduled via SendAfter (egress). It is
// not delivered until the harness fires its timers.
type SendAfter struct {
	From    gen.PID
	To      any
	Message any
	After   time.Duration
	Options gen.MessageOptions
	Error   error
}

func (SendAfter) Kind() string { return "scheduled_send" }
func (r SendAfter) String() string {
	return fmt.Sprintf("SendAfter(from=%s to=%v after=%s msg=%#v err=%v)", r.From, r.To, r.After, r.Message, r.Error)
}

type SendEvery struct {
	From    gen.PID
	To      any
	Message any
	Period  time.Duration
	Options gen.MessageOptions
	Error   error
}

func (SendEvery) Kind() string { return "periodic_send" }
func (r SendEvery) String() string {
	return fmt.Sprintf("SendEvery(from=%s to=%v period=%s msg=%#v err=%v)", r.From, r.To, r.Period, r.Message, r.Error)
}
