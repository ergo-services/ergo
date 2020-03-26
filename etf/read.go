package etf

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/big"
)

// linked list for decoding complex types like list/map/tuple
type stackElement struct {
	parent *stackElement

	termType byte

	term     Term //value
	i        int  // current
	children int
	tmp      Term // temporary value. uses as a temporary storage for a key of map
}

var (
	termNil = make(List, 0)

	biggestInt = big.NewInt(0xfffffffffffffff)
	lowestInt  = big.NewInt(-0xfffffffffffffff)

	ErrMalformedAtomUTF8      = fmt.Errorf("Malformed ETF. ettAtomUTF8")
	ErrMalformedSmallAtomUTF8 = fmt.Errorf("Malformed ETF. ettSmallAtomUTF8")
	ErrMalformedString        = fmt.Errorf("Malformed ETF. ettString")
	ErrMalformedCacheRef      = fmt.Errorf("Malformed ETF. ettCacheRef")
	ErrMalformedNewFloat      = fmt.Errorf("Malformed ETF. ettNewFloat")
	ErrMalformedSmallInteger  = fmt.Errorf("Malformed ETF. ettSmallInteger")
	ErrMalformedInteger       = fmt.Errorf("Malformed ETF. ettInteger")
	ErrMalformedSmallBig      = fmt.Errorf("Malformed ETF. ettSmallBig")
	ErrMalformedLargeBig      = fmt.Errorf("Malformed ETF. ettLargeBig")
	ErrMalformedUnknownType   = fmt.Errorf("Malformed ETF. unknown type")
	ErrMalformedList          = fmt.Errorf("Malformed ETF. ettList")
	ErrMalformedSmallTuple    = fmt.Errorf("Malformed ETF. ettSmallTuple")
	ErrMalformedLargeTuple    = fmt.Errorf("Malformed ETF. ettLargeTuple")
	ErrMalformedMap           = fmt.Errorf("Malformed ETF. ettMap")
	ErrMalformedBinary        = fmt.Errorf("Malformed ETF. ettBinary")
	ErrMalformedBitBinary     = fmt.Errorf("Malformed ETF. ettBitBinary")
	ErrMalformedPid           = fmt.Errorf("Malformed ETF. ettPid")
	ErrMalformedNewPid        = fmt.Errorf("Malformed ETF. ettNewPid")
	ErrMalformedRef           = fmt.Errorf("Malformed ETF. ettNewRef")
	ErrMalformedNewRef        = fmt.Errorf("Malformed ETF. ettNewerRef")
	ErrMalformedPort          = fmt.Errorf("Malformed ETF. ettPort")
	ErrMalformedNewPort       = fmt.Errorf("Malformed ETF. ettNewPort")
	ErrMalformedPacketLength  = fmt.Errorf("Malformed ETF. incorrect length of packet")

	ErrMalformed = fmt.Errorf("Malformed ETF")
	ErrInternal  = fmt.Errorf("Internal error")
)

func Decode(packet []byte, cache []Atom) (Term, error) {

	term, rest, err := decodeTerm(packet, []Atom{})
	if len(rest) > 0 {
		return nil, ErrMalformedPacketLength
	}
	return term, err
}

// i know it looks super hard to understand the logic, but there are only 3 stages
// 1) Stage1: decoding basic types (long list of type we have to support)
// 2) Stage2: decoding list/tuples/maps and complex types like Port/Pid/Ref using stack
// 3) Stage3: handling nested types like Tuple{List{Map}}
//
// see comments within this function

func decodeTerm(packet []byte, cache []Atom) (Term, []byte, error) {
	// could be a naive implementation with recursion but its too expensive.
	// using iterative way is speeding up it up to x25 times
	// as an example https://medium.com/@felipedutratine/iterative-vs-recursive-vs-tail-recursive-in-golang-c196ca5fd489

	var term Term
	var stack *stackElement
	var child *stackElement
	var t byte

	for {
		child = nil
		if len(packet) == 0 {
			return nil, nil, ErrMalformed
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
				return nil, nil, ErrMalformedAtomUTF8
			}

			n := binary.BigEndian.Uint16(packet)
			if len(packet) < int(n+2) {
				return nil, nil, ErrMalformedAtomUTF8
			}

			term = Atom(packet[2 : n+2])
			packet = packet[n+2:]

		case ettSmallAtomUTF8, ettSmallAtom:
			if len(packet) == 0 {
				return nil, nil, ErrMalformedSmallAtomUTF8
			}

			n := int(packet[0])
			if len(packet) < n+1 {
				return nil, nil, ErrMalformedSmallAtomUTF8
			}

			term = Atom(packet[1 : n+1])
			packet = packet[n+1:]

		case ettString:
			if len(packet) < 2 {
				return nil, nil, ErrMalformedString
			}

			n := binary.BigEndian.Uint16(packet)
			if len(packet) < int(n+2) {
				return nil, nil, ErrMalformedString
			}

			term = string(packet[2 : n+2])
			packet = packet[n+2:]

		case ettCacheRef:
			if len(packet) == 0 {
				return nil, nil, ErrMalformedCacheRef
			}
			term = cache[int(packet[0])]
			packet = packet[1:]

		case ettNewFloat:
			if len(packet) < 8 {
				return nil, nil, ErrMalformedNewFloat
			}
			bits := binary.BigEndian.Uint64(packet[:8])

			term = math.Float64frombits(bits)
			packet = packet[8:]

		case ettSmallInteger:
			if len(packet) == 0 {
				return nil, nil, ErrMalformedSmallInteger
			}

			term = int(packet[0])
			packet = packet[1:]

		case ettInteger:
			if len(packet) < 4 {
				return nil, nil, ErrMalformedInteger
			}

			term = int64(int32(binary.BigEndian.Uint32(packet[:4])))
			packet = packet[4:]

		case ettSmallBig:
			if len(packet) == 0 {
				return nil, nil, ErrMalformedSmallBig
			}

			n := packet[0]
			negative := packet[1] == 1 // sign

			///// this block improve the performance at least 4 times
			// see details in benchmarks
			if n < 8 { // treat as an int64
				le8 := make([]byte, 8)
				copy(le8, packet[2:n+2])
				smallBig := binary.LittleEndian.Uint64(le8)
				if negative {
					smallBig = -smallBig
				}

				term = int64(smallBig)
				packet = packet[n+2:]
				break
			}
			/////

			if len(packet) < int(n+2) {
				return nil, nil, ErrMalformedSmallBig
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
				return nil, nil, ErrMalformedLargeBig
			}

			n := binary.BigEndian.Uint32(packet[:4])
			negative := packet[4] == 1 // sign

			if len(packet) < int(n+5) {
				return nil, nil, ErrMalformedLargeBig
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
				return nil, nil, ErrMalformedList
			}

			n := binary.BigEndian.Uint32(packet[:4])
			if n == 0 {
				// must be encoded as ettNil
				return nil, nil, ErrMalformedList
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
				return nil, nil, ErrMalformedSmallTuple
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
				return nil, nil, ErrMalformedLargeTuple
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
				return nil, nil, ErrMalformedMap
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
				return nil, packet, ErrMalformedBinary
			}

			n := binary.BigEndian.Uint32(packet)
			if len(packet) < int(n+4) {
				return nil, packet, ErrMalformedBinary
			}

			b := make([]byte, n)
			copy(b, packet[4:n+4])

			term = b
			packet = packet[n+4:]

		case ettNil:
			term = termNil

		case ettPid, ettNewPid:
			child = &stackElement{
				parent:   stack,
				termType: t,
				children: 1,
			}

		case ettNewRef, ettNewerRef:
			if len(packet) < 2 {
				return nil, nil, ErrMalformedRef
			}

			l := binary.BigEndian.Uint16(packet[:2])
			packet = packet[2:]

			child = &stackElement{
				parent:   stack,
				termType: t,
				children: 1,
				tmp:      l, // save length in temporary place of the stack element
			}

			//case ettExport:
			//case ettFun:
			//case ettNewFun:

		case ettPort, ettNewPort:
			child = &stackElement{
				parent:   stack,
				termType: t,
				children: 1,
			}

		case ettBitBinary:
			if len(packet) < 6 {
				return nil, packet, ErrMalformedBitBinary
			}

			n := binary.BigEndian.Uint32(packet)
			bits := uint(packet[4])

			b := make([]byte, n)
			copy(b, packet[5:n+5])
			b[n-1] = b[n-1] >> (8 - bits)

			term = b
			packet = packet[n+5:]

		default:
			term = nil
			return nil, nil, ErrMalformedUnknownType
		}

		// it was a single element
		if stack == nil && child == nil {
			break
		}

		// Stage 2:
		// decoded item is an item of List/Map/Tuple
		// or
		// we decoding complex type like Pid/Port/Ref
		if stack != nil {
			switch stack.termType {
			case ettList:
				stack.term.(List)[stack.i] = term
				stack.i++
				// remove the last element for proper list (its ettNil)
				if stack.i == stack.children && t == ettNil {
					stack.term = stack.term.(List)[:stack.i-1]
				}

			case ettSmallTuple, ettLargeTuple:
				stack.term.(Tuple)[stack.i] = term
				stack.i++

			case ettMap:
				if stack.i&0x01 == 0x01 { // value
					stack.term.(Map)[stack.tmp] = term
					stack.i++
					break
				}

				// a key
				stack.tmp = term
				stack.i++

			case ettPid:
				if len(packet) != 9 {
					return nil, nil, ErrMalformedPid
				}

				name, ok := term.(Atom)
				if !ok {
					return nil, nil, ErrMalformedPid
				}

				pid := Pid{
					Node:     name,
					Id:       binary.BigEndian.Uint32(packet[:4]),
					Serial:   binary.BigEndian.Uint32(packet[4:8]),
					Creation: packet[8] & 3, // only two bits are significant, rest are to be 0
				}

				packet = packet[9:]
				stack.term = pid
				stack.i++

			case ettNewPid:
				if len(packet) != 12 {
					return nil, nil, ErrMalformedNewPid
				}

				name, ok := term.(Atom)
				if !ok {
					return nil, nil, ErrMalformedPid
				}

				pid := Pid{
					Node:   name,
					Id:     binary.BigEndian.Uint32(packet[:4]),
					Serial: binary.BigEndian.Uint32(packet[4:8]),
					// FIXME: we must upgrade this type to uint32
					// Creation: binary.BigEndian.Uint32(packet[8:12])
					Creation: packet[11], // use the last byte for a while
				}

				packet = packet[12:]
				stack.term = pid
				stack.i++

			case ettNewRef:
				var id uint32
				name, ok := term.(Atom)
				if !ok {
					return nil, nil, ErrMalformedRef
				}

				l := stack.tmp.(uint16)
				stack.tmp = nil
				expectedLength := int(1 + l*4)

				if len(packet) < expectedLength {
					return nil, nil, ErrMalformedRef
				}

				ref := Ref{
					Node:     name,
					Id:       make([]uint32, l),
					Creation: packet[0],
				}
				packet = packet[1:]

				for i := 0; i < int(l); i++ {
					id = binary.BigEndian.Uint32(packet[:4])
					ref.Id[i] = id
					packet = packet[4:]
				}

				stack.term = ref
				stack.i++

			case ettNewerRef:
				var id uint32
				name, ok := term.(Atom)
				if !ok {
					return nil, nil, ErrMalformedRef
				}

				l := stack.tmp.(uint16)
				stack.tmp = nil
				expectedLength := int(4 + l*4)

				if len(packet) < expectedLength {
					return nil, nil, ErrMalformedRef
				}

				ref := Ref{
					Node: name,
					Id:   make([]uint32, l),
					// FIXME: we must upgrade this type to uint32
					// ref.Creation = binary.BigEndian.Uint32(packet[:4])
					Creation: packet[3],
				}
				packet = packet[4:]

				for i := 0; i < int(l); i++ {
					id = binary.BigEndian.Uint32(packet[:4])
					ref.Id[i] = id
					packet = packet[4:]
				}

				stack.term = ref
				stack.i++

			case ettPort:
				if len(packet) != 5 {
					return nil, nil, ErrMalformedPort
				}

				name, ok := term.(Atom)
				if !ok {
					return nil, nil, ErrMalformedPort
				}

				port := Port{
					Node:     name,
					Id:       binary.BigEndian.Uint32(packet[:4]),
					Creation: packet[4],
				}

				packet = packet[5:]
				stack.term = port
				stack.i++

			case ettNewPort:
				if len(packet) != 8 {
					return nil, nil, ErrMalformedNewPort
				}

				name, ok := term.(Atom)
				if !ok {
					return nil, nil, ErrMalformedNewPort
				}

				port := Port{
					Node: name,
					Id:   binary.BigEndian.Uint32(packet[:4]),
					// FIXME: we must upgrade this type to uint32
					// Creation: binary.BigEndian.Uint32(packet[4:8])
					Creation: packet[7],
				}

				packet = packet[8:]
				stack.term = port
				stack.i++

			default:
				return nil, nil, ErrInternal
			}
		}

		// decoded child item is List/Map/Tuple. Going deeper
		if child != nil {
			stack = child
			continue
		}

		// we are still decoding children of Lis/Map/Tuple
		if stack.i < stack.children {
			continue
		}

		term = stack.term

		// this term was the last element of List/Map/Tuple (single level)
		// pop from the stack
		if stack.parent == nil {
			break
		}

		// stage 3: parent is List/Tuple/Map and we need to place
		// decoded term into the right place

		stack, stack.parent = stack.parent, nil // nil here is just a little help for GC

		switch stack.termType {
		case ettSmallTuple, ettLargeTuple:
			stack.term.(Tuple)[stack.i-1] = term

		case ettList:
			stack.term.(List)[stack.i-1] = term

		case ettMap:
			// stack.tmp has a key we stored earlier
			stack.term.(Map)[stack.tmp] = term

		default:
			return nil, nil, ErrInternal
		}

		// since we switched to the parent stack item we have to make the same
		// checks we have done before the stage 3 (at the end of stage 2)

		if stack.i < stack.children {
			continue
		}

		term = stack.term

		if stack.parent == nil {
			break
		}
	}

	return term, packet, nil
}

type Context struct{}
