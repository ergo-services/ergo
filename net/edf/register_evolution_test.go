package edf

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"sync"
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

// evoV1 and evoV2 model one logical type in two versions: V2 appends field C.
// Their fully-qualified names are the same length, so a test can retarget an
// encoded blob from one decoder to the other (the global registry cannot hold
// the same name with two layouts).
type evoV1 struct {
	A int64
	B string
}

type evoV2 struct {
	A int64
	B string
	C int64
}

// innerV1/innerV2 and wrapV1/wrapV2 model a nested struct that evolves: the
// inner type appends a field while the wrapper keeps the same field count. Only
// the wrapper name rides the wire (the inner is a positional field with no name
// of its own), so the inner drift is tolerated purely by its own length prefix.
type innerV1 struct{ P int64 }
type innerV2 struct {
	P int64
	Q int64
}
type wrapV1 struct {
	Pre  int64
	In   innerV1
	Post int64
}
type wrapV2 struct {
	Pre  int64
	In   innerV2
	Post int64
}

func registerEvo(t *testing.T, values ...any) {
	t.Helper()
	for _, v := range values {
		if err := RegisterTypeOf(v); err != nil && err != gen.ErrTaken {
			t.Fatal(err)
		}
	}
}

func TestSchemaEvolution(t *testing.T) {
	registerEvo(t, evoV1{}, evoV2{})
	opts := Options{SchemaEvolution: true}
	nameV1 := []byte(regTypeName(reflect.TypeOf(evoV1{})))
	nameV2 := []byte(regTypeName(reflect.TypeOf(evoV2{})))

	// forward: newer writer (V2, 3 fields) -> older reader (V1, 2 fields): C is skipped.
	b := lib.TakeBuffer()
	if err := Encode(evoV2{A: 7, B: "hi", C: 99}, b, opts); err != nil {
		t.Fatal(err)
	}
	v, tail, err := Decode(bytes.Replace(b.B, nameV2, nameV1, 1), opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(tail) != 0 {
		t.Fatalf("forward: unexpected tail %d", len(tail))
	}
	if got, ok := v.(evoV1); ok == false || got.A != 7 || got.B != "hi" {
		t.Fatalf("forward: got %#v", v)
	}
	lib.ReleaseBuffer(b)

	// backward: older writer (V1, 2 fields) -> newer reader (V2, 3 fields): C is zero-valued.
	b = lib.TakeBuffer()
	if err := Encode(evoV1{A: 5, B: "yo"}, b, opts); err != nil {
		t.Fatal(err)
	}
	v, tail, err = Decode(bytes.Replace(b.B, nameV1, nameV2, 1), opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(tail) != 0 {
		t.Fatalf("backward: unexpected tail %d", len(tail))
	}
	if got, ok := v.(evoV2); ok == false || got.A != 5 || got.B != "yo" || got.C != 0 {
		t.Fatalf("backward: got %#v", v)
	}
	lib.ReleaseBuffer(b)

	// strict mode adds no length prefix: evolution differs by exactly the 4-byte body length.
	bStrict := lib.TakeBuffer()
	if err := Encode(evoV2{A: 1, B: "x", C: 2}, bStrict, Options{}); err != nil {
		t.Fatal(err)
	}
	bEvo := lib.TakeBuffer()
	if err := Encode(evoV2{A: 1, B: "x", C: 2}, bEvo, opts); err != nil {
		t.Fatal(err)
	}
	if bEvo.Len() != bStrict.Len()+4 {
		t.Fatalf("evolution length prefix: strict=%d evo=%d (want +4)", bStrict.Len(), bEvo.Len())
	}
	lib.ReleaseBuffer(bStrict)
	lib.ReleaseBuffer(bEvo)
}

// TestSchemaEvolutionRegCache exercises evolution over the negotiated reg cache,
// where the struct's type tag on the wire is a 2-byte cache id, not the name.
// The drift is simulated by swapping that id (bytes [1:3] of the root tag).
func TestSchemaEvolutionRegCache(t *testing.T) {
	registerEvo(t, evoV1{}, evoV2{})

	rcDec := new(sync.Map)
	var names []string
	id := map[string]uint16{}
	for k, name := range GetRegCache() {
		names = append(names, name)
		rcDec.Store(k, name)
		id[name] = k
	}
	rcEnc := MakeEncodeRegTypeCache(names)
	if rcEnc == nil {
		t.Fatal("empty reg cache")
	}
	v1id := id[regTypeName(reflect.TypeOf(evoV1{}))]
	v2id := id[regTypeName(reflect.TypeOf(evoV2{}))]

	encOpts := Options{SchemaEvolution: true, RegCache: rcEnc, Cache: new(sync.Map)}
	decOpts := Options{SchemaEvolution: true, RegCache: rcDec, Cache: new(sync.Map)}

	// forward: encode V2 (cache id), retarget the id to V1, decode as V1: C skipped.
	b := lib.TakeBuffer()
	if err := Encode(evoV2{A: 7, B: "hi", C: 99}, b, encOpts); err != nil {
		t.Fatal(err)
	}
	binary.BigEndian.PutUint16(b.B[1:3], v1id) // b.B[0]==edtReg, [1:3]==cache id
	v, _, err := Decode(b.B, decOpts)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := v.(evoV1); ok == false || got.A != 7 || got.B != "hi" {
		t.Fatalf("regcache forward: got %#v", v)
	}
	lib.ReleaseBuffer(b)

	// backward: encode V1, retarget the id to V2, decode as V2: C zero-valued.
	b = lib.TakeBuffer()
	if err := Encode(evoV1{A: 5, B: "yo"}, b, encOpts); err != nil {
		t.Fatal(err)
	}
	binary.BigEndian.PutUint16(b.B[1:3], v2id)
	v, _, err = Decode(b.B, decOpts)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := v.(evoV2); ok == false || got.A != 5 || got.B != "yo" || got.C != 0 {
		t.Fatalf("regcache backward: got %#v", v)
	}
	lib.ReleaseBuffer(b)
}

// TestSchemaEvolutionNested verifies that an evolving nested struct is bounded by
// its own length prefix, so the field after it is read correctly despite drift.
func TestSchemaEvolutionNested(t *testing.T) {
	registerEvo(t, innerV1{}, innerV2{}, wrapV1{}, wrapV2{})
	opts := Options{SchemaEvolution: true}
	wrapName2 := []byte(regTypeName(reflect.TypeOf(wrapV2{})))
	wrapName1 := []byte(regTypeName(reflect.TypeOf(wrapV1{})))

	// writer has innerV2 (P,Q); reader has innerV1 (P). The inner Q must be skipped
	// via the inner length so Post is still read correctly (not corrupted).
	b := lib.TakeBuffer()
	if err := Encode(wrapV2{Pre: 1, In: innerV2{P: 2, Q: 3}, Post: 4}, b, opts); err != nil {
		t.Fatal(err)
	}
	// only the wrapper name is on the wire; the inner is a positional field. Retarget
	// the wrapper to the V1 decoder, whose In field decodes with the innerV1 layout.
	v, tail, err := Decode(bytes.Replace(b.B, wrapName2, wrapName1, 1), opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(tail) != 0 {
		t.Fatalf("nested: unexpected tail %d", len(tail))
	}
	got, ok := v.(wrapV1)
	if ok == false || got.Pre != 1 || got.In.P != 2 || got.Post != 4 {
		t.Fatalf("nested: got %#v", v)
	}
	lib.ReleaseBuffer(b)
}
