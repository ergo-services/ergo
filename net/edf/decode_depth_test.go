package edf

import (
	"errors"
	"testing"
)

// nestingLevel is one level of a value nested through any: the folded []any type
// followed by a slice holding a single element.
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

// A packet nesting deeper than the decoder follows is refused, instead of
// spending a stack frame per level until the process dies.
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
