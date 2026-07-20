package edf

import (
	"encoding"
	"encoding/binary"
	"fmt"
	"math"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

const deprecationDocsURL = "https://docs.ergo.services/networking/network-transparency"

// deprecation emits the legacy-API warning unless the call comes from
// framework-internal code (proxy in net/proto, edf init, registrar/handshake
// pre-registration, node-level atom caching).
func deprecation(name, replacement string) {
	pc, _, _, _ := runtime.Caller(2)
	if fn := runtime.FuncForPC(pc); fn != nil &&
		strings.HasPrefix(fn.Name(), "ergo.services/ergo/") {
		return
	}
	lib.EmitDeprecation(nil, name, replacement, deprecationDocsURL)
}

type decoder struct {
	Type   reflect.Type
	Decode func(*reflect.Value, []byte, *stateDecode) (*reflect.Value, []byte, error)
	// Info is the per-proto type metadata. Populated for all registered
	// and built-in types; nil for ad-hoc composite decoders constructed
	// in getDecoder for unregistered slice/map/array types.
	Info *gen.RegisteredTypeInfo
}

type encodeFunc func(value reflect.Value, b *lib.Buffer, state *stateEncode) error
type encoder struct {
	Prefix []byte
	Encode encodeFunc
	// Info is the per-proto type metadata. Populated for all registered
	// and built-in types; nil for ad-hoc composite encoders constructed
	// in getEncoder for unregistered slice/map/array types.
	Info *gen.RegisteredTypeInfo
}

func regTypeName(t reflect.Type) string {
	return fmt.Sprintf("#%s/%s", t.PkgPath(), t.Name())
}

func RegisterTypeOf(v any) error {
	deprecation("edf.RegisterTypeOf", "node.Network().RegisterType")
	vov := reflect.ValueOf(v)
	tov := vov.Type()

	if tov.Kind() == reflect.Pointer {
		return fmt.Errorf("pointer type is not supported")
	}

	switch v.(type) {
	case bool, string, error,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		[]byte,
		float32, float64:
		return fmt.Errorf("unable to register a regular type")

	case gen.Atom, gen.PID, gen.ProcessID, gen.Event, gen.Ref, gen.Alias, time.Time:
		return fmt.Errorf("unable to register a type of Ergo Framework")

	case Unmarshaler:
		return fmt.Errorf("UnmarshalEDF method of %v must be a method of *%v", tov, tov)

	case Marshaler:
		// unmarshaling must be implemented as a method of a pointer to the object
		if reflect.PointerTo(tov).Implements(reflect.TypeOf((*Unmarshaler)(nil)).Elem()) == false {
			return fmt.Errorf("UnmarshalEDF method of %v must be a method of *%v", tov, tov)
		}
		name := regTypeName(tov)

		fenc := func(value reflect.Value, b *lib.Buffer, _ *stateEncode) error {
			v := value.Interface().(Marshaler)
			// Record the offset for the length prefix instead of using the
			// slice returned by Extend. If MarshalEDF triggers buffer
			// reallocation, the Extend slice becomes stale but the offset
			// remains valid against the new backing array.
			lenPrefixOffset := b.Len()
			b.Extend(4)
			l := b.Len()
			if err := v.MarshalEDF(b); err != nil {
				return err
			}

			lenBinary := b.Len() - l
			if int64(lenBinary) > int64(math.MaxUint32-1) {
				return ErrBinaryTooLong
			}
			binary.BigEndian.PutUint32(b.B[lenPrefixOffset:], uint32(lenBinary))
			return nil
		}
		fdec := func(value *reflect.Value, packet []byte, state *stateDecode) (*reflect.Value, []byte, error) {
			if len(packet) < 4 {
				return nil, nil, errDecodeEOD
			}

			l := binary.BigEndian.Uint32(packet)
			if uint64(len(packet)) < uint64(l)+4 {
				return nil, nil, errDecodeEOD
			}

			if value == nil {
				v := reflect.Indirect(reflect.New(state.decoder.Type))
				value = &v
			}

			v := value.Addr().Interface().(Unmarshaler)
			if err := v.UnmarshalEDF(packet[4 : 4+int(l)]); err != nil {
				return nil, nil, err
			}

			packet = packet[4+int(l):]
			return value, packet, nil
		}
		addRegCache(tov)
		enc := regEncoder(name, fenc)
		encoders.Store(tov, enc)
		dec := &decoder{Type: tov, Decode: fdec}
		decoders.Store(name, dec)
		decoders.Store(tov, dec)
		info := registerInfo(tov, "marshaler", schemaFor(tov))
		enc.Info = info
		dec.Info = info
		return nil

	case encoding.BinaryUnmarshaler:
		return fmt.Errorf("UnmarshalBinary method of %v must be a method of *%v", tov, tov)

	case encoding.BinaryMarshaler:
		if reflect.PointerTo(tov).Implements(reflect.TypeOf((*encoding.BinaryUnmarshaler)(nil)).Elem()) == false {
			return fmt.Errorf("UnmarshalBinary method of %v must be a method of *%v", tov, tov)
		}
		name := regTypeName(tov)

		fenc := func(value reflect.Value, b *lib.Buffer, _ *stateEncode) error {
			v := value.Interface().(encoding.BinaryMarshaler)
			buf := b.Extend(4)

			bin, err := v.MarshalBinary()
			if err != nil {
				return err
			}

			lenBinary := len(bin)
			if int64(lenBinary) > int64(math.MaxUint32-1) {
				return ErrBinaryTooLong
			}
			binary.BigEndian.PutUint32(buf, uint32(lenBinary))

			b.Append(bin)
			return nil
		}
		fdec := func(value *reflect.Value, packet []byte, state *stateDecode) (*reflect.Value, []byte, error) {
			if len(packet) < 4 {
				return nil, nil, errDecodeEOD
			}

			l := binary.BigEndian.Uint32(packet)
			if uint64(len(packet)) < uint64(l)+4 {
				return nil, nil, errDecodeEOD
			}

			if value == nil {
				v := reflect.Indirect(reflect.New(state.decoder.Type))
				value = &v
			}

			v := value.Addr().Interface().(encoding.BinaryUnmarshaler)
			if err := v.UnmarshalBinary(packet[4 : 4+int(l)]); err != nil {
				return nil, nil, err
			}

			packet = packet[4+int(l):]
			return value, packet, nil
		}
		addRegCache(tov)
		enc := regEncoder(name, fenc)
		encoders.Store(tov, enc)
		dec := &decoder{Type: tov, Decode: fdec}
		decoders.Store(name, dec)
		decoders.Store(tov, dec)
		info := registerInfo(tov, "binarymarshaler", schemaFor(tov))
		enc.Info = info
		dec.Info = info
		return nil

	}

	return registerType(tov)
}

func registerType(tov reflect.Type) error {

	name := regTypeName(tov)

	if _, found := encoders.Load(tov); found {
		return gen.ErrTaken
	}

	if _, found := decoders.Load(name); found {
		return gen.ErrTaken
	}

	switch tov.Kind() {
	case reflect.Bool:
		addRegCache(tov)
		enc := regEncoder(name, encodeBool)
		encoders.Store(tov, enc)
		dec := &decoder{Type: tov, Decode: decodeBool}
		decoders.Store(name, dec)
		decoders.Store(tov, dec)
		info := registerInfo(tov, "bool", schemaFor(tov))
		enc.Info = info
		dec.Info = info
		return nil

	case reflect.Int:
		addRegCache(tov)
		enc := regEncoder(name, encodeInt)
		encoders.Store(tov, enc)
		dec := &decoder{Type: tov, Decode: decodeInt}
		decoders.Store(name, dec)
		decoders.Store(tov, dec)
		info := registerInfo(tov, "int", schemaFor(tov))
		enc.Info = info
		dec.Info = info
		return nil

	case reflect.Int8:
		addRegCache(tov)
		enc := regEncoder(name, encodeInt8)
		encoders.Store(tov, enc)
		dec := &decoder{Type: tov, Decode: decodeInt8}
		decoders.Store(name, dec)
		decoders.Store(tov, dec)
		info := registerInfo(tov, "int8", schemaFor(tov))
		enc.Info = info
		dec.Info = info
		return nil

	case reflect.Int16:
		addRegCache(tov)
		enc := regEncoder(name, encodeInt16)
		encoders.Store(tov, enc)
		dec := &decoder{Type: tov, Decode: decodeInt16}
		decoders.Store(name, dec)
		decoders.Store(tov, dec)
		info := registerInfo(tov, "int16", schemaFor(tov))
		enc.Info = info
		dec.Info = info
		return nil

	case reflect.Int32:
		addRegCache(tov)
		enc := regEncoder(name, encodeInt32)
		encoders.Store(tov, enc)
		dec := &decoder{Type: tov, Decode: decodeInt32}
		decoders.Store(name, dec)
		decoders.Store(tov, dec)
		info := registerInfo(tov, "int32", schemaFor(tov))
		enc.Info = info
		dec.Info = info
		return nil

	case reflect.Int64:
		addRegCache(tov)
		enc := regEncoder(name, encodeInt64)
		encoders.Store(tov, enc)
		dec := &decoder{Type: tov, Decode: decodeInt64}
		decoders.Store(name, dec)
		decoders.Store(tov, dec)
		info := registerInfo(tov, "int64", schemaFor(tov))
		enc.Info = info
		dec.Info = info
		return nil

	case reflect.Uint:
		addRegCache(tov)
		enc := regEncoder(name, encodeUint)
		encoders.Store(tov, enc)
		dec := &decoder{Type: tov, Decode: decodeUint}
		decoders.Store(name, dec)
		decoders.Store(tov, dec)
		info := registerInfo(tov, "uint", schemaFor(tov))
		enc.Info = info
		dec.Info = info
		return nil

	case reflect.Uint8:
		addRegCache(tov)
		enc := regEncoder(name, encodeUint8)
		encoders.Store(tov, enc)
		dec := &decoder{Type: tov, Decode: decodeUint8}
		decoders.Store(name, dec)
		decoders.Store(tov, dec)
		info := registerInfo(tov, "uint8", schemaFor(tov))
		enc.Info = info
		dec.Info = info
		return nil

	case reflect.Uint16:
		addRegCache(tov)
		enc := regEncoder(name, encodeUint16)
		encoders.Store(tov, enc)
		dec := &decoder{Type: tov, Decode: decodeUint16}
		decoders.Store(name, dec)
		decoders.Store(tov, dec)
		info := registerInfo(tov, "uint16", schemaFor(tov))
		enc.Info = info
		dec.Info = info
		return nil

	case reflect.Uint32:
		addRegCache(tov)
		enc := regEncoder(name, encodeUint32)
		encoders.Store(tov, enc)
		dec := &decoder{Type: tov, Decode: decodeUint32}
		decoders.Store(name, dec)
		decoders.Store(tov, dec)
		info := registerInfo(tov, "uint32", schemaFor(tov))
		enc.Info = info
		dec.Info = info
		return nil

	case reflect.Uint64:
		addRegCache(tov)
		enc := regEncoder(name, encodeUint64)
		encoders.Store(tov, enc)
		dec := &decoder{Type: tov, Decode: decodeUint64}
		decoders.Store(name, dec)
		decoders.Store(tov, dec)
		info := registerInfo(tov, "uint64", schemaFor(tov))
		enc.Info = info
		dec.Info = info
		return nil

	case reflect.Float32:
		addRegCache(tov)
		enc := regEncoder(name, encodeFloat32)
		encoders.Store(tov, enc)
		dec := &decoder{Type: tov, Decode: decodeFloat32}
		decoders.Store(name, dec)
		decoders.Store(tov, dec)
		info := registerInfo(tov, "float32", schemaFor(tov))
		enc.Info = info
		dec.Info = info
		return nil

	case reflect.Float64:
		addRegCache(tov)
		enc := regEncoder(name, encodeFloat64)
		encoders.Store(tov, enc)
		dec := &decoder{Type: tov, Decode: decodeFloat64}
		decoders.Store(name, dec)
		decoders.Store(tov, dec)
		info := registerInfo(tov, "float64", schemaFor(tov))
		enc.Info = info
		dec.Info = info
		return nil

	case reflect.String:
		addRegCache(tov)
		enc := regEncoder(name, encodeString)
		encoders.Store(tov, enc)
		dec := &decoder{Type: tov, Decode: decodeString}
		decoders.Store(name, dec)
		decoders.Store(tov, dec)
		info := registerInfo(tov, "string", schemaFor(tov))
		enc.Info = info
		dec.Info = info
		return nil

	case reflect.Struct:
		var encs []*encoder
		var decs []*decoder

		nf := tov.NumField()
		for i := 0; i < nf; i++ {
			field := tov.Field(i)

			// edf:"-" excludes the field from wire encoding entirely. Nil
			// slots in encs/decs are treated as skip markers in the closures.
			if field.Tag.Get("edf") == "-" {
				encs = append(encs, nil)
				decs = append(decs, nil)
				continue
			}

			if field.IsExported() == false {
				return fmt.Errorf("struct %s has unexported field(s)", tov.Name())
			}

			ft := field.Type
			enc, err := getEncoder(ft, &stateEncode{})
			if err != nil {
				return fmt.Errorf("(struct field encode) type %v must be registered first: %s", ft, err)
			}
			encs = append(encs, enc)

			dec, _, err := decodeType(enc.Prefix, &stateDecode{})
			if err != nil {
				return fmt.Errorf("(struct field decode) type %v must be registered first: %s", ft, err)
			}
			decs = append(decs, dec)
		}

		// encoder closure
		fenc := func(value reflect.Value, b *lib.Buffer, state *stateEncode) error {
			// schema evolution: prefix the body with its length so a peer with a
			// different field count tolerates the difference. Backfilled by offset
			// (encoding the body may reallocate b, invalidating an Extend slice).
			evolve := state.options.SchemaEvolution
			lenOffset := 0
			if evolve {
				lenOffset = b.Len()
				b.Extend(4)
			}
			if state.child == nil {
				state.child = &stateEncode{options: state.options}
			}
			state = state.child
			bodyStart := b.Len()
			for i := 0; i < nf; i++ {
				if encs[i] == nil {
					continue
				}
				state.encodeType = false
				if err := encs[i].Encode(value.Field(i), b, state); err != nil {
					return err
				}
			}
			if evolve {
				if int64(b.Len()-bodyStart) > int64(math.MaxUint32-1) {
					return ErrStructTooLong
				}
				binary.BigEndian.PutUint32(b.B[lenOffset:], uint32(b.Len()-bodyStart))
			}
			return nil
		}

		// decoder closure
		fdec := func(value *reflect.Value, packet []byte, state *stateDecode) (*reflect.Value, []byte, error) {
			var err error

			if value == nil {
				v := reflect.Indirect(reflect.New(state.decoder.Type))
				value = &v
			}

			// schema evolution: the body is length-prefixed. Decode fields within the
			// body only - a peer with fewer fields leaves the rest zero-valued, a peer
			// with more has its extra trailing fields skipped (rest resumes after body).
			body, rest := packet, []byte(nil)
			evolve := state.options.SchemaEvolution
			if evolve {
				if len(packet) < 4 {
					return nil, nil, errDecodeEOD
				}
				l := binary.BigEndian.Uint32(packet)
				if uint64(len(packet)) < uint64(l)+4 {
					return nil, nil, errDecodeEOD
				}
				body = packet[4 : 4+l]
				rest = packet[4+l:]
			}

			if state.child == nil {
				state.child = &stateDecode{options: state.options}
			}
			state = state.child
			for i := 0; i < nf; i++ {
				if decs[i] == nil {
					continue
				}
				if evolve && len(body) == 0 {
					break
				}
				field := value.Field(i)
				_, body, err = decs[i].Decode(&field, body, state)
				if err != nil {
					return nil, nil, err
				}
			}
			if evolve {
				return value, rest, nil
			}
			return value, body, nil
		}
		addRegCache(tov)
		enc := regEncoder(name, fenc)
		encoders.Store(tov, enc)
		dec := &decoder{Type: tov, Decode: fdec}
		decoders.Store(name, dec)
		info := registerInfo(tov, "struct", schemaFor(tov))
		enc.Info = info
		dec.Info = info

		return nil

	case reflect.Slice:
		itemType := tov.Elem()

		// encoder
		enc, err := getEncoder(itemType, &stateEncode{})
		if err != nil {
			return fmt.Errorf("(slice item encoder) type %v must be registered first: %s", itemType, err)
		}

		// decoder
		dec, _, err := decodeType(enc.Prefix, &stateDecode{})
		if err != nil {
			return fmt.Errorf("(slice item decoder) type %v must be registered first: %s", itemType, err)
		}

		// encode closure
		fenc := func(value reflect.Value, b *lib.Buffer, state *stateEncode) error {
			if value.IsNil() {
				b.AppendByte(edtNil)
				return nil
			}
			b.AppendByte(edtReg)

			n := value.Len()
			buf := b.Extend(4)
			binary.BigEndian.PutUint32(buf, uint32(n))
			if state.child == nil {
				state.child = &stateEncode{options: state.options}
			}
			state = state.child
			for i := 0; i < n; i++ {
				state.encodeType = false
				if err := enc.Encode(value.Index(i), b, state); err != nil {
					return err
				}
			}
			return nil
		}

		// decode closure
		fdec := func(value *reflect.Value, packet []byte, state *stateDecode) (*reflect.Value, []byte, error) {
			if len(packet) == 0 {
				return nil, nil, errDecodeEOD
			}

			if packet[0] == edtNil {
				packet = packet[1:]
				return nil, packet, nil
			}
			if packet[0] != edtReg {
				return nil, nil, fmt.Errorf("incorrect slice/array type %d", packet[0])
			}
			packet = packet[1:]

			if len(packet) < 4 {
				return nil, nil, errDecodeEOD
			}

			n := int(binary.BigEndian.Uint32(packet[:4]))
			packet = packet[4:]

			if n > len(packet) {
				return nil, nil, fmt.Errorf("incorrect data length %d", n)
			}

			alloc := preallocCount(n, len(packet), tov.Elem().Size())
			x := reflect.MakeSlice(tov, alloc, alloc)

			if n == 0 {
				if value == nil {
					value = &x
				} else {
					value.Set(x)
				}
				return value, packet, nil
			}

			if state.child == nil {
				state.child = &stateDecode{
					options: state.options,
					decoder: dec,
				}
			}
			state = state.child

			for i := 0; i < n; i++ {
				if i == x.Len() {
					grow := preallocCount(n-i, len(packet), tov.Elem().Size())
					if grow < 1 {
						grow = 1
					}
					x = reflect.AppendSlice(x, reflect.MakeSlice(tov, grow, grow))
				}
				item := x.Index(i)
				_, p, err := dec.Decode(&item, packet, state)
				if err != nil {
					return nil, nil, err
				}
				packet = p
			}

			if value == nil {
				value = &x
			} else {
				value.Set(x)
			}
			return value, packet, nil
		}
		addRegCache(tov)
		regEnc := regEncoder(name, fenc)
		encoders.Store(tov, regEnc)
		regDec := &decoder{Type: tov, Decode: fdec}
		decoders.Store(name, regDec)
		info := registerInfo(tov, "slice", schemaFor(tov))
		regEnc.Info = info
		regDec.Info = info

	case reflect.Array:
		itemType := tov.Elem()

		// encoder
		enc, err := getEncoder(itemType, &stateEncode{})
		if err != nil {
			return fmt.Errorf("(array item encoder) type %v must be registered first: %s", itemType, err)
		}

		// decoder
		dec, _, err := decodeType(enc.Prefix, &stateDecode{})
		if err != nil {
			return fmt.Errorf("(array item decoder) type %v must be registered first: %s", itemType, err)
		}

		fenc := func(value reflect.Value, b *lib.Buffer, state *stateEncode) error {
			if state.child == nil {
				state.child = &stateEncode{options: state.options}
			}
			state = state.child
			for i := 0; i < value.Len(); i++ {
				state.encodeType = false
				if err := enc.Encode(value.Index(i), b, state); err != nil {
					return err
				}
			}
			return nil
		}

		fdec := func(value *reflect.Value, packet []byte, state *stateDecode) (*reflect.Value, []byte, error) {
			if len(packet) == 0 {
				if tov.Len() == 0 {
					return value, packet, nil
				}
				return nil, nil, errDecodeEOD
			}
			if value == nil {
				x := reflect.Indirect(reflect.New(tov))
				value = &x
			}

			if state.child == nil {
				state.child = &stateDecode{
					options: state.options,
					decoder: dec,
				}
			}
			state = state.child

			for i := 0; i < tov.Len(); i++ {
				item := value.Index(i)
				_, p, err := dec.Decode(&item, packet, state)
				if err != nil {
					return nil, nil, err
				}
				packet = p
			}

			return value, packet, nil
		}
		addRegCache(tov)
		regEnc := regEncoder(name, fenc)
		encoders.Store(tov, regEnc)
		regDec := &decoder{Type: tov, Decode: fdec}
		decoders.Store(name, regDec)
		info := registerInfo(tov, "array", schemaFor(tov))
		regEnc.Info = info
		regDec.Info = info

	case reflect.Map:
		typeKey := tov.Key()
		typeValue := tov.Elem()

		// encoders for key/value
		encKey, err := getEncoder(typeKey, &stateEncode{})
		if err != nil {
			return fmt.Errorf("(map key encoder) type %v must be registered first: %s", typeKey, err)
		}
		encValue, err := getEncoder(typeValue, &stateEncode{})
		if err != nil {
			return fmt.Errorf("(map value encoder) type %v must be registered first: %s", typeValue, err)
		}

		// decoders for key/value
		decKey, _, err := decodeType(encKey.Prefix, &stateDecode{})
		if err != nil {
			return fmt.Errorf("(map key decoder) type %v must be registered first: %s", typeKey, err)
		}
		decValue, _, err := decodeType(encValue.Prefix, &stateDecode{})
		if err != nil {
			return fmt.Errorf("(map value decoder) type %v must be registered first: %s", typeValue, err)
		}

		fenc := func(value reflect.Value, b *lib.Buffer, state *stateEncode) error {
			if value.IsNil() {
				b.AppendByte(edtNil)
				return nil
			} else {
				b.AppendByte(edtReg)
			}

			if state.child == nil {
				state.child = &stateEncode{
					options: state.options,
				}
			}
			state = state.child

			n := value.Len()
			buf := b.Extend(4)
			binary.BigEndian.PutUint32(buf, uint32(n))

			iter := value.MapRange()
			for iter.Next() {
				state.encodeType = false
				if err := encKey.Encode(iter.Key(), b, state); err != nil {
					return err
				}
				state.encodeType = false
				if err := encValue.Encode(iter.Value(), b, state); err != nil {
					return err
				}
			}
			return nil
		}

		fdec := func(value *reflect.Value, packet []byte, state *stateDecode) (*reflect.Value, []byte, error) {
			if len(packet) == 0 {
				return nil, nil, errDecodeEOD
			}

			if packet[0] == edtNil {
				packet = packet[1:]
				return nil, packet, nil
			}

			if packet[0] != edtReg {
				return nil, nil, fmt.Errorf("incorrect map type %d", packet[0])
			}
			packet = packet[1:]

			if len(packet) < 4 {
				return nil, nil, errDecodeEOD
			}

			n := int(binary.BigEndian.Uint32(packet[:4]))
			packet = packet[4:]

			// reject oversized/negative count before allocating (n entries need >= n bytes)
			if n < 0 || n > len(packet) {
				return nil, nil, fmt.Errorf("incorrect data length")
			}

			x := reflect.MakeMapWithSize(tov, preallocCount(n, len(packet), tov.Key().Size()+tov.Elem().Size()))
			if value == nil {
				value = &x
			} else {
				value.Set(x)
			}

			if n == 0 {
				return value, packet, nil
			}

			if state.child == nil {
				state.child = &stateDecode{
					options: state.options,
				}
			}
			state = state.child

			for i := 0; i < n; i++ {
				k := reflect.Indirect(reflect.New(decKey.Type))
				state.decoder = decKey
				_, p, err := decKey.Decode(&k, packet, state)
				if err != nil {
					return nil, nil, err
				}
				packet = p

				v := reflect.Indirect(reflect.New(decValue.Type))
				state.decoder = decValue
				_, p, err = decValue.Decode(&v, packet, state)
				if err != nil {
					return nil, nil, err
				}
				packet = p

				value.SetMapIndex(k, v)
			}

			return value, packet, nil
		}
		addRegCache(tov)
		enc := regEncoder(name, fenc)
		encoders.Store(tov, enc)
		dec := &decoder{Type: tov, Decode: fdec}
		decoders.Store(name, dec)
		info := registerInfo(tov, "map", schemaFor(tov))
		enc.Info = info
		dec.Info = info

	default:
		return fmt.Errorf("type %v is not supported", tov)
	}

	return nil
}

func RegisterError(e error) error {
	deprecation("edf.RegisterError", "node.Network().RegisterError")
	return addErrCache(e)
}

func RegisterAtom(a gen.Atom) error {
	deprecation("edf.RegisterAtom", "node.Network().RegisterAtom")
	return addAtomCache(a)
}

var (
	encoders sync.Map
	decoders sync.Map

	registeredTypes sync.Map // reflect.Type -> *gen.RegisteredTypeInfo.
	registerOrder   atomic.Uint64
)

func registerInfo(t reflect.Type, kind, schema string) *gen.RegisteredTypeInfo {
	custom := kind == "marshaler" || kind == "binarymarshaler"

	info := &gen.RegisteredTypeInfo{
		ID:           registerOrder.Add(1),
		Name:         regTypeName(t),
		Kind:         kind,
		Schema:       schema,
		MinSize:      measureZeroSize(t, kind),
		SizeVariable: custom || hasVariableSize(t, make(map[reflect.Type]bool)),
		Stats:        gen.RegisteredTypeStats{Enabled: statsEnabled},
	}

	if actual, loaded := registeredTypes.LoadOrStore(t, info); loaded {
		return actual.(*gen.RegisteredTypeInfo)
	}
	return info
}

func measureZeroSize(t reflect.Type, kind string) (size uint32) {
	var fallback uint32
	switch {
	case kind == "marshaler" || kind == "binarymarshaler":
		// 3 bytes cached type-tag + 4 bytes length prefix
		fallback = 7
	case t == anyType:
		// nil interface encodes as edtNil (1 byte)
		return 1
	case t == errType:
		// nil error encodes as [0xff, 0xff] (2 bytes)
		return 2
	}
	defer func() {
		if r := recover(); r != nil {
			size = fallback
		}
	}()
	v := reflect.New(t).Elem().Interface()
	if v == nil {
		return fallback
	}
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)
	if err := Encode(v, buf, Options{RegCache: &regCache}); err != nil {
		return fallback
	}
	return uint32(buf.Len())
}

func hasVariableSize(t reflect.Type, visited map[reflect.Type]bool) bool {
	if visited[t] {
		return false
	}
	visited[t] = true
	switch t.Kind() {
	case reflect.String, reflect.Slice, reflect.Map, reflect.Pointer, reflect.Interface:
		return true
	case reflect.Array:
		return hasVariableSize(t.Elem(), visited)
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			if hasVariableSize(t.Field(i).Type, visited) {
				return true
			}
		}
	}
	return false
}

// regEncoder creates a registered-type encoder. Info is attached separately
// by the caller after registerInfo has been called (which itself relies on
// the encoder already being stored in the encoders map).
func regEncoder(name string, enc encodeFunc) *encoder {
	l := uint16(len(name))
	if l > 4095 {
		panic(fmt.Sprintf("unable to register type. too long name: %s", name))
	}
	prefix := []byte{edtReg, 0, 0}
	binary.BigEndian.PutUint16(prefix[1:3], l)
	prefix = append(prefix, name...)

	return &encoder{
		Prefix: prefix,
		Encode: func(value reflect.Value, b *lib.Buffer, state *stateEncode) error {
			var prev bool
			if state.encodeType {
				if state.options.RegCache != nil {
					if v, found := state.options.RegCache.Load(value.Type()); found {
						b.Append(v.([]byte))
					} else {
						b.Append(prefix)
					}
				} else {
					b.Append(prefix)
				}

				state.encodeType = false
				prev = true
			}
			err := enc(value, b, state)
			state.encodeType = prev
			if err != nil {
				return err
			}

			return nil
		},
	}
}

// for outgoing (encoding) messages.
var regCacheID uint32 = 4095 // 0..4095 - reserved (used as a length)
var regCache sync.Map

func addRegCache(t reflect.Type) error {
	id := atomic.AddUint32(&regCacheID, 1)
	if id > math.MaxUint16 {
		return fmt.Errorf("too many registered types")
	}
	reg := []byte{edtReg, 0, 0}
	binary.BigEndian.PutUint16(reg[1:3], uint16(id))

	if _, exist := regCache.LoadOrStore(t, reg); exist {
		return gen.ErrTaken
	}
	regCache.Store(uint16(id), regTypeName(t))
	return nil
}

// RegisteredTypes returns all registered EDF type metadata in registration order.
// Each entry is a value snapshot of the underlying *gen.RegisteredTypeInfo;
// counter fields reflect their values at snapshot time.
func RegisteredTypes() []gen.RegisteredTypeInfo {
	var list []gen.RegisteredTypeInfo
	registeredTypes.Range(func(_, v any) bool {
		list = append(list, *v.(*gen.RegisteredTypeInfo))
		return true
	})
	sort.Slice(list, func(i, j int) bool {
		return list[i].ID < list[j].ID
	})
	return list
}

// RegisterTypesOf registers a batch with iterative resolve. Order-agnostic:
// types with unresolved dependencies are retried while progress is made.
// Returns error listing types that cannot be resolved after exhausting passes.
func RegisterTypesOf(types []any) error {
	deprecation("edf.RegisterTypesOf", "node.Network().RegisterTypes")
	pending := types
	for len(pending) > 0 {
		var next []any
		progress := false
		for _, t := range pending {
			err := RegisterTypeOf(t)
			if err == nil || err == gen.ErrTaken {
				progress = true
				continue
			}
			next = append(next, t)
		}
		if progress == false && len(next) > 0 {
			names := make([]string, 0, len(next))
			for _, t := range next {
				names = append(names, fmt.Sprintf("%T", t))
			}
			return fmt.Errorf("unresolvable types: %s", strings.Join(names, ", "))
		}
		pending = next
	}
	return nil
}

// LookupType returns the reflect.Type for a registered type by name.
// The name can be a full EDF name ("#pkgpath/TypeName") or a short
// type name ("TypeName") which matches the first type with that suffix.
func LookupType(name string) (reflect.Type, bool) {
	// Try exact match first
	if v, ok := decoders.Load(name); ok {
		return v.(*decoder).Type, true
	}

	// Try short name match (suffix match on "/TypeName")
	suffix := "/" + name
	var found reflect.Type
	decoders.Range(func(k, v any) bool {
		s, ok := k.(string)
		if ok == false {
			return true
		}
		if len(s) > len(suffix) && s[len(s)-len(suffix):] == suffix {
			found = v.(*decoder).Type
			return false
		}
		return true
	})
	if found != nil {
		return found, true
	}
	return nil, false
}

func GetRegCache() map[uint16]string {
	cache := make(map[uint16]string)
	regCache.Range(func(k, v any) bool {
		id, ok := k.(uint16)
		if ok == false {
			return true
		}
		name := v.(string)
		cache[id] = name
		return true
	})
	if len(cache) == 0 {
		return nil
	}
	return cache
}

func MakeEncodeRegTypeCache(names []string) *sync.Map {
	mapnames := make(map[string]bool)
	for _, name := range names {
		mapnames[name] = true
	}
	if len(mapnames) == 0 {
		return nil
	}
	cache := new(sync.Map)
	regCache.Range(func(k, v any) bool {
		t, ok := k.(reflect.Type)
		if ok == false {
			return true
		}
		tn := regTypeName(t)
		if _, found := mapnames[tn]; found == false {
			return true
		}
		cache.Store(t, v)
		return true
	})
	return cache
}

var errCacheID uint32 = math.MaxInt16 // 0..32767 - reserved (used as a length)
var errCache sync.Map

func GetErrCache() map[uint16]error {
	cache := make(map[uint16]error)
	errCache.Range(func(k, v any) bool {
		if id, ok := v.(uint16); ok {
			err := k.(error)
			cache[id] = err
		}
		return true
	})
	if len(cache) == 0 {
		return nil
	}
	return cache
}

func addErrCache(e error) error {
	if _, ok := e.(*gen.Error); ok {
		return fmt.Errorf("cannot register *gen.Error: register markers (errors.New) and construct wrap chains via gen.Errorf")
	}
	id := atomic.AddUint32(&errCacheID, 1)
	// 0xFFFF reserved for nil error, 0xFFFE reserved for wrapped *gen.Error marker.
	if id > math.MaxUint16-2 {
		return fmt.Errorf("too many registered errors")
	}
	if _, exist := errCache.LoadOrStore(e, uint16(id)); exist {
		return gen.ErrTaken
	}
	return nil
}

var atomCacheID uint32 = 255 // 0..255 - reserved (used as a length)
var atomCache sync.Map

func GetAtomCache() map[uint16]gen.Atom {
	cache := make(map[uint16]gen.Atom)
	atomCache.Range(func(k, v any) bool {
		id, ok := v.(uint16)
		if ok == false {
			return true
		}
		atom := k.(gen.Atom)
		cache[id] = atom
		return true
	})
	if len(cache) == 0 {
		return nil
	}
	return cache
}

func addAtomCache(atom gen.Atom) error {
	id := atomic.AddUint32(&atomCacheID, 1)
	// the last 1000 ids for the custom atoms
	if id > math.MaxUint16-1000 {
		return fmt.Errorf("too many registered atoms")
	}
	if _, exist := atomCache.LoadOrStore(atom, uint16(id)); exist {
		return gen.ErrTaken
	}
	return nil
}
