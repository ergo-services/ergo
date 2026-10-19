package edf

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
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
	freshRegistry := NewRegistry()
	fresh := freshRegistry.Since(0)
	def := defaultRegistry.Since(0)
	for id, name := range fresh.Types {
		if def.Types[id] != name {
			t.Fatalf("type id %d: %q in a new registry, %q in the default one", id, name, def.Types[id])
		}
	}
	for id, e := range fresh.Errors {
		if def.Errors[id] != e {
			t.Fatalf("error id %d: %v in a new registry, %v in the default one", id, e, def.Errors[id])
		}
	}
	if len(fresh.Errors) != len(genErrors) {
		t.Fatalf("a new registry has %d errors, want %d", len(fresh.Errors), len(genErrors))
	}
	for _, v := range []any{gen.PID{}, gen.Ref{}, gen.Atom(""), int64(0), "", gen.Version{}} {
		buf1, buf2 := lib.TakeBuffer(), lib.TakeBuffer()
		if err := Encode(v, buf1, Options{Registry: freshRegistry}); err != nil {
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
	for id, n := range r.Since(0).Types {
		if n == name {
			return id
		}
	}
	return 0
}

func hasError(r *Registry, e error) bool {
	for _, known := range r.Since(0).Errors {
		if known == e {
			return true
		}
	}
	return false
}

// cachedIDs makes o encode exactly the given keys by the given cache ids: a
// reflect.Type for a type, gen.Atom for an atom, an error for an error.
func cachedIDs(t *testing.T, o Options, ids map[any]uint16) Options {
	t.Helper()
	r := defaultRegistry
	for k, id := range ids {
		key := k
		var cache *sync.Map
		var entry any
		switch k.(type) {
		case reflect.Type:
			cache, entry = &r.regCache, regEntry{[]byte{edtReg, byte(id >> 8), byte(id)}, 0}
		case gen.Atom:
			cache, entry = &r.atomCache, cacheEntry{id, 0}
		case error:
			cache, entry = &r.errCache, cacheEntry{id, 0}
		default:
			t.Fatalf("unsupported cache key %T", k)
		}
		prev, had := cache.Load(key)
		cache.Store(key, entry)
		t.Cleanup(func() {
			if had {
				cache.Store(key, prev)
				return
			}
			cache.Delete(key)
		})
	}
	o.Revision = new(atomic.Uint64)
	return o
}

type registryRevA struct{ A int }
type registryRevB struct{ B int }

func TestRegistrySince(t *testing.T) {
	r := NewRegistry()
	base := r.Revision()
	if base == 0 || r.Since(0).Revision != base {
		t.Fatalf("the profile publishes revisions: %d, %d", base, r.Since(0).Revision)
	}
	if err := r.RegisterType(registryRevA{}); err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("registry since")
	if err := r.RegisterError(sentinel); err != nil {
		t.Fatal(err)
	}
	if err := r.RegisterAtom("registry.since"); err != nil {
		t.Fatal(err)
	}
	d := r.Since(base)
	if d.Revision != base+3 || len(d.Types) != 1 || len(d.Errors) != 1 || len(d.Atoms) != 1 {
		t.Fatalf("unexpected delta: %+v", d)
	}
	for _, name := range d.Types {
		if name != regTypeName(reflect.TypeOf(registryRevA{})) {
			t.Fatalf("unexpected type %q", name)
		}
	}
	if empty := r.Since(d.Revision); empty.Revision != d.Revision || empty.Types != nil {
		t.Fatalf("nothing is newer than the last revision: %+v", empty)
	}
	if err := r.RegisterError(sentinel); err != nil || r.Revision() != d.Revision {
		t.Fatal("registering the same error again publishes nothing")
	}
	if e, found := r.Sentinel("registry since"); found == false || e != sentinel {
		t.Fatal("the sentinel is found by its text")
	}
}

func TestRegistryEncodeByRevision(t *testing.T) {
	r := NewRegistry()
	if err := r.RegisterType(registryRevA{}); err != nil {
		t.Fatal(err)
	}
	known := new(atomic.Uint64)
	known.Store(r.Revision() - 1)
	o := Options{Registry: r, Revision: known, Cache: new(sync.Map)}

	encode := func(v any) []byte {
		b := lib.TakeBuffer()
		defer lib.ReleaseBuffer(b)
		if err := Encode(v, b, o); err != nil {
			t.Fatal(err)
		}
		return append([]byte(nil), b.B...)
	}
	full := encode([]any{registryRevA{A: 1}, []registryRevA{{A: 2}}})
	known.Store(r.Revision())
	cached := encode([]any{registryRevA{A: 1}, []registryRevA{{A: 2}}})
	if len(cached) >= len(full) {
		t.Fatalf("the known type goes by id: %d bytes, %d by name", len(cached), len(full))
	}
	name := regTypeName(reflect.TypeOf(registryRevA{}))
	if bytes.Contains(cached, []byte(name)) {
		t.Fatal("a memoized encoder kept the name the peer had not known yet")
	}

	sentinel := errors.New("registry by revision")
	if err := r.RegisterError(sentinel); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encode(sentinel), []byte("registry by revision")) == false {
		t.Fatal("an error newer than the peer revision goes as text")
	}
	known.Store(r.Revision())
	if bytes.Contains(encode(sentinel), []byte("registry by revision")) {
		t.Fatal("a known error goes by id")
	}
}

func TestRegistrySinceConcurrent(t *testing.T) {
	r := NewRegistry()
	seen := r.Revision()
	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				if err := r.RegisterAtom(gen.Atom(fmt.Sprintf("concurrent.%d.%d", g, i))); err != nil {
					t.Error(err)
					return
				}
			}
		}(g)
	}
	for i := 0; i < 100; i++ {
		d := r.Since(seen)
		if d.Revision < seen {
			t.Fatal("the revision went back")
		}
		if uint64(len(d.Atoms)) != d.Revision-seen {
			t.Fatalf("delta (%d, %d] has %d atoms", seen, d.Revision, len(d.Atoms))
		}
		seen = d.Revision
	}
	wg.Wait()
	if d := r.Since(0); len(d.Atoms) != 800 {
		t.Fatalf("all atoms are published: %d", len(d.Atoms))
	}
}

func TestRegistryRemoteError(t *testing.T) {
	r := NewRegistry()
	sentinel := errors.New("registry remote error")
	dec := new(sync.Map)
	dec.Store(uint16(40000), r.RemoteError(errors.New(sentinel.Error())))
	packet := []byte{edtError, 0x9c, 0x40}

	v, _, err := Decode(packet, Options{Registry: r, ErrCache: dec})
	if err != nil {
		t.Fatal(err)
	}
	if v == sentinel || v.(error).Error() != sentinel.Error() {
		t.Fatalf("before the registration the received error is decoded: %v", v)
	}
	if err := r.RegisterError(sentinel); err != nil {
		t.Fatal(err)
	}
	v, _, err = Decode(packet, Options{Registry: r, ErrCache: dec})
	if err != nil {
		t.Fatal(err)
	}
	if v != sentinel {
		t.Fatal("after the registration the local sentinel is decoded")
	}
}
