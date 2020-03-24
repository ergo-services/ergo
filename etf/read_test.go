package etf

import (
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
