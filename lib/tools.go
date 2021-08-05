package lib

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"sync"
	"time"
)

type Buffer struct {
	B        []byte
	original []byte
}

var (
	nTrace              = false
	DefaultBufferLength = 16384
	buffers             = &sync.Pool{
		New: func() interface{} {
			b := &Buffer{
				B: make([]byte, 0, DefaultBufferLength),
			}
			b.original = b.B
			return b
		},
	}

	timers = &sync.Pool{
		New: func() interface{} {
			return time.NewTimer(time.Second * 5)
		},
	}

	ErrTooLarge = fmt.Errorf("Too large")
)

func init() {
	flag.BoolVar(&nTrace, "trace.node", false, "trace node")
}

func Log(f string, a ...interface{}) {
	if nTrace {
		log.Printf(f, a...)
	}
}

func TakeTimer() *time.Timer {
	return timers.Get().(*time.Timer)
}

func ReleaseTimer(t *time.Timer) {
	t.Stop()
	timers.Put(t)
}

func TakeBuffer() *Buffer {
	return buffers.Get().(*Buffer)
}

func ReleaseBuffer(b *Buffer) {
	// do not return it to the pool if its grew up too big
	if cap(b.B) > 65536 {
		b.B = nil // for GC
		b.original = nil
		return
	}
	b.B = b.original[:0]
	buffers.Put(b)
}

func (b *Buffer) Reset() {
	// use the original start point of the slice
	b.B = b.original[:0]
}

func (b *Buffer) Set(v []byte) {
	b.B = append(b.B[:0], v...)
}

func (b *Buffer) AppendByte(v byte) {
	b.B = append(b.B, v)
}

func (b *Buffer) Append(v []byte) {
	b.B = append(b.B, v...)
}

func (b *Buffer) String() string {
	return string(b.B)
}

func (b *Buffer) Len() int {
	return len(b.B)
}

func (b *Buffer) WriteDataTo(w io.Writer) error {
	l := len(b.B)
	if l == 0 {
		return nil
	}

	for {
		n, e := w.Write(b.B)
		if e != nil {
			return e
		}

		l -= n
		if l > 0 {
			continue
		}

		break
	}

	b.Reset()
	return nil
}

func (b *Buffer) ReadDataFrom(r io.Reader, limit int) (int, error) {
	capB := cap(b.B)
	lenB := len(b.B)
	if limit == 0 {
		limit = 4294967000
	}
	// if buffer becomes too large
	if lenB > limit {
		return 0, ErrTooLarge
	}
	if capB-lenB < capB>>1 {
		// less than (almost) 50% space left. increase capacity
		b.increase()
		capB = cap(b.B)
	}
	n, e := r.Read(b.B[lenB:capB])
	l := lenB + n
	b.B = b.B[:l]
	return n, e
}

func (b *Buffer) increase() {
	cap1 := cap(b.B) * 8
	b1 := make([]byte, cap(b.B), cap1)
	copy(b1, b.B)
	b.B = b1
}

func (b *Buffer) Allocate(n int) {
	for {
		if cap(b.B) < n {
			b.increase()
			continue
		}
		b.B = b.B[:n]
		return
	}
}

func (b *Buffer) Extend(n int) []byte {
	l := len(b.B)
	e := l + n
	for {
		if e > cap(b.B) {
			b.increase()
			continue
		}
		b.B = b.B[:e]
		return b.B[l:e]
	}
}

func RandomString(length int) string {
	buff := make([]byte, length)
	rand.Read(buff)
	return hex.EncodeToString(buff)
}
