package edf

import (
	"encoding/binary"
	"errors"
	"testing"
)

// nestingLevel is one level of a value nested through any
var nestingLevel = []byte{edtType, 0, 2, edtSlice, edtAny, edtSlice, 0, 0, 0, 1}

// nestingTail closes the nesting with a plain int.
var nestingTail = []byte{edtInt, 0, 0, 0, 0, 0, 0, 0, 42}

func nestedPacket(depth int) []byte {
	packet := make([]byte, 0, depth*len(nestingLevel)+len(nestingTail))
	for i := 0; i < depth; i++ {
		packet = append(packet, nestingLevel...)
	}
	return append(packet, nestingTail...)
}

// A value nested deeper than the decoder follows is refused
func TestDecodeNestingLimit(t *testing.T) {
	if _, _, err := Decode(nestedPacket(maxDecodeDepth), Options{}); err != nil {
		t.Fatalf("decoding %d levels: %s", maxDecodeDepth, err)
	}

	_, _, err := Decode(nestedPacket(maxDecodeDepth+1), Options{})
	if errors.Is(err, errDecodeDepth) == false {
		t.Fatalf("decoding %d levels returned %v, expected the depth limit", maxDecodeDepth+1, err)
	}
}

// A state handed back to the pool carries no nesting depth into the next decode.
func TestDecodeStatePoolResetsDepth(t *testing.T) {
	stateDecodePool.Put(&stateDecode{depth: maxDecodeDepth})

	state := getPooledStateDecode(Options{})
	if state.depth != 0 {
		t.Fatalf("a pooled decode state starts at depth %d, expected 0", state.depth)
	}
}

// typePacket is a folded type of n nested slices, with no value behind it
func typePacket(n int) []byte {
	fold := make([]byte, 0, n+1)
	for i := 0; i < n; i++ {
		fold = append(fold, edtSlice)
	}
	fold = append(fold, edtAny)

	packet := []byte{edtType, 0, 0}
	binary.BigEndian.PutUint16(packet[1:3], uint16(len(fold)))
	return append(packet, fold...)
}

// A type nested deeper than the decoder unfolds is refused
func TestDecodeTypeNestingLimit(t *testing.T) {
	_, _, err := Decode(typePacket(maxDecodeTypeDepth), Options{})
	if errors.Is(err, errDecodeEOD) == false {
		t.Fatalf("unfolding %d levels returned %v, expected the type to reach its value", maxDecodeTypeDepth, err)
	}

	_, _, err = Decode(typePacket(maxDecodeTypeDepth+1), Options{})
	if errors.Is(err, errDecodeTypeDepth) == false {
		t.Fatalf("unfolding %d levels returned %v, expected the type depth limit", maxDecodeTypeDepth+1, err)
	}
}
