package etf

import (
	"sync"
)

const (
	maxCacheItems = int16(2048)
)

// AtomCache
type AtomCache struct {
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
				L: make([]CacheItem, 0, 255),
			}
			l.original = l.L
			return l
		},
	}
)

// NewAtomCache
func NewAtomCache() *AtomCache {
	return &AtomCache{
		cacheMap: make(map[Atom]int16),
		id:       -1,
	}
}

// Append
func (a *AtomCache) Append(atom Atom) {
	a.Lock()
	defer a.Unlock()
	if _, exist := a.cacheMap[atom]; exist {
		return
	}
	a.id++
	a.cacheList[a.id] = atom
	a.cacheMap[atom] = a.id
}

// LastID
func (a *AtomCache) LastID() int16 {
	a.RLock()
	defer a.RUnlock()
	return a.id
}

// ListSince
func (a *AtomCache) ListSince(id int16) []Atom {
	if id > a.id {
		return nil
	}
	return a.cacheList[id:a.id]
}

// EncodingAtomCache
type EncodingAtomCache struct {
	L           []CacheItem
	original    []CacheItem
	HasLongAtom bool
}

// TakeEncodingAtomCache
func TakeEncodingAtomCache() *EncodingAtomCache {
	return encodingAtomCachePool.Get().(*EncodingAtomCache)
}

// ReleaseEncodingAtomCache
func ReleaseEncodingAtomCache(l *EncodingAtomCache) {
	l.L = l.original[:0]
	encodingAtomCachePool.Put(l)
}

// Reset
func (l *EncodingAtomCache) Reset() {
	l.L = l.original[:0]
	l.HasLongAtom = false
}

// Append
func (l *EncodingAtomCache) Append(a CacheItem) {
	l.L = append(l.L, a)
	if !a.Encoded && len(a.Name) > 255 {
		l.HasLongAtom = true
	}
}

// Len
func (l *EncodingAtomCache) Len() int {
	return len(l.L)
}
