package check

import (
	"fmt"
	"time"

	"ergo.services/ergo/gen"
)

// records

// Sent is an outgoing message observed at the sender (egress). Error is the
// outcome of the send call (nil on success). Options is the effective
// gen.MessageOptions the send was issued with (priority, compression, keep-order,
// important delivery) - what the routing core receives.
type Sent struct {
	From    gen.PID
	To      any
	Message any
	Options gen.MessageOptions
	Error   error
}

func (Sent) Kind() string { return "sent" }
func (r Sent) String() string {
	return fmt.Sprintf("Sent(from=%s to=%v msg=%#v err=%v)", r.From, r.To, r.Message, r.Error)
}

// Called is an outgoing request observed at the caller (egress). Error is the
// outcome of the call (nil on success); Response is the value returned.
type Called struct {
	From     gen.PID
	To       any
	Request  any
	Response any
	Error    error
}

func (Called) Kind() string { return "called" }
func (r Called) String() string {
	return fmt.Sprintf("Called(from=%s to=%v req=%#v err=%v)", r.From, r.To, r.Request, r.Error)
}

// Spawned is a child process created (or attempted) by a process (egress). On
// failure Child is the zero PID and Error is set. Factory is the spawned factory
// (set by harnesses that know it, e.g. the unit mock); zero otherwise. Register is
// the name for a SpawnRegister (empty for an anonymous Spawn). Options is the
// gen.ProcessOptions the spawn was requested with.
type Spawned struct {
	Parent   gen.PID
	Child    gen.PID
	Register gen.Atom
	Factory  gen.ProcessFactory
	Options  gen.ProcessOptions
	Error    error
}

func (Spawned) Kind() string { return "spawned" }
func (r Spawned) String() string {
	return fmt.Sprintf("Spawned(parent=%s child=%s register=%s err=%v)", r.Parent, r.Child, r.Register, r.Error)
}

// RemoteSpawned is a process spawned (or attempted) on a remote node by name
// (egress). Node is the target node, Name the remote factory name, Register the
// name to register the child under (empty for a plain RemoteSpawn). On failure
// Child is the zero PID and Error is set.
type RemoteSpawned struct {
	Parent   gen.PID
	Node     gen.Atom
	Name     gen.Atom
	Register gen.Atom
	Child    gen.PID
	Options  gen.ProcessOptions
	Error    error
}

func (RemoteSpawned) Kind() string { return "remote_spawned" }
func (r RemoteSpawned) String() string {
	return fmt.Sprintf("RemoteSpawned(parent=%s node=%s name=%s register=%s child=%s err=%v)",
		r.Parent, r.Node, r.Name, r.Register, r.Child, r.Error)
}

// MetaSpawned is a meta process spawned (or attempted) by a process (egress). On
// failure Alias is the zero alias and Error is set.
type MetaSpawned struct {
	Parent gen.PID
	Alias  gen.Alias
	Error  error
}

func (MetaSpawned) Kind() string { return "meta_spawned" }
func (r MetaSpawned) String() string {
	return fmt.Sprintf("MetaSpawned(parent=%s alias=%s err=%v)", r.Parent, r.Alias, r.Error)
}

// AliasCreated is an alias created (or attempted) by a process via CreateAlias
// (egress). On failure Alias is the zero alias and Error is set.
type AliasCreated struct {
	PID   gen.PID
	Alias gen.Alias
	Error error
}

func (AliasCreated) Kind() string { return "alias_created" }
func (r AliasCreated) String() string {
	return fmt.Sprintf("AliasCreated(pid=%s alias=%s err=%v)", r.PID, r.Alias, r.Error)
}

// AliasDeleted is an alias removed (or attempted) by a process via DeleteAlias
// (egress).
type AliasDeleted struct {
	PID   gen.PID
	Alias gen.Alias
	Error error
}

func (AliasDeleted) Kind() string { return "alias_deleted" }
func (r AliasDeleted) String() string {
	return fmt.Sprintf("AliasDeleted(pid=%s alias=%s err=%v)", r.PID, r.Alias, r.Error)
}

// EventRegistered is an event producer registered (or attempted) by a process via
// RegisterEvent (egress). Ref is the producer token returned on success.
type EventRegistered struct {
	PID   gen.PID
	Name  gen.Atom
	Ref   gen.Ref
	Error error
}

func (EventRegistered) Kind() string { return "event_registered" }
func (r EventRegistered) String() string {
	return fmt.Sprintf("EventRegistered(pid=%s name=%s err=%v)", r.PID, r.Name, r.Error)
}

// EventUnregistered is an event producer removed (or attempted) by a process via
// UnregisterEvent (egress).
type EventUnregistered struct {
	PID   gen.PID
	Name  gen.Atom
	Error error
}

func (EventUnregistered) Kind() string { return "event_unregistered" }
func (r EventUnregistered) String() string {
	return fmt.Sprintf("EventUnregistered(pid=%s name=%s err=%v)", r.PID, r.Name, r.Error)
}

// Forwarded is a message handed (or attempted) to another process via Forward,
// observed at the forwarder (egress). Used by act.Pool (round-robin) and
// act.Router (by-name routing). By is the forwarder, To the target, From the
// original sender; Error is the outcome of the forward.
type Forwarded struct {
	By      gen.PID
	To      gen.PID
	From    gen.PID
	Message any
	Error   error
}

func (Forwarded) Kind() string { return "forwarded" }
func (r Forwarded) String() string {
	return fmt.Sprintf("Forwarded(by=%s to=%s from=%s msg=%#v err=%v)", r.By, r.To, r.From, r.Message, r.Error)
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

// Monitored is a monitor set up (or attempted) by a process (egress).
type Monitored struct {
	From   gen.PID
	Target any
	Error  error
}

func (Monitored) Kind() string { return "monitored" }
func (r Monitored) String() string {
	return fmt.Sprintf("Monitored(from=%s target=%v err=%v)", r.From, r.Target, r.Error)
}

// Demonitored is a monitor removed (or attempted) by a process (egress).
type Demonitored struct {
	From   gen.PID
	Target any
	Error  error
}

func (Demonitored) Kind() string { return "demonitored" }
func (r Demonitored) String() string {
	return fmt.Sprintf("Demonitored(from=%s target=%v err=%v)", r.From, r.Target, r.Error)
}

// Linked is a link set up (or attempted) by a process (egress).
type Linked struct {
	From   gen.PID
	Target any
	Error  error
}

func (Linked) Kind() string { return "linked" }
func (r Linked) String() string {
	return fmt.Sprintf("Linked(from=%s target=%v err=%v)", r.From, r.Target, r.Error)
}

// Unlinked is a link removed (or attempted) by a process (egress).
type Unlinked struct {
	From   gen.PID
	Target any
	Error  error
}

func (Unlinked) Kind() string { return "unlinked" }
func (r Unlinked) String() string {
	return fmt.Sprintf("Unlinked(from=%s target=%v err=%v)", r.From, r.Target, r.Error)
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

// SentEvent is an event published by a process (egress). Error is the outcome of
// the publish (nil on success).
type SentEvent struct {
	From    gen.PID
	Name    gen.Atom
	Message any
	Options gen.MessageOptions
	Error   error
}

func (SentEvent) Kind() string { return "sent_event" }
func (r SentEvent) String() string {
	return fmt.Sprintf("SentEvent(from=%s name=%s msg=%#v err=%v)", r.From, r.Name, r.Message, r.Error)
}

// SentResponse is a response a process sent back to a caller's request (egress).
// Error is the outcome of the send call (nil on success); for an error response
// (SendResponseError) the responded error is carried in Message.
type SentResponse struct {
	From    gen.PID
	To      gen.PID
	Ref     gen.Ref
	Message any
	Options gen.MessageOptions
	Error   error
}

func (SentResponse) Kind() string { return "sent_response" }
func (r SentResponse) String() string {
	return fmt.Sprintf("SentResponse(from=%s to=%s msg=%#v err=%v)", r.From, r.To, r.Message, r.Error)
}


// SentExit is an exit signal a process sent via SendExit (egress).
type SentExit struct {
	From   gen.PID
	To     gen.PID
	Reason error
}

func (SentExit) Kind() string { return "sent_exit" }
func (r SentExit) String() string {
	return fmt.Sprintf("SentExit(from=%s to=%s reason=%v)", r.From, r.To, r.Reason)
}

// Logged is a log line emitted by a process (egress). Message is preformatted.
type Logged struct {
	From    gen.PID
	Level   gen.LogLevel
	Message string
}

func (Logged) Kind() string { return "logged" }
func (r Logged) String() string {
	return fmt.Sprintf("Logged(from=%s level=%v msg=%q)", r.From, r.Level, r.Message)
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

// ScheduledSend is a delayed send a process scheduled via SendAfter (egress). It is
// not delivered until the harness fires its timers.
type ScheduledSend struct {
	From    gen.PID
	To      any
	Message any
	After   time.Duration
	Options gen.MessageOptions
}

func (ScheduledSend) Kind() string { return "scheduled_send" }
func (r ScheduledSend) String() string {
	return fmt.Sprintf("ScheduledSend(from=%s to=%v after=%s msg=%#v)", r.From, r.To, r.After, r.Message)
}
