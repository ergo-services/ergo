package proto

import (
	"encoding/binary"
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
)

// requestRef is a sample request reference carried by a Call.
func requestRef() gen.Ref {
	return gen.Ref{Node: "me@localhost", Creation: 1, ID: [3]uint64{7, 8, 9}}
}

func TestCallPID(t *testing.T) {
	t.Run("encodes the request ref and target id", func(t *testing.T) {
		tc := newTestConn(t, gen.NetworkFlags{})
		from, to := localPID(5), peerPID(9)
		ref := requestRef()

		err := tc.c.CallPID(from, to, gen.MessageOptions{Ref: ref}, "q")
		check.NoError(t, err)

		_, mtype, body := tc.readFrame(t)
		check.Equal(t, protoRequestPID, mtype)
		check.Equal(t, from.ID, binary.BigEndian.Uint64(body[0:8]))
		check.Equal(t, ref.ID[0], binary.BigEndian.Uint64(body[9:17]))
		check.Equal(t, ref.ID[1], binary.BigEndian.Uint64(body[17:25]))
		check.Equal(t, ref.ID[2], binary.BigEndian.Uint64(body[25:33]))
		check.Equal(t, to.ID, binary.BigEndian.Uint64(body[33:41]))
		check.Equal(t, "q", tc.decode(t, body[41:]))
	})

}

func TestCallProcessID(t *testing.T) {
	t.Run("encodes the ref and inline target name", func(t *testing.T) {
		tc := newTestConn(t, gen.NetworkFlags{})
		from := localPID(5)
		to := gen.ProcessID{Name: "worker", Node: "peer@localhost"}
		ref := requestRef()

		err := tc.c.CallProcessID(from, to, gen.MessageOptions{Ref: ref}, "q")
		check.NoError(t, err)

		_, mtype, body := tc.readFrame(t)
		check.Equal(t, protoRequestName, mtype)
		check.Equal(t, from.ID, binary.BigEndian.Uint64(body[0:8]))
		check.Equal(t, ref.ID[0], binary.BigEndian.Uint64(body[9:17]))
		check.Equal(t, ref.ID[2], binary.BigEndian.Uint64(body[25:33]))
		nameLen := int(body[33])
		check.Equal(t, "worker", string(body[34:34+nameLen]))
		check.Equal(t, "q", tc.decode(t, body[34+nameLen:]))
	})
}

func TestCallAlias(t *testing.T) {
	t.Run("encodes the ref and the full alias id", func(t *testing.T) {
		tc := newTestConn(t, gen.NetworkFlags{})
		from := localPID(5)
		to := gen.Alias{Node: "peer@localhost", Creation: testPeerCreation, ID: [3]uint64{11, 22, 33}}
		ref := requestRef()

		err := tc.c.CallAlias(from, to, gen.MessageOptions{Ref: ref}, "q")
		check.NoError(t, err)

		_, mtype, body := tc.readFrame(t)
		check.Equal(t, protoRequestAlias, mtype)
		check.Equal(t, from.ID, binary.BigEndian.Uint64(body[0:8]))
		check.Equal(t, ref.ID[0], binary.BigEndian.Uint64(body[9:17]))
		check.Equal(t, to.ID[0], binary.BigEndian.Uint64(body[33:41]))
		check.Equal(t, to.ID[1], binary.BigEndian.Uint64(body[41:49]))
		check.Equal(t, to.ID[2], binary.BigEndian.Uint64(body[49:57]))
		check.Equal(t, "q", tc.decode(t, body[57:]))
	})

}
