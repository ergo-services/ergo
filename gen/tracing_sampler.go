package gen

import (
	"fmt"
	"sync/atomic"
	"time"
)

// TracingSampler decides whether to start a new trace for an outgoing message.
// Only consulted when there is no active propagating trace.
type TracingSampler interface {
	Sample() bool
	String() string
}

// TracingSamplerAlways is a sampler that always starts a trace.
var TracingSamplerAlways TracingSampler = &samplerAlways{}

// TracingSamplerDisable is a sampler that never starts a trace.
// Default for all processes.
var TracingSamplerDisable TracingSampler = &samplerDisable{}

type samplerAlways struct{}

func (s *samplerAlways) Sample() bool   { return true }
func (s *samplerAlways) String() string { return "always" }

type samplerDisable struct{}

func (s *samplerDisable) Sample() bool   { return false }
func (s *samplerDisable) String() string { return "disable" }

// TracingSamplerRatio returns a sampler that traces the given fraction of messages.
// Rate must be between 0.0 and 1.0.
func TracingSamplerRatio(rate float64) TracingSampler {
	if rate >= 1 {
		return TracingSamplerAlways
	}
	if rate <= 0 {
		return TracingSamplerDisable
	}
	step := uint64(rate * (1 << 32))
	if step == 0 {
		step = 1
	}
	return &samplerRatio{
		step: step,
		rate: rate,
	}
}

type samplerRatio struct {
	counter uint64
	step    uint64
	rate    float64
}

func (s *samplerRatio) Sample() bool {
	// fixed-point accumulator: add the rate (scaled by 2^32) each call and
	// sample whenever the integer part crosses, yielding ~rate of calls.
	n := atomic.AddUint64(&s.counter, s.step)
	return n>>32 != (n-s.step)>>32
}

func (s *samplerRatio) String() string {
	return fmt.Sprintf("ratio(%g)", s.rate)
}

// TracingSamplerRateLimit returns a sampler that allows at most perSecond traces per second.
func TracingSamplerRateLimit(perSecond int) TracingSampler {
	if perSecond <= 0 {
		return TracingSamplerDisable
	}
	now := uint32(time.Now().Unix())
	return &samplerRateLimit{
		state:     int64(now)<<32 | int64(uint32(perSecond)),
		max:       int64(perSecond),
		perSecond: perSecond,
	}
}

// state packs the current second (high 32 bits) with the tokens left in it (low
// 32 bits); Sample advances both in one CAS, so at most perSecond traces start per second.
type samplerRateLimit struct {
	state     int64
	max       int64
	perSecond int
}

func (s *samplerRateLimit) Sample() bool {
	now := uint32(time.Now().Unix())
	for {
		old := atomic.LoadInt64(&s.state)
		tokens := s.max
		if uint32(old>>32) == now {
			tokens = int64(uint32(old))
		}
		if tokens <= 0 {
			return false
		}
		newState := int64(now)<<32 | int64(uint32(tokens-1))
		if atomic.CompareAndSwapInt64(&s.state, old, newState) {
			return true
		}
	}
}

func (s *samplerRateLimit) String() string {
	return fmt.Sprintf("rate_limit(%d/s)", s.perSecond)
}
