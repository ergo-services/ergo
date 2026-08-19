package manage

import "ergo.services/ergo/gen"

// Wire types of the mutating plane. Everything here changes the node state.
// Read-only requests live in the inspect package.

// send

type RequestDoSend struct {
	PID      gen.PID
	Priority gen.MessagePriority
	Message  any
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

// send exit

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

// kill

type RequestDoKill struct {
	PID gen.PID
}
type ResponseDoKill struct {
	Error error
}

// log level

type RequestDoSetLogLevel struct {
	Level gen.LogLevel
}
type RequestDoSetProcessLogLevel struct {
	PID   gen.PID
	Level gen.LogLevel
}
type RequestDoSetMetaLogLevel struct {
	Meta  gen.Alias
	Level gen.LogLevel
}
type ResponseDoSetLogLevel struct {
	Error error
}

// tracing sampler

type RequestDoSetNodeTracingSampler struct {
	Type  string  // "always", "disable", "ratio", "rate_limit"
	Rate  float64 // for ratio
	Limit int     // for rate_limit
}

type RequestDoSetProcessTracingSampler struct {
	PID   gen.PID
	Type  string
	Rate  float64
	Limit int
}

// process settings

type RequestDoSetProcessSendPriority struct {
	PID      gen.PID
	Priority gen.MessagePriority
}

type RequestDoSetProcessCompression struct {
	PID     gen.PID
	Enabled bool
}

type RequestDoSetProcessCompressionType struct {
	PID  gen.PID
	Type gen.CompressionType
}

type RequestDoSetProcessCompressionLevel struct {
	PID   gen.PID
	Level gen.CompressionLevel
}

type RequestDoSetProcessCompressionThreshold struct {
	PID       gen.PID
	Threshold int
}

type RequestDoSetProcessKeepNetworkOrder struct {
	PID   gen.PID
	Order bool
}

type RequestDoSetProcessImportantDelivery struct {
	PID       gen.PID
	Important bool
}

// meta settings

type RequestDoSetMetaSendPriority struct {
	Meta     gen.Alias
	Priority gen.MessagePriority
}

// generic response for the set operations
type ResponseDoSet struct {
	Error error
}

// application lifecycle

type RequestDoAppStart struct {
	Name gen.Atom
	Mode gen.ApplicationMode
}
type ResponseDoAppStart struct {
	Error error
}

type RequestDoAppStop struct {
	Name  gen.Atom
	Force bool
}
type ResponseDoAppStop struct {
	Error error
}

type RequestDoAppUnload struct {
	Name gen.Atom
}
type ResponseDoAppUnload struct {
	Error error
}

// Types returns the wire-format types of the mutating plane for use in
// gen.ApplicationSpec.Network.RegisterTypes.
func Types() []any {
	return []any{
		RequestDoSend{}, ResponseDoSend{},
		RequestDoSendMeta{}, ResponseDoSendMeta{},
		RequestDoSendExit{}, ResponseDoSendExit{},
		RequestDoSendExitMeta{}, ResponseDoSendExitMeta{},
		RequestDoKill{}, ResponseDoKill{},
		RequestDoSetLogLevel{}, RequestDoSetProcessLogLevel{},
		RequestDoSetMetaLogLevel{}, ResponseDoSetLogLevel{},
		RequestDoSetNodeTracingSampler{}, RequestDoSetProcessTracingSampler{},
		RequestDoSetProcessSendPriority{}, RequestDoSetProcessCompression{},
		RequestDoSetProcessCompressionType{}, RequestDoSetProcessCompressionLevel{},
		RequestDoSetProcessCompressionThreshold{}, RequestDoSetProcessKeepNetworkOrder{},
		RequestDoSetProcessImportantDelivery{}, RequestDoSetMetaSendPriority{},
		ResponseDoSet{},
		RequestDoAppStart{}, ResponseDoAppStart{},
		RequestDoAppStop{}, ResponseDoAppStop{},
		RequestDoAppUnload{}, ResponseDoAppUnload{},
	}
}
