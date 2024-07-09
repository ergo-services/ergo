package edf

import (
	"fmt"
	"reflect"
	"time"

	"ergo.services/ergo/app/system/inspect"
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

		gen.Version{},

		gen.ApplicationDepends{},

		gen.LoggerInfo{},
		gen.NodeInfo{},
		gen.Compression{},
		gen.ProcessFallback{},
		gen.MailboxQueues{},
		gen.ProcessInfo{},
		gen.ProcessShortInfo{},
		gen.ProcessOptions{},
		gen.ProcessOptionsExtra{},
		gen.ApplicationOptions{},
		gen.ApplicationOptionsExtra{},
		gen.MetaInfo{},

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

		// inspector messages

		inspect.RequestInspectNode{},
		inspect.ResponseInspectNode{},
		inspect.MessageInspectNode{},

		inspect.RequestInspectNetwork{},
		inspect.ResponseInspectNetwork{},
		inspect.MessageInspectNetwork{},

		inspect.RequestInspectConnection{},
		inspect.ResponseInspectConnection{},
		inspect.MessageInspectConnection{},

		inspect.RequestInspectProcessList{},
		inspect.ResponseInspectProcessList{},
		inspect.MessageInspectProcessList{},

		inspect.RequestInspectLog{},
		inspect.ResponseInspectLog{},
		inspect.MessageInspectLogNode{},
		inspect.MessageInspectLogNetwork{},
		inspect.MessageInspectLogProcess{},
		inspect.MessageInspectLogMeta{},

		inspect.RequestInspectProcess{},
		inspect.ResponseInspectProcess{},
		inspect.MessageInspectProcess{},

		inspect.RequestInspectProcessState{},
		inspect.ResponseInspectProcessState{},
		inspect.MessageInspectProcessState{},

		inspect.RequestInspectMeta{},
		inspect.ResponseInspectMeta{},
		inspect.MessageInspectMeta{},

		inspect.RequestInspectMetaState{},
		inspect.ResponseInspectMetaState{},
		inspect.MessageInspectMetaState{},

		inspect.RequestDoSend{},
		inspect.ResponseDoSend{},

		inspect.RequestDoSendMeta{},
		inspect.ResponseDoSendMeta{},

		inspect.RequestDoSendExit{},
		inspect.ResponseDoSendExit{},

		inspect.RequestDoSendExitMeta{},
		inspect.ResponseDoSendExitMeta{},

		inspect.RequestDoKill{},
		inspect.ResponseDoKill{},

		inspect.RequestDoSetLogLevel{},
		inspect.RequestDoSetLogLevelProcess{},
		inspect.RequestDoSetLogLevelMeta{},
		inspect.ResponseDoSetLogLevel{},
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
	//
	// encoders
	//
	encoders.Store(reflect.TypeOf(gen.PID{}), &encoder{Prefix: []byte{edtPID}, Encode: encodePID})
	encoders.Store(reflect.TypeOf(gen.ProcessID{}), &encoder{Prefix: []byte{edtProcessID}, Encode: encodeProcessID})
	encoders.Store(reflect.TypeOf(gen.Ref{}), &encoder{Prefix: []byte{edtRef}, Encode: encodeRef})
	encoders.Store(reflect.TypeOf(gen.Alias{}), &encoder{Prefix: []byte{edtAlias}, Encode: encodeAlias})
	encoders.Store(reflect.TypeOf(gen.Event{}), &encoder{Prefix: []byte{edtEvent}, Encode: encodeEvent})
	encoders.Store(reflect.TypeOf(true), &encoder{Prefix: []byte{edtBool}, Encode: encodeBool})
	encoders.Store(reflect.TypeOf(gen.Atom("atom")), &encoder{Prefix: []byte{edtAtom}, Encode: encodeAtom})
	encoders.Store(reflect.TypeOf("string"), &encoder{Prefix: []byte{edtString}, Encode: encodeString})
	encoders.Store(reflect.TypeOf(int(0)), &encoder{Prefix: []byte{edtInt}, Encode: encodeInt})
	encoders.Store(reflect.TypeOf(int8(0)), &encoder{Prefix: []byte{edtInt8}, Encode: encodeInt8})
	encoders.Store(reflect.TypeOf(int16(0)), &encoder{Prefix: []byte{edtInt16}, Encode: encodeInt16})
	encoders.Store(reflect.TypeOf(int32(0)), &encoder{Prefix: []byte{edtInt32}, Encode: encodeInt32})
	encoders.Store(reflect.TypeOf(int64(0)), &encoder{Prefix: []byte{edtInt64}, Encode: encodeInt64})
	encoders.Store(reflect.TypeOf(uint(0)), &encoder{Prefix: []byte{edtUint}, Encode: encodeUint})
	encoders.Store(reflect.TypeOf(uint8(0)), &encoder{Prefix: []byte{edtUint8}, Encode: encodeUint8})
	encoders.Store(reflect.TypeOf(uint16(0)), &encoder{Prefix: []byte{edtUint16}, Encode: encodeUint16})
	encoders.Store(reflect.TypeOf(uint32(0)), &encoder{Prefix: []byte{edtUint32}, Encode: encodeUint32})
	encoders.Store(reflect.TypeOf(uint64(0)), &encoder{Prefix: []byte{edtUint64}, Encode: encodeUint64})
	encoders.Store(reflect.TypeOf([]byte(nil)), &encoder{Prefix: []byte{edtBinary}, Encode: encodeBinary})
	encoders.Store(reflect.TypeOf(float32(0.0)), &encoder{Prefix: []byte{edtFloat32}, Encode: encodeFloat32})
	encoders.Store(reflect.TypeOf(float64(0.0)), &encoder{Prefix: []byte{edtFloat64}, Encode: encodeFloat64})
	encoders.Store(reflect.TypeOf(time.Time{}), &encoder{Prefix: []byte{edtTime}, Encode: encodeTime})
	encoders.Store(anyType, &encoder{Prefix: []byte{edtAny}, Encode: encodeAny})

	// error types
	encoders.Store(errType, &encoder{Prefix: []byte{edtError}, Encode: encodeError})
	encoders.Store(reflect.TypeOf(fmt.Errorf("")), &encoder{Prefix: []byte{edtError}, Encode: encodeError})
	// wrapped error has a different type
	encoders.Store(reflect.TypeOf(fmt.Errorf("%w", nil)), &encoder{Prefix: []byte{edtError}, Encode: encodeError})

	//
	// decoders
	//
	decPID := &decoder{reflect.TypeOf(gen.PID{}), decodePID}
	decoders.Store(edtPID, decPID)
	decoders.Store(decPID.Type, decPID)

	decProcessID := &decoder{reflect.TypeOf(gen.ProcessID{}), decodeProcessID}
	decoders.Store(edtProcessID, decProcessID)
	decoders.Store(decProcessID.Type, decProcessID)

	decRef := &decoder{reflect.TypeOf(gen.Ref{}), decodeRef}
	decoders.Store(edtRef, decRef)
	decoders.Store(decRef.Type, decRef)

	decAlias := &decoder{reflect.TypeOf(gen.Alias{}), decodeAlias}
	decoders.Store(edtAlias, decAlias)
	decoders.Store(decAlias.Type, decAlias)

	decEvent := &decoder{reflect.TypeOf(gen.Event{}), decodeEvent}
	decoders.Store(edtEvent, decEvent)
	decoders.Store(decEvent.Type, decEvent)

	decTime := &decoder{reflect.TypeOf(time.Time{}), decodeTime}
	decoders.Store(edtTime, decTime)
	decoders.Store(decTime.Type, decTime)

	decBool := &decoder{reflect.TypeOf(true), decodeBool}
	decoders.Store(edtBool, decBool)
	decoders.Store(decBool.Type, decBool)

	decAtom := &decoder{reflect.TypeOf(gen.Atom("atom")), decodeAtom}
	decoders.Store(edtAtom, decAtom)
	decoders.Store(decAtom.Type, decAtom)

	decString := &decoder{reflect.TypeOf("string"), decodeString}
	decoders.Store(edtString, decString)
	decoders.Store(decString.Type, decString)

	decInt := &decoder{reflect.TypeOf(int(0)), decodeInt}
	decoders.Store(edtInt, decInt)
	decoders.Store(decInt.Type, decInt)

	decInt8 := &decoder{reflect.TypeOf(int8(0)), decodeInt8}
	decoders.Store(edtInt8, decInt8)
	decoders.Store(decInt8.Type, decInt8)

	decInt16 := &decoder{reflect.TypeOf(int16(0)), decodeInt16}
	decoders.Store(edtInt16, decInt16)
	decoders.Store(decInt16.Type, decInt16)

	decInt32 := &decoder{reflect.TypeOf(int32(0)), decodeInt32}
	decoders.Store(edtInt32, decInt32)
	decoders.Store(decInt32.Type, decInt32)

	decInt64 := &decoder{reflect.TypeOf(int64(0)), decodeInt64}
	decoders.Store(edtInt64, decInt64)
	decoders.Store(decInt64.Type, decInt64)

	decUint := &decoder{reflect.TypeOf(uint(0)), decodeUint}
	decoders.Store(edtUint, decUint)
	decoders.Store(decUint.Type, decUint)

	decUint8 := &decoder{reflect.TypeOf(uint8(0)), decodeUint8}
	decoders.Store(edtUint8, decUint8)
	decoders.Store(decUint8.Type, decUint8)

	decUint16 := &decoder{reflect.TypeOf(uint16(0)), decodeUint16}
	decoders.Store(edtUint16, decUint16)
	decoders.Store(decUint16.Type, decUint16)

	decUint32 := &decoder{reflect.TypeOf(uint32(0)), decodeUint32}
	decoders.Store(edtUint32, decUint32)
	decoders.Store(decUint32.Type, decUint32)

	decUint64 := &decoder{reflect.TypeOf(uint64(0)), decodeUint64}
	decoders.Store(edtUint64, decUint64)
	decoders.Store(decUint64.Type, decUint64)

	decBinary := &decoder{reflect.TypeOf([]byte(nil)), decodeBinary}
	decoders.Store(edtBinary, decBinary)
	decoders.Store(decBinary.Type, decBinary)

	decFloat32 := &decoder{reflect.TypeOf(float32(0.0)), decodeFloat32}
	decoders.Store(edtFloat32, decFloat32)
	decoders.Store(decFloat32.Type, decFloat32)

	decFloat64 := &decoder{reflect.TypeOf(float64(0.0)), decodeFloat64}
	decoders.Store(edtFloat64, decFloat64)
	decoders.Store(decFloat64.Type, decFloat64)

	decAny := &decoder{anyType, decodeAny}
	decoders.Store(edtAny, decAny)
	decoders.Store(anyType, decAny)

	decErr := &decoder{errType, decodeError}
	decoders.Store(edtError, decErr)
	decoders.Store(decErr.Type, decErr)

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
