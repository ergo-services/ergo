package proto

import (
	"encoding/binary"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
	"ergo.services/ergo/net/edf"
)

// testPeerCreation is the peer incarnation a testConn is built for; targets must
// carry it or the Send* methods reject them with ErrProcessIncarnation.
const testPeerCreation int64 = 1000

// testConn is a connection wired for the send path: its single pool item writes
// through a real flusher into one end of a net.Pipe, and the test reads framed
// output from the other end with readFrame.
type testConn struct {
	c    *connection
	read net.Conn
}

// newTestConn builds a connection ready to encode and send. peerFlags lets a test
// enable peer-side features (e.g. EnableImportantDelivery).
func newTestConn(t *testing.T, peerFlags gen.NetworkFlags) *testConn {
	t.Helper()
	srv, cli := net.Pipe()
	c := &connection{
		peer:          "peer@localhost",
		peer_creation: testPeerCreation,
		peer_flags:    peerFlags,
		encodeOptions: edf.Options{Cache: new(sync.Map)},
		decodeOptions: edf.Options{Cache: new(sync.Map)},
		requests:      make(map[gen.Ref]chan MessageResult),
	}
	pi := &pool_item{fl: lib.NewFlusher(srv)}
	pi.connection = srv
	c.pool = append(c.pool, pi)
	t.Cleanup(func() {
		c.terminated.Store(true)
		pi.fl.Stop()
		srv.Close()
		cli.Close()
	})
	return &testConn{c: c, read: cli}
}

// readFrame reads one wire frame and returns its order byte, message-type byte,
// and the body (everything after the 8-byte frame header, i.e. from the sender id
// onward). It fails the test on a malformed prefix or a read timeout.
func (tc *testConn) readFrame(t *testing.T) (order, mtype byte, body []byte) {
	t.Helper()
	tc.read.SetReadDeadline(time.Now().Add(2 * time.Second))

	hdr := make([]byte, 8)
	if _, err := io.ReadFull(tc.read, hdr); err != nil {
		t.Fatalf("read frame header: %s", err)
	}
	if hdr[0] != protoMagic || hdr[1] != protoVersion {
		t.Fatalf("bad frame prefix: magic=%d version=%d", hdr[0], hdr[1])
	}
	total := int(binary.BigEndian.Uint32(hdr[2:6]))
	body = make([]byte, total-8)
	if _, err := io.ReadFull(tc.read, body); err != nil {
		t.Fatalf("read frame body: %s", err)
	}
	return hdr[6], hdr[7], body
}

// readRawFrame reads one whole wire frame (header and body), suitable for feeding
// straight back into the receive path.
func (tc *testConn) readRawFrame(t *testing.T) []byte {
	t.Helper()
	tc.read.SetReadDeadline(time.Now().Add(2 * time.Second))

	hdr := make([]byte, 6)
	if _, err := io.ReadFull(tc.read, hdr); err != nil {
		t.Fatalf("read frame header: %s", err)
	}
	total := int(binary.BigEndian.Uint32(hdr[2:6]))
	rest := make([]byte, total-6)
	if _, err := io.ReadFull(tc.read, rest); err != nil {
		t.Fatalf("read frame body: %s", err)
	}
	return append(hdr, rest...)
}

// decode reads the trailing edf-encoded message from a frame body slice.
func (tc *testConn) decode(t *testing.T, b []byte) any {
	t.Helper()
	v, _, err := edf.Decode(b, tc.c.decodeOptions)
	if err != nil {
		t.Fatalf("edf decode: %s", err)
	}
	return v
}
