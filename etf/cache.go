package etf

import (
	"fmt"
	"sync"
)

const (
	maxCacheItems = int16(2048)
)

type AtomCache struct {
	In  *AtomCacheIn
	Out *AtomCacheOut
}

type AtomCacheIn struct {
	Atoms [maxCacheItems]*Atom
}

// AtomCache
type AtomCacheOut struct {
	sync.RWMutex
	cacheMap  map[Atom]int16
	id        int16
	cacheList [maxCacheItems]Atom
}

// CacheItem
type CacheItem struct {
	ID      int16
	Encoded bool
	Name    Atom
}

var (
	encodingAtomCachePool = &sync.Pool{
		New: func() interface{} {
			l := &EncodingAtomCache{
				L:     make([]CacheItem, 0, 255),
				added: make(map[Atom]int16),
			}
			l.original = l.L
			return l
		},
	}
)

// NewAtomCache
func NewAtomCache() AtomCache {
	return AtomCache{
		In: &AtomCacheIn{},
		Out: &AtomCacheOut{
			cacheMap: make(map[Atom]int16),
			id:       -1,
		},
	}
}

// Append
func (a *AtomCacheOut) Append(atom Atom) {
	a.Lock()
	defer a.Unlock()
	if a.id > maxCacheItems-2 {
		return
	}
	if _, exist := a.cacheMap[atom]; exist {
		return
	}
	a.id++
	a.cacheList[a.id] = atom
	a.cacheMap[atom] = a.id
}

// LastID
func (a *AtomCacheOut) LastID() int16 {
	a.RLock()
	defer a.RUnlock()
	return a.id
}

// ListSince
func (a *AtomCacheOut) ListSince(id int16) []Atom {
	if id > a.id {
		return nil
	}
	return a.cacheList[id:a.id]
}

// EncodingAtomCache
type EncodingAtomCache struct {
	L           []CacheItem
	original    []CacheItem
	added       map[Atom]int16
	HasLongAtom bool
}

// TakeEncodingAtomCache
func TakeEncodingAtomCache() *EncodingAtomCache {
	return encodingAtomCachePool.Get().(*EncodingAtomCache)
}

// ReleaseEncodingAtomCache
func ReleaseEncodingAtomCache(l *EncodingAtomCache) {
	l.L = l.original[:0]
	if len(l.added) > 0 {
		panic(fmt.Sprint("encoding atom is not empty on release: ", l.added))
	}
	encodingAtomCachePool.Put(l)
}

// Reset
func (l *EncodingAtomCache) Reset() {
	l.L = l.original[:0]
	l.HasLongAtom = false
	if len(l.added) > 0 {
		panic(fmt.Sprint("encoding atom is not empty on reset: ", l.added))
	}
}

// Append
func (l *EncodingAtomCache) Append(a CacheItem) (int16, bool) {
	id, added := l.added[a.Name]
	if added {
		return id, false
	}

	l.L = append(l.L, a)
	if !a.Encoded && len(a.Name) > 255 {
		l.HasLongAtom = true
	}
	l.added[a.Name] = a.ID
	return a.ID, true
}
func (l *EncodingAtomCache) Delete(atom Atom) {
	// clean up in order to get rid of reallocation
	delete(l.added, atom)
}

// Len
func (l *EncodingAtomCache) Len() int {
	return len(l.L)
}
