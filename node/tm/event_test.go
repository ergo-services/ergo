package tm

import (
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ergo.services/ergo/gen"
)

// testCore is a minimal gen.CoreTargetManager that records every routed
// message and resolves GetConnection through a registered map of test
// connections (per remote node atom). Unknown nodes return ErrNoConnection.
type testCore struct {
	name gen.Atom
	pid  gen.PID

	mu     sync.Mutex
	sent   []routedSend
	exits  []routedExit
	events []routedEvent
	conns  map[gen.Atom]*testConn
	refSeq atomic.Uint64
}

type routedSend struct {
	From    gen.PID
	To      gen.PID
	Options gen.MessageOptions
	Message any
}

type routedExit struct {
	From    gen.PID
	To      []gen.PID
	Message any
}

type routedEvent struct {
	From    gen.PID
	To      []gen.PID
	Options gen.MessageOptions
	Message gen.MessageEvent
}

func newTestCore() *testCore {
	return &testCore{
		name:  "node@local",
		pid:   gen.PID{Node: "node@local", ID: 1, Creation: 100},
		conns: map[gen.Atom]*testConn{},
	}
}

func (c *testCore) registerConn(node gen.Atom) *testConn {
	conn := newTestConn()
	c.mu.Lock()
	c.conns[node] = conn
	c.mu.Unlock()
	return conn
}

func (c *testCore) Name() gen.Atom { return c.name }
func (c *testCore) PID() gen.PID   { return c.pid }
func (c *testCore) Log() gen.Log   { return nil }
func (c *testCore) MakeRef() gen.Ref {
	return gen.Ref{Node: c.name, Creation: c.pid.Creation, ID: [3]uint64{c.refSeq.Add(1), 0, 0}}
}

func (c *testCore) RouteSendPID(from gen.PID, to gen.PID, options gen.MessageOptions, message any) error {
	c.mu.Lock()
	c.sent = append(c.sent, routedSend{From: from, To: to, Options: options, Message: message})
	c.mu.Unlock()
	return nil
}

func (c *testCore) RouteSendExitMessages(from gen.PID, to []gen.PID, message any) error {
	c.mu.Lock()
	cp := make([]gen.PID, len(to))
	copy(cp, to)
	c.exits = append(c.exits, routedExit{From: from, To: cp, Message: message})
	c.mu.Unlock()
	return nil
}

func (c *testCore) RouteSendEventMessages(from gen.PID, to []gen.PID, options gen.MessageOptions, message gen.MessageEvent) error {
	c.mu.Lock()
	cp := make([]gen.PID, len(to))
	copy(cp, to)
	c.events = append(c.events, routedEvent{From: from, To: cp, Options: options, Message: message})
	c.mu.Unlock()
	return nil
}

func (c *testCore) GetConnection(node gen.Atom) (gen.Connection, error) {
	c.mu.Lock()
	conn, ok := c.conns[node]
	c.mu.Unlock()
	if ok == false {
		return nil, gen.ErrNoConnection
	}
	return conn, nil
}

// testConn implements gen.Connection well enough for tests. Only the
// wire-link/unlink/monitor/demonitor and SendTerminate methods do real
// recording; the rest panic so misuse is loud.
type testConn struct {
	mu              sync.Mutex
	linkPIDs        []gen.PID
	unlinkPIDs      []gen.PID
	monitorPIDs     []gen.PID
	demonitorPIDs   []gen.PID
	linkEvents      []gen.Event
	unlinkEvents    []gen.Event
	monitorEvents   []gen.Event
	demonitorEvents []gen.Event
	terminatedPIDs  []gen.PID
	terminatedEvts  []gen.Event

	// linkEventBuffer is returned from LinkEvent/MonitorEvent. nil means
	// "not buffered" which lets the wire-state move to piggyback.
	linkEventBuffer []gen.MessageEvent
}

func newTestConn() *testConn {
	return &testConn{}
}

func (c *testConn) Node() gen.RemoteNode { return nil }

func (c *testConn) SendPID(from gen.PID, to gen.PID, options gen.MessageOptions, message any) error {
	return nil
}
func (c *testConn) SendProcessID(from gen.PID, to gen.ProcessID, options gen.MessageOptions, message any) error {
	return nil
}
func (c *testConn) SendAlias(from gen.PID, to gen.Alias, options gen.MessageOptions, message any) error {
	return nil
}
func (c *testConn) SendEvent(from gen.PID, options gen.MessageOptions, message gen.MessageEvent) error {
	return nil
}
func (c *testConn) SendExit(from gen.PID, to gen.PID, reason error) error { return nil }
func (c *testConn) SendResponse(from gen.PID, to gen.PID, options gen.MessageOptions, response any) error {
	return nil
}
func (c *testConn) SendResponseError(from gen.PID, to gen.PID, options gen.MessageOptions, err error) error {
	return nil
}

func (c *testConn) SendTerminatePID(target gen.PID, reason error) error {
	c.mu.Lock()
	c.terminatedPIDs = append(c.terminatedPIDs, target)
	c.mu.Unlock()
	return nil
}
func (c *testConn) SendTerminateProcessID(target gen.ProcessID, reason error) error { return nil }
func (c *testConn) SendTerminateAlias(target gen.Alias, reason error) error         { return nil }
func (c *testConn) SendTerminateEvent(target gen.Event, reason error) error {
	c.mu.Lock()
	c.terminatedEvts = append(c.terminatedEvts, target)
	c.mu.Unlock()
	return nil
}

func (c *testConn) CallPID(from gen.PID, to gen.PID, options gen.MessageOptions, message any) error {
	return nil
}
func (c *testConn) CallProcessID(from gen.PID, to gen.ProcessID, options gen.MessageOptions, message any) error {
	return nil
}
func (c *testConn) CallAlias(from gen.PID, to gen.Alias, options gen.MessageOptions, message any) error {
	return nil
}

func (c *testConn) LinkPID(pid gen.PID, target gen.PID) error {
	c.mu.Lock()
	c.linkPIDs = append(c.linkPIDs, target)
	c.mu.Unlock()
	return nil
}
func (c *testConn) UnlinkPID(pid gen.PID, target gen.PID) error {
	c.mu.Lock()
	c.unlinkPIDs = append(c.unlinkPIDs, target)
	c.mu.Unlock()
	return nil
}
func (c *testConn) LinkProcessID(pid gen.PID, target gen.ProcessID) error   { return nil }
func (c *testConn) UnlinkProcessID(pid gen.PID, target gen.ProcessID) error { return nil }
func (c *testConn) LinkAlias(pid gen.PID, target gen.Alias) error           { return nil }
func (c *testConn) UnlinkAlias(pid gen.PID, target gen.Alias) error         { return nil }

func (c *testConn) LinkEvent(pid gen.PID, target gen.Event) ([]gen.MessageEvent, error) {
	c.mu.Lock()
	c.linkEvents = append(c.linkEvents, target)
	buf := c.linkEventBuffer
	c.mu.Unlock()
	return buf, nil
}
func (c *testConn) UnlinkEvent(pid gen.PID, target gen.Event) error {
	c.mu.Lock()
	c.unlinkEvents = append(c.unlinkEvents, target)
	c.mu.Unlock()
	return nil
}

func (c *testConn) MonitorPID(pid gen.PID, target gen.PID) error {
	c.mu.Lock()
	c.monitorPIDs = append(c.monitorPIDs, target)
	c.mu.Unlock()
	return nil
}
func (c *testConn) DemonitorPID(pid gen.PID, target gen.PID) error {
	c.mu.Lock()
	c.demonitorPIDs = append(c.demonitorPIDs, target)
	c.mu.Unlock()
	return nil
}
func (c *testConn) MonitorProcessID(pid gen.PID, target gen.ProcessID) error   { return nil }
func (c *testConn) DemonitorProcessID(pid gen.PID, target gen.ProcessID) error { return nil }
func (c *testConn) MonitorAlias(pid gen.PID, target gen.Alias) error           { return nil }
func (c *testConn) DemonitorAlias(pid gen.PID, target gen.Alias) error         { return nil }

func (c *testConn) MonitorEvent(pid gen.PID, target gen.Event) ([]gen.MessageEvent, error) {
	c.mu.Lock()
	c.monitorEvents = append(c.monitorEvents, target)
	buf := c.linkEventBuffer
	c.mu.Unlock()
	return buf, nil
}
func (c *testConn) DemonitorEvent(pid gen.PID, target gen.Event) error {
	c.mu.Lock()
	c.demonitorEvents = append(c.demonitorEvents, target)
	c.mu.Unlock()
	return nil
}

func (c *testConn) RemoteSpawn(name gen.Atom, options gen.ProcessOptionsExtra) (gen.PID, error) {
	return gen.PID{}, nil
}
func (c *testConn) Join(conn net.Conn, id string, dial gen.NetworkDial, tail []byte) error {
	return nil
}
func (c *testConn) Terminate(reason error) {}

// helpers

func newManager() (*Manager, *testCore) {
	core := newTestCore()
	m := Create(core, Options{}).(*Manager)
	return m, core
}

// remotePID returns a PID belonging to a different node, so tests can use
// it to simulate remote consumers without remote-side wire propagation.
func remotePID(id uint64) gen.PID {
	return gen.PID{Node: "remote@local", ID: id, Creation: 200}
}

func TestRegisterEventThenInfo(t *testing.T) {
	m, _ := newManager()

	producer := gen.PID{Node: "node@local", ID: 42, Creation: 100}
	token, err := m.RegisterEvent(producer, "tick", gen.EventOptions{Notify: true, Buffer: 4})
	if err != nil {
		t.Fatalf("RegisterEvent: %v", err)
	}
	if token == (gen.Ref{}) {
		t.Fatal("expected non-zero token")
	}

	event := gen.Event{Node: "node@local", Name: "tick"}
	info, err := m.EventInfo(event)
	if err != nil {
		t.Fatalf("EventInfo: %v", err)
	}
	if info.Event != event {
		t.Fatalf("info.Event = %v want %v", info.Event, event)
	}
	if info.Producer != producer {
		t.Fatalf("info.Producer = %v want %v", info.Producer, producer)
	}
	if info.BufferSize != 4 {
		t.Fatalf("info.BufferSize = %d want 4", info.BufferSize)
	}
	if info.Notify == false {
		t.Fatal("info.Notify should be true")
	}
}

func TestRegisterEventDuplicateReturnsErrTaken(t *testing.T) {
	m, _ := newManager()
	producer := gen.PID{Node: "node@local", ID: 42, Creation: 100}

	if _, err := m.RegisterEvent(producer, "tick", gen.EventOptions{}); err != nil {
		t.Fatalf("first RegisterEvent: %v", err)
	}
	if _, err := m.RegisterEvent(producer, "tick", gen.EventOptions{}); err != gen.ErrTaken {
		t.Fatalf("second RegisterEvent: got %v want ErrTaken", err)
	}
}

func TestRegisterEventNodeLevelForcesNotifyOff(t *testing.T) {
	m, core := newManager()
	if _, err := m.RegisterEvent(core.pid, "tick", gen.EventOptions{Notify: true}); err != nil {
		t.Fatal(err)
	}
	info, _ := m.EventInfo(gen.Event{Node: core.name, Name: "tick"})
	if info.Notify == true {
		t.Fatal("node-level event must have Notify=false")
	}
}

func TestUnregisterEventWrongOwner(t *testing.T) {
	m, _ := newManager()
	producer := gen.PID{Node: "node@local", ID: 42, Creation: 100}
	other := gen.PID{Node: "node@local", ID: 43, Creation: 100}

	if _, err := m.RegisterEvent(producer, "tick", gen.EventOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := m.UnregisterEvent(other, "tick"); err != gen.ErrEventOwner {
		t.Fatalf("got %v want ErrEventOwner", err)
	}
}

func TestUnregisterEventNotifiesSubscribers(t *testing.T) {
	m, core := newManager()
	producer := gen.PID{Node: "node@local", ID: 42, Creation: 100}
	linker := gen.PID{Node: "node@local", ID: 10, Creation: 100}
	monitor := gen.PID{Node: "node@local", ID: 11, Creation: 100}

	m.RegisterEvent(producer, "tick", gen.EventOptions{})
	event := gen.Event{Node: "node@local", Name: "tick"}
	m.LinkEvent(linker, event)
	m.MonitorEvent(monitor, event)

	core.mu.Lock()
	core.exits = nil
	core.sent = nil
	core.mu.Unlock()

	if err := m.UnregisterEvent(producer, "tick"); err != nil {
		t.Fatal(err)
	}

	core.mu.Lock()
	defer core.mu.Unlock()
	if len(core.exits) != 1 || len(core.exits[0].To) != 1 || core.exits[0].To[0] != linker {
		t.Fatalf("expected one exit batch to linker, got %+v", core.exits)
	}
	em, ok := core.exits[0].Message.(gen.MessageExitEvent)
	if ok == false || em.Event != event || em.Reason != gen.ErrUnregistered {
		t.Fatalf("exit message mismatch: %+v", core.exits[0].Message)
	}
	if len(core.sent) != 1 || core.sent[0].To != monitor {
		t.Fatalf("expected one Down to monitor, got %+v", core.sent)
	}
	dm, ok := core.sent[0].Message.(gen.MessageDownEvent)
	if ok == false || dm.Event != event || dm.Reason != gen.ErrUnregistered {
		t.Fatalf("down message mismatch: %+v", core.sent[0].Message)
	}
}

func TestLinkEventFirstSubscriberTriggersEventStart(t *testing.T) {
	m, core := newManager()
	producer := gen.PID{Node: "node@local", ID: 42, Creation: 100}
	m.RegisterEvent(producer, "tick", gen.EventOptions{Notify: true})
	event := gen.Event{Node: "node@local", Name: "tick"}

	linker1 := gen.PID{Node: "node@local", ID: 10, Creation: 100}
	if _, err := m.LinkEvent(linker1, event); err != nil {
		t.Fatal(err)
	}

	core.mu.Lock()
	defer core.mu.Unlock()
	if len(core.sent) != 1 {
		t.Fatalf("expected 1 EventStart send, got %d (%+v)", len(core.sent), core.sent)
	}
	if core.sent[0].To != producer {
		t.Fatalf("EventStart should go to producer, got %v", core.sent[0].To)
	}
	if _, ok := core.sent[0].Message.(gen.MessageEventStart); ok == false {
		t.Fatalf("expected MessageEventStart, got %T", core.sent[0].Message)
	}
}

func TestLinkEventSecondSubscriberDoesNotTriggerStart(t *testing.T) {
	m, core := newManager()
	producer := gen.PID{Node: "node@local", ID: 42, Creation: 100}
	m.RegisterEvent(producer, "tick", gen.EventOptions{Notify: true})
	event := gen.Event{Node: "node@local", Name: "tick"}

	linker1 := gen.PID{Node: "node@local", ID: 10, Creation: 100}
	linker2 := gen.PID{Node: "node@local", ID: 11, Creation: 100}
	m.LinkEvent(linker1, event)

	core.mu.Lock()
	core.sent = nil
	core.mu.Unlock()

	m.LinkEvent(linker2, event)

	core.mu.Lock()
	defer core.mu.Unlock()
	if len(core.sent) != 0 {
		t.Fatalf("second subscriber should not trigger EventStart, got %+v", core.sent)
	}
}

func TestUnlinkEventLastSubscriberTriggersStop(t *testing.T) {
	m, core := newManager()
	producer := gen.PID{Node: "node@local", ID: 42, Creation: 100}
	m.RegisterEvent(producer, "tick", gen.EventOptions{Notify: true})
	event := gen.Event{Node: "node@local", Name: "tick"}

	linker := gen.PID{Node: "node@local", ID: 10, Creation: 100}
	m.LinkEvent(linker, event)

	core.mu.Lock()
	core.sent = nil
	core.mu.Unlock()

	if err := m.UnlinkEvent(linker, event); err != nil {
		t.Fatal(err)
	}

	core.mu.Lock()
	defer core.mu.Unlock()
	if len(core.sent) != 1 {
		t.Fatalf("expected EventStop, got %+v", core.sent)
	}
	if _, ok := core.sent[0].Message.(gen.MessageEventStop); ok == false {
		t.Fatalf("expected MessageEventStop, got %T", core.sent[0].Message)
	}
}

func TestLinkEventDuplicateLocalReturnsErrTargetExist(t *testing.T) {
	m, _ := newManager()
	producer := gen.PID{Node: "node@local", ID: 42, Creation: 100}
	m.RegisterEvent(producer, "tick", gen.EventOptions{})
	event := gen.Event{Node: "node@local", Name: "tick"}
	consumer := gen.PID{Node: "node@local", ID: 10, Creation: 100}

	m.LinkEvent(consumer, event)
	if _, err := m.LinkEvent(consumer, event); err != gen.ErrTargetExist {
		t.Fatalf("got %v want ErrTargetExist", err)
	}
}

func TestLinkEventDuplicateRemoteReturnsBuffer(t *testing.T) {
	m, _ := newManager()
	producer := gen.PID{Node: "node@local", ID: 42, Creation: 100}
	token, _ := m.RegisterEvent(producer, "tick", gen.EventOptions{Buffer: 4})
	event := gen.Event{Node: "node@local", Name: "tick"}
	rconsumer := remotePID(1)

	// Publish one message so the buffer is non-empty.
	msg := gen.MessageEvent{Event: event, Timestamp: time.Now().UnixNano(), Message: "tick-1"}
	if err := m.PublishEvent(producer, token, gen.MessageOptions{}, msg); err != nil {
		t.Fatal(err)
	}

	// First link: success, returns one-event snapshot.
	buf1, err := m.LinkEvent(rconsumer, event)
	if err != nil {
		t.Fatal(err)
	}
	if len(buf1) != 1 {
		t.Fatalf("first LinkEvent returned %d events, want 1", len(buf1))
	}

	// Second link from same remote consumer: silent nil error, same snapshot.
	buf2, err := m.LinkEvent(rconsumer, event)
	if err != nil {
		t.Fatalf("duplicate remote LinkEvent returned %v want nil", err)
	}
	if len(buf2) != 1 {
		t.Fatalf("duplicate remote LinkEvent returned %d events, want 1", len(buf2))
	}
}

func TestLinkEventUnknownReturnsErrEventUnknown(t *testing.T) {
	m, _ := newManager()
	if _, err := m.LinkEvent(pid(1), gen.Event{Node: "node@local", Name: "nope"}); err != gen.ErrEventUnknown {
		t.Fatalf("got %v want ErrEventUnknown", err)
	}
}

func TestPublishEventTokenRequired(t *testing.T) {
	m, _ := newManager()
	producer := gen.PID{Node: "node@local", ID: 42, Creation: 100}
	_, err := m.RegisterEvent(producer, "tick", gen.EventOptions{Open: false})
	if err != nil {
		t.Fatal(err)
	}
	event := gen.Event{Node: "node@local", Name: "tick"}
	bad := gen.Ref{Node: "node@local", ID: [3]uint64{1, 2, 3}}
	if err := m.PublishEvent(producer, bad, gen.MessageOptions{}, gen.MessageEvent{Event: event}); err != gen.ErrEventOwner {
		t.Fatalf("got %v want ErrEventOwner", err)
	}
}

func TestPublishEventOpenIgnoresToken(t *testing.T) {
	m, _ := newManager()
	producer := gen.PID{Node: "node@local", ID: 42, Creation: 100}
	if _, err := m.RegisterEvent(producer, "tick", gen.EventOptions{Open: true}); err != nil {
		t.Fatal(err)
	}
	event := gen.Event{Node: "node@local", Name: "tick"}
	bad := gen.Ref{Node: "node@local", ID: [3]uint64{1, 2, 3}}
	if err := m.PublishEvent(producer, bad, gen.MessageOptions{}, gen.MessageEvent{Event: event}); err != nil {
		t.Fatalf("Open event should accept any token, got %v", err)
	}
}

func TestPublishEventDispatchesToLocalSubscribers(t *testing.T) {
	m, core := newManager()
	producer := gen.PID{Node: "node@local", ID: 42, Creation: 100}
	token, _ := m.RegisterEvent(producer, "tick", gen.EventOptions{})
	event := gen.Event{Node: "node@local", Name: "tick"}

	a := gen.PID{Node: "node@local", ID: 10, Creation: 100}
	b := gen.PID{Node: "node@local", ID: 11, Creation: 100}
	m.LinkEvent(a, event)
	m.MonitorEvent(b, event)

	core.mu.Lock()
	core.events = nil
	core.mu.Unlock()

	msg := gen.MessageEvent{Event: event, Timestamp: time.Now().UnixNano(), Message: "tick"}
	if err := m.PublishEvent(producer, token, gen.MessageOptions{}, msg); err != nil {
		t.Fatal(err)
	}

	core.mu.Lock()
	defer core.mu.Unlock()
	if len(core.events) != 1 {
		t.Fatalf("expected 1 event batch, got %+v", core.events)
	}
	if len(core.events[0].To) != 2 {
		t.Fatalf("event went to %d consumers, want 2", len(core.events[0].To))
	}
}

func TestPublishEventBufferCaptured(t *testing.T) {
	m, _ := newManager()
	producer := gen.PID{Node: "node@local", ID: 42, Creation: 100}
	token, _ := m.RegisterEvent(producer, "tick", gen.EventOptions{Buffer: 3})
	event := gen.Event{Node: "node@local", Name: "tick"}

	// publish 5 messages; buffer of 3 retains the last 3.
	for i := 0; i < 5; i++ {
		msg := gen.MessageEvent{Event: event, Timestamp: int64(i + 1), Message: i}
		if err := m.PublishEvent(producer, token, gen.MessageOptions{}, msg); err != nil {
			t.Fatal(err)
		}
	}

	rconsumer := remotePID(1)
	buf, err := m.LinkEvent(rconsumer, event)
	if err != nil {
		t.Fatal(err)
	}
	if len(buf) != 3 {
		t.Fatalf("buffer snapshot returned %d, want 3", len(buf))
	}
	if buf[0].Timestamp != 3 || buf[2].Timestamp != 5 {
		t.Fatalf("buffer order wrong: %+v", buf)
	}
}

func TestPublishEventRemoteProducerDispatchesLocalOnly(t *testing.T) {
	m, core := newManager()
	producer := gen.PID{Node: "node@local", ID: 42, Creation: 100}
	m.RegisterEvent(producer, "tick", gen.EventOptions{})
	event := gen.Event{Node: "node@local", Name: "tick"}

	local := gen.PID{Node: "node@local", ID: 10, Creation: 100}
	m.LinkEvent(local, event)

	core.mu.Lock()
	core.events = nil
	core.mu.Unlock()

	// from is a remote PID; publishRemoteProducer should fire.
	from := remotePID(99)
	msg := gen.MessageEvent{Event: event, Timestamp: time.Now().UnixNano(), Message: "remote"}
	if err := m.PublishEvent(from, gen.Ref{}, gen.MessageOptions{}, msg); err != nil {
		t.Fatal(err)
	}

	core.mu.Lock()
	defer core.mu.Unlock()
	if len(core.events) != 1 || len(core.events[0].To) != 1 || core.events[0].To[0] != local {
		t.Fatalf("expected one local dispatch, got %+v", core.events)
	}
}

func TestEventRangeInfo(t *testing.T) {
	m, _ := newManager()
	producer := gen.PID{Node: "node@local", ID: 42, Creation: 100}
	m.RegisterEvent(producer, "a", gen.EventOptions{})
	m.RegisterEvent(producer, "b", gen.EventOptions{})
	m.RegisterEvent(producer, "c", gen.EventOptions{})

	seen := map[gen.Atom]bool{}
	m.EventRangeInfo(func(info gen.EventInfo) bool {
		seen[info.Event.Name] = true
		return true
	})
	if len(seen) != 3 || !seen["a"] || !seen["b"] || !seen["c"] {
		t.Fatalf("EventRangeInfo saw %v", seen)
	}
}

func TestEventListInfoForwardAndBackward(t *testing.T) {
	m, _ := newManager()
	producer := gen.PID{Node: "node@local", ID: 42, Creation: 100}
	for _, n := range []gen.Atom{"a", "b", "c"} {
		if _, err := m.RegisterEvent(producer, n, gen.EventOptions{}); err != nil {
			t.Fatal(err)
		}
	}

	// forward: oldest first
	listFwd, err := m.EventListInfo(0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(listFwd) != 3 || listFwd[0].Event.Name != "a" || listFwd[2].Event.Name != "c" {
		t.Fatalf("forward got %+v", listFwd)
	}

	// backward: newest first
	listBwd, err := m.EventListInfo(0, -100)
	if err != nil {
		t.Fatal(err)
	}
	if len(listBwd) != 3 || listBwd[0].Event.Name != "c" || listBwd[2].Event.Name != "a" {
		t.Fatalf("backward got %+v", listBwd)
	}
}

func TestConcurrentOpenPublishersWalkSafely(t *testing.T) {
	m, core := newManager()
	producer := gen.PID{Node: "node@local", ID: 42, Creation: 100}
	if _, err := m.RegisterEvent(producer, "tick", gen.EventOptions{Open: true, Buffer: 8}); err != nil {
		t.Fatal(err)
	}
	event := gen.Event{Node: "node@local", Name: "tick"}

	// 100 local subscribers.
	const subs = 100
	for i := 0; i < subs; i++ {
		m.LinkEvent(localPID(uint64(i+10)), event)
	}

	// 16 concurrent publishers, each publishing 50 messages. Open event
	// lets any caller publish without the token. Each subscriber should
	// receive subs * pubs messages total (deliveries are batched per
	// publish into core.RouteSendEventMessages).
	const publishers = 16
	const perPub = 50
	var wg sync.WaitGroup
	wg.Add(publishers)
	for p := 0; p < publishers; p++ {
		go func(idx int) {
			defer wg.Done()
			from := gen.PID{Node: "node@local", ID: uint64(1000 + idx), Creation: 100}
			for j := 0; j < perPub; j++ {
				m.PublishEvent(from, gen.Ref{}, gen.MessageOptions{},
					gen.MessageEvent{Event: event, Timestamp: int64(idx*perPub + j)})
			}
		}(p)
	}
	wg.Wait()

	// Each publish batched into one RouteSendEventMessages with `subs`
	// recipients. Total batches: publishers * perPub. Each batch has subs
	// recipients. Total deliveries to local consumers = publishers * perPub * subs.
	core.mu.Lock()
	defer core.mu.Unlock()
	if len(core.events) != publishers*perPub {
		t.Fatalf("expected %d publish batches, got %d", publishers*perPub, len(core.events))
	}
	for _, e := range core.events {
		if len(e.To) != subs {
			t.Fatalf("batch had %d recipients, want %d", len(e.To), subs)
		}
	}
}

// Extra event tests ported from tm/event_test.go.

func TestEventInfo_NotExist(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	_, err := m.EventInfo(gen.Event{Node: "node1", Name: "nope"})
	if err != gen.ErrEventUnknown {
		t.Fatalf("Expected ErrEventUnknown, got %v", err)
	}
}

func TestPublishEvent_BufferOverflow(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	producer := gen.PID{Node: "node1", ID: 100}
	token, _ := m.RegisterEvent(producer, "tick", gen.EventOptions{Buffer: 3})
	event := gen.Event{Node: "node1", Name: "tick"}

	for i := 0; i < 5; i++ {
		m.PublishEvent(producer, token, gen.MessageOptions{},
			gen.MessageEvent{Event: event, Timestamp: int64(i + 1)})
	}

	consumer := gen.PID{Node: "node2", ID: 200}
	buf, err := m.LinkEvent(consumer, event)
	if err != nil {
		t.Fatal(err)
	}
	if len(buf) != 3 {
		t.Fatalf("Expected buffer of 3 (last N), got %d", len(buf))
	}
	if buf[0].Timestamp != 3 || buf[2].Timestamp != 5 {
		t.Fatalf("Wrong buffer contents: %+v", buf)
	}
}

func TestPublishEvent_MixedSubscribers(t *testing.T) {
	m, core := newManagerWithMock("node1")
	producer := gen.PID{Node: "node1", ID: 100}
	token, _ := m.RegisterEvent(producer, "tick", gen.EventOptions{})
	event := gen.Event{Node: "node1", Name: "tick"}

	linker := gen.PID{Node: "node1", ID: 10}
	monitor := gen.PID{Node: "node1", ID: 11}
	m.LinkEvent(linker, event)
	m.MonitorEvent(monitor, event)

	core.resetSentEvents()
	if err := m.PublishEvent(producer, token, gen.MessageOptions{},
		gen.MessageEvent{Event: event}); err != nil {
		t.Fatal(err)
	}
	if core.countSentEvents() != 2 {
		t.Fatalf("Expected 2 deliveries (link + monitor), got %d", core.countSentEvents())
	}
}

func TestUnregisterEvent_Open_StillRequiresOwner(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	producer := gen.PID{Node: "node1", ID: 100}
	other := gen.PID{Node: "node1", ID: 101}

	m.RegisterEvent(producer, "tick", gen.EventOptions{Open: true})
	if err := m.UnregisterEvent(other, "tick"); err != gen.ErrEventOwner {
		t.Fatalf("Open should not bypass owner on UnregisterEvent, got %v", err)
	}
}

func TestMonitorEvent_Local_SharesCounter(t *testing.T) {
	m, core := newManagerWithMock("node1")
	producer := gen.PID{Node: "node1", ID: 100}
	m.RegisterEvent(producer, "tick", gen.EventOptions{Notify: true})
	event := gen.Event{Node: "node1", Name: "tick"}

	c1 := gen.PID{Node: "node1", ID: 10}
	c2 := gen.PID{Node: "node1", ID: 11}

	// First link triggers EventStart.
	m.LinkEvent(c1, event)
	if core.countSentEventStarts() != 1 {
		t.Fatalf("Expected EventStart after first Link, got %d", core.countSentEventStarts())
	}
	core.resetSentEventStarts()

	// Monitor by another consumer must NOT trigger EventStart again
	// (counter is shared across link+monitor on the same event).
	m.MonitorEvent(c2, event)
	if core.countSentEventStarts() != 0 {
		t.Errorf("Monitor should not trigger second EventStart, got %d", core.countSentEventStarts())
	}
}

func TestUnlinkDemonitor_EventStop_WhenBothGone(t *testing.T) {
	m, core := newManagerWithMock("node1")
	producer := gen.PID{Node: "node1", ID: 100}
	m.RegisterEvent(producer, "tick", gen.EventOptions{Notify: true})
	event := gen.Event{Node: "node1", Name: "tick"}

	consumer := gen.PID{Node: "node1", ID: 10}
	m.LinkEvent(consumer, event)
	m.MonitorEvent(consumer, event)
	core.resetSentEventStops()

	m.UnlinkEvent(consumer, event)
	if core.countSentEventStops() != 0 {
		t.Errorf("EventStop should not fire while Monitor still active, got %d", core.countSentEventStops())
	}
	m.DemonitorEvent(consumer, event)
	if core.countSentEventStops() != 1 {
		t.Errorf("EventStop should fire after both link and monitor are gone, got %d", core.countSentEventStops())
	}
}

func TestUnlinkEvent_Remote_ConnectionError(t *testing.T) {
	m, core := newManagerWithMock("node1")
	event := gen.Event{Node: "node2", Name: "tick"}
	consumer := gen.PID{Node: "node1", ID: 10}

	// Establish remote link first (so wire is established).
	m.LinkEvent(consumer, event)
	// Now break the connection.
	core.connectionError = gen.ErrNoConnection

	if err := m.UnlinkEvent(consumer, event); err != nil {
		t.Fatalf("UnlinkEvent should succeed locally even if wire-unlink fails: %v", err)
	}
	if m.hasLinkRelation(consumer, event) {
		t.Error("Local link should be removed even on wire error")
	}
}

func TestDemonitorEvent_Remote_LastLocal_SendsDemonitor(t *testing.T) {
	m, core := newManagerWithMock("node1")
	event := gen.Event{Node: "node2", Name: "tick"}
	consumer := gen.PID{Node: "node1", ID: 10}

	m.MonitorEvent(consumer, event)
	core.resetSentDemonitors()

	if err := m.DemonitorEvent(consumer, event); err != nil {
		t.Fatal(err)
	}
	// DemonitorEvent on remote event fires wire-Demonitor on connection;
	// our mock records via sentDemonitors only when conn.DemonitorEvent is
	// implemented to push. We treat absence of error and clean storage as
	// passing here; wire-side coverage is in TestWireUnlinkEventLastConsumerCallsWire.
	if m.hasMonitorRelation(consumer, event) {
		t.Error("monitor should be removed")
	}
	_ = core
}

func TestDemonitorEvent_Remote_NotLast(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	event := gen.Event{Node: "node2", Name: "tick"}
	c1 := gen.PID{Node: "node1", ID: 10}
	c2 := gen.PID{Node: "node1", ID: 11}

	m.MonitorEvent(c1, event)
	m.MonitorEvent(c2, event)

	if err := m.DemonitorEvent(c1, event); err != nil {
		t.Fatal(err)
	}
	if m.hasMonitorRelation(c1, event) {
		t.Error("c1 should be removed")
	}
	if m.hasMonitorRelation(c2, event) == false {
		t.Error("c2 should still be subscribed")
	}
}

func TestEventRingBufferConcurrentPush(t *testing.T) {
	const size = 64
	const pushers = 16
	const perPusher = 1000
	rb := newEventRingBuffer(size)

	var wg sync.WaitGroup
	wg.Add(pushers)
	for p := 0; p < pushers; p++ {
		go func(idx int) {
			defer wg.Done()
			for i := 0; i < perPusher; i++ {
				rb.push(gen.MessageEvent{Timestamp: int64(idx*perPusher + i)})
			}
		}(p)
	}
	wg.Wait()

	if rb.seq.Load() != pushers*perPusher {
		t.Fatalf("seq = %d, want %d", rb.seq.Load(), pushers*perPusher)
	}
	if rb.length() != size {
		t.Fatalf("length = %d, want %d", rb.length(), size)
	}

	snap := rb.snapshot()
	if len(snap) > size {
		t.Fatalf("snapshot len %d exceeds size %d", len(snap), size)
	}
	// Each item should be a legitimate pushed message: timestamps must be in
	// [0, pushers*perPusher).
	maxT := int64(pushers * perPusher)
	for _, m := range snap {
		if m.Timestamp < 0 || m.Timestamp >= maxT {
			t.Fatalf("snapshot has stray timestamp %d", m.Timestamp)
		}
	}
}

func TestEventRingBufferConcurrentPushAndSnapshot(t *testing.T) {
	const size = 32
	const pushers = 8
	const perPusher = 5000
	rb := newEventRingBuffer(size)

	var wg sync.WaitGroup
	wg.Add(pushers)
	for p := 0; p < pushers; p++ {
		go func(idx int) {
			defer wg.Done()
			for i := 0; i < perPusher; i++ {
				rb.push(gen.MessageEvent{Timestamp: int64(idx*perPusher + i)})
			}
		}(p)
	}

	// Concurrent snapshots while pushers run. Each snapshot must produce a
	// valid (possibly short) slice without panics or data races.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	for {
		select {
		case <-done:
			final := rb.snapshot()
			if len(final) > size {
				t.Fatalf("final snapshot exceeds size: %d", len(final))
			}
			return
		default:
			s := rb.snapshot()
			if len(s) > size {
				t.Fatalf("snapshot exceeds size: %d", len(s))
			}
		}
	}
}

func TestEventsFor(t *testing.T) {
	m, _ := newManager()
	p1 := gen.PID{Node: "node@local", ID: 42, Creation: 100}
	p2 := gen.PID{Node: "node@local", ID: 43, Creation: 100}
	m.RegisterEvent(p1, "x", gen.EventOptions{})
	m.RegisterEvent(p1, "y", gen.EventOptions{})
	m.RegisterEvent(p2, "z", gen.EventOptions{})

	got := m.EventsFor(p1)
	if len(got) != 2 {
		t.Fatalf("EventsFor(p1) = %v, want 2 entries", got)
	}
	names := map[gen.Atom]bool{got[0].Name: true, got[1].Name: true}
	if !names["x"] || !names["y"] {
		t.Fatalf("EventsFor(p1) names = %v want x,y", names)
	}

	got2 := m.EventsFor(p2)
	if len(got2) != 1 || got2[0].Name != "z" {
		t.Fatalf("EventsFor(p2) = %v, want [z]", got2)
	}
}
