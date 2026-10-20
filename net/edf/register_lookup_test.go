package edf

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"ergo.services/ergo/lib"
)

type testRegDurationHolder struct {
	Name string
	TTL  time.Duration
}

type testRegElapsed int64

func TestRegisterNamedScalarIsFoundByName(t *testing.T) {
	if err := defaultRegistry.RegisterType(testRegElapsed(0)); err != nil {
		t.Fatalf("register: %s", err)
	}

	want := reflect.TypeOf(testRegElapsed(0))
	got, found := defaultRegistry.LookupType(regTypeName(want))
	if found == false {
		t.Fatalf("%s is registered and still not found by name", want)
	}
	if got != want {
		t.Errorf("looking up %s answered with %s", want, got)
	}
}

func TestRegisterDurationIsAlreadyRegistered(t *testing.T) {
	err := defaultRegistry.RegisterType(time.Duration(0))
	if err != nil {
		t.Fatalf("registering time.Duration answered %v, which fails an application load", err)
	}
}

func TestRegisterSeededTypesAreFoundByName(t *testing.T) {
	for _, v := range []any{uint(0), int64(0), "", true, float64(0), time.Time{}, time.Duration(0)} {
		want := reflect.TypeOf(v)
		name := regTypeName(want)
		got, found := defaultRegistry.LookupType(name)
		if found == false {
			t.Errorf("%s is seeded by the framework and not found under %q", want, name)
			continue
		}
		if got != want {
			t.Errorf("looking up %q answered with %s, not %s", name, got, want)
		}
	}
}

func TestRegisterDurationRoundTrip(t *testing.T) {
	if err := defaultRegistry.RegisterType(testRegDurationHolder{}); err != nil {
		t.Fatalf("register holder: %s", err)
	}

	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	in := testRegDurationHolder{Name: "ttl", TTL: 90 * time.Second}
	if err := Encode(in, b, Options{}); err != nil {
		t.Fatalf("encode: %s", err)
	}
	out, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatalf("decode: %s", err)
	}
	if reflect.DeepEqual(in, out) == false {
		t.Fatalf("round trip: %#v came back as %#v", in, out)
	}

	bare := lib.TakeBuffer()
	defer lib.ReleaseBuffer(bare)
	if err := Encode(5*time.Second, bare, Options{}); err != nil {
		t.Fatalf("encode bare: %s", err)
	}
	if bare.B[0] != edtReg {
		t.Errorf("a Duration is framed as %d, not by its name", bare.B[0])
	}
	back, _, err := Decode(bare.B, Options{})
	if err != nil {
		t.Fatalf("decode bare: %s", err)
	}
	if back != 5*time.Second {
		t.Errorf("a bare Duration came back as %T %v", back, back)
	}
}

type testRegDurationParts struct {
	Slice  []time.Duration
	Array  [2]time.Duration
	Map    map[string]time.Duration
	Keys   map[time.Duration]string
	Ptr    *time.Duration
	Nested [][]time.Duration
}

func TestRegisterDurationInComposites(t *testing.T) {
	if err := defaultRegistry.RegisterType(testRegDurationParts{}); err != nil {
		t.Fatalf("register: %s", err)
	}

	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	d := time.Second
	in := testRegDurationParts{
		Slice:  []time.Duration{d, time.Minute},
		Array:  [2]time.Duration{d, time.Hour},
		Map:    map[string]time.Duration{"a": d},
		Keys:   map[time.Duration]string{d: "a"},
		Ptr:    &d,
		Nested: [][]time.Duration{{d}},
	}
	if err := Encode(in, b, Options{}); err != nil {
		t.Fatalf("encode: %s", err)
	}
	out, _, err := Decode(b.B, Options{})
	if err != nil {
		t.Fatalf("decode: %s", err)
	}
	if reflect.DeepEqual(in, out) == false {
		t.Fatalf("round trip: %#v came back as %#v", in, out)
	}

	for _, v := range []any{[]time.Duration{d}, map[string]time.Duration{"a": d}} {
		b.Reset()
		if err := Encode(v, b, Options{}); err != nil {
			t.Fatalf("encode %T: %s", v, err)
		}
		back, _, err := Decode(b.B, Options{})
		if err != nil {
			t.Fatalf("decode %T: %s", v, err)
		}
		if reflect.DeepEqual(v, back) == false {
			t.Errorf("%T came back as %T %v", v, back, back)
		}
	}
}

func TestRegisterDurationIsNeverNegative(t *testing.T) {
	for _, v := range []any{testRegDurationParts{}, testRegDurationHolder{}} {
		if err := defaultRegistry.RegisterType(v); err != nil {
			t.Fatalf("register %T: %s", v, err)
		}
	}

	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)

	d := -time.Second
	for _, v := range []any{
		d,
		[]time.Duration{time.Second, d},
		&d,
		map[string]time.Duration{"a": d},
		testRegDurationParts{Keys: map[time.Duration]string{d: "a"}},
		testRegDurationHolder{TTL: d},
	} {
		b.Reset()
		if err := Encode(v, b, Options{}); err != ErrNegativeDuration {
			t.Errorf("%T with a negative duration encoded with %v", v, err)
		}
	}

	b.Reset()
	if err := Encode(time.Second, b, Options{}); err != nil {
		t.Fatalf("encode: %s", err)
	}
	b.B[len(b.B)-8] = 0xff
	if _, _, err := Decode(b.B, Options{}); errors.Is(err, ErrNegativeDuration) == false {
		t.Errorf("a negative duration from the wire decoded with %v", err)
	}
}
