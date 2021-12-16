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
	lastID    int16
	cacheList [maxCacheItems]Atom
	sync.RWMutex
}

// CacheItem
type CacheItem struct {
	ID      int16
	Encoded bool
	Name    Atom
}

// ListAtomCache
type ListAtomCache struct {
	L           []CacheItem
	original    []CacheItem
	HasLongAtom bool
}

var (
	listAtomCachePool = &sync.Pool{
		New: func() interface{} {
			l := &ListAtomCache{
				L: make([]CacheItem, 0, 256),
			}
			l.original = l.L
			return l
		},
	}
)

// Append
func (a *AtomCache) Append(atom Atom) {
	a.Lock()
	id := a.lastID
	a.Unlock()
	if id < maxCacheItems {
		a.update <- atom
	}
	// otherwise ignore
}

// GetLastID
func (a *AtomCache) GetLastID() int16 {
	a.RLock()
	id := a.lastID
	a.RUnlock()
	return id
}

func (a *AtomCache) Stop() {
	close(a.stop)
}

// StartAtomCache
func StartAtomCache() *AtomCache {
	var id int16

	a := &AtomCache{
		cacheMap: make(map[Atom]int16),
		update:   make(chan Atom, 100),
		stop:     make(chan struct{}, 1),
		lastID:   -1,
	}

	go func() {
		for {
			select {
			case atom := <-a.update:
				if _, ok := a.cacheMap[atom]; ok {
					// already exist
					continue
				}

				id = a.lastID
				id++
				a.cacheMap[atom] = id
				a.Lock()
				a.cacheList[id] = atom
				a.lastID = id
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

// TakeListAtomCache
func TakeListAtomCache() *ListAtomCache {
	return listAtomCachePool.Get().(*ListAtomCache)
}

// ReleaseListAtomCache
func ReleaseListAtomCache(l *ListAtomCache) {
	l.L = l.original[:0]
	listAtomCachePool.Put(l)
}

// Reset
func (l *ListAtomCache) Reset() {
	l.L = l.original[:0]
	l.HasLongAtom = false
}

// Append
func (l *ListAtomCache) Append(a CacheItem) {
	l.L = append(l.L, a)
	if !a.Encoded && len(a.Name) > 255 {
		l.HasLongAtom = true
	}
}

// Len
func (l *ListAtomCache) Len() int {
	return len(l.L)
}
