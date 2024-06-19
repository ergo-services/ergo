package lib

import (
	"bufio"
	"io"
	"net"
	"sync"
	"time"
)

const (
	latency time.Duration = 300 * time.Nanosecond
)

func NewFlusherWithKeepAlive(conn net.Conn, keepalive []byte, keepalivePeriod time.Duration) io.Writer {
	f := &flusher{
		writer: bufio.NewWriter(conn),
	}
	// first time it should be longer
	f.timer = time.AfterFunc(latency*10, func() {
		f.Lock()
		defer f.Unlock()

		if f.pending == false {
			// nothing to write. send keepalive.
			f.writer.Write(keepalive)
			if err := f.writer.Flush(); err != nil {
				return
			}

			f.timer.Reset(keepalivePeriod)
			return
		}

		f.writer.Flush()
		f.pending = false
		f.timer.Reset(latency)
	})

	return f

}

func NewFlusher(conn net.Conn) io.Writer {
	f := &flusher{
		writer: bufio.NewWriter(conn),
	}
	f.timer = time.AfterFunc(latency, func() {
		f.Lock()
		defer f.Unlock()

		if f.pending == false {
			// nothing to write
			return
		}

		f.writer.Flush()
		f.pending = false
		f.timer.Reset(latency)
	})
	return f
}

type flusher struct {
	sync.Mutex
	timer   *time.Timer
	writer  *bufio.Writer
	pending bool
}

func (f *flusher) Write(b []byte) (n int, err error) {
	f.Lock()
	defer f.Unlock()

	l := len(b)

	// write data to the buffer
	for {
		n, e := f.writer.Write(b)
		if e != nil {
			return n, e
		}
		// check if something left
		l -= n
		if l > 0 {
			continue
		}
		break
	}

	if f.pending {
		return len(b), nil
	}

	// if f.writer.Size() > 65000 {
	// 	f.writer.Flush()
	// 	return len(b), nil
	// }

	f.pending = true
	f.timer.Reset(latency)
	return len(b), nil
}
