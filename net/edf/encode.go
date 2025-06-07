package edf

import (
	"encoding/binary"
	"fmt"
	"math"
	"reflect"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

var (
	ErrBinaryTooLong = fmt.Errorf("binary too long - max allowed length is 2^32-1 bytes (4GB)")
	ErrStringTooLong = fmt.Errorf("string too long - max allowed length is 2^16-1 (65535) bytes")
	ErrAtomTooLong   = fmt.Errorf("atom too long - max allowed length is 255 bytes")
	ErrErrorTooLong  = fmt.Errorf("error too long - max allowed length is 32767 bytes")
)

type stateEncode struct {
	child *stateEncode

	// TODO loop detection (in slices)
	//loop map[unsafe.Pointer]struct{}
	//ptr unsafe.Pointer

	encodeType bool

	options Options
	encoder encoder
}

// Encode
func Encode(x any, b *lib.Buffer, options Options) (ret error) {
	if x == nil {
		return fmt.Errorf("nothing to encode")
	}

	// Use pooled state instead of allocation
	state := getPooledStateEncode(options)
	defer putPooledStateEncode(state)

	xv := reflect.ValueOf(x)
	enc, err := getEncoder(xv.Type(), state)
	if err != nil {
		return err
	}

	if lib.Recover() {
		defer func() {
			if r := recover(); r != nil {
				ret = fmt.Errorf("%v", r)
			}
		}()
	}

	l := len(enc.Prefix)
	if l > 1 && enc.Prefix[0] != edtReg {
		buf := b.Extend(3)
		buf[0] = edtType
		binary.BigEndian.PutUint16(buf[1:3], uint16(l))
	}

	b.Append(enc.Prefix)
	return enc.Encode(xv, b, state)
}

func getEncoder(t reflect.Type, state *stateEncode) (*encoder, error) {
	// look in the common cache
	if state.options.Cache != nil {
		if v, found := state.options.Cache.Load(t); found {
			return v.(*encoder), nil
		}
	}

	// try to find by the type
	if v, found := encoders.Load(t); found {
		enc := v.(*encoder)

		if state.options.RegCache == nil {
			if state.options.Cache == nil {
				return enc, nil
			}
			// store it in the cache
			state.options.Cache.Store(t, v)
			return enc, nil
		}

		// check if this type is a registered one.
		if v, found := state.options.RegCache.Load(t); found {
			cachedenc := &encoder{
				Prefix: v.([]byte), // use cache ID (3 bytes only) instead of the full name
				Encode: enc.Encode,
			}
			if state.options.Cache == nil {
				return cachedenc, nil
			}
			// store encoder with cached prefix in the cache
			state.options.Cache.Store(t, cachedenc)
			return cachedenc, nil
		}

		if state.options.Cache == nil {
			return enc, nil
		}

		// store it in the cache
		state.options.Cache.Store(t, v)
		return enc, nil
	}

	kind := t.Kind()
	switch kind {
	case reflect.Map:
		encKey, err := getEncoder(t.Key(), state)
		if err != nil {
			return nil, err
		}
		encItem, err := getEncoder(t.Elem(), state)
		if err != nil {
			return nil, err
		}

		keyPrefix := encKey.Prefix
		if state.options.RegCache != nil {
			if v, found := state.options.RegCache.Load(t.Key()); found {
				keyPrefix = v.([]byte)
			}
		}
		itemPrefix := encItem.Prefix
		if state.options.RegCache != nil {
			if v, found := state.options.RegCache.Load(t.Elem()); found {
				itemPrefix = v.([]byte)
			}
		}

		prefix := append([]byte{edtMap}, keyPrefix...)
		prefix = append(prefix, itemPrefix...)

		fenc := func(value reflect.Value, b *lib.Buffer, state *stateEncode) error {
			if state.encodeType {
				buf := b.Extend(3)
				buf[0] = edtType
				binary.BigEndian.PutUint16(buf[1:3], uint16(len(prefix)))
				b.Append(prefix)
			}
			if value.IsNil() {
				b.AppendByte(edtNil)
				return nil
			} else {
				b.AppendByte(edtMap)
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
				if err := encItem.Encode(iter.Value(), b, state); err != nil {
					return err
				}
			}

			return nil
		}

		enc := &encoder{
			Prefix: prefix,
			Encode: fenc,
		}

		if state.options.Cache != nil {
			state.options.Cache.Store(t, enc)
		}

		return enc, nil

	case reflect.Slice:
		encItem, err := getEncoder(t.Elem(), state)
		if err != nil {
			return nil, err
		}

		itemPrefix := encItem.Prefix
		if state.options.RegCache != nil {
			if v, found := state.options.RegCache.Load(t.Elem()); found {
				itemPrefix = v.([]byte)
			}
		}
		prefix := append([]byte{edtSlice}, itemPrefix...)
		fenc := func(value reflect.Value, b *lib.Buffer, state *stateEncode) error {
			if state.encodeType {
				buf := b.Extend(3)
				buf[0] = edtType
				binary.BigEndian.PutUint16(buf[1:3], uint16(len(prefix)))
				b.Append(prefix)
			}

			if value.IsNil() {
				b.AppendByte(edtNil)
				return nil
			} else {
				b.AppendByte(edtSlice)
			}

			if state.child == nil {
				state.child = getPooledStateEncode(state.options)
			}
			state = state.child

			n := value.Len()
			buf := b.Extend(4)
			binary.BigEndian.PutUint32(buf, uint32(n))

			for i := 0; i < n; i++ {
				state.encodeType = false
				if err := encItem.Encode(value.Index(i), b, state); err != nil {
					return err
				}
			}

			return nil
		}

		enc := &encoder{
			Prefix: prefix,
			Encode: fenc,
		}
		if state.options.Cache != nil {
			state.options.Cache.Store(t, enc)
		}
		return enc, nil

	case reflect.Array:
		encItem, err := getEncoder(t.Elem(), state)
		if err != nil {
			return nil, err
		}

		itemPrefix := encItem.Prefix
		if state.options.RegCache != nil {
			if v, found := state.options.RegCache.Load(t.Elem()); found {
				itemPrefix = v.([]byte)
			}
		}

		l := t.Len()
		if l > math.MaxUint32 {
			return nil, fmt.Errorf("too big array size (allowed max: %d)", math.MaxUint32)
		}

		prefix := append([]byte{edtArray, 0, 0, 0, 0}, itemPrefix...)
		binary.BigEndian.PutUint32(prefix[1:5], uint32(l))

		fenc := func(value reflect.Value, b *lib.Buffer, state *stateEncode) error {
			if state.encodeType {
				buf := b.Extend(3)
				buf[0] = edtType
				binary.BigEndian.PutUint16(buf[1:3], uint16(len(prefix)))
				b.Append(prefix)
			}

			if state.child == nil {
				state.child = getPooledStateEncode(state.options)
			}
			state = state.child

			for i := 0; i < l; i++ {
				state.encodeType = false
				if err := encItem.Encode(value.Index(i), b, state); err != nil {
					return err
				}
			}

			return nil
		}

		enc := &encoder{
			Prefix: prefix,
			Encode: fenc,
		}
		if state.options.Cache != nil {
			state.options.Cache.Store(t, enc)
		}
		return enc, nil

	case reflect.Pointer:
		return nil, fmt.Errorf("pointer type is not supported")
	}

	// look among the standard types
	v, found := encoders.Load(kind)
	if found == false {
		return nil, fmt.Errorf("no encoder for type %v", t)
	}

	enc := v.(*encoder)
	// store encoder t => v
	encoders.Store(t, v)

	return enc, nil
}

func encodePID(value reflect.Value, b *lib.Buffer, state *stateEncode) error {
	if state.encodeType {
		b.AppendByte(edtPID)
	}

	pid := value.Interface().(gen.PID)

	if len(pid.Node) > 255 {
		return ErrAtomTooLong
	}
	writeAtom(pid.Node, b, state)

	buf := b.Extend(16)
	binary.BigEndian.PutUint64(buf[:8], pid.ID)
	binary.BigEndian.PutUint64(buf[8:16], uint64(pid.Creation))
	return nil
}

func encodeProcessID(value reflect.Value, b *lib.Buffer, state *stateEncode) error {
	if state.encodeType {
		b.AppendByte(edtProcessID)
	}

	p := value.Interface().(gen.ProcessID)

	if len(p.Node) > 255 {
		return ErrAtomTooLong
	}
	writeAtom(p.Node, b, state)

	if len(p.Name) > 255 {
		return ErrAtomTooLong
	}
	writeAtom(p.Name, b, state)

	return nil
}

func encodeRef(value reflect.Value, b *lib.Buffer, state *stateEncode) error {
	if state.encodeType {
		b.AppendByte(edtRef)
	}
	r := value.Interface().(gen.Ref)

	if len(r.Node) > 255 {
		return ErrAtomTooLong
	}
	writeAtom(r.Node, b, state)

	// 8 (creation) + 24 ([3]uint64)
	buf := b.Extend(8 + 24)
	binary.BigEndian.PutUint64(buf[0:8], uint64(r.Creation))
	binary.BigEndian.PutUint64(buf[8:16], r.ID[0])
	binary.BigEndian.PutUint64(buf[16:24], r.ID[1])
	binary.BigEndian.PutUint64(buf[24:32], r.ID[2])
	return nil
}

func encodeAlias(value reflect.Value, b *lib.Buffer, state *stateEncode) error {
	if state.encodeType {
		b.AppendByte(edtAlias)
	}
	a := value.Interface().(gen.Alias)

	if len(a.Node) > 255 {
		return ErrAtomTooLong
	}
	writeAtom(a.Node, b, state)

	// 8 (creation) + 24 ([3]uint64)
	buf := b.Extend(8 + 24)
	binary.BigEndian.PutUint64(buf[0:8], uint64(a.Creation))
	binary.BigEndian.PutUint64(buf[8:16], a.ID[0])
	binary.BigEndian.PutUint64(buf[16:24], a.ID[1])
	binary.BigEndian.PutUint64(buf[24:32], a.ID[2])
	return nil
}

func encodeEvent(value reflect.Value, b *lib.Buffer, state *stateEncode) error {
	if state.encodeType {
		b.AppendByte(edtEvent)
	}

	e := value.Interface().(gen.Event)

	if len(e.Node) > 255 {
		return ErrAtomTooLong
	}
	writeAtom(e.Node, b, state)

	if len(e.Name) > 255 {
		return ErrAtomTooLong
	}
	writeAtom(e.Name, b, state)

	return nil
}

func encodeAny(value reflect.Value, b *lib.Buffer, state *stateEncode) error {
	if value.IsNil() {
		b.AppendByte(edtNil)
		return nil
	}

	if state.child != nil {
		state.child = nil
	}
	enc, err := getEncoder(value.Elem().Type(), state)
	if err != nil {
		return err
	}

	state.encodeType = true
	return enc.Encode(value.Elem(), b, state)
}

func encodeTime(value reflect.Value, b *lib.Buffer, state *stateEncode) error {
	if state.encodeType {
		b.AppendByte(edtTime)
	}
	bin, err := value.Interface().(time.Time).MarshalBinary()
	if err != nil {
		return err
	}
	lbin := len(bin)
	buf := b.Extend(1 + lbin)
	buf[0] = byte(lbin)
	copy(buf[1:], bin)
	return nil
}

func encodeAtom(value reflect.Value, b *lib.Buffer, state *stateEncode) error {
	atom := value.Interface().(gen.Atom)
	if len(atom) > 255 {
		return ErrAtomTooLong
	}

	if state.encodeType {
		b.AppendByte(edtAtom)
	}

	writeAtom(atom, b, state)
	return nil
}

func encodeBool(value reflect.Value, b *lib.Buffer, state *stateEncode) error {
	if state.encodeType {
		b.AppendByte(edtBool)
	}
	if value.Bool() {
		b.AppendByte(1)
		return nil
	}
	b.AppendByte(0)
	return nil
}

func encodeString(value reflect.Value, b *lib.Buffer, state *stateEncode) error {
	s := value.String()
	// Use fast path for string encoding
	return encodeStringFast(s, b, state.encodeType)
}

func encodeBinary(value reflect.Value, b *lib.Buffer, state *stateEncode) error {
	data := value.Bytes()
	// Use fast path for binary encoding
	return encodeBinaryFast(data, b, state.encodeType)
}

func encodeInt(value reflect.Value, b *lib.Buffer, state *stateEncode) error {
	if state.encodeType {
		b.AppendByte(edtInt)
	}
	buf := b.Extend(8)
	binary.BigEndian.PutUint64(buf, uint64(value.Int()))
	return nil
}

func encodeInt8(value reflect.Value, b *lib.Buffer, state *stateEncode) error {
	if state.encodeType {
		b.AppendByte(edtInt8)
	}
	b.AppendByte(byte(value.Int()))
	return nil
}

func encodeInt16(value reflect.Value, b *lib.Buffer, state *stateEncode) error {
	if state.encodeType {
		b.AppendByte(edtInt16)
	}
	buf := b.Extend(2)
	binary.BigEndian.PutUint16(buf, uint16(value.Int()))
	return nil
}

func encodeInt32(value reflect.Value, b *lib.Buffer, state *stateEncode) error {
	if state.encodeType {
		b.AppendByte(edtInt32)
	}
	buf := b.Extend(4)
	binary.BigEndian.PutUint32(buf, uint32(value.Int()))
	return nil
}

func encodeInt64(value reflect.Value, b *lib.Buffer, state *stateEncode) error {
	v := value.Int()
	// Use fast path for int64 encoding
	return encodeInt64Fast(v, b, state.encodeType)
}

func encodeUint(value reflect.Value, b *lib.Buffer, state *stateEncode) error {
	if state.encodeType {
		b.AppendByte(edtUint)
	}
	buf := b.Extend(8)
	binary.BigEndian.PutUint64(buf, value.Uint())
	return nil
}

func encodeUint8(value reflect.Value, b *lib.Buffer, state *stateEncode) error {
	if state.encodeType {
		b.AppendByte(edtUint8)
	}
	b.AppendByte(byte(value.Uint()))
	return nil
}

func encodeUint16(value reflect.Value, b *lib.Buffer, state *stateEncode) error {
	if state.encodeType {
		b.AppendByte(edtUint16)
	}
	buf := b.Extend(2)
	binary.BigEndian.PutUint16(buf, uint16(value.Uint()))
	return nil
}

func encodeUint32(value reflect.Value, b *lib.Buffer, state *stateEncode) error {
	if state.encodeType {
		b.AppendByte(edtUint32)
	}
	buf := b.Extend(4)
	binary.BigEndian.PutUint32(buf, uint32(value.Uint()))
	return nil
}

func encodeUint64(value reflect.Value, b *lib.Buffer, state *stateEncode) error {
	if state.encodeType {
		b.AppendByte(edtUint64)
	}
	buf := b.Extend(8)
	binary.BigEndian.PutUint64(buf, value.Uint())
	return nil
}

func encodeFloat32(value reflect.Value, b *lib.Buffer, state *stateEncode) error {
	if state.encodeType {
		b.AppendByte(edtFloat32)
	}

	buf := b.Extend(4)
	binary.BigEndian.PutUint32(buf, math.Float32bits(float32(value.Float())))
	return nil
}

func encodeFloat64(value reflect.Value, b *lib.Buffer, state *stateEncode) error {
	if state.encodeType {
		b.AppendByte(edtFloat64)
	}
	buf := b.Extend(8)
	binary.BigEndian.PutUint64(buf, math.Float64bits(value.Float()))
	return nil
}

func encodeError(value reflect.Value, b *lib.Buffer, state *stateEncode) error {
	if value.IsNil() {
		b.Append([]byte{0xff, 0xff})
		return nil
	}

	if state.encodeType {
		b.AppendByte(edtError)
	}

	err := value.Interface().(error)
	if state.options.ErrCache != nil {
		if x, found := state.options.ErrCache.Load(err); found {
			id := x.(uint16)
			// atom cache id MUST be > math.MaxInt16, otherwise encode as a regular string
			if id > math.MaxInt16 {
				buf := b.Extend(2)
				binary.BigEndian.PutUint16(buf, id)
				return nil
			}
		}
	}

	estr := err.Error()
	lenErr := len(estr)
	if lenErr > math.MaxInt16 {
		return ErrErrorTooLong
	}

	buf := b.Extend(2 + lenErr)
	binary.BigEndian.PutUint16(buf[:2], uint16(lenErr))
	copy(buf[2:], estr)

	return nil
}

func writeAtom(atom gen.Atom, b *lib.Buffer, state *stateEncode) {
	// replace atom value if we have mapped value for it
	if state.options.AtomMapping != nil {
		v, found := state.options.AtomMapping.Load(atom)
		if found {
			atom = v.(gen.Atom)
		}
	}

	if state.options.AtomCache != nil {
		if x, found := state.options.AtomCache.Load(atom); found {
			id := x.(uint16)
			// atom cache id MUST be > 255, otherwise encode as a regular atom
			if id > 255 {
				buf := b.Extend(2)
				binary.BigEndian.PutUint16(buf, id)
				return
			}
		}
	}

	lenAtom := len(atom)
	buf := b.Extend(2 + lenAtom)
	binary.BigEndian.PutUint16(buf[:2], uint16(lenAtom))
	copy(buf[2:], atom)
}
