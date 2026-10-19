package edf

import (
	"fmt"
	"reflect"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/meta"
)

var (
	// register generic Ergo Framework types for the networking
	genTypes = []any{

		gen.Env(""),
		gen.LogLevel(0),
		gen.ProcessState(0),
		gen.ProcessKind(""),
		gen.MetaState(0),
		gen.NetworkMode(0),
		gen.MessagePriority(0),
		gen.CompressionType(""),
		gen.CompressionLevel(0),
		gen.ApplicationMode(0),
		gen.ApplicationState(0),

		gen.Version{},

		gen.ApplicationDepends{},

		gen.Tracing{},
		gen.TracingFlags(0),
		gen.TracingAttribute{},
		gen.TracingInfo{},
		gen.TracingExporterInfo{},

		gen.LoggerInfo{},
		gen.ProcessFallback{},
		gen.CronJobInfo{},
		gen.CronInfo{},
		gen.CronSchedule{},
		gen.NodeInfo{},
		gen.RemoteNodeShortInfo{},
		gen.NodeShortInfo{},

		gen.Compression{},
		gen.MailboxQueues{},
		gen.ProcessInfo{},
		gen.ProcessShortInfo{},
		gen.ProcessOptions{},
		gen.ProcessOptionsExtra{},
		gen.ApplicationOptions{},
		gen.ApplicationOptionsExtra{},
		gen.ApplicationInfo{},
		gen.MetaInfo{},
		gen.EventInfo{},
		gen.LogField{},

		gen.Error{},

		gen.NetworkFlags{},
		gen.NetworkProxyFlags{},
		gen.NetworkSpawnInfo{},
		gen.NetworkApplicationStartInfo{},
		gen.RemoteNodeInfo{},
		gen.RouteInfo{},
		gen.ProxyRouteInfo{},
		gen.Route{},
		gen.ApplicationRoute{},
		gen.ProxyRoute{},
		gen.RegisterRoutes{},
		gen.RegistrarInfo{},
		gen.AcceptorInfo{},
		gen.NetworkInfo{},
		gen.MessageEvent{},
		gen.MessageEventStart{},
		gen.MessageEventStop{},

		gen.MessageRegistrarNodeJoined{},
		gen.MessageRegistrarNodeLeft{},
		gen.MessageRegistrarConfigUpdate{},
		gen.MessageRegistrarApplicationLoaded{},
		gen.MessageRegistrarApplicationInitializing{},
		gen.MessageRegistrarApplicationStarted{},
		gen.MessageRegistrarApplicationStopping{},
		gen.MessageRegistrarApplicationUnloaded{},
		gen.MessageRegistrarProxyRegistered{},
		gen.MessageRegistrarProxyUnregistered{},

		gen.MessageCoreApplicationStarted{},
		gen.MessageCoreApplicationStopped{},
		gen.MessageCoreNodeConnected{},
		gen.MessageCoreNodeDisconnected{},

		gen.TracingPoint(0),
		gen.TracingKind(0),
		gen.TracingFlags(0),
		gen.TracingSpan{},

		meta.MessageTCPConnect{},
		meta.MessageTCPDisconnect{},
		meta.MessageTCP{},
		meta.MessageUDP{},

		gen.RegisteredTypeStats{},
		gen.RegisteredTypeInfo{},
		gen.RegisteredErrorInfo{},
		gen.RegisteredAtomInfo{},
	}

	// register standard errors of the Ergo Framework. Identity of these
	// sentinels is preserved across the network via the EDF errCache: each
	// peer assigns its local instance a uint16 id during handshake and maps
	// remote ids back to its own instance by Error() string match. So
	// errors.Is(reason, gen.ErrTimeout) keeps working after a network hop.
	genErrors = []error{
		gen.ErrNameUnknown,
		gen.ErrParentUnknown,
		gen.ErrNodeTerminated,

		gen.ErrProcessMailboxFull,
		gen.ErrProcessUnknown,
		gen.ErrProcessIncarnation,
		gen.ErrProcessTerminated,

		gen.ErrMetaUnknown,
		gen.ErrMetaMailboxFull,

		gen.ErrApplicationUnknown,
		gen.ErrApplicationDepends,
		gen.ErrApplicationState,
		gen.ErrApplicationLoadPanic,
		gen.ErrApplicationEmpty,
		gen.ErrApplicationName,
		gen.ErrApplicationStopping,
		gen.ErrApplicationRunning,

		gen.ErrTargetUnknown,
		gen.ErrTargetExist,
		gen.ErrTargetManagerOverload,

		gen.ErrRegistrarTerminated,

		gen.ErrAliasUnknown,
		gen.ErrAliasOwner,
		gen.ErrEventUnknown,
		gen.ErrEventOwner,
		gen.ErrTaken,

		gen.ErrAtomTooLong,
		gen.ErrAtomLimit,

		gen.ErrTimeout,
		gen.ErrCanceled,
		gen.ErrUnsupported,
		gen.ErrUnknown,
		gen.ErrNotAllowed,
		gen.ErrDiscarded,
		gen.ErrDisabled,
		gen.ErrBusy,
		gen.ErrExceeded,

		gen.ErrIncorrect,
		gen.ErrMalformed,
		gen.ErrResponseIgnored,
		gen.ErrUnregistered,
		gen.ErrTooLarge,

		gen.ErrNetworkStopped,
		gen.ErrNoConnection,
		gen.ErrNoRoute,

		gen.ErrInternal,

		gen.TerminateReasonNormal,
		gen.TerminateReasonShutdown,
		gen.TerminateReasonKill,
		gen.TerminateReasonPanic,
	}
)

func init() {
	defaultRegistry = NewRegistry()
}

func (r *Registry) registerBuiltins() {
	// For each built-in type: encoder/decoder are stored first so that
	// registerInfo (which calls measureZeroSize -> Encode) can resolve them.
	// The resulting *RegisteredTypeInfo is then attached back to the encoder
	// and decoder via the Info field used by encodeWithStats/decodeWithStats.

	pidType := reflect.TypeOf(gen.PID{})
	pidEnc := &encoder{Prefix: []byte{edtPID}, Encode: encodePID}
	r.encoders.Store(pidType, pidEnc)
	pidDec := &decoder{Type: pidType, Decode: decodePID}
	r.decoders.Store(edtPID, pidDec)
	r.decoders.Store(pidType, pidDec)
	pidInfo := r.registerInfo(pidType, "framework", "gen.PID")
	pidEnc.Info = pidInfo
	pidDec.Info = pidInfo

	processIDType := reflect.TypeOf(gen.ProcessID{})
	processIDEnc := &encoder{Prefix: []byte{edtProcessID}, Encode: encodeProcessID}
	r.encoders.Store(processIDType, processIDEnc)
	processIDDec := &decoder{Type: processIDType, Decode: decodeProcessID}
	r.decoders.Store(edtProcessID, processIDDec)
	r.decoders.Store(processIDType, processIDDec)
	processIDInfo := r.registerInfo(processIDType, "framework", "gen.ProcessID")
	processIDEnc.Info = processIDInfo
	processIDDec.Info = processIDInfo

	refType := reflect.TypeOf(gen.Ref{})
	refEnc := &encoder{Prefix: []byte{edtRef}, Encode: encodeRef}
	r.encoders.Store(refType, refEnc)
	refDec := &decoder{Type: refType, Decode: decodeRef}
	r.decoders.Store(edtRef, refDec)
	r.decoders.Store(refType, refDec)
	refInfo := r.registerInfo(refType, "framework", "gen.Ref")
	refEnc.Info = refInfo
	refDec.Info = refInfo

	aliasType := reflect.TypeOf(gen.Alias{})
	aliasEnc := &encoder{Prefix: []byte{edtAlias}, Encode: encodeAlias}
	r.encoders.Store(aliasType, aliasEnc)
	aliasDec := &decoder{Type: aliasType, Decode: decodeAlias}
	r.decoders.Store(edtAlias, aliasDec)
	r.decoders.Store(aliasType, aliasDec)
	aliasInfo := r.registerInfo(aliasType, "framework", "gen.Alias")
	aliasEnc.Info = aliasInfo
	aliasDec.Info = aliasInfo

	eventType := reflect.TypeOf(gen.Event{})
	eventEnc := &encoder{Prefix: []byte{edtEvent}, Encode: encodeEvent}
	r.encoders.Store(eventType, eventEnc)
	eventDec := &decoder{Type: eventType, Decode: decodeEvent}
	r.decoders.Store(edtEvent, eventDec)
	r.decoders.Store(eventType, eventDec)
	eventInfo := r.registerInfo(eventType, "framework", "gen.Event")
	eventEnc.Info = eventInfo
	eventDec.Info = eventInfo

	timeType := reflect.TypeOf(time.Time{})
	timeEnc := &encoder{Prefix: []byte{edtTime}, Encode: encodeTime}
	r.encoders.Store(timeType, timeEnc)
	timeDec := &decoder{Type: timeType, Decode: decodeTime}
	r.decoders.Store(edtTime, timeDec)
	r.decoders.Store(timeType, timeDec)
	timeInfo := r.registerInfo(timeType, "framework", "time.Time")
	timeEnc.Info = timeInfo
	timeDec.Info = timeInfo

	boolType := reflect.TypeOf(true)
	boolEnc := &encoder{Prefix: []byte{edtBool}, Encode: encodeBool}
	r.encoders.Store(boolType, boolEnc)
	boolDec := &decoder{Type: boolType, Decode: decodeBool}
	r.decoders.Store(edtBool, boolDec)
	r.decoders.Store(boolType, boolDec)
	boolInfo := r.registerInfo(boolType, "bool", "bool")
	boolEnc.Info = boolInfo
	boolDec.Info = boolInfo

	atomType := reflect.TypeOf(gen.Atom("atom"))
	atomEnc := &encoder{Prefix: []byte{edtAtom}, Encode: encodeAtom}
	r.encoders.Store(atomType, atomEnc)
	atomDec := &decoder{Type: atomType, Decode: decodeAtom}
	r.decoders.Store(edtAtom, atomDec)
	r.decoders.Store(atomType, atomDec)
	atomInfo := r.registerInfo(atomType, "framework", "gen.Atom")
	atomEnc.Info = atomInfo
	atomDec.Info = atomInfo

	stringType := reflect.TypeOf("string")
	stringEnc := &encoder{Prefix: []byte{edtString}, Encode: encodeString}
	r.encoders.Store(stringType, stringEnc)
	stringDec := &decoder{Type: stringType, Decode: decodeString}
	r.decoders.Store(edtString, stringDec)
	r.decoders.Store(stringType, stringDec)
	stringInfo := r.registerInfo(stringType, "string", "string")
	stringEnc.Info = stringInfo
	stringDec.Info = stringInfo

	intType := reflect.TypeOf(int(0))
	intEnc := &encoder{Prefix: []byte{edtInt}, Encode: encodeInt}
	r.encoders.Store(intType, intEnc)
	intDec := &decoder{Type: intType, Decode: decodeInt}
	r.decoders.Store(edtInt, intDec)
	r.decoders.Store(intType, intDec)
	intInfo := r.registerInfo(intType, "int", "int")
	intEnc.Info = intInfo
	intDec.Info = intInfo

	int8Type := reflect.TypeOf(int8(0))
	int8Enc := &encoder{Prefix: []byte{edtInt8}, Encode: encodeInt8}
	r.encoders.Store(int8Type, int8Enc)
	int8Dec := &decoder{Type: int8Type, Decode: decodeInt8}
	r.decoders.Store(edtInt8, int8Dec)
	r.decoders.Store(int8Type, int8Dec)
	int8Info := r.registerInfo(int8Type, "int8", "int8")
	int8Enc.Info = int8Info
	int8Dec.Info = int8Info

	int16Type := reflect.TypeOf(int16(0))
	int16Enc := &encoder{Prefix: []byte{edtInt16}, Encode: encodeInt16}
	r.encoders.Store(int16Type, int16Enc)
	int16Dec := &decoder{Type: int16Type, Decode: decodeInt16}
	r.decoders.Store(edtInt16, int16Dec)
	r.decoders.Store(int16Type, int16Dec)
	int16Info := r.registerInfo(int16Type, "int16", "int16")
	int16Enc.Info = int16Info
	int16Dec.Info = int16Info

	int32Type := reflect.TypeOf(int32(0))
	int32Enc := &encoder{Prefix: []byte{edtInt32}, Encode: encodeInt32}
	r.encoders.Store(int32Type, int32Enc)
	int32Dec := &decoder{Type: int32Type, Decode: decodeInt32}
	r.decoders.Store(edtInt32, int32Dec)
	r.decoders.Store(int32Type, int32Dec)
	int32Info := r.registerInfo(int32Type, "int32", "int32")
	int32Enc.Info = int32Info
	int32Dec.Info = int32Info

	int64Type := reflect.TypeOf(int64(0))
	int64Enc := &encoder{Prefix: []byte{edtInt64}, Encode: encodeInt64}
	r.encoders.Store(int64Type, int64Enc)
	int64Dec := &decoder{Type: int64Type, Decode: decodeInt64}
	r.decoders.Store(edtInt64, int64Dec)
	r.decoders.Store(int64Type, int64Dec)
	int64Info := r.registerInfo(int64Type, "int64", "int64")
	int64Enc.Info = int64Info
	int64Dec.Info = int64Info

	durationType := reflect.TypeOf(time.Duration(0))
	durationEnc := &encoder{Prefix: []byte{edtInt64}, Encode: encodeInt64}
	r.encoders.Store(durationType, durationEnc)
	durationDec := &decoder{Type: durationType, Decode: decodeInt64}
	r.decoders.Store(durationType, durationDec)
	durationInfo := r.registerInfo(durationType, "int64", "int64")
	durationEnc.Info = durationInfo
	durationDec.Info = durationInfo

	uintType := reflect.TypeOf(uint(0))
	uintEnc := &encoder{Prefix: []byte{edtUint}, Encode: encodeUint}
	r.encoders.Store(uintType, uintEnc)
	uintDec := &decoder{Type: uintType, Decode: decodeUint}
	r.decoders.Store(edtUint, uintDec)
	r.decoders.Store(uintType, uintDec)
	uintInfo := r.registerInfo(uintType, "uint", "uint")
	uintEnc.Info = uintInfo
	uintDec.Info = uintInfo

	uint8Type := reflect.TypeOf(uint8(0))
	uint8Enc := &encoder{Prefix: []byte{edtUint8}, Encode: encodeUint8}
	r.encoders.Store(uint8Type, uint8Enc)
	uint8Dec := &decoder{Type: uint8Type, Decode: decodeUint8}
	r.decoders.Store(edtUint8, uint8Dec)
	r.decoders.Store(uint8Type, uint8Dec)
	uint8Info := r.registerInfo(uint8Type, "uint8", "uint8")
	uint8Enc.Info = uint8Info
	uint8Dec.Info = uint8Info

	uint16Type := reflect.TypeOf(uint16(0))
	uint16Enc := &encoder{Prefix: []byte{edtUint16}, Encode: encodeUint16}
	r.encoders.Store(uint16Type, uint16Enc)
	uint16Dec := &decoder{Type: uint16Type, Decode: decodeUint16}
	r.decoders.Store(edtUint16, uint16Dec)
	r.decoders.Store(uint16Type, uint16Dec)
	uint16Info := r.registerInfo(uint16Type, "uint16", "uint16")
	uint16Enc.Info = uint16Info
	uint16Dec.Info = uint16Info

	uint32Type := reflect.TypeOf(uint32(0))
	uint32Enc := &encoder{Prefix: []byte{edtUint32}, Encode: encodeUint32}
	r.encoders.Store(uint32Type, uint32Enc)
	uint32Dec := &decoder{Type: uint32Type, Decode: decodeUint32}
	r.decoders.Store(edtUint32, uint32Dec)
	r.decoders.Store(uint32Type, uint32Dec)
	uint32Info := r.registerInfo(uint32Type, "uint32", "uint32")
	uint32Enc.Info = uint32Info
	uint32Dec.Info = uint32Info

	uint64Type := reflect.TypeOf(uint64(0))
	uint64Enc := &encoder{Prefix: []byte{edtUint64}, Encode: encodeUint64}
	r.encoders.Store(uint64Type, uint64Enc)
	uint64Dec := &decoder{Type: uint64Type, Decode: decodeUint64}
	r.decoders.Store(edtUint64, uint64Dec)
	r.decoders.Store(uint64Type, uint64Dec)
	uint64Info := r.registerInfo(uint64Type, "uint64", "uint64")
	uint64Enc.Info = uint64Info
	uint64Dec.Info = uint64Info

	binaryType := reflect.TypeOf([]byte(nil))
	binaryEnc := &encoder{Prefix: []byte{edtBinary}, Encode: encodeBinary}
	r.encoders.Store(binaryType, binaryEnc)
	binaryDec := &decoder{Type: binaryType, Decode: decodeBinary}
	r.decoders.Store(edtBinary, binaryDec)
	r.decoders.Store(binaryType, binaryDec)
	binaryInfo := r.registerInfo(binaryType, "binary", "[]byte")
	binaryEnc.Info = binaryInfo
	binaryDec.Info = binaryInfo

	float32Type := reflect.TypeOf(float32(0.0))
	float32Enc := &encoder{Prefix: []byte{edtFloat32}, Encode: encodeFloat32}
	r.encoders.Store(float32Type, float32Enc)
	float32Dec := &decoder{Type: float32Type, Decode: decodeFloat32}
	r.decoders.Store(edtFloat32, float32Dec)
	r.decoders.Store(float32Type, float32Dec)
	float32Info := r.registerInfo(float32Type, "float32", "float32")
	float32Enc.Info = float32Info
	float32Dec.Info = float32Info

	float64Type := reflect.TypeOf(float64(0.0))
	float64Enc := &encoder{Prefix: []byte{edtFloat64}, Encode: encodeFloat64}
	r.encoders.Store(float64Type, float64Enc)
	float64Dec := &decoder{Type: float64Type, Decode: decodeFloat64}
	r.decoders.Store(edtFloat64, float64Dec)
	r.decoders.Store(float64Type, float64Dec)
	float64Info := r.registerInfo(float64Type, "float64", "float64")
	float64Enc.Info = float64Info
	float64Dec.Info = float64Info

	anyEnc := &encoder{Prefix: []byte{edtAny}, Encode: encodeAny}
	r.encoders.Store(anyType, anyEnc)
	anyDec := &decoder{Type: anyType, Decode: decodeAny}
	r.decoders.Store(edtAny, anyDec)
	r.decoders.Store(anyType, anyDec)
	anyInfo := r.registerInfo(anyType, "any", "any")
	anyEnc.Info = anyInfo
	anyDec.Info = anyInfo

	// error types: errType, *errors.errorString, *fmt.wrapError. They share
	// the same encoder/decoder and the same RegisteredTypeInfo; counters
	// aggregate across all error concrete types.
	errEnc := &encoder{Prefix: []byte{edtError}, Encode: encodeError}
	r.encoders.Store(errType, errEnc)
	r.encoders.Store(reflect.TypeOf(fmt.Errorf("")), errEnc)
	r.encoders.Store(reflect.TypeOf(fmt.Errorf("%w", nil)), errEnc)
	r.encoders.Store(reflect.TypeOf((*gen.Error)(nil)), errEnc)
	errDec := &decoder{Type: errType, Decode: decodeError}
	r.decoders.Store(edtError, errDec)
	r.decoders.Store(errType, errDec)
	errInfo := r.registerInfo(errType, "error", "error")
	errEnc.Info = errInfo
	errDec.Info = errInfo
}
