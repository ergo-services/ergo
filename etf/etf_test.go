package etf

import (
	"bytes"
	"testing"
)

func TestTermIntoStruct_Slice(t *testing.T) {
	dest := struct{ Slice []byte }{}

	tests := []struct {
		want []byte
		term Term
	}{
		{[]byte{1, 2, 3}, List{1, 2, 3}},
		{[]byte{3, 4, 5}, Tuple{3, 4, 5}},
	}

	for _, tt := range tests {
		term := Map{"slice": tt.term}
		if err := TermIntoStruct(term, &dest); err != nil {
			t.Errorf("%#v: conversion failed: %v", term, err)
		}

		if !bytes.Equal(dest.Slice, tt.want) {
			t.Errorf("%#v: got %v, want %v", term, dest.Slice, tt.want)
		}
	}
}

func TestTermIntoStruct_Array(t *testing.T) {
	dest := struct{ Array [3]byte }{}

	tests := []struct {
		want [3]byte
		term Term
	}{
		{[...]byte{1, 2, 3}, List{1, 2, 3}},
		{[...]byte{3, 4, 5}, Tuple{3, 4, 5}},
	}

	for _, tt := range tests {
		term := Map{"array": tt.term}
		if err := TermIntoStruct(term, &dest); err != nil {
			t.Errorf("%#v: conversion failed: %v", term, err)
		}

		if dest.Array != tt.want {
			t.Errorf("%#v: got %v, want %v", term, dest.Array, tt.want)
		}
	}
}
