package proto

import (
	"encoding/binary"
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
)

func TestSendAck(t *testing.T) {
	ref := gen.Ref{Node: "peer@localhost", Creation: testPeerCreation, ID: [3]uint64{7, 8, 9}}

	t.Run("an ack frame to a peer that accepts it", func(t *testing.T) {
		tc := newTestConn(t, gen.NetworkFlags{Enable: true, EnableAck: true})
		from, to := localPID(5), peerPID(9)

		options := gen.MessageOptions{Ref: ref, Priority: gen.MessagePriorityHigh, ImportantDelivery: true}
		check.NoError(t, tc.c.SendAck(from, to, options, nil))

		_, mtype, body := tc.readFrame(t)
		check.Equal(t, protoMessageAck, mtype)
		check.Equal(t, from.ID, binary.BigEndian.Uint64(body[0:8]))
		check.Equal(t, byte(0), body[8])
		check.Equal(t, to.ID, binary.BigEndian.Uint64(body[9:17]))
		check.Equal(t, ref.ID[0], binary.BigEndian.Uint64(body[17:25]))
		check.Equal(t, ref.ID[1], binary.BigEndian.Uint64(body[25:33]))
		check.Equal(t, ref.ID[2], binary.BigEndian.Uint64(body[33:41]))
		check.Equal(t, byte(0), body[41])
		check.Equal(t, 42, len(body))
	})

	t.Run("a response error to a peer without the ack frame", func(t *testing.T) {
		tc := newTestConn(t, gen.NetworkFlags{Enable: true, EnableImportantDelivery: true})
		from, to := localPID(5), peerPID(9)

		options := gen.MessageOptions{Ref: ref, Priority: gen.MessagePriorityHigh, ImportantDelivery: true}
		check.NoError(t, tc.c.SendAck(from, to, options, gen.ErrProcessUnknown))

		_, mtype, body := tc.readFrame(t)
		check.Equal(t, protoMessageResponseError, mtype)
		check.Equal(t, byte(gen.MessagePriorityHigh), body[8])
		check.Equal(t, to.ID, binary.BigEndian.Uint64(body[9:17]))
		check.Equal(t, ref.ID[0], binary.BigEndian.Uint64(body[17:25]))
		check.Equal(t, ref.ID[1], binary.BigEndian.Uint64(body[25:33]))
		check.Equal(t, ref.ID[2], binary.BigEndian.Uint64(body[33:41]))
		check.Equal(t, byte(1), body[41])
	})

	t.Run("result codes", func(t *testing.T) {
		codes := []struct {
			result error
			code   byte
		}{
			{nil, 0},
			{gen.ErrProcessUnknown, 1},
			{gen.ErrProcessMailboxFull, 2},
			{gen.ErrProcessTerminated, 3},
			{gen.ErrTaken, 255},
		}
		for _, c := range codes {
			tc := newTestConn(t, gen.NetworkFlags{Enable: true, EnableAck: true})
			check.NoError(t, tc.c.SendAck(localPID(5), peerPID(9), gen.MessageOptions{Ref: ref}, c.result))
			_, mtype, body := tc.readFrame(t)
			check.Equal(t, protoMessageAck, mtype)
			check.Equal(t, c.code, body[41])
		}
	})

	t.Run("a target of another incarnation", func(t *testing.T) {
		tc := newTestConn(t, gen.NetworkFlags{Enable: true, EnableAck: true})
		to := peerPID(9)
		to.Creation++
		err := tc.c.SendAck(localPID(5), to, gen.MessageOptions{Ref: ref}, nil)
		check.True(t, err == gen.ErrProcessIncarnation)
	})
}

// a received ack goes to RouteSendAck, never to RouteSendResponseError: the ref
// it carries is never read for a reply-to.
func TestRecvAck(t *testing.T) {
	ref := gen.Ref{Node: "peer@localhost", Creation: testPeerCreation, ID: [3]uint64{7, 8, 9}}

	for _, result := range []error{nil, gen.ErrProcessMailboxFull, gen.ErrTaken} {
		tc := newTestConn(t, gen.NetworkFlags{Enable: true, EnableAck: true})
		check.NoError(t, tc.c.SendAck(localPID(5), peerPID(9), gen.MessageOptions{Ref: ref}, result))
		frame := tc.readRawFrame(t)

		rc, core := newRecvConn(t)
		var acks int
		var gotFrom, gotTo gen.PID
		var gotRef gen.Ref
		var gotResult error
		core.OnRouteSendAck(func(from gen.PID, to gen.PID, options gen.MessageOptions, result error) error {
			acks++
			gotFrom, gotTo, gotRef, gotResult = from, to, options.Ref, result
			return nil
		})
		var responses int
		core.OnRouteSendResponseError(func(from gen.PID, to gen.PID, options gen.MessageOptions, err error) error {
			responses++
			return nil
		})
		feedFrame(rc, frame)

		check.Equal(t, 1, acks)
		check.Equal(t, 0, responses)
		check.Equal(t, senderPID(5), gotFrom)
		check.Equal(t, uint64(9), gotTo.ID)
		check.Equal(t, ref.ID, gotRef.ID)
		if result == nil {
			check.True(t, gotResult == nil)
			continue
		}
		check.True(t, gotResult != nil)
		check.Equal(t, result.Error(), gotResult.Error())
	}
}
