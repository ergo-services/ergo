package edf

import (
	"fmt"
	"reflect"
	"time"

	"ergo.services/ergo/gen"
)

var (
	// register generic Ergo Framework types for the networking
	genTypes = []any{

		gen.Env(""),
		gen.LogLevel(0),
		gen.ProcessState(0),
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

		gen.TracingPoint(0),
		gen.TracingKind(0),
		gen.TracingFlags(0),
		gen.TracingSpan{},

		gen.RegisteredTypeStats{},
		gen.RegisteredTypeInfo{},
	}

	// register standard errors of the Ergo Framework
	genErrors = []error{
		gen.ErrIncorrect,
		gen.ErrTimeout,
		gen.ErrUnsupported,
		gen.ErrUnknown,
		gen.ErrNameUnknown,
		gen.ErrNotAllowed,
		gen.ErrProcessUnknown,
		gen.ErrProcessTerminated,
		gen.ErrMetaUnknown,
		gen.ErrApplicationUnknown,
		gen.ErrTaken,
		gen.TerminateReasonNormal,
		gen.TerminateReasonShutdown,
		gen.TerminateReasonKill,
		gen.TerminateReasonPanic,
	}
)

func init() {
	// For each built-in type: encoder/decoder are stored first so that
	// registerInfo (which calls measureZeroSize → Encode) can resolve them.
	// The resulting *RegisteredTypeInfo is then attached back to the encoder
	// and decoder via the Info field used by encodeWithStats/decodeWithStats.

	pidType := reflect.TypeOf(gen.PID{})
	pidEnc := &encoder{Prefix: []byte{edtPID}, Encode: encodePID}
	encoders.Store(pidType, pidEnc)
	pidDec := &decoder{Type: pidType, Decode: decodePID}
	decoders.Store(edtPID, pidDec)
	decoders.Store(pidType, pidDec)
	pidInfo := registerInfo(pidType, "framework", "gen.PID")
	pidEnc.Info = pidInfo
	pidDec.Info = pidInfo

	processIDType := reflect.TypeOf(gen.ProcessID{})
	processIDEnc := &encoder{Prefix: []byte{edtProcessID}, Encode: encodeProcessID}
	encoders.Store(processIDType, processIDEnc)
	processIDDec := &decoder{Type: processIDType, Decode: decodeProcessID}
	decoders.Store(edtProcessID, processIDDec)
	decoders.Store(processIDType, processIDDec)
	processIDInfo := registerInfo(processIDType, "framework", "gen.ProcessID")
	processIDEnc.Info = processIDInfo
	processIDDec.Info = processIDInfo

	refType := reflect.TypeOf(gen.Ref{})
	refEnc := &encoder{Prefix: []byte{edtRef}, Encode: encodeRef}
	encoders.Store(refType, refEnc)
	refDec := &decoder{Type: refType, Decode: decodeRef}
	decoders.Store(edtRef, refDec)
	decoders.Store(refType, refDec)
	refInfo := registerInfo(refType, "framework", "gen.Ref")
	refEnc.Info = refInfo
	refDec.Info = refInfo

	aliasType := reflect.TypeOf(gen.Alias{})
	aliasEnc := &encoder{Prefix: []byte{edtAlias}, Encode: encodeAlias}
	encoders.Store(aliasType, aliasEnc)
	aliasDec := &decoder{Type: aliasType, Decode: decodeAlias}
	decoders.Store(edtAlias, aliasDec)
	decoders.Store(aliasType, aliasDec)
	aliasInfo := registerInfo(aliasType, "framework", "gen.Alias")
	aliasEnc.Info = aliasInfo
	aliasDec.Info = aliasInfo

	eventType := reflect.TypeOf(gen.Event{})
	eventEnc := &encoder{Prefix: []byte{edtEvent}, Encode: encodeEvent}
	encoders.Store(eventType, eventEnc)
	eventDec := &decoder{Type: eventType, Decode: decodeEvent}
	decoders.Store(edtEvent, eventDec)
	decoders.Store(eventType, eventDec)
	eventInfo := registerInfo(eventType, "framework", "gen.Event")
	eventEnc.Info = eventInfo
	eventDec.Info = eventInfo

	timeType := reflect.TypeOf(time.Time{})
	timeEnc := &encoder{Prefix: []byte{edtTime}, Encode: encodeTime}
	encoders.Store(timeType, timeEnc)
	timeDec := &decoder{Type: timeType, Decode: decodeTime}
	decoders.Store(edtTime, timeDec)
	decoders.Store(timeType, timeDec)
	timeInfo := registerInfo(timeType, "framework", "time.Time")
	timeEnc.Info = timeInfo
	timeDec.Info = timeInfo

	boolType := reflect.TypeOf(true)
	boolEnc := &encoder{Prefix: []byte{edtBool}, Encode: encodeBool}
	encoders.Store(boolType, boolEnc)
	boolDec := &decoder{Type: boolType, Decode: decodeBool}
	decoders.Store(edtBool, boolDec)
	decoders.Store(boolType, boolDec)
	boolInfo := registerInfo(boolType, "bool", "bool")
	boolEnc.Info = boolInfo
	boolDec.Info = boolInfo

	atomType := reflect.TypeOf(gen.Atom("atom"))
	atomEnc := &encoder{Prefix: []byte{edtAtom}, Encode: encodeAtom}
	encoders.Store(atomType, atomEnc)
	atomDec := &decoder{Type: atomType, Decode: decodeAtom}
	decoders.Store(edtAtom, atomDec)
	decoders.Store(atomType, atomDec)
	atomInfo := registerInfo(atomType, "framework", "gen.Atom")
	atomEnc.Info = atomInfo
	atomDec.Info = atomInfo

	stringType := reflect.TypeOf("string")
	stringEnc := &encoder{Prefix: []byte{edtString}, Encode: encodeString}
	encoders.Store(stringType, stringEnc)
	stringDec := &decoder{Type: stringType, Decode: decodeString}
	decoders.Store(edtString, stringDec)
	decoders.Store(stringType, stringDec)
	stringInfo := registerInfo(stringType, "string", "string")
	stringEnc.Info = stringInfo
	stringDec.Info = stringInfo

	intType := reflect.TypeOf(int(0))
	intEnc := &encoder{Prefix: []byte{edtInt}, Encode: encodeInt}
	encoders.Store(intType, intEnc)
	intDec := &decoder{Type: intType, Decode: decodeInt}
	decoders.Store(edtInt, intDec)
	decoders.Store(intType, intDec)
	intInfo := registerInfo(intType, "int", "int")
	intEnc.Info = intInfo
	intDec.Info = intInfo

	int8Type := reflect.TypeOf(int8(0))
	int8Enc := &encoder{Prefix: []byte{edtInt8}, Encode: encodeInt8}
	encoders.Store(int8Type, int8Enc)
	int8Dec := &decoder{Type: int8Type, Decode: decodeInt8}
	decoders.Store(edtInt8, int8Dec)
	decoders.Store(int8Type, int8Dec)
	int8Info := registerInfo(int8Type, "int8", "int8")
	int8Enc.Info = int8Info
	int8Dec.Info = int8Info

	int16Type := reflect.TypeOf(int16(0))
	int16Enc := &encoder{Prefix: []byte{edtInt16}, Encode: encodeInt16}
	encoders.Store(int16Type, int16Enc)
	int16Dec := &decoder{Type: int16Type, Decode: decodeInt16}
	decoders.Store(edtInt16, int16Dec)
	decoders.Store(int16Type, int16Dec)
	int16Info := registerInfo(int16Type, "int16", "int16")
	int16Enc.Info = int16Info
	int16Dec.Info = int16Info

	int32Type := reflect.TypeOf(int32(0))
	int32Enc := &encoder{Prefix: []byte{edtInt32}, Encode: encodeInt32}
	encoders.Store(int32Type, int32Enc)
	int32Dec := &decoder{Type: int32Type, Decode: decodeInt32}
	decoders.Store(edtInt32, int32Dec)
	decoders.Store(int32Type, int32Dec)
	int32Info := registerInfo(int32Type, "int32", "int32")
	int32Enc.Info = int32Info
	int32Dec.Info = int32Info

	int64Type := reflect.TypeOf(int64(0))
	int64Enc := &encoder{Prefix: []byte{edtInt64}, Encode: encodeInt64}
	encoders.Store(int64Type, int64Enc)
	int64Dec := &decoder{Type: int64Type, Decode: decodeInt64}
	decoders.Store(edtInt64, int64Dec)
	decoders.Store(int64Type, int64Dec)
	int64Info := registerInfo(int64Type, "int64", "int64")
	int64Enc.Info = int64Info
	int64Dec.Info = int64Info

	uintType := reflect.TypeOf(uint(0))
	uintEnc := &encoder{Prefix: []byte{edtUint}, Encode: encodeUint}
	encoders.Store(uintType, uintEnc)
	uintDec := &decoder{Type: uintType, Decode: decodeUint}
	decoders.Store(edtUint, uintDec)
	decoders.Store(uintType, uintDec)
	uintInfo := registerInfo(uintType, "uint", "uint")
	uintEnc.Info = uintInfo
	uintDec.Info = uintInfo

	uint8Type := reflect.TypeOf(uint8(0))
	uint8Enc := &encoder{Prefix: []byte{edtUint8}, Encode: encodeUint8}
	encoders.Store(uint8Type, uint8Enc)
	uint8Dec := &decoder{Type: uint8Type, Decode: decodeUint8}
	decoders.Store(edtUint8, uint8Dec)
	decoders.Store(uint8Type, uint8Dec)
	uint8Info := registerInfo(uint8Type, "uint8", "uint8")
	uint8Enc.Info = uint8Info
	uint8Dec.Info = uint8Info

	uint16Type := reflect.TypeOf(uint16(0))
	uint16Enc := &encoder{Prefix: []byte{edtUint16}, Encode: encodeUint16}
	encoders.Store(uint16Type, uint16Enc)
	uint16Dec := &decoder{Type: uint16Type, Decode: decodeUint16}
	decoders.Store(edtUint16, uint16Dec)
	decoders.Store(uint16Type, uint16Dec)
	uint16Info := registerInfo(uint16Type, "uint16", "uint16")
	uint16Enc.Info = uint16Info
	uint16Dec.Info = uint16Info

	uint32Type := reflect.TypeOf(uint32(0))
	uint32Enc := &encoder{Prefix: []byte{edtUint32}, Encode: encodeUint32}
	encoders.Store(uint32Type, uint32Enc)
	uint32Dec := &decoder{Type: uint32Type, Decode: decodeUint32}
	decoders.Store(edtUint32, uint32Dec)
	decoders.Store(uint32Type, uint32Dec)
	uint32Info := registerInfo(uint32Type, "uint32", "uint32")
	uint32Enc.Info = uint32Info
	uint32Dec.Info = uint32Info

	uint64Type := reflect.TypeOf(uint64(0))
	uint64Enc := &encoder{Prefix: []byte{edtUint64}, Encode: encodeUint64}
	encoders.Store(uint64Type, uint64Enc)
	uint64Dec := &decoder{Type: uint64Type, Decode: decodeUint64}
	decoders.Store(edtUint64, uint64Dec)
	decoders.Store(uint64Type, uint64Dec)
	uint64Info := registerInfo(uint64Type, "uint64", "uint64")
	uint64Enc.Info = uint64Info
	uint64Dec.Info = uint64Info

	binaryType := reflect.TypeOf([]byte(nil))
	binaryEnc := &encoder{Prefix: []byte{edtBinary}, Encode: encodeBinary}
	encoders.Store(binaryType, binaryEnc)
	binaryDec := &decoder{Type: binaryType, Decode: decodeBinary}
	decoders.Store(edtBinary, binaryDec)
	decoders.Store(binaryType, binaryDec)
	binaryInfo := registerInfo(binaryType, "binary", "[]byte")
	binaryEnc.Info = binaryInfo
	binaryDec.Info = binaryInfo

	float32Type := reflect.TypeOf(float32(0.0))
	float32Enc := &encoder{Prefix: []byte{edtFloat32}, Encode: encodeFloat32}
	encoders.Store(float32Type, float32Enc)
	float32Dec := &decoder{Type: float32Type, Decode: decodeFloat32}
	decoders.Store(edtFloat32, float32Dec)
	decoders.Store(float32Type, float32Dec)
	float32Info := registerInfo(float32Type, "float32", "float32")
	float32Enc.Info = float32Info
	float32Dec.Info = float32Info

	float64Type := reflect.TypeOf(float64(0.0))
	float64Enc := &encoder{Prefix: []byte{edtFloat64}, Encode: encodeFloat64}
	encoders.Store(float64Type, float64Enc)
	float64Dec := &decoder{Type: float64Type, Decode: decodeFloat64}
	decoders.Store(edtFloat64, float64Dec)
	decoders.Store(float64Type, float64Dec)
	float64Info := registerInfo(float64Type, "float64", "float64")
	float64Enc.Info = float64Info
	float64Dec.Info = float64Info

	anyEnc := &encoder{Prefix: []byte{edtAny}, Encode: encodeAny}
	encoders.Store(anyType, anyEnc)
	anyDec := &decoder{Type: anyType, Decode: decodeAny}
	decoders.Store(edtAny, anyDec)
	decoders.Store(anyType, anyDec)
	anyInfo := registerInfo(anyType, "any", "any")
	anyEnc.Info = anyInfo
	anyDec.Info = anyInfo

	// error types: errType, *errors.errorString, *fmt.wrapError. They share
	// the same encoder/decoder and the same RegisteredTypeInfo; counters
	// aggregate across all error concrete types.
	errEnc := &encoder{Prefix: []byte{edtError}, Encode: encodeError}
	encoders.Store(errType, errEnc)
	encoders.Store(reflect.TypeOf(fmt.Errorf("")), errEnc)
	encoders.Store(reflect.TypeOf(fmt.Errorf("%w", nil)), errEnc)
	errDec := &decoder{Type: errType, Decode: decodeError}
	decoders.Store(edtError, errDec)
	decoders.Store(errType, errDec)
	errInfo := registerInfo(errType, "error", "error")
	errEnc.Info = errInfo
	errDec.Info = errInfo

	for _, t := range genTypes {
		err := RegisterTypeOf(t)
		if err == nil || err == gen.ErrTaken {
			continue
		}
		panic(err)
	}

	for _, e := range genErrors {
		err := RegisterError(e)
		if err == nil || err == gen.ErrTaken {
			continue
		}
		panic(err)
	}
}
