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
}

type CacheItem struct {
	ID      int16
	Encoded bool
	Name    Atom
}

type ListAtomCache struct {
	L        []CacheItem
	original []CacheItem
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
	if a.lastID < maxCacheItems {
		a.update <- atom
	}
	// otherwise ignore
}

func (a *AtomCache) GetLastID() int16 {
	return a.lastID
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
				a.cacheList[id] = atom
				a.lastID = id

			case <-ctx.Done():
				return
			}
		}
	}()

	return a
}

func (a *AtomCache) List() [maxCacheItems]Atom {
	return a.cacheList
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
}
func (l *ListAtomCache) Append(a CacheItem) {
	l.L = append(l.L, a)
}

func (l *ListAtomCache) Len() int {
	return len(l.L)
}
