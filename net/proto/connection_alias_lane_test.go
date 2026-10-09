package proto

import (
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
	"ergo.services/ergo/testing/check"
)

func (tc *testConn) requestOrder(t *testing.T, ref gen.Ref, call func() error) byte {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- call() }()

	order, mtype, _ := tc.readFrame(t)
	check.Equal(t, protoMessageAny, mtype)

	tc.c.requestsMutex.RLock()
	ch := tc.c.requests[ref]
	tc.c.requestsMutex.RUnlock()
	if ch == nil {
		t.Fatal("no pending request registered for the expected ref")
	}
	ch <- MessageResult{Ref: ref}
	check.NoError(t, <-done)
	return order
}

// TestAliasLane: everything addressed to an alias picks its ordering lane and its
// route queue from ID[0], the whole uniqueness of an alias. ID[1] is zero for
// every alias, so a lane taken from it would put them all into one.
func TestAliasLane(t *testing.T) {
	aliases := []gen.Alias{
		{Node: "peer@localhost", Creation: testPeerCreation, ID: [3]uint64{1791504000123456789, 0, 0}},
		{Node: "peer@localhost", Creation: testPeerCreation, ID: [3]uint64{1791504000123456790, 0, 0}},
	}
	lane := func(a gen.Alias) byte { return uint8(a.ID[0]%255 + 1) }
	check.True(t, lane(aliases[0]) != lane(aliases[1]))

	for _, alias := range aliases {
		tc := newTestConn(t, gen.NetworkFlags{})
		check.NoError(t, tc.c.SendAlias(localPID(5), alias, gen.MessageOptions{KeepNetworkOrder: true}, "x"))
		order, mtype, _ := tc.readFrame(t)
		check.Equal(t, protoMessageAlias, mtype)
		check.Equal(t, lane(alias), order)

		tc = newTestConn(t, gen.NetworkFlags{})
		ref := gen.Ref{Node: "me@localhost", Creation: 1, ID: [3]uint64{7, 0, 0}}
		check.NoError(t, tc.c.CallAlias(localPID(5), alias, gen.MessageOptions{Ref: ref, KeepNetworkOrder: true}, "x"))
		order, mtype, _ = tc.readFrame(t)
		check.Equal(t, protoRequestAlias, mtype)
		check.Equal(t, lane(alias), order)

		calls := map[string]func(c *connection) error{
			"link":      func(c *connection) error { return c.LinkAlias(localPID(5), alias) },
			"unlink":    func(c *connection) error { return c.UnlinkAlias(localPID(5), alias) },
			"monitor":   func(c *connection) error { return c.MonitorAlias(localPID(5), alias) },
			"demonitor": func(c *connection) error { return c.DemonitorAlias(localPID(5), alias) },
		}
		for name, call := range calls {
			tc := newTestConn(t, gen.NetworkFlags{})
			ref := setCore(tc)
			order := tc.requestOrder(t, ref, func() error { return call(tc.c) })
			if order != lane(alias) {
				t.Fatalf("%s: lane %d, want %d", name, order, lane(alias))
			}
		}
	}

	messages := func(a gen.Alias) []any {
		return []any{
			MessageLinkAlias{Source: localPID(5), Target: a, Ref: aliveRef},
			MessageUnlinkAlias{Source: localPID(5), Target: a, Ref: aliveRef},
			MessageMonitorAlias{Source: localPID(5), Target: a, Ref: aliveRef},
			MessageDemonitorAlias{Source: localPID(5), Target: a, Ref: aliveRef},
		}
	}
	for _, alias := range aliases {
		for _, msg := range messages(alias) {
			rc, _ := newRouteConn(t)
			for i := range rc.routeQueues {
				rc.routeQueues[i] = lib.NewQueueMPSC()
				rc.routeQueues[i].Lock()
			}
			rc.dispatchRoute(msg)

			want := alias.ID[0] & uint64(routeQueuesPerConn-1)
			for i, q := range rc.routeQueues {
				if uint64(i) == want {
					check.True(t, q.Item() != nil)
					continue
				}
				check.True(t, q.Item() == nil)
			}
		}
	}
	want0 := aliases[0].ID[0] & uint64(routeQueuesPerConn-1)
	want1 := aliases[1].ID[0] & uint64(routeQueuesPerConn-1)
	check.True(t, want0 != want1)
}
