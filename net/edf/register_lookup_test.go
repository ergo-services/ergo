package edf

import (
	"reflect"
	"testing"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

type testRegDurationHolder struct {
	Name string
	TTL  time.Duration
}

type testRegElapsed int64

func TestRegisterNamedScalarIsFoundByName(t *testing.T) {
	if err := RegisterTypeOf(testRegElapsed(0)); err != nil {
		t.Fatalf("register: %s", err)
	}

	want := reflect.TypeOf(testRegElapsed(0))
	got, found := LookupType(regTypeName(want))
	if found == false {
		t.Fatalf("%s is registered and still not found by name", want)
	}
	if got != want {
		t.Errorf("looking up %s answered with %s", want, got)
	}
}

func TestRegisterDurationIsAlreadyRegistered(t *testing.T) {
	err := RegisterTypeOf(time.Duration(0))
	if err != nil && err != gen.ErrTaken {
		t.Fatalf("registering time.Duration answered %v, which fails an application load", err)
	}
}

func TestRegisterSeededTypesAreFoundByName(t *testing.T) {
	for _, v := range []any{uint(0), int64(0), "", true, float64(0), time.Time{}, time.Duration(0)} {
		want := reflect.TypeOf(v)
		name := regTypeName(want)
		got, found := LookupType(name)
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
	if err := RegisterTypeOf(testRegDurationHolder{}); err != nil {
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
	if bare.B[0] != edtInt64 {
		t.Errorf("a Duration is framed as %d, not as the int64 every node expects", bare.B[0])
	}
}
