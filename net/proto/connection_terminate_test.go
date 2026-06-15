package proto

import (
	"encoding/binary"
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
)

func TestSendTerminatePID(t *testing.T) {
	tc := newTestConn(t, gen.NetworkFlags{})
	target := peerPID(9)

	err := tc.c.SendTerminatePID(target, gen.TerminateReasonNormal)
	check.NoError(t, err)

	order, mtype, body := tc.readFrame(t)
	check.Equal(t, protoMessageTerminatePID, mtype)
	check.Equal(t, uint8(0), order)
	check.Equal(t, byte(gen.MessagePriorityHigh), body[0])
	check.Equal(t, target.ID, binary.BigEndian.Uint64(body[1:9]))
}

func TestSendTerminateProcessID(t *testing.T) {
	tc := newTestConn(t, gen.NetworkFlags{})
	target := gen.ProcessID{Name: "worker", Node: "peer@localhost"}

	err := tc.c.SendTerminateProcessID(target, gen.TerminateReasonNormal)
	check.NoError(t, err)

	_, mtype, body := tc.readFrame(t)
	check.Equal(t, protoMessageTerminateName, mtype)
	check.Equal(t, byte(gen.MessagePriorityHigh), body[0])
	nameLen := int(body[1])
	check.Equal(t, "worker", string(body[2:2+nameLen]))
}

func TestSendTerminateAlias(t *testing.T) {
	tc := newTestConn(t, gen.NetworkFlags{})
	target := gen.Alias{Node: "peer@localhost", Creation: testPeerCreation, ID: [3]uint64{11, 22, 33}}

	err := tc.c.SendTerminateAlias(target, gen.TerminateReasonNormal)
	check.NoError(t, err)

	_, mtype, body := tc.readFrame(t)
	check.Equal(t, protoMessageTerminateAlias, mtype)
	check.Equal(t, target.ID[0], binary.BigEndian.Uint64(body[1:9]))
	check.Equal(t, target.ID[1], binary.BigEndian.Uint64(body[9:17]))
	check.Equal(t, target.ID[2], binary.BigEndian.Uint64(body[17:25]))
}

func TestSendTerminateEvent(t *testing.T) {
	tc := newTestConn(t, gen.NetworkFlags{})
	target := gen.Event{Name: "tick", Node: "peer@localhost"}

	err := tc.c.SendTerminateEvent(target, gen.TerminateReasonNormal)
	check.NoError(t, err)

	_, mtype, body := tc.readFrame(t)
	check.Equal(t, protoMessageTerminateEvent, mtype)
	nameLen := int(body[1])
	check.Equal(t, "tick", string(body[2:2+nameLen]))
}
