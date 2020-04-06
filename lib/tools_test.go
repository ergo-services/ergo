package lib

import (
	"testing"
)

func TestBuffer(t *testing.T) {
	b := TakeBuffer()

	if cap(b.B) != DefaultBufferLength {
		t.Fatal("incorrect capacity")
	}

	if len(b.B) != 0 {
		t.Fatal("should be zero length")
	}

}
