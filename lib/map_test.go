package lib

import "testing"

func TestMapStoreLoadDelete(t *testing.T) {
	var m Map[string, int]
	m.Store("a", 1)
	m.Store("b", 2)

	v, ok := m.Load("a")
	if ok == false || v != 1 {
		t.Fatalf("Load(a) = %d,%v", v, ok)
	}
	if m.Len() != 2 {
		t.Fatalf("Len = %d, want 2", m.Len())
	}

	m.Delete("a")
	if _, ok := m.Load("a"); ok {
		t.Fatal("Load(a) after Delete should miss")
	}
}

func TestMapLoadOrStore(t *testing.T) {
	var m Map[string, int]

	v, loaded := m.LoadOrStore("x", 1)
	if loaded || v != 1 {
		t.Fatalf("first LoadOrStore = %d,%v, want 1,false", v, loaded)
	}

	v, loaded = m.LoadOrStore("x", 2)
	if loaded == false || v != 1 {
		t.Fatalf("second LoadOrStore = %d,%v, want 1,true", v, loaded)
	}
}

func TestMapLoadAndDelete(t *testing.T) {
	var m Map[string, int]
	m.Store("k", 7)

	v, ok := m.LoadAndDelete("k")
	if ok == false || v != 7 {
		t.Fatalf("LoadAndDelete = %d,%v", v, ok)
	}
	if _, ok := m.Load("k"); ok {
		t.Fatal("key still present after LoadAndDelete")
	}
}

func TestMapRange(t *testing.T) {
	var m Map[string, int]
	m.Store("a", 1)
	m.Store("b", 2)

	sum := 0
	m.Range(func(_ string, v int) bool {
		sum += v
		return true
	})
	if sum != 3 {
		t.Fatalf("Range sum = %d, want 3", sum)
	}
}
