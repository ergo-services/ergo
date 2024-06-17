package lib

import (
	"sync"
)

type Map[K comparable, V any] struct {
	sync.RWMutex
	m map[K]V
}

func (m *Map[K, V]) Load(key K) (V, bool) {
	m.RLock()
	v, found := m.m[key]
	m.RUnlock()
	return v, found
}

func (m *Map[K, V]) LoadAndDelete(key K) (V, bool) {
	m.Lock()
	v, found := m.m[key]
	m.Unlock()
	return v, found
}

func (m *Map[K, V]) Store(key K, value V) {
	m.Lock()
	if m.m == nil {
		m.m = make(map[K]V)
	}
	m.m[key] = value
	m.Unlock()
}

func (m *Map[K, V]) Delete(key K) {
	m.Lock()
	delete(m.m, key)
	m.Unlock()
}

// DeleteNoLock to be used within RangeLock method
func (m *Map[K, V]) DeleteNoLock(key K) {
	delete(m.m, key)
}

func (m *Map[K, V]) Range(f func(k K, v V) bool) {
	m.RLock()
	for mk, mv := range m.m {
		if f(mk, mv) == false {
			break
		}
	}
	m.RUnlock()
}

// RangeLock locks map during iterating. You can use DeleteNoLock/StoreNoLock
// within your f-function
func (m *Map[K, V]) RangeLock(f func(k K, v V) bool) {
	m.Lock()
	for mk, mv := range m.m {
		if f(mk, mv) == false {
			break
		}
	}
	m.Unlock()
}

func (m *Map[K, V]) Len() int {
	m.RLock()
	l := len(m.m)
	m.RUnlock()
	return l
}

func (m *Map[K, V]) LoadOrStore(key K, value V) (V, bool) {
	m.Lock()
	if m.m == nil {
		m.m = make(map[K]V)
	}
	if x, exist := m.m[key]; exist {
		m.Unlock()
		return x, true
	}
	m.m[key] = value
	m.Unlock()
	return value, false
}
