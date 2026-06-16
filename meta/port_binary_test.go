package meta

import (
	"bytes"
	"fmt"
	"io"
	"testing"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/mock"
)

// newPortMeta returns a meta-process mock whose Send hands the message to the
// returned channel (consumed by waitForPortData), and whose logger panics on
// Error/Panic so an unexpected port error surfaces loudly.
func newPortMeta() (gen.MetaProcess, chan any) {
	result := make(chan any)

	mp := mock.NewMeta()
	mp.OnSend(func(to, message any) error {
		select {
		case result <- message:
		case <-time.After(100 * time.Millisecond):
			panic("no reader")
		}
		return nil
	})

	lg := mock.NewLog()
	lg.OnError(func(format string, args ...any) { panic(fmt.Sprintf(format, args...)) })
	lg.OnPanic(func(format string, args ...any) { panic(fmt.Sprintf(format, args...)) })
	mp.OnLog(func() gen.Log { return lg })

	return mp, result
}

func waitForPortData(result chan any, expecting []byte) error {
	select {
	case r := <-result:
		data, ok := r.(MessagePortData)
		if ok == false {
			fmt.Printf("got incorrect result (expected MessagePortData): %#v\n", r)
			return gen.ErrIncorrect
		}
		if bytes.Equal(data.Data, expecting) == false {
			fmt.Printf("got incorrect data (expected %#v): %#v\n", expecting, data.Data)
			return gen.ErrMalformed
		}
		return nil
	case <-time.After(100 * time.Millisecond):
		return gen.ErrTimeout
	}
}

func TestPortBinaryWithHeader(t *testing.T) {
	r, w := io.Pipe()
	p := port{
		out: r,
	}
	mp, result := newPortMeta()

	p.MetaProcess = mp
	p.options.Binary.Enable = true
	p.options.Binary.ReadChunk.Enable = true
	p.options.Binary.ReadChunk.HeaderSize = 3
	p.options.Binary.ReadChunk.HeaderLengthSize = 1
	p.options.Binary.ReadChunk.HeaderLengthPosition = 2
	p.options.Binary.ReadBufferSize = 50

	go func() {
		p.readStdoutDataChunk("x")
	}()

	//            chunk1......  chunk2................  chunk3...........
	buf := []byte{0, 0, 1, 100, 0, 0, 3, 101, 102, 103, 0, 0, 2, 104, 105}
	//            buf1...........  buf2..........  buf3..................
	buf1 := buf[:5]
	buf2 := buf[5:9]
	buf3 := buf[9:]

	w.Write(buf1) // chunk1 + tail
	if err := waitForPortData(result, []byte{0, 0, 1, 100}); err != nil {
		panic(err)
	}

	w.Write(buf2) // not enough for the chunk2
	if err := waitForPortData(result, []byte{}); err != gen.ErrTimeout {
		panic("malformed")
	}

	w.Write(buf3) // expecting chunk2 and chunk3
	if err := waitForPortData(result, []byte{0, 0, 3, 101, 102, 103}); err != nil {
		panic(err)
	}
	if err := waitForPortData(result, []byte{0, 0, 2, 104, 105}); err != nil {
		panic(err)
	}
	if err := waitForPortData(result, []byte{}); err != gen.ErrTimeout {
		panic("malformed. must be timeout here")
	}
}

func TestPortBinaryFixedLength(t *testing.T) {
	r, w := io.Pipe()
	p := port{
		out: r,
	}
	mp, result := newPortMeta()

	p.MetaProcess = mp
	p.options.Binary.Enable = true
	p.options.Binary.ReadChunk.Enable = true
	p.options.Binary.ReadChunk.FixedLength = 3
	p.options.Binary.ReadBufferSize = 50

	go func() {
		p.readStdoutDataChunk("x")
	}()

	//            chunk1.......  chunk2.......  chunk3.......
	buf := []byte{100, 101, 102, 103, 104, 105, 106, 107, 108}
	//            buf1....  buf2.......... buf3..............
	buf1 := buf[:2]
	buf2 := buf[2:5]
	buf3 := buf[5:]

	w.Write(buf1) // not enough for the chunk1
	if err := waitForPortData(result, []byte{}); err != gen.ErrTimeout {
		panic(err)
	}

	w.Write(buf2) // expecting chunk1
	if err := waitForPortData(result, []byte{100, 101, 102}); err != nil {
		panic(err)
	}
	// not enough for the chunk2
	if err := waitForPortData(result, []byte{103, 104, 105}); err != gen.ErrTimeout {
		panic("malformed")
	}

	w.Write(buf3) // expecting chunk2 and chunk3
	if err := waitForPortData(result, []byte{103, 104, 105}); err != nil {
		panic(err)
	}
	if err := waitForPortData(result, []byte{106, 107, 108}); err != nil {
		panic(err)
	}
	if err := waitForPortData(result, []byte{}); err != gen.ErrTimeout {
		panic("malformed. must be timeout here")
	}
}
