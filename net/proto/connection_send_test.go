package proto

import (
	"encoding/binary"
	"strings"
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
)

// peerPID builds a target PID carrying the peer incarnation a testConn expects.
func peerPID(id uint64) gen.PID {
	return gen.PID{Node: "peer@localhost", ID: id, Creation: testPeerCreation}
}

func localPID(id uint64) gen.PID {
	return gen.PID{Node: "me@localhost", ID: id, Creation: 1}
}

func TestSendPID(t *testing.T) {
	t.Run("encodes header, ids, priority and payload", func(t *testing.T) {
		tc := newTestConn(t, gen.NetworkFlags{})
		from, to := localPID(5), peerPID(9)

		err := tc.c.SendPID(from, to, gen.MessageOptions{Priority: gen.MessagePriorityHigh}, "hello")
		check.NoError(t, err)

		order, mtype, body := tc.readFrame(t)
		check.Equal(t, protoMessagePID, mtype)
		check.Equal(t, uint8(0), order) // KeepNetworkOrder is off -> order 0
		check.Equal(t, from.ID, binary.BigEndian.Uint64(body[0:8]))
		check.Equal(t, byte(gen.MessagePriorityHigh), body[8])
		check.Equal(t, to.ID, binary.BigEndian.Uint64(body[17:25]))
		check.Equal(t, "hello", tc.decode(t, body[25:]))
	})

	t.Run("KeepNetworkOrder sets the wire order from the target id", func(t *testing.T) {
		tc := newTestConn(t, gen.NetworkFlags{})
		from, to := localPID(5), peerPID(9)

		err := tc.c.SendPID(from, to, gen.MessageOptions{KeepNetworkOrder: true}, "hello")
		check.NoError(t, err)

		order, _, _ := tc.readFrame(t)
		check.Equal(t, uint8(to.ID%255+1), order)
	})

	t.Run("important delivery is rejected when the peer lacks the flag", func(t *testing.T) {
		tc := newTestConn(t, gen.NetworkFlags{})
		err := tc.c.SendPID(localPID(5), peerPID(9), gen.MessageOptions{ImportantDelivery: true}, "hello")
		check.ErrorIs(t, err, gen.ErrUnsupported)
	})

	t.Run("important delivery sets the priority bit and writes the ref", func(t *testing.T) {
		tc := newTestConn(t, gen.NetworkFlags{EnableImportantDelivery: true})
		ref := gen.Ref{Node: "me@localhost", Creation: 1, ID: [3]uint64{42, 0, 0}}

		err := tc.c.SendPID(localPID(5), peerPID(9), gen.MessageOptions{ImportantDelivery: true, Ref: ref}, "hello")
		check.NoError(t, err)

		_, _, body := tc.readFrame(t)
		check.True(t, body[8]&128 != 0) // important bit on the priority byte
		check.Equal(t, ref.ID[0], binary.BigEndian.Uint64(body[9:17]))
	})

	t.Run("rejects a payload larger than the peer limit", func(t *testing.T) {
		tc := newTestConn(t, gen.NetworkFlags{})
		tc.c.peer_maxmessagesize = 1

		err := tc.c.SendPID(localPID(5), peerPID(9), gen.MessageOptions{}, "hello")
		check.ErrorIs(t, err, gen.ErrTooLarge)
	})
}

func TestSendExit(t *testing.T) {
	t.Run("forces max priority and carries from, to and reason", func(t *testing.T) {
		tc := newTestConn(t, gen.NetworkFlags{})
		from, to := localPID(5), peerPID(9)

		err := tc.c.SendExit(from, to, gen.TerminateReasonNormal)
		check.NoError(t, err)

		order, mtype, body := tc.readFrame(t)
		check.Equal(t, protoMessageExit, mtype)
		check.Equal(t, uint8(to.ID%255+1), order)
		check.Equal(t, from.ID, binary.BigEndian.Uint64(body[0:8]))
		check.Equal(t, byte(gen.MessagePriorityMax), body[8])
		check.Equal(t, to.ID, binary.BigEndian.Uint64(body[9:17]))
	})

}

func TestSendResponse(t *testing.T) {
	t.Run("carries to, the full request ref and the response payload", func(t *testing.T) {
		tc := newTestConn(t, gen.NetworkFlags{})
		from, to := localPID(5), peerPID(9)
		ref := gen.Ref{Node: "peer@localhost", Creation: testPeerCreation, ID: [3]uint64{7, 8, 9}}

		err := tc.c.SendResponse(from, to, gen.MessageOptions{Ref: ref}, "pong")
		check.NoError(t, err)

		_, mtype, body := tc.readFrame(t)
		check.Equal(t, protoMessageResponse, mtype)
		check.Equal(t, to.ID, binary.BigEndian.Uint64(body[9:17]))
		check.Equal(t, ref.ID[0], binary.BigEndian.Uint64(body[17:25]))
		check.Equal(t, ref.ID[1], binary.BigEndian.Uint64(body[25:33]))
		check.Equal(t, ref.ID[2], binary.BigEndian.Uint64(body[33:41]))
		check.Equal(t, "pong", tc.decode(t, body[41:]))
	})
}

func TestSendProcessID(t *testing.T) {
	t.Run("encodes the target name inline with the payload", func(t *testing.T) {
		tc := newTestConn(t, gen.NetworkFlags{})
		from := localPID(5)
		to := gen.ProcessID{Name: "worker", Node: "peer@localhost"}

		err := tc.c.SendProcessID(from, to, gen.MessageOptions{Priority: gen.MessagePriorityHigh}, "hello")
		check.NoError(t, err)

		_, mtype, body := tc.readFrame(t)
		check.Equal(t, protoMessageName, mtype)
		check.Equal(t, from.ID, binary.BigEndian.Uint64(body[0:8]))
		check.Equal(t, byte(gen.MessagePriorityHigh), body[8])
		nameLen := int(body[17])
		check.Equal(t, len("worker"), nameLen)
		check.Equal(t, "worker", string(body[18:18+nameLen]))
		check.Equal(t, "hello", tc.decode(t, body[18+nameLen:]))
	})

	t.Run("rejects a name longer than 255 bytes", func(t *testing.T) {
		tc := newTestConn(t, gen.NetworkFlags{})
		to := gen.ProcessID{Name: gen.Atom(strings.Repeat("x", 256)), Node: "peer@localhost"}
		err := tc.c.SendProcessID(localPID(5), to, gen.MessageOptions{}, "hi")
		check.Error(t, err)
	})
}

func TestSendAlias(t *testing.T) {
	t.Run("carries the full alias id and payload", func(t *testing.T) {
		tc := newTestConn(t, gen.NetworkFlags{})
		from := localPID(5)
		to := gen.Alias{Node: "peer@localhost", Creation: testPeerCreation, ID: [3]uint64{11, 22, 33}}

		err := tc.c.SendAlias(from, to, gen.MessageOptions{}, "hello")
		check.NoError(t, err)

		_, mtype, body := tc.readFrame(t)
		check.Equal(t, protoMessageAlias, mtype)
		check.Equal(t, from.ID, binary.BigEndian.Uint64(body[0:8]))
		check.Equal(t, to.ID[0], binary.BigEndian.Uint64(body[17:25]))
		check.Equal(t, to.ID[1], binary.BigEndian.Uint64(body[25:33]))
		check.Equal(t, to.ID[2], binary.BigEndian.Uint64(body[33:41]))
		check.Equal(t, "hello", tc.decode(t, body[41:]))
	})

}

func TestSendEvent(t *testing.T) {
	t.Run("encodes the event name, timestamp and payload", func(t *testing.T) {
		tc := newTestConn(t, gen.NetworkFlags{})
		from := localPID(5)
		ev := gen.MessageEvent{
			Event:     gen.Event{Name: "tick", Node: "peer@localhost"},
			Timestamp: 12345,
			Message:   "payload",
		}

		err := tc.c.SendEvent(from, gen.MessageOptions{}, ev)
		check.NoError(t, err)

		_, mtype, body := tc.readFrame(t)
		check.Equal(t, protoMessageEvent, mtype)
		check.Equal(t, from.ID, binary.BigEndian.Uint64(body[0:8]))
		check.Equal(t, uint64(12345), binary.BigEndian.Uint64(body[9:17]))
		nameLen := int(body[17])
		check.Equal(t, "tick", string(body[18:18+nameLen]))
		check.Equal(t, "payload", tc.decode(t, body[18+nameLen:]))
	})
}

func TestSendResponseError(t *testing.T) {
	t.Run("encodes a known error as a single code byte", func(t *testing.T) {
		tc := newTestConn(t, gen.NetworkFlags{})
		from, to := localPID(5), peerPID(9)
		ref := gen.Ref{Node: "peer@localhost", Creation: testPeerCreation, ID: [3]uint64{7, 8, 9}}

		err := tc.c.SendResponseError(from, to, gen.MessageOptions{Ref: ref}, gen.ErrProcessUnknown)
		check.NoError(t, err)

		_, mtype, body := tc.readFrame(t)
		check.Equal(t, protoMessageResponseError, mtype)
		check.Equal(t, to.ID, binary.BigEndian.Uint64(body[9:17]))
		check.Equal(t, ref.ID[0], binary.BigEndian.Uint64(body[17:25]))
		check.Equal(t, byte(1), body[41]) // ErrProcessUnknown encodes as code 1
	})

}
