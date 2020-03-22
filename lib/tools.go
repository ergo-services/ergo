package lib

import (
	"flag"
	"fmt"
	"io"
	"log"
	"sync"
)

type Buffer struct {
	B        []byte
	original []byte
}

var (
	nTrace              = false
	DefaultBufferLength = 1024
	buffers             = &sync.Pool{
		New: func() interface{} {
			b := &Buffer{
				B: make([]byte, 0, DefaultBufferLength),
			}
			b.original = b.B
			return b
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

func TakeBuffer() *Buffer {
	return buffers.Get().(*Buffer)
}

func ReleaseBuffer(b *Buffer) {
	b.Reset()
	// do not return it to the pool if its grew up too big
	if cap(b.B) > 65536 {
		b.B = nil // for GC
		return
	}
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

func (b *Buffer) WriteTo(w io.Writer) error {
	l := len(b.B)
	if l == 0 {
		return nil
	}

	n, e := w.Write(b.B)
	if l != n {
		return fmt.Errorf("invalid write count")
	}

	if e != nil {
		return e
	}

	b.Reset()
	return nil
}

func (b *Buffer) ReadFrom(r io.Reader) (int, error) {
	capB := cap(b.B)
	lenB := len(b.B)
	// do panic if buffer becomes too large
	if lenB > 4294967000 {
		panic(ErrTooLarge)
	}
	if capB-lenB < capB>>1 {
		// less than (almost) 50% space left. increase capacity
		b.increase()
	}
	n, e := r.Read(b.B[lenB:capB])
	l := lenB + n
	b.B = b.B[:l]
	return n, e
}

func (b *Buffer) increase() {
	cap1 := cap(b.B) * 8
	b1 := make([]byte, 0, cap1)
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
