package mock

import (
	"sync/atomic"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
)

// Meta is a standalone gen.MetaProcess mock. Every method has an On<Method> override;
// unset, Send/SendWithPriority record a check.Send, SendResponse/SendResponseError a
// check.SendResponse, Spawn a check.SpawnMeta, and the accessors return safe defaults.
type Meta struct {
	recorder
	parent      gen.PID
	id          gen.Alias
	priority    gen.MessagePriority
	compression bool
	log         *Log
	next        atomic.Uint64
	ov          metaOverrides
}

type metaOverrides struct {
	id                func() gen.Alias
	parent            func() gen.PID
	send              func(to any, message any) error
	sendWithPriority  func(to any, message any, priority gen.MessagePriority) error
	sendResponse      func(to gen.PID, ref gen.Ref, message any) error
	sendResponseError func(to gen.PID, ref gen.Ref, err error) error
	spawn             func(behavior gen.MetaBehavior, options gen.MetaOptions) (gen.Alias, error)
	sendPriority      func() gen.MessagePriority
	setSendPriority   func(priority gen.MessagePriority) error
	env               func(name gen.Env) (any, bool)
	envList           func() map[gen.Env]any
	envDefault        func(name gen.Env, def any) any
	log               func() gen.Log
	compression       func() bool
	setCompression    func(enabled bool) error
}

var _ gen.MetaProcess = (*Meta)(nil)

// NewMeta returns a dumb gen.MetaProcess mock (no recording; use NewMetaT for Should*).
func NewMeta() *Meta { return newMeta(recorder{}) }

// NewMetaT returns a gen.MetaProcess mock that records egress and asserts through t.
func NewMetaT(t check.T) *Meta { return newMeta(newRecorder(t)) }

func newMeta(r recorder) *Meta {
	m := &Meta{
		recorder: r,
		parent:   synthPID(1000),
		id:       synthAlias(1),
	}
	m.log = newLog(r)
	return m
}

// On<Method> overrides

func (m *Meta) OnID(fn func() gen.Alias)                  { m.ov.id = fn }
func (m *Meta) OnParent(fn func() gen.PID)                { m.ov.parent = fn }
func (m *Meta) OnSend(fn func(to any, message any) error) { m.ov.send = fn }
func (m *Meta) OnSendWithPriority(fn func(to any, message any, priority gen.MessagePriority) error) {
	m.ov.sendWithPriority = fn
}
func (m *Meta) OnSendResponse(fn func(to gen.PID, ref gen.Ref, message any) error) {
	m.ov.sendResponse = fn
}
func (m *Meta) OnSendResponseError(fn func(to gen.PID, ref gen.Ref, err error) error) {
	m.ov.sendResponseError = fn
}
func (m *Meta) OnSpawn(fn func(behavior gen.MetaBehavior, options gen.MetaOptions) (gen.Alias, error)) {
	m.ov.spawn = fn
}
func (m *Meta) OnSendPriority(fn func() gen.MessagePriority) { m.ov.sendPriority = fn }
func (m *Meta) OnSetSendPriority(fn func(priority gen.MessagePriority) error) {
	m.ov.setSendPriority = fn
}
func (m *Meta) OnEnv(fn func(name gen.Env) (any, bool))         { m.ov.env = fn }
func (m *Meta) OnEnvList(fn func() map[gen.Env]any)             { m.ov.envList = fn }
func (m *Meta) OnEnvDefault(fn func(name gen.Env, def any) any) { m.ov.envDefault = fn }
func (m *Meta) OnLog(fn func() gen.Log)                         { m.ov.log = fn }
func (m *Meta) OnCompression(fn func() bool)                    { m.ov.compression = fn }
func (m *Meta) OnSetCompression(fn func(enabled bool) error)    { m.ov.setCompression = fn }

// gen.MetaProcess

func (m *Meta) ID() gen.Alias {
	if m.ov.id != nil {
		return m.ov.id()
	}
	return m.id
}

func (m *Meta) Parent() gen.PID {
	if m.ov.parent != nil {
		return m.ov.parent()
	}
	return m.parent
}

func (m *Meta) Send(to any, message any) error {
	var err error
	if m.ov.send != nil {
		err = m.ov.send(to, message)
	}
	m.put(check.Send{From: m.parent, To: to, Message: message, Error: err})
	return err
}

func (m *Meta) SendWithPriority(to any, message any, priority gen.MessagePriority) error {
	var err error
	if m.ov.sendWithPriority != nil {
		err = m.ov.sendWithPriority(to, message, priority)
	}
	m.put(check.Send{From: m.parent, To: to, Message: message, Options: gen.MessageOptions{Priority: priority}, Error: err})
	return err
}

func (m *Meta) SendResponse(to gen.PID, ref gen.Ref, message any) error {
	var err error
	if m.ov.sendResponse != nil {
		err = m.ov.sendResponse(to, ref, message)
	}
	m.put(check.SendResponse{From: m.parent, To: to, Ref: ref, Message: message, Error: err})
	return err
}

func (m *Meta) SendResponseError(to gen.PID, ref gen.Ref, err error) error {
	var rerr error
	if m.ov.sendResponseError != nil {
		rerr = m.ov.sendResponseError(to, ref, err)
	}
	m.put(check.SendResponse{From: m.parent, To: to, Ref: ref, Message: err, Error: rerr})
	return rerr
}

func (m *Meta) Spawn(behavior gen.MetaBehavior, options gen.MetaOptions) (gen.Alias, error) {
	alias, err := synthAlias(m.next.Add(1)), error(nil)
	if m.ov.spawn != nil {
		alias, err = m.ov.spawn(behavior, options)
	}
	m.put(check.SpawnMeta{Parent: m.parent, Alias: alias, Error: err})
	return alias, err
}

func (m *Meta) SendPriority() gen.MessagePriority {
	if m.ov.sendPriority != nil {
		return m.ov.sendPriority()
	}
	return m.priority
}

func (m *Meta) SetSendPriority(priority gen.MessagePriority) error {
	if m.ov.setSendPriority != nil {
		return m.ov.setSendPriority(priority)
	}
	m.priority = priority
	return nil
}

func (m *Meta) Env(name gen.Env) (any, bool) {
	if m.ov.env != nil {
		return m.ov.env(name)
	}
	return nil, false
}

func (m *Meta) EnvList() map[gen.Env]any {
	if m.ov.envList != nil {
		return m.ov.envList()
	}
	return nil
}

func (m *Meta) EnvDefault(name gen.Env, def any) any {
	if m.ov.envDefault != nil {
		return m.ov.envDefault(name, def)
	}
	return def
}

func (m *Meta) Log() gen.Log {
	if m.ov.log != nil {
		return m.ov.log()
	}
	return m.log
}

func (m *Meta) Compression() bool {
	if m.ov.compression != nil {
		return m.ov.compression()
	}
	return m.compression
}

func (m *Meta) SetCompression(enabled bool) error {
	if m.ov.setCompression != nil {
		return m.ov.setCompression(enabled)
	}
	m.compression = enabled
	return nil
}
