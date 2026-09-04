package lib

import (
	"sync"
	"time"
)

var (
	timers = &sync.Pool{
		New: func() any {
			// Return a stopped timer with empty C so the first Reset by the
			// caller is race-free per time.Timer.Reset contract. NewTimer(1h)
			// then Stop() runs within nanoseconds; Stop is guaranteed to win.
			t := time.NewTimer(time.Hour)
			t.Stop()
			return t
		},
	}
)

// TakeTimer
func TakeTimer() *time.Timer {
	return timers.Get().(*time.Timer)
}

// ReleaseTimer
func ReleaseTimer(t *time.Timer) {
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
	timers.Put(t)
}
