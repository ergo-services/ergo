package manage

import "ergo.services/ergo/gen"

// operation is one planned mutation. undo is nil when it cannot be reversed.
type operation struct {
	name   string
	target string
	apply  func() any
	undo   func() error
}

func (m *manage) plan(request any) (operation, bool) {
	switch r := request.(type) {

	// delivery and termination: nothing to undo once it happened

	case RequestDoSend:
		return operation{
			name:   CapSend,
			target: r.PID.String(),
			apply: func() any {
				return ResponseDoSend{Error: m.SendWithPriority(r.PID, r.Message, r.Priority)}
			},
		}, true

	case RequestDoSendMeta:
		return operation{
			name:   CapSendMeta,
			target: r.Meta.String(),
			apply: func() any {
				return ResponseDoSendMeta{Error: m.SendAlias(r.Meta, r.Message)}
			},
		}, true

	case RequestDoSendExit:
		return operation{
			name:   CapSendExit,
			target: r.PID.String(),
			apply: func() any {
				return ResponseDoSendExit{Error: m.SendExit(r.PID, r.Reason)}
			},
		}, true

	case RequestDoSendExitMeta:
		return operation{
			name:   CapSendExitMeta,
			target: r.Meta.String(),
			apply: func() any {
				return ResponseDoSendExitMeta{Error: m.SendExitMeta(r.Meta, r.Reason)}
			},
		}, true

	case RequestDoKill:
		return operation{
			name:   CapKill,
			target: r.PID.String(),
			apply: func() any {
				return ResponseDoKill{Error: m.Node().Kill(r.PID)}
			},
		}, true

	// log levels: the previous level is readable, so these are reversible

	case RequestDoSetLogLevel:
		previous := m.Node().Log().Level()
		return m.logLevelOp(CapSetLogLevel, string(m.Node().Name()),
			func() error { return m.Node().Log().SetLevel(r.Level) },
			func() error { return m.Node().Log().SetLevel(previous) },
		), true

	case RequestDoSetProcessLogLevel:
		var undo func() error
		if info, found := m.processInfo(r.PID); found {
			previous := info.LogLevel
			undo = func() error { return m.Node().SetProcessLogLevel(r.PID, previous) }
		}
		return m.logLevelOp(CapSetProcessLogLevel, r.PID.String(),
			func() error { return m.Node().SetProcessLogLevel(r.PID, r.Level) },
			undo,
		), true

	case RequestDoSetMetaLogLevel:
		var undo func() error
		if info, err := m.Node().MetaInfo(r.Meta); err == nil {
			previous := info.LogLevel
			undo = func() error { return m.Node().SetMetaLogLevel(r.Meta, previous) }
		}
		return m.logLevelOp(CapSetMetaLogLevel, r.Meta.String(),
			func() error { return m.Node().SetMetaLogLevel(r.Meta, r.Level) },
			undo,
		), true

	// tracing samplers

	case RequestDoSetNodeTracingSampler:
		previous := m.Node().TracingSampler()
		return m.settingOp(CapSetNodeTracingSampler, string(m.Node().Name()),
			func() error { return m.Node().SetTracingSampler(makeSampler(r.Type, r.Rate, r.Limit)) },
			func() error { return m.Node().SetTracingSampler(previous) },
		), true

	case RequestDoSetProcessTracingSampler:
		// no getter for the per-process sampler, so this one cannot be restored
		return m.settingOp(CapSetProcessTracingSampler, r.PID.String(),
			func() error {
				return m.Node().SetProcessTracingSampler(r.PID, makeSampler(r.Type, r.Rate, r.Limit))
			},
			nil,
		), true

	// process settings: every previous value comes from ProcessInfo

	case RequestDoSetProcessSendPriority:
		var undo func() error
		if info, found := m.processInfo(r.PID); found {
			previous := info.MessagePriority
			undo = func() error { return m.Node().SetProcessSendPriority(r.PID, previous) }
		}
		return m.settingOp(CapSetProcessSendPriority, r.PID.String(),
			func() error { return m.Node().SetProcessSendPriority(r.PID, r.Priority) },
			undo,
		), true

	case RequestDoSetProcessCompression:
		var undo func() error
		if info, found := m.processInfo(r.PID); found {
			previous := info.Compression.Enable
			undo = func() error { return m.Node().SetProcessCompression(r.PID, previous) }
		}
		return m.settingOp(CapSetProcessCompression, r.PID.String(),
			func() error { return m.Node().SetProcessCompression(r.PID, r.Enabled) },
			undo,
		), true

	case RequestDoSetProcessCompressionType:
		var undo func() error
		if info, found := m.processInfo(r.PID); found {
			previous := info.Compression.Type
			undo = func() error { return m.Node().SetProcessCompressionType(r.PID, previous) }
		}
		return m.settingOp(CapSetProcessCompressionType, r.PID.String(),
			func() error { return m.Node().SetProcessCompressionType(r.PID, r.Type) },
			undo,
		), true

	case RequestDoSetProcessCompressionLevel:
		var undo func() error
		if info, found := m.processInfo(r.PID); found {
			previous := info.Compression.Level
			undo = func() error { return m.Node().SetProcessCompressionLevel(r.PID, previous) }
		}
		return m.settingOp(CapSetProcessCompressionLevel, r.PID.String(),
			func() error { return m.Node().SetProcessCompressionLevel(r.PID, r.Level) },
			undo,
		), true

	case RequestDoSetProcessCompressionThreshold:
		var undo func() error
		if info, found := m.processInfo(r.PID); found {
			previous := info.Compression.Threshold
			undo = func() error { return m.Node().SetProcessCompressionThreshold(r.PID, previous) }
		}
		return m.settingOp(CapSetProcessCompressionThreshold, r.PID.String(),
			func() error { return m.Node().SetProcessCompressionThreshold(r.PID, r.Threshold) },
			undo,
		), true

	case RequestDoSetProcessKeepNetworkOrder:
		var undo func() error
		if info, found := m.processInfo(r.PID); found {
			previous := info.KeepNetworkOrder
			undo = func() error { return m.Node().SetProcessKeepNetworkOrder(r.PID, previous) }
		}
		return m.settingOp(CapSetProcessKeepNetworkOrder, r.PID.String(),
			func() error { return m.Node().SetProcessKeepNetworkOrder(r.PID, r.Order) },
			undo,
		), true

	case RequestDoSetProcessImportantDelivery:
		var undo func() error
		if info, found := m.processInfo(r.PID); found {
			previous := info.ImportantDelivery
			undo = func() error { return m.Node().SetProcessImportantDelivery(r.PID, previous) }
		}
		return m.settingOp(CapSetProcessImportantDelivery, r.PID.String(),
			func() error { return m.Node().SetProcessImportantDelivery(r.PID, r.Important) },
			undo,
		), true

	// meta settings

	case RequestDoSetMetaSendPriority:
		var undo func() error
		if info, err := m.Node().MetaInfo(r.Meta); err == nil {
			previous := info.MessagePriority
			undo = func() error { return m.Node().SetMetaSendPriority(r.Meta, previous) }
		}
		return m.settingOp(CapSetMetaSendPriority, r.Meta.String(),
			func() error { return m.Node().SetMetaSendPriority(r.Meta, r.Priority) },
			undo,
		), true

	// application lifecycle: not reversed, a lost response is reported instead

	case RequestDoAppStart:
		return operation{
			name:   CapAppStart,
			target: string(r.Name),
			apply: func() any {
				return ResponseDoAppStart{Error: m.appStart(r)}
			},
		}, true

	case RequestDoAppStop:
		return operation{
			name:   CapAppStop,
			target: string(r.Name),
			apply: func() any {
				if r.Force {
					return ResponseDoAppStop{Error: m.Node().ApplicationStopForce(r.Name)}
				}
				return ResponseDoAppStop{Error: m.Node().ApplicationStop(r.Name)}
			},
		}, true

	case RequestDoAppUnload:
		return operation{
			name:   CapAppUnload,
			target: string(r.Name),
			apply: func() any {
				return ResponseDoAppUnload{Error: m.Node().ApplicationUnload(r.Name)}
			},
		}, true
	}

	return operation{}, false
}

func (m *manage) appStart(r RequestDoAppStart) error {
	opts := gen.ApplicationOptions{}
	switch r.Mode {
	case gen.ApplicationModeTemporary:
		return m.Node().ApplicationStartTemporary(r.Name, opts)
	case gen.ApplicationModeTransient:
		return m.Node().ApplicationStartTransient(r.Name, opts)
	case gen.ApplicationModePermanent:
		return m.Node().ApplicationStartPermanent(r.Name, opts)
	}
	return m.Node().ApplicationStart(r.Name, opts)
}

func (m *manage) settingOp(name, target string, set func() error, undo func() error) operation {
	return operation{
		name:   name,
		target: target,
		apply:  func() any { return ResponseDoSet{Error: set()} },
		undo:   undo,
	}
}

func (m *manage) logLevelOp(name, target string, set func() error, undo func() error) operation {
	return operation{
		name:   name,
		target: target,
		apply:  func() any { return ResponseDoSetLogLevel{Error: set()} },
		undo:   undo,
	}
}

// processInfo reads current state; absent means no rollback for this operation.
func (m *manage) processInfo(pid gen.PID) (gen.ProcessInfo, bool) {
	info, err := m.Node().ProcessInfo(pid)
	if err != nil {
		return gen.ProcessInfo{}, false
	}
	return info, true
}

func makeSampler(typ string, rate float64, limit int) gen.TracingSampler {
	switch typ {
	case "always":
		return gen.TracingSamplerAlways
	case "ratio":
		return gen.TracingSamplerRatio(rate)
	case "rate_limit":
		return gen.TracingSamplerRateLimit(limit)
	}
	return gen.TracingSamplerDisable
}
