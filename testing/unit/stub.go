package unit

import (
	"reflect"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
)

// stubs configures what the process's outbound operations return. The default for an
// unstubbed operation is permissive (see the mock process): Call returns (nil,nil),
// the value-producing operations return a deterministic synthetic value, and the
// error-only operations succeed. A negative scenario is opted into with Fail.
//
// When several stubs match an operation, the most recently registered one wins.
type stubs struct {
	call      []*CallStub
	spawn     []*SpawnStub
	meta      []*SpawnMetaStub
	remote    []*RemoteSpawnStub
	alias     []*CreateAliasStub
	regev     []*RegisterEventStub
	send      []*FailStub
	link      []*FailStub
	unlink    []*FailStub
	monitor   []*FailStub
	demonitor []*FailStub
	exit      []*FailStub
	exitMeta  []*FailStub
	forward   []*FailStub
}

func newStubs() *stubs { return &stubs{} }

// targetMatch reports whether a stub target matches the actual target. A nil stub
// target matches anything.
func targetMatch(stub, actual any) bool {
	return stub == nil || reflect.DeepEqual(stub, actual)
}

func factoryPtr(f gen.ProcessFactory) uintptr { return reflect.ValueOf(f).Pointer() }

// Call

// CallStub stubs the process's outbound Call to a target.
type CallStub struct {
	to    any
	where check.Matcher
	fn    func(request any) (any, error)
}

// OnCall stubs the process's outbound Call to `to` (PID/ProcessID/Alias/Atom).
func (a *Subject) OnCall(to any) *CallStub {
	s := &CallStub{to: to}
	a.stubs.call = append(a.stubs.call, s)
	return s
}

// Where narrows the stub to requests matching m.
func (s *CallStub) Where(m check.Matcher) *CallStub { s.where = m; return s }

// Respond returns (resp, nil) for a matching call.
func (s *CallStub) Respond(resp any) { s.fn = func(any) (any, error) { return resp, nil } }

// Fail returns (nil, err) for a matching call.
func (s *CallStub) Fail(err error) { s.fn = func(any) (any, error) { return nil, err } }

// RespondWith computes the (response, error) from the actual request.
func (s *CallStub) RespondWith(fn func(request any) (any, error)) { s.fn = fn }

func (st *stubs) resolveCall(to any, request any) (any, error, bool) {
	for i := len(st.call) - 1; i >= 0; i-- {
		s := st.call[i]
		if targetMatch(s.to, to) == false {
			continue
		}
		if s.where != nil && s.where(request) == false {
			continue
		}
		v, err := s.fn(request)
		return v, err, true
	}
	return nil, nil, false
}

// Spawn

// SpawnStub stubs the process's outbound Spawn / SpawnRegister of a factory.
type SpawnStub struct {
	factory uintptr
	pid     gen.PID
	err     error
	fn      func() (gen.PID, error)
}

// OnSpawn stubs Spawn/SpawnRegister of the given factory.
func (a *Subject) OnSpawn(factory gen.ProcessFactory) *SpawnStub {
	s := &SpawnStub{factory: factoryPtr(factory)}
	a.stubs.spawn = append(a.stubs.spawn, s)
	return s
}

// Return makes the spawn return pid.
func (s *SpawnStub) Return(pid gen.PID) { s.pid = pid }

// Fail makes the spawn return err.
func (s *SpawnStub) Fail(err error) { s.err = err }

// ReturnFunc computes the (pid, error) per matching spawn. A counter in the closure
// fails selectively, e.g. only the 3rd spawn of this factory.
func (s *SpawnStub) ReturnFunc(fn func() (gen.PID, error)) { s.fn = fn }

func (st *stubs) resolveSpawn(factory gen.ProcessFactory) (gen.PID, error, bool) {
	fp := factoryPtr(factory)
	for i := len(st.spawn) - 1; i >= 0; i-- {
		if st.spawn[i].factory == fp {
			if st.spawn[i].fn != nil {
				pid, err := st.spawn[i].fn()
				return pid, err, true
			}
			return st.spawn[i].pid, st.spawn[i].err, true
		}
	}
	return gen.PID{}, nil, false
}

// SpawnMeta

// SpawnMetaStub stubs SpawnMeta (matched by the meta behavior's dynamic type).
type SpawnMetaStub struct {
	typ   reflect.Type
	alias gen.Alias
	err   error
}

// OnSpawnMeta stubs SpawnMeta of a behavior of the same dynamic type as sample.
func (a *Subject) OnSpawnMeta(sample gen.MetaBehavior) *SpawnMetaStub {
	s := &SpawnMetaStub{typ: reflect.TypeOf(sample)}
	a.stubs.meta = append(a.stubs.meta, s)
	return s
}

// Return makes SpawnMeta return alias.
func (s *SpawnMetaStub) Return(alias gen.Alias) { s.alias = alias }

// Fail makes SpawnMeta return err.
func (s *SpawnMetaStub) Fail(err error) { s.err = err }

func (st *stubs) resolveSpawnMeta(behavior gen.MetaBehavior) (gen.Alias, error, bool) {
	bt := reflect.TypeOf(behavior)
	for i := len(st.meta) - 1; i >= 0; i-- {
		if st.meta[i].typ == bt {
			return st.meta[i].alias, st.meta[i].err, true
		}
	}
	return gen.Alias{}, nil, false
}

// RemoteSpawn

// RemoteSpawnStub stubs RemoteSpawn / RemoteSpawnRegister (matched by node+name).
type RemoteSpawnStub struct {
	node gen.Atom
	name gen.Atom
	pid  gen.PID
	err  error
}

// OnRemoteSpawn stubs RemoteSpawn of name on node.
func (a *Subject) OnRemoteSpawn(node, name gen.Atom) *RemoteSpawnStub {
	s := &RemoteSpawnStub{node: node, name: name}
	a.stubs.remote = append(a.stubs.remote, s)
	return s
}

// Return makes the remote spawn return pid.
func (s *RemoteSpawnStub) Return(pid gen.PID) { s.pid = pid }

// Fail makes the remote spawn return err.
func (s *RemoteSpawnStub) Fail(err error) { s.err = err }

func (st *stubs) resolveRemoteSpawn(node, name gen.Atom) (gen.PID, error, bool) {
	for i := len(st.remote) - 1; i >= 0; i-- {
		if st.remote[i].node == node && st.remote[i].name == name {
			return st.remote[i].pid, st.remote[i].err, true
		}
	}
	return gen.PID{}, nil, false
}

// CreateAlias

// CreateAliasStub stubs CreateAlias.
type CreateAliasStub struct {
	alias gen.Alias
	err   error
}

// OnCreateAlias stubs the process's CreateAlias.
func (a *Subject) OnCreateAlias() *CreateAliasStub {
	s := &CreateAliasStub{}
	a.stubs.alias = append(a.stubs.alias, s)
	return s
}

// Return makes CreateAlias return alias.
func (s *CreateAliasStub) Return(alias gen.Alias) { s.alias = alias }

// Fail makes CreateAlias return err.
func (s *CreateAliasStub) Fail(err error) { s.err = err }

func (st *stubs) resolveCreateAlias() (gen.Alias, error, bool) {
	if n := len(st.alias); n > 0 {
		return st.alias[n-1].alias, st.alias[n-1].err, true
	}
	return gen.Alias{}, nil, false
}

// RegisterEvent

// RegisterEventStub stubs RegisterEvent (matched by event name).
type RegisterEventStub struct {
	name  gen.Atom
	token gen.Ref
	err   error
}

// OnRegisterEvent stubs RegisterEvent of the given name.
func (a *Subject) OnRegisterEvent(name gen.Atom) *RegisterEventStub {
	s := &RegisterEventStub{name: name}
	a.stubs.regev = append(a.stubs.regev, s)
	return s
}

// Return makes RegisterEvent return token.
func (s *RegisterEventStub) Return(token gen.Ref) { s.token = token }

// Fail makes RegisterEvent return err.
func (s *RegisterEventStub) Fail(err error) { s.err = err }

func (st *stubs) resolveRegisterEvent(name gen.Atom) (gen.Ref, error, bool) {
	for i := len(st.regev) - 1; i >= 0; i-- {
		if st.regev[i].name == name {
			return st.regev[i].token, st.regev[i].err, true
		}
	}
	return gen.Ref{}, nil, false
}

// error-only operations (Send / Link / Monitor / SendExit)

// FailStub makes an error-only operation to a target fail.
type FailStub struct {
	to  any
	err error
	fn  func() error
}

// Fail sets the error returned for a matching operation.
func (s *FailStub) Fail(err error) { s.err = err }

// FailFunc computes the error per matching invocation. A counter in the closure
// fails selectively, e.g. only the 2nd and 5th call:
//
//	i := 0
//	sub.OnSend("svc").FailFunc(func() error {
//		i++
//		if i == 2 || i == 5 { return gen.ErrProcessMailboxFull }
//		return nil
//	})
func (s *FailStub) FailFunc(fn func() error) { s.fn = fn }

func resolveFail(list []*FailStub, to any) (error, bool) {
	for i := len(list) - 1; i >= 0; i-- {
		if targetMatch(list[i].to, to) {
			if list[i].fn != nil {
				return list[i].fn(), true
			}
			return list[i].err, true
		}
	}
	return nil, false
}

// OnSend stubs a Send to `to` to fail.
func (a *Subject) OnSend(to any) *FailStub {
	s := &FailStub{to: to}
	a.stubs.send = append(a.stubs.send, s)
	return s
}

// OnLink stubs a Link to target to fail.
func (a *Subject) OnLink(target any) *FailStub {
	s := &FailStub{to: target}
	a.stubs.link = append(a.stubs.link, s)
	return s
}

// OnUnlink stubs an Unlink of target to fail.
func (a *Subject) OnUnlink(target any) *FailStub {
	s := &FailStub{to: target}
	a.stubs.unlink = append(a.stubs.unlink, s)
	return s
}

// OnMonitor stubs a Monitor of target to fail.
func (a *Subject) OnMonitor(target any) *FailStub {
	s := &FailStub{to: target}
	a.stubs.monitor = append(a.stubs.monitor, s)
	return s
}

// OnDemonitor stubs a Demonitor of target to fail.
func (a *Subject) OnDemonitor(target any) *FailStub {
	s := &FailStub{to: target}
	a.stubs.demonitor = append(a.stubs.demonitor, s)
	return s
}

// OnSendExit stubs a SendExit to pid to fail.
func (a *Subject) OnSendExit(pid gen.PID) *FailStub {
	s := &FailStub{to: pid}
	a.stubs.exit = append(a.stubs.exit, s)
	return s
}

// OnSendExitMeta stubs a SendExitMeta to the meta alias to fail.
func (a *Subject) OnSendExitMeta(meta gen.Alias) *FailStub {
	s := &FailStub{to: meta}
	a.stubs.exitMeta = append(a.stubs.exitMeta, s)
	return s
}

// OnForward stubs a Forward to the target PID to fail.
func (a *Subject) OnForward(to gen.PID) *FailStub {
	s := &FailStub{to: to}
	a.stubs.forward = append(a.stubs.forward, s)
	return s
}

// Node-level egress stubs. The mock node has its own egress (Call, Send, Spawn,
// ...) routing through the same stubs as the process under test, so these setters
// promote onto MockNode: they configure the node's own egress and let any egress
// be stubbed before the process is created, e.g. unit.StartNode(t).OnCall(...).Spawn(...)
// or sub := n.Prepare(...); sub.OnCall(...); sub.Run(). They share storage with the
// Subject setters above.

func (n *mockNode) OnCall(to any) *CallStub {
	s := &CallStub{to: to}
	n.stubs.call = append(n.stubs.call, s)
	return s
}

func (n *mockNode) OnSpawn(factory gen.ProcessFactory) *SpawnStub {
	s := &SpawnStub{factory: factoryPtr(factory)}
	n.stubs.spawn = append(n.stubs.spawn, s)
	return s
}

func (n *mockNode) OnSpawnMeta(sample gen.MetaBehavior) *SpawnMetaStub {
	s := &SpawnMetaStub{typ: reflect.TypeOf(sample)}
	n.stubs.meta = append(n.stubs.meta, s)
	return s
}

func (n *mockNode) OnRemoteSpawn(node, name gen.Atom) *RemoteSpawnStub {
	s := &RemoteSpawnStub{node: node, name: name}
	n.stubs.remote = append(n.stubs.remote, s)
	return s
}

func (n *mockNode) OnCreateAlias() *CreateAliasStub {
	s := &CreateAliasStub{}
	n.stubs.alias = append(n.stubs.alias, s)
	return s
}

func (n *mockNode) OnRegisterEvent(name gen.Atom) *RegisterEventStub {
	s := &RegisterEventStub{name: name}
	n.stubs.regev = append(n.stubs.regev, s)
	return s
}

func (n *mockNode) OnSend(to any) *FailStub {
	s := &FailStub{to: to}
	n.stubs.send = append(n.stubs.send, s)
	return s
}

func (n *mockNode) OnLink(target any) *FailStub {
	s := &FailStub{to: target}
	n.stubs.link = append(n.stubs.link, s)
	return s
}

func (n *mockNode) OnUnlink(target any) *FailStub {
	s := &FailStub{to: target}
	n.stubs.unlink = append(n.stubs.unlink, s)
	return s
}

func (n *mockNode) OnMonitor(target any) *FailStub {
	s := &FailStub{to: target}
	n.stubs.monitor = append(n.stubs.monitor, s)
	return s
}

func (n *mockNode) OnDemonitor(target any) *FailStub {
	s := &FailStub{to: target}
	n.stubs.demonitor = append(n.stubs.demonitor, s)
	return s
}

func (n *mockNode) OnSendExit(pid gen.PID) *FailStub {
	s := &FailStub{to: pid}
	n.stubs.exit = append(n.stubs.exit, s)
	return s
}

func (n *mockNode) OnSendExitMeta(meta gen.Alias) *FailStub {
	s := &FailStub{to: meta}
	n.stubs.exitMeta = append(n.stubs.exitMeta, s)
	return s
}

func (n *mockNode) OnForward(to gen.PID) *FailStub {
	s := &FailStub{to: to}
	n.stubs.forward = append(n.stubs.forward, s)
	return s
}

// Meta-level egress stubs. A meta process has its own stub store, isolated from the
// parent actor: only Send and child SpawnMeta consult stubs (SendResponse is always
// recorded, and a meta has no Call). Set them on the MetaSubject before the egress
// happens; for Init-time egress, on a PrepareMeta'd subject before Run.

func (m *MetaSubject) OnSend(to any) *FailStub {
	s := &FailStub{to: to}
	m.stubs.send = append(m.stubs.send, s)
	return s
}

func (m *MetaSubject) OnSpawnMeta(sample gen.MetaBehavior) *SpawnMetaStub {
	s := &SpawnMetaStub{typ: reflect.TypeOf(sample)}
	m.stubs.meta = append(m.stubs.meta, s)
	return s
}
