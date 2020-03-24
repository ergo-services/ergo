package etf

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/big"
)

var (
	UseTypeRegistrar bool

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
)

func Decode(packet []byte, cache []Atom) (Term, error) {

	term, rest, err := decodeTerm(packet[0], packet[1:], []Atom{})
	if len(rest) > 0 {
		return nil, ErrMalformedPacketLength
	}
	return term, err
}

func decodeTerm(t byte, packet []byte, cache []Atom) (Term, []byte, error) {
	switch t {
	case ettAtomUTF8:
		if len(packet) < 2 {
			return nil, nil, ErrMalformedAtomUTF8
		}

		n := binary.BigEndian.Uint16(packet)
		if len(packet) < int(n+2) {
			return nil, nil, ErrMalformedAtomUTF8
		}

		return Atom(packet[2 : n+2]), packet[n+2:], nil

	case ettSmallAtomUTF8:
		if len(packet) == 0 {
			return nil, nil, ErrMalformedSmallAtomUTF8
		}

		n := int(packet[0])
		if len(packet) < n+1 {
			return nil, nil, ErrMalformedSmallAtomUTF8
		}

		return Atom(packet[1 : n+1]), packet[n+1:], nil

	case ettString:
		if len(packet) < 2 {
			return nil, nil, ErrMalformedString
		}

		n := binary.BigEndian.Uint16(packet)
		if len(packet) < int(n+2) {
			return nil, nil, ErrMalformedString
		}

		return string(packet[2 : n+2]), packet[n+2:], nil

	case ettCacheRef:
		if len(packet) == 0 {
			return nil, nil, ErrMalformedCacheRef
		}
		return cache[int(packet[0])], packet[1:], nil

	case ettNewFloat:
		if len(packet) < 8 {
			return nil, nil, ErrMalformedNewFloat
		}
		bits := binary.BigEndian.Uint64(packet[:8])
		return math.Float64frombits(bits), packet[8:], nil

	case ettSmallInteger:
		if len(packet) == 0 {
			return nil, nil, ErrMalformedSmallInteger
		}
		return uint8(packet[0]), packet[1:], nil

	case ettInteger:
		if len(packet) < 4 {
			return nil, nil, ErrMalformedInteger
		}
		return int32(binary.BigEndian.Uint32(packet[:4])), packet[4:], nil

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
			return int64(smallBig), packet[n+2:], nil
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
			return bigInt.Int64(), packet[n+2:], nil
		}

		return bigInt, packet[n+2:], nil

	case ettLargeBig:
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

		return bigInt, packet[n+5:], nil

	//case ettList:
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

		return b, packet[n+4:], nil

	case ettBitBinary:
		if len(packet) < 6 {
			return nil, packet, fmt.Errorf("Malformed ETF. ettBitBinary")
		}

		n := binary.BigEndian.Uint32(packet)
		bits := uint(packet[4])

		b := make([]byte, n)
		copy(b, packet[5:n+5])
		b[n-1] = b[n-1] >> (8 - bits)

		return b, packet[n+5:], nil

	case ettNil:
		return make(List, 0), packet, nil

	//case ettPid:
	//case ettNewRef:
	//case ettNewerRef:

	//case ettExport:
	//case ettFun:
	//case ettNewFun:

	//case ettPort:

	case ettFloat: // legacy. wont test and bench this shit
		var f float64

		if len(packet) < 32 {
			return nil, packet, fmt.Errorf("Malformed ETF. ettFloat")
		}

		n, err := fmt.Sscanf(string(packet[:32]), "%f", &f)
		if err != nil || n != 1 {
			return nil, packet, fmt.Errorf("Malformed ETF. ettFloat")
		}

		return f, packet[32:], nil
	}

	return nil, packet, fmt.Errorf("Malformed ETF. unsupported type %d", t)
}

type Context struct{}
