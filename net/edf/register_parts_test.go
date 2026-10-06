package edf

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

type testPartsInner struct {
	X int64
}

type testPartsItem struct {
	Name string
}

type testPartsOuter struct {
	Inner  testPartsInner
	Items  []testPartsItem
	ByName map[gen.Atom]*testPartsInner
}

type testPartsTree struct {
	Kids []testPartsTree
}

type testPartsCode int16

type testPartsCodes []testPartsCode

type testPartsCounts map[testPartsCode]testPartsCodes

type testPartsNamed struct {
	Counts testPartsCounts
	Pair   [2]testPartsCode
}

func TestRegisterPartsRegistersFieldTypes(t *testing.T) {
	if err := RegisterTypeOf(testPartsOuter{}); err != nil {
		t.Fatalf("register: %s", err)
	}
	for _, v := range []any{testPartsInner{}, testPartsItem{}} {
		if _, found := LookupType(regTypeName(reflect.TypeOf(v))); found == false {
			t.Errorf("%T is not registered with the type that has it in a field", v)
		}
	}

	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)
	if err := Encode(testPartsInner{X: 1}, b, Options{}); err != nil {
		t.Fatalf("encode a field type: %s", err)
	}
	v, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatalf("decode a field type: %s", err)
	}
	if v.(testPartsInner).X != 1 {
		t.Errorf("decoded %#v", v)
	}
}

func TestRegisterIsIdempotent(t *testing.T) {
	for i := 0; i < 2; i++ {
		if err := RegisterTypeOf(testPartsOuter{}); err != nil {
			t.Fatalf("register a type again: %s", err)
		}
		if err := RegisterTypeOf(testPartsItem{}); err != nil {
			t.Fatalf("register a field type again: %s", err)
		}
	}
	marker := errors.New("parts marker")
	for i := 0; i < 2; i++ {
		if err := RegisterError(marker); err != nil {
			t.Fatalf("register an error again: %s", err)
		}
		if err := RegisterAtom("parts.atom"); err != nil {
			t.Fatalf("register an atom again: %s", err)
		}
	}
}

func testPartsFirst() any {
	type testPartsConflict struct {
		A int
	}
	return testPartsConflict{}
}

func testPartsHolder() any {
	type testPartsConflict struct {
		B string
	}
	type testPartsHolder struct {
		Second testPartsConflict
	}
	return testPartsHolder{}
}

func TestRegisterConflictNamesThePath(t *testing.T) {
	if err := RegisterTypeOf(testPartsFirst()); err != nil {
		t.Fatalf("register: %s", err)
	}
	holder := testPartsHolder()
	err := RegisterTypeOf(holder)
	if err == nil {
		t.Fatal("another type with the name of a registered one must be refused")
	}
	want := regTypeName(reflect.TypeOf(holder)) + ": Second: " +
		regTypeName(reflect.TypeOf(testPartsFirst())) + ": " + gen.ErrTaken.Error()
	if err.Error() != want {
		t.Errorf("got %q, want %q", err, want)
	}
	if _, found := LookupType(regTypeName(reflect.TypeOf(holder))); found {
		t.Error("the type with the conflict in a field is registered")
	}
}

func TestRegisterSelfReferenceIsRefused(t *testing.T) {
	err := RegisterTypeOf(testPartsTree{})
	if err == nil || strings.Contains(err.Error(), "must be registered first") == false {
		t.Fatalf("got %v", err)
	}
}

func TestRegisterPartsRegistersNamedScalars(t *testing.T) {
	if err := RegisterTypeOf(testPartsNamed{}); err != nil {
		t.Fatalf("register: %s", err)
	}
	if _, found := LookupType(regTypeName(reflect.TypeOf(testPartsCode(0)))); found == false {
		t.Error("a named scalar is not registered with the type that has it in a field")
	}
	for _, v := range []any{testPartsCodes{}, testPartsCounts{}} {
		if _, found := LookupType(regTypeName(reflect.TypeOf(v))); found {
			t.Errorf("%T is registered: a named slice or map goes as its underlying type", v)
		}
	}
}
