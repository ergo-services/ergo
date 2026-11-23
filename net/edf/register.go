package edf

import (
	"encoding"
	"encoding/binary"
	"fmt"
	"math"
	"reflect"
	"sync/atomic"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

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
	raw  encodeFunc
}

func regTypeName(t reflect.Type) string {
	return fmt.Sprintf("#%s/%s", t.PkgPath(), t.Name())
}

// checkRegistered reports whether the type is registered already. Another
// type registered with its name is ErrTaken.
func (r *Registry) checkRegistered(tov reflect.Type) (bool, error) {
	known, found := r.typesByName.Load(regTypeName(tov))
	if found == false {
		return false, nil
	}
	if known.(reflect.Type) == tov {
		return true, nil
	}
	return false, gen.ErrTaken
}

// registerParts registers the types a field type is made of, so registering a
// type registers the types of its fields: the structs, the named scalars and
// the named slices, maps and arrays. A pointer is not a type of its own here,
// its element is.
func (r *Registry) registerParts(t reflect.Type, visiting map[reflect.Type]bool) error {
	switch t.Kind() {
	case reflect.Interface, reflect.Chan, reflect.Func, reflect.UnsafePointer:
		return nil

	case reflect.Pointer:
		return r.registerParts(t.Elem(), visiting)

	case reflect.Map:
		if err := r.registerParts(t.Key(), visiting); err != nil {
			return err
		}
		if err := r.registerParts(t.Elem(), visiting); err != nil {
			return err
		}

	case reflect.Slice, reflect.Array:
		if err := r.registerParts(t.Elem(), visiting); err != nil {
			return err
		}
	}

	if t.PkgPath() == "" {
		return nil
	}
	if _, found := r.encoders.Load(t); found {
		return nil
	}
	if visiting[t] {
		return nil
	}
	if err := r.registerTypeOf(t, visiting); err != nil {
		return fmt.Errorf("%s: %s", regTypeName(t), err)
	}
	return nil
}

func (r *Registry) registerTypeOf(tov reflect.Type, visiting map[reflect.Type]bool) error {
	if tov.Kind() == reflect.Pointer {
		return fmt.Errorf("pointer type is not supported")
	}
	v := reflect.Zero(tov).Interface()

	switch v.(type) {
	case error:
		// Caught before the regular-type case below, which would answer with a
		// message that reads as "your type is too simple". A concrete error type
		// has no structural encoding on the wire at all: register sentinel
		// markers instead, and carry per-instance data outside the error.
		return fmt.Errorf("error types cannot be registered: register sentinel markers (errors.New) with RegisterError, and carry structured data in a typed field beside the error field")

	case bool, string,
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
		if done, err := r.checkRegistered(tov); done || err != nil {
			return err
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
		if err := r.addRegCache(tov); err != nil {
			return err
		}
		enc := regEncoder(name, fenc)
		r.encoders.Store(tov, enc)
		dec := &decoder{Type: tov, Decode: fdec}
		r.decoders.Store(name, dec)
		r.decoders.Store(tov, dec)
		info := r.registerInfo(tov, "marshaler", schemaFor(tov))
		enc.Info = info
		dec.Info = info
		return nil

	case encoding.BinaryUnmarshaler:
		return fmt.Errorf("UnmarshalBinary method of %v must be a method of *%v", tov, tov)

	case encoding.BinaryMarshaler:
		if reflect.PointerTo(tov).Implements(reflect.TypeOf((*encoding.BinaryUnmarshaler)(nil)).Elem()) == false {
			return fmt.Errorf("UnmarshalBinary method of %v must be a method of *%v", tov, tov)
		}
		if done, err := r.checkRegistered(tov); done || err != nil {
			return err
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
		if err := r.addRegCache(tov); err != nil {
			return err
		}
		enc := regEncoder(name, fenc)
		r.encoders.Store(tov, enc)
		dec := &decoder{Type: tov, Decode: fdec}
		r.decoders.Store(name, dec)
		r.decoders.Store(tov, dec)
		info := r.registerInfo(tov, "binarymarshaler", schemaFor(tov))
		enc.Info = info
		dec.Info = info
		return nil

	}

	return r.registerType(tov, visiting)
}

func (r *Registry) registerType(tov reflect.Type, visiting map[reflect.Type]bool) error {

	name := regTypeName(tov)

	if done, err := r.checkRegistered(tov); done || err != nil {
		return err
	}

	visiting[tov] = true
	defer delete(visiting, tov)

	switch tov.Kind() {
	case reflect.Bool:
		if err := r.addRegCache(tov); err != nil {
			return err
		}
		enc := regEncoder(name, encodeBool)
		r.encoders.Store(tov, enc)
		dec := &decoder{Type: tov, Decode: decodeBool}
		r.decoders.Store(name, dec)
		r.decoders.Store(tov, dec)
		info := r.registerInfo(tov, "bool", schemaFor(tov))
		enc.Info = info
		dec.Info = info
		return nil

	case reflect.Int:
		if err := r.addRegCache(tov); err != nil {
			return err
		}
		enc := regEncoder(name, encodeInt)
		r.encoders.Store(tov, enc)
		dec := &decoder{Type: tov, Decode: decodeInt}
		r.decoders.Store(name, dec)
		r.decoders.Store(tov, dec)
		info := r.registerInfo(tov, "int", schemaFor(tov))
		enc.Info = info
		dec.Info = info
		return nil

	case reflect.Int8:
		if err := r.addRegCache(tov); err != nil {
			return err
		}
		enc := regEncoder(name, encodeInt8)
		r.encoders.Store(tov, enc)
		dec := &decoder{Type: tov, Decode: decodeInt8}
		r.decoders.Store(name, dec)
		r.decoders.Store(tov, dec)
		info := r.registerInfo(tov, "int8", schemaFor(tov))
		enc.Info = info
		dec.Info = info
		return nil

	case reflect.Int16:
		if err := r.addRegCache(tov); err != nil {
			return err
		}
		enc := regEncoder(name, encodeInt16)
		r.encoders.Store(tov, enc)
		dec := &decoder{Type: tov, Decode: decodeInt16}
		r.decoders.Store(name, dec)
		r.decoders.Store(tov, dec)
		info := r.registerInfo(tov, "int16", schemaFor(tov))
		enc.Info = info
		dec.Info = info
		return nil

	case reflect.Int32:
		if err := r.addRegCache(tov); err != nil {
			return err
		}
		enc := regEncoder(name, encodeInt32)
		r.encoders.Store(tov, enc)
		dec := &decoder{Type: tov, Decode: decodeInt32}
		r.decoders.Store(name, dec)
		r.decoders.Store(tov, dec)
		info := r.registerInfo(tov, "int32", schemaFor(tov))
		enc.Info = info
		dec.Info = info
		return nil

	case reflect.Int64:
		if err := r.addRegCache(tov); err != nil {
			return err
		}
		enc := regEncoder(name, encodeInt64)
		r.encoders.Store(tov, enc)
		dec := &decoder{Type: tov, Decode: decodeInt64}
		r.decoders.Store(name, dec)
		r.decoders.Store(tov, dec)
		info := r.registerInfo(tov, "int64", schemaFor(tov))
		enc.Info = info
		dec.Info = info
		return nil

	case reflect.Uint:
		if err := r.addRegCache(tov); err != nil {
			return err
		}
		enc := regEncoder(name, encodeUint)
		r.encoders.Store(tov, enc)
		dec := &decoder{Type: tov, Decode: decodeUint}
		r.decoders.Store(name, dec)
		r.decoders.Store(tov, dec)
		info := r.registerInfo(tov, "uint", schemaFor(tov))
		enc.Info = info
		dec.Info = info
		return nil

	case reflect.Uint8:
		if err := r.addRegCache(tov); err != nil {
			return err
		}
		enc := regEncoder(name, encodeUint8)
		r.encoders.Store(tov, enc)
		dec := &decoder{Type: tov, Decode: decodeUint8}
		r.decoders.Store(name, dec)
		r.decoders.Store(tov, dec)
		info := r.registerInfo(tov, "uint8", schemaFor(tov))
		enc.Info = info
		dec.Info = info
		return nil

	case reflect.Uint16:
		if err := r.addRegCache(tov); err != nil {
			return err
		}
		enc := regEncoder(name, encodeUint16)
		r.encoders.Store(tov, enc)
		dec := &decoder{Type: tov, Decode: decodeUint16}
		r.decoders.Store(name, dec)
		r.decoders.Store(tov, dec)
		info := r.registerInfo(tov, "uint16", schemaFor(tov))
		enc.Info = info
		dec.Info = info
		return nil

	case reflect.Uint32:
		if err := r.addRegCache(tov); err != nil {
			return err
		}
		enc := regEncoder(name, encodeUint32)
		r.encoders.Store(tov, enc)
		dec := &decoder{Type: tov, Decode: decodeUint32}
		r.decoders.Store(name, dec)
		r.decoders.Store(tov, dec)
		info := r.registerInfo(tov, "uint32", schemaFor(tov))
		enc.Info = info
		dec.Info = info
		return nil

	case reflect.Uint64:
		if err := r.addRegCache(tov); err != nil {
			return err
		}
		enc := regEncoder(name, encodeUint64)
		r.encoders.Store(tov, enc)
		dec := &decoder{Type: tov, Decode: decodeUint64}
		r.decoders.Store(name, dec)
		r.decoders.Store(tov, dec)
		info := r.registerInfo(tov, "uint64", schemaFor(tov))
		enc.Info = info
		dec.Info = info
		return nil

	case reflect.Float32:
		if err := r.addRegCache(tov); err != nil {
			return err
		}
		enc := regEncoder(name, encodeFloat32)
		r.encoders.Store(tov, enc)
		dec := &decoder{Type: tov, Decode: decodeFloat32}
		r.decoders.Store(name, dec)
		r.decoders.Store(tov, dec)
		info := r.registerInfo(tov, "float32", schemaFor(tov))
		enc.Info = info
		dec.Info = info
		return nil

	case reflect.Float64:
		if err := r.addRegCache(tov); err != nil {
			return err
		}
		enc := regEncoder(name, encodeFloat64)
		r.encoders.Store(tov, enc)
		dec := &decoder{Type: tov, Decode: decodeFloat64}
		r.decoders.Store(name, dec)
		r.decoders.Store(tov, dec)
		info := r.registerInfo(tov, "float64", schemaFor(tov))
		enc.Info = info
		dec.Info = info
		return nil

	case reflect.String:
		if err := r.addRegCache(tov); err != nil {
			return err
		}
		enc := regEncoder(name, encodeString)
		r.encoders.Store(tov, enc)
		dec := &decoder{Type: tov, Decode: decodeString}
		r.decoders.Store(name, dec)
		r.decoders.Store(tov, dec)
		info := r.registerInfo(tov, "string", schemaFor(tov))
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
			if err := r.registerParts(ft, visiting); err != nil {
				return fmt.Errorf("%s: %s: %s", name, field.Name, err)
			}
			enc, err := getEncoder(ft, &stateEncode{options: Options{Registry: r}})
			if err != nil {
				return fmt.Errorf("(struct field encode) type %v must be registered first: %s", ft, err)
			}
			encs = append(encs, enc)

			dec, _, err := decodeType(enc.Prefix, &stateDecode{options: Options{Registry: r}}, 0)
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
				state.child = getPooledStateEncode(state.options)
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
				state.child = getPooledStateDecode(state.options)
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
		if err := r.addRegCache(tov); err != nil {
			return err
		}
		enc := regEncoder(name, fenc)
		r.encoders.Store(tov, enc)
		dec := &decoder{Type: tov, Decode: fdec}
		r.decoders.Store(name, dec)
		info := r.registerInfo(tov, "struct", schemaFor(tov))
		enc.Info = info
		dec.Info = info

		return nil

	case reflect.Slice:
		itemType := tov.Elem()
		if err := r.registerParts(itemType, visiting); err != nil {
			return fmt.Errorf("%s: %s", name, err)
		}

		// encoder
		enc, err := getEncoder(itemType, &stateEncode{options: Options{Registry: r}})
		if err != nil {
			return fmt.Errorf("(slice item encoder) type %v must be registered first: %s", itemType, err)
		}

		// decoder
		dec, _, err := decodeType(enc.Prefix, &stateDecode{options: Options{Registry: r}}, 0)
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
				state.child = getPooledStateEncode(state.options)
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
				state.child = getPooledStateDecode(state.options)
				state.child.decoder = dec
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
		if err := r.addRegCache(tov); err != nil {
			return err
		}
		regEnc := regEncoder(name, fenc)
		r.encoders.Store(tov, regEnc)
		regDec := &decoder{Type: tov, Decode: fdec}
		r.decoders.Store(name, regDec)
		info := r.registerInfo(tov, "slice", schemaFor(tov))
		regEnc.Info = info
		regDec.Info = info

	case reflect.Array:
		itemType := tov.Elem()
		if err := r.registerParts(itemType, visiting); err != nil {
			return fmt.Errorf("%s: %s", name, err)
		}

		// encoder
		enc, err := getEncoder(itemType, &stateEncode{options: Options{Registry: r}})
		if err != nil {
			return fmt.Errorf("(array item encoder) type %v must be registered first: %s", itemType, err)
		}

		// decoder
		dec, _, err := decodeType(enc.Prefix, &stateDecode{options: Options{Registry: r}}, 0)
		if err != nil {
			return fmt.Errorf("(array item decoder) type %v must be registered first: %s", itemType, err)
		}

		fenc := func(value reflect.Value, b *lib.Buffer, state *stateEncode) error {
			if state.child == nil {
				state.child = getPooledStateEncode(state.options)
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
				state.child = getPooledStateDecode(state.options)
				state.child.decoder = dec
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
		if err := r.addRegCache(tov); err != nil {
			return err
		}
		regEnc := regEncoder(name, fenc)
		r.encoders.Store(tov, regEnc)
		regDec := &decoder{Type: tov, Decode: fdec}
		r.decoders.Store(name, regDec)
		info := r.registerInfo(tov, "array", schemaFor(tov))
		regEnc.Info = info
		regDec.Info = info

	case reflect.Map:
		typeKey := tov.Key()
		typeValue := tov.Elem()
		if err := r.registerParts(typeKey, visiting); err != nil {
			return fmt.Errorf("%s: %s", name, err)
		}
		if err := r.registerParts(typeValue, visiting); err != nil {
			return fmt.Errorf("%s: %s", name, err)
		}

		// encoders for key/value
		encKey, err := getEncoder(typeKey, &stateEncode{options: Options{Registry: r}})
		if err != nil {
			return fmt.Errorf("(map key encoder) type %v must be registered first: %s", typeKey, err)
		}
		encValue, err := getEncoder(typeValue, &stateEncode{options: Options{Registry: r}})
		if err != nil {
			return fmt.Errorf("(map value encoder) type %v must be registered first: %s", typeValue, err)
		}

		// decoders for key/value
		decKey, _, err := decodeType(encKey.Prefix, &stateDecode{options: Options{Registry: r}}, 0)
		if err != nil {
			return fmt.Errorf("(map key decoder) type %v must be registered first: %s", typeKey, err)
		}
		decValue, _, err := decodeType(encValue.Prefix, &stateDecode{options: Options{Registry: r}}, 0)
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
				state.child = getPooledStateEncode(state.options)
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
				state.child = getPooledStateDecode(state.options)
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
		if err := r.addRegCache(tov); err != nil {
			return err
		}
		enc := regEncoder(name, fenc)
		r.encoders.Store(tov, enc)
		dec := &decoder{Type: tov, Decode: fdec}
		r.decoders.Store(name, dec)
		info := r.registerInfo(tov, "map", schemaFor(tov))
		enc.Info = info
		dec.Info = info

	default:
		return fmt.Errorf("type %v is not supported", tov)
	}

	return nil
}

func (r *Registry) registerInfo(t reflect.Type, kind, schema string) *gen.RegisteredTypeInfo {
	custom := kind == "marshaler" || kind == "binarymarshaler"

	info := &gen.RegisteredTypeInfo{
		ID:           r.registerOrder.Add(1),
		Name:         regTypeName(t),
		Kind:         kind,
		Schema:       schema,
		MinSize:      r.measureZeroSize(t, kind),
		SizeVariable: custom || hasVariableSize(t, make(map[reflect.Type]bool)),
		Stats:        gen.RegisteredTypeStats{Enabled: statsEnabled},
	}

	if actual, loaded := r.registeredTypes.LoadOrStore(t, info); loaded {
		return actual.(*gen.RegisteredTypeInfo)
	}
	r.typesByName.Store(info.Name, t)
	return info
}

func (r *Registry) measureZeroSize(t reflect.Type, kind string) (size uint32) {
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
	var known atomic.Uint64
	known.Store(math.MaxUint64)
	if err := Encode(v, buf, Options{Registry: r, Revision: &known}); err != nil {
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
		raw:    enc,
		Encode: func(value reflect.Value, b *lib.Buffer, state *stateEncode) error {
			if state.encodeType {
				if cached, ok := state.regPrefix(value.Type()); ok {
					return withPrefix(cached, enc, value, b, state)
				}
				return withPrefix(prefix, enc, value, b, state)
			}
			return enc(value, b, state)
		},
	}
}

func cachedEncoder(enc *encoder, prefix []byte) *encoder {
	raw := enc.raw
	return &encoder{
		Prefix: prefix,
		Info:   enc.Info,
		raw:    raw,
		Encode: func(value reflect.Value, b *lib.Buffer, state *stateEncode) error {
			if state.encodeType {
				return withPrefix(prefix, raw, value, b, state)
			}
			return raw(value, b, state)
		},
	}
}

func withPrefix(prefix []byte, enc encodeFunc, value reflect.Value, b *lib.Buffer, state *stateEncode) error {
	b.Append(prefix)
	state.encodeType = false
	err := enc(value, b, state)
	state.encodeType = true
	return err
}

// addRegCache assigns a 16-bit cache id to a registered type. Returns an error
// if the id space is exhausted (more than MaxUint16 registered types) or the
// type is already cached.
func (r *Registry) addRegCache(t reflect.Type) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exist := r.regCache.Load(t); exist {
		return gen.ErrTaken
	}
	id := r.regCacheID + 1
	if id > math.MaxUint16 {
		return fmt.Errorf("too many registered types")
	}
	r.regCacheID = id
	reg := []byte{edtReg, 0, 0}
	binary.BigEndian.PutUint16(reg[1:3], uint16(id))

	name := regTypeName(t)
	rev := uint64(len(r.log)) + 1
	r.regCache.Store(t, regEntry{reg, rev})
	r.regCache.Store(uint16(id), name)
	r.commit(published{kind: publishedType, id: uint16(id), name: name}, rev)
	return nil
}
