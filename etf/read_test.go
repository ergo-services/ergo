package etf

import (
	"fmt"
	"math/big"
	"reflect"
	"testing"
)

func TestReadAtom(t *testing.T) {
	expected := Atom("abc")
	packet := []byte{ettAtomUTF8, 0, 3, 97, 98, 99}
	term, err := Decode(packet, []Atom{})
	if err != nil || term != expected {
		t.Fatal(err)
	}

	packet = []byte{ettSmallAtomUTF8, 3, 97, 98, 99}
	term, err = Decode(packet, []Atom{})
	if err != nil || term != expected {
		t.Fatal(err)
	}

	packet = []byte{ettSmallAtomUTF8, 4, 97, 98, 99}
	term, err = Decode(packet, []Atom{})
	if err != ErrMalformedSmallAtomUTF8 {
		t.Fatal(err)
	}

	packet = []byte{119}
	term, err = Decode(packet, []Atom{})
	if err != ErrMalformedSmallAtomUTF8 {
		t.Fatal(err)
	}

	packet = []byte{ettSmallAtomUTF8, 3, 97, 98, 99, 0, 0}
	term, err = Decode(packet, []Atom{})
	if err != ErrMalformedPacketLength {
		t.Fatal(err)
	}
}

func TestReadString(t *testing.T) {
	expected := "abc"
	packet := []byte{ettString, 0, 3, 97, 98, 99}
	term, err := Decode(packet, []Atom{})
	if err != nil || term != expected {
		t.Fatal(err)
	}

	packet = []byte{ettString, 3, 97, 98, 99}
	term, err = Decode(packet, []Atom{})
	if err != ErrMalformedString {
		t.Fatal(err)
	}

	packet = []byte{ettString, 0, 3, 97, 98, 99, 0, 0}
	term, err = Decode(packet, []Atom{})
	if err != ErrMalformedPacketLength {
		t.Fatal(err)
	}

}

func TestReadNewFloat(t *testing.T) {
	expected := float64(2.1)
	packet := []byte{ettNewFloat, 64, 0, 204, 204, 204, 204, 204, 205}
	term, err := Decode(packet, []Atom{})
	if err != nil || term != expected {
		t.Fatal(err)
	}

	packet = []byte{ettNewFloat, 64, 0, 204, 204, 204, 204, 204}
	term, err = Decode(packet, []Atom{})
	if err != ErrMalformedNewFloat {
		t.Fatal(err)
	}

	packet = []byte{ettNewFloat, 64, 0, 204, 204, 204, 204, 204, 205, 0, 0}
	term, err = Decode(packet, []Atom{})
	if err != ErrMalformedPacketLength {
		t.Fatal(err)
	}
}

func TestReadInteger(t *testing.T) {
	expected := int(88)
	packet := []byte{ettSmallInteger, 88}

	term, err := Decode(packet, []Atom{})
	if err != nil || term != expected {
		t.Fatal(err)
	}

	packet = []byte{ettSmallInteger}
	term, err = Decode(packet, []Atom{})
	if err != ErrMalformedSmallInteger {
		t.Fatal(err)
	}

	expectedInteger := int64(-1234567890)
	packet = []byte{ettInteger, 182, 105, 253, 46}
	term, err = Decode(packet, []Atom{})
	if err != nil || term != expectedInteger {
		t.Fatal(err, expectedInteger, term)
	}

	packet = []byte{ettInteger, 182, 105, 253}
	term, err = Decode(packet, []Atom{})
	if err != ErrMalformedInteger {
		t.Fatal(err)
	}

	//-1234567890987654321
	bigInt := new(big.Int)
	bigInt.SetString("-1234567890987654321", 10)
	packet = []byte{ettSmallBig, 8, 1, 177, 28, 108, 177, 244, 16, 34, 17}
	term, err = Decode(packet, []Atom{})
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

	term, err = Decode(packet, []Atom{})
	if err != nil || bigInt.Cmp(term.(*big.Int)) != 0 {
		t.Fatal(err, term, bigInt)
	}

	//-123456789098 should be treated as int64
	expectedInt64 := int64(-123456789098)
	packet = []byte{ettSmallBig, 5, 1, 106, 26, 153, 190, 28}
	term, err = Decode(packet, []Atom{})
	if err != nil || term != expectedInt64 {
		t.Fatal(err, term, expectedInt64)
	}
}

func TestReadList(t *testing.T) {
	expected := List{3.14, Atom("abc"), int64(987654321)}
	packet := []byte{ettList, 0, 0, 0, 3, 70, 64, 9, 30, 184, 81, 235, 133, 31, 100, 0, 3, 97,
		98, 99, 98, 58, 222, 104, 177, 106}
	term, err := Decode(packet, []Atom{})
	if err != nil {
		t.Fatal(err)
	}
	result := term.(List)
	if !reflect.DeepEqual(expected, result) {
		t.Fatal("result != expected")
	}

}

func TestReadTuple(t *testing.T) {
	expected := Tuple{3.14, Atom("abc"), int64(987654321)}
	packet := []byte{ettSmallTuple, 3, 70, 64, 9, 30, 184, 81, 235, 133, 31, 100, 0, 3, 97, 98, 99,
		98, 58, 222, 104, 177}

	term, err := Decode(packet, []Atom{})
	if err != nil {
		t.Fatal(err)
	}
	result := term.(Tuple)
	if !reflect.DeepEqual(expected, result) {
		t.Fatal("result != expected")
	}
}

func TestReadMap(t *testing.T) {
	expected := Map{
		Atom("abc"): 123,
		"abc":       4.56,
	}
	packet := []byte{116, 0, 0, 0, 2, 100, 0, 3, 97, 98, 99, 97, 123, 107, 0, 3, 97, 98,
		99, 70, 64, 18, 61, 112, 163, 215, 10, 61}

	term, err := Decode(packet, []Atom{})
	if err != nil {
		t.Fatal(err)
	}

	result := term.(Map)
	if !reflect.DeepEqual(expected, result) {
		t.Fatal("result != expected")
	}

}

func TestReadBinary(t *testing.T) {
	expected := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 0}
	packet := []byte{ettBinary, 0, 0, 0, 10, 1, 2, 3, 4, 5, 6, 7, 8, 9, 0}

	term, err := Decode(packet, []Atom{})
	if err != nil {
		t.Fatal(err)
	}

	result := term.([]byte)
	if !reflect.DeepEqual(expected, result) {
		t.Fatal("result != expected")
	}
}

func TestReadBitBinary(t *testing.T) {
	expected := []byte{1, 2, 3, 4, 5}
	packet := []byte{77, 0, 0, 0, 5, 3, 1, 2, 3, 4, 160}

	term, err := Decode(packet, []Atom{})
	if err != nil {
		t.Fatal(err)
	}

	result := term.([]byte)
	if !reflect.DeepEqual(expected, result) {
		t.Fatal("result != expected")
	}

}

func TestReadPid(t *testing.T) {
	expected := Pid{
		Node:     Atom("erl-demo@127.0.0.1"),
		Id:       142,
		Serial:   0,
		Creation: 2,
	}
	packet := []byte{103, 100, 0, 18, 101, 114, 108, 45, 100, 101, 109, 111, 64, 49,
		50, 55, 46, 48, 46, 48, 46, 49, 0, 0, 0, 142, 0, 0, 0, 0, 2}
	term, err := Decode(packet, []Atom{})
	if err != nil {
		t.Fatal(err)
	}

	result := term.(Pid)
	if !reflect.DeepEqual(expected, result) {
		t.Fatal("result != expected")
	}
}

func TestReadRef(t *testing.T) {
	expected := Ref{
		Node:     Atom("erl-demo@127.0.0.1"),
		Creation: 2,
		Id:       []uint32{73444, 3082813441, 2373634851},
	}
	packet := []byte{114, 0, 3, 100, 0, 18, 101, 114, 108, 45, 100, 101, 109, 111, 64,
		49, 50, 55, 46, 48, 46, 48, 46, 49, 2, 0, 1, 30, 228, 183, 192, 0, 1, 141,
		122, 203, 35}
	term, err := Decode(packet, []Atom{})
	if err != nil {
		t.Fatal(err)
	}

	result := term.(Ref)
	if !reflect.DeepEqual(expected, result) {
		t.Fatal("result != expected")
	}
}

func TestReadTupleRefPid(t *testing.T) {
	packet := []byte{ettSmallTuple, 2, ettNewRef, 0, 3, ettAtom, 0, 18, 101, 114, 108, 45, 100, 101, 109,
		111, 64, 49, 50, 55, 46, 48, 46, 48, 46, 49, 2, 0, 1, 31, 28, 183, 192, 0,
		1, 141, 122, 203, 35, 103, 100, 0, 18, 101, 114, 108, 45, 100, 101,
		109, 111, 64, 49, 50, 55, 46, 48, 46, 48, 46, 49, 0, 0, 0, 142, 0, 0, 0, 0,
		2}
	term, err := Decode(packet, []Atom{})
	if err != nil {
		t.Fatal(err)
	}

	fmt.Printf("term = %+v\n", term)
}

func TestReadPort(t *testing.T) {
	expected := Port{
		Node:     Atom("erl-demo@127.0.0.1"),
		Creation: 2,
		Id:       32,
	}
	packet := []byte{102, 100, 0, 18, 101, 114, 108, 45, 100, 101, 109, 111, 64, 49,
		50, 55, 46, 48, 46, 48, 46, 49, 0, 0, 0, 32, 2}

	term, err := Decode(packet, []Atom{})
	if err != nil {
		t.Fatal(err)
	}

	result := term.(Port)
	if !reflect.DeepEqual(expected, result) {
		t.Fatal("result != expected")
	}
}

//
// benchmarks
//

func BenchmarkReadAtom(b *testing.B) {
	packet := []byte{ettAtomUTF8, 0, 3, 97, 98, 99}
	for i := 0; i < b.N; i++ {
		_, err := Decode(packet, []Atom{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReadString(b *testing.B) {
	packet := []byte{ettString, 0, 3, 97, 98, 99}
	for i := 0; i < b.N; i++ {
		_, err := Decode(packet, []Atom{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReadNewFloat(b *testing.B) {
	packet := []byte{ettNewFloat, 64, 0, 204, 204, 204, 204, 204, 205}
	for i := 0; i < b.N; i++ {
		_, err := Decode(packet, []Atom{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReadInteger(b *testing.B) {
	packet := []byte{ettInteger, 182, 105, 253, 46}
	for i := 0; i < b.N; i++ {
		_, err := Decode(packet, []Atom{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReadSmallBigInteger(b *testing.B) {
	packet := []byte{ettSmallBig, 8, 1, 177, 28, 108, 177, 244, 16, 34, 17}
	for i := 0; i < b.N; i++ {
		_, err := Decode(packet, []Atom{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReadSmallBigIntegerWithinInt64Range(b *testing.B) {
	packet := []byte{ettSmallBig, 5, 1, 106, 26, 153, 190, 28}
	for i := 0; i < b.N; i++ {
		_, err := Decode(packet, []Atom{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReadList100Integer(b *testing.B) {
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
		_, err := Decode(packet, []Atom{})
		if err != nil {
			b.Fatal(err)
		}
	}

}

func BenchmarkReadTuple(b *testing.B) {
	packet := []byte{ettSmallTuple, 3, 70, 64, 9, 30, 184, 81, 235, 133, 31, 100, 0, 3, 97, 98, 99,
		98, 58, 222, 104, 177}
	for i := 0; i < b.N; i++ {
		_, err := Decode(packet, []Atom{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReadPid(b *testing.B) {
	packet := []byte{103, 100, 0, 18, 101, 114, 108, 45, 100, 101, 109, 111, 64, 49,
		50, 55, 46, 48, 46, 48, 46, 49, 0, 0, 0, 142, 0, 0, 0, 0, 2}
	for i := 0; i < b.N; i++ {
		_, err := Decode(packet, []Atom{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReadRef(b *testing.B) {
	packet := []byte{114, 0, 3, 100, 0, 18, 101, 114, 108, 45, 100, 101, 109, 111, 64,
		49, 50, 55, 46, 48, 46, 48, 46, 49, 2, 0, 1, 30, 228, 183, 192, 0, 1, 141,
		122, 203, 35}
	for i := 0; i < b.N; i++ {
		_, err := Decode(packet, []Atom{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReadPort(b *testing.B) {
	packet := []byte{102, 100, 0, 18, 101, 114, 108, 45, 100, 101, 109, 111, 64, 49,
		50, 55, 46, 48, 46, 48, 46, 49, 0, 0, 0, 32, 2}
	for i := 0; i < b.N; i++ {
		_, err := Decode(packet, []Atom{})
		if err != nil {
			b.Fatal(err)
		}
	}
}
