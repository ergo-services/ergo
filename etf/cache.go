package etf

import (
	"sync"
)

const (
	maxCacheItems = int16(2048)
)

// AtomCache
type AtomCache struct {
	cacheMap  map[Atom]int16
	update    chan Atom
	stop      chan struct{}
	id        int16
	cacheList [maxCacheItems]Atom
	sync.RWMutex
}

// CacheItem
type CacheItem struct {
	ID      int16
	Encoded bool
	Name    Atom
}

// EncodingAtomCache
type EncodingAtomCache struct {
	L           []CacheItem
	original    []CacheItem
	HasLongAtom bool
}

var (
	listAtomCachePool = &sync.Pool{
		New: func() interface{} {
			l := &EncodingAtomCache{
				L: make([]CacheItem, 0, 255),
			}
			l.original = l.L
			return l
		},
	}
)

// Append
func (a *AtomCache) Append(atom Atom) {
	a.RLock()
	defer a.RUnlock()
	if a.id < maxCacheItems {
		a.update <- atom
	}
}

// LastID
func (a *AtomCache) LastID() int16 {
	a.RLock()
	defer a.RUnlock()
	return a.id
}

func (a *AtomCache) Stop() {
	close(a.stop)
}

// StartAtomCache
func StartAtomCache() *AtomCache {
	a := &AtomCache{
		cacheMap: make(map[Atom]int16),
		update:   make(chan Atom, 100),
		stop:     make(chan struct{}, 1),
		id:       -1,
	}

	go func() {
		for {
			select {
			case atom := <-a.update:
				if _, ok := a.cacheMap[atom]; ok {
					// already exist
					continue
				}

				a.Lock()
				if a.id < maxCacheItems {
					a.id++
					a.cacheMap[atom] = a.id
					a.cacheList[a.id] = atom
				}
				a.Unlock()

			case <-a.stop:
				return
			}
		}
	}()

	return a
}

// List
func (a *AtomCache) List() [maxCacheItems]Atom {
	a.Lock()
	l := a.cacheList
	a.Unlock()
	return l
}

// ListSince
func (a *AtomCache) ListSince(id int16) []Atom {
	return a.cacheList[id:]
}

// TakeEncodingAtomCache
func TakeEncodingAtomCache() *EncodingAtomCache {
	return listAtomCachePool.Get().(*EncodingAtomCache)
}

// ReleaseEncodingAtomCache
func ReleaseEncodingAtomCache(l *EncodingAtomCache) {
	l.L = l.original[:0]
	listAtomCachePool.Put(l)
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
