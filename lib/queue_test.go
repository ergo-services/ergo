package lib

import "testing"

func TestQueueMPSCFIFO(t *testing.T) {
	q := NewQueueMPSC()
	for _, v := range []int{1, 2, 3} {
		if q.Push(v) == false {
			t.Fatalf("Push(%d) returned false", v)
		}
	}
	if q.Len() != 3 {
		t.Fatalf("Len = %d, want 3", q.Len())
	}

	for _, want := range []int{1, 2, 3} {
		v, ok := q.Pop()
		if ok == false || v.(int) != want {
			t.Fatalf("Pop = %v,%v want %d", v, ok, want)
		}
	}
	if _, ok := q.Pop(); ok {
		t.Fatal("Pop on an empty queue should return false")
	}
}

func TestQueueMPSCItemTraversal(t *testing.T) {
	q := NewQueueMPSC()
	q.Push("a")
	q.Push("b")

	item := q.Item()
	if item == nil {
		t.Fatal("Item on a non-empty queue returned nil")
	}
	if item.Value() != "a" {
		t.Fatalf("first item = %v, want a", item.Value())
	}

	next := item.Next()
	if next == nil || next.Value() != "b" {
		t.Fatalf("second item = %v, want b", next)
	}
}
