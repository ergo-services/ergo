package edf

import (
	"encoding/binary"
	"fmt"
	"math"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

type decoder struct {
	Type   reflect.Type
	Decode func(*reflect.Value, []byte, *stateDecode) (*reflect.Value, []byte, error)
}

type encodeFunc func(value reflect.Value, b *lib.Buffer, state *stateEncode) error
type encoder struct {
	Prefix []byte
	Encode encodeFunc
}

func regTypeName(t reflect.Type) string {
	return fmt.Sprintf("#%s/%s", t.PkgPath(), t.Name())
}

func RegisterTypeOf(v any) error {
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
			buf := b.Extend(4)
			l := b.Len()
			if err := v.MarshalEDF(b); err != nil {
				return err
			}

			lenBinary := b.Len() - l
			if int64(lenBinary) > int64(math.MaxUint32-1) {
				return ErrBinaryTooLong
			}
			binary.BigEndian.PutUint32(buf, uint32(lenBinary))
			return nil
		}
		encoders.Store(tov, regEncoder(name, fenc))

		fdec := func(value *reflect.Value, packet []byte, state *stateDecode) (*reflect.Value, []byte, error) {
			if len(packet) < 4 {
				return nil, nil, errDecodeEOD
			}

			l := binary.BigEndian.Uint32(packet)
			if len(packet) < int(l+4) {
				return nil, nil, errDecodeEOD
			}

			if value == nil {
				v := reflect.Indirect(reflect.New(state.decoder.Type))
				value = &v
			}

			v := value.Addr().Interface().(Unmarshaler)
			if err := v.UnmarshalEDF(packet[4 : l+4]); err != nil {
				return nil, nil, err
			}

			packet = packet[l+4:]
			return value, packet, nil
		}
		dec := &decoder{tov, fdec}
		decoders.Store(name, dec)
		decoders.Store(tov, dec)
		addRegCache(tov)
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
		encoders.Store(tov, regEncoder(name, encodeBool))
		dec := &decoder{tov, decodeBool}
		decoders.Store(name, dec)
		decoders.Store(tov, dec)
		addRegCache(tov)
		return nil

	case reflect.Int:
		encoders.Store(tov, regEncoder(name, encodeInt))
		dec := &decoder{tov, decodeInt}
		decoders.Store(name, dec)
		decoders.Store(tov, dec)
		addRegCache(tov)
		return nil

	case reflect.Int8:
		encoders.Store(tov, regEncoder(name, encodeInt8))
		dec := &decoder{tov, decodeInt8}
		decoders.Store(name, dec)
		decoders.Store(tov, dec)
		addRegCache(tov)
		return nil

	case reflect.Int16:
		encoders.Store(tov, regEncoder(name, encodeInt16))
		dec := &decoder{tov, decodeInt16}
		decoders.Store(name, dec)
		decoders.Store(tov, dec)
		addRegCache(tov)
		return nil

	case reflect.Int32:
		encoders.Store(tov, regEncoder(name, encodeInt32))
		dec := &decoder{tov, decodeInt32}
		decoders.Store(name, dec)
		decoders.Store(tov, dec)
		addRegCache(tov)
		return nil

	case reflect.Int64:
		encoders.Store(tov, regEncoder(name, encodeInt64))
		dec := &decoder{tov, decodeInt64}
		decoders.Store(name, dec)
		decoders.Store(tov, dec)
		addRegCache(tov)
		return nil

	case reflect.Uint:
		encoders.Store(tov, regEncoder(name, encodeUint))
		dec := &decoder{tov, decodeUint}
		decoders.Store(name, dec)
		decoders.Store(tov, dec)
		addRegCache(tov)
		return nil

	case reflect.Uint8:
		encoders.Store(tov, regEncoder(name, encodeUint8))
		dec := &decoder{tov, decodeUint8}
		decoders.Store(name, dec)
		decoders.Store(tov, dec)
		addRegCache(tov)
		return nil

	case reflect.Uint16:
		encoders.Store(tov, regEncoder(name, encodeUint16))
		dec := &decoder{tov, decodeUint16}
		decoders.Store(name, dec)
		decoders.Store(tov, dec)
		addRegCache(tov)
		return nil

	case reflect.Uint32:
		encoders.Store(tov, regEncoder(name, encodeUint32))
		dec := &decoder{tov, decodeUint32}
		decoders.Store(name, dec)
		decoders.Store(tov, dec)
		addRegCache(tov)
		return nil

	case reflect.Uint64:
		encoders.Store(tov, regEncoder(name, encodeUint64))
		dec := &decoder{tov, decodeUint64}
		decoders.Store(name, dec)
		decoders.Store(tov, dec)
		addRegCache(tov)
		return nil

	case reflect.Float32:
		encoders.Store(tov, regEncoder(name, encodeFloat32))
		dec := &decoder{tov, decodeFloat32}
		decoders.Store(name, dec)
		decoders.Store(tov, dec)
		addRegCache(tov)
		return nil

	case reflect.Float64:
		encoders.Store(tov, regEncoder(name, encodeFloat64))
		dec := &decoder{tov, decodeFloat64}
		decoders.Store(name, dec)
		decoders.Store(tov, dec)
		addRegCache(tov)
		return nil

	case reflect.String:
		encoders.Store(tov, regEncoder(name, encodeString))
		dec := &decoder{tov, decodeString}
		decoders.Store(name, dec)
		decoders.Store(tov, dec)
		addRegCache(tov)
		return nil

	case reflect.Struct:
		var encs []*encoder
		var decs []*decoder

		nf := tov.NumField()
		for i := 0; i < nf; i++ {
			f := tov.Field(i)
			ft := f.Type

			if f.IsExported() == false {
				return fmt.Errorf("struct %s has unexported field(s)", tov.Name())
			}

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
			if state.child == nil {
				state.child = &stateEncode{options: state.options}
			}
			state = state.child
			for i := 0; i < nf; i++ {
				state.encodeType = false
				if err := encs[i].Encode(value.Field(i), b, state); err != nil {
					return err
				}
			}
			return nil
		}
		encoders.Store(tov, regEncoder(name, fenc))

		// decoder closure
		fdec := func(value *reflect.Value, packet []byte, state *stateDecode) (*reflect.Value, []byte, error) {
			var err error
			if state.child == nil {
				state.child = &stateDecode{options: state.options}
			}

			if value == nil {
				v := reflect.Indirect(reflect.New(state.decoder.Type))
				value = &v
			}

			state = state.child
			for i := 0; i < nf; i++ {
				field := value.Field(i)
				_, packet, err = decs[i].Decode(&field, packet, state)
				if err != nil {
					return nil, nil, err
				}
			}
			return value, packet, nil
		}
		decoders.Store(name, &decoder{tov, fdec})
		addRegCache(tov)

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
		encoders.Store(tov, regEncoder(name, fenc))

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

			x := reflect.MakeSlice(tov, n, n)
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
					decoder: dec,
				}
			}
			state = state.child

			for i := 0; i < n; i++ {
				item := value.Index(i)
				_, p, err := dec.Decode(&item, packet, state)
				if err != nil {
					return nil, nil, err
				}
				packet = p
			}

			return value, packet, nil
		}
		decoders.Store(name, &decoder{tov, fdec})
		addRegCache(tov)

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
		encoders.Store(tov, regEncoder(name, fenc))

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
		decoders.Store(name, &decoder{tov, fdec})
		addRegCache(tov)

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
		encoders.Store(tov, regEncoder(name, fenc))

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

			x := reflect.MakeMapWithSize(tov, n)
			if value == nil {
				value = &x
			} else {
				value.Set(x)
			}

			if n == 0 {
				return value, packet, nil
			}

			if n > len(packet) {
				return nil, nil, fmt.Errorf("incorrect data length")
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
		decoders.Store(name, &decoder{tov, fdec})
		addRegCache(tov)

	default:
		return fmt.Errorf("type %v is not supported", tov)
	}

	return nil
}

func RegisterError(e error) error {
	return addErrCache(e)
}

func RegisterAtom(a gen.Atom) error {
	return addAtomCache(a)
}

var (
	encoders sync.Map
	decoders sync.Map
)

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
	id := atomic.AddUint32(&errCacheID, 1)
	// math.MaxUint16 is used for encoding a nil value
	if id > math.MaxUint16-1 {
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
