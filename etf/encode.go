package etf

import (
	"encoding/binary"
	"fmt"
	"github.com/halturin/ergo/lib"
	"math"
	"math/big"
	"reflect"
)

var (
	ErrStringTooLong = fmt.Errorf("Encoding error. String too long")

	goSlice = byte(240) // internal type
)

func Encode(term Term, b *lib.Buffer,
	linkAtomCache *AtomCache,
	writerAtomCache map[Atom]CacheItem,
	encodingAtomCache *ListAtomCache) error {

	var stack, child *stackElement

	cacheEnabled := linkAtomCache != nil
	cacheIndex := uint16(0)

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

			case goSlice:
				if stack.i == stack.children-1 {
					// last item of list should be ettNil
					term = nil
					break
				}

				term = stack.term.(func(int) reflect.Value)(stack.i).Interface()

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
				// add it to the cache and encode as usual Atom
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

		case Ref:

		case Map:

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
				term = transformStructToMap(t)
				continue

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
				term = v.Interface().(Map)
				continue

			case reflect.Ptr:
				// dereference value
				term = v.Elem().Interface()
				continue

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

func transformStructToMap(interface{}) Map {
	//	a := &A{Foo: "afoo"}
	//  val := reflect.Indirect(reflect.ValueOf(a))
	//  fmt.Println(val.Type().Field(0).Name)
	return nil
}
