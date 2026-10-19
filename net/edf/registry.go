package edf

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"ergo.services/ergo/gen"
)

// Registry holds the types, errors and atoms EDF can encode and decode, with
// their cache ids. A new registry has the built-in and framework ones.
// Registries are independent: a type registered in one is unknown to another.
type Registry struct {
	encoders sync.Map
	decoders sync.Map

	registeredTypes sync.Map // reflect.Type -> *gen.RegisteredTypeInfo.
	typesByName     sync.Map // registered name -> reflect.Type, for LookupType.
	registerOrder   atomic.Uint64

	regCacheID  uint32
	regCache    sync.Map
	errCacheID  uint32
	errCache    sync.Map
	atomCacheID uint32
	atomCache   sync.Map
}

var defaultRegistry *Registry

// NewRegistry creates a registry with the built-in and framework types and errors.
func NewRegistry() *Registry {
	r := &Registry{
		regCacheID:  4095,          // 0..4095 - reserved (used as a length)
		errCacheID:  math.MaxInt16, // 0..32767 - reserved (used as a length)
		atomCacheID: 255,           // 0..255 - reserved (used as a length)
	}
	r.registerBuiltins()
	for _, t := range genTypes {
		if err := r.RegisterType(t); err != nil {
			panic(err)
		}
	}
	for _, e := range genErrors {
		if err := r.RegisterError(e); err != nil {
			panic(err)
		}
	}
	return r
}

// RegisterType registers the type of v and the types of its fields.
func (r *Registry) RegisterType(v any) error {
	return r.registerTypeOf(reflect.TypeOf(v), make(map[reflect.Type]bool))
}

// RegisterTypes registers a batch in any order: a type whose dependencies are
// not registered yet is retried while the others make progress.
func (r *Registry) RegisterTypes(types []any) error {
	pending := types
	for len(pending) > 0 {
		var next []any
		progress := false
		for _, t := range pending {
			err := r.RegisterType(t)
			if err == nil {
				progress = true
				continue
			}
			next = append(next, t)
		}
		if progress == false && len(next) > 0 {
			names := make([]string, 0, len(next))
			for _, t := range next {
				names = append(names, fmt.Sprintf("%T", t))
			}
			return fmt.Errorf("unresolvable types: %s", strings.Join(names, ", "))
		}
		pending = next
	}
	return nil
}

// RegisterError registers a sentinel error for the error cache.
func (r *Registry) RegisterError(e error) error {
	return r.addErrCache(e)
}

// RegisterErrors registers sentinel errors, stopping at the first failure.
func (r *Registry) RegisterErrors(errs []error) error {
	for _, e := range errs {
		if err := r.addErrCache(e); err != nil {
			return err
		}
	}
	return nil
}

// RegisterAtom registers an atom for the atom cache.
func (r *Registry) RegisterAtom(a gen.Atom) error {
	return r.addAtomCache(a)
}

// RegisterAtoms registers atoms, stopping at the first failure.
func (r *Registry) RegisterAtoms(atoms []gen.Atom) error {
	for _, a := range atoms {
		if err := r.addAtomCache(a); err != nil {
			return err
		}
	}
	return nil
}

// RegisteredTypes returns the type metadata in registration order. Each entry
// is a snapshot; its counters are the values at the time of the call.
func (r *Registry) RegisteredTypes() []gen.RegisteredTypeInfo {
	var list []gen.RegisteredTypeInfo
	r.registeredTypes.Range(func(_, v any) bool {
		list = append(list, *v.(*gen.RegisteredTypeInfo))
		return true
	})
	sort.Slice(list, func(i, j int) bool {
		return list[i].ID < list[j].ID
	})
	return list
}

// RegisteredErrors returns the error cache entries ordered by cache id,
// the framework sentinels included.
func (r *Registry) RegisteredErrors() []gen.RegisteredErrorInfo {
	var list []gen.RegisteredErrorInfo
	r.errCache.Range(func(k, v any) bool {
		id, ok := v.(uint16)
		if ok == false {
			return true
		}
		e, ok := k.(error)
		if ok == false {
			return true
		}
		list = append(list, gen.RegisteredErrorInfo{ID: id, Text: e.Error()})
		return true
	})
	sort.Slice(list, func(i, j int) bool {
		return list[i].ID < list[j].ID
	})
	return list
}

// RegisteredAtoms returns the atom cache entries ordered by cache id.
func (r *Registry) RegisteredAtoms() []gen.RegisteredAtomInfo {
	var list []gen.RegisteredAtomInfo
	r.atomCache.Range(func(k, v any) bool {
		id, ok := v.(uint16)
		if ok == false {
			return true
		}
		a, ok := k.(gen.Atom)
		if ok == false {
			return true
		}
		list = append(list, gen.RegisteredAtomInfo{ID: id, Name: a})
		return true
	})
	sort.Slice(list, func(i, j int) bool {
		return list[i].ID < list[j].ID
	})
	return list
}

// LookupType returns the type registered under the canonical EDF name
// ("#pkgpath/TypeName"), matched exactly.
func (r *Registry) LookupType(name string) (reflect.Type, bool) {
	if v, ok := r.decoders.Load(name); ok {
		return v.(*decoder).Type, true
	}
	// The types the framework seeds - the primitives, time.Time, time.Duration - are
	// keyed by their wire tag and by their Go type, never by name: they are not framed
	// with one. They are registered all the same, and a caller asking whether this node
	// speaks a type is asking about that, not about the framing.
	if v, ok := r.typesByName.Load(name); ok {
		return v.(reflect.Type), true
	}
	return nil, false
}

// GetRegCache returns the type cache: cache id -> type name.
func (r *Registry) GetRegCache() map[uint16]string {
	cache := make(map[uint16]string)
	r.regCache.Range(func(k, v any) bool {
		id, ok := k.(uint16)
		if ok == false {
			return true
		}
		cache[id] = v.(string)
		return true
	})
	if len(cache) == 0 {
		return nil
	}
	return cache
}

// MakeEncodeRegTypeCache returns the encoding cache for the named types.
func (r *Registry) MakeEncodeRegTypeCache(names []string) *sync.Map {
	mapnames := make(map[string]bool)
	for _, name := range names {
		mapnames[name] = true
	}
	if len(mapnames) == 0 {
		return nil
	}
	cache := new(sync.Map)
	r.regCache.Range(func(k, v any) bool {
		t, ok := k.(reflect.Type)
		if ok == false {
			return true
		}
		if _, found := mapnames[regTypeName(t)]; found == false {
			return true
		}
		cache.Store(t, v)
		return true
	})
	return cache
}

// GetErrCache returns the error cache: cache id -> error.
func (r *Registry) GetErrCache() map[uint16]error {
	cache := make(map[uint16]error)
	r.errCache.Range(func(k, v any) bool {
		if id, ok := v.(uint16); ok {
			cache[id] = k.(error)
		}
		return true
	})
	if len(cache) == 0 {
		return nil
	}
	return cache
}

// GetAtomCache returns the atom cache: cache id -> atom.
func (r *Registry) GetAtomCache() map[uint16]gen.Atom {
	cache := make(map[uint16]gen.Atom)
	r.atomCache.Range(func(k, v any) bool {
		id, ok := v.(uint16)
		if ok == false {
			return true
		}
		cache[id] = k.(gen.Atom)
		return true
	})
	if len(cache) == 0 {
		return nil
	}
	return cache
}

func (r *Registry) addErrCache(e error) error {
	if _, ok := e.(*gen.Error); ok {
		return fmt.Errorf("cannot register *gen.Error: register markers (errors.New) and construct wrap chains via gen.Errorf")
	}
	id := atomic.AddUint32(&r.errCacheID, 1)
	// 0xFFFF reserved for nil error, 0xFFFE reserved for wrapped *gen.Error marker.
	if id > math.MaxUint16-2 {
		return fmt.Errorf("too many registered errors")
	}
	r.errCache.LoadOrStore(e, uint16(id))
	return nil
}

func (r *Registry) addAtomCache(atom gen.Atom) error {
	id := atomic.AddUint32(&r.atomCacheID, 1)
	// the last 1000 ids for the custom atoms
	if id > math.MaxUint16-1000 {
		return fmt.Errorf("too many registered atoms")
	}
	r.atomCache.LoadOrStore(atom, uint16(id))
	return nil
}
