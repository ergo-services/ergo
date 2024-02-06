package lib

import (
	"sync"
	"time"
)

var (
	timers = &sync.Pool{
		New: func() interface{} {
			return time.NewTimer(time.Second * 5)
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
