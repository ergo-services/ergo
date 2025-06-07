package lib

import (
	"testing"
)

func TestBiMap(t *testing.T) {
	var bm BiMap[int, string]

	bm.Set(12, "12")

	if b, _ := bm.GetB(12); b != "12" {
		t.Fatal("incorrect B")
	}

	if a, _ := bm.GetA("12"); a != 12 {
		t.Fatal("incorrect A")
	}

	la := bm.ListA()
	if len(la) != 1 {
		t.Fatal("incorrect list A")
	}
	if la[0] != 12 {
		t.Fatal("incorrect value in list A")
	}
	lb := bm.ListB()
	if len(lb) != 1 {
		t.Fatal("incorrect list B")
	}
	if lb[0] != "12" {
		t.Fatal("incorrect value in list B")
	}

	bm.DeleteA(12)
	if _, found := bm.GetB(12); found {
		t.Fatal("found B after detele")
	}
	if _, found := bm.GetA("12"); found {
		t.Fatal("found A after delete")
	}

}
