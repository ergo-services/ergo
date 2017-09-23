package etf

import (
	"bytes"
	"math/big"
	"testing"
)

func TestReadAtom(t *testing.T) {
	c := new(Context)

	// 'abc'
	in := bytes.NewBuffer([]byte{100, 0, 3, 97, 98, 99})
	if v, err := c.Read(in); err != nil {
		t.Fatal(err)
	} else if l := in.Len(); l != 0 {
		t.Errorf("buffer len %d", l)
	} else if exp := Atom("abc"); exp != v {
		t.Errorf("expected %v, got %v", exp, v)
	}

	// ''
	in = bytes.NewBuffer([]byte{100, 0, 0})
	if v, err := c.Read(in); err != nil {
		t.Fatal(err)
	} else if l := in.Len(); l != 0 {
		t.Errorf("buffer len %d", l)
	} else if exp := Atom(""); exp != v {
		t.Errorf("expected %v, got %v", exp, v)
	}

	// 'abc' as SmallAtom
	in = bytes.NewBuffer([]byte{115, 3, 97, 98, 99})
	if v, err := c.Read(in); err != nil {
		t.Fatal(err)
	} else if l := in.Len(); l != 0 {
		t.Errorf("buffer len %d", l)
	} else if exp := Atom("abc"); exp != v {
		t.Errorf("expected %v, got %v", exp, v)
	}

	// '' as SmallAtom
	in = bytes.NewBuffer([]byte{115, 0})
	if v, err := c.Read(in); err != nil {
		t.Fatal(err)
	} else if l := in.Len(); l != 0 {
		t.Errorf("buffer len %d", l)
	} else if exp := Atom(""); exp != v {
		t.Errorf("expected %v, got %v", exp, v)
	}

	// error (ends abruptly)
	if _, err := c.Read(bytes.NewBuffer([]byte{100, 0, 4, 97, 98, 99})); err == nil {
		t.Error("err == nil")
	}

	// error (bad length)
	if _, err := c.Read(bytes.NewBuffer([]byte{100})); err == nil {
		t.Error("err == nil")
	}
}

func TestReadBinary(t *testing.T) {
	c := new(Context)

	// <<1,2,3,4,5>>
	in := bytes.NewBuffer([]byte{109, 0, 0, 0, 5, 1, 2, 3, 4, 5})
	if v, err := c.Read(in); err != nil {
		t.Error(err)
	} else if l := in.Len(); l != 0 {
		t.Errorf("buffer len %d", l)
	} else if exp := []byte{1, 2, 3, 4, 5}; bytes.Compare(exp, v.([]byte)) != 0 {
		t.Errorf("expected %v, got %v", exp, v)
	}
}

func TestReadBitBinary(t *testing.T) {
	c := new(Context)

	// <<1,2,3,4,5:3>>
	in := bytes.NewBuffer([]byte{77, 0, 0, 0, 5, 3, 1, 2, 3, 4, 160})
	if v, err := c.Read(in); err != nil {
		t.Error(err)
	} else if l := in.Len(); l != 0 {
		t.Errorf("buffer len %d", l)
	} else if exp := []byte{1, 2, 3, 4, 5}; bytes.Compare(exp, v.([]byte)) != 0 {
		t.Errorf("expected %v, got %v", exp, v)
	}
}

// func TestReadBool(t *testing.T) {
// 	c := new(Context)

// 	// true
// 	in := bytes.NewBuffer([]byte{100, 0, 4, 't', 'r', 'u', 'e'})
// 	if v, err := c.Read(in); err != nil {
// 		t.Error(err)
// 	} else if l := in.Len(); l != 0 {
// 		t.Errorf("buffer len %d", l)
// 	} else if exp := true; exp != v {
// 		t.Errorf("expected %v, got %v", exp, v)
// 	}

// 	// false
// 	in = bytes.NewBuffer([]byte{100, 0, 5, 'f', 'a', 'l', 's', 'e'})
// 	if v, err := c.Read(in); err != nil {
// 		t.Error(err)
// 	} else if l := in.Len(); l != 0 {
// 		t.Errorf("buffer len %d", l)
// 	} else if exp := false; exp != v {
// 		t.Errorf("expected %v, got %v", exp, v)
// 	}

// 	// error
// 	in = bytes.NewBuffer([]byte{100, 0, 3, 97, 98, 99})
// 	if v, err := c.Read(in); err != nil {
// 		t.Error(err)
// 	} else if l := in.Len(); l != 0 {
// 		t.Errorf("buffer len %d", l)
// 	} else if exp := Atom("abc"); exp != v {
// 		t.Errorf("expected %v, got %v", exp, v)
// 	}
// }

func TestReadFloat(t *testing.T) {
	c := new(Context)

	// 0.1
	in := bytes.NewBuffer([]byte{
		99, 49, 46, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48,
		48, 48, 48, 53, 53, 53, 49, 101, 45, 48, 49, 0, 0, 0, 0, 0,
	})
	if v, err := c.Read(in); err != nil {
		t.Error(err)
	} else if l := in.Len(); l != 0 {
		t.Errorf("buffer len %d", l)
	} else if exp := 0.1; exp != v {
		t.Errorf("expected %v, got %v", exp, v)
	}

	// 0.1
	in = bytes.NewBuffer([]byte{70, 63, 185, 153, 153, 153, 153, 153, 154})
	if v, err := c.Read(in); err != nil {
		t.Error(err)
	} else if l := in.Len(); l != 0 {
		t.Errorf("buffer len %d", l)
	} else if exp := 0.1; exp != v {
		t.Errorf("expected %v, got %v", exp, v)
	}

	// error (31 bytes instead of 32)
	in = bytes.NewBuffer([]byte{
		99, 49, 46, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48,
		48, 48, 48, 53, 53, 53, 49, 101, 45, 48, 49, 0, 0, 0, 0,
	})
	if _, err := c.Read(in); err == nil {
		t.Error("err == nil")
	}

	// error (fail on Sscanf)
	in = bytes.NewBuffer([]byte{
		99, 99, 46, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48, 48,
		48, 48, 48, 53, 53, 53, 49, 101, 45, 48, 49, 0, 0, 0, 0, 0,
	})
	if _, err := c.Read(in); err == nil {
		t.Error("err == nil")
	}

	// error (bad length)
	if _, err := c.Read(bytes.NewBuffer([]byte{99})); err == nil {
		t.Error("err == nil")
	}

	// error (bad length)
	if _, err := c.Read(bytes.NewBuffer([]byte{70})); err == nil {
		t.Error("err == nil")
	}
}

func TestReadInt(t *testing.T) {
	c := new(Context)

	// 255
	in := bytes.NewBuffer([]byte{97, 255})
	if v, err := c.Read(in); err != nil {
		t.Error(err)
	} else if l := in.Len(); l != 0 {
		t.Errorf("buffer len %d", l)
	} else if exp := 255; exp != v {
		t.Errorf("expected %v, got %v", exp, v)
	}

	// 0x7fffffff
	in = bytes.NewBuffer([]byte{98, 127, 255, 255, 255})
	if v, err := c.Read(in); err != nil {
		t.Error(err)
	} else if l := in.Len(); l != 0 {
		t.Errorf("buffer len %d", l)
	} else if exp := 0x7fffffff; exp != v {
		t.Errorf("expected %v, got %v", exp, v)
	}

	// -0x80000000
	in = bytes.NewBuffer([]byte{98, 128, 0, 0, 0})
	if v, err := c.Read(in); err != nil {
		t.Error(err)
	} else if l := in.Len(); l != 0 {
		t.Errorf("buffer len %d", l)
	} else if exp := -0x80000000; exp != v {
		t.Errorf("expected %v, got %v", exp, v)
	}

	// 0x7fffffffffffffff
	in = bytes.NewBuffer([]byte{110, 8, 0, 255, 255, 255, 255, 255, 255, 255, 127})
	if v, err := c.Read(in); err != nil {
		t.Error(err)
	} else if l := in.Len(); l != 0 {
		t.Errorf("buffer len %d", l)
	} else {
		var v64 int64
		switch v := v.(type) {
		case int:
			v64 = int64(v)
		case int64:
			v64 = v
		}
		if exp := int64(0x7fffffffffffffff); exp != v64 {
			t.Errorf("expected %v, got %v", exp, v64)
		}
	}

	// -0x8000000000000000
	in = bytes.NewBuffer([]byte{110, 8, 1, 0, 0, 0, 0, 0, 0, 0, 128})
	if v, err := c.Read(in); err != nil {
		t.Error(err)
	} else if l := in.Len(); l != 0 {
		t.Errorf("buffer len %d", l)
	} else {
		var v64 int64
		switch v := v.(type) {
		case int:
			v64 = int64(v)
		case int64:
			v64 = v
		}
		if exp := int64(-0x8000000000000000); exp != v64 {
			t.Errorf("expected %v, got %v", exp, v64)
		}
	}

	// error (bad length)
	for _, b := range []byte{97, 98, 110, 111} {
		if _, err := c.Read(bytes.NewBuffer([]byte{b})); err == nil {
			t.Error("err == nil (%d)", b)
		}
	}

	// (1<<2040)
	in = bytes.NewBuffer([]byte{
		111, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1,
	})
	if v, err := c.Read(in); err != nil {
		t.Error(err)
	} else if l := in.Len(); l != 0 {
		t.Errorf("buffer len %d", l)
	} else if v, exp := new(big.Int).Lsh(big.NewInt(1), 2040).Cmp(v.(*big.Int)), 0; v != exp {
		t.Errorf("expected %v, got %v", exp, v)
	}

	// -(1<<2040)
	in = bytes.NewBuffer([]byte{
		111, 0, 0, 1, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1,
	})
	exp := new(big.Int).Neg(new(big.Int).Lsh(big.NewInt(1), 2040))
	if v, err := c.Read(in); err != nil {
		t.Error(err)
	} else if l := in.Len(); l != 0 {
		t.Errorf("buffer len %d", l)
	} else if exp.Cmp(v.(*big.Int)) != 0 {
		t.Errorf("expected %v, got %v", exp, v)
	}

	// 0 (small big)
	in = bytes.NewBuffer([]byte{110, 0, 0})
	if v, err := c.Read(in); err != nil {
		t.Error(err)
	} else if l := in.Len(); l != 0 {
		t.Errorf("buffer len %d", l)
	} else if exp := int64(0); v != exp {
		t.Errorf("expected %v, got %v", exp, v)
	}

	// 0 (large big)
	in = bytes.NewBuffer([]byte{111, 0, 0, 0, 0, 0})
	if v, err := c.Read(in); err != nil {
		t.Error(err)
	} else if l := in.Len(); l != 0 {
		t.Errorf("buffer len %d", l)
	} else if exp := int(0); v != exp {
		t.Errorf("expected %v, got %v", exp, v)
	}
}

func TestReadPid(t *testing.T) {
	c := new(Context)

	// lol@localhost
	in := bytes.NewBuffer([]byte{
		103, 100, 0, 13, 108, 111,
		108, 64, 108, 111, 99, 97,
		108, 104, 111, 115, 116, 0,
		0, 0, 38, 0, 0, 0, 0, 3,
	})
	exp := Pid{Atom("lol@localhost"), 38, 0, 3}
	if v, err := c.Read(in); err != nil {
		t.Error(err)
	} else if l := in.Len(); l != 0 {
		t.Errorf("buffer len %d", l)
	} else if v != exp {
		t.Errorf("expected %v, got %v", exp, v)
	}

	// nonode@nohost
	in = bytes.NewBuffer([]byte{
		103, 100, 0, 13, 110, 111,
		110, 111, 100, 101, 64, 110,
		111, 104, 111, 115, 116, 0,
		0, 0, 32, 0, 0, 0, 0, 0,
	})
	exp = Pid{Atom("nonode@nohost"), 32, 0, 0}
	if v, err := c.Read(in); err != nil {
		t.Error(err)
	} else if l := in.Len(); l != 0 {
		t.Errorf("buffer len %d", l)
	} else if v != exp {
		t.Errorf("expected %v, got %v", exp, v)
	}
}

func TestReadString(t *testing.T) {
	c := new(Context)

	// "" (empty string)
	in := bytes.NewBuffer([]byte{107, 0, 0})
	if v, err := c.Read(in); err != nil {
		t.Error(err)
	} else if l := in.Len(); l != 0 {
		t.Errorf("buffer len %d", l)
	} else if exp := ""; v != exp {
		t.Errorf("expected %v, got %v", exp, v)
	}

	// "abc"
	in = bytes.NewBuffer([]byte{107, 0, 3, 97, 98, 99})
	if v, err := c.Read(in); err != nil {
		t.Error(err)
	} else if l := in.Len(); l != 0 {
		t.Errorf("buffer len %d", l)
	} else if exp := "abc"; v != exp {
		t.Errorf("expected %v, got %v", exp, v)
	}

	// error (wrong length) in string
	in = bytes.NewBuffer([]byte{107, 0, 3, 97, 98})
	if _, err := c.Read(in); err == nil {
		t.Error("err == nil")
	}

	// error (wrong length) in binary string
	in = bytes.NewBuffer([]byte{109, 0, 0, 0, 3, 97, 98})
	if _, err := c.Read(in); err == nil {
		t.Error("err == nil")
	}

	// error (improper list) [$a,$b,$c|0]
	in = bytes.NewBuffer([]byte{108, 0, 0, 0, 3, 97, 98, 99, 0})
	if _, err := c.Read(in); err == nil {
		t.Error("err == nil")
	}

	// error (bad length)
	for _, b := range []byte{107, 108, 109} {
		if _, err := c.Read(bytes.NewBuffer([]byte{b})); err == nil {
			t.Error("err == nil (%d)", b)
		}
	}
}

func TestReadTerm(t *testing.T) {
	c := new(Context)

	in := bytes.NewBuffer([]byte{
		104, 2,
		104, 3,
		100, 0, 4, 98, 108, 97,
		104, 97, 4,
		108, 0, 0, 0, 4,
		98, 0, 0, 4, 68,
		98, 0, 0, 4, 75,
		98, 0, 0, 4, 50,
		98, 0, 0, 4, 48,
		106,
		98, 0, 0, 2, 154,
	})
	_, err := c.Read(in)
	if err != nil {
		t.Fatal(err)
	}
	if l := in.Len(); l != 0 {
		t.Errorf("buffer len %d", l)
	}
}
