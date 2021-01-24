package etf

import (
	"context"
	"sync"
)

const (
	maxCacheItems = int16(2048)
)

type AtomCache struct {
	cacheMap  map[Atom]int16
	update    chan Atom
	lastID    int16
	cacheList [maxCacheItems]Atom
	sync.Mutex
}

type CacheItem struct {
	ID      int16
	Encoded bool
	Name    Atom
}

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

func (a *AtomCache) Append(atom Atom) {
	a.Lock()
	id := a.lastID
	a.Unlock()
	if id < maxCacheItems {
		a.update <- atom
	}
	// otherwise ignore
}

func (a *AtomCache) GetLastID() int16 {
	a.Lock()
	id := a.lastID
	a.Unlock()
	return id
}

func NewAtomCache(ctx context.Context) *AtomCache {
	var id int16

	a := &AtomCache{
		cacheMap: make(map[Atom]int16),
		update:   make(chan Atom, 100),
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

			case <-ctx.Done():
				return
			}
		}
	}()

	return a
}

func (a *AtomCache) List() [maxCacheItems]Atom {
	a.Lock()
	l := a.cacheList
	a.Unlock()
	return l
}

func (a *AtomCache) ListSince(id int16) []Atom {
	return a.cacheList[id:]
}

func TakeListAtomCache() *ListAtomCache {
	return listAtomCachePool.Get().(*ListAtomCache)
}

func ReleaseListAtomCache(l *ListAtomCache) {
	l.L = l.original[:0]
	listAtomCachePool.Put(l)
}
func (l *ListAtomCache) Reset() {
	l.L = l.original[:0]
	l.HasLongAtom = false
}
func (l *ListAtomCache) Append(a CacheItem) {
	l.L = append(l.L, a)
	if !a.Encoded && len(a.Name) > 255 {
		l.HasLongAtom = true
	}
}

func (l *ListAtomCache) Len() int {
	return len(l.L)
}
