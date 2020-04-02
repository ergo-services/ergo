package etf

import (
	"github.com/halturin/ergo/lib"
	"math/big"
	"reflect"
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
			b.AppendByte(byte(t))
		case int16, int32:

		case int, int64:

		case *big.Int:

		case string:

		case Atom:

		case float32, float64:

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
