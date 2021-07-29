package etf

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/big"
	"reflect"

	"github.com/halturin/ergo/lib"
)

var (
	ErrStringTooLong = fmt.Errorf("Encoding error. String too long")

	goSlice  = byte(240) // internal type
	goMap    = byte(241) // internal type
	goStruct = byte(242) // internal type
)

func Encode(term Term, b *lib.Buffer,
	linkAtomCache *AtomCache,
	writerAtomCache map[Atom]CacheItem,
	encodingAtomCache *ListAtomCache) (retErr error) {
	defer func() {
		// We should catch any panic happend during encoding Golang types.
		if r := recover(); r != nil {
			retErr = fmt.Errorf("%v", r)
		}
	}()

	var stack, child *stackElement

	cacheEnabled := linkAtomCache != nil

	cacheIndex := uint16(0)
	if encodingAtomCache != nil {
		cacheIndex = uint16(len(encodingAtomCache.L))
	}

	// Atom cache: (if its enabled: linkAtomCache != nil)
	// 1. check for an atom in writerAtomCache (map)
	// 2. if not found in writerAtomCache call linkAtomCache.Append(atom),
	//    encode it as a regular atom (ettAtom*)
	// 3. if found
	//    add encodingAtomCache[i] = CacheItem, where i is just a counter
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

			case ettSmallTuple:
				term = stack.term.(Tuple)[stack.i]

			case ettPid:
				p := stack.term.(Pid)
				if stack.i == 0 {
					term = p.Node
					break
				}

				buf := b.Extend(9)
				binary.BigEndian.PutUint32(buf[:4], p.ID)
				binary.BigEndian.PutUint32(buf[4:8], p.Serial)
				buf[8] = p.Creation

				stack.i++
				continue

			case ettNewRef:
				r := stack.term.(Ref)
				if stack.i == 0 {
					term = stack.term.(Ref).Node
					break
				}

				lenID := len(r.ID)
				buf := b.Extend(1 + lenID*4)
				buf[0] = r.Creation
				buf = buf[1:]
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

			case goSlice:
				if stack.i == stack.children-1 {
					// last item of list should be ettNil
					term = nil
					break
				}
				term = stack.term.(func(int) reflect.Value)(stack.i).Interface()

			case goMap:
				key := stack.tmp.([]reflect.Value)[stack.i/2]
				if stack.i&0x01 == 0x01 { // a value
					term = stack.term.(func(reflect.Value) reflect.Value)(key).Interface()
					break
				}
				term = key.Interface()

			case goStruct:
				if stack.i&0x01 == 0x01 { // a value
					term = stack.term.(func(int) reflect.Value)(stack.i / 2).Interface()
					break
				}

				// a key (field name)
				term = Atom(stack.tmp.(func(int) reflect.StructField)(stack.i / 2).Name)

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
				ci, found := writerAtomCache[value]
				if found {
					encodingAtomCache.Append(ci)
					b.Append([]byte{ettCacheRef, byte(cacheIndex)})
					cacheIndex++
					break
				}
				// add it to the cache and encode as usual Atom
				linkAtomCache.Append(value)

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

		case Atom:
			if cacheEnabled && cacheIndex < 256 {
				// looking for CacheItem
				ci, found := writerAtomCache[t]
				if found {
					encodingAtomCache.Append(ci)
					b.Append([]byte{ettCacheRef, byte(cacheIndex)})
					cacheIndex++
					break
				}

				linkAtomCache.Append(t)
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
			b.AppendByte(ettPid)
			child = &stackElement{
				parent:   stack,
				termType: ettPid,
				term:     t,
				children: 2,
			}

		case Ref:
			buf := b.Extend(3)
			buf[0] = ettNewRef
			binary.BigEndian.PutUint16(buf[1:3], uint16(len(t.ID)))

			child = &stackElement{
				parent:   stack,
				termType: ettNewRef,
				term:     t,
				children: 2,
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

		case List:
			lenList := len(t)
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
				return fmt.Errorf("unsupported type %#v", v)
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
