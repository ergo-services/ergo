package lib

import (
	"fmt"
	"io"
	"math"
	"sync"
	"sync/atomic"
)

// Buffer
type Buffer struct {
	B        []byte
	original []byte
}

var (
	DefaultBufferLength = 4096
	buffers             = &sync.Pool{
		New: func() interface{} {
			b := &Buffer{
				B: make([]byte, 0, DefaultBufferLength),
			}
			b.original = b.B
			return b
		},
	}
	buffersTaken        uint64 = 0
	buffersTakenSize    uint64 = 0
	buffersReturned     uint64 = 0
	buffersReturnedSize uint64 = 0
)

func StatBuffers() {
	fmt.Printf("taken: %d (size: %d)\n", buffersTaken, buffersTakenSize)
	fmt.Printf("returned: %d (size: %d)\n", buffersReturned, buffersReturnedSize)
}

// TakeBuffer
func TakeBuffer() *Buffer {
	b := buffers.Get().(*Buffer)
	atomic.AddUint64(&buffersTaken, 1)
	atomic.AddUint64(&buffersTakenSize, uint64(cap(b.B)))
	return b
}

// ReleaseBuffer
func ReleaseBuffer(b *Buffer) {
	b.B = b.original[:0]
	atomic.AddUint64(&buffersReturned, 1)
	atomic.AddUint64(&buffersReturnedSize, uint64(cap(b.B)))
	buffers.Put(b)
}

// Reset
func (b *Buffer) Reset() {
	b.B = b.B[:0]
}

// Set
func (b *Buffer) Set(v []byte) {
	if len(v) > cap(b.original) {
		b.B = append(b.B[:0], v...)
		return
	}
	b.B = append(b.original[:0], v...)
}

// AppendByte
func (b *Buffer) AppendByte(v byte) {
	b.B = append(b.B, v)
}

// Append
func (b *Buffer) Append(v []byte) {
	b.B = append(b.B, v...)
}

// AppendString
func (b *Buffer) AppendString(s string) {
	b.B = append(b.B, s...)
}

// String
func (b *Buffer) String() string {
	return string(b.B)
}

// Len
func (b *Buffer) Len() int {
	return len(b.B)
}

func (b *Buffer) Cap() int {
	return cap(b.B)
}

// WriteDataTo
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

	return nil
}

// ReadDataFrom
func (b *Buffer) ReadDataFrom(r io.Reader, limit int) (int, error) {
	capB := cap(b.B)
	lenB := len(b.B)
	if limit == 0 {
		limit = math.MaxInt
	}
	// if buffer becomes too large
	if lenB > limit {
		return 0, fmt.Errorf("too large")
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

func (b *Buffer) Write(v []byte) (n int, err error) {
	b.B = append(b.B, v...)
	return len(v), nil
}

func (b *Buffer) Read(v []byte) (n int, err error) {
	copy(v, b.B)
	return len(b.B), io.EOF
}

func (b *Buffer) increase() {
	cap1 := cap(b.B) * 2
	b1 := make([]byte, cap(b.B), cap1)
	copy(b1, b.B)
	b.B = b1
}

// Allocate
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

// Extend
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
