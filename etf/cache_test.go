package etf

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestAtomCache(t *testing.T) {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := NewAtomCache(ctx)

	a.Append(Atom("test1"))
	time.Sleep(100 * time.Millisecond)

	if a.GetLastID() != 1 {
		t.Fatal("LastID != 1")
	}

	a.Append(Atom("test1"))
	time.Sleep(100 * time.Millisecond)

	if a.GetLastID() != 1 {
		t.Fatal("LastID != 1")
	}

	a.Append(Atom("test2"))
	time.Sleep(100 * time.Millisecond)

	expected := []Atom{"test1", "test2"}
	result := a.ListSince(0)
	if reflect.DeepEqual(result, expected) {
		t.Fatal("got incorrect result", result)
	}

	expectedArray := make([]Atom, 2048)
	expectedArray[0] = "test1"
	expectedArray[1] = "test2"

	resultArray := a.List()
	if reflect.DeepEqual(resultArray, expectedArray) {
		t.Fatal("got incorrect resultArray", result)
	}
}
