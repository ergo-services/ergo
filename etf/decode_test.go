package etf

import (
	"math/big"
	"reflect"
	"testing"
)

func TestDecodeAtom(t *testing.T) {
	expected := Atom("abc")
	packet := []byte{ettAtomUTF8, 0, 3, 97, 98, 99}
	term, _, err := Decode(packet, []Atom{})
	if err != nil || term != expected {
		t.Fatal(err)
	}

	packet = []byte{ettSmallAtomUTF8, 3, 97, 98, 99}
	term, _, err = Decode(packet, []Atom{})
	if err != nil || term != expected {
		t.Fatal(err)
	}

	packet = []byte{ettSmallAtomUTF8, 4, 97, 98, 99}
	term, _, err = Decode(packet, []Atom{})
	if err != errMalformedSmallAtomUTF8 {
		t.Fatal(err)
	}

	packet = []byte{119}
	term, _, err = Decode(packet, []Atom{})
	if err != errMalformedSmallAtomUTF8 {
		t.Fatal(err)
	}

}

func TestDecodeString(t *testing.T) {
	expected := "abc"
	packet := []byte{ettString, 0, 3, 97, 98, 99}
	term, _, err := Decode(packet, []Atom{})
	if err != nil || term != expected {
		t.Fatal(err)
	}

	packet = []byte{ettString, 3, 97, 98, 99}
	term, _, err = Decode(packet, []Atom{})
	if err != errMalformedString {
		t.Fatal(err)
	}
}

func TestDecodeNewFloat(t *testing.T) {
	expected := float64(2.1)
	packet := []byte{ettNewFloat, 64, 0, 204, 204, 204, 204, 204, 205}
	term, _, err := Decode(packet, []Atom{})
	if err != nil || term != expected {
		t.Fatal(err)
	}

	packet = []byte{ettNewFloat, 64, 0, 204, 204, 204, 204, 204}
	term, _, err = Decode(packet, []Atom{})
	if err != errMalformedNewFloat {
		t.Fatal(err)
	}
}

func TestDecodeInteger(t *testing.T) {
	expected := int(88)
	packet := []byte{ettSmallInteger, 88}

	term, _, err := Decode(packet, []Atom{})
	if err != nil || term != expected {
		t.Fatal(err)
	}

	packet = []byte{ettSmallInteger}
	term, _, err = Decode(packet, []Atom{})
	if err != errMalformedSmallInteger {
		t.Fatal(err)
	}

	expectedInteger := int64(-1234567890)
	packet = []byte{ettInteger, 182, 105, 253, 46}
	term, _, err = Decode(packet, []Atom{})
	if err != nil || term != expectedInteger {
		t.Fatal(err, expectedInteger, term)
	}

	packet = []byte{ettInteger, 182, 105, 253}
	term, _, err = Decode(packet, []Atom{})
	if err != errMalformedInteger {
		t.Fatal(err)
	}

	//-1234567890987654321
	bigInt := new(big.Int)
	bigInt.SetString("-1234567890987654321", 10)
	packet = []byte{ettSmallBig, 8, 1, 177, 28, 108, 177, 244, 16, 34, 17}
	term, _, err = Decode(packet, []Atom{})
	if err != nil || bigInt.Cmp(term.(*big.Int)) != 0 {
		t.Fatal(err, term, bigInt)
	}

	largeBigString := "-12345678909876543210123456789098765432101234567890987654321012345678909876543210123456789098765432101234567890987654321012345678909876543210123456789098765432101234567890987654321012345678909876543210123456789098765432101234567890987654321012345678909876543210123456789098765432101234567890987654321012345678909876543210123456789098765432101234567890987654321012345678909876543210123456789098765432101234567890987654321012345678909876543210123456789098765432101234567890987654321012345678909876543210123456789098765432101234567890987654321012345678909876543210123456789098765432101234567890987654321012345678909876543210123456789098765432101234567890987654321012345678909876543210123456789098765432101234567890987654321012345678909876543210123456789098765432101234567890987654321012345678909876543210123456789098765432101234567890987654321012345678909876543210123456789098765432101234567890987654321012345678909876543210123456789098765432101234567890"

	bigInt.SetString(largeBigString, 10)
	packet = []byte{ettLargeBig, 0, 0, 1, 139, 1, 210, 106, 44, 197, 54, 176, 151, 83, 243,
		228, 193, 133, 194, 193, 21, 90, 196, 4, 252, 150, 226, 188, 79, 11, 8,
		190, 106, 189, 21, 73, 176, 196, 54, 221, 118, 232, 212, 126, 141,
		118, 154, 78, 238, 143, 34, 118, 245, 135, 17, 231, 224, 86, 71, 12,
		175, 207, 224, 19, 206, 5, 241, 241, 207, 125, 243, 87, 18, 14, 162,
		71, 3, 244, 85, 240, 211, 12, 141, 5, 38, 124, 232, 122, 104, 228, 36,
		40, 124, 109, 196, 20, 94, 46, 167, 215, 107, 53, 51, 28, 45, 249, 146,
		151, 18, 11, 246, 151, 220, 138, 139, 97, 63, 166, 255, 101, 12, 153,
		247, 201, 62, 9, 131, 235, 67, 85, 13, 151, 200, 233, 239, 35, 224, 10,
		101, 144, 107, 82, 206, 71, 226, 67, 212, 254, 15, 29, 122, 128, 38,
		230, 60, 97, 146, 52, 241, 216, 220, 114, 82, 90, 166, 207, 31, 63,
		112, 254, 19, 111, 225, 104, 159, 133, 186, 15, 5, 93, 220, 56, 6, 4,
		197, 4, 196, 204, 94, 34, 144, 141, 31, 165, 188, 241, 105, 197, 82,
		69, 77, 136, 207, 152, 76, 112, 79, 57, 159, 232, 165, 215, 0, 164,
		231, 132, 124, 252, 90, 91, 71, 198, 254, 203, 83, 96, 42, 35, 240,
		218, 174, 37, 112, 86, 218, 203, 135, 7, 88, 24, 245, 50, 173, 72, 133,
		70, 2, 160, 235, 61, 151, 28, 124, 173, 254, 244, 37, 96, 19, 40, 192,
		194, 51, 75, 51, 186, 229, 93, 142, 165, 50, 43, 129, 0, 78, 253, 159,
		105, 151, 150, 253, 24, 109, 22, 123, 95, 55, 143, 126, 122, 109, 57,
		73, 240, 191, 25, 140, 131, 27, 64, 252, 238, 174, 211, 89, 167, 38,
		137, 32, 176, 174, 122, 64, 66, 171, 175, 113, 174, 247, 236, 67, 180,
		179, 23, 58, 17, 117, 223, 18, 184, 223, 156, 151, 179, 18, 84, 145,
		16, 18, 194, 121, 19, 186, 170, 49, 21, 7, 108, 174, 89, 59, 53, 62,
		247, 232, 9, 184, 242, 60, 137, 96, 54, 183, 89, 206, 219, 81, 208,
		214, 197, 254, 207, 3, 41, 224, 169, 181, 56, 132, 18, 116, 141, 89,
		185, 133, 186, 46, 81, 244, 139, 188, 171, 206, 52, 225, 160, 232,
		246, 254, 193, 1}

	term, _, err = Decode(packet, []Atom{})
	if err != nil || bigInt.Cmp(term.(*big.Int)) != 0 {
		t.Fatal(err, term, bigInt)
	}

	//-123456789098 should be treated as int64
	expectedInt64 := int64(-123456789098)
	packet = []byte{ettSmallBig, 5, 1, 106, 26, 153, 190, 28}
	term, _, err = Decode(packet, []Atom{})
	if err != nil || term != expectedInt64 {
		t.Fatal(err, term, expectedInt64)
	}
}

func TestDecodeList(t *testing.T) {
	expected := List{3.14, Atom("abc"), int64(987654321)}
	packet := []byte{ettList, 0, 0, 0, 3, 70, 64, 9, 30, 184, 81, 235, 133, 31, 100, 0, 3, 97,
		98, 99, 98, 58, 222, 104, 177, 106}
	term, _, err := Decode(packet, []Atom{})
	if err != nil {
		t.Fatal(err)
	}
	result := term.(List)
	if !reflect.DeepEqual(expected, result) {
		t.Fatal("result != expected")
	}

}

func TestDecodeListNested(t *testing.T) {
	// [1,[2,3,[4,5],6]]
	expected := List{1, List{2, 3, List{4, 5}, 6}}
	packet := []byte{108, 0, 0, 0, 2, 97, 1, 108, 0, 0, 0, 4, 97, 2, 97, 3, 108, 0, 0, 0, 2, 97, 4, 97, 5, 106,
		97, 6, 106, 106}

	term, _, err := Decode(packet, []Atom{})
	if err != nil {
		t.Fatal(err)
	}

	result := term.(List)
	if !reflect.DeepEqual(expected, result) {
		t.Fatal("result != expected")
	}
}

func TestDecodeTuple(t *testing.T) {
	expected := Tuple{3.14, Atom("abc"), int64(987654321)}
	packet := []byte{ettSmallTuple, 3, 70, 64, 9, 30, 184, 81, 235, 133, 31, 100, 0, 3, 97, 98, 99,
		98, 58, 222, 104, 177}

	term, _, err := Decode(packet, []Atom{})
	if err != nil {
		t.Fatal(err)
	}
	result := term.(Tuple)
	if !reflect.DeepEqual(expected, result) {
		t.Fatal("result != expected")
	}
}

func TestDecodeMap(t *testing.T) {
	expected := Map{
		Atom("abc"): 123,
		"abc":       4.56,
	}
	packet := []byte{116, 0, 0, 0, 2, 100, 0, 3, 97, 98, 99, 97, 123, 107, 0, 3, 97, 98,
		99, 70, 64, 18, 61, 112, 163, 215, 10, 61}

	term, _, err := Decode(packet, []Atom{})
	if err != nil {
		t.Fatal(err)
	}

	result := term.(Map)
	if !reflect.DeepEqual(expected, result) {
		t.Fatal("result != expected")
	}

}

func TestDecodeBinary(t *testing.T) {
	expected := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 0}
	packet := []byte{ettBinary, 0, 0, 0, 10, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0}

	term, _, err := Decode(packet, []Atom{})
	if err != nil {
		t.Fatal(err)
	}

	result := term.([]byte)
	if !reflect.DeepEqual(expected, result) {
		t.Fatal("result != expected")
	}
}

func TestDecodeBitBinary(t *testing.T) {
	expected := []byte{1, 2, 3, 4, 5}
	packet := []byte{77, 0, 0, 0, 5, 3, 1, 2, 3, 4, 160}

	term, _, err := Decode(packet, []Atom{})
	if err != nil {
		t.Fatal(err)
	}

	result := term.([]byte)
	if !reflect.DeepEqual(expected, result) {
		t.Fatal("result != expected")
	}

}

func TestDecodePid(t *testing.T) {
	expected := Pid{
		Node:     Atom("erl-demo@127.0.0.1"),
		ID:       142,
		Serial:   0,
		Creation: 2,
	}
	packet := []byte{103, 100, 0, 18, 101, 114, 108, 45, 100, 101, 109, 111, 64, 49,
		50, 55, 46, 48, 46, 48, 46, 49, 0, 0, 0, 142, 0, 0, 0, 0, 2}
	term, _, err := Decode(packet, []Atom{})
	if err != nil {
		t.Fatal(err)
	}

	result := term.(Pid)
	if !reflect.DeepEqual(expected, result) {
		t.Fatal("result != expected")
	}
}

func TestDecodePidWithCacheAtom(t *testing.T) {
	expected := Pid{
		Node:     Atom("erl-demo@127.0.0.1"),
		ID:       142,
		Serial:   0,
		Creation: 2,
	}
	packet := []byte{103, ettCacheRef, 0, 0, 0, 0, 142, 0, 0, 0, 0, 2}
	cache := []Atom{Atom("erl-demo@127.0.0.1")}
	term, _, err := Decode(packet, cache)
	if err != nil {
		t.Fatal(err)
	}

	result := term.(Pid)
	if !reflect.DeepEqual(expected, result) {
		t.Fatal("result != expected")
	}
}

func TestDecodeRef(t *testing.T) {
	expected := Ref{
		Node:     Atom("erl-demo@127.0.0.1"),
		Creation: 2,
		ID:       []uint32{73444, 3082813441, 2373634851},
	}
	packet := []byte{114, 0, 3, 100, 0, 18, 101, 114, 108, 45, 100, 101, 109, 111, 64,
		49, 50, 55, 46, 48, 46, 48, 46, 49, 2, 0, 1, 30, 228, 183, 192, 0, 1, 141,
		122, 203, 35}
	term, _, err := Decode(packet, []Atom{})
	if err != nil {
		t.Fatal(err)
	}

	result := term.(Ref)
	if !reflect.DeepEqual(expected, result) {
		t.Fatal("result != expected")
	}
}

func TestDecodeRefWithAtomCache(t *testing.T) {
	expected := Ref{
		Node:     Atom("erl-demo@127.0.0.1"),
		Creation: 2,
		ID:       []uint32{73444, 3082813441, 2373634851},
	}
	packet := []byte{114, 0, 3, ettCacheRef, 0, 2, 0, 1, 30, 228, 183, 192, 0, 1, 141,
		122, 203, 35}
	cache := []Atom{Atom("erl-demo@127.0.0.1")}

	term, _, err := Decode(packet, cache)
	if err != nil {
		t.Fatal(err)
	}

	result := term.(Ref)
	if !reflect.DeepEqual(expected, result) {
		t.Fatal("result != expected")
	}
}
func TestDecodeTupleRefPid(t *testing.T) {
	expected := Tuple{
		Ref{
			Node:     Atom("erl-demo@127.0.0.1"),
			Creation: 2,
			ID:       []uint32{0x11f1c, 0xb7c00001, 0x8d7acb23}},
		Pid{
			Node:     Atom("erl-demo@127.0.0.1"),
			ID:       0x8e,
			Serial:   0x0,
			Creation: 0x2}}
	packet := []byte{ettSmallTuple, 2, ettNewRef, 0, 3, ettAtom, 0, 18, 101, 114, 108, 45, 100, 101, 109,
		111, 64, 49, 50, 55, 46, 48, 46, 48, 46, 49, 2, 0, 1, 31, 28, 183, 192, 0,
		1, 141, 122, 203, 35, 103, 100, 0, 18, 101, 114, 108, 45, 100, 101,
		109, 111, 64, 49, 50, 55, 46, 48, 46, 48, 46, 49, 0, 0, 0, 142, 0, 0, 0, 0,
		2}
	term, _, err := Decode(packet, []Atom{})
	if err != nil {
		t.Fatal(err)
	}

	result := term.(Tuple)
	if !reflect.DeepEqual(expected, result) {
		t.Fatal("result != expected")
	}
}

func TestDecodeTupleRefPidWithAtomCache(t *testing.T) {
	expected := Tuple{
		Ref{
			Node:     Atom("erl-demo@127.0.0.1"),
			Creation: 2,
			ID:       []uint32{0x11f1c, 0xb7c00001, 0x8d7acb23}},
		Pid{
			Node:     Atom("erl-demo@127.0.0.1"),
			ID:       0x8e,
			Serial:   0x0,
			Creation: 0x2}}
	packet := []byte{ettSmallTuple, 2, ettNewRef, 0, 3, ettCacheRef, 0,
		2, 0, 1, 31, 28, 183, 192, 0, 1, 141, 122, 203, 35, 103, ettCacheRef, 0,
		0, 0, 0, 142, 0, 0, 0, 0, 2}
	cache := []Atom{Atom("erl-demo@127.0.0.1")}
	term, _, err := Decode(packet, cache)
	if err != nil {
		t.Fatal(err)
	}

	result := term.(Tuple)
	if !reflect.DeepEqual(expected, result) {
		t.Fatal("result != expected")
	}
}
func TestDecodePort(t *testing.T) {
	expected := Port{
		Node:     Atom("erl-demo@127.0.0.1"),
		Creation: 2,
		ID:       32,
	}
	packet := []byte{102, 100, 0, 18, 101, 114, 108, 45, 100, 101, 109, 111, 64, 49,
		50, 55, 46, 48, 46, 48, 46, 49, 0, 0, 0, 32, 2}

	term, _, err := Decode(packet, []Atom{})
	if err != nil {
		t.Fatal(err)
	}

	result := term.(Port)
	if !reflect.DeepEqual(expected, result) {
		t.Fatal("result != expected")
	}
}

func TestDecodeComplex(t *testing.T) {
	//{"hello",[], #{v1 => [{3,13,3.13}, {abc, "abc"}], v2 => 12345}}.
	expected := Tuple{"hello", List{},
		Map{Atom("v1"): List{Tuple{3, 13, 3.13}, Tuple{Atom("abc"), "abc"}},
			Atom("v2"): int64(12345)}}
	packet := []byte{104, 3, 107, 0, 5, 104, 101, 108, 108, 111, 106, 116, 0, 0, 0, 2,
		100, 0, 2, 118, 49, 108, 0, 0, 0, 2, 104, 3, 97, 3, 97, 13, 70, 64, 9, 10,
		61, 112, 163, 215, 10, 104, 2, 100, 0, 3, 97, 98, 99, 107, 0, 3, 97, 98,
		99, 106, 100, 0, 2, 118, 50, 98, 0, 0, 48, 57}
	term, _, err := Decode(packet, []Atom{})
	if err != nil {
		t.Fatal(err)
	}

	result := term.(Tuple)
	if !reflect.DeepEqual(expected, result) {
		t.Fatal("result != expected")
	}
}

var packetFunction []byte

func TestDecodeFunction(t *testing.T) {
	// A = fun(X) -> X*2 end.
	packet := []byte{112, 0, 0, 3, 76, 1, 245, 82, 198, 227, 120, 209, 152, 67, 80, 234,
		138, 144, 123, 165, 151, 196, 0, 0, 0, 6, 0, 0, 0, 1, 100, 0, 8, 101, 114,
		108, 95, 101, 118, 97, 108, 97, 6, 98, 7, 170, 150, 55, 103, 100, 0, 20,
		101, 114, 108, 45, 100, 101, 109, 111, 50, 50, 64, 49, 50, 55, 46, 48,
		46, 48, 46, 49, 0, 0, 0, 83, 0, 0, 0, 0, 1, 104, 4, 106, 104, 2, 100, 0, 4,
		101, 118, 97, 108, 112, 0, 0, 2, 29, 3, 196, 150, 93, 173, 104, 167,
		134, 253, 184, 200, 203, 147, 166, 63, 88, 201, 0, 0, 0, 21, 0, 0, 0, 4,
		100, 0, 5, 115, 104, 101, 108, 108, 97, 21, 98, 6, 36, 178, 237, 103,
		100, 0, 20, 101, 114, 108, 45, 100, 101, 109, 111, 50, 50, 64, 49, 50,
		55, 46, 48, 46, 48, 46, 49, 0, 0, 0, 83, 0, 0, 0, 0, 1, 104, 2, 100, 0, 5,
		118, 97, 108, 117, 101, 112, 0, 0, 0, 110, 2, 196, 150, 93, 173, 104,
		167, 134, 253, 184, 200, 203, 147, 166, 63, 88, 201, 0, 0, 0, 5, 0, 0, 0,
		1, 100, 0, 5, 115, 104, 101, 108, 108, 97, 5, 98, 6, 36, 178, 237, 103,
		100, 0, 20, 101, 114, 108, 45, 100, 101, 109, 111, 50, 50, 64, 49, 50,
		55, 46, 48, 46, 48, 46, 49, 0, 0, 0, 83, 0, 0, 0, 0, 1, 103, 100, 0, 20,
		101, 114, 108, 45, 100, 101, 109, 111, 50, 50, 64, 49, 50, 55, 46, 48,
		46, 48, 46, 49, 0, 0, 0, 77, 0, 0, 0, 0, 1, 114, 0, 3, 100, 0, 20, 101, 114,
		108, 45, 100, 101, 109, 111, 50, 50, 64, 49, 50, 55, 46, 48, 46, 48, 46,
		49, 1, 0, 3, 219, 136, 225, 146, 0, 9, 253, 168, 114, 208, 103, 100, 0,
		20, 101, 114, 108, 45, 100, 101, 109, 111, 50, 50, 64, 49, 50, 55, 46,
		48, 46, 48, 46, 49, 0, 0, 0, 77, 0, 0, 0, 0, 1, 112, 0, 0, 1, 14, 1, 196,
		150, 93, 173, 104, 167, 134, 253, 184, 200, 203, 147, 166, 63, 88,
		201, 0, 0, 0, 12, 0, 0, 0, 3, 100, 0, 5, 115, 104, 101, 108, 108, 97, 12,
		98, 6, 36, 178, 237, 103, 100, 0, 20, 101, 114, 108, 45, 100, 101, 109,
		111, 50, 50, 64, 49, 50, 55, 46, 48, 46, 48, 46, 49, 0, 0, 0, 83, 0, 0, 0,
		0, 1, 104, 2, 100, 0, 5, 118, 97, 108, 117, 101, 112, 0, 0, 0, 110, 2,
		196, 150, 93, 173, 104, 167, 134, 253, 184, 200, 203, 147, 166, 63,
		88, 201, 0, 0, 0, 5, 0, 0, 0, 1, 100, 0, 5, 115, 104, 101, 108, 108, 97, 5,
		98, 6, 36, 178, 237, 103, 100, 0, 20, 101, 114, 108, 45, 100, 101, 109,
		111, 50, 50, 64, 49, 50, 55, 46, 48, 46, 48, 46, 49, 0, 0, 0, 83, 0, 0, 0,
		0, 1, 103, 100, 0, 20, 101, 114, 108, 45, 100, 101, 109, 111, 50, 50,
		64, 49, 50, 55, 46, 48, 46, 48, 46, 49, 0, 0, 0, 77, 0, 0, 0, 0, 1, 114, 0,
		3, 100, 0, 20, 101, 114, 108, 45, 100, 101, 109, 111, 50, 50, 64, 49,
		50, 55, 46, 48, 46, 48, 46, 49, 1, 0, 3, 219, 136, 225, 146, 0, 9, 253,
		168, 114, 208, 103, 100, 0, 20, 101, 114, 108, 45, 100, 101, 109, 111,
		50, 50, 64, 49, 50, 55, 46, 48, 46, 48, 46, 49, 0, 0, 0, 77, 0, 0, 0, 0, 1,
		104, 2, 100, 0, 5, 118, 97, 108, 117, 101, 112, 0, 0, 0, 110, 2, 196,
		150, 93, 173, 104, 167, 134, 253, 184, 200, 203, 147, 166, 63, 88,
		201, 0, 0, 0, 5, 0, 0, 0, 1, 100, 0, 5, 115, 104, 101, 108, 108, 97, 5, 98,
		6, 36, 178, 237, 103, 100, 0, 20, 101, 114, 108, 45, 100, 101, 109,
		111, 50, 50, 64, 49, 50, 55, 46, 48, 46, 48, 46, 49, 0, 0, 0, 83, 0, 0, 0,
		0, 1, 103, 100, 0, 20, 101, 114, 108, 45, 100, 101, 109, 111, 50, 50,
		64, 49, 50, 55, 46, 48, 46, 48, 46, 49, 0, 0, 0, 77, 0, 0, 0, 0, 1, 108, 0,
		0, 0, 1, 104, 5, 100, 0, 6, 99, 108, 97, 117, 115, 101, 97, 1, 108, 0, 0,
		0, 1, 104, 3, 100, 0, 3, 118, 97, 114, 97, 1, 100, 0, 1, 78, 106, 106,
		108, 0, 0, 0, 1, 104, 5, 100, 0, 2, 111, 112, 97, 1, 100, 0, 1, 42, 104, 3,
		100, 0, 3, 118, 97, 114, 97, 1, 100, 0, 1, 78, 104, 3, 100, 0, 7, 105,
		110, 116, 101, 103, 101, 114, 97, 1, 97, 2, 106, 106}

	packetFunction = packet // save for benchmark
	_, _, err := Decode(packet, []Atom{})
	if err != nil {
		t.Fatal(err)
	}

}

//
// benchmarks
//

func BenchmarkDecodeAtom(b *testing.B) {
	packet := []byte{ettAtomUTF8, 0, 3, 97, 98, 99}
	for i := 0; i < b.N; i++ {
		_, _, err := Decode(packet, []Atom{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeString(b *testing.B) {
	packet := []byte{ettString, 0, 3, 97, 98, 99}
	for i := 0; i < b.N; i++ {
		_, _, err := Decode(packet, []Atom{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeNewFloat(b *testing.B) {
	packet := []byte{ettNewFloat, 64, 0, 204, 204, 204, 204, 204, 205}
	for i := 0; i < b.N; i++ {
		_, _, err := Decode(packet, []Atom{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeInteger(b *testing.B) {
	packet := []byte{ettInteger, 182, 105, 253, 46}
	for i := 0; i < b.N; i++ {
		_, _, err := Decode(packet, []Atom{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeSmallBigInteger(b *testing.B) {
	packet := []byte{ettSmallBig, 8, 1, 177, 28, 108, 177, 244, 16, 34, 17}
	for i := 0; i < b.N; i++ {
		_, _, err := Decode(packet, []Atom{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeSmallBigIntegerWithinInt64Range(b *testing.B) {
	packet := []byte{ettSmallBig, 5, 1, 106, 26, 153, 190, 28}
	for i := 0; i < b.N; i++ {
		_, _, err := Decode(packet, []Atom{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeList100Integer(b *testing.B) {
	packet := []byte{}
	packetInt := []byte{ettInteger, 182, 105, 253, 46}
	packetList := []byte{ettList, 0, 0, 0, 100}

	packet = append(packet, packetList...)
	packet = append(packet, byte(106))

	for i := 0; i < 100; i++ {
		packet = append(packet, packetInt...)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _, err := Decode(packet, []Atom{})
		if err != nil {
			b.Fatal(err)
		}
	}

}

func BenchmarkDecodeTuple(b *testing.B) {
	packet := []byte{ettSmallTuple, 3, 70, 64, 9, 30, 184, 81, 235, 133, 31, 100, 0, 3, 97, 98, 99,
		98, 58, 222, 104, 177}
	for i := 0; i < b.N; i++ {
		_, _, err := Decode(packet, []Atom{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodePid(b *testing.B) {
	packet := []byte{103, 100, 0, 18, 101, 114, 108, 45, 100, 101, 109, 111, 64, 49,
		50, 55, 46, 48, 46, 48, 46, 49, 0, 0, 0, 142, 0, 0, 0, 0, 2}
	for i := 0; i < b.N; i++ {
		_, _, err := Decode(packet, []Atom{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodePidWithAtomCache(b *testing.B) {
	packet := []byte{103, ettCacheRef, 0, 0, 0, 0, 142, 0, 0, 0, 0, 2}
	cache := []Atom{Atom("erl-demo@127.0.0.1")}
	for i := 0; i < b.N; i++ {
		_, _, err := Decode(packet, cache)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeRef(b *testing.B) {
	packet := []byte{114, 0, 3, 100, 0, 18, 101, 114, 108, 45, 100, 101, 109, 111, 64,
		49, 50, 55, 46, 48, 46, 48, 46, 49, 2, 0, 1, 30, 228, 183, 192, 0, 1, 141,
		122, 203, 35}
	for i := 0; i < b.N; i++ {
		_, _, err := Decode(packet, []Atom{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeRefWithAtomCache(b *testing.B) {
	packet := []byte{114, 0, 3, ettCacheRef, 0, 2, 0, 1, 30, 228, 183, 192, 0, 1, 141,
		122, 203, 35}
	cache := []Atom{Atom("erl-demo@127.0.0.1")}
	for i := 0; i < b.N; i++ {
		_, _, err := Decode(packet, cache)
		if err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkDecodePort(b *testing.B) {
	packet := []byte{102, 100, 0, 18, 101, 114, 108, 45, 100, 101, 109, 111, 64, 49,
		50, 55, 46, 48, 46, 48, 46, 49, 0, 0, 0, 32, 2}
	for i := 0; i < b.N; i++ {
		_, _, err := Decode(packet, []Atom{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeTupleRefPid(b *testing.B) {

	packet := []byte{ettSmallTuple, 2, ettNewRef, 0, 3, ettAtom, 0, 18, 101, 114, 108, 45, 100, 101, 109,
		111, 64, 49, 50, 55, 46, 48, 46, 48, 46, 49, 2, 0, 1, 31, 28, 183, 192, 0,
		1, 141, 122, 203, 35, 103, 100, 0, 18, 101, 114, 108, 45, 100, 101,
		109, 111, 64, 49, 50, 55, 46, 48, 46, 48, 46, 49, 0, 0, 0, 142, 0, 0, 0, 0,
		2}
	for i := 0; i < b.N; i++ {
		_, _, err := Decode(packet, []Atom{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeTupleRefPidWithAtomCache(b *testing.B) {

	packet := []byte{ettSmallTuple, 2, ettNewRef, 0, 3, ettCacheRef, 0,
		2, 0, 1, 31, 28, 183, 192, 0, 1, 141, 122, 203, 35, 103, ettCacheRef, 0,
		0, 0, 0, 142, 0, 0, 0, 0, 2}
	cache := []Atom{Atom("erl-demo@127.0.0.1")}
	for i := 0; i < b.N; i++ {
		_, _, err := Decode(packet, cache)
		if err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkDecodeFunction(b *testing.B) {
	packet := packetFunction
	for i := 0; i < b.N; i++ {
		_, _, err := Decode(packet, []Atom{})
		if err != nil {
			b.Fatal(err)
		}
	}
}
