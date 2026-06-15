package proto

import (
	"sync"
	"testing"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/net/edf"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/mock"
)

// aliveRef has a zero deadline (ID[2] == 0), so Ref.IsAlive() is always true and
// routeMessage proceeds to the core instead of dropping the request.
var aliveRef = gen.Ref{Node: "peer@localhost", Creation: testPeerCreation, ID: [3]uint64{1, 2, 0}}

// newRouteConn builds a connection for driving routeMessage: a recording core plus
// the bits routeMessage touches when it replies (encode cache, requests map).
func newRouteConn(t *testing.T) (*connection, *mock.Core) {
	core := mock.NewCoreT(t)
	rc := &connection{
		peer:          "peer@localhost",
		peer_creation: testPeerCreation,
		core:          core,
		log:           mock.NewLog(),
		encodeOptions: edf.Options{Cache: new(sync.Map)},
		requests:      make(map[gen.Ref]chan MessageResult),
	}
	return rc, core
}

func TestRouteLinkPID(t *testing.T) {
	rc, core := newRouteConn(t)
	rc.routeMessage(MessageLinkPID{Source: localPID(5), Target: peerPID(9), Ref: aliveRef})
	core.ShouldWireLink().From(localPID(5)).Target(peerPID(9)).Once().Assert()
}

func TestRouteUnlinkPID(t *testing.T) {
	rc, core := newRouteConn(t)
	rc.routeMessage(MessageUnlinkPID{Source: localPID(5), Target: peerPID(9), Ref: aliveRef})
	core.ShouldWireUnlink().From(localPID(5)).Target(peerPID(9)).Once().Assert()
}

func TestRouteMonitorPID(t *testing.T) {
	rc, core := newRouteConn(t)
	rc.routeMessage(MessageMonitorPID{Source: localPID(5), Target: peerPID(9), Ref: aliveRef})
	core.ShouldWireMonitor().From(localPID(5)).Target(peerPID(9)).Once().Assert()
}

func TestRouteDemonitorPID(t *testing.T) {
	rc, core := newRouteConn(t)
	rc.routeMessage(MessageDemonitorPID{Source: localPID(5), Target: peerPID(9), Ref: aliveRef})
	core.ShouldWireDemonitor().From(localPID(5)).Target(peerPID(9)).Once().Assert()
}

func TestRouteLinkProcessID(t *testing.T) {
	rc, core := newRouteConn(t)
	target := gen.ProcessID{Name: "worker", Node: "me@localhost"}
	rc.routeMessage(MessageLinkProcessID{Source: localPID(5), Target: target, Ref: aliveRef})
	core.ShouldWireLink().From(localPID(5)).Target(target).Once().Assert()
}

// a dead request ref is dropped before reaching the core.
func TestRouteIgnoresDeadRef(t *testing.T) {
	rc, core := newRouteConn(t)
	dead := gen.Ref{Node: "peer@localhost", ID: [3]uint64{1, 2, uint64(time.Now().Unix() - 60)}}
	rc.routeMessage(MessageLinkPID{Source: localPID(5), Target: peerPID(9), Ref: dead})
	core.ShouldWireLink().None().Assert()
}

// a MessageResult is delivered to the goroutine waiting on its ref.
func TestRouteResultDeliversToWaiter(t *testing.T) {
	rc, _ := newRouteConn(t)
	ch := make(chan MessageResult, 1)
	rc.requests[aliveRef] = ch

	rc.routeMessage(MessageResult{Ref: aliveRef, Error: gen.ErrProcessUnknown})

	got := <-ch
	check.ErrorIs(t, got.Error, gen.ErrProcessUnknown)
}
