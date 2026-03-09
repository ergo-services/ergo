package lib

import (
	"bufio"
	"io"
	"sync"
	"time"
)

const (
	latency time.Duration = 300 * time.Nanosecond
)

func NewFlusherWithKeepAlive(w io.Writer, keepalive []byte, keepalivePeriod time.Duration) io.Writer {
	f := &flusher{
		writer: bufio.NewWriter(w),
	}
	if closer, ok := w.(io.Closer); ok {
		f.conn = closer
	}
	callback := func() {
		f.Lock()
		defer f.Unlock()

		if f.pending == false {
			// nothing to write. send keepalive.
			f.writer.Write(keepalive)
			if err := f.writer.Flush(); err != nil {
				f.err = err
				if f.conn != nil {
					f.conn.Close()
				}
				return
			}

			f.timer.Reset(keepalivePeriod)
			return
		}

		if err := f.writer.Flush(); err != nil {
			f.err = err
			if f.conn != nil {
				f.conn.Close()
			}
			return
		}
		f.pending = false
		f.timer.Reset(keepalivePeriod)
	}
	f.Lock()
	f.timer = time.AfterFunc(latency*10, callback)
	f.Unlock()

	return f
}

func NewFlusher(w io.Writer) io.Writer {
	f := &flusher{
		writer: bufio.NewWriter(w),
	}
	if closer, ok := w.(io.Closer); ok {
		f.conn = closer
	}
	callback := func() {
		f.Lock()
		defer f.Unlock()

		if f.pending == false {
			return
		}

		if err := f.writer.Flush(); err != nil {
			f.err = err
			if f.conn != nil {
				f.conn.Close()
			}
			return
		}
		f.pending = false
		f.timer.Reset(latency)
	}
	f.Lock()
	f.timer = time.AfterFunc(latency, callback)
	f.Unlock()
	return f
}

type flusher struct {
	sync.Mutex
	timer   *time.Timer
	writer  *bufio.Writer
	pending bool
	err     error
	conn    io.Closer
}

func (f *flusher) Write(b []byte) (n int, err error) {
	f.Lock()
	defer f.Unlock()

	if f.err != nil {
		return 0, f.err
	}

	l := len(b)

	for {
		n, e := f.writer.Write(b)
		if e != nil {
			return n, e
		}
		l -= n
		if l > 0 {
			continue
		}
		break
	}

	if f.pending {
		return len(b), nil
	}

	f.pending = true
	f.timer.Reset(latency)
	return len(b), nil
}

func (f *flusher) Stop() {
	f.Lock()
	defer f.Unlock()
	if f.timer != nil {
		f.timer.Stop()
	}
}
