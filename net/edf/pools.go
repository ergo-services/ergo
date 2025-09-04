package edf

import (
	"sync"
)

// Object pools to reduce allocations in hot paths
var (
	// State pools for encode/decode operations
	stateEncodePool = sync.Pool{
		New: func() any {
			return &stateEncode{}
		},
	}

	stateDecodePool = sync.Pool{
		New: func() any {
			return &stateDecode{}
		},
	}
)

// getPooledStateEncode returns a clean state from pool
func getPooledStateEncode(options Options) *stateEncode {
	state := stateEncodePool.Get().(*stateEncode)
	// Reset state to clean condition
	state.child = nil
	state.encodeType = false
	state.options = options
	return state
}

// putPooledStateEncode returns state to pool after cleaning
func putPooledStateEncode(state *stateEncode) {
	// Recursively return child states to pool
	if state.child != nil {
		putPooledStateEncode(state.child)
		state.child = nil
	}
	// Clear options to prevent memory leaks
	state.options = Options{}
	stateEncodePool.Put(state)
}

// getPooledStateDecode returns a clean decode state from pool
func getPooledStateDecode(options Options) *stateDecode {
	state := stateDecodePool.Get().(*stateDecode)
	// Reset state to clean condition
	state.child = nil
	state.decodeType = false
	state.options = options
	state.decoder = nil
	return state
}

// putPooledStateDecode returns decode state to pool after cleaning
func putPooledStateDecode(state *stateDecode) {
	// Recursively return child states to pool
	if state.child != nil {
		putPooledStateDecode(state.child)
		state.child = nil
	}
	// Clear options and decoder to prevent memory leaks
	state.options = Options{}
	state.decoder = nil
	stateDecodePool.Put(state)
}
