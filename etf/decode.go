package etf

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/big"
	"reflect"

	"github.com/ergo-services/ergo/lib"
)

// linked list for decoding complex types like list/map/tuple
type stackElement struct {
	parent   *stackElement
	reg      *reflect.Value // used for registered types decoding
	term     Term           //value
	tmp      Term           // temporary value. used as a temporary storage for a key of map
	i        int            // current
	children int
	termType byte
	strict   bool // if encoding/decoding registered types must be strict
}

var (
	biggestInt = big.NewInt(0xfffffffffffffff)
	lowestInt  = big.NewInt(-0xfffffffffffffff)

	termNil = make(List, 0)

	errMalformedAtomUTF8      = fmt.Errorf("Malformed ETF. ettAtomUTF8")
	errMalformedSmallAtomUTF8 = fmt.Errorf("Malformed ETF. ettSmallAtomUTF8")
	errMalformedString        = fmt.Errorf("Malformed ETF. ettString")
	errMalformedCacheRef      = fmt.Errorf("Malformed ETF. ettCacheRef")
	errMalformedNewFloat      = fmt.Errorf("Malformed ETF. ettNewFloat")
	errMalformedFloat         = fmt.Errorf("Malformed ETF. ettFloat")
	errMalformedSmallInteger  = fmt.Errorf("Malformed ETF. ettSmallInteger")
	errMalformedInteger       = fmt.Errorf("Malformed ETF. ettInteger")
	errMalformedSmallBig      = fmt.Errorf("Malformed ETF. ettSmallBig")
	errMalformedLargeBig      = fmt.Errorf("Malformed ETF. ettLargeBig")
	errMalformedList          = fmt.Errorf("Malformed ETF. ettList")
	errMalformedSmallTuple    = fmt.Errorf("Malformed ETF. ettSmallTuple")
	errMalformedLargeTuple    = fmt.Errorf("Malformed ETF. ettLargeTuple")
	errMalformedMap           = fmt.Errorf("Malformed ETF. ettMap")
	errMalformedBinary        = fmt.Errorf("Malformed ETF. ettBinary")
	errMalformedBitBinary     = fmt.Errorf("Malformed ETF. ettBitBinary")
	errMalformedPid           = fmt.Errorf("Malformed ETF. ettPid")
	errMalformedNewPid        = fmt.Errorf("Malformed ETF. ettNewPid")
	errMalformedRef           = fmt.Errorf("Malformed ETF. ettNewRef")
	errMalformedNewRef        = fmt.Errorf("Malformed ETF. ettNewerRef")
	errMalformedPort          = fmt.Errorf("Malformed ETF. ettPort")
	errMalformedNewPort       = fmt.Errorf("Malformed ETF. ettNewPort")
	errMalformedFun           = fmt.Errorf("Malformed ETF. ettNewFun")
	errMalformedExport        = fmt.Errorf("Malformed ETF. ettExport")
	errMalformedUnknownType   = fmt.Errorf("Malformed ETF. unknown type")

	errMalformed = fmt.Errorf("Malformed ETF")
	errInternal  = fmt.Errorf("Internal error")
)

// DecodeOptions
type DecodeOptions struct {
	AtomMapping   AtomMapping
	FlagBigPidRef bool
}

// stackless implementation is speeding up decoding function up to x25 times

// it might looks hard to understand the logic, but
// there are only two stages
// 1) Stage1: decoding basic types (atoms, strings, numbers...)
// 2) Stage2: decoding list/tuples/maps and complex types like Port/Pid/Ref using linked list named 'stack'
//
// see comments within this function

// Decode
func Decode(packet []byte, cache []Atom, options DecodeOptions) (retTerm Term, retByte []byte, retErr error) {
	var term Term
	var stack *stackElement
	var child *stackElement
	var t byte
	if lib.CatchPanic() {
		defer func() {
			// We should catch any panic happened during decoding the raw data.
			// Some of the Erlang' types can not be supported in Golang.
			// As an example: Erlang map with tuple as a key cause a panic
			// in Golang runtime with message:
			// 'panic: runtime error: hash of unhashable type etf.Tuple'
			// The problem is in etf.Tuple type - it is interface type. At the same
			// time Golang does support hashable key in map (like struct as a key),
			// but it should be known implicitly. It means we can encode such kind
			// of data, but can not to decode it back.
			if r := recover(); r != nil {
				retTerm = nil
				retByte = nil
				retErr = fmt.Errorf("%v", r)
			}
		}()
	}

	for {
		child = nil
		if len(packet) == 0 {
			return nil, nil, errMalformed
		}

		t = packet[0]
		packet = packet[1:]

		// Stage 1: decoding base type. if have encountered List/Map/Tuple
		// or complex type like Pid/Ref/Port:
		//  save the state in stackElement and push it to the stack (basically,
		//  we just append the new item to the linked list)
		//

		switch t {
		case ettAtomUTF8, ettAtom:
			if len(packet) < 2 {
				return nil, nil, errMalformedAtomUTF8
			}

			n := binary.BigEndian.Uint16(packet)
			if len(packet) < int(n+2) {
				return nil, nil, errMalformedAtomUTF8
			}

			atom := Atom(packet[2 : n+2])
			if len([]rune(atom)) > 255 {
				return nil, nil, errMalformedAtomUTF8
			}

			// replace atom value if we have mapped value for it
			options.AtomMapping.MutexIn.RLock()
			if mapped, ok := options.AtomMapping.In[atom]; ok {
				atom = mapped
			}
			options.AtomMapping.MutexIn.RUnlock()

			term = atom
			packet = packet[n+2:]

		case ettSmallAtomUTF8, ettSmallAtom:
			if len(packet) == 0 {
				return nil, nil, errMalformedSmallAtomUTF8
			}

			n := int(packet[0])
			if len(packet) < n+1 {
				return nil, nil, errMalformedSmallAtomUTF8
			}

			switch Atom(packet[1 : n+1]) {
			case "true":
				term = true
			case "false":
				term = false
			default:
				atom := Atom(packet[1 : n+1])
				// replace atom value if we have mapped value for it
				options.AtomMapping.MutexIn.RLock()
				if mapped, ok := options.AtomMapping.In[atom]; ok {
					atom = mapped
				}
				options.AtomMapping.MutexIn.RUnlock()
				term = atom
			}
			packet = packet[n+1:]

		case ettString:
			if len(packet) < 2 {
				return nil, nil, errMalformedString
			}

			n := binary.BigEndian.Uint16(packet)
			if len(packet) < int(n+2) {
				return nil, nil, errMalformedString
			}

			term = string(packet[2 : n+2])
			packet = packet[n+2:]

		case ettCacheRef:
			if len(packet) == 0 {
				return nil, nil, errMalformedCacheRef
			}

			switch cache[int(packet[0])] {
			case "true":
				term = true
			case "false":
				term = false
			default:
				atom := cache[int(packet[0])]
				// replace atom value if we have mapped value for it
				options.AtomMapping.MutexIn.RLock()
				if mapped, ok := options.AtomMapping.In[atom]; ok {
					atom = mapped
				}
				options.AtomMapping.MutexIn.RUnlock()
				term = atom
			}
			packet = packet[1:]

		case ettNewFloat:
			if len(packet) < 8 {
				return nil, nil, errMalformedNewFloat
			}
			bits := binary.BigEndian.Uint64(packet[:8])

			term = math.Float64frombits(bits)
			packet = packet[8:]

		case ettSmallInteger:
			if len(packet) == 0 {
				return nil, nil, errMalformedSmallInteger
			}

			term = int(packet[0])
			packet = packet[1:]

		case ettInteger:
			if len(packet) < 4 {
				return nil, nil, errMalformedInteger
			}

			term = int64(int32(binary.BigEndian.Uint32(packet[:4])))
			packet = packet[4:]

		case ettSmallBig:
			if len(packet) == 0 {
				return nil, nil, errMalformedSmallBig
			}

			n := packet[0]
			negative := packet[1] == 1 // sign

			///// this block improves performance at least 4 times
			if n < 9 { // treat as an int64/uint64
				le8 := make([]byte, 8)
				copy(le8, packet[2:n+2])
				smallBig := binary.LittleEndian.Uint64(le8)
				if negative {
					smallBig = -smallBig
					term = int64(smallBig)
				} else {
					if smallBig > math.MaxInt64 {
						term = uint64(smallBig)
					} else {
						term = int64(smallBig)
					}
				}

				packet = packet[n+2:]
				break
			}
			/////

			if len(packet) < int(n+2) {
				return nil, nil, errMalformedSmallBig
			}
			bytes := packet[2 : n+2]

			// encoded as a little endian. convert it to the big endian order
			l := len(bytes)
			for i := 0; i < l/2; i++ {
				bytes[i], bytes[l-1-i] = bytes[l-1-i], bytes[i]
			}

			bigInt := &big.Int{}
			bigInt.SetBytes(bytes)
			if negative {
				bigInt = bigInt.Neg(bigInt)
			}

			// try int and int64
			if bigInt.Cmp(biggestInt) < 0 && bigInt.Cmp(lowestInt) > 0 {
				term = bigInt.Int64()
				packet = packet[n+2:]
				break
			}

			term = bigInt
			packet = packet[n+2:]

		case ettLargeBig:
			if len(packet) < 256 { // must be longer than ettSmallBig
				return nil, nil, errMalformedLargeBig
			}

			n := binary.BigEndian.Uint32(packet[:4])
			negative := packet[4] == 1 // sign

			if len(packet) < int(n+5) {
				return nil, nil, errMalformedLargeBig
			}
			bytes := packet[5 : n+5]

			// encoded as a little endian. convert it to the big endian order
			l := len(bytes)
			for i := 0; i < l/2; i++ {
				bytes[i], bytes[l-1-i] = bytes[l-1-i], bytes[i]
			}

			bigInt := &big.Int{}
			bigInt.SetBytes(bytes)
			if negative {
				bigInt = bigInt.Neg(bigInt)
			}

			term = bigInt
			packet = packet[n+5:]

		case ettList:
			if len(packet) < 4 {
				return nil, nil, errMalformedList
			}

			n := binary.BigEndian.Uint32(packet[:4])
			if n == 0 {
				// must be encoded as ettNil
				return nil, nil, errMalformedList
			}

			term = make(List, n+1)
			packet = packet[4:]
			child = &stackElement{
				parent:   stack,
				termType: ettList,
				term:     term,
				children: int(n + 1),
			}

		case ettSmallTuple:
			if len(packet) == 0 {
				return nil, nil, errMalformedSmallTuple
			}

			n := packet[0]
			packet = packet[1:]
			term = make(Tuple, n)

			if n == 0 {
				break
			}

			child = &stackElement{
				parent:   stack,
				termType: ettSmallTuple,
				term:     term,
				children: int(n),
			}

		case ettLargeTuple:
			if len(packet) < 4 {
				return nil, nil, errMalformedLargeTuple
			}

			n := binary.BigEndian.Uint32(packet[:4])
			packet = packet[4:]
			term = make(Tuple, n)

			if n == 0 {
				break
			}

			child = &stackElement{
				parent:   stack,
				termType: ettLargeTuple,
				term:     term,
				children: int(n),
			}

		case ettMap:
			if len(packet) < 4 {
				return nil, nil, errMalformedMap
			}

			n := binary.BigEndian.Uint32(packet[:4])
			packet = packet[4:]
			term = make(Map)

			if n == 0 {
				break
			}

			child = &stackElement{
				parent:   stack,
				termType: ettMap,
				term:     term,
				children: int(n) * 2,
			}

		case ettBinary:
			if len(packet) < 4 {
				return nil, nil, errMalformedBinary
			}

			n := binary.BigEndian.Uint32(packet)
			if len(packet) < int(n+4) {
				return nil, nil, errMalformedBinary
			}

			b := make([]byte, n)
			copy(b, packet[4:n+4])

			term = b
			packet = packet[n+4:]

		case ettNil:
			// for registered types we should use a nil value
			// otherwise - treat it as an empty list
			if stack.reg != nil {
				term = nil
			} else {
				term = termNil
			}

		case ettPid, ettNewPid:
			child = &stackElement{
				parent:   stack,
				termType: t,
				children: 1,
			}

		case ettNewRef, ettNewerRef:
			if len(packet) < 2 {
				return nil, nil, errMalformedRef
			}

			l := binary.BigEndian.Uint16(packet[:2])
			packet = packet[2:]

			child = &stackElement{
				parent:   stack,
				termType: t,
				children: 1,
				tmp:      l, // save length in temporary place of the stack element
			}

		case ettExport:
			child = &stackElement{
				parent:   stack,
				termType: t,
				term:     Export{},
				children: 3,
			}

		case ettNewFun:
			var unique [16]byte

			if len(packet) < 32 {
				return nil, nil, errMalformedFun
			}

			copy(unique[:], packet[5:21])
			l := binary.BigEndian.Uint32(packet[25:29])

			fun := Function{
				Arity:    packet[4],
				Unique:   unique,
				Index:    binary.BigEndian.Uint32(packet[21:25]),
				FreeVars: make([]Term, l),
			}

			child = &stackElement{
				parent:   stack,
				termType: t,
				term:     fun,
				children: 4 + int(l),
			}
			packet = packet[29:]

		case ettPort, ettNewPort:
			child = &stackElement{
				parent:   stack,
				termType: t,
				children: 1,
			}

		case ettBitBinary:
			if len(packet) < 6 {
				return nil, nil, errMalformedBitBinary
			}

			n := binary.BigEndian.Uint32(packet)
			bits := uint(packet[4])

			b := make([]byte, n)
			copy(b, packet[5:n+5])
			b[n-1] = b[n-1] >> (8 - bits)

			term = b
			packet = packet[n+5:]

		case ettFloat:
			if len(packet) < 31 {
				return nil, nil, errMalformedFloat
			}

			var f float64
			if r, err := fmt.Sscanf(string(packet[:31]), "%f", &f); err != nil || r != 1 {
				return nil, nil, errMalformedFloat
			}
			term = f
			packet = packet[31:]

		default:
			term = nil
			return nil, nil, errMalformedUnknownType
		}

		// it was a single element
		if stack == nil && child == nil {
			break
		}

		// decoding child item of List/Map/Tuple/Pid/Ref/Port/... going deeper
		if child != nil {
			stack = child
			continue
		}

		// Stage 2
	processStack:
		if stack != nil {
			var field reflect.Value
			var set_field bool

			switch stack.termType {
			case ettList:
				if stack.i == 0 {
					// if the first value is atom, check for the registered type
					if typeName, isAtom := term.(Atom); isAtom == true {
						registered.RLock()
						r, found := registered.typesDec[typeName]
						registered.RUnlock()
						if found == true {
							reg := reflect.Indirect(reflect.New(r.rtype))
							if r.rtype.Kind() == reflect.Slice {
								reg = reflect.MakeSlice(r.rtype, stack.children-2, stack.children-1)
							}
							stack.reg = &reg
							stack.strict = r.strict
							if r.strict == false {
								stack.term.(List)[stack.i] = term
							}
							stack.i++
							break
						}
					}
				}

				if stack.reg != nil {
					if stack.i+1 == stack.children {
						if t != ettNil {
							x := reflect.Append(*stack.reg, reflect.ValueOf(term))
							stack.reg = &x
						}
					} else {
						set_field = true
						field = stack.reg.Index(stack.i - 1)
					}

					if stack.strict == true {
						stack.i++
						break
					}
				}

				stack.term.(List)[stack.i] = term
				stack.i++
				// remove the last element for proper list (its ettNil)
				if stack.i == stack.children && t == ettNil {
					stack.term = stack.term.(List)[:stack.i-1]
				}

			case ettSmallTuple, ettLargeTuple:

				if stack.i == 0 {
					// if the first value is atom, check for the registered type
					if typeName, isAtom := term.(Atom); isAtom == true {
						registered.RLock()
						r, found := registered.typesDec[typeName]
						registered.RUnlock()
						if found == true {
							reg := reflect.Indirect(reflect.New(r.rtype))
							stack.reg = &reg
							stack.strict = r.strict
							if r.strict == false {
								stack.term.(Tuple)[stack.i] = term
							}
							stack.i++
							break
						}
					}
				}

				if stack.reg != nil {
					set_field = true
					field = stack.reg.Field(stack.i - 1)
					if stack.strict == true {
						stack.i++
						break
					}
				}
				stack.term.(Tuple)[stack.i] = term
				stack.i++

			case ettMap:
				if stack.i == 0 {
					// if the first key is atom, check for the registered type
					if typeName, isAtom := term.(Atom); isAtom == true {
						registered.RLock()
						r, found := registered.typesDec[typeName]
						registered.RUnlock()
						if found == true {
							reg := reflect.MakeMapWithSize(r.rtype, stack.children/2)
							stack.reg = &reg
							stack.strict = r.strict
							if r.strict == false {
								if stack.i&0x01 == 0x01 { // a value
									stack.term.(Map)[stack.tmp] = term
								} else {
									stack.tmp = term
								}
							}
							stack.i++
							break
						}
					}
				}
				if stack.i == 1 && stack.reg != nil && stack.strict == true {
					// skip it. the value of the key which is the registered type
					stack.i++
					break

				}
				if stack.i&0x01 == 0x01 { // a value
					if stack.i > 1 && stack.reg != nil {
						set_field = true
						field = *stack.reg
					}
					stack.term.(Map)[stack.tmp] = term
					stack.i++
					break
				}

				// a key
				stack.tmp = term
				stack.i++

			case ettPid:
				if len(packet) < 9 {
					return nil, nil, errMalformedPid
				}

				name, ok := term.(Atom)
				if !ok {
					return nil, nil, errMalformedPid
				}
				pid := Pid{
					Node: name,
					// Same as NEW_PID_EXT except the Creation field is
					// only one byte and only two bits are significant,
					// the rest are to be 0.
					Creation: uint32(packet[8]) & 3,
				}

				id := uint64(binary.BigEndian.Uint32(packet[:4]))
				serial := uint64(binary.BigEndian.Uint32(packet[4:8]))
				if options.FlagBigPidRef {
					id = id | (serial << 32)
				} else {
					// id 15 bits only 2**15 - 1 = 32767
					// serial 13 bits only 2**13 - 1 = 8191
					id = (id & 32767) | ((serial & 8191) << 15)
				}
				pid.ID = id

				packet = packet[9:]
				stack.term = pid
				stack.i++

			case ettNewPid:
				if len(packet) < 12 {
					return nil, nil, errMalformedNewPid
				}

				name, ok := term.(Atom)
				if !ok {
					return nil, nil, errMalformedPid
				}

				id := uint64(binary.BigEndian.Uint32(packet[:4]))
				serial := uint64(binary.BigEndian.Uint32(packet[4:8]))
				pid := Pid{
					Node:     name,
					Creation: binary.BigEndian.Uint32(packet[8:12]),
				}
				if options.FlagBigPidRef {
					id = id | (serial << 32)
				} else {
					// id 15 bits only 2**15 - 1 = 32767
					// serial 13 bits only 2**13 - 1 = 8191
					id = (id & 32767) | ((serial & 8191) << 15)
				}
				pid.ID = id

				packet = packet[12:]
				stack.term = pid
				stack.i++

			case ettNewRef:
				var id uint32
				name, ok := term.(Atom)
				if !ok {
					return nil, nil, errMalformedRef
				}

				l := stack.tmp.(uint16)
				if l > 5 {
					return nil, nil, errMalformedRef
				}
				if l > 3 && !options.FlagBigPidRef {
					return nil, nil, errMalformedRef
				}
				stack.tmp = nil
				expectedLength := int(1 + l*4)

				if len(packet) < expectedLength {
					return nil, nil, errMalformedRef
				}

				ref := Ref{
					Node:     name,
					Creation: uint32(packet[0]),
				}
				packet = packet[1:]

				for i := 0; i < int(l); i++ {
					// In the first word (4 bytes) of ID, only 18 bits
					// are significant, the rest must be 0.
					if i == 0 {
						// 2**18 - 1 = 262143
						id = binary.BigEndian.Uint32(packet[:4]) & 262143
					} else {
						id = binary.BigEndian.Uint32(packet[:4])
					}
					ref.ID[i] = id
					packet = packet[4:]
				}

				stack.term = ref
				stack.i++

			case ettNewerRef:
				var id uint32
				name, ok := term.(Atom)
				if !ok {
					return nil, nil, errMalformedRef
				}

				l := stack.tmp.(uint16)
				if l > 5 {
					return nil, nil, errMalformedRef
				}
				if l > 3 && !options.FlagBigPidRef {
					return nil, nil, errMalformedRef
				}
				stack.tmp = nil
				expectedLength := int(4 + l*4)

				if len(packet) < expectedLength {
					return nil, nil, errMalformedRef
				}

				ref := Ref{
					Node:     name,
					Creation: binary.BigEndian.Uint32(packet[:4]),
				}
				packet = packet[4:]

				for i := 0; i < int(l); i++ {
					// In the first word (4 bytes) of ID, only 18 bits
					// are significant, the rest must be 0.
					if i == 0 {
						// 2**18 - 1 = 262143
						id = binary.BigEndian.Uint32(packet[:4]) & 262143
					} else {
						id = binary.BigEndian.Uint32(packet[:4])
					}
					ref.ID[i] = id
					packet = packet[4:]
				}

				stack.term = ref
				stack.i++

			case ettPort:
				if len(packet) < 5 {
					return nil, nil, errMalformedPort
				}

				name, ok := term.(Atom)
				if !ok {
					return nil, nil, errMalformedPort
				}

				port := Port{
					Node:     name,
					ID:       binary.BigEndian.Uint32(packet[:4]),
					Creation: uint32(packet[4]),
				}

				packet = packet[5:]
				stack.term = port
				stack.i++

			case ettNewPort:
				if len(packet) < 8 {
					return nil, nil, errMalformedNewPort
				}

				name, ok := term.(Atom)
				if !ok {
					return nil, nil, errMalformedNewPort
				}

				port := Port{
					Node:     name,
					ID:       binary.BigEndian.Uint32(packet[:4]),
					Creation: binary.BigEndian.Uint32(packet[4:8]),
				}

				packet = packet[8:]
				stack.term = port
				stack.i++

			case ettNewFun:
				fun := stack.term.(Function)
				switch stack.i {
				case 0:
					// Module
					module, ok := term.(Atom)
					if !ok {
						return nil, nil, errMalformedFun
					}
					fun.Module = module

				case 1:
					// OldIndex
					oldindex, ok := term.(int)
					if !ok {
						return nil, nil, errMalformedFun
					}
					fun.OldIndex = uint32(oldindex)

				case 2:
					// OldUnique
					olduniq, ok := term.(int64)
					if !ok {
						return nil, nil, errMalformedFun
					}
					fun.OldUnique = uint32(olduniq)

				case 3:
					// Pid
					pid, ok := term.(Pid)
					if !ok {
						return nil, nil, errMalformedFun
					}
					fun.Pid = pid

				default:
					if len(fun.FreeVars) < (stack.i-4)+1 {
						return nil, nil, errMalformedFun
					}
					fun.FreeVars[stack.i-4] = term
				}

				stack.term = fun
				stack.i++

			case ettExport:
				exp := stack.term.(Export)
				switch stack.i {
				case 0:
					module, ok := term.(Atom)
					if !ok {
						return nil, nil, errMalformedExport
					}
					exp.Module = module

				case 1:
					function, ok := term.(Atom)
					if !ok {
						return nil, nil, errMalformedExport
					}
					exp.Function = function

				case 2:
					arity, ok := term.(int)
					if !ok {
						return nil, nil, errMalformedExport
					}
					exp.Arity = arity

				default:
					return nil, nil, errMalformedExport

				}

			default:
				return nil, nil, errInternal
			}

			if field.Kind() == reflect.Ptr {
				pfield := reflect.New(field.Type().Elem())
				field.Set(pfield)
				field = pfield.Elem()
			}

			if set_field && term != nil {
				switch field.Kind() {
				case reflect.Int8:
					switch v := term.(type) {
					case int:
						if v > math.MaxInt8 || v < math.MinInt8 {
							// overflows
							if stack.strict {
								panic("overflows int8")
							}
							stack.reg = nil
							break
						}
						field.SetInt(int64(v))
					case int64:
						if v > math.MaxInt8 || v < math.MinInt8 {
							// overflows
							if stack.strict {
								panic("overflows int8")
							}
							stack.reg = nil
							break
						}
						field.SetInt(v)
					case uint64:
						if v > math.MaxInt8 {
							// overflows
							if stack.strict {
								panic("overflows int8")
							}
							stack.reg = nil
							break
						}
						field.SetInt(int64(v))
					default:
						if stack.strict {
							panic("wrong int8 value")
						}
						stack.reg = nil
					}

				case reflect.Int16:
					switch v := term.(type) {
					case int:
						if v > math.MaxInt16 || v < math.MinInt16 {
							// overflows
							if stack.strict {
								panic("overflows int16")
							}
							stack.reg = nil
							break
						}
						field.SetInt(int64(v))
					case int64:
						if v > math.MaxInt16 || v < math.MinInt16 {
							// overflows
							if stack.strict {
								panic("overflows int16")
							}
							stack.reg = nil
							break
						}
						field.SetInt(v)
					case uint64:
						if v > math.MaxInt16 {
							// overflows
							if stack.strict {
								panic("overflows int16")
							}
							stack.reg = nil
							break
						}
						field.SetInt(int64(v))
					default:
						if stack.strict {
							panic("wrong int16 value")
						}
						stack.reg = nil
					}

				case reflect.Int32:
					switch v := term.(type) {
					case int:
						if v > math.MaxInt32 || v < math.MinInt32 {
							// overflows
							if stack.strict {
								panic("overflows int32")
							}
							stack.reg = nil
							break
						}
						field.SetInt(int64(v))
					case int64:
						if v > math.MaxInt32 || v < math.MinInt32 {
							// overflows
							if stack.strict {
								panic("overflows int32")
							}
							stack.reg = nil
							break
						}
						field.SetInt(v)
					case uint64:
						if v > math.MaxInt32 {
							// overflows
							if stack.strict {
								panic("overflows int32")
							}
							stack.reg = nil
							break
						}
						field.SetInt(int64(v))
					default:
						if stack.strict {
							panic("wrong int32 value")
						}
						stack.reg = nil
					}
				case reflect.Int64:
					switch v := term.(type) {
					case int:
						field.SetInt(int64(v))
					case int64:
						field.SetInt(v)
					case uint64:
						if v > math.MaxInt64 {
							// overflows
							if stack.strict {
								panic("overflows int64")
							}
							stack.reg = nil
							break
						}
						field.SetInt(int64(v))
					default:
						if stack.strict {
							panic("wrong int64 value")
						}
						stack.reg = nil
					}
				case reflect.Int:
					switch v := term.(type) {
					case int:
						field.SetInt(int64(v))
					case int64:
						if v > math.MaxInt {
							// overflows
							if stack.strict {
								panic("overflows int")
							}
							stack.reg = nil
							break
						}
						field.SetInt(v)
					case uint64:
						if v > math.MaxInt {
							// overflows
							if stack.strict {
								panic("overflows int")
								return nil, nil, errMalformed
							}
							stack.reg = nil
							break
						}
						field.SetInt(int64(v))
					default:
						if stack.strict {
							panic("wrong int value")
						}
						stack.reg = nil
					}

				case reflect.Uint8:
					switch v := term.(type) {
					case int:
						if int64(v) > math.MaxUint8 || v < 0 {
							// overflows
							if stack.strict {
								panic("overflows uint8")
							}
							stack.reg = nil
							break
						}
						field.SetUint(uint64(v))
					case int64:
						if v > math.MaxUint8 || v < 0 {
							// overflows
							if stack.strict {
								panic("overflows uint8")
							}
							stack.reg = nil
							break
						}
						field.SetUint(uint64(v))
					case uint64:
						if v > math.MaxUint8 {
							// overflows
							if stack.strict {
								panic("overflows uint8")
							}
							stack.reg = nil
							break
						}
						field.SetUint(v)
					default:
						if stack.strict {
							panic("wrong uint8 value")
						}
						stack.reg = nil
					}

				case reflect.Uint16:
					switch v := term.(type) {
					case int:
						if int64(v) > math.MaxUint16 || v < 0 {
							// overflows
							if stack.strict {
								panic("overflows uint16")
							}
							stack.reg = nil
							break
						}
						field.SetUint(uint64(v))
					case int64:
						if v > math.MaxUint16 || v < 0 {
							// overflows
							if stack.strict {
								panic("overflows uint16")
							}
							stack.reg = nil
							break
						}
						field.SetUint(uint64(v))
					case uint64:
						if v > math.MaxUint16 {
							// overflows
							if stack.strict {
								panic("overflows uint16")
							}
							stack.reg = nil
							break
						}
						field.SetUint(v)
					default:
						if stack.strict {
							panic("wrong uint16 value")
						}
						stack.reg = nil
					}
				case reflect.Uint32:
					switch v := term.(type) {
					case int:
						if int64(v) > math.MaxUint32 || v < 0 {
							// overflows
							if stack.strict {
								panic("overflows uint32")
							}
							stack.reg = nil
							break
						}
						field.SetUint(uint64(v))
					case int64:
						if v > math.MaxUint32 || v < 0 {
							// overflows
							if stack.strict {
								panic("overflows uint32")
							}
							stack.reg = nil
							break
						}
						field.SetUint(uint64(v))
					case uint64:
						if v > math.MaxUint32 {
							// overflows
							if stack.strict {
								panic("overflows uint32")
							}
							stack.reg = nil
							break
						}
						field.SetUint(v)
					default:
						if stack.strict {
							panic("wrong uint32 value")
						}
						stack.reg = nil
					}

				case reflect.Uint64:
					switch v := term.(type) {
					case int:
						if v < 0 {
							// overflows
							if stack.strict {
								panic("overflows uint64")
							}
							stack.reg = nil
							break
						}
						field.SetUint(uint64(v))
					case int64:
						if v < 0 {
							// overflows
							if stack.strict {
								panic("overflows uint64")
							}
							stack.reg = nil
							break
						}
						field.SetUint(uint64(v))
					case uint64:
						field.SetUint(v)
					default:
						if stack.strict {
							panic("wrong uint64 value")
						}
						stack.reg = nil
					}

				case reflect.Uint:
					switch v := term.(type) {
					case int:
						if v < 0 {
							// overflows
							if stack.strict {
								panic("overflows uint")
							}
							stack.reg = nil
							break
						}
						field.SetUint(uint64(v))
					case int64:
						if v > math.MaxInt || v < 0 {
							// overflows
							if stack.strict {
								panic("overflows uint")
							}
							stack.reg = nil
							break
						}
						field.SetUint(uint64(v))
					case uint64:
						if v > math.MaxUint {
							// overflows
							if stack.strict {
								panic("overflows uint")
							}
							stack.reg = nil
							break
						}
						field.SetUint(v)
					default:
						if stack.strict {
							panic("wrong uint value")
						}
						stack.reg = nil
					}
				case reflect.Float32:
					f, ok := term.(float64)
					if ok == false {
						if stack.strict {
							panic("wrong float value")
						}
						stack.reg = nil
						break
					}
					field.SetFloat(f)

				case reflect.Float64:
					f64, ok := term.(float64)
					if ok == false {
						if stack.strict {
							panic("wrong float64")
						}
						stack.reg = nil
						break
					}
					field.SetFloat(f64)

				case reflect.String:
					switch v := term.(type) {
					case List:
						s, err := convertCharlistToString(v)
						if err != nil {
							if stack.strict {
								panic("can't convert charlist into string")
							}
							stack.reg = nil
							break
						}
						field.SetString(s)
					case []byte:
						field.SetString(string(v))
					case string:
						field.SetString(v)
					case Atom:
						field.SetString(string(v))
					default:
						if stack.strict {
							panic("wrong string value")
						}
						stack.reg = nil
					}
				case reflect.Bool:
					b, ok := term.(bool)
					if !ok {
						if stack.strict {
							panic("wrong bool value")
						}
						stack.reg = nil
						break
					}
					field.SetBool(b)

				case reflect.Map:
					if stack.tmp == nil {
						field.Set(reflect.ValueOf(term))
						break
					}
					destkey := reflect.ValueOf(stack.tmp)
					destval := reflect.ValueOf(term)
					stack.reg.SetMapIndex(destkey, destval)

				default:
					if stack.strict {
						if field.Type().Name() == "Alias" {
							term = Alias(term.(Ref))
						}
						field.Set(reflect.ValueOf(term))
					} else {
						// wrap it to catch the panic
						setValue := func(f reflect.Value, v interface{}) (ok bool) {
							if lib.CatchPanic() {
								defer func() {
									if r := recover(); r != nil {
										ok = false
									}
								}()
							}
							if field.Type().Name() == "Alias" {
								v = Alias(v.(Ref))
							}
							f.Set(reflect.ValueOf(v))
							return true
						}
						if setValue(field, term) == false {
							stack.reg = nil
						}

					}
				}

			}

		}

		// we are still decoding children of Lis/Map/Tuple/...
		if stack.i < stack.children {
			continue
		}

		if stack.reg != nil {
			term = (*stack.reg).Interface()
		} else {
			term = stack.term
		}

		// this term was the last element of List/Map/Tuple/...
		// pop from the stack, but if its the root just finish
		if stack.parent == nil {
			break
		}

		stack, stack.parent = stack.parent, nil // nil here is just a little help for GC
		goto processStack

	}

	return term, packet, nil
}
