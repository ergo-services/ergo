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
}

var (
	UseTypeRegistrar bool

	termNil = make(List, 0)

	biggestInt = big.NewInt(0xfffffffffffffff)
	lowestInt  = big.NewInt(-0xfffffffffffffff)

	ErrMalformedAtomUTF8      = fmt.Errorf("Malformed ETF. ettAtomUTF8")
	ErrMalformedSmallAtomUTF8 = fmt.Errorf("Malformed ETF. ettSmallAtomUTF8")
	ErrMalformedPacketLength  = fmt.Errorf("Malformed ETF. incorrect length of packet")
	ErrMalformedString        = fmt.Errorf("Malformed ETF. ettString")
	ErrMalformedCacheRef      = fmt.Errorf("Malformed ETF. ettCacheRef")
	ErrMalformedNewFloat      = fmt.Errorf("Malformed ETF. ettNewFloat")
	ErrMalformedSmallInteger  = fmt.Errorf("Malformed ETF. ettSmallInteger")
	ErrMalformedInteger       = fmt.Errorf("Malformed ETF. ettInteger")
	ErrMalformedSmallBig      = fmt.Errorf("Malformed ETF. ettSmallBig")
	ErrMalformedLargeBig      = fmt.Errorf("Malformed ETF. ettLargeBig")
	ErrMalformedUnknownType   = fmt.Errorf("Malformed ETF. unknown type")
	ErrMalformedList          = fmt.Errorf("Malformed ETF. ettList")
	ErrInternal               = fmt.Errorf("Internal error")
	ErrMalformed              = fmt.Errorf("Malformed ETF.")
)

func Decode(packet []byte, cache []Atom) (Term, error) {

	term, rest, err := decodeTerm(packet, []Atom{})
	if len(rest) > 0 {
		return nil, ErrMalformedPacketLength
	}
	return term, err
}

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

			term = uint8(packet[0])
			packet = packet[1:]

		case ettInteger:
			if len(packet) < 4 {
				return nil, nil, ErrMalformedInteger
			}

			term = int32(binary.BigEndian.Uint32(packet[:4]))
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
			fmt.Println("large big")
			if len(packet) < 4 {
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

		//case ettSmallTuple:
		//case ettLargeTuple:
		//case ettMap:

		case ettBinary:
			if len(packet) < 4 {
				return nil, packet, fmt.Errorf("Malformed ETF. ettBinary")
			}

			n := binary.BigEndian.Uint32(packet)
			if len(packet) < int(n+4) {
				return nil, packet, fmt.Errorf("Malformed ETF. ettBinary")
			}

			b := make([]byte, n)
			copy(b, packet[4:n+4])

			term = b
			packet = packet[n+4:]

		case ettNil:
			term = termNil

			//case ettPid:
			//case ettNewRef:
			//case ettNewerRef:

			//case ettExport:
			//case ettFun:
			//case ettNewFun:

			//case ettPort:

		case ettBitBinary:
			if len(packet) < 6 {
				return nil, packet, fmt.Errorf("Malformed ETF. ettBitBinary")
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

		// it was single element
		if stack == nil && child == nil {
			break
		}

		if stack != nil {
			switch stack.termType {
			case ettList, ettSmallTuple, ettLargeTuple:
				stack.term.(List)[stack.i] = term
				stack.i++
				// remove the last element for proper list (its ettNil)
				if stack.i == stack.children && t == ettNil {
					stack.term = stack.term.(List)[:stack.i-1]
				}
			default:
				return nil, nil, ErrInternal
			}
		}

		if child != nil {
			stack = child
			continue
		}

		if stack.i < stack.children {
			continue
		}

		// this term was the last element of List/Map/Tuple
		// pop from the stack
		if stack.parent == nil {
			term = stack.term
			break
		}

		stack, stack.parent = stack.parent, nil // nil here is just a little help for GC

	}

	return term, packet, nil
}

type Context struct{}
