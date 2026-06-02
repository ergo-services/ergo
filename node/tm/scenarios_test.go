package tm

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"ergo.services/ergo/gen"
)

// Multiple consumers

func TestMultipleConsumers_Link_AllNotified(t *testing.T) {
	m, core := newManagerWithMock("node1")
	target := gen.PID{Node: "node1", ID: 200}
	consumers := []gen.PID{
		{Node: "node1", ID: 100},
		{Node: "node1", ID: 101},
		{Node: "node1", ID: 102},
	}
	for _, c := range consumers {
		m.LinkPID(c, target)
	}
	m.TerminatedTargetPID(target, gen.TerminateReasonNormal)
	if core.countSentExits() != 3 {
		t.Errorf("Expected 3 exits, got %d", core.countSentExits())
	}
}

func TestMultipleConsumers_Monitor_AllNotified(t *testing.T) {
	m, core := newManagerWithMock("node1")
	target := gen.PID{Node: "node1", ID: 200}
	consumers := []gen.PID{
		{Node: "node1", ID: 100},
		{Node: "node1", ID: 101},
		{Node: "node1", ID: 102},
	}
	for _, c := range consumers {
		m.MonitorPID(c, target)
	}
	m.TerminatedTargetPID(target, gen.TerminateReasonNormal)
	if core.countSentDowns() != 3 {
		t.Errorf("Expected 3 downs, got %d", core.countSentDowns())
	}
}

// Node down scenarios

func TestNodeDown_NotifiesLinkedPID(t *testing.T) {
	m, core := newManagerWithMock("node1")
	doomed := gen.Atom("doomed")
	target := gen.PID{Node: doomed, ID: 200}
	consumer := gen.PID{Node: "node1", ID: 100}

	m.LinkPID(consumer, target)
	core.resetSentExits()

	m.TerminatedTargetNode(doomed, gen.ErrNoConnection)
	if core.countSentExits() != 1 {
		t.Errorf("Expected 1 exit, got %d", core.countSentExits())
	}
}

func TestNodeDown_NotifiesMonitoringPID(t *testing.T) {
	m, core := newManagerWithMock("node1")
	doomed := gen.Atom("doomed")
	target := gen.PID{Node: doomed, ID: 200}
	consumer := gen.PID{Node: "node1", ID: 100}

	m.MonitorPID(consumer, target)
	core.resetSentDowns()

	m.TerminatedTargetNode(doomed, gen.ErrNoConnection)
	if core.countSentDowns() != 1 {
		t.Errorf("Expected 1 down, got %d", core.countSentDowns())
	}
}

func TestNodeDown_NotifiesLinkedAlias(t *testing.T) {
	m, core := newManagerWithMock("node1")
	doomed := gen.Atom("doomed")
	target := gen.Alias{Node: doomed, ID: [3]uint64{1, 2, 3}}
	consumer := gen.PID{Node: "node1", ID: 100}

	m.LinkAlias(consumer, target)
	core.resetSentExits()

	m.TerminatedTargetNode(doomed, gen.ErrNoConnection)
	if core.countSentExits() != 1 {
		t.Errorf("Expected 1 exit, got %d", core.countSentExits())
	}
}

func TestNodeDown_NotifiesLinkedProcessID(t *testing.T) {
	m, core := newManagerWithMock("node1")
	doomed := gen.Atom("doomed")
	target := gen.ProcessID{Node: doomed, Name: "proc"}
	consumer := gen.PID{Node: "node1", ID: 100}

	m.LinkProcessID(consumer, target)
	core.resetSentExits()

	m.TerminatedTargetNode(doomed, gen.ErrNoConnection)
	if core.countSentExits() != 1 {
		t.Errorf("Expected 1 exit, got %d", core.countSentExits())
	}
}

func TestNodeDown_NotifiesLinkedEvent(t *testing.T) {
	m, core := newManagerWithMock("node1")
	doomed := gen.Atom("doomed")
	target := gen.Event{Node: doomed, Name: "evt"}
	consumer := gen.PID{Node: "node1", ID: 100}

	m.LinkEvent(consumer, target)
	core.resetSentExits()

	m.TerminatedTargetNode(doomed, gen.ErrNoConnection)
	if core.countSentExits() != 1 {
		t.Errorf("Expected 1 exit, got %d", core.countSentExits())
	}
}

// Link/Monitor terminate reasons

func TestLink_PID_TerminateReasonKill(t *testing.T) {
	m, core := newManagerWithMock("node1")
	target := gen.PID{Node: "node1", ID: 200}
	consumer := gen.PID{Node: "node1", ID: 100}
	m.LinkPID(consumer, target)

	m.TerminatedTargetPID(target, gen.TerminateReasonKill)
	if exit, ok := core.getFirstSentExit(); ok {
		em := exit.message.(gen.MessageExitPID)
		if em.Reason != gen.TerminateReasonKill {
			t.Errorf("Wrong reason: %v", em.Reason)
		}
	}
}

func TestLink_PID_TerminateReasonShutdown(t *testing.T) {
	m, core := newManagerWithMock("node1")
	target := gen.PID{Node: "node1", ID: 200}
	consumer := gen.PID{Node: "node1", ID: 100}
	m.LinkPID(consumer, target)

	m.TerminatedTargetPID(target, gen.TerminateReasonShutdown)
	if exit, ok := core.getFirstSentExit(); ok {
		em := exit.message.(gen.MessageExitPID)
		if em.Reason != gen.TerminateReasonShutdown {
			t.Errorf("Wrong reason: %v", em.Reason)
		}
	}
}

func TestLink_PID_CustomReason(t *testing.T) {
	m, core := newManagerWithMock("node1")
	target := gen.PID{Node: "node1", ID: 200}
	consumer := gen.PID{Node: "node1", ID: 100}
	m.LinkPID(consumer, target)

	custom := errors.New("custom")
	m.TerminatedTargetPID(target, custom)
	if exit, ok := core.getFirstSentExit(); ok {
		em := exit.message.(gen.MessageExitPID)
		if em.Reason != custom {
			t.Errorf("Wrong reason: %v", em.Reason)
		}
	}
}

func TestMonitor_PID_CustomReason(t *testing.T) {
	m, core := newManagerWithMock("node1")
	target := gen.PID{Node: "node1", ID: 200}
	consumer := gen.PID{Node: "node1", ID: 100}
	m.MonitorPID(consumer, target)

	custom := errors.New("boom")
	m.TerminatedTargetPID(target, custom)
	if d, ok := core.getFirstSentDown(); ok {
		dm := d.message.(gen.MessageDownPID)
		if dm.Reason != custom {
			t.Errorf("Wrong reason: %v", dm.Reason)
		}
	}
}

// Corner cases

func TestCorner_ReSubscribe(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node1", ID: 200}

	m.LinkPID(consumer, target)
	m.UnlinkPID(consumer, target)
	if err := m.LinkPID(consumer, target); err != nil {
		t.Fatalf("Re-link should succeed, got %v", err)
	}
	if m.hasLinkRelation(consumer, target) == false {
		t.Error("Re-link should be recorded")
	}
}

func TestCorner_HasAfterError(t *testing.T) {
	m, core := newManagerWithMock("node1")
	core.connectionError = gen.ErrNoConnection
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node2", ID: 200}

	m.LinkPID(consumer, target)
	// After rollback, Has should return false.
	if m.HasLink(consumer, target) {
		t.Error("HasLink should be false after rollback")
	}
}

func TestCorner_LinkFromTwoNodes(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	target := gen.PID{Node: "node1", ID: 200}
	c1 := gen.PID{Node: "node1", ID: 100} // local
	c2 := gen.PID{Node: "node3", ID: 100} // remote

	m.LinkPID(c1, target)
	m.LinkPID(c2, target)
	if m.consumerCount(target) != 2 {
		t.Errorf("Expected 2 consumers, got %d", m.consumerCount(target))
	}
}

func TestCorner_ProcessID_EmptyNode(t *testing.T) {
	m, core := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.ProcessID{Name: "proc"} // empty Node treated as local

	m.LinkProcessID(consumer, target)
	if core.countSentLinks() != 0 {
		t.Errorf("Empty Node ProcessID should be local, got %d wire sends", core.countSentLinks())
	}
}

func TestCorner_PublishEvent_NoSubscribers(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	producer := gen.PID{Node: "node1", ID: 100}
	token, _ := m.RegisterEvent(producer, "tick", gen.EventOptions{})
	event := gen.Event{Node: "node1", Name: "tick"}

	// Publishing with no subscribers must not error.
	if err := m.PublishEvent(producer, token, gen.MessageOptions{},
		gen.MessageEvent{Event: event}); err != nil {
		t.Fatalf("Publish with no subscribers: %v", err)
	}
}

func TestCorner_Event_EmptyBufferVsNil(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	producer := gen.PID{Node: "node1", ID: 100}

	// Without buffer.
	m.RegisterEvent(producer, "nobuf", gen.EventOptions{})
	event1 := gen.Event{Node: "node1", Name: "nobuf"}
	c := gen.PID{Node: "node1", ID: 10}
	buf, _ := m.LinkEvent(c, event1)
	if buf != nil {
		t.Errorf("No-buffer event should return nil snapshot, got %v", buf)
	}

	// With buffer, never published.
	m.RegisterEvent(producer, "withbuf", gen.EventOptions{Buffer: 4})
	event2 := gen.Event{Node: "node1", Name: "withbuf"}
	buf2, _ := m.LinkEvent(c, event2)
	if buf2 == nil {
		t.Error("Buffered event should return non-nil (possibly empty) snapshot")
	}
}

func TestCorner_TerminatedProcess_NoSubscriptions(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	pid := gen.PID{Node: "node1", ID: 100}
	// Process with no subscriptions; should not panic.
	m.TerminatedProcess(pid, gen.TerminateReasonNormal)
}

func TestCorner_TargetIndexCleanedAfterRollback(t *testing.T) {
	m, core := newManagerWithMock("node1")
	core.connectionError = gen.ErrNoConnection
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node2", ID: 200}

	m.LinkPID(consumer, target)
	// After rollback, consumer should be gone from target index.
	if m.consumerCount(target) != 0 {
		t.Errorf("target index should be empty after rollback, got %d", m.consumerCount(target))
	}
}

// Scenario tests

func TestScenario_DuplicateLink_ReturnsError(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node1", ID: 200}
	m.LinkPID(consumer, target)
	if err := m.LinkPID(consumer, target); err != gen.ErrTargetExist {
		t.Errorf("Duplicate link should be ErrTargetExist, got %v", err)
	}
}

func TestScenario_DuplicateMonitor_ReturnsError(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node1", ID: 200}
	m.MonitorPID(consumer, target)
	if err := m.MonitorPID(consumer, target); err != gen.ErrTargetExist {
		t.Errorf("Duplicate monitor should be ErrTargetExist, got %v", err)
	}
}

func TestScenario_UnlinkNonExistent_IsIdempotent(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node1", ID: 200}
	if err := m.UnlinkPID(consumer, target); err != nil {
		t.Errorf("Unlink non-existent should be idempotent, got %v", err)
	}
}

func TestScenario_DemonitorNonExistent_IsIdempotent(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node1", ID: 200}
	if err := m.DemonitorPID(consumer, target); err != nil {
		t.Errorf("Demonitor non-existent should be idempotent, got %v", err)
	}
}

func TestScenario_MixedLinkMonitor_SameTarget(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node1", ID: 200}

	if err := m.LinkPID(consumer, target); err != nil {
		t.Fatal(err)
	}
	if err := m.MonitorPID(consumer, target); err != nil {
		t.Fatal(err)
	}
	if m.HasLink(consumer, target) == false {
		t.Error("link should exist")
	}
	if m.HasMonitor(consumer, target) == false {
		t.Error("monitor should exist")
	}
}

func TestScenario_PartialUnlink_NoNetworkUnlinkUntilLast(t *testing.T) {
	m, core := newManagerWithMock("node1")
	target := gen.PID{Node: "node2", ID: 200}
	c1 := gen.PID{Node: "node1", ID: 100}
	c2 := gen.PID{Node: "node1", ID: 101}
	c3 := gen.PID{Node: "node1", ID: 102}

	m.LinkPID(c1, target)
	m.LinkPID(c2, target)
	m.LinkPID(c3, target)
	core.resetSentUnlinks()

	m.UnlinkPID(c1, target)
	m.UnlinkPID(c2, target)
	if core.countSentUnlinks() != 0 {
		t.Errorf("Wire UnlinkPID should not fire until last consumer, got %d", core.countSentUnlinks())
	}
	m.UnlinkPID(c3, target)
	if core.countSentUnlinks() != 1 {
		t.Errorf("Wire UnlinkPID should fire once after last consumer, got %d", core.countSentUnlinks())
	}
}

func TestScenario_PartialDemonitor_NoNetworkDemonitorUntilLast(t *testing.T) {
	m, core := newManagerWithMock("node1")
	target := gen.PID{Node: "node2", ID: 200}
	c1 := gen.PID{Node: "node1", ID: 100}
	c2 := gen.PID{Node: "node1", ID: 101}

	m.MonitorPID(c1, target)
	m.MonitorPID(c2, target)
	core.resetSentDemonitors()

	m.DemonitorPID(c1, target)
	if core.countSentDemonitors() != 0 {
		t.Errorf("Wire DemonitorPID should not fire while c2 remains, got %d", core.countSentDemonitors())
	}
	m.DemonitorPID(c2, target)
	if core.countSentDemonitors() != 1 {
		t.Errorf("Wire DemonitorPID should fire once after last consumer, got %d", core.countSentDemonitors())
	}
}

func TestScenario_MultipleConsumers_SameRemoteTarget_Link(t *testing.T) {
	m, core := newManagerWithMock("node1")
	target := gen.PID{Node: "node2", ID: 200}
	for i := 0; i < 5; i++ {
		m.LinkPID(gen.PID{Node: "node1", ID: uint64(100 + i)}, target)
	}
	if core.countSentLinks() != 1 {
		t.Errorf("Expected exactly 1 wire LinkPID for 5 local consumers, got %d", core.countSentLinks())
	}
	if m.totalLinks() != 5 {
		t.Errorf("Expected 5 link relations, got %d", m.totalLinks())
	}
}

func TestScenario_MultipleConsumers_SameRemoteTarget_Monitor(t *testing.T) {
	m, core := newManagerWithMock("node1")
	target := gen.PID{Node: "node2", ID: 200}
	for i := 0; i < 5; i++ {
		m.MonitorPID(gen.PID{Node: "node1", ID: uint64(100 + i)}, target)
	}
	if core.countSentMonitors() != 1 {
		t.Errorf("Expected exactly 1 wire MonitorPID for 5 local consumers, got %d", core.countSentMonitors())
	}
}

func TestScenario_RemoteTargetTermination_NotifiesAllLocalConsumers(t *testing.T) {
	m, core := newManagerWithMock("node1")
	target := gen.PID{Node: "node2", ID: 200}
	consumers := []gen.PID{
		{Node: "node1", ID: 100},
		{Node: "node1", ID: 101},
	}
	for _, c := range consumers {
		m.LinkPID(c, target)
	}
	core.resetSentExits()

	m.TerminatedTargetPID(target, gen.TerminateReasonNormal)
	if core.countSentExits() != 2 {
		t.Errorf("Expected 2 exits, got %d", core.countSentExits())
	}
}

func TestScenario_RemoteTargetTermination_NotifiesAllLocalMonitors(t *testing.T) {
	m, core := newManagerWithMock("node1")
	target := gen.PID{Node: "node2", ID: 200}
	consumers := []gen.PID{
		{Node: "node1", ID: 100},
		{Node: "node1", ID: 101},
	}
	for _, c := range consumers {
		m.MonitorPID(c, target)
	}
	core.resetSentDowns()

	m.TerminatedTargetPID(target, gen.TerminateReasonNormal)
	if core.countSentDowns() != 2 {
		t.Errorf("Expected 2 downs, got %d", core.countSentDowns())
	}
}

func TestScenario_ProcessTermination_CleansUpAllRelations(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	t1 := gen.PID{Node: "node1", ID: 200}
	t2 := gen.PID{Node: "node1", ID: 201}
	m.LinkPID(consumer, t1)
	m.MonitorPID(consumer, t2)

	m.TerminatedProcess(consumer, gen.TerminateReasonNormal)
	if m.HasLink(consumer, t1) || m.HasMonitor(consumer, t2) {
		t.Error("All relations should be cleared")
	}
}

func TestScenario_TerminationReasons_Link(t *testing.T) {
	m, core := newManagerWithMock("node1")
	target := gen.PID{Node: "node1", ID: 200}
	consumer := gen.PID{Node: "node1", ID: 100}
	m.LinkPID(consumer, target)

	reasons := []error{
		gen.TerminateReasonNormal,
		gen.TerminateReasonShutdown,
		gen.TerminateReasonKill,
	}
	for _, r := range reasons {
		core.resetSentExits()
		m.LinkPID(consumer, target)
		m.TerminatedTargetPID(target, r)
		if exit, ok := core.getFirstSentExit(); ok {
			em := exit.message.(gen.MessageExitPID)
			if em.Reason != r {
				t.Errorf("reason %v: got %v", r, em.Reason)
			}
		}
	}
}

// Stress tests; race detector must stay clean.

func TestStress_1000ConcurrentLinks(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	const n = 1000
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			c := gen.PID{Node: "node1", ID: uint64(100 + idx)}
			tgt := gen.PID{Node: "node1", ID: uint64(10000 + idx)}
			m.LinkPID(c, tgt)
		}(i)
	}
	wg.Wait()
	if m.totalLinks() != n {
		t.Errorf("Expected %d links, got %d", n, m.totalLinks())
	}
}

func TestStress_ConcurrentAddRemove(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	const n = 500
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			c := gen.PID{Node: "node1", ID: uint64(100 + idx)}
			tgt := gen.PID{Node: "node1", ID: 9999}
			m.LinkPID(c, tgt)
			m.UnlinkPID(c, tgt)
		}(i)
	}
	wg.Wait()
	if m.totalLinks() != 0 {
		t.Errorf("Expected 0 links after add/remove, got %d", m.totalLinks())
	}
}

func TestStress_RapidSubscribeUnsubscribe(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	producer := gen.PID{Node: "node1", ID: 1}
	m.RegisterEvent(producer, "tick", gen.EventOptions{})
	event := gen.Event{Node: "node1", Name: "tick"}

	const cycles = 200
	const subs = 16
	var wg sync.WaitGroup
	wg.Add(subs)
	for s := 0; s < subs; s++ {
		go func(idx int) {
			defer wg.Done()
			c := gen.PID{Node: "node1", ID: uint64(100 + idx)}
			for j := 0; j < cycles; j++ {
				m.LinkEvent(c, event)
				m.UnlinkEvent(c, event)
			}
		}(s)
	}
	wg.Wait()
}

func TestStress_MassTermination(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	const n = 200
	consumers := make([]gen.PID, n)
	for i := 0; i < n; i++ {
		consumers[i] = gen.PID{Node: "node1", ID: uint64(100 + i)}
		target := gen.PID{Node: "node1", ID: uint64(10000 + i)}
		m.LinkPID(consumers[i], target)
	}
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			target := gen.PID{Node: "node1", ID: uint64(10000 + idx)}
			m.TerminatedTargetPID(target, gen.TerminateReasonNormal)
		}(i)
	}
	wg.Wait()
	if m.totalLinks() != 0 {
		t.Errorf("Expected 0 links after mass termination, got %d", m.totalLinks())
	}
}

func TestStress_Event_1000Subscribers(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	producer := gen.PID{Node: "node1", ID: 1}
	m.RegisterEvent(producer, "tick", gen.EventOptions{})
	event := gen.Event{Node: "node1", Name: "tick"}

	const n = 1000
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			c := gen.PID{Node: "node1", ID: uint64(100 + idx)}
			m.LinkEvent(c, event)
		}(i)
	}
	wg.Wait()
	if m.consumerCount(event) != n {
		t.Errorf("Expected %d consumers, got %d", n, m.consumerCount(event))
	}
}

func TestStress_Event_RapidPublish(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	producer := gen.PID{Node: "node1", ID: 1}
	token, _ := m.RegisterEvent(producer, "tick", gen.EventOptions{Buffer: 16})
	event := gen.Event{Node: "node1", Name: "tick"}

	for i := 0; i < 50; i++ {
		m.LinkEvent(gen.PID{Node: "node1", ID: uint64(10 + i)}, event)
	}

	const pubs = 1000
	var wg sync.WaitGroup
	wg.Add(pubs)
	for j := 0; j < pubs; j++ {
		go func(idx int) {
			defer wg.Done()
			m.PublishEvent(producer, token, gen.MessageOptions{},
				gen.MessageEvent{Event: event, Timestamp: int64(idx)})
		}(j)
	}
	wg.Wait()
}

func TestStress_Event_ConcurrentOperations(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	producer := gen.PID{Node: "node1", ID: 1}
	token, _ := m.RegisterEvent(producer, "tick", gen.EventOptions{Open: true, Buffer: 8})
	event := gen.Event{Node: "node1", Name: "tick"}

	const workers = 16
	var wg sync.WaitGroup
	wg.Add(workers * 3)

	// Subscribers.
	for i := 0; i < workers; i++ {
		go func(idx int) {
			defer wg.Done()
			c := gen.PID{Node: "node1", ID: uint64(100 + idx)}
			for j := 0; j < 50; j++ {
				m.LinkEvent(c, event)
				m.UnlinkEvent(c, event)
			}
		}(i)
	}
	// Publishers.
	for i := 0; i < workers; i++ {
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				m.PublishEvent(producer, token, gen.MessageOptions{},
					gen.MessageEvent{Event: event, Timestamp: int64(idx*50 + j)})
			}
		}(i)
	}
	// Monitors.
	for i := 0; i < workers; i++ {
		go func(idx int) {
			defer wg.Done()
			c := gen.PID{Node: "node1", ID: uint64(200 + idx)}
			for j := 0; j < 50; j++ {
				m.MonitorEvent(c, event)
				m.DemonitorEvent(c, event)
			}
		}(i)
	}
	wg.Wait()
}

func TestStress_Memory_10KCycles(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node1", ID: 200}
	for i := 0; i < 10000; i++ {
		m.LinkPID(consumer, target)
		m.UnlinkPID(consumer, target)
	}
	if m.totalLinks() != 0 {
		t.Errorf("Expected 0 links after cycles, got %d", m.totalLinks())
	}
}

func TestStress_MixedOperations(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	const n = 500
	var counter atomic.Int64
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			c := gen.PID{Node: "node1", ID: uint64(100 + idx)}
			tgt := gen.PID{Node: "node1", ID: uint64(10000 + (idx % 50))}
			switch idx % 4 {
			case 0:
				m.LinkPID(c, tgt)
			case 1:
				m.MonitorPID(c, tgt)
			case 2:
				m.LinkPID(c, tgt)
				m.UnlinkPID(c, tgt)
			case 3:
				m.MonitorPID(c, tgt)
				m.DemonitorPID(c, tgt)
			}
			counter.Add(1)
		}(i)
	}
	wg.Wait()
	if counter.Load() != int64(n) {
		t.Errorf("Expected %d ops, got %d", n, counter.Load())
	}
}

func TestStress_TerminatedNode_1000Subscriptions(t *testing.T) {
	m, _ := newManagerWithMock("node1")
	doomed := gen.Atom("doomed")
	const n = 1000
	for i := 0; i < n; i++ {
		c := gen.PID{Node: "node1", ID: uint64(100 + i)}
		tgt := gen.PID{Node: doomed, ID: uint64(10000 + i)}
		m.LinkPID(c, tgt)
	}
	m.TerminatedTargetNode(doomed, gen.ErrNoConnection)
	if m.totalLinks() != 0 {
		t.Errorf("Expected 0 links after node down, got %d", m.totalLinks())
	}
}

func TestScenario_TerminationReasons_Monitor(t *testing.T) {
	m, core := newManagerWithMock("node1")
	target := gen.PID{Node: "node1", ID: 200}
	consumer := gen.PID{Node: "node1", ID: 100}

	reasons := []error{
		gen.TerminateReasonNormal,
		gen.TerminateReasonShutdown,
		gen.TerminateReasonKill,
	}
	for _, r := range reasons {
		core.resetSentDowns()
		m.MonitorPID(consumer, target)
		m.TerminatedTargetPID(target, r)
		if d, ok := core.getFirstSentDown(); ok {
			dm := d.message.(gen.MessageDownPID)
			if dm.Reason != r {
				t.Errorf("reason %v: got %v", r, dm.Reason)
			}
		}
	}
}
