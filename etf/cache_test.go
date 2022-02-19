package etf

import (
	"reflect"
	"testing"
)

func TestAtomCache(t *testing.T) {

	a := NewAtomCache()

	a.Out.Append(Atom("test1"))

	if a.Out.LastID() != 0 {
		t.Fatal("LastID != 0", a.Out.LastID())
	}

	a.Out.Append(Atom("test1"))

	if a.Out.LastID() != 0 {
		t.Fatalf("LastID != 0")
	}

	a.Out.Append(Atom("test2"))

	expected := []Atom{"test1", "test2"}
	result := a.Out.ListSince(0)
	if reflect.DeepEqual(result, expected) {
		t.Fatal("got incorrect result", result)
	}

	expectedArray := make([]Atom, 2048)
	expectedArray[0] = "test1"
	expectedArray[1] = "test2"

	resultArray := a.Out.ListSince(0)
	if reflect.DeepEqual(resultArray, expectedArray) {
		t.Fatal("got incorrect resultArray", result)
	}
}
