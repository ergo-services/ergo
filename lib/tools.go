package lib

import (
	"bytes"
	"flag"
	"log"
	"sync"
)

var (
	nTrace  bool
	buffers = &sync.Pool{
		New: func() interface{} {
			return &bytes.Buffer{}
		},
	}
)

func init() {
	flag.BoolVar(&nTrace, "trace.node", false, "trace node")
}

func Log(f string, a ...interface{}) {
	if nTrace {
		log.Printf(f, a...)
	}
}

func TakeBuffer() *bytes.Buffer {
	return buffers.Get().(*bytes.Buffer)
}

func ReleaseBuffer(b *bytes.Buffer) {
	b.Reset()
	buffers.Put(b)
}
