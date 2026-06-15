package check

import (
	"errors"
	"reflect"
	"strings"
	"time"

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

// SendAssert asserts over outgoing messages observed on a node.
type SendAssert struct{ *Assertion[Send] }

// ShouldSend starts an egress message assertion on this node.
func (a *Asserter) ShouldSend() *SendAssert { return &SendAssert{For[Send](a.t, a.rec)} }
func (a *SendAssert) From(p gen.PID) *SendAssert {
	a.Where(func(r Send) bool { return r.From == p })
	return a
}
func (a *SendAssert) To(to any) *SendAssert {
	a.Where(func(r Send) bool { return reflect.DeepEqual(r.To, to) })
	return a
}
func (a *SendAssert) Message(v any) *SendAssert {
	a.Where(func(r Send) bool { return reflect.DeepEqual(r.Message, v) })
	return a
}
func (a *SendAssert) Error(target error) *SendAssert {
	a.Where(func(r Send) bool { return r.Error == target })
	return a
}
func (a *SendAssert) ErrorIs(target error) *SendAssert {
	a.Where(func(r Send) bool { return errors.Is(r.Error, target) })
	return a
}

// Priority narrows to messages sent with the given priority (the effective priority,
// from the sender's default or an explicit SendWithPriority).
func (a *SendAssert) Priority(p gen.MessagePriority) *SendAssert {
	a.Where(func(r Send) bool { return r.Options.Priority == p })
	return a
}

// Important narrows to messages sent with the given important-delivery flag.
func (a *SendAssert) Important(important bool) *SendAssert {
	a.Where(func(r Send) bool { return r.Options.ImportantDelivery == important })
	return a
}

// KeepNetworkOrder narrows to messages sent with the given keep-network-order flag.
func (a *SendAssert) KeepNetworkOrder(keep bool) *SendAssert {
	a.Where(func(r Send) bool { return r.Options.KeepNetworkOrder == keep })
	return a
}

// SpawnAssert asserts over child processes spawned on a node (egress).
type SpawnAssert struct{ *Assertion[Spawn] }

// ShouldSpawn starts a spawn assertion on this node.
func (a *Asserter) ShouldSpawn() *SpawnAssert { return &SpawnAssert{For[Spawn](a.t, a.rec)} }
func (a *SpawnAssert) From(parent gen.PID) *SpawnAssert {
	a.Where(func(r Spawn) bool { return r.Parent == parent })
	return a
}
func (a *SpawnAssert) Child(pid gen.PID) *SpawnAssert {
	a.Where(func(r Spawn) bool { return r.Child == pid })
	return a
}
func (a *SpawnAssert) Register(name gen.Atom) *SpawnAssert {
	a.Where(func(r Spawn) bool { return r.Register == name })
	return a
}
func (a *SpawnAssert) Factory(f gen.ProcessFactory) *SpawnAssert {
	want := reflect.ValueOf(f).Pointer()
	a.Where(func(r Spawn) bool {
		return r.Factory != nil && reflect.ValueOf(r.Factory).Pointer() == want
	})
	return a
}
func (a *SpawnAssert) Error(target error) *SpawnAssert {
	a.Where(func(r Spawn) bool { return r.Error == target })
	return a
}
func (a *SpawnAssert) ErrorIs(target error) *SpawnAssert {
	a.Where(func(r Spawn) bool { return errors.Is(r.Error, target) })
	return a
}

// RemoteSpawnAssert asserts over remote spawns (egress).
type RemoteSpawnAssert struct{ *Assertion[RemoteSpawn] }

// ShouldRemoteSpawn starts a remote-spawn assertion on this node.
func (a *Asserter) ShouldRemoteSpawn() *RemoteSpawnAssert {
	return &RemoteSpawnAssert{For[RemoteSpawn](a.t, a.rec)}
}
func (a *RemoteSpawnAssert) From(parent gen.PID) *RemoteSpawnAssert {
	a.Where(func(r RemoteSpawn) bool { return r.Parent == parent })
	return a
}
func (a *RemoteSpawnAssert) To(node gen.Atom) *RemoteSpawnAssert {
	a.Where(func(r RemoteSpawn) bool { return r.Node == node })
	return a
}
func (a *RemoteSpawnAssert) Name(name gen.Atom) *RemoteSpawnAssert {
	a.Where(func(r RemoteSpawn) bool { return r.Name == name })
	return a
}
func (a *RemoteSpawnAssert) Register(name gen.Atom) *RemoteSpawnAssert {
	a.Where(func(r RemoteSpawn) bool { return r.Register == name })
	return a
}
func (a *RemoteSpawnAssert) Child(pid gen.PID) *RemoteSpawnAssert {
	a.Where(func(r RemoteSpawn) bool { return r.Child == pid })
	return a
}
func (a *RemoteSpawnAssert) Error(target error) *RemoteSpawnAssert {
	a.Where(func(r RemoteSpawn) bool { return r.Error == target })
	return a
}
func (a *RemoteSpawnAssert) ErrorIs(target error) *RemoteSpawnAssert {
	a.Where(func(r RemoteSpawn) bool { return errors.Is(r.Error, target) })
	return a
}

// RemoteApplicationStartAssert asserts over remote application starts (egress).
type RemoteApplicationStartAssert struct {
	*Assertion[RemoteApplicationStart]
}

// ShouldRemoteApplicationStart starts a remote-application-start assertion on this node.
func (a *Asserter) ShouldRemoteApplicationStart() *RemoteApplicationStartAssert {
	return &RemoteApplicationStartAssert{For[RemoteApplicationStart](a.t, a.rec)}
}
func (a *RemoteApplicationStartAssert) From(pid gen.PID) *RemoteApplicationStartAssert {
	a.Where(func(r RemoteApplicationStart) bool { return r.From == pid })
	return a
}
func (a *RemoteApplicationStartAssert) To(node gen.Atom) *RemoteApplicationStartAssert {
	a.Where(func(r RemoteApplicationStart) bool { return r.Node == node })
	return a
}
func (a *RemoteApplicationStartAssert) Name(name gen.Atom) *RemoteApplicationStartAssert {
	a.Where(func(r RemoteApplicationStart) bool { return r.Name == name })
	return a
}
func (a *RemoteApplicationStartAssert) Mode(mode gen.ApplicationMode) *RemoteApplicationStartAssert {
	a.Where(func(r RemoteApplicationStart) bool { return r.Mode == mode })
	return a
}
func (a *RemoteApplicationStartAssert) Error(target error) *RemoteApplicationStartAssert {
	a.Where(func(r RemoteApplicationStart) bool { return r.Error == target })
	return a
}
func (a *RemoteApplicationStartAssert) ErrorIs(target error) *RemoteApplicationStartAssert {
	a.Where(func(r RemoteApplicationStart) bool { return errors.Is(r.Error, target) })
	return a
}

// SpawnMetaAssert asserts over meta-process spawns (egress).
type SpawnMetaAssert struct{ *Assertion[SpawnMeta] }

// ShouldSpawnMeta starts a meta-spawn assertion on this node.
func (a *Asserter) ShouldSpawnMeta() *SpawnMetaAssert {
	return &SpawnMetaAssert{For[SpawnMeta](a.t, a.rec)}
}
func (a *SpawnMetaAssert) From(parent gen.PID) *SpawnMetaAssert {
	a.Where(func(r SpawnMeta) bool { return r.Parent == parent })
	return a
}
func (a *SpawnMetaAssert) Alias(alias gen.Alias) *SpawnMetaAssert {
	a.Where(func(r SpawnMeta) bool { return r.Alias == alias })
	return a
}
func (a *SpawnMetaAssert) Error(target error) *SpawnMetaAssert {
	a.Where(func(r SpawnMeta) bool { return r.Error == target })
	return a
}
func (a *SpawnMetaAssert) ErrorIs(target error) *SpawnMetaAssert {
	a.Where(func(r SpawnMeta) bool { return errors.Is(r.Error, target) })
	return a
}

// CreateAliasAssert asserts over CreateAlias (egress).
type CreateAliasAssert struct{ *Assertion[CreateAlias] }

// ShouldCreateAlias starts a create-alias assertion on this node.
func (a *Asserter) ShouldCreateAlias() *CreateAliasAssert {
	return &CreateAliasAssert{For[CreateAlias](a.t, a.rec)}
}
func (a *CreateAliasAssert) From(pid gen.PID) *CreateAliasAssert {
	a.Where(func(r CreateAlias) bool { return r.PID == pid })
	return a
}
func (a *CreateAliasAssert) Alias(alias gen.Alias) *CreateAliasAssert {
	a.Where(func(r CreateAlias) bool { return r.Alias == alias })
	return a
}
func (a *CreateAliasAssert) Error(target error) *CreateAliasAssert {
	a.Where(func(r CreateAlias) bool { return r.Error == target })
	return a
}
func (a *CreateAliasAssert) ErrorIs(target error) *CreateAliasAssert {
	a.Where(func(r CreateAlias) bool { return errors.Is(r.Error, target) })
	return a
}

// DeleteAliasAssert asserts over DeleteAlias (egress).
type DeleteAliasAssert struct{ *Assertion[DeleteAlias] }

// ShouldDeleteAlias starts a delete-alias assertion on this node.
func (a *Asserter) ShouldDeleteAlias() *DeleteAliasAssert {
	return &DeleteAliasAssert{For[DeleteAlias](a.t, a.rec)}
}
func (a *DeleteAliasAssert) From(pid gen.PID) *DeleteAliasAssert {
	a.Where(func(r DeleteAlias) bool { return r.PID == pid })
	return a
}
func (a *DeleteAliasAssert) Alias(alias gen.Alias) *DeleteAliasAssert {
	a.Where(func(r DeleteAlias) bool { return r.Alias == alias })
	return a
}
func (a *DeleteAliasAssert) Error(target error) *DeleteAliasAssert {
	a.Where(func(r DeleteAlias) bool { return r.Error == target })
	return a
}
func (a *DeleteAliasAssert) ErrorIs(target error) *DeleteAliasAssert {
	a.Where(func(r DeleteAlias) bool { return errors.Is(r.Error, target) })
	return a
}

// RegisterEventAssert asserts over RegisterEvent (egress).
type RegisterEventAssert struct{ *Assertion[RegisterEvent] }

// ShouldRegisterEvent starts a register-event assertion on this node.
func (a *Asserter) ShouldRegisterEvent() *RegisterEventAssert {
	return &RegisterEventAssert{For[RegisterEvent](a.t, a.rec)}
}
func (a *RegisterEventAssert) From(pid gen.PID) *RegisterEventAssert {
	a.Where(func(r RegisterEvent) bool { return r.PID == pid })
	return a
}
func (a *RegisterEventAssert) Name(name gen.Atom) *RegisterEventAssert {
	a.Where(func(r RegisterEvent) bool { return r.Name == name })
	return a
}
func (a *RegisterEventAssert) Ref(ref gen.Ref) *RegisterEventAssert {
	a.Where(func(r RegisterEvent) bool { return r.Ref == ref })
	return a
}
func (a *RegisterEventAssert) Error(target error) *RegisterEventAssert {
	a.Where(func(r RegisterEvent) bool { return r.Error == target })
	return a
}
func (a *RegisterEventAssert) ErrorIs(target error) *RegisterEventAssert {
	a.Where(func(r RegisterEvent) bool { return errors.Is(r.Error, target) })
	return a
}

// UnregisterEventAssert asserts over UnregisterEvent (egress).
type UnregisterEventAssert struct{ *Assertion[UnregisterEvent] }

// ShouldUnregisterEvent starts an unregister-event assertion on this node.
func (a *Asserter) ShouldUnregisterEvent() *UnregisterEventAssert {
	return &UnregisterEventAssert{For[UnregisterEvent](a.t, a.rec)}
}
func (a *UnregisterEventAssert) From(pid gen.PID) *UnregisterEventAssert {
	a.Where(func(r UnregisterEvent) bool { return r.PID == pid })
	return a
}
func (a *UnregisterEventAssert) Name(name gen.Atom) *UnregisterEventAssert {
	a.Where(func(r UnregisterEvent) bool { return r.Name == name })
	return a
}
func (a *UnregisterEventAssert) Error(target error) *UnregisterEventAssert {
	a.Where(func(r UnregisterEvent) bool { return r.Error == target })
	return a
}
func (a *UnregisterEventAssert) ErrorIs(target error) *UnregisterEventAssert {
	a.Where(func(r UnregisterEvent) bool { return errors.Is(r.Error, target) })
	return a
}

// ForwardAssert asserts over messages forwarded by a process (egress).
type ForwardAssert struct{ *Assertion[Forward] }

// ShouldForward starts a forward assertion on this node.
func (a *Asserter) ShouldForward() *ForwardAssert {
	return &ForwardAssert{For[Forward](a.t, a.rec)}
}
func (a *ForwardAssert) By(pid gen.PID) *ForwardAssert {
	a.Where(func(r Forward) bool { return r.By == pid })
	return a
}
func (a *ForwardAssert) From(pid gen.PID) *ForwardAssert {
	a.Where(func(r Forward) bool { return r.From == pid })
	return a
}
func (a *ForwardAssert) To(pid gen.PID) *ForwardAssert {
	a.Where(func(r Forward) bool { return r.To == pid })
	return a
}
func (a *ForwardAssert) Message(v any) *ForwardAssert {
	a.Where(func(r Forward) bool { return reflect.DeepEqual(r.Message, v) })
	return a
}
func (a *ForwardAssert) Error(target error) *ForwardAssert {
	a.Where(func(r Forward) bool { return r.Error == target })
	return a
}
func (a *ForwardAssert) ErrorIs(target error) *ForwardAssert {
	a.Where(func(r Forward) bool { return errors.Is(r.Error, target) })
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

// CallAssert asserts over outgoing requests observed on a node.
type CallAssert struct{ *Assertion[Call] }

// ShouldCall starts an egress call assertion on this node.
func (a *Asserter) ShouldCall() *CallAssert { return &CallAssert{For[Call](a.t, a.rec)} }
func (a *CallAssert) From(p gen.PID) *CallAssert {
	a.Where(func(r Call) bool { return r.From == p })
	return a
}
func (a *CallAssert) To(to any) *CallAssert {
	a.Where(func(r Call) bool { return reflect.DeepEqual(r.To, to) })
	return a
}
func (a *CallAssert) Request(v any) *CallAssert {
	a.Where(func(r Call) bool { return reflect.DeepEqual(r.Request, v) })
	return a
}
func (a *CallAssert) Error(target error) *CallAssert {
	a.Where(func(r Call) bool { return r.Error == target })
	return a
}
func (a *CallAssert) ErrorIs(target error) *CallAssert {
	a.Where(func(r Call) bool { return errors.Is(r.Error, target) })
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
	case gen.MessageDownProxy:
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
func (a *DownAssert) AboutNode(name gen.Atom) *DownAssert {
	a.Where(func(r Down) bool { m, ok := r.Message.(gen.MessageDownNode); return ok && m.Name == name })
	return a
}
func (a *DownAssert) AboutProxy(node gen.Atom) *DownAssert {
	a.Where(func(r Down) bool { m, ok := r.Message.(gen.MessageDownProxy); return ok && m.Node == node })
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
func (a *ExitAssert) AboutNode(name gen.Atom) *ExitAssert {
	a.Where(func(r Exit) bool { m, ok := r.Message.(gen.MessageExitNode); return ok && m.Name == name })
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
func (a *EventAssert) Timestamp(ts int64) *EventAssert {
	a.Where(func(r Event) bool { return r.Timestamp == ts })
	return a
}

// MonitorAssert asserts over monitors set up on a node (egress).
type MonitorAssert struct{ *Assertion[Monitor] }

// ShouldMonitor starts a monitor-setup assertion on this node.
func (a *Asserter) ShouldMonitor() *MonitorAssert {
	return &MonitorAssert{For[Monitor](a.t, a.rec)}
}
func (a *MonitorAssert) From(p gen.PID) *MonitorAssert {
	a.Where(func(r Monitor) bool { return r.From == p })
	return a
}
func (a *MonitorAssert) Target(t any) *MonitorAssert {
	a.Where(func(r Monitor) bool { return reflect.DeepEqual(r.Target, t) })
	return a
}
func (a *MonitorAssert) Error(target error) *MonitorAssert {
	a.Where(func(r Monitor) bool { return r.Error == target })
	return a
}
func (a *MonitorAssert) ErrorIs(target error) *MonitorAssert {
	a.Where(func(r Monitor) bool { return errors.Is(r.Error, target) })
	return a
}

// DemonitorAssert asserts over monitors removed on a node (egress).
type DemonitorAssert struct{ *Assertion[Demonitor] }

// ShouldDemonitor starts a demonitor assertion on this node.
func (a *Asserter) ShouldDemonitor() *DemonitorAssert {
	return &DemonitorAssert{For[Demonitor](a.t, a.rec)}
}
func (a *DemonitorAssert) From(p gen.PID) *DemonitorAssert {
	a.Where(func(r Demonitor) bool { return r.From == p })
	return a
}
func (a *DemonitorAssert) Target(t any) *DemonitorAssert {
	a.Where(func(r Demonitor) bool { return reflect.DeepEqual(r.Target, t) })
	return a
}
func (a *DemonitorAssert) Error(target error) *DemonitorAssert {
	a.Where(func(r Demonitor) bool { return r.Error == target })
	return a
}
func (a *DemonitorAssert) ErrorIs(target error) *DemonitorAssert {
	a.Where(func(r Demonitor) bool { return errors.Is(r.Error, target) })
	return a
}

// LinkAssert asserts over links set up on a node (egress).
type LinkAssert struct{ *Assertion[Link] }

// ShouldLink starts a link-setup assertion on this node.
func (a *Asserter) ShouldLink() *LinkAssert { return &LinkAssert{For[Link](a.t, a.rec)} }
func (a *LinkAssert) From(p gen.PID) *LinkAssert {
	a.Where(func(r Link) bool { return r.From == p })
	return a
}
func (a *LinkAssert) Target(t any) *LinkAssert {
	a.Where(func(r Link) bool { return reflect.DeepEqual(r.Target, t) })
	return a
}
func (a *LinkAssert) Error(target error) *LinkAssert {
	a.Where(func(r Link) bool { return r.Error == target })
	return a
}
func (a *LinkAssert) ErrorIs(target error) *LinkAssert {
	a.Where(func(r Link) bool { return errors.Is(r.Error, target) })
	return a
}

// UnlinkAssert asserts over links removed on a node (egress).
type UnlinkAssert struct{ *Assertion[Unlink] }

// ShouldUnlink starts an unlink assertion on this node.
func (a *Asserter) ShouldUnlink() *UnlinkAssert { return &UnlinkAssert{For[Unlink](a.t, a.rec)} }
func (a *UnlinkAssert) From(p gen.PID) *UnlinkAssert {
	a.Where(func(r Unlink) bool { return r.From == p })
	return a
}
func (a *UnlinkAssert) Target(t any) *UnlinkAssert {
	a.Where(func(r Unlink) bool { return reflect.DeepEqual(r.Target, t) })
	return a
}
func (a *UnlinkAssert) Error(target error) *UnlinkAssert {
	a.Where(func(r Unlink) bool { return r.Error == target })
	return a
}
func (a *UnlinkAssert) ErrorIs(target error) *UnlinkAssert {
	a.Where(func(r Unlink) bool { return errors.Is(r.Error, target) })
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
type SendEventAssert struct{ *Assertion[SendEvent] }

// ShouldSendEvent starts an event-publish assertion on this node.
func (a *Asserter) ShouldSendEvent() *SendEventAssert {
	return &SendEventAssert{For[SendEvent](a.t, a.rec)}
}
func (a *SendEventAssert) From(p gen.PID) *SendEventAssert {
	a.Where(func(r SendEvent) bool { return r.From == p })
	return a
}
func (a *SendEventAssert) Name(name gen.Atom) *SendEventAssert {
	a.Where(func(r SendEvent) bool { return r.Name == name })
	return a
}
func (a *SendEventAssert) Error(target error) *SendEventAssert {
	a.Where(func(r SendEvent) bool { return r.Error == target })
	return a
}
func (a *SendEventAssert) ErrorIs(target error) *SendEventAssert {
	a.Where(func(r SendEvent) bool { return errors.Is(r.Error, target) })
	return a
}
func (a *SendEventAssert) Priority(p gen.MessagePriority) *SendEventAssert {
	a.Where(func(r SendEvent) bool { return r.Options.Priority == p })
	return a
}
func (a *SendEventAssert) Important(important bool) *SendEventAssert {
	a.Where(func(r SendEvent) bool { return r.Options.ImportantDelivery == important })
	return a
}
func (a *SendEventAssert) KeepNetworkOrder(keep bool) *SendEventAssert {
	a.Where(func(r SendEvent) bool { return r.Options.KeepNetworkOrder == keep })
	return a
}

// SendResponseAssert asserts over responses a process sent to requests (egress).
type SendResponseAssert struct{ *Assertion[SendResponse] }

// ShouldSendResponse starts a response assertion on this node.
func (a *Asserter) ShouldSendResponse() *SendResponseAssert {
	return &SendResponseAssert{For[SendResponse](a.t, a.rec)}
}
func (a *SendResponseAssert) From(p gen.PID) *SendResponseAssert {
	a.Where(func(r SendResponse) bool { return r.From == p })
	return a
}
func (a *SendResponseAssert) To(p gen.PID) *SendResponseAssert {
	a.Where(func(r SendResponse) bool { return r.To == p })
	return a
}
func (a *SendResponseAssert) Message(v any) *SendResponseAssert {
	a.Where(func(r SendResponse) bool { return reflect.DeepEqual(r.Message, v) })
	return a
}
func (a *SendResponseAssert) Ref(ref gen.Ref) *SendResponseAssert {
	a.Where(func(r SendResponse) bool { return r.Ref == ref })
	return a
}
func (a *SendResponseAssert) Error(target error) *SendResponseAssert {
	a.Where(func(r SendResponse) bool { return r.Error == target })
	return a
}
func (a *SendResponseAssert) ErrorIs(target error) *SendResponseAssert {
	a.Where(func(r SendResponse) bool { return errors.Is(r.Error, target) })
	return a
}
func (a *SendResponseAssert) Important(important bool) *SendResponseAssert {
	a.Where(func(r SendResponse) bool { return r.Options.ImportantDelivery == important })
	return a
}
func (a *SendResponseAssert) Priority(p gen.MessagePriority) *SendResponseAssert {
	a.Where(func(r SendResponse) bool { return r.Options.Priority == p })
	return a
}
func (a *SendResponseAssert) KeepNetworkOrder(keep bool) *SendResponseAssert {
	a.Where(func(r SendResponse) bool { return r.Options.KeepNetworkOrder == keep })
	return a
}

// SendExitAssert asserts over exit signals a process sent via SendExit (egress).
type SendExitAssert struct{ *Assertion[SendExit] }

func (a *Asserter) ShouldSendExit() *SendExitAssert {
	return &SendExitAssert{For[SendExit](a.t, a.rec)}
}
func (x *SendExitAssert) From(p gen.PID) *SendExitAssert {
	x.Where(func(r SendExit) bool { return r.From == p })
	return x
}
func (x *SendExitAssert) To(p gen.PID) *SendExitAssert {
	x.Where(func(r SendExit) bool { return r.To == p })
	return x
}
func (x *SendExitAssert) Reason(target error) *SendExitAssert {
	x.Where(func(r SendExit) bool { return r.Reason == target })
	return x
}
func (x *SendExitAssert) ReasonIs(target error) *SendExitAssert {
	x.Where(func(r SendExit) bool { return errors.Is(r.Reason, target) })
	return x
}

// LogAssert asserts over log lines a process emitted (egress).
type LogAssert struct{ *Assertion[Log] }

func (a *Asserter) ShouldLog() *LogAssert { return &LogAssert{For[Log](a.t, a.rec)} }
func (x *LogAssert) From(p gen.PID) *LogAssert {
	x.Where(func(r Log) bool { return r.From == p })
	return x
}
func (x *LogAssert) Level(l gen.LogLevel) *LogAssert {
	x.Where(func(r Log) bool { return r.Level == l })
	return x
}
func (x *LogAssert) Message(msg string) *LogAssert {
	x.Where(func(r Log) bool { return r.Message == msg })
	return x
}
func (x *LogAssert) Containing(substr string) *LogAssert {
	x.Where(func(r Log) bool { return strings.Contains(r.Message, substr) })
	return x
}

// TerminateAssert asserts over the subject actor's own termination (unit). With
// no terminate, None() confirms the actor survived.
type TerminateAssert struct{ *Assertion[Terminated] }

func (a *Asserter) ShouldTerminate() *TerminateAssert {
	return &TerminateAssert{For[Terminated](a.t, a.rec)}
}
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

// SendAfterAssert asserts over delayed sends scheduled via SendAfter (egress).
type SendAfterAssert struct{ *Assertion[SendAfter] }

func (a *Asserter) ShouldSendAfter() *SendAfterAssert {
	return &SendAfterAssert{For[SendAfter](a.t, a.rec)}
}
func (x *SendAfterAssert) From(p gen.PID) *SendAfterAssert {
	x.Where(func(r SendAfter) bool { return r.From == p })
	return x
}
func (x *SendAfterAssert) To(to any) *SendAfterAssert {
	x.Where(func(r SendAfter) bool { return reflect.DeepEqual(r.To, to) })
	return x
}
func (x *SendAfterAssert) Message(v any) *SendAfterAssert {
	x.Where(func(r SendAfter) bool { return reflect.DeepEqual(r.Message, v) })
	return x
}
func (x *SendAfterAssert) After(after time.Duration) *SendAfterAssert {
	x.Where(func(r SendAfter) bool { return r.After == after })
	return x
}
func (x *SendAfterAssert) Priority(p gen.MessagePriority) *SendAfterAssert {
	x.Where(func(r SendAfter) bool { return r.Options.Priority == p })
	return x
}
func (x *SendAfterAssert) Important(important bool) *SendAfterAssert {
	x.Where(func(r SendAfter) bool { return r.Options.ImportantDelivery == important })
	return x
}
func (x *SendAfterAssert) KeepNetworkOrder(keep bool) *SendAfterAssert {
	x.Where(func(r SendAfter) bool { return r.Options.KeepNetworkOrder == keep })
	return x
}
