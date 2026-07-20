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
	return &samplerRateLimit{
		tokens:    int64(perSecond),
		max:       int64(perSecond),
		lastTick:  time.Now().Unix(),
		perSecond: perSecond,
	}
}

type samplerRateLimit struct {
	tokens    int64
	max       int64
	lastTick  int64
	perSecond int
}

func (s *samplerRateLimit) Sample() bool {
	now := time.Now().Unix()
	last := atomic.LoadInt64(&s.lastTick)
	if now > last {
		if atomic.CompareAndSwapInt64(&s.lastTick, last, now) {
			atomic.StoreInt64(&s.tokens, s.max)
		}
	}
	return atomic.AddInt64(&s.tokens, -1) >= 0
}

func (s *samplerRateLimit) String() string {
	return fmt.Sprintf("rate_limit(%d/s)", s.perSecond)
}
