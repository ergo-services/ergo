package etf

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/big"
	"unicode/utf8"
)

var (
	ConvertBinaryToString bool
	UseTypeRegistrar      bool

	biggestInt = big.NewInt(0xfffffffffffffff)
	lowestInt  = big.NewInt(-0xfffffffffffffff)
)

func Decode(packet []byte, cache []Atom) (Term, error) {

	return nil, nil
}

func decodeTerm(t byte, packet []byte, cache []Atom) (Term, []byte, error) {
	switch t {
	case ettAtomUTF8:
		if len(packet) < 2 {
			return nil, nil, fmt.Errorf("Malformed ETF. ettAtomUTF8")
		}

		n := binary.BigEndian.Uint16(packet)
		if len(packet) < int(n+2) {
			return nil, nil, fmt.Errorf("Malformed ETF. ettAtomUTF8")
		}

		return Atom(packet[2 : n+2]), packet[n+2:], nil

	case ettSmallAtomUTF8:
		if len(packet) == 0 {
			return nil, nil, fmt.Errorf("Malformed ETF. ettSmallAtomUTF8")
		}

		n := int(packet[0])
		if len(packet) < n+1 {
			return nil, nil, fmt.Errorf("Malformed ETF. ettSmallAtomUTF8")
		}

		return Atom(packet[1 : n+1]), packet[n+1:], nil

	case ettString:
		if len(packet) < 2 {
			return nil, nil, fmt.Errorf("Malformed ETF. ettString")
		}

		n := binary.BigEndian.Uint16(packet)
		if len(packet) < int(n+2) {
			return nil, nil, fmt.Errorf("Malformed ETF. ettString")
		}

		return string(packet[2 : n+2]), packet[n+2:], nil

	case ettCacheRef:
		if len(packet) == 0 {
			return nil, nil, fmt.Errorf("Malformed ETF. ettCacheRef")
		}
		return cache[int(packet[0])], packet[1:], nil

	case ettNewFloat:
		if len(packet) < 8 {
			return nil, nil, fmt.Errorf("Malformed ETF. ettNewFloat")
		}
		bits := binary.BigEndian.Uint64(packet[:8])
		return math.Float64frombits(bits), packet[8:], nil

	case ettSmallInteger:
		if len(packet) == 0 {
			return nil, nil, fmt.Errorf("Malformed ETF. ettSmallInteger")
		}
		return uint8(packet[0]), packet[1:], nil

	case ettInteger:
		if len(packet) < 4 {
			return nil, nil, fmt.Errorf("Malformed ETF. ettInteger")
		}
		return int(binary.BigEndian.Uint32(packet[:4])), packet[4:], nil

	case ettSmallBig:
		n := packet[0]
		negative := packet[1] // sign
		bytes := packet[2 : n+2]

		// encoded as little endian. convert it to big endian order
		l := len(bytes)
		for i := 0; i < l/2; i++ {
			bytes[i], bytes[l-1-i] = bytes[l-1-i], bytes[i]
		}

		bigInt := &big.Int{}
		bigInt.SetBytes(bytes)
		if negative == 1 {
			bigInt = bigInt.Neg(bigInt)
		}

		// try int and int64
		if bigInt.Cmp(biggestInt) < 0 && bigInt.Cmp(lowestInt) > 0 {
			return bigInt.Int64(), packet[n+2:], nil
		}

		return bigInt, packet[n+2:], nil

	//case ettLargeBig:

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

		if ConvertBinaryToString && utf8.Valid(packet[4:n]) {
			return string(packet[4 : n+4]), packet[n+4:], nil
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

	case ettFloat:
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

	return nil, packet, fmt.Errorf("Malformed ETF. unsupported type", t)
}

type Context struct{}
