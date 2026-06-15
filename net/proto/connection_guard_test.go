package proto

import (
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
)

// every operation addressed at a peer process rejects a target whose incarnation
// (Creation) no longer matches the connected peer.
func TestRejectsStaleTargetIncarnation(t *testing.T) {
	stalePID := func() gen.PID { p := peerPID(9); p.Creation = testPeerCreation + 1; return p }
	staleAlias := gen.Alias{Node: "peer@localhost", Creation: testPeerCreation + 1, ID: [3]uint64{1, 0, 0}}

	cases := []struct {
		name string
		call func(c *connection) error
	}{
		{"SendPID", func(c *connection) error { return c.SendPID(localPID(5), stalePID(), gen.MessageOptions{}, "x") }},
		{"SendAlias", func(c *connection) error { return c.SendAlias(localPID(5), staleAlias, gen.MessageOptions{}, "x") }},
		{"SendExit", func(c *connection) error { return c.SendExit(localPID(5), stalePID(), gen.TerminateReasonNormal) }},
		{"SendResponse", func(c *connection) error { return c.SendResponse(localPID(5), stalePID(), gen.MessageOptions{}, "x") }},
		{"SendResponseError", func(c *connection) error {
			return c.SendResponseError(localPID(5), stalePID(), gen.MessageOptions{}, gen.ErrProcessUnknown)
		}},
		{"CallPID", func(c *connection) error { return c.CallPID(localPID(5), stalePID(), gen.MessageOptions{}, "x") }},
		{"CallAlias", func(c *connection) error { return c.CallAlias(localPID(5), staleAlias, gen.MessageOptions{}, "x") }},
		{"LinkPID", func(c *connection) error { return c.LinkPID(localPID(5), stalePID()) }},
		{"LinkAlias", func(c *connection) error { return c.LinkAlias(localPID(5), staleAlias) }},
		{"MonitorPID", func(c *connection) error { return c.MonitorPID(localPID(5), stalePID()) }},
		{"MonitorAlias", func(c *connection) error { return c.MonitorAlias(localPID(5), staleAlias) }},
	}

	for _, tcase := range cases {
		t.Run(tcase.name, func(t *testing.T) {
			tc := newTestConn(t, gen.NetworkFlags{})
			check.ErrorIs(t, tcase.call(tc.c), gen.ErrProcessIncarnation)
		})
	}
}
