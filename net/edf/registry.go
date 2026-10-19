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
// Each published cache id takes the next revision of the registry.
type Registry struct {
	encoders sync.Map
	decoders sync.Map

	registeredTypes sync.Map // reflect.Type -> *gen.RegisteredTypeInfo.
	typesByName     sync.Map // registered name -> reflect.Type, for LookupType.
	registerOrder   atomic.Uint64

	regCache  sync.Map // reflect.Type -> regEntry, uint16 -> type name
	errCache  sync.Map // error -> cacheEntry
	slots     sync.Map // error text -> *errorSlot
	atomCache sync.Map // gen.Atom -> cacheEntry

	mu          sync.Mutex
	regCacheID  uint32
	errCacheID  uint32
	atomCacheID uint32
	log         []published
	revision    atomic.Uint64
}

type regEntry struct {
	prefix []byte
	rev    uint64
}

type cacheEntry struct {
	id  uint16
	rev uint64
}

type errorSlot struct {
	local atomic.Pointer[error]
}

// RemoteError is an error received from a peer. It resolves to the local
// sentinel with the same text, registered now or later.
type RemoteError struct {
	remote error
	slot   *errorSlot
}

// Value returns the local sentinel if it is registered, the received error otherwise.
func (e RemoteError) Value() error {
	if p := e.slot.local.Load(); p != nil {
		return *p
	}
	return e.remote
}

const (
	publishedType = iota
	publishedAtom
	publishedError
)

type published struct {
	kind int
	id   uint16
	name string
	err  error
}

// Dictionary is what a peer learns from a registry: cache ids with their
// type names, atoms and errors, up to Revision.
type Dictionary struct {
	Revision uint64
	Types    map[uint16]string
	Atoms    map[uint16]gen.Atom
	Errors   map[uint16]error
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
		entry, ok := v.(cacheEntry)
		if ok == false {
			return true
		}
		id := entry.id
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
		entry, ok := v.(cacheEntry)
		if ok == false {
			return true
		}
		id := entry.id
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

// Since returns the entries published after revision rev. Since(0) is the
// whole dictionary.
func (r *Registry) Since(rev uint64) Dictionary {
	r.mu.Lock()
	log := r.log
	r.mu.Unlock()

	d := Dictionary{Revision: uint64(len(log))}
	if rev >= d.Revision {
		return d
	}
	for _, p := range log[rev:] {
		switch p.kind {
		case publishedType:
			if d.Types == nil {
				d.Types = make(map[uint16]string)
			}
			d.Types[p.id] = p.name
		case publishedAtom:
			if d.Atoms == nil {
				d.Atoms = make(map[uint16]gen.Atom)
			}
			d.Atoms[p.id] = gen.Atom(p.name)
		case publishedError:
			if d.Errors == nil {
				d.Errors = make(map[uint16]error)
			}
			d.Errors[p.id] = p.err
		}
	}
	return d
}

// Revision returns the revision of the last published cache id.
func (r *Registry) Revision() uint64 {
	return r.revision.Load()
}

// Sentinel returns the registered error with the given text.
func (r *Registry) Sentinel(text string) (error, bool) {
	v, found := r.slots.Load(text)
	if found == false {
		return nil, false
	}
	if p := v.(*errorSlot).local.Load(); p != nil {
		return *p, true
	}
	return nil, false
}

// RemoteError binds an error received from a peer to the local sentinel with its text.
func (r *Registry) RemoteError(err error) RemoteError {
	return RemoteError{remote: err, slot: r.slot(err.Error())}
}

func (r *Registry) slot(text string) *errorSlot {
	if v, found := r.slots.Load(text); found {
		return v.(*errorSlot)
	}
	v, _ := r.slots.LoadOrStore(text, &errorSlot{})
	return v.(*errorSlot)
}

func (r *Registry) commit(p published, rev uint64) {
	r.log = append(r.log, p)
	r.revision.Store(rev)
}

func (r *Registry) addErrCache(e error) error {
	if _, ok := e.(*gen.Error); ok {
		return fmt.Errorf("cannot register *gen.Error: register markers (errors.New) and construct wrap chains via gen.Errorf")
	}
	if len(e.Error()) > math.MaxInt16 {
		return ErrErrorTooLong
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exist := r.errCache.Load(e); exist {
		return nil
	}
	id := r.errCacheID + 1
	// 0xFFFF reserved for nil error, 0xFFFE reserved for wrapped *gen.Error marker.
	if id > math.MaxUint16-2 {
		return fmt.Errorf("too many registered errors")
	}
	r.errCacheID = id
	rev := uint64(len(r.log)) + 1
	r.errCache.Store(e, cacheEntry{uint16(id), rev})
	if slot := r.slot(e.Error()); slot.local.Load() == nil {
		slot.local.Store(&e)
	}
	r.commit(published{kind: publishedError, id: uint16(id), name: e.Error(), err: e}, rev)
	return nil
}

func (r *Registry) addAtomCache(atom gen.Atom) error {
	if len(atom) > 255 {
		return ErrAtomTooLong
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exist := r.atomCache.Load(atom); exist {
		return nil
	}
	id := r.atomCacheID + 1
	// the last 1000 ids for the custom atoms
	if id > math.MaxUint16-1000 {
		return fmt.Errorf("too many registered atoms")
	}
	r.atomCacheID = id
	rev := uint64(len(r.log)) + 1
	r.atomCache.Store(atom, cacheEntry{uint16(id), rev})
	r.commit(published{kind: publishedAtom, id: uint16(id), name: string(atom)}, rev)
	return nil
}
