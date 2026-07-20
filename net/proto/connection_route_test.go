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

var (
	routeProcessID = gen.ProcessID{Name: "worker", Node: "me@localhost"}
	routeAlias     = gen.Alias{Node: "me@localhost", Creation: 1, ID: [3]uint64{7, 8, 0}}
	routeEvent     = gen.Event{Name: "ev", Node: "me@localhost"}
)

func TestRouteUnlinkProcessID(t *testing.T) {
	rc, core := newRouteConn(t)
	rc.routeMessage(MessageUnlinkProcessID{Source: localPID(5), Target: routeProcessID, Ref: aliveRef})
	core.ShouldWireUnlink().From(localPID(5)).Target(routeProcessID).Once().Assert()
}

func TestRouteLinkAlias(t *testing.T) {
	rc, core := newRouteConn(t)
	rc.routeMessage(MessageLinkAlias{Source: localPID(5), Target: routeAlias, Ref: aliveRef})
	core.ShouldWireLink().From(localPID(5)).Target(routeAlias).Once().Assert()
}

func TestRouteUnlinkAlias(t *testing.T) {
	rc, core := newRouteConn(t)
	rc.routeMessage(MessageUnlinkAlias{Source: localPID(5), Target: routeAlias, Ref: aliveRef})
	core.ShouldWireUnlink().From(localPID(5)).Target(routeAlias).Once().Assert()
}

func TestRouteLinkEvent(t *testing.T) {
	rc, core := newRouteConn(t)
	rc.routeMessage(MessageLinkEvent{Source: localPID(5), Target: routeEvent, Ref: aliveRef})
	core.ShouldWireLink().From(localPID(5)).Target(routeEvent).Once().Assert()
}

func TestRouteUnlinkEvent(t *testing.T) {
	rc, core := newRouteConn(t)
	rc.routeMessage(MessageUnlinkEvent{Source: localPID(5), Target: routeEvent, Ref: aliveRef})
	core.ShouldWireUnlink().From(localPID(5)).Target(routeEvent).Once().Assert()
}

func TestRouteMonitorProcessID(t *testing.T) {
	rc, core := newRouteConn(t)
	rc.routeMessage(MessageMonitorProcessID{Source: localPID(5), Target: routeProcessID, Ref: aliveRef})
	core.ShouldWireMonitor().From(localPID(5)).Target(routeProcessID).Once().Assert()
}

func TestRouteDemonitorProcessID(t *testing.T) {
	rc, core := newRouteConn(t)
	rc.routeMessage(MessageDemonitorProcessID{Source: localPID(5), Target: routeProcessID, Ref: aliveRef})
	core.ShouldWireDemonitor().From(localPID(5)).Target(routeProcessID).Once().Assert()
}

func TestRouteMonitorAlias(t *testing.T) {
	rc, core := newRouteConn(t)
	rc.routeMessage(MessageMonitorAlias{Source: localPID(5), Target: routeAlias, Ref: aliveRef})
	core.ShouldWireMonitor().From(localPID(5)).Target(routeAlias).Once().Assert()
}

func TestRouteDemonitorAlias(t *testing.T) {
	rc, core := newRouteConn(t)
	rc.routeMessage(MessageDemonitorAlias{Source: localPID(5), Target: routeAlias, Ref: aliveRef})
	core.ShouldWireDemonitor().From(localPID(5)).Target(routeAlias).Once().Assert()
}

func TestRouteMonitorEvent(t *testing.T) {
	rc, core := newRouteConn(t)
	rc.routeMessage(MessageMonitorEvent{Source: localPID(5), Target: routeEvent, Ref: aliveRef})
	core.ShouldWireMonitor().From(localPID(5)).Target(routeEvent).Once().Assert()
}

func TestRouteDemonitorEvent(t *testing.T) {
	rc, core := newRouteConn(t)
	rc.routeMessage(MessageDemonitorEvent{Source: localPID(5), Target: routeEvent, Ref: aliveRef})
	core.ShouldWireDemonitor().From(localPID(5)).Target(routeEvent).Once().Assert()
}

// A remote-spawn request reaches the core with the sender node as the source.
func TestRouteSpawn(t *testing.T) {
	rc, core := newRouteConn(t)
	var gotName, gotSource gen.Atom
	called := false
	core.OnRouteSpawn(func(node, name gen.Atom, opts gen.ProcessOptionsExtra, source gen.Atom) (gen.PID, error) {
		called, gotName, gotSource = true, name, source
		return gen.PID{}, nil
	})
	rc.routeMessage(MessageSpawn{Name: "factory", Ref: aliveRef})
	check.True(t, called)
	check.Equal(t, gen.Atom("factory"), gotName)
	check.Equal(t, gen.Atom("peer@localhost"), gotSource)
}

// A remote application-start request reaches the core with the right name.
func TestRouteApplicationStart(t *testing.T) {
	rc, core := newRouteConn(t)
	var gotName gen.Atom
	called := false
	core.OnRouteApplicationStart(func(name gen.Atom, mode gen.ApplicationMode, opts gen.ApplicationOptionsExtra, source gen.Atom) error {
		called, gotName = true, name
		return nil
	})
	rc.routeMessage(MessageApplicationStart{Name: "app", Ref: aliveRef})
	check.True(t, called)
	check.Equal(t, gen.Atom("app"), gotName)
}

// An application-info request reaches the core.
func TestRouteApplicationInfo(t *testing.T) {
	rc, core := newRouteConn(t)
	called := false
	core.OnRouteApplicationInfo(func(name gen.Atom) (gen.ApplicationInfo, error) {
		called = true
		return gen.ApplicationInfo{}, nil
	})
	rc.routeMessage(MessageApplicationInfo{Name: "app", Ref: aliveRef})
	check.True(t, called)
}

// A cache update from the peer lands in the connection's decode caches.
func TestRouteUpdateCache(t *testing.T) {
	core := mock.NewCoreT(t)
	rc := &connection{
		peer:          "peer@localhost",
		peer_creation: testPeerCreation,
		core:          core,
		log:           mock.NewLog(),
		encodeOptions: edf.Options{Cache: new(sync.Map)},
		decodeOptions: edf.Options{
			AtomCache:   new(sync.Map),
			AtomMapping: new(sync.Map),
			RegCache:    new(sync.Map),
			ErrCache:    new(sync.Map),
		},
	}
	rc.routeMessage(MessageUpdateCache{AtomCache: map[uint16]gen.Atom{7: "hello"}, Ref: aliveRef})

	v, ok := rc.decodeOptions.AtomCache.Load(uint16(7))
	check.True(t, ok)
	check.Equal(t, gen.Atom("hello"), v.(gen.Atom))
}

// An unsupported message type is logged and dropped, not routed.
func TestRouteUnsupportedTypeIgnored(t *testing.T) {
	rc, core := newRouteConn(t)
	rc.routeMessage("unexpected value")
	core.ShouldWireLink().None().Assert()
	core.ShouldDeliver().None().Assert()
}
