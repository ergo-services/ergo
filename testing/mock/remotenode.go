package mock

import (
	"sync/atomic"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
)

// RemoteNode is a standalone gen.RemoteNode mock. Every method has an On<Method>
// override; unset, the spawn/application-start emitters record their egress and the
// rest return safe defaults.
type RemoteNode struct {
	recorder
	name gen.Atom
	next atomic.Uint64
	ov   remoteNodeOverrides
}

type remoteNodeOverrides struct {
	name                      func() gen.Atom
	uptime                    func() int64
	connectionUptime          func() int64
	version                   func() gen.Version
	info                      func() gen.RemoteNodeInfo
	spawn                     func(name gen.Atom, options gen.ProcessOptions, args ...any) (gen.PID, error)
	spawnRegister             func(register gen.Atom, name gen.Atom, options gen.ProcessOptions, args ...any) (gen.PID, error)
	applicationStart          func(name gen.Atom, options gen.ApplicationOptions) error
	applicationStartTemporary func(name gen.Atom, options gen.ApplicationOptions) error
	applicationStartTransient func(name gen.Atom, options gen.ApplicationOptions) error
	applicationStartPermanent func(name gen.Atom, options gen.ApplicationOptions) error
	applicationInfo           func(name gen.Atom) (gen.ApplicationInfo, error)
	creation                  func() int64
	disconnect                func()
}

var _ gen.RemoteNode = (*RemoteNode)(nil)

// NewRemoteNode returns a dumb gen.RemoteNode mock (no recording; use NewRemoteNodeT
// for Should*).
func NewRemoteNode() *RemoteNode { return newRemoteNode(recorder{}) }

// NewRemoteNodeT returns a gen.RemoteNode mock that records every spawn and
// application start as an egress record and asserts through t.
func NewRemoteNodeT(t check.T) *RemoteNode { return newRemoteNode(newRecorder(t)) }

func newRemoteNode(r recorder) *RemoteNode {
	return &RemoteNode{recorder: r, name: mockNode}
}

// On<Method> overrides

func (rn *RemoteNode) OnName(fn func() gen.Atom)           { rn.ov.name = fn }
func (rn *RemoteNode) OnUptime(fn func() int64)            { rn.ov.uptime = fn }
func (rn *RemoteNode) OnConnectionUptime(fn func() int64)  { rn.ov.connectionUptime = fn }
func (rn *RemoteNode) OnVersion(fn func() gen.Version)     { rn.ov.version = fn }
func (rn *RemoteNode) OnInfo(fn func() gen.RemoteNodeInfo) { rn.ov.info = fn }
func (rn *RemoteNode) OnCreation(fn func() int64)          { rn.ov.creation = fn }
func (rn *RemoteNode) OnDisconnect(fn func())              { rn.ov.disconnect = fn }
func (rn *RemoteNode) OnSpawn(fn func(name gen.Atom, options gen.ProcessOptions, args ...any) (gen.PID, error)) {
	rn.ov.spawn = fn
}
func (rn *RemoteNode) OnSpawnRegister(fn func(register gen.Atom, name gen.Atom, options gen.ProcessOptions, args ...any) (gen.PID, error)) {
	rn.ov.spawnRegister = fn
}
func (rn *RemoteNode) OnApplicationStart(fn func(name gen.Atom, options gen.ApplicationOptions) error) {
	rn.ov.applicationStart = fn
}
func (rn *RemoteNode) OnApplicationStartTemporary(fn func(name gen.Atom, options gen.ApplicationOptions) error) {
	rn.ov.applicationStartTemporary = fn
}
func (rn *RemoteNode) OnApplicationStartTransient(fn func(name gen.Atom, options gen.ApplicationOptions) error) {
	rn.ov.applicationStartTransient = fn
}
func (rn *RemoteNode) OnApplicationStartPermanent(fn func(name gen.Atom, options gen.ApplicationOptions) error) {
	rn.ov.applicationStartPermanent = fn
}
func (rn *RemoteNode) OnApplicationInfo(fn func(name gen.Atom) (gen.ApplicationInfo, error)) {
	rn.ov.applicationInfo = fn
}

// gen.RemoteNode

func (rn *RemoteNode) Name() gen.Atom {
	if rn.ov.name != nil {
		return rn.ov.name()
	}
	return rn.name
}

func (rn *RemoteNode) Uptime() int64 {
	if rn.ov.uptime != nil {
		return rn.ov.uptime()
	}
	return 0
}

func (rn *RemoteNode) ConnectionUptime() int64 {
	if rn.ov.connectionUptime != nil {
		return rn.ov.connectionUptime()
	}
	return 0
}

func (rn *RemoteNode) Version() gen.Version {
	if rn.ov.version != nil {
		return rn.ov.version()
	}
	return gen.Version{}
}

func (rn *RemoteNode) Info() gen.RemoteNodeInfo {
	if rn.ov.info != nil {
		return rn.ov.info()
	}
	return gen.RemoteNodeInfo{}
}

func (rn *RemoteNode) Spawn(name gen.Atom, options gen.ProcessOptions, args ...any) (gen.PID, error) {
	child, err := synthPID(rn.next.Add(1)), error(nil)
	if rn.ov.spawn != nil {
		child, err = rn.ov.spawn(name, options, args...)
	}
	rn.put(check.RemoteSpawn{Node: rn.name, Name: name, Child: child, Options: options, Error: err})
	return child, err
}

func (rn *RemoteNode) SpawnRegister(register gen.Atom, name gen.Atom, options gen.ProcessOptions, args ...any) (gen.PID, error) {
	child, err := synthPID(rn.next.Add(1)), error(nil)
	if rn.ov.spawnRegister != nil {
		child, err = rn.ov.spawnRegister(register, name, options, args...)
	}
	rn.put(check.RemoteSpawn{Node: rn.name, Name: name, Register: register, Child: child, Options: options, Error: err})
	return child, err
}

func (rn *RemoteNode) ApplicationStart(name gen.Atom, options gen.ApplicationOptions) error {
	var err error
	if rn.ov.applicationStart != nil {
		err = rn.ov.applicationStart(name, options)
	}
	rn.put(check.RemoteApplicationStart{Node: rn.name, Name: name, Error: err})
	return err
}

func (rn *RemoteNode) ApplicationStartTemporary(name gen.Atom, options gen.ApplicationOptions) error {
	var err error
	if rn.ov.applicationStartTemporary != nil {
		err = rn.ov.applicationStartTemporary(name, options)
	}
	rn.put(check.RemoteApplicationStart{Node: rn.name, Name: name, Mode: gen.ApplicationModeTemporary, Error: err})
	return err
}

func (rn *RemoteNode) ApplicationStartTransient(name gen.Atom, options gen.ApplicationOptions) error {
	var err error
	if rn.ov.applicationStartTransient != nil {
		err = rn.ov.applicationStartTransient(name, options)
	}
	rn.put(check.RemoteApplicationStart{Node: rn.name, Name: name, Mode: gen.ApplicationModeTransient, Error: err})
	return err
}

func (rn *RemoteNode) ApplicationStartPermanent(name gen.Atom, options gen.ApplicationOptions) error {
	var err error
	if rn.ov.applicationStartPermanent != nil {
		err = rn.ov.applicationStartPermanent(name, options)
	}
	rn.put(check.RemoteApplicationStart{Node: rn.name, Name: name, Mode: gen.ApplicationModePermanent, Error: err})
	return err
}

func (rn *RemoteNode) ApplicationInfo(name gen.Atom) (gen.ApplicationInfo, error) {
	if rn.ov.applicationInfo != nil {
		return rn.ov.applicationInfo(name)
	}
	return gen.ApplicationInfo{}, nil
}

func (rn *RemoteNode) Creation() int64 {
	if rn.ov.creation != nil {
		return rn.ov.creation()
	}
	return 0
}

func (rn *RemoteNode) Disconnect() {
	if rn.ov.disconnect != nil {
		rn.ov.disconnect()
		return
	}
}
