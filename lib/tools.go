package lib

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"sync"
	"time"
)

// Buffer
type Buffer struct {
	B        []byte
	original []byte
}

var (
	ergoTrace     = false
	ergoNoRecover = false

	DefaultBufferLength = 16384
	buffers             = &sync.Pool{
		New: func() interface{} {
			return &bytes.Buffer{}
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
	flag.BoolVar(&ergoTrace, "ergo.trace", false, "enable extended debug info")
	flag.BoolVar(&ergoNoRecover, "ergo.norecover", false, "disable panic catching")
}

// Log
func Log(f string, a ...interface{}) {
	if ergoTrace {
		log.Printf(f, a...)
	}
}

// CatchPanic
func CatchPanic() bool {
	return ergoNoRecover == false
}

// TakeTimer
func TakeTimer() *time.Timer {
	return timers.Get().(*time.Timer)
}

// ReleaseTimer
func ReleaseTimer(t *time.Timer) {
	t.Stop()
	timers.Put(t)
}

// TakeBuffer
func TakeBuffer() *bytes.Buffer {
	b := buffers.Get().(*bytes.Buffer)
	b.Reset()
	return b
}

// ReleaseBuffer
func ReleaseBuffer(b *bytes.Buffer) {
	// do not return it to the pool if its grew up too big
	if b.Cap() > 65536 {
		return
	}
	buffers.Put(b)
}

// RandomString
func RandomString(length int) string {
	buff := make([]byte, length/2)
	rand.Read(buff)
	return hex.EncodeToString(buff)
}
