package edf

import (
	"errors"
	"reflect"
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

type registryOnlyA struct{ A int }
type registryOnlyB struct{ B string }

func TestRegistryIndependent(t *testing.T) {
	a, b := NewRegistry(), NewRegistry()
	if err := a.RegisterType(registryOnlyA{}); err != nil {
		t.Fatal(err)
	}
	if err := b.RegisterType(registryOnlyB{}); err != nil {
		t.Fatal(err)
	}
	nameA := regTypeName(reflect.TypeOf(registryOnlyA{}))
	nameB := regTypeName(reflect.TypeOf(registryOnlyB{}))
	if _, found := a.LookupType(nameA); found == false {
		t.Fatal("a must know its type")
	}
	if _, found := b.LookupType(nameA); found {
		t.Fatal("b must not know the type of a")
	}
	if _, found := a.LookupType(nameB); found {
		t.Fatal("a must not know the type of b")
	}
	if _, found := defaultRegistry.LookupType(nameA); found {
		t.Fatal("the default registry must not know the type of a")
	}

	idA, idB := cacheID(a, nameA), cacheID(b, nameB)
	if idA == 0 || idA != idB {
		t.Fatalf("first registrations after the same profile get the same id: %d, %d", idA, idB)
	}

	sentinel := errors.New("registry only")
	if err := a.RegisterError(sentinel); err != nil {
		t.Fatal(err)
	}
	if hasError(b, sentinel) || hasError(defaultRegistry, sentinel) {
		t.Fatal("an error registered in a must stay in a")
	}
	if err := a.RegisterAtom("registry.only"); err != nil {
		t.Fatal(err)
	}
	for _, info := range b.RegisteredAtoms() {
		if info.Name == "registry.only" {
			t.Fatal("an atom registered in a must stay in a")
		}
	}
}

func TestRegistryCodecIsolation(t *testing.T) {
	a, b := NewRegistry(), NewRegistry()
	if err := a.RegisterType(registryOnlyA{}); err != nil {
		t.Fatal(err)
	}
	buf := lib.TakeBuffer()
	defer lib.ReleaseBuffer(buf)

	if err := Encode(registryOnlyA{A: 7}, buf, Options{Registry: b}); err == nil {
		t.Fatal("b must not encode the type of a")
	}
	buf.Reset()
	if err := Encode(registryOnlyA{A: 7}, buf, Options{Registry: a}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Decode(buf.B, Options{Registry: b}); err == nil {
		t.Fatal("b must not decode the type of a")
	}
	v, _, err := Decode(buf.B, Options{Registry: a})
	if err != nil {
		t.Fatal(err)
	}
	if v.(registryOnlyA).A != 7 {
		t.Fatalf("unexpected value %#v", v)
	}
}

func TestRegistryProfile(t *testing.T) {
	fresh := NewRegistry()
	def := defaultRegistry.GetRegCache()
	for id, name := range fresh.GetRegCache() {
		if def[id] != name {
			t.Fatalf("type id %d: %q in a new registry, %q in the default one", id, name, def[id])
		}
	}
	defErrs := defaultRegistry.GetErrCache()
	for id, e := range fresh.GetErrCache() {
		if defErrs[id] != e {
			t.Fatalf("error id %d: %v in a new registry, %v in the default one", id, e, defErrs[id])
		}
	}
	if len(fresh.GetErrCache()) != len(genErrors) {
		t.Fatalf("a new registry has %d errors, want %d", len(fresh.GetErrCache()), len(genErrors))
	}
	for _, v := range []any{gen.PID{}, gen.Ref{}, gen.Atom(""), int64(0), "", gen.Version{}} {
		buf1, buf2 := lib.TakeBuffer(), lib.TakeBuffer()
		if err := Encode(v, buf1, Options{Registry: fresh}); err != nil {
			t.Fatal(err)
		}
		if err := Encode(v, buf2, Options{}); err != nil {
			t.Fatal(err)
		}
		if string(buf1.B) != string(buf2.B) {
			t.Fatalf("%T encodes differently: %v, %v", v, buf1.B, buf2.B)
		}
		lib.ReleaseBuffer(buf1)
		lib.ReleaseBuffer(buf2)
	}
}

func cacheID(r *Registry, name string) uint16 {
	for id, n := range r.GetRegCache() {
		if n == name {
			return id
		}
	}
	return 0
}

func hasError(r *Registry, e error) bool {
	for _, known := range r.GetErrCache() {
		if known == e {
			return true
		}
	}
	return false
}
