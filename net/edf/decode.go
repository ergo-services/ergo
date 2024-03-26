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
	errInternal  = fmt.Errorf("internal error")
	errDecodeEOD = fmt.Errorf("end of data")

	anyType = reflect.TypeOf((*any)(nil)).Elem()
	errType = reflect.TypeOf((*error)(nil)).Elem()
)

type stateDecode struct {
	child   *stateDecode
	options Options

	decodeType bool
	decoder    *decoder
}

// Decode
func Decode(packet []byte, options Options) (_ any, _ []byte, ret error) {
	if lib.Recover() {
		defer func() {
			if r := recover(); r != nil {
				ret = fmt.Errorf("%v", r)
			}
		}()
	}

	state := &stateDecode{
		options:    options,
		decodeType: true,
	}

	dec, packet, err := getDecoder(packet, state)
	if err != nil {
		return nil, nil, err
	}

	if dec == nil {
		return nil, packet, nil
	}
	state.decoder = dec
	v := reflect.Indirect(reflect.New(dec.Type))

	value, packet, err := dec.Decode(&v, packet, state)
	if err != nil {
		return nil, nil, fmt.Errorf("malformed EDF: %w", err)
	}
	return value.Interface(), packet, nil
}

func getDecoder(packet []byte, state *stateDecode) (*decoder, []byte, error) {
	if len(packet) == 0 {
		return nil, nil, errDecodeEOD
	}

	id := packet[0]
	packet = packet[1:]

	switch id {
	case edtReg:
		return getRegDecoder(packet, state)

	case edtType:
		if len(packet) < 2 {
			return nil, nil, errDecodeEOD
		}
		n := binary.BigEndian.Uint16(packet[:2])
		packet = packet[2:]

		if len(packet) < int(n) {
			return nil, nil, errDecodeEOD
		}

		dec, _, err := decodeType(packet[:n], state)
		if err != nil {
			return nil, nil, err
		}
		packet = packet[n:]
		return dec, packet, nil

	case edtNil:
		return nil, packet, nil
	}

	state.decodeType = false

	if v, found := decoders.Load(id); found {
		return v.(*decoder), packet, nil
	}

	return nil, nil, fmt.Errorf("unknown type %v for decoding", id)
}

func getRegDecoder(packet []byte, state *stateDecode) (*decoder, []byte, error) {
	var id string

	if len(packet) < 2 {
		return nil, nil, errDecodeEOD
	}

	n := binary.BigEndian.Uint16(packet[:2])
	packet = packet[2:]

	if n > 4095 {
		// cached. n is a cache id
		if state.options.RegCache == nil {
			return nil, nil, fmt.Errorf("no RegCache to decode cached id %d", n)
		}
		v, found := state.options.RegCache.Load(n)
		if found == false {
			return nil, nil, fmt.Errorf("unknown RegCache id %d", n)

		}
		id = v.(string)
	} else {
		if len(packet) < int(n) {
			return nil, nil, errDecodeEOD
		}
		id = string(packet[:n])
		packet = packet[n:]
	}

	if v, found := decoders.Load(id); found {
		state.decodeType = false
		return v.(*decoder), packet, nil
	}
	return nil, nil, fmt.Errorf("unknown reg type %v for decoding", id)
}

func decodeType(fold []byte, state *stateDecode) (*decoder, []byte, error) {
	// check in the decoder cache first
	if state.options.Cache != nil {
		if v, found := state.options.Cache.Load(string(fold)); found {
			return v.(*decoder), nil, nil
		}
	}

	if len(fold) == 0 {
		return nil, nil, errDecodeEOD
	}

	switch fold[0] {
	case edtMap:
		// unfold key type
		decKey, f, err := decodeType(fold[1:], state)
		if err != nil {
			return nil, nil, fmt.Errorf("unable to unfold type (map key): %s", err)
		}
		decValue, f, err := decodeType(f, state)
		if err != nil {
			return nil, nil, fmt.Errorf("unable to unfold type (map value): %s", err)
		}
		if len(f) > 0 {
			return nil, nil, fmt.Errorf("extra data in folded type (map): %#v", f)
		}

		vtype := reflect.MapOf(decKey.Type, decValue.Type)

		fdec := func(value *reflect.Value, packet []byte, state *stateDecode) (*reflect.Value, []byte, error) {
			if len(packet) == 0 {
				return nil, nil, errDecodeEOD
			}

			if packet[0] == edtNil {
				packet = packet[1:]
				return nil, packet, nil
			}

			if packet[0] != edtMap {
				return nil, nil, fmt.Errorf("incorrect map type %d", packet[0])
			}
			packet = packet[1:]

			if len(packet) < 4 {
				return nil, nil, errDecodeEOD
			}

			n := int(binary.BigEndian.Uint32(packet[:4]))
			packet = packet[4:]

			if n == 0 {
				x := reflect.MakeMap(vtype)
				if value == nil {
					value = &x
				} else {
					value.Set(x)
				}
				return value, packet, nil
			}

			if n > len(packet) {
				return nil, nil, fmt.Errorf("incorrect data length")
			}

			x := reflect.MakeMapWithSize(vtype, n)
			if value == nil {
				value = &x
			} else {
				value.Set(x)
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

		dec := decoder{
			Type:   vtype,
			Decode: fdec,
		}
		if state.options.Cache != nil {
			state.options.Cache.LoadOrStore(string(fold), &dec)
		}

		return &dec, nil, nil

	case edtSlice:
		// unfold key type
		decItem, f, err := decodeType(fold[1:], state)
		if err != nil {
			return nil, nil, fmt.Errorf("unable to unfold type (slice): %s", err)
		}
		if len(f) > 0 {
			return nil, nil, fmt.Errorf("extra data in folded type (slice): %#v", f)
		}

		vtype := reflect.SliceOf(decItem.Type)

		fdec := func(value *reflect.Value, packet []byte, state *stateDecode) (*reflect.Value, []byte, error) {
			if len(packet) == 0 {
				return nil, nil, errDecodeEOD
			}

			if packet[0] == edtNil {
				packet = packet[1:]
				return nil, packet, nil
			}

			if packet[0] != edtSlice {
				return nil, nil, fmt.Errorf("incorrect slice type %d", packet[0])
			}
			packet = packet[1:]

			if len(packet) < 4 {
				return nil, nil, errDecodeEOD
			}

			n := int(binary.BigEndian.Uint32(packet[:4]))
			packet = packet[4:]

			if n == 0 {
				x := reflect.MakeSlice(vtype, 0, 0)
				if value == nil {
					value = &x
				} else {
					value.Set(x)
				}
				return value, packet, nil
			}

			if n > len(packet) {
				return nil, nil, fmt.Errorf("incorrect data length")
			}

			x := reflect.MakeSlice(vtype, n, n)
			if value == nil {
				value = &x
			} else {
				value.Set(x)
			}

			if state.child == nil {
				state.child = &stateDecode{
					options: state.options,
					decoder: decItem,
				}
			}
			state = state.child

			for i := 0; i < n; i++ {
				item := value.Index(i)
				_, p, err := decItem.Decode(&item, packet, state)
				if err != nil {
					return nil, nil, err
				}
				packet = p
			}

			return value, packet, nil
		}

		dec := decoder{
			Type:   vtype,
			Decode: fdec,
		}
		if state.options.Cache != nil {
			state.options.Cache.LoadOrStore(string(fold), &dec)
		}

		return &dec, nil, nil

	case edtArray:
		// length of the array
		if len(fold) < 6 {
			return nil, nil, errDecodeEOD
		}

		n := int(binary.BigEndian.Uint32(fold[1:5]))

		// unfold key type
		decItem, f, err := decodeType(fold[5:], state)
		if err != nil {
			return nil, nil, fmt.Errorf("unable to unfold type (array): %s", err)
		}
		if len(f) > 0 {
			return nil, nil, fmt.Errorf("extra data in folded type (array): %#v", f)
		}

		vtype := reflect.ArrayOf(n, decItem.Type)

		fdec := func(value *reflect.Value, packet []byte, state *stateDecode) (*reflect.Value, []byte, error) {
			if len(packet) == 0 {
				if n == 0 {
					return value, packet, nil
				}
				return nil, nil, errDecodeEOD
			}

			if value == nil {
				x := reflect.Indirect(reflect.New(vtype))
				value = &x
			}

			if state.child == nil {
				state.child = &stateDecode{
					options: state.options,
					decoder: decItem,
				}
			}
			state = state.child

			for i := 0; i < n; i++ {
				item := value.Index(i)
				_, p, err := decItem.Decode(&item, packet, state)
				if err != nil {
					return nil, nil, err
				}
				packet = p
			}

			return value, packet, nil
		}

		dec := decoder{
			Type:   vtype,
			Decode: fdec,
		}

		if state.options.Cache != nil {
			state.options.Cache.LoadOrStore(string(fold), &dec)
		}

		return &dec, nil, nil

	case edtReg:
		return getRegDecoder(fold[1:], state)
	}

	if v, found := decoders.Load(fold[0]); found {
		return v.(*decoder), fold[1:], nil
	}
	return nil, nil, fmt.Errorf("no decoder for type %d", fold[0])
}

func decodePID(value *reflect.Value, packet []byte, state *stateDecode) (*reflect.Value, []byte, error) {
	if state.decodeType {
		if len(packet) < 1 {
			return nil, nil, errDecodeEOD
		}
		t := packet[0]
		if t != edtPID {
			return nil, nil, fmt.Errorf("incorrect gen.PID type id %d", t)
		}
		packet = packet[1:]
	}

	var pid gen.PID
	var err error

	pid.Node, packet, err = readAtom(packet, state)
	if err != nil {
		return nil, nil, err
	}

	if len(packet) < 16 {
		return nil, nil, errDecodeEOD
	}
	pid.ID = binary.BigEndian.Uint64(packet[:8])
	pid.Creation = int64(binary.BigEndian.Uint64(packet[8:16]))
	packet = packet[16:]

	v := reflect.ValueOf(pid)
	if value == nil {
		return &v, packet, nil
	}
	// should we check the value type?
	value.Set(v)
	return value, packet, nil
}

func decodeProcessID(value *reflect.Value, packet []byte, state *stateDecode) (*reflect.Value, []byte, error) {
	if state.decodeType {
		if len(packet) < 1 {
			return nil, nil, errDecodeEOD
		}
		t := packet[0]
		if t != edtProcessID {
			return nil, nil, fmt.Errorf("incorrect gen.ProcessID type id %d", t)
		}
		packet = packet[1:]
	}

	var p gen.ProcessID
	var err error

	if p.Node, packet, err = readAtom(packet, state); err != nil {
		return nil, nil, err
	}
	if p.Name, packet, err = readAtom(packet, state); err != nil {
		return nil, nil, err
	}

	v := reflect.ValueOf(p)
	if value == nil {
		return &v, packet, nil
	}
	// should we check the value type?
	value.Set(v)
	return value, packet, nil
}

func decodeRef(value *reflect.Value, packet []byte, state *stateDecode) (*reflect.Value, []byte, error) {
	if state.decodeType {
		if len(packet) < 1 {
			return nil, nil, errDecodeEOD
		}
		t := packet[0]
		if t != edtRef {
			return nil, nil, fmt.Errorf("incorrect gen.Ref type id %d", t)
		}
		packet = packet[1:]
	}

	var ref gen.Ref
	var err error

	if ref.Node, packet, err = readAtom(packet, state); err != nil {
		return nil, nil, err
	}

	// 8 (creation) + 12 ([3]uint32)
	if len(packet) < 20 {
		return nil, nil, errDecodeEOD
	}
	ref.Creation = int64(binary.BigEndian.Uint64(packet[:8]))
	ref.ID[0] = binary.BigEndian.Uint32(packet[8:12])
	ref.ID[1] = binary.BigEndian.Uint32(packet[12:16])
	ref.ID[2] = binary.BigEndian.Uint32(packet[16:20])
	packet = packet[20:]

	v := reflect.ValueOf(ref)
	if value == nil {
		return &v, packet, nil
	}
	// should we check the value type?
	value.Set(v)
	return value, packet, nil
}

func decodeAlias(value *reflect.Value, packet []byte, state *stateDecode) (*reflect.Value, []byte, error) {
	if state.decodeType {
		if len(packet) < 1 {
			return nil, nil, errDecodeEOD
		}
		t := packet[0]
		if t != edtAlias {
			return nil, nil, fmt.Errorf("incorrect gen.Alias type id %d", t)
		}
		packet = packet[1:]
	}

	var alias gen.Alias
	var err error

	if alias.Node, packet, err = readAtom(packet, state); err != nil {
		return nil, nil, err
	}

	// 8 (creation) + 12 ([3]uint32)
	if len(packet) < 20 {
		return nil, nil, errDecodeEOD
	}
	alias.Creation = int64(binary.BigEndian.Uint64(packet[:8]))
	alias.ID[0] = binary.BigEndian.Uint32(packet[8:12])
	alias.ID[1] = binary.BigEndian.Uint32(packet[12:16])
	alias.ID[2] = binary.BigEndian.Uint32(packet[16:20])
	packet = packet[20:]

	v := reflect.ValueOf(alias)
	if value == nil {
		return &v, packet, nil
	}
	// should we check the value type?
	value.Set(v)
	return value, packet, nil
}

func decodeEvent(value *reflect.Value, packet []byte, state *stateDecode) (*reflect.Value, []byte, error) {
	if state.decodeType {
		if len(packet) < 1 {
			return nil, nil, errDecodeEOD
		}
		t := packet[0]
		if t != edtEvent {
			return nil, nil, fmt.Errorf("incorrect gen.Event type id %d", t)
		}
		packet = packet[1:]
	}

	var e gen.Event
	var err error

	if e.Node, packet, err = readAtom(packet, state); err != nil {
		return nil, nil, err
	}
	if e.Name, packet, err = readAtom(packet, state); err != nil {
		return nil, nil, err
	}
	v := reflect.ValueOf(e)
	if value == nil {
		return &v, packet, nil
	}
	// should we check the value type?
	value.Set(v)
	return value, packet, nil
}

func decodeTime(value *reflect.Value, packet []byte, state *stateDecode) (*reflect.Value, []byte, error) {
	if state.decodeType {
		if len(packet) < 1 {
			return nil, nil, errDecodeEOD
		}
		t := packet[0]
		if t != edtTime {
			return nil, nil, fmt.Errorf("incorrect time.Time type id %d", t)
		}
		packet = packet[1:]
	}
	if len(packet) == 0 {
		return nil, nil, errDecodeEOD
	}
	l := int(packet[0])
	if len(packet) < 1+l {
		return nil, nil, errDecodeEOD
	}

	var t time.Time
	if err := t.UnmarshalBinary(packet[1 : 1+l]); err != nil {
		return nil, nil, err
	}
	packet = packet[1+l:]

	v := reflect.ValueOf(t)
	if value == nil {
		return &v, packet, nil
	}
	// should we check the value type?
	value.Set(v)
	return value, packet, nil
}

func decodeString(value *reflect.Value, packet []byte, state *stateDecode) (*reflect.Value, []byte, error) {
	if state.decodeType {
		if len(packet) < 1 {
			return nil, nil, errDecodeEOD
		}
		t := packet[0]
		if t != edtString {
			return nil, nil, fmt.Errorf("incorrect string type id %d", t)
		}
		packet = packet[1:]
	}
	if len(packet) < 2 {
		return nil, nil, errDecodeEOD
	}
	l := binary.BigEndian.Uint16(packet)
	if len(packet) < int(2+l) {
		return nil, nil, errDecodeEOD
	}

	s := string(packet[2 : 2+l])
	packet = packet[2+l:]

	if value == nil {
		v := reflect.ValueOf(s)
		return &v, packet, nil
	}

	if value.Kind() == reflect.String {
		value.SetString(s)
		return value, packet, nil
	}

	value.Set(reflect.ValueOf(s))
	return value, packet, nil
}

func decodeBinary(value *reflect.Value, packet []byte, state *stateDecode) (*reflect.Value, []byte, error) {
	if state.decodeType {
		if len(packet) < 1 {
			return nil, nil, errDecodeEOD
		}
		t := packet[0]
		if t != edtBinary {
			return nil, nil, fmt.Errorf("incorrect []byte type id %d", t)
		}
		packet = packet[1:]
	}
	if len(packet) < 4 {
		return nil, nil, errDecodeEOD
	}
	l := binary.BigEndian.Uint32(packet)
	if len(packet) < int(4+l) {
		return nil, nil, errDecodeEOD
	}

	// we can't reuse the underlying slice since it is a part of the buffer
	// which is bringing back to the buffer pool after all.
	bin := append([]byte{}, packet[4:4+l]...)
	packet = packet[4+l:]

	if value == nil {
		v := reflect.ValueOf(bin)
		return &v, packet, nil
	}
	if value.Kind() == reflect.Interface { // if type is 'any'
		value.Set(reflect.ValueOf(bin))
		return value, packet, nil
	}

	value.SetBytes(bin)
	return value, packet, nil
}

func decodeAtom(value *reflect.Value, packet []byte, state *stateDecode) (*reflect.Value, []byte, error) {
	if state.decodeType {
		if len(packet) < 1 {
			return nil, nil, errDecodeEOD
		}
		t := packet[0]
		if t != edtAtom {
			return nil, nil, fmt.Errorf("incorrect gen.Atom type id %d", t)
		}
		packet = packet[1:]
	}

	atom, p, err := readAtom(packet, state)
	if err != nil {
		return nil, nil, err
	}
	packet = p
	v := reflect.ValueOf(atom)
	if value == nil {
		return &v, packet, nil
	}
	value.Set(v)
	return value, packet, nil
}

func decodeInt(value *reflect.Value, packet []byte, state *stateDecode) (*reflect.Value, []byte, error) {
	if state.decodeType {
		if len(packet) < 1 {
			return nil, nil, errDecodeEOD
		}
		t := packet[0]
		if t != edtInt {
			return nil, nil, fmt.Errorf("incorrect int type id %d", t)
		}
		packet = packet[1:]
	}
	if len(packet) < 8 {
		return nil, nil, errDecodeEOD
	}
	v := binary.BigEndian.Uint64(packet[:8])
	packet = packet[8:]
	if value == nil {
		v := reflect.ValueOf(int(v))
		return &v, packet, nil
	}

	if value.Kind() == reflect.Int {
		value.SetInt(int64(v))
		return value, packet, nil
	}

	value.Set(reflect.ValueOf(int(v)))
	return value, packet, nil
}

func decodeInt8(value *reflect.Value, packet []byte, state *stateDecode) (*reflect.Value, []byte, error) {
	if state.decodeType {
		if len(packet) < 1 {
			return nil, nil, errDecodeEOD
		}
		t := packet[0]
		if t != edtInt8 {
			return nil, nil, fmt.Errorf("incorrect int8 type id %d", t)
		}
		packet = packet[1:]
	}

	if len(packet) == 0 {
		return nil, nil, errDecodeEOD
	}

	v := packet[0]
	packet = packet[1:]

	if value == nil {
		v := reflect.ValueOf(int8(v))
		return &v, packet, nil
	}

	if value.Kind() == reflect.Int8 {
		value.SetInt(int64(v))
		return value, packet, nil
	}

	value.Set(reflect.ValueOf(int8(v)))
	return value, packet, nil
}

func decodeInt16(value *reflect.Value, packet []byte, state *stateDecode) (*reflect.Value, []byte, error) {
	if state.decodeType {
		if len(packet) < 1 {
			return nil, nil, errDecodeEOD
		}
		t := packet[0]
		if t != edtInt16 {
			return nil, nil, fmt.Errorf("incorrect int16 type id %d", t)
		}
		packet = packet[1:]
	}

	if len(packet) < 2 {
		return nil, nil, errDecodeEOD
	}

	v := binary.BigEndian.Uint16(packet[:2])
	packet = packet[2:]

	if value == nil {
		v := reflect.ValueOf(int16(v))
		return &v, packet, nil
	}

	if value.Kind() == reflect.Int16 {
		value.SetInt(int64(v))
		return value, packet, nil
	}

	value.Set(reflect.ValueOf(int16(v)))
	return value, packet, nil
}

func decodeInt32(value *reflect.Value, packet []byte, state *stateDecode) (*reflect.Value, []byte, error) {
	if state.decodeType {
		if len(packet) < 1 {
			return nil, nil, errDecodeEOD
		}
		t := packet[0]
		if t != edtInt32 {
			return nil, nil, fmt.Errorf("incorrect int32 type id %d", t)
		}
		packet = packet[1:]
	}
	if len(packet) < 4 {
		return nil, nil, errDecodeEOD
	}
	v := binary.BigEndian.Uint32(packet[:4])
	packet = packet[4:]
	if value == nil {
		v := reflect.ValueOf(int32(v))
		return &v, packet, nil
	}
	if value.Kind() == reflect.Int32 {
		value.SetInt(int64(v))
		return value, packet, nil
	}

	value.Set(reflect.ValueOf(int32(v)))
	return value, packet, nil
}

func decodeInt64(value *reflect.Value, packet []byte, state *stateDecode) (*reflect.Value, []byte, error) {
	if state.decodeType {
		if len(packet) < 1 {
			return nil, nil, errDecodeEOD
		}
		t := packet[0]
		if t != edtInt64 {
			return nil, nil, fmt.Errorf("incorrect int64 type id %d", t)
		}
		packet = packet[1:]
	}
	if len(packet) < 8 {
		return nil, nil, errDecodeEOD
	}
	v := int64(binary.BigEndian.Uint64(packet[:8]))
	packet = packet[8:]

	if value == nil {
		v := reflect.ValueOf(v)
		return &v, packet, nil
	}
	if value.Kind() == reflect.Int64 {
		value.SetInt(v)
		return value, packet, nil
	}

	value.Set(reflect.ValueOf(v))
	return value, packet, nil
}

func decodeUint(value *reflect.Value, packet []byte, state *stateDecode) (*reflect.Value, []byte, error) {
	if state.decodeType {
		if len(packet) < 1 {
			return nil, nil, errDecodeEOD
		}
		t := packet[0]
		if t != edtUint {
			return nil, nil, fmt.Errorf("incorrect uint type id %d", t)
		}
		packet = packet[1:]
	}

	if len(packet) < 8 {
		return nil, nil, errDecodeEOD
	}

	v := binary.BigEndian.Uint64(packet[:8])
	packet = packet[8:]

	if value == nil {
		v := reflect.ValueOf(uint(v))
		return &v, packet, nil
	}

	if value.Kind() == reflect.Uint {
		value.SetUint(uint64(v))
		return value, packet, nil
	}

	value.Set(reflect.ValueOf(uint(v)))
	return value, packet, nil
}

func decodeUint8(value *reflect.Value, packet []byte, state *stateDecode) (*reflect.Value, []byte, error) {
	if state.decodeType {
		if len(packet) < 1 {
			return nil, nil, errDecodeEOD
		}
		t := packet[0]
		if t != edtUint8 {
			return nil, nil, fmt.Errorf("incorrect uint8 type id %d", t)
		}
		packet = packet[1:]
	}

	if len(packet) == 0 {
		return nil, nil, errDecodeEOD
	}

	v := packet[0]
	packet = packet[1:]

	if value == nil {
		v := reflect.ValueOf(uint8(v))
		return &v, packet, nil
	}

	if value.Kind() == reflect.Uint8 {
		value.SetUint(uint64(v))
		return value, packet, nil
	}

	value.Set(reflect.ValueOf(uint8(v)))
	return value, packet, nil
}

func decodeUint16(value *reflect.Value, packet []byte, state *stateDecode) (*reflect.Value, []byte, error) {
	if state.decodeType {
		if len(packet) < 1 {
			return nil, nil, errDecodeEOD
		}
		t := packet[0]
		if t != edtUint16 {
			return nil, nil, fmt.Errorf("incorrect uint16 type id %d", t)
		}
		packet = packet[1:]
	}

	if len(packet) < 2 {
		return nil, nil, errDecodeEOD
	}

	v := binary.BigEndian.Uint16(packet[:2])
	packet = packet[2:]

	if value == nil {
		v := reflect.ValueOf(v)
		return &v, packet, nil
	}

	if value.Kind() == reflect.Uint16 {
		value.SetUint(uint64(v))
		return value, packet, nil
	}

	value.Set(reflect.ValueOf(v))
	return value, packet, nil
}

func decodeUint32(value *reflect.Value, packet []byte, state *stateDecode) (*reflect.Value, []byte, error) {
	if state.decodeType {
		if len(packet) < 1 {
			return nil, nil, errDecodeEOD
		}
		t := packet[0]
		if t != edtUint32 {
			return nil, nil, fmt.Errorf("incorrect uint32 type id %d", t)
		}
		packet = packet[1:]
	}

	if len(packet) < 4 {
		return nil, nil, errDecodeEOD
	}

	v := binary.BigEndian.Uint32(packet[:4])
	packet = packet[4:]

	if value == nil {
		v := reflect.ValueOf(v)
		return &v, packet, nil
	}

	if value.Kind() == reflect.Uint32 {
		value.SetUint(uint64(v))
		return value, packet, nil
	}

	value.Set(reflect.ValueOf(v))
	return value, packet, nil
}

func decodeUint64(value *reflect.Value, packet []byte, state *stateDecode) (*reflect.Value, []byte, error) {
	if state.decodeType {
		if len(packet) < 1 {
			return nil, nil, errDecodeEOD
		}
		t := packet[0]
		if t != edtUint64 {
			return nil, nil, fmt.Errorf("incorrect uint64 type id %d", t)
		}
		packet = packet[1:]
	}

	if len(packet) < 8 {
		return nil, nil, errDecodeEOD
	}

	v := binary.BigEndian.Uint64(packet[:8])
	packet = packet[8:]

	if value == nil {
		v := reflect.ValueOf(v)
		return &v, packet, nil
	}

	if value.Kind() == reflect.Uint64 {
		value.SetUint(v)
		return value, packet, nil
	}

	value.Set(reflect.ValueOf(v))
	return value, packet, nil
}

func decodeFloat32(value *reflect.Value, packet []byte, state *stateDecode) (*reflect.Value, []byte, error) {
	if state.decodeType {
		if len(packet) < 1 {
			return nil, nil, errDecodeEOD
		}
		t := packet[0]
		if t != edtFloat32 {
			return nil, nil, fmt.Errorf("incorrect float32 type id %d", t)
		}
		packet = packet[1:]
	}

	if len(packet) < 4 {
		return nil, nil, errDecodeEOD
	}
	bits := binary.BigEndian.Uint32(packet[:4])
	packet = packet[4:]

	if value == nil {
		v := reflect.ValueOf(math.Float32frombits(bits))
		return &v, packet, nil
	}

	if value.Kind() == reflect.Float32 {
		value.SetFloat(float64(math.Float32frombits(bits)))
		return value, packet, nil
	}

	value.Set(reflect.ValueOf(math.Float32frombits(bits)))
	return value, packet, nil
}

func decodeFloat64(value *reflect.Value, packet []byte, state *stateDecode) (*reflect.Value, []byte, error) {
	if state.decodeType {
		if len(packet) < 1 {
			return nil, nil, errDecodeEOD
		}
		t := packet[0]
		if t != edtFloat64 {
			return nil, nil, fmt.Errorf("incorrect float64 type id %d", t)
		}
		packet = packet[1:]
	}

	if len(packet) < 8 {
		return nil, nil, fmt.Errorf("float64. not enough data")
	}
	bits := binary.BigEndian.Uint64(packet[:8])
	packet = packet[8:]

	if value == nil {
		v := reflect.ValueOf(math.Float64frombits(bits))
		return &v, packet, nil
	}
	if value.Kind() == reflect.Float64 {
		value.SetFloat(math.Float64frombits(bits))
		return value, packet, nil
	}

	value.Set(reflect.ValueOf(math.Float64frombits(bits)))
	return value, packet, nil
}

func decodeBool(value *reflect.Value, packet []byte, state *stateDecode) (*reflect.Value, []byte, error) {
	if state.decodeType {
		if len(packet) < 1 {
			return nil, nil, errDecodeEOD
		}
		t := packet[0]
		if t != edtBool {
			return nil, nil, fmt.Errorf("incorrect bool type id %d", t)
		}
		packet = packet[1:]
	}
	if len(packet) == 0 {
		return nil, nil, errDecodeEOD
	}
	b := packet[0] == 1
	packet = packet[1:]
	if value == nil {
		v := reflect.ValueOf(b)
		return &v, packet, nil
	}
	if value.Kind() == reflect.Bool {
		value.SetBool(b)
		return value, packet, nil
	}

	value.Set(reflect.ValueOf(b))
	return value, packet, nil
}

func decodeAny(value *reflect.Value, packet []byte, state *stateDecode) (*reflect.Value, []byte, error) {
	dec, p, err := getDecoder(packet, state)
	if err != nil {
		return nil, nil, err
	}

	if dec == nil {
		return value, p, nil
	}

	if value == nil {
		v := reflect.Indirect(reflect.New(dec.Type))
		value, packet, err = dec.Decode(&v, p, state)
		if err != nil {
			return nil, nil, err
		}
		return value, packet, nil
	}

	if value.Type() == dec.Type {
		value, packet, err = dec.Decode(value, p, state)
		if err != nil {
			return nil, nil, err
		}
		return value, packet, nil
	}

	v := reflect.Indirect(reflect.New(dec.Type))
	_, packet, err = dec.Decode(&v, p, state)
	if err != nil {
		return nil, nil, err
	}
	value.Set(v)

	return value, packet, nil
}
func decodeError(value *reflect.Value, packet []byte, state *stateDecode) (*reflect.Value, []byte, error) {
	var err error

	if state.decodeType {
		if len(packet) == 0 {
			return nil, nil, errDecodeEOD
		}

		if packet[0] != edtError {
			return nil, nil, fmt.Errorf("incorrect error type id %d", packet[0])
		}
		packet = packet[1:]
	}

	if len(packet) < 2 {
		return nil, nil, errDecodeEOD
	}

	id := binary.BigEndian.Uint16(packet)
	packet = packet[2:]

	if id == math.MaxUint16 {
		// nil value
		return value, packet, nil
	}

	if id > math.MaxInt16 {
		// registered error
		if state.options.ErrCache == nil {
			return nil, nil, fmt.Errorf("no ErrCache to decode id %d", id)
		}

		v, found := state.options.ErrCache.Load(id)
		if found == false {
			return nil, nil, fmt.Errorf("unknown ErrCache id %d", id)
		}
		err = v.(error)
	} else {
		// regular error. id has a length value
		l := int(id)
		if len(packet) < l {
			return nil, nil, errDecodeEOD
		}
		err = fmt.Errorf(string(packet[:l]))
		packet = packet[l:]
	}

	v := reflect.ValueOf(err)
	if value == nil {
		return &v, packet, nil
	}
	value.Set(v)
	return value, packet, nil
}

func readAtom(packet []byte, state *stateDecode) (gen.Atom, []byte, error) {
	var atom gen.Atom

	if len(packet) < 2 {
		return atom, nil, errDecodeEOD
	}

	id := binary.BigEndian.Uint16(packet)
	packet = packet[2:]

	if id > 255 {
		// cached atom
		if state.options.AtomCache == nil {
			return atom, nil, fmt.Errorf("no AtomCache to decode id %d", id)
		}
		v, found := state.options.AtomCache.Load(id)
		if found == false {
			return atom, nil, fmt.Errorf("unknown AtomCache id %d", id)
		}
		atom = v.(gen.Atom)
	} else {
		// this is atom and 'id' is the len of it
		l := int(id) // len
		if len(packet) < l {
			return atom, nil, errDecodeEOD
		}
		atom = gen.Atom(packet[:l])
		packet = packet[l:]
	}

	if state.options.AtomMapping != nil {
		if v, found := state.options.AtomMapping.Load(atom); found {
			return v.(gen.Atom), packet, nil
		}
	}

	return atom, packet, nil
}
