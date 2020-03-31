package etf

import (
	"context"
)

const (
	maxCacheItems = uint16(2048)
)

type AtomCache struct {
	cacheMap  map[Atom]uint16
	update    chan Atom
	lastID    uint16
	cacheList [maxCacheItems]Atom
}

type CacheItem struct {
	ID      uint16
	Encoded bool
	Name    Atom
}

func (a *AtomCache) Append(atom Atom) {
	if a.lastID < maxCacheItems {
		a.update <- atom
	}
	// otherwise ignore
}

func (a *AtomCache) GetLastID() uint16 {
	return a.lastID
}

func NewAtomCache(ctx context.Context) *AtomCache {
	var id uint16

	a := &AtomCache{
		cacheMap: make(map[Atom]uint16),
		update:   make(chan Atom, 100),
		lastID:   0,
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

func (a *AtomCache) ListSince(id uint16) []Atom {
	return a.cacheList[id:]
}
