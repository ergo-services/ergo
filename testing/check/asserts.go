package check

import (
	"errors"
	"reflect"
	"strings"

	"ergo.services/ergo/gen"
)

// Asserter exposes the fluent Should* assertions over a Recorder. Harnesses
// (stage.Node, unit.Subject) embed *Asserter to gain the whole grammar.
type Asserter struct {
	t   T
	rec *Recorder
}

// NewAsserter binds an asserter to a recorder.
func NewAsserter(t T, rec *Recorder) *Asserter { return &Asserter{t: t, rec: rec} }

// Mark returns the current recorder position (for Since scoping).
func (a *Asserter) Mark() int { return a.rec.Mark() }

// Records returns the full ordered history observed so far. Intended for
// introspection (debugging, exploratory tests); assertions should use the Should*
// grammar.
func (a *Asserter) Records() []Record { return a.rec.Records() }

// fluent assertions (thin wrappers over For)

// SentAssert asserts over outgoing messages observed on a node.
type SentAssert struct{ *Assertion[Sent] }

// ShouldSend starts an egress message assertion on this node.
func (a *Asserter) ShouldSend() *SentAssert { return &SentAssert{For[Sent](a.t, a.rec)} }
func (a *SentAssert) From(p gen.PID) *SentAssert {
	a.Where(func(r Sent) bool { return r.From == p })
	return a
}
func (a *SentAssert) To(to any) *SentAssert {
	a.Where(func(r Sent) bool { return reflect.DeepEqual(r.To, to) })
	return a
}
func (a *SentAssert) Message(v any) *SentAssert {
	a.Where(func(r Sent) bool { return reflect.DeepEqual(r.Message, v) })
	return a
}
func (a *SentAssert) Error(target error) *SentAssert {
	a.Where(func(r Sent) bool { return r.Error == target })
	return a
}

// Priority narrows to messages sent with the given priority (the effective priority,
// from the sender's default or an explicit SendWithPriority).
func (a *SentAssert) Priority(p gen.MessagePriority) *SentAssert {
	a.Where(func(r Sent) bool { return r.Options.Priority == p })
	return a
}

// Important narrows to messages sent with the given important-delivery flag.
func (a *SentAssert) Important(important bool) *SentAssert {
	a.Where(func(r Sent) bool { return r.Options.ImportantDelivery == important })
	return a
}

// KeepNetworkOrder narrows to messages sent with the given keep-network-order flag.
func (a *SentAssert) KeepNetworkOrder(keep bool) *SentAssert {
	a.Where(func(r Sent) bool { return r.Options.KeepNetworkOrder == keep })
	return a
}

// SpawnAssert asserts over child processes spawned on a node (egress).
type SpawnAssert struct{ *Assertion[Spawned] }

// ShouldSpawn starts a spawn assertion on this node.
func (a *Asserter) ShouldSpawn() *SpawnAssert { return &SpawnAssert{For[Spawned](a.t, a.rec)} }
func (a *SpawnAssert) From(parent gen.PID) *SpawnAssert {
	a.Where(func(r Spawned) bool { return r.Parent == parent })
	return a
}
func (a *SpawnAssert) Child(pid gen.PID) *SpawnAssert {
	a.Where(func(r Spawned) bool { return r.Child == pid })
	return a
}
func (a *SpawnAssert) Register(name gen.Atom) *SpawnAssert {
	a.Where(func(r Spawned) bool { return r.Register == name })
	return a
}
func (a *SpawnAssert) Factory(f gen.ProcessFactory) *SpawnAssert {
	want := reflect.ValueOf(f).Pointer()
	a.Where(func(r Spawned) bool {
		return r.Factory != nil && reflect.ValueOf(r.Factory).Pointer() == want
	})
	return a
}
func (a *SpawnAssert) Error(target error) *SpawnAssert {
	a.Where(func(r Spawned) bool { return r.Error == target })
	return a
}

// RemoteSpawnAssert asserts over remote spawns (egress).
type RemoteSpawnAssert struct{ *Assertion[RemoteSpawned] }

// ShouldRemoteSpawn starts a remote-spawn assertion on this node.
func (a *Asserter) ShouldRemoteSpawn() *RemoteSpawnAssert {
	return &RemoteSpawnAssert{For[RemoteSpawned](a.t, a.rec)}
}
func (a *RemoteSpawnAssert) From(parent gen.PID) *RemoteSpawnAssert {
	a.Where(func(r RemoteSpawned) bool { return r.Parent == parent })
	return a
}
func (a *RemoteSpawnAssert) To(node gen.Atom) *RemoteSpawnAssert {
	a.Where(func(r RemoteSpawned) bool { return r.Node == node })
	return a
}
func (a *RemoteSpawnAssert) Name(name gen.Atom) *RemoteSpawnAssert {
	a.Where(func(r RemoteSpawned) bool { return r.Name == name })
	return a
}
func (a *RemoteSpawnAssert) Register(name gen.Atom) *RemoteSpawnAssert {
	a.Where(func(r RemoteSpawned) bool { return r.Register == name })
	return a
}
func (a *RemoteSpawnAssert) Child(pid gen.PID) *RemoteSpawnAssert {
	a.Where(func(r RemoteSpawned) bool { return r.Child == pid })
	return a
}
func (a *RemoteSpawnAssert) Error(target error) *RemoteSpawnAssert {
	a.Where(func(r RemoteSpawned) bool { return r.Error == target })
	return a
}
func (a *RemoteSpawnAssert) ErrorIs(target error) *RemoteSpawnAssert {
	a.Where(func(r RemoteSpawned) bool { return errors.Is(r.Error, target) })
	return a
}

// MetaSpawnAssert asserts over meta-process spawns (egress).
type MetaSpawnAssert struct{ *Assertion[MetaSpawned] }

// ShouldSpawnMeta starts a meta-spawn assertion on this node.
func (a *Asserter) ShouldSpawnMeta() *MetaSpawnAssert {
	return &MetaSpawnAssert{For[MetaSpawned](a.t, a.rec)}
}
func (a *MetaSpawnAssert) From(parent gen.PID) *MetaSpawnAssert {
	a.Where(func(r MetaSpawned) bool { return r.Parent == parent })
	return a
}
func (a *MetaSpawnAssert) Alias(alias gen.Alias) *MetaSpawnAssert {
	a.Where(func(r MetaSpawned) bool { return r.Alias == alias })
	return a
}
func (a *MetaSpawnAssert) Error(target error) *MetaSpawnAssert {
	a.Where(func(r MetaSpawned) bool { return r.Error == target })
	return a
}
func (a *MetaSpawnAssert) ErrorIs(target error) *MetaSpawnAssert {
	a.Where(func(r MetaSpawned) bool { return errors.Is(r.Error, target) })
	return a
}

// AliasCreatedAssert asserts over CreateAlias (egress).
type AliasCreatedAssert struct{ *Assertion[AliasCreated] }

// ShouldCreateAlias starts a create-alias assertion on this node.
func (a *Asserter) ShouldCreateAlias() *AliasCreatedAssert {
	return &AliasCreatedAssert{For[AliasCreated](a.t, a.rec)}
}
func (a *AliasCreatedAssert) From(pid gen.PID) *AliasCreatedAssert {
	a.Where(func(r AliasCreated) bool { return r.PID == pid })
	return a
}
func (a *AliasCreatedAssert) Alias(alias gen.Alias) *AliasCreatedAssert {
	a.Where(func(r AliasCreated) bool { return r.Alias == alias })
	return a
}
func (a *AliasCreatedAssert) Error(target error) *AliasCreatedAssert {
	a.Where(func(r AliasCreated) bool { return r.Error == target })
	return a
}
func (a *AliasCreatedAssert) ErrorIs(target error) *AliasCreatedAssert {
	a.Where(func(r AliasCreated) bool { return errors.Is(r.Error, target) })
	return a
}

// AliasDeletedAssert asserts over DeleteAlias (egress).
type AliasDeletedAssert struct{ *Assertion[AliasDeleted] }

// ShouldDeleteAlias starts a delete-alias assertion on this node.
func (a *Asserter) ShouldDeleteAlias() *AliasDeletedAssert {
	return &AliasDeletedAssert{For[AliasDeleted](a.t, a.rec)}
}
func (a *AliasDeletedAssert) From(pid gen.PID) *AliasDeletedAssert {
	a.Where(func(r AliasDeleted) bool { return r.PID == pid })
	return a
}
func (a *AliasDeletedAssert) Alias(alias gen.Alias) *AliasDeletedAssert {
	a.Where(func(r AliasDeleted) bool { return r.Alias == alias })
	return a
}
func (a *AliasDeletedAssert) Error(target error) *AliasDeletedAssert {
	a.Where(func(r AliasDeleted) bool { return r.Error == target })
	return a
}
func (a *AliasDeletedAssert) ErrorIs(target error) *AliasDeletedAssert {
	a.Where(func(r AliasDeleted) bool { return errors.Is(r.Error, target) })
	return a
}

// EventRegisteredAssert asserts over RegisterEvent (egress).
type EventRegisteredAssert struct{ *Assertion[EventRegistered] }

// ShouldRegisterEvent starts a register-event assertion on this node.
func (a *Asserter) ShouldRegisterEvent() *EventRegisteredAssert {
	return &EventRegisteredAssert{For[EventRegistered](a.t, a.rec)}
}
func (a *EventRegisteredAssert) From(pid gen.PID) *EventRegisteredAssert {
	a.Where(func(r EventRegistered) bool { return r.PID == pid })
	return a
}
func (a *EventRegisteredAssert) Name(name gen.Atom) *EventRegisteredAssert {
	a.Where(func(r EventRegistered) bool { return r.Name == name })
	return a
}
func (a *EventRegisteredAssert) Error(target error) *EventRegisteredAssert {
	a.Where(func(r EventRegistered) bool { return r.Error == target })
	return a
}
func (a *EventRegisteredAssert) ErrorIs(target error) *EventRegisteredAssert {
	a.Where(func(r EventRegistered) bool { return errors.Is(r.Error, target) })
	return a
}

// EventUnregisteredAssert asserts over UnregisterEvent (egress).
type EventUnregisteredAssert struct{ *Assertion[EventUnregistered] }

// ShouldUnregisterEvent starts an unregister-event assertion on this node.
func (a *Asserter) ShouldUnregisterEvent() *EventUnregisteredAssert {
	return &EventUnregisteredAssert{For[EventUnregistered](a.t, a.rec)}
}
func (a *EventUnregisteredAssert) From(pid gen.PID) *EventUnregisteredAssert {
	a.Where(func(r EventUnregistered) bool { return r.PID == pid })
	return a
}
func (a *EventUnregisteredAssert) Name(name gen.Atom) *EventUnregisteredAssert {
	a.Where(func(r EventUnregistered) bool { return r.Name == name })
	return a
}
func (a *EventUnregisteredAssert) Error(target error) *EventUnregisteredAssert {
	a.Where(func(r EventUnregistered) bool { return r.Error == target })
	return a
}
func (a *EventUnregisteredAssert) ErrorIs(target error) *EventUnregisteredAssert {
	a.Where(func(r EventUnregistered) bool { return errors.Is(r.Error, target) })
	return a
}

// ForwardAssert asserts over messages forwarded by a process (egress).
type ForwardAssert struct{ *Assertion[Forwarded] }

// ShouldForward starts a forward assertion on this node.
func (a *Asserter) ShouldForward() *ForwardAssert {
	return &ForwardAssert{For[Forwarded](a.t, a.rec)}
}
func (a *ForwardAssert) By(pid gen.PID) *ForwardAssert {
	a.Where(func(r Forwarded) bool { return r.By == pid })
	return a
}
func (a *ForwardAssert) To(pid gen.PID) *ForwardAssert {
	a.Where(func(r Forwarded) bool { return r.To == pid })
	return a
}
func (a *ForwardAssert) Message(v any) *ForwardAssert {
	a.Where(func(r Forwarded) bool { return reflect.DeepEqual(r.Message, v) })
	return a
}
func (a *ForwardAssert) Error(target error) *ForwardAssert {
	a.Where(func(r Forwarded) bool { return r.Error == target })
	return a
}

// DeliveredAssert asserts over messages delivered into local mailboxes (ingress).
type DeliveredAssert struct{ *Assertion[Delivered] }

// ShouldDeliver starts an ingress delivery assertion on this node.
func (a *Asserter) ShouldDeliver() *DeliveredAssert {
	return &DeliveredAssert{For[Delivered](a.t, a.rec)}
}
func (a *DeliveredAssert) From(p gen.PID) *DeliveredAssert {
	a.Where(func(r Delivered) bool { return r.From == p })
	return a
}
func (a *DeliveredAssert) To(pid gen.PID) *DeliveredAssert {
	a.Where(func(r Delivered) bool { t, ok := r.To.(gen.PID); return ok && t == pid })
	return a
}
func (a *DeliveredAssert) ToProcessID(target gen.ProcessID) *DeliveredAssert {
	a.Where(func(r Delivered) bool { t, ok := r.To.(gen.ProcessID); return ok && t == target })
	return a
}
func (a *DeliveredAssert) ToAlias(target gen.Alias) *DeliveredAssert {
	a.Where(func(r Delivered) bool { t, ok := r.To.(gen.Alias); return ok && t == target })
	return a
}
func (a *DeliveredAssert) Message(v any) *DeliveredAssert {
	a.Where(func(r Delivered) bool { return reflect.DeepEqual(r.Message, v) })
	return a
}

// CalledAssert asserts over outgoing requests observed on a node.
type CalledAssert struct{ *Assertion[Called] }

// ShouldCall starts an egress call assertion on this node.
func (a *Asserter) ShouldCall() *CalledAssert { return &CalledAssert{For[Called](a.t, a.rec)} }
func (a *CalledAssert) From(p gen.PID) *CalledAssert {
	a.Where(func(r Called) bool { return r.From == p })
	return a
}
func (a *CalledAssert) To(to any) *CalledAssert {
	a.Where(func(r Called) bool { return reflect.DeepEqual(r.To, to) })
	return a
}
func (a *CalledAssert) Request(v any) *CalledAssert {
	a.Where(func(r Called) bool { return reflect.DeepEqual(r.Request, v) })
	return a
}
func (a *CalledAssert) Error(target error) *CalledAssert {
	a.Where(func(r Called) bool { return r.Error == target })
	return a
}

// downReason extracts the reason from any gen.MessageDown* value.
func downReason(m any) (error, bool) {
	switch d := m.(type) {
	case gen.MessageDownPID:
		return d.Reason, true
	case gen.MessageDownProcessID:
		return d.Reason, true
	case gen.MessageDownAlias:
		return d.Reason, true
	case gen.MessageDownEvent:
		return d.Reason, true
	}
	return nil, false
}

// exitReason extracts the reason from any gen.MessageExit* value (no reason for node).
func exitReason(m any) (error, bool) {
	switch e := m.(type) {
	case gen.MessageExitPID:
		return e.Reason, true
	case gen.MessageExitProcessID:
		return e.Reason, true
	case gen.MessageExitAlias:
		return e.Reason, true
	case gen.MessageExitEvent:
		return e.Reason, true
	}
	return nil, false
}

// DownAssert asserts over down notifications received on a node (ingress).
type DownAssert struct{ *Assertion[Down] }

// ShouldReceiveDown starts a down-reception assertion on this node.
func (a *Asserter) ShouldReceiveDown() *DownAssert { return &DownAssert{For[Down](a.t, a.rec)} }
func (a *DownAssert) To(consumer gen.PID) *DownAssert {
	a.Where(func(r Down) bool { return r.To == consumer })
	return a
}
func (a *DownAssert) About(target gen.PID) *DownAssert {
	a.Where(func(r Down) bool { m, ok := r.Message.(gen.MessageDownPID); return ok && m.PID == target })
	return a
}
func (a *DownAssert) AboutProcessID(target gen.ProcessID) *DownAssert {
	a.Where(func(r Down) bool { m, ok := r.Message.(gen.MessageDownProcessID); return ok && m.ProcessID == target })
	return a
}
func (a *DownAssert) AboutAlias(target gen.Alias) *DownAssert {
	a.Where(func(r Down) bool { m, ok := r.Message.(gen.MessageDownAlias); return ok && m.Alias == target })
	return a
}
func (a *DownAssert) AboutEvent(target gen.Event) *DownAssert {
	a.Where(func(r Down) bool { m, ok := r.Message.(gen.MessageDownEvent); return ok && m.Event == target })
	return a
}
func (a *DownAssert) Reason(target error) *DownAssert {
	a.Where(func(r Down) bool { reason, ok := downReason(r.Message); return ok && reason == target })
	return a
}

// ReasonIs matches when the down reason wraps target (errors.Is). Use it for a
// cascade termination, where the reason is wrapped (e.g. a non-trapping linked
// process terminating from a partner's exit).
func (a *DownAssert) ReasonIs(target error) *DownAssert {
	a.Where(func(r Down) bool { reason, ok := downReason(r.Message); return ok && errors.Is(reason, target) })
	return a
}

// ExitAssert asserts over exit signals received on a node (ingress).
type ExitAssert struct{ *Assertion[Exit] }

// ShouldReceiveExit starts an exit-reception assertion on this node.
func (a *Asserter) ShouldReceiveExit() *ExitAssert { return &ExitAssert{For[Exit](a.t, a.rec)} }
func (a *ExitAssert) To(consumer gen.PID) *ExitAssert {
	a.Where(func(r Exit) bool { return r.To == consumer })
	return a
}
func (a *ExitAssert) About(target gen.PID) *ExitAssert {
	a.Where(func(r Exit) bool { m, ok := r.Message.(gen.MessageExitPID); return ok && m.PID == target })
	return a
}
func (a *ExitAssert) AboutProcessID(target gen.ProcessID) *ExitAssert {
	a.Where(func(r Exit) bool { m, ok := r.Message.(gen.MessageExitProcessID); return ok && m.ProcessID == target })
	return a
}
func (a *ExitAssert) AboutAlias(target gen.Alias) *ExitAssert {
	a.Where(func(r Exit) bool { m, ok := r.Message.(gen.MessageExitAlias); return ok && m.Alias == target })
	return a
}
func (a *ExitAssert) AboutEvent(target gen.Event) *ExitAssert {
	a.Where(func(r Exit) bool { m, ok := r.Message.(gen.MessageExitEvent); return ok && m.Event == target })
	return a
}
func (a *ExitAssert) Reason(target error) *ExitAssert {
	a.Where(func(r Exit) bool { reason, ok := exitReason(r.Message); return ok && reason == target })
	return a
}

// ReasonIs matches when the exit reason wraps target (errors.Is). Use it when the
// reason is wrapped rather than exact, e.g. a process spawned with PreserveMailbox
// terminating abnormally (the reason is captured into a *gen.Error).
func (a *ExitAssert) ReasonIs(target error) *ExitAssert {
	a.Where(func(r Exit) bool { reason, ok := exitReason(r.Message); return ok && errors.Is(reason, target) })
	return a
}

// EventAssert asserts over pub/sub events received on a node (ingress).
type EventAssert struct{ *Assertion[Event] }

// ShouldReceiveEvent starts an event-reception assertion on this node.
func (a *Asserter) ShouldReceiveEvent() *EventAssert { return &EventAssert{For[Event](a.t, a.rec)} }
func (a *EventAssert) To(subscriber gen.PID) *EventAssert {
	a.Where(func(r Event) bool { return r.To == subscriber })
	return a
}
func (a *EventAssert) Event(ev gen.Event) *EventAssert {
	a.Where(func(r Event) bool { return r.Event == ev })
	return a
}
func (a *EventAssert) Message(v any) *EventAssert {
	a.Where(func(r Event) bool { return reflect.DeepEqual(r.Message, v) })
	return a
}

// MonitorAssert asserts over monitors set up on a node (egress).
type MonitorAssert struct{ *Assertion[Monitored] }

// ShouldMonitor starts a monitor-setup assertion on this node.
func (a *Asserter) ShouldMonitor() *MonitorAssert {
	return &MonitorAssert{For[Monitored](a.t, a.rec)}
}
func (a *MonitorAssert) From(p gen.PID) *MonitorAssert {
	a.Where(func(r Monitored) bool { return r.From == p })
	return a
}
func (a *MonitorAssert) Target(t any) *MonitorAssert {
	a.Where(func(r Monitored) bool { return reflect.DeepEqual(r.Target, t) })
	return a
}
func (a *MonitorAssert) Error(target error) *MonitorAssert {
	a.Where(func(r Monitored) bool { return r.Error == target })
	return a
}

// DemonitorAssert asserts over monitors removed on a node (egress).
type DemonitorAssert struct{ *Assertion[Demonitored] }

// ShouldDemonitor starts a demonitor assertion on this node.
func (a *Asserter) ShouldDemonitor() *DemonitorAssert {
	return &DemonitorAssert{For[Demonitored](a.t, a.rec)}
}
func (a *DemonitorAssert) From(p gen.PID) *DemonitorAssert {
	a.Where(func(r Demonitored) bool { return r.From == p })
	return a
}
func (a *DemonitorAssert) Target(t any) *DemonitorAssert {
	a.Where(func(r Demonitored) bool { return reflect.DeepEqual(r.Target, t) })
	return a
}
func (a *DemonitorAssert) Error(target error) *DemonitorAssert {
	a.Where(func(r Demonitored) bool { return r.Error == target })
	return a
}

// LinkAssert asserts over links set up on a node (egress).
type LinkAssert struct{ *Assertion[Linked] }

// ShouldLink starts a link-setup assertion on this node.
func (a *Asserter) ShouldLink() *LinkAssert { return &LinkAssert{For[Linked](a.t, a.rec)} }
func (a *LinkAssert) From(p gen.PID) *LinkAssert {
	a.Where(func(r Linked) bool { return r.From == p })
	return a
}
func (a *LinkAssert) Target(t any) *LinkAssert {
	a.Where(func(r Linked) bool { return reflect.DeepEqual(r.Target, t) })
	return a
}
func (a *LinkAssert) Error(target error) *LinkAssert {
	a.Where(func(r Linked) bool { return r.Error == target })
	return a
}

// UnlinkAssert asserts over links removed on a node (egress).
type UnlinkAssert struct{ *Assertion[Unlinked] }

// ShouldUnlink starts an unlink assertion on this node.
func (a *Asserter) ShouldUnlink() *UnlinkAssert { return &UnlinkAssert{For[Unlinked](a.t, a.rec)} }
func (a *UnlinkAssert) From(p gen.PID) *UnlinkAssert {
	a.Where(func(r Unlinked) bool { return r.From == p })
	return a
}
func (a *UnlinkAssert) Target(t any) *UnlinkAssert {
	a.Where(func(r Unlinked) bool { return reflect.DeepEqual(r.Target, t) })
	return a
}
func (a *UnlinkAssert) Error(target error) *UnlinkAssert {
	a.Where(func(r Unlinked) bool { return r.Error == target })
	return a
}

// WireLinkAssert asserts over remote links arriving over the wire (ingress).
type WireLinkAssert struct{ *Assertion[WireLink] }

// ShouldWireLink starts a wire-link ingress assertion on this node.
func (a *Asserter) ShouldWireLink() *WireLinkAssert {
	return &WireLinkAssert{For[WireLink](a.t, a.rec)}
}
func (a *WireLinkAssert) From(p gen.PID) *WireLinkAssert {
	a.Where(func(r WireLink) bool { return r.From == p })
	return a
}
func (a *WireLinkAssert) Target(t any) *WireLinkAssert {
	a.Where(func(r WireLink) bool { return reflect.DeepEqual(r.Target, t) })
	return a
}

// WireUnlinkAssert asserts over remote unlinks arriving over the wire (ingress).
type WireUnlinkAssert struct{ *Assertion[WireUnlink] }

// ShouldWireUnlink starts a wire-unlink ingress assertion on this node.
func (a *Asserter) ShouldWireUnlink() *WireUnlinkAssert {
	return &WireUnlinkAssert{For[WireUnlink](a.t, a.rec)}
}
func (a *WireUnlinkAssert) From(p gen.PID) *WireUnlinkAssert {
	a.Where(func(r WireUnlink) bool { return r.From == p })
	return a
}
func (a *WireUnlinkAssert) Target(t any) *WireUnlinkAssert {
	a.Where(func(r WireUnlink) bool { return reflect.DeepEqual(r.Target, t) })
	return a
}

// WireMonitorAssert asserts over remote monitors arriving over the wire (ingress).
type WireMonitorAssert struct{ *Assertion[WireMonitor] }

// ShouldWireMonitor starts a wire-monitor ingress assertion on this node.
func (a *Asserter) ShouldWireMonitor() *WireMonitorAssert {
	return &WireMonitorAssert{For[WireMonitor](a.t, a.rec)}
}
func (a *WireMonitorAssert) From(p gen.PID) *WireMonitorAssert {
	a.Where(func(r WireMonitor) bool { return r.From == p })
	return a
}
func (a *WireMonitorAssert) Target(t any) *WireMonitorAssert {
	a.Where(func(r WireMonitor) bool { return reflect.DeepEqual(r.Target, t) })
	return a
}

// WireDemonitorAssert asserts over remote demonitors arriving over the wire (ingress).
type WireDemonitorAssert struct {
	*Assertion[WireDemonitor]
}

// ShouldWireDemonitor starts a wire-demonitor ingress assertion on this node.
func (a *Asserter) ShouldWireDemonitor() *WireDemonitorAssert {
	return &WireDemonitorAssert{For[WireDemonitor](a.t, a.rec)}
}
func (a *WireDemonitorAssert) From(p gen.PID) *WireDemonitorAssert {
	a.Where(func(r WireDemonitor) bool { return r.From == p })
	return a
}
func (a *WireDemonitorAssert) Target(t any) *WireDemonitorAssert {
	a.Where(func(r WireDemonitor) bool { return reflect.DeepEqual(r.Target, t) })
	return a
}

// SendEventAssert asserts over events published on a node (egress).
type SendEventAssert struct{ *Assertion[SentEvent] }

// ShouldSendEvent starts an event-publish assertion on this node.
func (a *Asserter) ShouldSendEvent() *SendEventAssert {
	return &SendEventAssert{For[SentEvent](a.t, a.rec)}
}
func (a *SendEventAssert) From(p gen.PID) *SendEventAssert {
	a.Where(func(r SentEvent) bool { return r.From == p })
	return a
}
func (a *SendEventAssert) Name(name gen.Atom) *SendEventAssert {
	a.Where(func(r SentEvent) bool { return r.Name == name })
	return a
}
func (a *SendEventAssert) Error(target error) *SendEventAssert {
	a.Where(func(r SentEvent) bool { return r.Error == target })
	return a
}

// SendResponseAssert asserts over responses a process sent to requests (egress).
type SendResponseAssert struct{ *Assertion[SentResponse] }

// ShouldSendResponse starts a response assertion on this node.
func (a *Asserter) ShouldSendResponse() *SendResponseAssert {
	return &SendResponseAssert{For[SentResponse](a.t, a.rec)}
}
func (a *SendResponseAssert) From(p gen.PID) *SendResponseAssert {
	a.Where(func(r SentResponse) bool { return r.From == p })
	return a
}
func (a *SendResponseAssert) To(p gen.PID) *SendResponseAssert {
	a.Where(func(r SentResponse) bool { return r.To == p })
	return a
}
func (a *SendResponseAssert) Message(v any) *SendResponseAssert {
	a.Where(func(r SentResponse) bool { return reflect.DeepEqual(r.Message, v) })
	return a
}
func (a *SendResponseAssert) Error(target error) *SendResponseAssert {
	a.Where(func(r SentResponse) bool { return r.Error == target })
	return a
}

// SendExitAssert asserts over exit signals a process sent via SendExit (egress).
type SendExitAssert struct{ *Assertion[SentExit] }

func (a *Asserter) ShouldSendExit() *SendExitAssert { return &SendExitAssert{For[SentExit](a.t, a.rec)} }
func (x *SendExitAssert) From(p gen.PID) *SendExitAssert {
	x.Where(func(r SentExit) bool { return r.From == p })
	return x
}
func (x *SendExitAssert) To(p gen.PID) *SendExitAssert {
	x.Where(func(r SentExit) bool { return r.To == p })
	return x
}
func (x *SendExitAssert) Reason(target error) *SendExitAssert {
	x.Where(func(r SentExit) bool { return r.Reason == target })
	return x
}
func (x *SendExitAssert) ReasonIs(target error) *SendExitAssert {
	x.Where(func(r SentExit) bool { return errors.Is(r.Reason, target) })
	return x
}

// LogAssert asserts over log lines a process emitted (egress).
type LogAssert struct{ *Assertion[Logged] }

func (a *Asserter) ShouldLog() *LogAssert { return &LogAssert{For[Logged](a.t, a.rec)} }
func (x *LogAssert) Level(l gen.LogLevel) *LogAssert {
	x.Where(func(r Logged) bool { return r.Level == l })
	return x
}
func (x *LogAssert) Message(msg string) *LogAssert {
	x.Where(func(r Logged) bool { return r.Message == msg })
	return x
}
func (x *LogAssert) Containing(substr string) *LogAssert {
	x.Where(func(r Logged) bool { return strings.Contains(r.Message, substr) })
	return x
}

// TerminateAssert asserts over the subject actor's own termination (unit). With
// no terminate, None() confirms the actor survived.
type TerminateAssert struct{ *Assertion[Terminated] }

func (a *Asserter) ShouldTerminate() *TerminateAssert { return &TerminateAssert{For[Terminated](a.t, a.rec)} }
func (x *TerminateAssert) Reason(target error) *TerminateAssert {
	x.Where(func(r Terminated) bool { return r.Reason == target })
	return x
}
func (x *TerminateAssert) ReasonIs(target error) *TerminateAssert {
	x.Where(func(r Terminated) bool { return errors.Is(r.Reason, target) })
	return x
}

// Normally asserts the actor terminated for an orderly reason: gen.TerminateReasonNormal
// or gen.TerminateReasonShutdown (a supervisor-initiated stop). Use Reason/ReasonIs for an
// exact reason match.
func (x *TerminateAssert) Normally() *TerminateAssert {
	x.Where(func(r Terminated) bool {
		return errors.Is(r.Reason, gen.TerminateReasonNormal) ||
			errors.Is(r.Reason, gen.TerminateReasonShutdown)
	})
	return x
}

// Abnormally asserts the actor terminated for any reason other than the orderly
// gen.TerminateReasonNormal / gen.TerminateReasonShutdown (a crash, panic, kill, or
// custom error reason).
func (x *TerminateAssert) Abnormally() *TerminateAssert {
	x.Where(func(r Terminated) bool {
		return errors.Is(r.Reason, gen.TerminateReasonNormal) == false &&
			errors.Is(r.Reason, gen.TerminateReasonShutdown) == false
	})
	return x
}

// ScheduleAssert asserts over delayed sends scheduled via SendAfter (egress).
type ScheduleAssert struct{ *Assertion[ScheduledSend] }

func (a *Asserter) ShouldScheduleSend() *ScheduleAssert { return &ScheduleAssert{For[ScheduledSend](a.t, a.rec)} }
func (x *ScheduleAssert) From(p gen.PID) *ScheduleAssert {
	x.Where(func(r ScheduledSend) bool { return r.From == p })
	return x
}
func (x *ScheduleAssert) To(to any) *ScheduleAssert {
	x.Where(func(r ScheduledSend) bool { return reflect.DeepEqual(r.To, to) })
	return x
}
func (x *ScheduleAssert) Message(v any) *ScheduleAssert {
	x.Where(func(r ScheduledSend) bool { return reflect.DeepEqual(r.Message, v) })
	return x
}
