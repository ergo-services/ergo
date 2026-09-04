package proto

import (
	"sync"
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
	"ergo.services/ergo/net/edf"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/mock"
)

// newRecvConn builds a receiving connection backed by a recording core, so a frame
// fed into the receive path can be asserted as the core route call it triggers.
func newRecvConn(t *testing.T) (*connection, *mock.Core) {
	core := mock.NewCoreT(t)
	rc := &connection{
		peer:          "peer@localhost",
		peer_creation: testPeerCreation,
		core:          core,
		log:           mock.NewLog(),
		decodeOptions: edf.Options{Cache: new(sync.Map)},
	}
	return rc, core
}

// feedFrame pushes one wire frame through the synchronous receive handler.
func feedFrame(rc *connection, frame []byte) {
	q := lib.NewQueueMPSC()
	buf := lib.TakeBuffer()
	buf.B = append(buf.B, frame...)
	q.Push(buf)
	rc.handleRecvQueue(q, 0)
}

// senderPID is the from PID a received frame is attributed to (peer node + the sent id).
func senderPID(id uint64) gen.PID {
	return gen.PID{Node: "peer@localhost", ID: id, Creation: testPeerCreation}
}

func TestRecvSendPID(t *testing.T) {
	tc := newTestConn(t, gen.NetworkFlags{})
	check.NoError(t, tc.c.SendPID(localPID(5), peerPID(9), gen.MessageOptions{}, "hello"))
	frame := tc.readRawFrame(t)

	rc, core := newRecvConn(t)
	feedFrame(rc, frame)

	core.ShouldDeliver().From(senderPID(5)).Message("hello").Once().Assert()
}

func TestRecvSendProcessID(t *testing.T) {
	tc := newTestConn(t, gen.NetworkFlags{})
	to := gen.ProcessID{Name: "worker", Node: "peer@localhost"}
	check.NoError(t, tc.c.SendProcessID(localPID(5), to, gen.MessageOptions{}, "hello"))
	frame := tc.readRawFrame(t)

	rc, core := newRecvConn(t)
	feedFrame(rc, frame)

	core.ShouldDeliver().From(senderPID(5)).Message("hello").Once().Assert()
}

func TestRecvSendAlias(t *testing.T) {
	tc := newTestConn(t, gen.NetworkFlags{})
	to := gen.Alias{Node: "peer@localhost", Creation: testPeerCreation, ID: [3]uint64{11, 22, 33}}
	check.NoError(t, tc.c.SendAlias(localPID(5), to, gen.MessageOptions{}, "hello"))
	frame := tc.readRawFrame(t)

	rc, core := newRecvConn(t)
	feedFrame(rc, frame)

	core.ShouldDeliver().From(senderPID(5)).Message("hello").Once().Assert()
}

func TestRecvSendExit(t *testing.T) {
	tc := newTestConn(t, gen.NetworkFlags{})
	check.NoError(t, tc.c.SendExit(localPID(5), peerPID(9), gen.TerminateReasonNormal))
	frame := tc.readRawFrame(t)

	rc, core := newRecvConn(t)
	feedFrame(rc, frame)

	core.ShouldReceiveExit().About(senderPID(5)).Once().Assert()
}

// a frame that decodes to nothing usable is dropped without reaching the core.
func TestRecvMalformedPIDIsDropped(t *testing.T) {
	rc, core := newRecvConn(t)

	frame := make([]byte, 20) // shorter than a PID message requires
	frame[7] = protoMessagePID
	feedFrame(rc, frame)

	core.ShouldDeliver().None().Assert()
}

// an unknown wire message-type byte is dropped by the receive handler.
func TestRecvUnknownTypeIgnored(t *testing.T) {
	rc, core := newRecvConn(t)
	frame := make([]byte, 20)
	frame[7] = 255 // no such message type
	feedFrame(rc, frame)
	core.ShouldDeliver().None().Assert()
}
