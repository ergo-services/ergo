package etf

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/big"
	"reflect"

	"github.com/ergo-services/ergo/lib"
)

var (
	ErrStringTooLong = fmt.Errorf("Encoding error. String too long. Max allowed length is 65535")
	ErrAtomTooLong   = fmt.Errorf("Encoding error. Atom too long. Max allowed UTF-8 chars is 255")

	goSlice  = byte(240) // internal type
	goMap    = byte(241) // internal type
	goStruct = byte(242) // internal type

	marshalerType = reflect.TypeOf((*Marshaler)(nil)).Elem()
)

// EncodeOptions
type EncodeOptions struct {
	LinkAtomCache     *AtomCache
	WriterAtomCache   map[Atom]CacheItem
	EncodingAtomCache *ListAtomCache

	// FlagBigPidRef The node accepts a larger amount of data in pids
	// and references (node container types version 4).
	// In the pid case full 32-bit ID and Serial fields in NEW_PID_EXT
	// and in the reference case up to 5 32-bit ID words are now
	// accepted in NEWER_REFERENCE_EXT. Introduced in OTP 24.
	FlagBigPidRef bool

	// FlagBigCreation The node understands big node creation tags NEW_PID_EXT,
	// NEWER_REFERENCE_EXT.
	FlagBigCreation bool
}

// Encode
func Encode(term Term, b *lib.Buffer, options EncodeOptions) (retErr error) {
	defer func() {
		// We should catch any panic happened during encoding Golang types.
		if r := recover(); r != nil {
			retErr = fmt.Errorf("%v", r)
		}
	}()

	var stack, child *stackElement

	cacheEnabled := options.LinkAtomCache != nil

	cacheIndex := uint16(0)
	if options.EncodingAtomCache != nil {
		cacheIndex = uint16(len(options.EncodingAtomCache.L))
	}

	// Atom cache: (if its enabled: options.LinkAtomCache != nil)
	// 1. check for an atom in options.WriterAtomCache (map)
	// 2. if not found in WriterAtomCache call LinkAtomCache.Append(atom),
	//    encode it as a regular atom (ettAtom*)
	// 3. if found
	//    add options.EncodingAtomCache[i] = CacheItem, where i is just a counter
	//    within this encoding process.

	//    encode atom as ettCacheRef with value = i
	for {

		child = nil

		if stack != nil {

			if stack.i == stack.children {
				if stack.parent == nil {
					return nil
				}
				stack, stack.parent = stack.parent, nil
				continue
			}

			switch stack.termType {
			case ettList:
				if stack.i == stack.children-1 {
					// last item of list should be ettNil
					term = nil
					break
				}
				term = stack.term.(List)[stack.i]
			case ettListImproper:
				// improper list like [a|b] has no ettNil as a last item
				term = stack.term.(ListImproper)[stack.i]

			case ettSmallTuple:
				term = stack.term.(Tuple)[stack.i]

			case ettPid:
				p := stack.term.(Pid)
				if stack.i == 0 {
					term = p.Node
					break
				}

				buf := b.Extend(9)

				// ID a 32-bit big endian unsigned integer.
				// If FlagBigPidRef is not set, only 15 bits may be used
				// and the rest must be 0.
				if options.FlagBigPidRef {
					binary.BigEndian.PutUint32(buf[:4], uint32(p.ID))
				} else {
					// 15 bits only 2**15 - 1 = 32767
					binary.BigEndian.PutUint32(buf[:4], uint32(p.ID)&32767)
				}

				// Serial a 32-bit big endian unsigned integer.
				// If distribution FlagBigPidRef is not set, only 13 bits may be used
				// and the rest must be 0.
				if options.FlagBigPidRef {
					binary.BigEndian.PutUint32(buf[4:8], uint32(p.ID>>32))
				} else {
					// 13 bits only 2**13 - 1 = 8191
					binary.BigEndian.PutUint32(buf[4:8], (uint32(p.ID>>15) & 8191))
				}

				// Same as NEW_PID_EXT except the Creation field is
				// only one byte and only two bits are significant,
				// the rest are to be 0.
				buf[8] = byte(p.Creation) & 3

				stack.i++
				continue

			case ettNewPid:
				p := stack.term.(Pid)
				if stack.i == 0 {
					term = p.Node
					break
				}

				buf := b.Extend(12)
				// ID
				if options.FlagBigPidRef {
					binary.BigEndian.PutUint32(buf[:4], uint32(p.ID))
				} else {
					// 15 bits only 2**15 - 1 = 32767
					binary.BigEndian.PutUint32(buf[:4], uint32(p.ID)&32767)
				}
				// Serial
				if options.FlagBigPidRef {
					binary.BigEndian.PutUint32(buf[4:8], uint32(p.ID>>32))
				} else {
					// 13 bits only 2**13 - 1 = 8191
					binary.BigEndian.PutUint32(buf[4:8], (uint32(p.ID>>32))&8191)
				}
				// Creation
				binary.BigEndian.PutUint32(buf[8:12], p.Creation)

				stack.i++
				continue

			case ettNewRef:
				r := stack.term.(Ref)
				if stack.i == 0 {
					term = stack.term.(Ref).Node
					break
				}

				lenID := 3
				if options.FlagBigPidRef {
					lenID = 5
				}
				buf := b.Extend(1 + lenID*4)
				// Only one byte long and only two bits are significant, the rest must be 0.
				buf[0] = byte(r.Creation & 3)
				buf = buf[1:]
				for i := 0; i < lenID; i++ {
					// In the first word (4 bytes) of ID, only 18 bits
					// are significant, the rest must be 0.
					if i == 0 {
						// 2**18 - 1 = 262143
						binary.BigEndian.PutUint32(buf[:4], r.ID[i]&262143)
					} else {
						binary.BigEndian.PutUint32(buf[:4], r.ID[i])
					}
					buf = buf[4:]
				}

				stack.i++
				continue

			case ettNewerRef:
				r := stack.term.(Ref)
				if stack.i == 0 {
					term = stack.term.(Ref).Node
					break
				}

				lenID := 3
				if options.FlagBigPidRef {
					lenID = 5
				}
				buf := b.Extend(4 + lenID*4)
				binary.BigEndian.PutUint32(buf[0:4], r.Creation)
				buf = buf[4:]
				for i := 0; i < lenID; i++ {
					binary.BigEndian.PutUint32(buf[:4], r.ID[i])
					buf = buf[4:]
				}

				stack.i++
				continue

			case ettMap:
				key := stack.tmp.(List)[stack.i/2]
				if stack.i&0x01 == 0x01 { // a value
					term = stack.term.(Map)[key]
					break
				}
				term = key

			case goMap:
				key := stack.tmp.([]reflect.Value)[stack.i/2]
				if stack.i&0x01 == 0x01 { // a value
					term = stack.term.(func(reflect.Value) reflect.Value)(key).Interface()
					break
				}
				term = key.Interface() // a key

			case goSlice:
				if stack.i == stack.children-1 {
					// last item of list should be ettNil
					term = nil
					break
				}
				term = stack.term.(func(int) reflect.Value)(stack.i).Interface()

			case goStruct:
				field := stack.tmp.(func(int) reflect.StructField)(stack.i / 2)
				fieldName := field.Name

				if tag := field.Tag.Get("etf"); tag != "" {
					fieldName = tag
				}

				if stack.i&0x01 != 0x01 { // a key (field name)
					term = Atom(fieldName)
					break
				}

				// a value
				term = stack.term.(func(int) reflect.Value)(stack.i / 2).Interface()

			default:

				return errInternal
			}

			stack.i++
		}

	recasting:
		switch t := term.(type) {
		case bool:

			if cacheEnabled && cacheIndex < 256 {
				value := Atom("false")
				if t {
					value = Atom("true")
				}

				// looking for CacheItem
				ci, found := options.WriterAtomCache[value]
				if found {
					options.EncodingAtomCache.Append(ci)
					b.Append([]byte{ettCacheRef, byte(cacheIndex)})
					cacheIndex++
					break
				}
				// add it to the cache and encode as usual Atom
				options.LinkAtomCache.Append(value)

			}

			if t {
				b.Append([]byte{ettSmallAtom, 4, 't', 'r', 'u', 'e'})
				break
			}

			b.Append([]byte{ettSmallAtom, 5, 'f', 'a', 'l', 's', 'e'})

		// do not use reflect.ValueOf(t) because its too expensive
		case uint8:
			b.Append([]byte{ettSmallInteger, t})

		case int8:
			if t < 0 {
				term = int32(t)
				continue
			}

			b.Append([]byte{ettSmallInteger, uint8(t)})
			break

		case uint16:
			if t <= math.MaxUint8 {
				b.Append([]byte{ettSmallInteger, byte(t)})
				break
			}
			term = int32(t)
			goto recasting

		case int16:
			if t >= 0 && t <= math.MaxUint8 {
				b.Append([]byte{ettSmallInteger, byte(t)})
				break
			}

			term = int32(t)
			goto recasting

		case uint32:
			if t <= math.MaxUint8 {
				b.Append([]byte{ettSmallInteger, byte(t)})
				break
			}

			if t > math.MaxInt32 {
				term = int64(t)
				goto recasting
			}

			term = int32(t)
			goto recasting

		case int32:
			if t >= 0 && t <= math.MaxUint8 {
				b.Append([]byte{ettSmallInteger, byte(t)})
				break
			}

			// 1 (ettInteger) + 4 (32bit integer)
			buf := b.Extend(1 + 4)
			buf[0] = ettInteger
			binary.BigEndian.PutUint32(buf[1:5], uint32(t))

		case uint:
			if t <= math.MaxUint8 {
				b.Append([]byte{ettSmallInteger, byte(t)})
				break
			}

			if t > math.MaxInt32 {
				term = int64(t)
				goto recasting
			}

			term = int32(t)
			goto recasting

		case int:
			if t >= 0 && t <= math.MaxUint8 {
				b.Append([]byte{ettSmallInteger, byte(t)})
				break
			}

			if t > math.MaxInt32 || t < math.MinInt32 {
				term = int64(t)
				goto recasting
			}

			term = int32(t)
			goto recasting

		case uint64:
			if t <= math.MaxUint8 {
				b.Append([]byte{ettSmallInteger, byte(t)})
				break
			}

			if t <= math.MaxInt32 {
				term = int32(t)
				goto recasting
			}

			if t <= math.MaxInt64 {
				term = int64(t)
				goto recasting
			}

			buf := []byte{ettSmallBig, 8, 0, 0, 0, 0, 0, 0, 0, 0, 0}
			binary.LittleEndian.PutUint64(buf[3:], uint64(t))
			b.Append(buf)

		case int64:
			if t >= 0 && t <= math.MaxUint8 {
				b.Append([]byte{ettSmallInteger, byte(t)})
				break
			}

			if t >= math.MinInt32 && t <= math.MaxInt32 {
				term = int32(t)
				goto recasting
			}

			if t == math.MinInt64 {
				// corner case:
				// if t = -9223372036854775808 (which is math.MinInt64)
				// we can't just revert the sign because it overflows math.MaxInt64 value
				buf := []byte{ettSmallBig, 8, 1, 0, 0, 0, 0, 0, 0, 0, 128}
				b.Append(buf)
				break
			}

			negative := byte(0)
			if t < 0 {
				negative = 1
				t = -t
			}

			buf := []byte{ettSmallBig, 0, negative, 0, 0, 0, 0, 0, 0, 0, 0}
			binary.LittleEndian.PutUint64(buf[3:], uint64(t))

			switch {
			case t < 4294967296:
				buf[1] = 4
				b.Append(buf[:7])

			case t < 1099511627776:
				buf[1] = 5
				b.Append(buf[:8])

			case t < 281474976710656:
				buf[1] = 6
				b.Append(buf[:9])

			case t < 72057594037927936:
				buf[1] = 7
				b.Append(buf[:10])

			default:
				buf[1] = 8
				b.Append(buf)
			}

		case big.Int:
			bytes := t.Bytes()
			negative := t.Sign() < 0
			l := len(bytes)

			for i := 0; i < l/2; i++ {
				bytes[i], bytes[l-1-i] = bytes[l-1-i], bytes[i]
			}

			if l < 256 {
				// 1 (ettSmallBig) + 1 (len) + 1 (sign) + bytes
				buf := b.Extend(1 + 1 + 1 + l)
				buf[0] = ettSmallBig
				buf[1] = byte(l)

				if negative {
					buf[2] = 1
				} else {
					buf[2] = 0
				}

				copy(buf[3:], bytes)

				break
			}

			// 1 (ettLargeBig) + 4 (len) + 1(sign) + bytes
			buf := b.Extend(1 + 4 + 1 + l)
			buf[0] = ettLargeBig
			binary.BigEndian.PutUint32(buf[1:5], uint32(l))

			if negative {
				buf[5] = 1
			} else {
				buf[5] = 0
			}

			copy(buf[6:], bytes)

		case string:
			lenString := len(t)

			if lenString > 65535 {
				return ErrStringTooLong
			}

			// 1 (ettString) + 2 (len) + string
			buf := b.Extend(1 + 2 + lenString)
			buf[0] = ettString
			binary.BigEndian.PutUint16(buf[1:3], uint16(lenString))
			copy(buf[3:], t)

		case Charlist:
			term = []rune(t)
			goto recasting

		case String:
			term = []byte(t)
			goto recasting

		case Atom:
			// As from ERTS 9.0 (OTP 20), atoms may contain any Unicode
			// characters and are always encoded using the UTF-8 external
			// formats ATOM_UTF8_EXT or SMALL_ATOM_UTF8_EXT.

			if cacheEnabled && cacheIndex < 256 {
				// looking for CacheItem
				ci, found := options.WriterAtomCache[t]
				if found {
					options.EncodingAtomCache.Append(ci)
					b.Append([]byte{ettCacheRef, byte(cacheIndex)})
					cacheIndex++
					break
				}

				options.LinkAtomCache.Append(t)
			}

			// https://erlang.org/doc/apps/erts/erl_ext_dist.html#utf8_atoms
			// The maximum number of allowed characters in an atom is 255.
			// In the UTF-8 case, each character can need 4 bytes to be encoded.
			if len([]rune(t)) > 255 {
				return ErrAtomTooLong
			}

			lenAtom := len(t)
			if lenAtom < 256 {
				buf := b.Extend(1 + 1 + lenAtom)
				buf[0] = ettSmallAtomUTF8
				buf[1] = byte(lenAtom)
				copy(buf[2:], t)
				break
			}

			// 1 (ettAtomUTF8) + 2 (len) + atom
			buf := b.Extend(1 + 2 + lenAtom)
			buf[0] = ettAtomUTF8
			binary.BigEndian.PutUint16(buf[1:3], uint16(lenAtom))
			copy(b.B[3:], t)

		case float32:
			term = float64(t)
			goto recasting

		case float64:
			// 1 (ettNewFloat) + 8 (float)
			buf := b.Extend(1 + 8)
			buf[0] = ettNewFloat
			bits := math.Float64bits(t)
			binary.BigEndian.PutUint64(buf[1:9], uint64(bits))

		case nil:
			b.AppendByte(ettNil)

		case Tuple:
			lenTuple := len(t)
			if lenTuple < 256 {
				b.Append([]byte{ettSmallTuple, byte(lenTuple)})
			} else {
				buf := b.Extend(5)
				buf[0] = ettLargeTuple
				binary.BigEndian.PutUint32(buf[1:5], uint32(lenTuple))
			}
			child = &stackElement{
				parent:   stack,
				termType: ettSmallTuple, // doesn't matter what exact type for the further processing
				term:     t,
				children: lenTuple,
			}

		case Pid:
			child = &stackElement{
				parent:   stack,
				term:     t,
				children: 2,
			}
			if options.FlagBigCreation {
				child.termType = ettNewPid
				b.AppendByte(ettNewPid)
			} else {
				child.termType = ettPid
				b.AppendByte(ettPid)
			}

		case Alias:
			term = Ref(t)
			goto recasting

		case Ref:
			buf := b.Extend(3)

			child = &stackElement{
				parent:   stack,
				term:     t,
				children: 2,
			}
			if options.FlagBigCreation {
				buf[0] = ettNewerRef
				child.termType = ettNewerRef

			} else {
				buf[0] = ettNewRef
				child.termType = ettNewRef
			}

			// LEN a 16-bit big endian unsigned integer not larger
			// than 5 when the FlagBigPidRef has been set; otherwise not larger than 3.
			if options.FlagBigPidRef {
				binary.BigEndian.PutUint16(buf[1:3], 5)
			} else {
				binary.BigEndian.PutUint16(buf[1:3], 3)
			}

		case Map:
			lenMap := len(t)
			buf := b.Extend(5)
			buf[0] = ettMap
			binary.BigEndian.PutUint32(buf[1:], uint32(lenMap))

			keys := make(List, 0, lenMap)
			for key := range t {
				keys = append(keys, key)
			}

			child = &stackElement{
				parent:   stack,
				termType: ettMap,
				term:     t,
				children: lenMap * 2,
				tmp:      keys,
			}

		case ListImproper:
			if len(t) == 0 {
				b.AppendByte(ettNil)
				continue
			}
			lenList := len(t) - 1
			buf := b.Extend(5)
			buf[0] = ettList
			binary.BigEndian.PutUint32(buf[1:], uint32(lenList))
			child = &stackElement{
				parent:   stack,
				termType: ettListImproper,
				term:     t,
				children: lenList + 1,
			}

		case List:
			lenList := len(t)
			if lenList == 0 {
				b.AppendByte(ettNil)
				continue
			}
			buf := b.Extend(5)
			buf[0] = ettList
			binary.BigEndian.PutUint32(buf[1:], uint32(lenList))
			child = &stackElement{
				parent:   stack,
				termType: ettList,
				term:     t,
				children: lenList + 1,
			}

		case []byte:
			lenBinary := len(t)
			buf := b.Extend(1 + 4 + lenBinary)
			buf[0] = ettBinary
			binary.BigEndian.PutUint32(buf[1:5], uint32(lenBinary))
			copy(buf[5:], t)

		case Marshaler:
			m, err := t.MarshalETF()
			if err != nil {
				return err
			}

			lenBinary := len(m)
			buf := b.Extend(1 + 4 + lenBinary)
			buf[0] = ettBinary
			binary.BigEndian.PutUint32(buf[1:5], uint32(lenBinary))
			copy(buf[5:], m)

		default:
			v := reflect.ValueOf(t)

			switch v.Kind() {
			case reflect.Struct:
				lenStruct := v.NumField()
				buf := b.Extend(5)
				buf[0] = ettMap
				binary.BigEndian.PutUint32(buf[1:], uint32(lenStruct))

				child = &stackElement{
					parent:   stack,
					termType: goStruct,
					term:     v.Field,
					children: lenStruct * 2,
					tmp:      v.Type().Field,
				}

			case reflect.Array, reflect.Slice:
				lenList := v.Len()
				buf := b.Extend(5)
				buf[0] = ettList
				binary.BigEndian.PutUint32(buf[1:], uint32(lenList))
				child = &stackElement{
					parent:   stack,
					termType: goSlice,
					term:     v.Index,
					children: lenList + 1,
				}

			case reflect.Map:
				lenMap := v.Len()
				buf := b.Extend(5)
				buf[0] = ettMap
				binary.BigEndian.PutUint32(buf[1:], uint32(lenMap))

				child = &stackElement{
					parent:   stack,
					termType: goMap,
					term:     v.MapIndex,
					children: lenMap * 2,
					tmp:      v.MapKeys(),
				}

			case reflect.Ptr:
				// dereference value
				if !v.IsNil() {
					term = v.Elem().Interface()
					goto recasting
				}

				b.AppendByte(ettNil)
				if stack == nil {
					break
				}

			default:
				return fmt.Errorf("unsupported type %v with value %#v", v.Type(), v)
			}
		}

		if stack == nil && child == nil {
			return nil
		}

		if child != nil {
			stack = child
		}

	}
}
