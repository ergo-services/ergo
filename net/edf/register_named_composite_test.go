package edf

import (
	"testing"

	"ergo.services/ergo/lib"
)

type namedTags []string
type namedCounts map[string]int
type namedPair [2]string

type namedHolder struct {
	Tags   namedTags
	Counts namedCounts
	Pair   namedPair
	PTags  *namedTags
	Many   []*namedTags
}

func roundTrip(t *testing.T, r *Registry, v any) any {
	t.Helper()
	b := lib.TakeBuffer()
	defer lib.ReleaseBuffer(b)
	if err := Encode(v, b, Options{Registry: r}); err != nil {
		t.Fatalf("encode %T: %s", v, err)
	}
	got, tail, err := Decode(b.B, Options{Registry: r})
	if err != nil {
		t.Fatalf("decode %T: %s", v, err)
	}
	if len(tail) > 0 {
		t.Fatalf("decode %T left %d byte(s)", v, len(tail))
	}
	return got
}

// a named slice, map or array in a field is registered with the type that has
// it, the way a named scalar is, so it keeps its name on the wire and a pointer
// to it has somewhere to decode.
func TestRegisterNamedCompositeInField(t *testing.T) {
	r := NewRegistry()
	if err := r.RegisterType(namedHolder{}); err != nil {
		t.Fatalf("register: %s", err)
	}

	for _, name := range []string{
		"#ergo.services/ergo/net/edf/namedTags",
		"#ergo.services/ergo/net/edf/namedCounts",
		"#ergo.services/ergo/net/edf/namedPair",
	} {
		if _, found := r.LookupType(name); found == false {
			t.Errorf("%s is not registered with the type that has it in a field", name)
		}
	}

	tags := namedTags{"a", "b"}
	original := namedHolder{
		Tags:   namedTags{"x"},
		Counts: namedCounts{"n": 1},
		Pair:   namedPair{"k", "v"},
		PTags:  &tags,
		Many:   []*namedTags{&tags, nil},
	}

	got, ok := roundTrip(t, r, original).(namedHolder)
	if ok == false {
		t.Fatal("decoded value is not namedHolder")
	}
	if len(got.Tags) != 1 || got.Tags[0] != "x" {
		t.Errorf("Tags = %v", got.Tags)
	}
	if got.Counts["n"] != 1 {
		t.Errorf("Counts = %v", got.Counts)
	}
	if got.Pair != original.Pair {
		t.Errorf("Pair = %v", got.Pair)
	}
	if got.PTags == nil || (*got.PTags)[1] != "b" {
		t.Errorf("PTags = %v", got.PTags)
	}
	if len(got.Many) != 2 || got.Many[0] == nil || (*got.Many[0])[0] != "a" || got.Many[1] != nil {
		t.Errorf("Many = %v", got.Many)
	}
}

// once registered, a named composite keeps its type as a message of its own
// instead of arriving as the underlying one.
func TestNamedCompositeKeepsItsTypeAsMessage(t *testing.T) {
	r := NewRegistry()
	if err := r.RegisterType(namedHolder{}); err != nil {
		t.Fatalf("register: %s", err)
	}

	got := roundTrip(t, r, namedTags{"a"})
	if _, ok := got.(namedTags); ok == false {
		t.Fatalf("decoded %T, want namedTags", got)
	}
}
