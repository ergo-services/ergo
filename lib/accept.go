package lib

import (
	"errors"
	"syscall"
	"time"
)

const (
	acceptBackoffFirst = 5 * time.Millisecond
	acceptBackoffMax   = time.Second
)

// AcceptExhausted reports whether an accept failed for want of what the system
// gives a socket: descriptors (EMFILE, ENFILE), buffers (ENOBUFS), memory (ENOMEM).
// The listener stays usable: the caller waits (AcceptBackoff) and accepts again.
func AcceptExhausted(err error) bool {
	return errors.Is(err, syscall.EMFILE) ||
		errors.Is(err, syscall.ENFILE) ||
		errors.Is(err, syscall.ENOBUFS) ||
		errors.Is(err, syscall.ENOMEM)
}

// AcceptBackoff is the wait of a listener after an exhausted accept: 5 ms doubling
// up to 1 s, as net/http serves.
type AcceptBackoff struct {
	delay time.Duration
}

// Next returns the wait before the next accept.
func (b *AcceptBackoff) Next() time.Duration {
	if b.delay == 0 {
		b.delay = acceptBackoffFirst
	} else {
		b.delay = min(b.delay*2, acceptBackoffMax)
	}
	return b.delay
}

// Reset ends the wait: an accept went through.
func (b *AcceptBackoff) Reset() {
	b.delay = 0
}
