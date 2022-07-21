package dist

import (
	"bufio"
	"io"
	"sync"
	"time"
)

var (
	// KeepAlive packet is just 4 bytes with zero value
	keepAlivePacket = []byte{0, 0, 0, 0}
	keepAlivePeriod = 15 * time.Second
)

func newLinkFlusher(w io.Writer, latency time.Duration) *linkFlusher {
	lf := &linkFlusher{
		latency: latency,
		writer:  bufio.NewWriter(w),
		w:       w, // in case if we skip buffering
	}

	lf.timer = time.AfterFunc(keepAlivePeriod, func() {

		lf.mutex.Lock()
		defer lf.mutex.Unlock()

		// if we have no pending data to send we should
		// send a KeepAlive packet
		if lf.pending == false {
			lf.w.Write(keepAlivePacket)
			lf.timer.Reset(keepAlivePeriod)
			return
		}

		lf.writer.Flush()
		lf.pending = false
		lf.timer.Reset(keepAlivePeriod)
	})

	return lf
}

type linkFlusher struct {
	mutex   sync.Mutex
	latency time.Duration
	writer  *bufio.Writer
	w       io.Writer

	timer   *time.Timer
	pending bool
}

func (lf *linkFlusher) Write(b []byte) (int, error) {
	lf.mutex.Lock()
	defer lf.mutex.Unlock()

	l := len(b)
	lenB := l

	// long data write directly to the socket.
	if l > 64000 {
		for {
			n, e := lf.w.Write(b[lenB-l:])
			if e != nil {
				return n, e
			}
			// check if something left
			l -= n
			if l > 0 {
				continue
			}
			return lenB, nil
		}
	}

	// write data to the buffer
	for {
		n, e := lf.writer.Write(b)
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

	if lf.pending {
		return lenB, nil
	}

	lf.pending = true
	lf.timer.Reset(lf.latency)

	return lenB, nil
}

func (lf *linkFlusher) Stop() {
	if lf.timer != nil {
		lf.timer.Stop()
	}
}
