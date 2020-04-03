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
)

func Encode(term Term, b *lib.Buffer,
	linkAtomCache *AtomCache,
	writerAtomCache map[Atom]CacheItem,
	encodingAtomCache *ListAtomCache) error {

	var stack *stackElement

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

		case uint8:
			b.Append([]byte{ettSmallInteger, t})

		case int8, int16, int32, uint16, uint32:
			// 1 (ettInteger) + 4 (32bit integer)
			buf := b.Extend(1 + 4)
			buf[0] = ettInteger
			binary.BigEndian.PutUint32(buf[1:5], t.(uint32))

		case int, int64:
			if t.(uint64) < math.MaxInt32 {
				term = t.(int32)
				continue
			}

			term = new(big.Int).SetInt64(t.(int64))
			continue

		case uint, uint64:
			if t.(uint64) < math.MaxInt32 {
				term = t.(int32)
				continue
			}

			term = new(big.Int).SetUint64(t.(uint64))
			continue

		case *big.Int:
			if t.BitLen() < 33 {
				term = int32(t.Int64())
				continue
			}
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

				break
			}

			// 1 (ettLargeBig) + 4 (len) + 1(sing) + bytes
			buf := b.Extend(1 + 4 + 1 + l)
			buf[0] = ettLargeBig
			binary.BigEndian.PutUint32(buf[1:5], uint32(l))
			if negative {
				buf[5] = 1
			} else {
				buf[5] = 0
			}

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
				b.Append([]byte{ettSmallAtomUTF8, byte(lenAtom)})
				b.Append([]byte(t))
				break
			}

			// 1 (ettAtomUTF8) + 2 (len) + atom
			buf := b.Extend(1 + 2 + lenAtom)
			buf[0] = ettAtomUTF8
			binary.BigEndian.PutUint16(buf[1:3], uint16(lenAtom))
			copy(b.B[3:], t)

		case float32, float64:
			// 1 (ettNewFloat) + 8 (float)
			buf := b.Extend(1 + 8)
			buf[0] = ettNewFloat
			bits := math.Float64bits(t.(float64))
			binary.BigEndian.PutUint64(buf[1:9], uint64(bits))

		case Tuple:

		case Pid:
		case Ref:
		default:
			v := reflect.ValueOf(t)
			switch v.Kind() {
			case reflect.Struct:
			case reflect.Array, reflect.Slice:
			case reflect.Map:
			case reflect.Ptr:
				term = v.Elem()
				continue
			default:
				// unknown

			}
		}

		if stack == nil {
			return nil
		}
	}

}
