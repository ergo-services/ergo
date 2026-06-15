package proto

import (
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/mock"
)

// setCore gives the connection a mock core that hands out a fixed request ref, so
// a test knows which pending request to answer. Returns that ref.
func setCore(tc *testConn) gen.Ref {
	ref := gen.Ref{Node: "me@localhost", Creation: 1, ID: [3]uint64{99, 0, 0}}
	core := mock.NewCore()
	core.OnMakeRefWithDeadline(func(int64) (gen.Ref, error) { return ref, nil })
	tc.c.core = core
	return ref
}

// request runs a blocking link/monitor call in a goroutine, captures the
// protoMessageAny it sends, answers the pending request with success, and returns
// the decoded request message. It fails the test if the call errors.
func (tc *testConn) request(t *testing.T, ref gen.Ref, call func() error) any {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- call() }()

	_, mtype, body := tc.readFrame(t)
	check.Equal(t, protoMessageAny, mtype)
	msg := tc.decode(t, body)

	tc.c.requestsMutex.RLock()
	ch := tc.c.requests[ref]
	tc.c.requestsMutex.RUnlock()
	if ch == nil {
		t.Fatal("no pending request registered for the expected ref")
	}
	ch <- MessageResult{Ref: ref, Result: []gen.MessageEvent{}}
	check.NoError(t, <-done)
	return msg
}

func TestLinkPID(t *testing.T) {
	tc := newTestConn(t, gen.NetworkFlags{})
	ref := setCore(tc)
	pid, target := localPID(5), peerPID(9)

	m := tc.request(t, ref, func() error { return tc.c.LinkPID(pid, target) }).(MessageLinkPID)
	check.Equal(t, pid, m.Source)
	check.Equal(t, target, m.Target)
}

func TestUnlinkPID(t *testing.T) {
	tc := newTestConn(t, gen.NetworkFlags{})
	ref := setCore(tc)
	pid, target := localPID(5), peerPID(9)

	m := tc.request(t, ref, func() error { return tc.c.UnlinkPID(pid, target) }).(MessageUnlinkPID)
	check.Equal(t, pid, m.Source)
	check.Equal(t, target, m.Target)
}

func TestLinkProcessID(t *testing.T) {
	tc := newTestConn(t, gen.NetworkFlags{})
	ref := setCore(tc)
	pid := localPID(5)
	target := gen.ProcessID{Name: "worker", Node: "peer@localhost"}

	m := tc.request(t, ref, func() error { return tc.c.LinkProcessID(pid, target) }).(MessageLinkProcessID)
	check.Equal(t, pid, m.Source)
	check.Equal(t, target, m.Target)
}

func TestLinkAlias(t *testing.T) {
	tc := newTestConn(t, gen.NetworkFlags{})
	ref := setCore(tc)
	pid := localPID(5)
	target := gen.Alias{Node: "peer@localhost", Creation: testPeerCreation, ID: [3]uint64{11, 22, 33}}

	m := tc.request(t, ref, func() error { return tc.c.LinkAlias(pid, target) }).(MessageLinkAlias)
	check.Equal(t, pid, m.Source)
	check.Equal(t, target, m.Target)
}

func TestLinkEvent(t *testing.T) {
	tc := newTestConn(t, gen.NetworkFlags{})
	ref := setCore(tc)
	pid := localPID(5)
	target := gen.Event{Name: "tick", Node: "peer@localhost"}

	m := tc.request(t, ref, func() error {
		_, err := tc.c.LinkEvent(pid, target)
		return err
	}).(MessageLinkEvent)
	check.Equal(t, pid, m.Source)
	check.Equal(t, target, m.Target)
}

func TestMonitorPID(t *testing.T) {
	tc := newTestConn(t, gen.NetworkFlags{})
	ref := setCore(tc)
	pid, target := localPID(5), peerPID(9)

	m := tc.request(t, ref, func() error { return tc.c.MonitorPID(pid, target) }).(MessageMonitorPID)
	check.Equal(t, pid, m.Source)
	check.Equal(t, target, m.Target)
}

func TestDemonitorPID(t *testing.T) {
	tc := newTestConn(t, gen.NetworkFlags{})
	ref := setCore(tc)
	pid, target := localPID(5), peerPID(9)

	m := tc.request(t, ref, func() error { return tc.c.DemonitorPID(pid, target) }).(MessageDemonitorPID)
	check.Equal(t, pid, m.Source)
	check.Equal(t, target, m.Target)
}

func TestMonitorEvent(t *testing.T) {
	tc := newTestConn(t, gen.NetworkFlags{})
	ref := setCore(tc)
	pid := localPID(5)
	target := gen.Event{Name: "tick", Node: "peer@localhost"}

	m := tc.request(t, ref, func() error {
		_, err := tc.c.MonitorEvent(pid, target)
		return err
	}).(MessageMonitorEvent)
	check.Equal(t, pid, m.Source)
	check.Equal(t, target, m.Target)
}

// a request returns the error the peer replied with.
func TestRequestReturnsPeerError(t *testing.T) {
	tc := newTestConn(t, gen.NetworkFlags{})
	ref := setCore(tc)
	pid, target := localPID(5), peerPID(9)

	done := make(chan error, 1)
	go func() { done <- tc.c.LinkPID(pid, target) }()
	tc.readFrame(t) // request sent; its ref is registered and waiting

	tc.c.requestsMutex.RLock()
	ch := tc.c.requests[ref]
	tc.c.requestsMutex.RUnlock()
	ch <- MessageResult{Ref: ref, Error: gen.ErrProcessUnknown}

	check.ErrorIs(t, <-done, gen.ErrProcessUnknown)
}

// a pending request fails with ErrNoConnection the moment the connection
// terminates, instead of blocking until the request timeout.
func TestRequestFailsOnTerminate(t *testing.T) {
	tc := newTestConn(t, gen.NetworkFlags{})
	setCore(tc)
	pid, target := localPID(5), peerPID(9)

	done := make(chan error, 1)
	go func() { done <- tc.c.LinkPID(pid, target) }()
	tc.readFrame(t) // request sent; its ref is registered and waiting

	tc.c.Terminate(gen.ErrNoConnection)
	check.ErrorIs(t, <-done, gen.ErrNoConnection)
}
