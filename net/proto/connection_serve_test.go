package proto

import (
	"encoding/binary"
	"net"
	"sync"
	"testing"
	"time"

	"ergo.services/ergo/lib"
	"ergo.services/ergo/net/edf"
	"ergo.services/ergo/testing/mock"
)

// frameHdr builds an 8-byte frame header of total length l carrying the given
// order and message-type byte.
func frameHdr(order, mtype byte, l uint32) []byte {
	h := []byte{protoMagic, protoVersion, 0, 0, 0, 0, order, mtype}
	binary.BigEndian.PutUint32(h[2:6], l)
	return h
}

// runServe drives connection.serve over an in-memory pipe: it writes each frame,
// then closes the writer so read() sees EOF and serve returns. It returns the
// recording core once serve has exited (or fails the test on timeout).
func runServe(t *testing.T, frames ...[]byte) *mock.Core {
	t.Helper()
	srv, cli := net.Pipe()
	core := mock.NewCoreT(t)
	c := &connection{
		peer:          "peer@localhost",
		peer_creation: testPeerCreation,
		core:          core,
		log:           mock.NewLog(),
		decodeOptions: edf.Options{Cache: new(sync.Map)},
		recvQueues:    []lib.QueueMPSC{lib.NewQueueMPSC()},
	}
	pi := &pool_item{fl: lib.NewFlusher(srv)}
	pi.connection.Store(&poolConn{srv})

	done := make(chan struct{})
	go func() {
		c.serve(pi, nil)
		close(done)
	}()
	go func() {
		for _, f := range frames {
			cli.SetWriteDeadline(time.Now().Add(2 * time.Second))
			cli.Write(f)
		}
		cli.Close()
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("serve did not return")
	}
	pi.fl.Stop()
	srv.Close()
	return core
}

// A frame with the wrong magic byte tears the connection down without delivering.
func TestServeRejectsBadMagic(t *testing.T) {
	frame := frameHdr(0, protoMessagePID, 8)
	frame[0] = protoMagic + 1
	core := runServe(t, frame)
	core.ShouldDeliver().None().Assert()
}

// A frame with an unexpected protocol version is rejected the same way.
func TestServeRejectsBadVersion(t *testing.T) {
	frame := frameHdr(0, protoMessagePID, 8)
	frame[1] = protoVersion + 1
	core := runServe(t, frame)
	core.ShouldDeliver().None().Assert()
}

// A keepalive frame is consumed silently and never reaches the core.
func TestServeSkipsKeepalive(t *testing.T) {
	core := runServe(t, frameHdr(0, protoMessageK, 8))
	core.ShouldDeliver().None().Assert()
}

// A skew frame shorter than the fixed 32-byte skew payload is dropped.
func TestServeRejectsShortSkew(t *testing.T) {
	core := runServe(t, frameHdr(0, protoMessageS, 8))
	core.ShouldDeliver().None().Assert()
}

// read() rejects a frame whose declared length exceeds the configured limit.
func TestReadRejectsOversizeFrame(t *testing.T) {
	client, server := net.Pipe()
	go func() {
		client.Write(frameHdr(0, protoMessagePID, 64))
		client.Close()
	}()
	defer server.Close()

	c := &connection{node_maxmessagesize: 16}
	buf := lib.TakeBuffer()
	if _, err := c.read(server, buf); err == nil {
		t.Fatal("expected an error for a frame exceeding node_maxmessagesize, got nil")
	}
}
