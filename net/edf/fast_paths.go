package edf

import (
	"encoding/binary"
	"math"

	"ergo.services/ergo/lib"
)

// Fast path optimizations for common types
// These functions bypass some reflection overhead and use optimized buffer operations

// encodeStringFast optimizes string encoding with batched buffer operations
func encodeStringFast(s string, b *lib.Buffer, encodeType bool) error {
	lenString := len(s)
	if lenString > math.MaxUint16 {
		return ErrStringTooLong
	}

	var totalSize int
	if encodeType {
		totalSize = 1 + 2 + lenString // type + length + data
	} else {
		totalSize = 2 + lenString // length + data
	}

	// Single buffer extension instead of multiple small ones
	buf := b.Extend(totalSize)
	offset := 0

	if encodeType {
		buf[offset] = edtString
		offset++
	}

	// Write length
	binary.BigEndian.PutUint16(buf[offset:], uint16(lenString))
	offset += 2

	// Write string data
	copy(buf[offset:], s)
	return nil
}

// encodeInt64Fast optimizes int64 encoding with single buffer operation
func encodeInt64Fast(value int64, b *lib.Buffer, encodeType bool) error {
	var totalSize int
	if encodeType {
		totalSize = 1 + 8 // type + data
	} else {
		totalSize = 8 // data only
	}

	buf := b.Extend(totalSize)
	offset := 0

	if encodeType {
		buf[offset] = edtInt64
		offset++
	}

	// Write int64 value directly
	binary.BigEndian.PutUint64(buf[offset:], uint64(value))
	return nil
}

// encodeBinaryFast optimizes binary encoding with single buffer operation
func encodeBinaryFast(data []byte, b *lib.Buffer, encodeType bool) error {
	lenBinary := len(data)
	if lenBinary > math.MaxUint32 {
		return ErrBinaryTooLong
	}

	var totalSize int
	if encodeType {
		totalSize = 1 + 4 + lenBinary // type + length + data
	} else {
		totalSize = 4 + lenBinary // length + data
	}

	buf := b.Extend(totalSize)
	offset := 0

	if encodeType {
		buf[offset] = edtBinary
		offset++
	}

	// Write length
	binary.BigEndian.PutUint32(buf[offset:], uint32(lenBinary))
	offset += 4

	// Write binary data
	copy(buf[offset:], data)
	return nil
}

// Optimized child state creation for nested structures
func (state *stateEncode) getChildState() *stateEncode {
	if state.child == nil {
		state.child = getPooledStateEncode(state.options)
	}
	return state.child
}

func (state *stateDecode) getChildState() *stateDecode {
	if state.child == nil {
		state.child = getPooledStateDecode(state.options)
	}
	return state.child
}
