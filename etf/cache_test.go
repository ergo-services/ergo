package etf

import (
	"reflect"
	"testing"
)

func TestAtomCache(t *testing.T) {

	a := NewAtomCache()

	a.Append(Atom("test1"))

	if a.LastID() != 0 {
		t.Fatal("LastID != 0", a.LastID())
	}

	a.Append(Atom("test1"))

	if a.LastID() != 0 {
		t.Fatalf("LastID != 0")
	}

	a.Append(Atom("test2"))

	expected := []Atom{"test1", "test2"}
	result := a.ListSince(0)
	if reflect.DeepEqual(result, expected) {
		t.Fatal("got incorrect result", result)
	}

	expectedArray := make([]Atom, 2048)
	expectedArray[0] = "test1"
	expectedArray[1] = "test2"

	resultArray := a.ListSince(0)
	if reflect.DeepEqual(resultArray, expectedArray) {
		t.Fatal("got incorrect resultArray", result)
	}
}
