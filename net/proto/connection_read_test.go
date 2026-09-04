package proto

import (
	"encoding/binary"
	"net"
	"testing"

	"ergo.services/ergo/lib"
)

// header builds an 8-byte frame header declaring total frame length l.
func header(l uint32) []byte {
	h := []byte{protoMagic, protoVersion, 0, 0, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(h[2:6], l)
	return h
}

// readFrame drives connection.read over an in-memory pipe carrying frame.
func readFrame(t *testing.T, frame []byte) (*lib.Buffer, error) {
	t.Helper()
	client, server := net.Pipe()
	go func() {
		client.Write(frame)
		client.Close()
	}()
	defer server.Close()

	c := &connection{}
	buf := lib.TakeBuffer()
	return c.read(server, buf)
}

// A frame declaring a total length below the 8-byte header must be rejected by
// read() instead of returning a short buffer that serve() would index out of
// range (buf.B[7]).
func TestReadRejectsShortFrame(t *testing.T) {
	for _, l := range []uint32{0, 1, 6, 7} {
		if _, err := readFrame(t, header(l)); err == nil {
			t.Fatalf("length %d: expected an error, got nil", l)
		}
	}
}

// The minimal valid frame (header only, length 8) is accepted by read().
func TestReadAcceptsMinimalFrame(t *testing.T) {
	if _, err := readFrame(t, header(8)); err != nil {
		t.Fatalf("length 8: expected no error, got %v", err)
	}
}
