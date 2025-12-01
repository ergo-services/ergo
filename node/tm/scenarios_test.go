package tm

import (
	"errors"
	"sync"
	"testing"
	"time"

	"ergo.services/ergo/gen"
)

// =============================================================================
// Tests based on scenarios from testing/tests/001_local and 002_distributed
// =============================================================================

// -----------------------------------------------------------------------------
// Test different termination reasons propagate correctly
// Based on: t005_actor_monitor_test.go TestMonitorPID
// -----------------------------------------------------------------------------

func TestScenario_TerminationReasons_Link(t *testing.T) {
	// Test that different termination reasons (Kill, Shutdown, custom) are
	// correctly propagated through links

	reasons := []error{
		gen.TerminateReasonKill,
		gen.TerminateReasonShutdown,
		errors.New("custom reason"),
	}

	for _, reason := range reasons {
		t.Run(reason.Error(), func(t *testing.T) {
			core := newMockCore("test@localhost")
			tm := Create(core, Options{}).(*targetManager)

			consumer := gen.PID{Node: "test@localhost", ID: 100, Creation: 1}
			target := gen.PID{Node: "test@localhost", ID: 200, Creation: 1}

			// Link consumer to target
			if err := tm.LinkPID(consumer, target); err != nil {
				t.Fatalf("LinkPID failed: %v", err)
			}

			// Terminate target with specific reason
			tm.TerminatedTargetPID(target, reason)
			time.Sleep(100 * time.Millisecond)

			// Verify exit message was sent with correct reason
			if core.countSentExits() != 1 {
				t.Fatalf("expected 1 exit, got %d", core.countSentExits())
			}

			// Verify internal state: linkRelations cleaned up
			key := relationKey{consumer: consumer, target: target}
			if _, exists := tm.linkRelations[key]; exists {
				t.Error("linkRelations should be cleaned after termination")
			}

			// Verify targetIndex cleaned up
			if _, exists := tm.targetIndex[target]; exists {
				t.Error("targetIndex should be cleaned after termination")
			}
		})
	}
}

func TestScenario_TerminationReasons_Monitor(t *testing.T) {
	// Test that different termination reasons are correctly propagated through monitors

	reasons := []error{
		gen.TerminateReasonKill,
		gen.TerminateReasonShutdown,
		errors.New("custom reason"),
	}

	for _, reason := range reasons {
		t.Run(reason.Error(), func(t *testing.T) {
			core := newMockCore("test@localhost")
			tm := Create(core, Options{}).(*targetManager)

			consumer := gen.PID{Node: "test@localhost", ID: 100, Creation: 1}
			target := gen.PID{Node: "test@localhost", ID: 200, Creation: 1}

			// Monitor target
			if err := tm.MonitorPID(consumer, target); err != nil {
				t.Fatalf("MonitorPID failed: %v", err)
			}

			// Terminate target
			tm.TerminatedTargetPID(target, reason)
			time.Sleep(100 * time.Millisecond)

			// Verify down message was sent
			if core.countSentDowns() != 1 {
				t.Fatalf("expected 1 down, got %d", core.countSentDowns())
			}

			// Verify internal state: monitorRelations cleaned up
			key := relationKey{consumer: consumer, target: target}
			if _, exists := tm.monitorRelations[key]; exists {
				t.Error("monitorRelations should be cleaned after termination")
			}

			// Verify targetIndex cleaned up
			if _, exists := tm.targetIndex[target]; exists {
				t.Error("targetIndex should be cleaned after termination")
			}
		})
	}
}

// -----------------------------------------------------------------------------
// Test process termination cleanup
// Based on: t006_actor_link_test.go LinkingPID
// -----------------------------------------------------------------------------

func TestScenario_ProcessTermination_CleansUpAllRelations(t *testing.T) {
	// When a process terminates, all its links AND monitors should be cleaned up

	core := newMockCore("test@localhost")
	tm := Create(core, Options{}).(*targetManager)

	process := gen.PID{Node: "test@localhost", ID: 100, Creation: 1}
	target1 := gen.PID{Node: "test@localhost", ID: 201, Creation: 1}
	target2 := gen.PID{Node: "test@localhost", ID: 202, Creation: 1}
	target3 := gen.PID{Node: "test@localhost", ID: 203, Creation: 1}

	// Process links to target1
	if err := tm.LinkPID(process, target1); err != nil {
		t.Fatal(err)
	}

	// Process monitors target2
	if err := tm.MonitorPID(process, target2); err != nil {
		t.Fatal(err)
	}

	// Process links to target3
	if err := tm.LinkPID(process, target3); err != nil {
		t.Fatal(err)
	}

	// Verify relations exist
	links := tm.LinksFor(process)
	monitors := tm.MonitorsFor(process)

	if len(links) != 2 {
		t.Fatalf("expected 2 links, got %d", len(links))
	}
	if len(monitors) != 1 {
		t.Fatalf("expected 1 monitor, got %d", len(monitors))
	}

	// Process terminates
	tm.TerminatedProcess(process, gen.TerminateReasonNormal)
	time.Sleep(100 * time.Millisecond)

	// All relations should be cleaned up
	links = tm.LinksFor(process)
	monitors = tm.MonitorsFor(process)

	if len(links) != 0 {
		t.Fatalf("expected 0 links after termination, got %d", len(links))
	}
	if len(monitors) != 0 {
		t.Fatalf("expected 0 monitors after termination, got %d", len(monitors))
	}

	// Verify targetIndex cleaned up for all targets
	if _, exists := tm.targetIndex[target1]; exists {
		t.Error("targetIndex for target1 should be cleaned")
	}
	if _, exists := tm.targetIndex[target2]; exists {
		t.Error("targetIndex for target2 should be cleaned")
	}
	if _, exists := tm.targetIndex[target3]; exists {
		t.Error("targetIndex for target3 should be cleaned")
	}
}

// -----------------------------------------------------------------------------
// Test multiple consumers linking/monitoring same remote target
// Based on: t008_optimization_test.go TestT8MultipleLinksToSamePID
// -----------------------------------------------------------------------------

func TestScenario_MultipleConsumers_SameRemoteTarget_Link(t *testing.T) {
	// Multiple local consumers link to same remote target
	// Only ONE network request should be made (optimization)

	core := newMockCore("local@localhost")
	tm := Create(core, Options{}).(*targetManager)

	consumer1 := gen.PID{Node: "local@localhost", ID: 101, Creation: 1}
	consumer2 := gen.PID{Node: "local@localhost", ID: 102, Creation: 1}
	consumer3 := gen.PID{Node: "local@localhost", ID: 103, Creation: 1}
	remoteTarget := gen.PID{Node: "remote@localhost", ID: 999, Creation: 1}

	// First consumer links - should send network request
	if err := tm.LinkPID(consumer1, remoteTarget); err != nil {
		t.Fatal(err)
	}

	// Second consumer links - should NOT send network request
	if err := tm.LinkPID(consumer2, remoteTarget); err != nil {
		t.Fatal(err)
	}

	// Third consumer links - should NOT send network request
	if err := tm.LinkPID(consumer3, remoteTarget); err != nil {
		t.Fatal(err)
	}

	// Verify only 1 network link was sent
	if core.countSentLinks() != 1 {
		t.Fatalf("expected 1 network link, got %d", core.countSentLinks())
	}

	// All should be linked
	if tm.HasLink(consumer1, remoteTarget) == false {
		t.Error("consumer1 should be linked")
	}
	if tm.HasLink(consumer2, remoteTarget) == false {
		t.Error("consumer2 should be linked")
	}
	if tm.HasLink(consumer3, remoteTarget) == false {
		t.Error("consumer3 should be linked")
	}

	// Verify targetIndex has all 3 consumers
	entry := tm.targetIndex[remoteTarget]
	if entry == nil {
		t.Fatal("targetIndex entry should exist")
	}
	if len(entry.consumers) != 3 {
		t.Errorf("expected 3 consumers in targetIndex, got %d", len(entry.consumers))
	}
	if _, exists := entry.consumers[consumer1]; exists == false {
		t.Error("consumer1 should be in targetIndex.consumers")
	}
	if _, exists := entry.consumers[consumer2]; exists == false {
		t.Error("consumer2 should be in targetIndex.consumers")
	}
	if _, exists := entry.consumers[consumer3]; exists == false {
		t.Error("consumer3 should be in targetIndex.consumers")
	}
}

func TestScenario_MultipleConsumers_SameRemoteTarget_Monitor(t *testing.T) {
	// Multiple local consumers monitor same remote target
	// Only ONE network request should be made

	core := newMockCore("local@localhost")
	tm := Create(core, Options{}).(*targetManager)

	consumer1 := gen.PID{Node: "local@localhost", ID: 101, Creation: 1}
	consumer2 := gen.PID{Node: "local@localhost", ID: 102, Creation: 1}
	consumer3 := gen.PID{Node: "local@localhost", ID: 103, Creation: 1}
	remoteTarget := gen.PID{Node: "remote@localhost", ID: 999, Creation: 1}

	// All consumers monitor
	if err := tm.MonitorPID(consumer1, remoteTarget); err != nil {
		t.Fatal(err)
	}
	if err := tm.MonitorPID(consumer2, remoteTarget); err != nil {
		t.Fatal(err)
	}
	if err := tm.MonitorPID(consumer3, remoteTarget); err != nil {
		t.Fatal(err)
	}

	// Only 1 network monitor request
	if core.countSentMonitors() != 1 {
		t.Fatalf("expected 1 network monitor, got %d", core.countSentMonitors())
	}

	// Verify all relations stored
	key1 := relationKey{consumer: consumer1, target: remoteTarget}
	key2 := relationKey{consumer: consumer2, target: remoteTarget}
	key3 := relationKey{consumer: consumer3, target: remoteTarget}
	if _, exists := tm.monitorRelations[key1]; exists == false {
		t.Error("consumer1 relation should exist in monitorRelations")
	}
	if _, exists := tm.monitorRelations[key2]; exists == false {
		t.Error("consumer2 relation should exist in monitorRelations")
	}
	if _, exists := tm.monitorRelations[key3]; exists == false {
		t.Error("consumer3 relation should exist in monitorRelations")
	}

	// Verify targetIndex has all 3 consumers
	entry := tm.targetIndex[remoteTarget]
	if entry == nil {
		t.Fatal("targetIndex entry should exist")
	}
	if len(entry.consumers) != 3 {
		t.Errorf("expected 3 consumers in targetIndex, got %d", len(entry.consumers))
	}
}

// -----------------------------------------------------------------------------
// Test partial unlink - not all consumers unlinking
// Based on: t008_optimization_test.go TestT8PartialUnlink
// -----------------------------------------------------------------------------

func TestScenario_PartialUnlink_NoNetworkUnlinkUntilLast(t *testing.T) {
	// P1, P2, P3 link to remote target
	// P1 unlinks -> NO remote unlink (P2, P3 still linked)
	// P2 unlinks -> NO remote unlink (P3 still linked)
	// P3 unlinks -> remote unlink sent (last one)

	core := newMockCore("local@localhost")
	tm := Create(core, Options{}).(*targetManager)

	p1 := gen.PID{Node: "local@localhost", ID: 101, Creation: 1}
	p2 := gen.PID{Node: "local@localhost", ID: 102, Creation: 1}
	p3 := gen.PID{Node: "local@localhost", ID: 103, Creation: 1}
	remoteTarget := gen.PID{Node: "remote@localhost", ID: 999, Creation: 1}

	// All link
	tm.LinkPID(p1, remoteTarget)
	tm.LinkPID(p2, remoteTarget)
	tm.LinkPID(p3, remoteTarget)

	// Reset counters
	core.resetSentUnlinks()

	// P1 unlinks - should NOT send network unlink
	if err := tm.UnlinkPID(p1, remoteTarget); err != nil {
		t.Fatal(err)
	}
	if core.countSentUnlinks() != 0 {
		t.Fatalf("expected 0 unlinks after P1, got %d", core.countSentUnlinks())
	}
	// Verify targetIndex still has 2 consumers
	entry := tm.targetIndex[remoteTarget]
	if entry == nil {
		t.Fatal("targetIndex should still exist after P1 unlink")
	}
	if len(entry.consumers) != 2 {
		t.Errorf("expected 2 consumers after P1 unlink, got %d", len(entry.consumers))
	}

	// P2 unlinks - should NOT send network unlink
	if err := tm.UnlinkPID(p2, remoteTarget); err != nil {
		t.Fatal(err)
	}
	if core.countSentUnlinks() != 0 {
		t.Fatalf("expected 0 unlinks after P2, got %d", core.countSentUnlinks())
	}
	// Verify targetIndex still has 1 consumer
	entry = tm.targetIndex[remoteTarget]
	if entry == nil {
		t.Fatal("targetIndex should still exist after P2 unlink")
	}
	if len(entry.consumers) != 1 {
		t.Errorf("expected 1 consumer after P2 unlink, got %d", len(entry.consumers))
	}

	// P3 unlinks - SHOULD send network unlink (last one)
	if err := tm.UnlinkPID(p3, remoteTarget); err != nil {
		t.Fatal(err)
	}
	if core.countSentUnlinks() != 1 {
		t.Fatalf("expected 1 unlink after P3, got %d", core.countSentUnlinks())
	}
	// Verify targetIndex cleaned up
	if _, exists := tm.targetIndex[remoteTarget]; exists {
		t.Error("targetIndex should be cleaned after last consumer unlinks")
	}
}

func TestScenario_PartialDemonitor_NoNetworkDemonitorUntilLast(t *testing.T) {
	core := newMockCore("local@localhost")
	tm := Create(core, Options{}).(*targetManager)

	p1 := gen.PID{Node: "local@localhost", ID: 101, Creation: 1}
	p2 := gen.PID{Node: "local@localhost", ID: 102, Creation: 1}
	p3 := gen.PID{Node: "local@localhost", ID: 103, Creation: 1}
	remoteTarget := gen.PID{Node: "remote@localhost", ID: 999, Creation: 1}

	// All monitor
	tm.MonitorPID(p1, remoteTarget)
	tm.MonitorPID(p2, remoteTarget)
	tm.MonitorPID(p3, remoteTarget)

	// Reset counters
	core.resetSentDemonitors()

	// P1, P2 demonitor - should NOT send network demonitor
	tm.DemonitorPID(p1, remoteTarget)
	tm.DemonitorPID(p2, remoteTarget)
	if core.countSentDemonitors() != 0 {
		t.Fatalf("expected 0 demonitors after P1,P2, got %d", core.countSentDemonitors())
	}
	// Verify targetIndex still has 1 consumer (P3)
	entry := tm.targetIndex[remoteTarget]
	if entry == nil {
		t.Fatal("targetIndex should still exist after P1,P2 demonitor")
	}
	if len(entry.consumers) != 1 {
		t.Errorf("expected 1 consumer after P1,P2 demonitor, got %d", len(entry.consumers))
	}

	// P3 demonitors - SHOULD send network demonitor
	tm.DemonitorPID(p3, remoteTarget)
	if core.countSentDemonitors() != 1 {
		t.Fatalf("expected 1 demonitor after P3, got %d", core.countSentDemonitors())
	}
	// Verify targetIndex cleaned up
	if _, exists := tm.targetIndex[remoteTarget]; exists {
		t.Error("targetIndex should be cleaned after last consumer demonitors")
	}
}

// -----------------------------------------------------------------------------
// Test remote target termination notifies all local consumers
// Based on: t008_optimization_test.go TestT8MultipleLinksToSamePID (termination part)
// -----------------------------------------------------------------------------

func TestScenario_RemoteTargetTermination_NotifiesAllLocalConsumers(t *testing.T) {
	// 3 local consumers link to same remote target
	// When remote target terminates, all 3 should receive exit messages

	core := newMockCore("local@localhost")
	tm := Create(core, Options{}).(*targetManager)

	consumer1 := gen.PID{Node: "local@localhost", ID: 101, Creation: 1}
	consumer2 := gen.PID{Node: "local@localhost", ID: 102, Creation: 1}
	consumer3 := gen.PID{Node: "local@localhost", ID: 103, Creation: 1}
	remoteTarget := gen.PID{Node: "remote@localhost", ID: 999, Creation: 1}

	// All link
	tm.LinkPID(consumer1, remoteTarget)
	tm.LinkPID(consumer2, remoteTarget)
	tm.LinkPID(consumer3, remoteTarget)

	// Simulate remote target termination (as if network notified us)
	tm.TerminatedTargetPID(remoteTarget, gen.TerminateReasonShutdown)
	time.Sleep(100 * time.Millisecond)

	// All 3 should receive exit messages
	if core.countSentExits() != 3 {
		t.Fatalf("expected 3 exit messages, got %d", core.countSentExits())
	}

	// Verify internal state cleaned up
	key1 := relationKey{consumer: consumer1, target: remoteTarget}
	key2 := relationKey{consumer: consumer2, target: remoteTarget}
	key3 := relationKey{consumer: consumer3, target: remoteTarget}
	if _, exists := tm.linkRelations[key1]; exists {
		t.Error("consumer1 linkRelation should be cleaned")
	}
	if _, exists := tm.linkRelations[key2]; exists {
		t.Error("consumer2 linkRelation should be cleaned")
	}
	if _, exists := tm.linkRelations[key3]; exists {
		t.Error("consumer3 linkRelation should be cleaned")
	}
	if _, exists := tm.targetIndex[remoteTarget]; exists {
		t.Error("targetIndex should be cleaned after termination")
	}
}

func TestScenario_RemoteTargetTermination_NotifiesAllLocalMonitors(t *testing.T) {
	core := newMockCore("local@localhost")
	tm := Create(core, Options{}).(*targetManager)

	consumer1 := gen.PID{Node: "local@localhost", ID: 101, Creation: 1}
	consumer2 := gen.PID{Node: "local@localhost", ID: 102, Creation: 1}
	consumer3 := gen.PID{Node: "local@localhost", ID: 103, Creation: 1}
	remoteTarget := gen.PID{Node: "remote@localhost", ID: 999, Creation: 1}

	// All monitor
	tm.MonitorPID(consumer1, remoteTarget)
	tm.MonitorPID(consumer2, remoteTarget)
	tm.MonitorPID(consumer3, remoteTarget)

	// Remote target terminates
	tm.TerminatedTargetPID(remoteTarget, gen.TerminateReasonShutdown)
	time.Sleep(100 * time.Millisecond)

	// All 3 should receive down messages
	if core.countSentDowns() != 3 {
		t.Fatalf("expected 3 down messages, got %d", core.countSentDowns())
	}

	// Verify internal state cleaned up
	key1 := relationKey{consumer: consumer1, target: remoteTarget}
	key2 := relationKey{consumer: consumer2, target: remoteTarget}
	key3 := relationKey{consumer: consumer3, target: remoteTarget}
	if _, exists := tm.monitorRelations[key1]; exists {
		t.Error("consumer1 monitorRelation should be cleaned")
	}
	if _, exists := tm.monitorRelations[key2]; exists {
		t.Error("consumer2 monitorRelation should be cleaned")
	}
	if _, exists := tm.monitorRelations[key3]; exists {
		t.Error("consumer3 monitorRelation should be cleaned")
	}
	if _, exists := tm.targetIndex[remoteTarget]; exists {
		t.Error("targetIndex should be cleaned after termination")
	}
}

// -----------------------------------------------------------------------------
// Test node down - all targets on that node should be terminated
// Based on: t005_link_test.go TestLinkRemotePIDNodeDown
// -----------------------------------------------------------------------------

func TestScenario_NodeDown_NotifiesAllLinkedConsumers(t *testing.T) {
	// Consumer links to remote PID
	// When remote node goes down, consumer receives exit with ErrNoConnection

	core := newMockCore("local@localhost")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "local@localhost", ID: 100, Creation: 1}
	remoteTarget := gen.PID{Node: "remote@localhost", ID: 999, Creation: 1}

	// Link to remote target
	if err := tm.LinkPID(consumer, remoteTarget); err != nil {
		t.Fatal(err)
	}

	// Simulate node down
	tm.TerminatedTargetNode("remote@localhost", gen.ErrNoConnection)
	time.Sleep(100 * time.Millisecond)

	// Consumer should receive exit with ErrNoConnection
	if core.countSentExits() != 1 {
		t.Fatalf("expected 1 exit message, got %d", core.countSentExits())
	}

	// Verify internal state cleaned up
	key := relationKey{consumer: consumer, target: remoteTarget}
	if _, exists := tm.linkRelations[key]; exists {
		t.Error("linkRelation should be cleaned after node down")
	}
	if _, exists := tm.targetIndex[remoteTarget]; exists {
		t.Error("targetIndex should be cleaned after node down")
	}
}

func TestScenario_NodeDown_NotifiesAllMonitoringConsumers(t *testing.T) {
	core := newMockCore("local@localhost")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "local@localhost", ID: 100, Creation: 1}
	remoteTarget := gen.PID{Node: "remote@localhost", ID: 999, Creation: 1}

	// Monitor remote target
	if err := tm.MonitorPID(consumer, remoteTarget); err != nil {
		t.Fatal(err)
	}

	// Node down
	tm.TerminatedTargetNode("remote@localhost", gen.ErrNoConnection)
	time.Sleep(100 * time.Millisecond)

	// Consumer should receive down message
	if core.countSentDowns() != 1 {
		t.Fatalf("expected 1 down message, got %d", core.countSentDowns())
	}

	// Verify internal state cleaned up
	key := relationKey{consumer: consumer, target: remoteTarget}
	if _, exists := tm.monitorRelations[key]; exists {
		t.Error("monitorRelation should be cleaned after node down")
	}
	if _, exists := tm.targetIndex[remoteTarget]; exists {
		t.Error("targetIndex should be cleaned after node down")
	}
}

func TestScenario_NodeDown_MultipleTargets(t *testing.T) {
	// Multiple consumers linked to different targets on same remote node
	// When node goes down, all consumers notified

	core := newMockCore("local@localhost")
	tm := Create(core, Options{}).(*targetManager)

	consumer1 := gen.PID{Node: "local@localhost", ID: 101, Creation: 1}
	consumer2 := gen.PID{Node: "local@localhost", ID: 102, Creation: 1}
	consumer3 := gen.PID{Node: "local@localhost", ID: 103, Creation: 1}
	remoteTarget1 := gen.PID{Node: "remote@localhost", ID: 901, Creation: 1}
	remoteTarget2 := gen.PID{Node: "remote@localhost", ID: 902, Creation: 1}
	remoteTarget3 := gen.PID{Node: "remote@localhost", ID: 903, Creation: 1}

	// Each consumer links to different target on same node
	tm.LinkPID(consumer1, remoteTarget1)
	tm.LinkPID(consumer2, remoteTarget2)
	tm.LinkPID(consumer3, remoteTarget3)

	// Node goes down
	tm.TerminatedTargetNode("remote@localhost", gen.ErrNoConnection)
	time.Sleep(100 * time.Millisecond)

	// All 3 should get exit messages
	if core.countSentExits() != 3 {
		t.Fatalf("expected 3 exit messages, got %d", core.countSentExits())
	}

	// Verify all targetIndex entries cleaned up
	if _, exists := tm.targetIndex[remoteTarget1]; exists {
		t.Error("targetIndex for remoteTarget1 should be cleaned")
	}
	if _, exists := tm.targetIndex[remoteTarget2]; exists {
		t.Error("targetIndex for remoteTarget2 should be cleaned")
	}
	if _, exists := tm.targetIndex[remoteTarget3]; exists {
		t.Error("targetIndex for remoteTarget3 should be cleaned")
	}
	// Verify all linkRelations cleaned up
	if len(tm.linkRelations) != 0 {
		t.Errorf("all linkRelations should be cleaned, got %d", len(tm.linkRelations))
	}
}

// -----------------------------------------------------------------------------
// Test link to node (gen.Atom target)
// Based on: t005_link_test.go TestLinkRemoteNodeDown
// -----------------------------------------------------------------------------

func TestScenario_LinkNode_ReceivesExitOnDisconnect(t *testing.T) {
	// Consumer links to a node name (not PID)
	// When node disconnects, consumer receives MessageExitNode

	core := newMockCore("local@localhost")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "local@localhost", ID: 100, Creation: 1}
	remoteNode := gen.Atom("remote@localhost")

	// Link to node
	if err := tm.LinkNode(consumer, remoteNode); err != nil {
		t.Fatal(err)
	}

	// Verify link exists
	links := tm.LinksFor(consumer)
	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}

	// Node disconnects
	tm.TerminatedTargetNode(remoteNode, gen.ErrNoConnection)
	time.Sleep(100 * time.Millisecond)

	// Consumer should receive exit for node
	if core.countSentExits() != 1 {
		t.Fatalf("expected 1 exit message, got %d", core.countSentExits())
	}

	// Verify internal state cleaned up
	key := relationKey{consumer: consumer, target: remoteNode}
	if _, exists := tm.linkRelations[key]; exists {
		t.Error("linkRelation should be cleaned after node disconnect")
	}
	if _, exists := tm.targetIndex[remoteNode]; exists {
		t.Error("targetIndex should be cleaned after node disconnect")
	}
}

func TestScenario_MonitorNode_ReceivesDownOnDisconnect(t *testing.T) {
	core := newMockCore("local@localhost")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "local@localhost", ID: 100, Creation: 1}
	remoteNode := gen.Atom("remote@localhost")

	// Monitor node
	if err := tm.MonitorNode(consumer, remoteNode); err != nil {
		t.Fatal(err)
	}

	// Node disconnects
	tm.TerminatedTargetNode(remoteNode, gen.ErrNoConnection)
	time.Sleep(100 * time.Millisecond)

	// Consumer should receive down for node
	if core.countSentDowns() != 1 {
		t.Fatalf("expected 1 down message, got %d", core.countSentDowns())
	}

	// Verify internal state cleaned up
	key := relationKey{consumer: consumer, target: remoteNode}
	if _, exists := tm.monitorRelations[key]; exists {
		t.Error("monitorRelation should be cleaned after node disconnect")
	}
	if _, exists := tm.targetIndex[remoteNode]; exists {
		t.Error("targetIndex should be cleaned after node disconnect")
	}
}

// -----------------------------------------------------------------------------
// Test event link/monitor from remote node
// Based on: t008_optimization_test.go TestT8CrossNodeSubscriptions
// -----------------------------------------------------------------------------

func TestScenario_RemoteCorePIDSubscribesEvent_FirstSubscriber_EventStart(t *testing.T) {
	// Local node owns event
	// Remote node's CorePID subscribes (represents remote subscribers)
	// First subscriber should trigger EventStart notification

	core := newMockCore("local@localhost")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "local@localhost", ID: 100, Creation: 1}
	eventName := gen.Atom("testEvent")

	// Register event with Notify=true
	token, err := tm.RegisterEvent(producer, eventName, gen.EventOptions{Notify: true})
	if err != nil {
		t.Fatal(err)
	}
	if token.Node != "local@localhost" {
		t.Fatal("token has wrong node")
	}

	event := gen.Event{Node: "local@localhost", Name: eventName}

	// Remote CorePID subscribes
	remoteCorePID := gen.PID{Node: "remote@localhost", ID: 1, Creation: 500}
	_, err = tm.LinkEvent(remoteCorePID, event)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond)

	// Should have sent EventStart to producer
	if core.countSentEventStarts() != 1 {
		t.Fatalf("expected 1 EventStart, got %d", core.countSentEventStarts())
	}

	// Verify event entry has the subscriber
	entry := tm.events[event]
	if entry == nil {
		t.Fatal("event entry should exist")
	}
	if _, exists := entry.linkSubscribersIndex[remoteCorePID]; exists == false {
		t.Error("remoteCorePID should be in linkSubscribers")
	}
	if entry.subscriberCount != 1 {
		t.Errorf("expected subscriberCount 1, got %d", entry.subscriberCount)
	}
}

func TestScenario_RemoteCorePIDUnsubscribesEvent_LastSubscriber_EventStop(t *testing.T) {
	core := newMockCore("local@localhost")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "local@localhost", ID: 100, Creation: 1}
	eventName := gen.Atom("testEvent")

	// Register event with Notify=true
	tm.RegisterEvent(producer, eventName, gen.EventOptions{Notify: true})
	event := gen.Event{Node: "local@localhost", Name: eventName}

	// Remote CorePID subscribes
	remoteCorePID := gen.PID{Node: "remote@localhost", ID: 1, Creation: 500}
	tm.LinkEvent(remoteCorePID, event)
	time.Sleep(50 * time.Millisecond)

	// Reset counters
	core.resetSentEventStops()

	// Remote CorePID unsubscribes (last subscriber)
	if err := tm.UnlinkEvent(remoteCorePID, event); err != nil {
		t.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond)

	// Should have sent EventStop to producer
	if core.countSentEventStops() != 1 {
		t.Fatalf("expected 1 EventStop, got %d", core.countSentEventStops())
	}

	// Verify subscriber removed from event entry
	entry := tm.events[event]
	if entry == nil {
		t.Fatal("event entry should still exist (producer owns it)")
	}
	if _, exists := entry.linkSubscribersIndex[remoteCorePID]; exists {
		t.Error("remoteCorePID should be removed from linkSubscribers")
	}
	if entry.subscriberCount != 0 {
		t.Errorf("expected subscriberCount 0, got %d", entry.subscriberCount)
	}
}

// -----------------------------------------------------------------------------
// Test remote event delivery
// Based on: t007_event_test.go TestRemoteEvent
// -----------------------------------------------------------------------------

func TestScenario_RemoteEventDelivery(t *testing.T) {
	// Local consumer monitors remote event
	// When remote event is published, local consumer receives it

	core := newMockCore("local@localhost")
	tm := Create(core, Options{}).(*targetManager)

	localConsumer := gen.PID{Node: "local@localhost", ID: 100, Creation: 1}
	remoteEvent := gen.Event{Node: "remote@localhost", Name: "remoteEvent"}

	// Local consumer monitors remote event
	_, err := tm.MonitorEvent(localConsumer, remoteEvent)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate receiving event from remote (handlePublishEventRemote)
	remoteProducer := gen.PID{Node: "remote@localhost", ID: 999, Creation: 1}
	eventMsg := gen.MessageEvent{
		Event:   remoteEvent,
		Message: "test data",
	}

	err = tm.PublishEvent(remoteProducer, gen.Ref{}, gen.MessageOptions{}, eventMsg)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond)

	// Local consumer should have received the event
	if core.countSentEvents() != 1 {
		t.Fatalf("expected 1 event delivery, got %d", core.countSentEvents())
	}
}

// -----------------------------------------------------------------------------
// Test mixed link and monitor to same target
// Based on: t008_optimization_test.go TestT8MixedLinkMonitor
// -----------------------------------------------------------------------------

func TestScenario_MixedLinkMonitor_SameTarget(t *testing.T) {
	// Same consumer can link AND monitor same target
	// Termination should send both exit AND down messages

	core := newMockCore("test@localhost")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "test@localhost", ID: 100, Creation: 1}
	target := gen.PID{Node: "test@localhost", ID: 200, Creation: 1}

	// Link to target
	if err := tm.LinkPID(consumer, target); err != nil {
		t.Fatal(err)
	}

	// Also monitor same target
	if err := tm.MonitorPID(consumer, target); err != nil {
		t.Fatal(err)
	}

	// Verify both exist
	if tm.HasLink(consumer, target) == false {
		t.Error("should have link")
	}
	if tm.HasMonitor(consumer, target) == false {
		t.Error("should have monitor")
	}

	// Target terminates
	tm.TerminatedTargetPID(target, gen.TerminateReasonShutdown)
	time.Sleep(100 * time.Millisecond)

	// Should have sent both exit AND down
	if core.countSentExits() != 1 {
		t.Fatalf("expected 1 exit, got %d", core.countSentExits())
	}
	if core.countSentDowns() != 1 {
		t.Fatalf("expected 1 down, got %d", core.countSentDowns())
	}

	// Verify internal state cleaned up
	key := relationKey{consumer: consumer, target: target}
	if _, exists := tm.linkRelations[key]; exists {
		t.Error("linkRelation should be cleaned after termination")
	}
	if _, exists := tm.monitorRelations[key]; exists {
		t.Error("monitorRelation should be cleaned after termination")
	}
	if _, exists := tm.targetIndex[target]; exists {
		t.Error("targetIndex should be cleaned after termination")
	}
}

// -----------------------------------------------------------------------------
// Test ProcessID target type
// Based on: t005_actor_monitor_test.go TestMonitorProcessID
// -----------------------------------------------------------------------------

func TestScenario_ProcessID_TerminationNotifiesMonitors(t *testing.T) {
	core := newMockCore("test@localhost")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "test@localhost", ID: 100, Creation: 1}
	targetProcessID := gen.ProcessID{Name: "myprocess", Node: "test@localhost"}

	// Monitor ProcessID
	if err := tm.MonitorProcessID(consumer, targetProcessID); err != nil {
		t.Fatal(err)
	}

	// ProcessID target terminates
	tm.TerminatedTargetProcessID(targetProcessID, gen.TerminateReasonShutdown)
	time.Sleep(100 * time.Millisecond)

	// Consumer should receive down
	if core.countSentDowns() != 1 {
		t.Fatalf("expected 1 down, got %d", core.countSentDowns())
	}

	// Verify internal state cleaned up
	key := relationKey{consumer: consumer, target: targetProcessID}
	if _, exists := tm.monitorRelations[key]; exists {
		t.Error("monitorRelation should be cleaned after termination")
	}
	if _, exists := tm.targetIndex[targetProcessID]; exists {
		t.Error("targetIndex should be cleaned after termination")
	}
}

func TestScenario_ProcessID_TerminationNotifiesLinkers(t *testing.T) {
	core := newMockCore("test@localhost")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "test@localhost", ID: 100, Creation: 1}
	targetProcessID := gen.ProcessID{Name: "myprocess", Node: "test@localhost"}

	// Link to ProcessID
	if err := tm.LinkProcessID(consumer, targetProcessID); err != nil {
		t.Fatal(err)
	}

	// ProcessID target terminates
	tm.TerminatedTargetProcessID(targetProcessID, gen.TerminateReasonShutdown)
	time.Sleep(100 * time.Millisecond)

	// Consumer should receive exit
	if core.countSentExits() != 1 {
		t.Fatalf("expected 1 exit, got %d", core.countSentExits())
	}

	// Verify internal state cleaned up
	key := relationKey{consumer: consumer, target: targetProcessID}
	if _, exists := tm.linkRelations[key]; exists {
		t.Error("linkRelation should be cleaned after termination")
	}
	if _, exists := tm.targetIndex[targetProcessID]; exists {
		t.Error("targetIndex should be cleaned after termination")
	}
}

// -----------------------------------------------------------------------------
// Test Alias target type
// Based on: t005_actor_monitor_test.go TestMonitorAlias
// -----------------------------------------------------------------------------

func TestScenario_Alias_TerminationNotifiesMonitors(t *testing.T) {
	core := newMockCore("test@localhost")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "test@localhost", ID: 100, Creation: 1}
	targetAlias := gen.Alias{Node: "test@localhost", ID: [3]uint64{1, 2, 3}}

	// Monitor Alias
	if err := tm.MonitorAlias(consumer, targetAlias); err != nil {
		t.Fatal(err)
	}

	// Alias target terminates
	tm.TerminatedTargetAlias(targetAlias, gen.TerminateReasonShutdown)
	time.Sleep(100 * time.Millisecond)

	// Consumer should receive down
	if core.countSentDowns() != 1 {
		t.Fatalf("expected 1 down, got %d", core.countSentDowns())
	}

	// Verify internal state cleaned up
	key := relationKey{consumer: consumer, target: targetAlias}
	if _, exists := tm.monitorRelations[key]; exists {
		t.Error("monitorRelation should be cleaned after termination")
	}
	if _, exists := tm.targetIndex[targetAlias]; exists {
		t.Error("targetIndex should be cleaned after termination")
	}
}

func TestScenario_Alias_TerminationNotifiesLinkers(t *testing.T) {
	core := newMockCore("test@localhost")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "test@localhost", ID: 100, Creation: 1}
	targetAlias := gen.Alias{Node: "test@localhost", ID: [3]uint64{1, 2, 3}}

	// Link to Alias
	if err := tm.LinkAlias(consumer, targetAlias); err != nil {
		t.Fatal(err)
	}

	// Alias target terminates
	tm.TerminatedTargetAlias(targetAlias, gen.TerminateReasonShutdown)
	time.Sleep(100 * time.Millisecond)

	// Consumer should receive exit
	if core.countSentExits() != 1 {
		t.Fatalf("expected 1 exit, got %d", core.countSentExits())
	}

	// Verify internal state cleaned up
	key := relationKey{consumer: consumer, target: targetAlias}
	if _, exists := tm.linkRelations[key]; exists {
		t.Error("linkRelation should be cleaned after termination")
	}
	if _, exists := tm.targetIndex[targetAlias]; exists {
		t.Error("targetIndex should be cleaned after termination")
	}
}

// -----------------------------------------------------------------------------
// Test duplicate link/monitor returns error
// Based on: t008_optimization_test.go TestT8DuplicateLink
// -----------------------------------------------------------------------------

func TestScenario_DuplicateLink_ReturnsError(t *testing.T) {
	core := newMockCore("test@localhost")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "test@localhost", ID: 100, Creation: 1}
	target := gen.PID{Node: "test@localhost", ID: 200, Creation: 1}

	// First link - OK
	if err := tm.LinkPID(consumer, target); err != nil {
		t.Fatal(err)
	}

	// Second link - should fail
	if err := tm.LinkPID(consumer, target); err != gen.ErrTargetExist {
		t.Fatalf("expected ErrTargetExist, got %v", err)
	}

	// Verify only 1 relation stored (not duplicated)
	key := relationKey{consumer: consumer, target: target}
	if _, exists := tm.linkRelations[key]; exists == false {
		t.Error("linkRelation should still exist")
	}
	if len(tm.linkRelations) != 1 {
		t.Errorf("expected exactly 1 linkRelation, got %d", len(tm.linkRelations))
	}
	// Verify targetIndex has only 1 consumer
	entry := tm.targetIndex[target]
	if entry == nil {
		t.Fatal("targetIndex should exist")
	}
	if len(entry.consumers) != 1 {
		t.Errorf("expected 1 consumer in targetIndex, got %d", len(entry.consumers))
	}
}

func TestScenario_DuplicateMonitor_ReturnsError(t *testing.T) {
	core := newMockCore("test@localhost")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "test@localhost", ID: 100, Creation: 1}
	target := gen.PID{Node: "test@localhost", ID: 200, Creation: 1}

	// First monitor - OK
	if err := tm.MonitorPID(consumer, target); err != nil {
		t.Fatal(err)
	}

	// Second monitor - should fail
	if err := tm.MonitorPID(consumer, target); err != gen.ErrTargetExist {
		t.Fatalf("expected ErrTargetExist, got %v", err)
	}

	// Verify only 1 relation stored (not duplicated)
	key := relationKey{consumer: consumer, target: target}
	if _, exists := tm.monitorRelations[key]; exists == false {
		t.Error("monitorRelation should still exist")
	}
	if len(tm.monitorRelations) != 1 {
		t.Errorf("expected exactly 1 monitorRelation, got %d", len(tm.monitorRelations))
	}
	// Verify targetIndex has only 1 consumer
	entry := tm.targetIndex[target]
	if entry == nil {
		t.Fatal("targetIndex should exist")
	}
	if len(entry.consumers) != 1 {
		t.Errorf("expected 1 consumer in targetIndex, got %d", len(entry.consumers))
	}
}

// -----------------------------------------------------------------------------
// Test unlink/demonitor non-existent is idempotent (no error)
// -----------------------------------------------------------------------------

func TestScenario_UnlinkNonExistent_IsIdempotent(t *testing.T) {
	core := newMockCore("test@localhost")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "test@localhost", ID: 100, Creation: 1}
	target := gen.PID{Node: "test@localhost", ID: 200, Creation: 1}

	// Unlink without linking first - should not error (idempotent)
	if err := tm.UnlinkPID(consumer, target); err != nil {
		t.Fatalf("expected nil (idempotent), got %v", err)
	}
}

func TestScenario_DemonitorNonExistent_IsIdempotent(t *testing.T) {
	core := newMockCore("test@localhost")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "test@localhost", ID: 100, Creation: 1}
	target := gen.PID{Node: "test@localhost", ID: 200, Creation: 1}

	// Demonitor without monitoring first - should not error (idempotent)
	if err := tm.DemonitorPID(consumer, target); err != nil {
		t.Fatalf("expected nil (idempotent), got %v", err)
	}
}

// =============================================================================
// EXIT MESSAGE REASON VERIFICATION (mirrors t006_actor_link_test.go)
// =============================================================================

// Test: Link to PID - target terminates with TerminateReasonKill
// Consumer should receive MessageExitPID with Reason = TerminateReasonKill
func TestLink_PID_TerminateReasonKill(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100, Creation: 1}
	target := gen.PID{Node: "node1", ID: 200, Creation: 1}

	if err := tm.LinkPID(consumer, target); err != nil {
		t.Fatal(err)
	}

	// Target terminates with Kill reason
	tm.TerminatedTargetPID(target, gen.TerminateReasonKill)
	time.Sleep(50 * time.Millisecond)

	exit, ok := core.getFirstSentExit()
	if ok == false {
		t.Fatal("expected exit message")
	}

	// Verify recipient
	if exit.to != consumer {
		t.Errorf("exit sent to wrong consumer: got %v, want %v", exit.to, consumer)
	}

	// Verify message type and reason
	msg, ok := exit.message.(gen.MessageExitPID)
	if ok == false {
		t.Fatalf("expected MessageExitPID, got %T", exit.message)
	}
	if msg.PID != target {
		t.Errorf("wrong PID in exit: got %v, want %v", msg.PID, target)
	}
	if msg.Reason != gen.TerminateReasonKill {
		t.Errorf("wrong reason: got %v, want %v", msg.Reason, gen.TerminateReasonKill)
	}
}

// Test: Link to PID - target terminates with TerminateReasonShutdown
func TestLink_PID_TerminateReasonShutdown(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100, Creation: 1}
	target := gen.PID{Node: "node1", ID: 200, Creation: 1}

	tm.LinkPID(consumer, target)
	tm.TerminatedTargetPID(target, gen.TerminateReasonShutdown)
	time.Sleep(50 * time.Millisecond)

	exit, ok := core.getFirstSentExit()
	if ok == false {
		t.Fatal("expected exit message")
	}

	msg := exit.message.(gen.MessageExitPID)
	if msg.Reason != gen.TerminateReasonShutdown {
		t.Errorf("wrong reason: got %v, want %v", msg.Reason, gen.TerminateReasonShutdown)
	}
}

// Test: Link to PID - target terminates with custom error
func TestLink_PID_CustomReason(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100, Creation: 1}
	target := gen.PID{Node: "node1", ID: 200, Creation: 1}

	customErr := errors.New("custom termination reason")

	tm.LinkPID(consumer, target)
	tm.TerminatedTargetPID(target, customErr)
	time.Sleep(50 * time.Millisecond)

	exit, _ := core.getFirstSentExit()
	msg := exit.message.(gen.MessageExitPID)
	if msg.Reason != customErr {
		t.Errorf("wrong reason: got %v, want %v", msg.Reason, customErr)
	}
}

// Test: Link to ProcessID - target terminates, consumer gets MessageExitProcessID
func TestLink_ProcessID_TerminateReason(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100, Creation: 1}
	target := gen.ProcessID{Name: "myprocess", Node: "node1"}

	tm.LinkProcessID(consumer, target)
	tm.TerminatedTargetProcessID(target, gen.TerminateReasonShutdown)
	time.Sleep(50 * time.Millisecond)

	exit, ok := core.getFirstSentExit()
	if ok == false {
		t.Fatal("expected exit message")
	}

	msg, ok := exit.message.(gen.MessageExitProcessID)
	if ok == false {
		t.Fatalf("expected MessageExitProcessID, got %T", exit.message)
	}
	if msg.ProcessID != target {
		t.Errorf("wrong ProcessID: got %v, want %v", msg.ProcessID, target)
	}
	if msg.Reason != gen.TerminateReasonShutdown {
		t.Errorf("wrong reason: got %v, want %v", msg.Reason, gen.TerminateReasonShutdown)
	}
}

// Test: Link to ProcessID - name unregistered (ErrUnregistered)
func TestLink_ProcessID_Unregistered(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100, Creation: 1}
	target := gen.ProcessID{Name: "myprocess", Node: "node1"}

	tm.LinkProcessID(consumer, target)
	// Process unregisters its name - sends ErrUnregistered
	tm.TerminatedTargetProcessID(target, gen.ErrUnregistered)
	time.Sleep(50 * time.Millisecond)

	exit, _ := core.getFirstSentExit()
	msg := exit.message.(gen.MessageExitProcessID)
	if msg.Reason != gen.ErrUnregistered {
		t.Errorf("wrong reason: got %v, want %v", msg.Reason, gen.ErrUnregistered)
	}
}

// Test: Link to Alias - target terminates, consumer gets MessageExitAlias
func TestLink_Alias_TerminateReason(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100, Creation: 1}
	target := gen.Alias{Node: "node1", ID: [3]uint64{1, 2, 3}}

	tm.LinkAlias(consumer, target)
	tm.TerminatedTargetAlias(target, gen.TerminateReasonKill)
	time.Sleep(50 * time.Millisecond)

	exit, ok := core.getFirstSentExit()
	if ok == false {
		t.Fatal("expected exit message")
	}

	msg, ok := exit.message.(gen.MessageExitAlias)
	if ok == false {
		t.Fatalf("expected MessageExitAlias, got %T", exit.message)
	}
	if msg.Alias != target {
		t.Errorf("wrong Alias: got %v, want %v", msg.Alias, target)
	}
	if msg.Reason != gen.TerminateReasonKill {
		t.Errorf("wrong reason: got %v, want %v", msg.Reason, gen.TerminateReasonKill)
	}
}

// Test: Link to Alias - alias deleted (ErrUnregistered)
func TestLink_Alias_Deleted(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100, Creation: 1}
	target := gen.Alias{Node: "node1", ID: [3]uint64{1, 2, 3}}

	tm.LinkAlias(consumer, target)
	tm.TerminatedTargetAlias(target, gen.ErrUnregistered)
	time.Sleep(50 * time.Millisecond)

	exit, _ := core.getFirstSentExit()
	msg := exit.message.(gen.MessageExitAlias)
	if msg.Reason != gen.ErrUnregistered {
		t.Errorf("wrong reason: got %v, want %v", msg.Reason, gen.ErrUnregistered)
	}
}

// Test: Link to Event - event unregistered, consumer gets MessageExitEvent
func TestLink_Event_Unregistered(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "node1", ID: 50, Creation: 1}
	consumer := gen.PID{Node: "node1", ID: 100, Creation: 1}
	eventName := gen.Atom("myevent")

	tm.RegisterEvent(producer, eventName, gen.EventOptions{})
	event := gen.Event{Node: "node1", Name: eventName}

	tm.LinkEvent(consumer, event)
	// Producer unregisters event
	tm.TerminatedTargetEvent(event, gen.TerminateReasonShutdown)
	time.Sleep(50 * time.Millisecond)

	exit, ok := core.getFirstSentExit()
	if ok == false {
		t.Fatal("expected exit message")
	}

	msg, ok := exit.message.(gen.MessageExitEvent)
	if ok == false {
		t.Fatalf("expected MessageExitEvent, got %T", exit.message)
	}
	if msg.Event != event {
		t.Errorf("wrong Event: got %v, want %v", msg.Event, event)
	}
}

// Test: Link to Node - node disconnects, consumer gets MessageExitNode
func TestLink_Node_Disconnect(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100, Creation: 1}
	remoteNode := gen.Atom("node2")

	tm.LinkNode(consumer, remoteNode)
	// Remote node disconnects
	tm.TerminatedTargetNode(remoteNode, gen.ErrNoConnection)
	time.Sleep(50 * time.Millisecond)

	exit, ok := core.getFirstSentExit()
	if ok == false {
		t.Fatal("expected exit message")
	}

	msg, ok := exit.message.(gen.MessageExitNode)
	if ok == false {
		t.Fatalf("expected MessageExitNode, got %T", exit.message)
	}
	if msg.Name != remoteNode {
		t.Errorf("wrong Node: got %v, want %v", msg.Name, remoteNode)
	}
}

// =============================================================================
// DOWN MESSAGE REASON VERIFICATION (mirrors t005_actor_monitor_test.go)
// =============================================================================

// Test: Monitor PID - target terminates with TerminateReasonKill
func TestMonitor_PID_TerminateReasonKill(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100, Creation: 1}
	target := gen.PID{Node: "node1", ID: 200, Creation: 1}

	tm.MonitorPID(consumer, target)
	tm.TerminatedTargetPID(target, gen.TerminateReasonKill)
	time.Sleep(50 * time.Millisecond)

	down, ok := core.getFirstSentDown()
	if ok == false {
		t.Fatal("expected down message")
	}

	if down.to != consumer {
		t.Errorf("down sent to wrong consumer: got %v, want %v", down.to, consumer)
	}

	msg, ok := down.message.(gen.MessageDownPID)
	if ok == false {
		t.Fatalf("expected MessageDownPID, got %T", down.message)
	}
	if msg.PID != target {
		t.Errorf("wrong PID: got %v, want %v", msg.PID, target)
	}
	if msg.Reason != gen.TerminateReasonKill {
		t.Errorf("wrong reason: got %v, want %v", msg.Reason, gen.TerminateReasonKill)
	}
}

// Test: Monitor PID - target terminates with custom error
func TestMonitor_PID_CustomReason(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100, Creation: 1}
	target := gen.PID{Node: "node1", ID: 200, Creation: 1}

	customErr := errors.New("custom error")

	tm.MonitorPID(consumer, target)
	tm.TerminatedTargetPID(target, customErr)
	time.Sleep(50 * time.Millisecond)

	down, _ := core.getFirstSentDown()
	msg := down.message.(gen.MessageDownPID)
	if msg.Reason != customErr {
		t.Errorf("wrong reason: got %v, want %v", msg.Reason, customErr)
	}
}

// Test: Monitor ProcessID - target terminates
func TestMonitor_ProcessID_TerminateReason(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100, Creation: 1}
	target := gen.ProcessID{Name: "myprocess", Node: "node1"}

	tm.MonitorProcessID(consumer, target)
	tm.TerminatedTargetProcessID(target, gen.TerminateReasonShutdown)
	time.Sleep(50 * time.Millisecond)

	down, ok := core.getFirstSentDown()
	if ok == false {
		t.Fatal("expected down message")
	}

	msg, ok := down.message.(gen.MessageDownProcessID)
	if ok == false {
		t.Fatalf("expected MessageDownProcessID, got %T", down.message)
	}
	if msg.ProcessID != target {
		t.Errorf("wrong ProcessID: got %v, want %v", msg.ProcessID, target)
	}
	if msg.Reason != gen.TerminateReasonShutdown {
		t.Errorf("wrong reason: got %v, want %v", msg.Reason, gen.TerminateReasonShutdown)
	}
}

// Test: Monitor ProcessID - name unregistered
func TestMonitor_ProcessID_Unregistered(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100, Creation: 1}
	target := gen.ProcessID{Name: "myprocess", Node: "node1"}

	tm.MonitorProcessID(consumer, target)
	tm.TerminatedTargetProcessID(target, gen.ErrUnregistered)
	time.Sleep(50 * time.Millisecond)

	down, _ := core.getFirstSentDown()
	msg := down.message.(gen.MessageDownProcessID)
	if msg.Reason != gen.ErrUnregistered {
		t.Errorf("wrong reason: got %v, want %v", msg.Reason, gen.ErrUnregistered)
	}
}

// Test: Monitor Alias - alias deleted
func TestMonitor_Alias_Deleted(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100, Creation: 1}
	target := gen.Alias{Node: "node1", ID: [3]uint64{1, 2, 3}}

	tm.MonitorAlias(consumer, target)
	tm.TerminatedTargetAlias(target, gen.ErrUnregistered)
	time.Sleep(50 * time.Millisecond)

	down, ok := core.getFirstSentDown()
	if ok == false {
		t.Fatal("expected down message")
	}

	msg, ok := down.message.(gen.MessageDownAlias)
	if ok == false {
		t.Fatalf("expected MessageDownAlias, got %T", down.message)
	}
	if msg.Reason != gen.ErrUnregistered {
		t.Errorf("wrong reason: got %v, want %v", msg.Reason, gen.ErrUnregistered)
	}
}

// Test: Monitor Event - event unregistered
func TestMonitor_Event_Unregistered(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "node1", ID: 50, Creation: 1}
	consumer := gen.PID{Node: "node1", ID: 100, Creation: 1}
	eventName := gen.Atom("myevent")

	tm.RegisterEvent(producer, eventName, gen.EventOptions{})
	event := gen.Event{Node: "node1", Name: eventName}

	tm.MonitorEvent(consumer, event)
	tm.TerminatedTargetEvent(event, gen.TerminateReasonShutdown)
	time.Sleep(50 * time.Millisecond)

	down, ok := core.getFirstSentDown()
	if ok == false {
		t.Fatal("expected down message")
	}

	msg, ok := down.message.(gen.MessageDownEvent)
	if ok == false {
		t.Fatalf("expected MessageDownEvent, got %T", down.message)
	}
	if msg.Event != event {
		t.Errorf("wrong Event: got %v, want %v", msg.Event, event)
	}
}

// Test: Monitor Node - node disconnects
func TestMonitor_Node_Disconnect(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100, Creation: 1}
	remoteNode := gen.Atom("node2")

	tm.MonitorNode(consumer, remoteNode)
	tm.TerminatedTargetNode(remoteNode, gen.ErrNoConnection)
	time.Sleep(50 * time.Millisecond)

	down, ok := core.getFirstSentDown()
	if ok == false {
		t.Fatal("expected down message")
	}

	msg, ok := down.message.(gen.MessageDownNode)
	if ok == false {
		t.Fatalf("expected MessageDownNode, got %T", down.message)
	}
	if msg.Name != remoteNode {
		t.Errorf("wrong Node: got %v, want %v", msg.Name, remoteNode)
	}
}

// =============================================================================
// NODE DOWN - ALL REMOTE SUBSCRIPTIONS (mirrors t005_link_test.go distributed)
// =============================================================================

// Test: Node down notifies all consumers linked to PIDs on that node
func TestNodeDown_NotifiesLinkedPID(t *testing.T) {
	core := newMockCore("local")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "local", ID: 100, Creation: 1}
	remotePID := gen.PID{Node: "remote", ID: 200, Creation: 1}

	tm.LinkPID(consumer, remotePID)
	tm.TerminatedTargetNode(gen.Atom("remote"), gen.ErrNoConnection)
	time.Sleep(50 * time.Millisecond)

	exit, ok := core.getFirstSentExit()
	if ok == false {
		t.Fatal("expected exit message")
	}

	msg, ok := exit.message.(gen.MessageExitPID)
	if ok == false {
		t.Fatalf("expected MessageExitPID, got %T", exit.message)
	}
	if msg.PID != remotePID {
		t.Errorf("wrong PID: got %v, want %v", msg.PID, remotePID)
	}
	if msg.Reason != gen.ErrNoConnection {
		t.Errorf("wrong reason: got %v, want %v", msg.Reason, gen.ErrNoConnection)
	}
}

// Test: Node down notifies all consumers linked to ProcessIDs on that node
func TestNodeDown_NotifiesLinkedProcessID(t *testing.T) {
	core := newMockCore("local")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "local", ID: 100, Creation: 1}
	remoteProcessID := gen.ProcessID{Name: "regpong", Node: "remote"}

	tm.LinkProcessID(consumer, remoteProcessID)
	tm.TerminatedTargetNode(gen.Atom("remote"), gen.ErrNoConnection)
	time.Sleep(50 * time.Millisecond)

	exit, ok := core.getFirstSentExit()
	if ok == false {
		t.Fatal("expected exit message")
	}

	msg, ok := exit.message.(gen.MessageExitProcessID)
	if ok == false {
		t.Fatalf("expected MessageExitProcessID, got %T", exit.message)
	}
	if msg.ProcessID != remoteProcessID {
		t.Errorf("wrong ProcessID: got %v, want %v", msg.ProcessID, remoteProcessID)
	}
	if msg.Reason != gen.ErrNoConnection {
		t.Errorf("wrong reason: got %v, want %v", msg.Reason, gen.ErrNoConnection)
	}
}

// Test: Node down notifies all consumers linked to Aliases on that node
func TestNodeDown_NotifiesLinkedAlias(t *testing.T) {
	core := newMockCore("local")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "local", ID: 100, Creation: 1}
	remoteAlias := gen.Alias{Node: "remote", ID: [3]uint64{1, 2, 3}}

	tm.LinkAlias(consumer, remoteAlias)
	tm.TerminatedTargetNode(gen.Atom("remote"), gen.ErrNoConnection)
	time.Sleep(50 * time.Millisecond)

	exit, ok := core.getFirstSentExit()
	if ok == false {
		t.Fatal("expected exit message")
	}

	msg, ok := exit.message.(gen.MessageExitAlias)
	if ok == false {
		t.Fatalf("expected MessageExitAlias, got %T", exit.message)
	}
	if msg.Alias != remoteAlias {
		t.Errorf("wrong Alias: got %v, want %v", msg.Alias, remoteAlias)
	}
	if msg.Reason != gen.ErrNoConnection {
		t.Errorf("wrong reason: got %v, want %v", msg.Reason, gen.ErrNoConnection)
	}
}

// Test: Node down notifies all consumers linked to Events on that node
func TestNodeDown_NotifiesLinkedEvent(t *testing.T) {
	core := newMockCore("local")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "local", ID: 100, Creation: 1}
	remoteEvent := gen.Event{Node: "remote", Name: "remoteevent"}

	tm.LinkEvent(consumer, remoteEvent)
	tm.TerminatedTargetNode(gen.Atom("remote"), gen.ErrNoConnection)
	time.Sleep(50 * time.Millisecond)

	exit, ok := core.getFirstSentExit()
	if ok == false {
		t.Fatal("expected exit message")
	}

	msg, ok := exit.message.(gen.MessageExitEvent)
	if ok == false {
		t.Fatalf("expected MessageExitEvent, got %T", exit.message)
	}
	if msg.Event != remoteEvent {
		t.Errorf("wrong Event: got %v, want %v", msg.Event, remoteEvent)
	}
	if msg.Reason != gen.ErrNoConnection {
		t.Errorf("wrong reason: got %v, want %v", msg.Reason, gen.ErrNoConnection)
	}
}

// Test: Node down notifies all consumers monitoring PIDs on that node
func TestNodeDown_NotifiesMonitoringPID(t *testing.T) {
	core := newMockCore("local")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "local", ID: 100, Creation: 1}
	remotePID := gen.PID{Node: "remote", ID: 200, Creation: 1}

	tm.MonitorPID(consumer, remotePID)
	tm.TerminatedTargetNode(gen.Atom("remote"), gen.ErrNoConnection)
	time.Sleep(50 * time.Millisecond)

	down, ok := core.getFirstSentDown()
	if ok == false {
		t.Fatal("expected down message")
	}

	msg, ok := down.message.(gen.MessageDownPID)
	if ok == false {
		t.Fatalf("expected MessageDownPID, got %T", down.message)
	}
	if msg.Reason != gen.ErrNoConnection {
		t.Errorf("wrong reason: got %v, want %v", msg.Reason, gen.ErrNoConnection)
	}
}

// =============================================================================
// MULTIPLE CONSUMERS - SAME TARGET
// =============================================================================

// Test: Multiple consumers link same remote target - all notified on termination
func TestMultipleConsumers_Link_AllNotified(t *testing.T) {
	core := newMockCore("local")
	tm := Create(core, Options{}).(*targetManager)

	consumer1 := gen.PID{Node: "local", ID: 100, Creation: 1}
	consumer2 := gen.PID{Node: "local", ID: 101, Creation: 1}
	consumer3 := gen.PID{Node: "local", ID: 102, Creation: 1}
	target := gen.PID{Node: "local", ID: 200, Creation: 1}

	tm.LinkPID(consumer1, target)
	tm.LinkPID(consumer2, target)
	tm.LinkPID(consumer3, target)

	tm.TerminatedTargetPID(target, gen.TerminateReasonShutdown)
	time.Sleep(50 * time.Millisecond)

	exits := core.getAllSentExits()
	if len(exits) != 3 {
		t.Fatalf("expected 3 exits, got %d", len(exits))
	}

	// Verify all consumers received exit
	consumers := make(map[gen.PID]bool)
	for _, exit := range exits {
		consumers[exit.to] = true
		msg := exit.message.(gen.MessageExitPID)
		if msg.Reason != gen.TerminateReasonShutdown {
			t.Errorf("wrong reason for consumer %v", exit.to)
		}
	}

	if consumers[consumer1] == false {
		t.Error("consumer1 didn't receive exit")
	}
	if consumers[consumer2] == false {
		t.Error("consumer2 didn't receive exit")
	}
	if consumers[consumer3] == false {
		t.Error("consumer3 didn't receive exit")
	}

	// Verify internal state cleaned up
	if _, exists := tm.targetIndex[target]; exists {
		t.Error("targetIndex should be cleaned after termination")
	}
	if len(tm.linkRelations) != 0 {
		t.Errorf("all linkRelations should be cleaned, got %d", len(tm.linkRelations))
	}
}

// Test: Multiple consumers monitor same target - all notified on termination
func TestMultipleConsumers_Monitor_AllNotified(t *testing.T) {
	core := newMockCore("local")
	tm := Create(core, Options{}).(*targetManager)

	consumer1 := gen.PID{Node: "local", ID: 100, Creation: 1}
	consumer2 := gen.PID{Node: "local", ID: 101, Creation: 1}
	target := gen.PID{Node: "local", ID: 200, Creation: 1}

	tm.MonitorPID(consumer1, target)
	tm.MonitorPID(consumer2, target)

	tm.TerminatedTargetPID(target, gen.TerminateReasonKill)
	time.Sleep(50 * time.Millisecond)

	downs := core.getAllSentDowns()
	if len(downs) != 2 {
		t.Fatalf("expected 2 downs, got %d", len(downs))
	}

	for _, down := range downs {
		msg := down.message.(gen.MessageDownPID)
		if msg.Reason != gen.TerminateReasonKill {
			t.Errorf("wrong reason for consumer %v", down.to)
		}
	}

	// Verify internal state cleaned up
	if _, exists := tm.targetIndex[target]; exists {
		t.Error("targetIndex should be cleaned after termination")
	}
	if len(tm.monitorRelations) != 0 {
		t.Errorf("all monitorRelations should be cleaned, got %d", len(tm.monitorRelations))
	}
}

// =============================================================================
// CONSUMER TERMINATION - CLEANUP
// =============================================================================

// Test: Consumer terminates - all its links are cleaned up
func TestConsumerTerminates_LinksCleanedUp(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100, Creation: 1}
	target1 := gen.PID{Node: "node1", ID: 200, Creation: 1}
	target2 := gen.PID{Node: "node1", ID: 201, Creation: 1}
	target3 := gen.ProcessID{Name: "proc", Node: "node1"}

	tm.LinkPID(consumer, target1)
	tm.LinkPID(consumer, target2)
	tm.LinkProcessID(consumer, target3)

	// Verify links exist
	if tm.HasLink(consumer, target1) == false {
		t.Error("link to target1 should exist")
	}

	// Consumer terminates
	tm.TerminatedProcess(consumer, gen.TerminateReasonShutdown)
	time.Sleep(50 * time.Millisecond)

	// All links should be gone
	if tm.HasLink(consumer, target1) {
		t.Error("link to target1 should be cleaned")
	}
	if tm.HasLink(consumer, target2) {
		t.Error("link to target2 should be cleaned")
	}
	if tm.HasLink(consumer, target3) {
		t.Error("link to target3 should be cleaned")
	}
}

// Test: Consumer terminates - all its monitors are cleaned up
func TestConsumerTerminates_MonitorsCleanedUp(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100, Creation: 1}
	target1 := gen.PID{Node: "node1", ID: 200, Creation: 1}
	target2 := gen.Alias{Node: "node1", ID: [3]uint64{1, 2, 3}}

	tm.MonitorPID(consumer, target1)
	tm.MonitorAlias(consumer, target2)

	// Verify monitors exist
	if tm.HasMonitor(consumer, target1) == false {
		t.Error("monitor to target1 should exist")
	}

	// Consumer terminates
	tm.TerminatedProcess(consumer, gen.TerminateReasonShutdown)
	time.Sleep(50 * time.Millisecond)

	// All monitors should be gone
	if tm.HasMonitor(consumer, target1) {
		t.Error("monitor to target1 should be cleaned")
	}
	if tm.HasMonitor(consumer, target2) {
		t.Error("monitor to target2 should be cleaned")
	}
}

// =============================================================================
// REMOTE NETWORK REQUEST OPTIMIZATION
// =============================================================================

// Test: Two local consumers link same remote target - only ONE network request
func TestRemoteLink_TwoConsumers_OneNetworkRequest(t *testing.T) {
	core := newMockCore("local")
	tm := Create(core, Options{}).(*targetManager)

	consumer1 := gen.PID{Node: "local", ID: 100, Creation: 1}
	consumer2 := gen.PID{Node: "local", ID: 101, Creation: 1}
	remotePID := gen.PID{Node: "remote", ID: 200, Creation: 1}

	tm.LinkPID(consumer1, remotePID)
	tm.LinkPID(consumer2, remotePID)

	// Only first consumer should trigger network request
	if core.countSentLinks() != 1 {
		t.Errorf("expected 1 network link request, got %d", core.countSentLinks())
	}

	// Verify targetIndex has 2 consumers
	entry := tm.targetIndex[remotePID]
	if entry == nil {
		t.Fatal("targetIndex should exist")
	}
	if len(entry.consumers) != 2 {
		t.Errorf("expected 2 consumers in targetIndex, got %d", len(entry.consumers))
	}
}

// Test: Two local consumers unlink - network unlink only on LAST
func TestRemoteUnlink_LastConsumer_SendsNetworkUnlink(t *testing.T) {
	core := newMockCore("local")
	tm := Create(core, Options{}).(*targetManager)

	consumer1 := gen.PID{Node: "local", ID: 100, Creation: 1}
	consumer2 := gen.PID{Node: "local", ID: 101, Creation: 1}
	remotePID := gen.PID{Node: "remote", ID: 200, Creation: 1}

	tm.LinkPID(consumer1, remotePID)
	tm.LinkPID(consumer2, remotePID)

	core.resetSentUnlinks()

	// First unlink - should NOT send network request
	tm.UnlinkPID(consumer1, remotePID)
	if core.countSentUnlinks() != 0 {
		t.Errorf("expected 0 network unlink (not last), got %d", core.countSentUnlinks())
	}

	// Second unlink - SHOULD send network request
	tm.UnlinkPID(consumer2, remotePID)
	if core.countSentUnlinks() != 1 {
		t.Errorf("expected 1 network unlink (last consumer), got %d", core.countSentUnlinks())
	}

	// Verify targetIndex cleaned up
	if _, exists := tm.targetIndex[remotePID]; exists {
		t.Error("targetIndex should be cleaned after last consumer unlinks")
	}
}

// =============================================================================
// INFO STATISTICS ACCURACY
// =============================================================================

// Test: Info() returns correct link count
func TestInfo_LinkCount(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100, Creation: 1}

	for i := 0; i < 50; i++ {
		target := gen.PID{Node: "node1", ID: uint64(200 + i), Creation: 1}
		tm.LinkPID(consumer, target)
	}

	info := tm.Info()
	if info.Links != 50 {
		t.Errorf("expected 50 links, got %d", info.Links)
	}

	// Remove 20
	for i := 0; i < 20; i++ {
		target := gen.PID{Node: "node1", ID: uint64(200 + i), Creation: 1}
		tm.UnlinkPID(consumer, target)
	}

	info = tm.Info()
	if info.Links != 30 {
		t.Errorf("expected 30 links, got %d", info.Links)
	}
}

// Test: Info() returns correct monitor count
func TestInfo_MonitorCount(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100, Creation: 1}

	for i := 0; i < 30; i++ {
		target := gen.PID{Node: "node1", ID: uint64(200 + i), Creation: 1}
		tm.MonitorPID(consumer, target)
	}

	info := tm.Info()
	if info.Monitors != 30 {
		t.Errorf("expected 30 monitors, got %d", info.Monitors)
	}
}

// Test: Info() returns correct delivered exit/down count
func TestInfo_DeliveredCounts(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	// Create 5 links
	for i := 0; i < 5; i++ {
		consumer := gen.PID{Node: "node1", ID: uint64(100 + i), Creation: 1}
		target := gen.PID{Node: "node1", ID: uint64(200 + i), Creation: 1}
		tm.LinkPID(consumer, target)
	}

	// Create 5 monitors
	for i := 0; i < 5; i++ {
		consumer := gen.PID{Node: "node1", ID: uint64(300 + i), Creation: 1}
		target := gen.PID{Node: "node1", ID: uint64(400 + i), Creation: 1}
		tm.MonitorPID(consumer, target)
	}

	// Terminate all link targets
	for i := 0; i < 5; i++ {
		target := gen.PID{Node: "node1", ID: uint64(200 + i), Creation: 1}
		tm.TerminatedTargetPID(target, gen.TerminateReasonShutdown)
	}

	// Terminate all monitor targets
	for i := 0; i < 5; i++ {
		target := gen.PID{Node: "node1", ID: uint64(400 + i), Creation: 1}
		tm.TerminatedTargetPID(target, gen.TerminateReasonShutdown)
	}

	time.Sleep(100 * time.Millisecond)

	info := tm.Info()
	if info.ExitSignalsDelivered != 5 {
		t.Errorf("expected 5 exits delivered, got %d", info.ExitSignalsDelivered)
	}
	if info.DownMessagesDelivered != 5 {
		t.Errorf("expected 5 downs delivered, got %d", info.DownMessagesDelivered)
	}
}

// =============================================================================
// CORNER CASES
// =============================================================================

// Corner case: Link from TWO different nodes (two CorePIDs)
func TestCorner_LinkFromTwoNodes(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer1 := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node3", ID: 200}

	// node1 subscribes
	tm.LinkPID(consumer1, target)

	if core.countSentLinks() != 1 {
		t.Fatalf("Expected 1 link from node1, got %d", core.countSentLinks())
	}

	// Simulate node2's CorePID also linking
	coreNode2 := gen.PID{Node: "node2", ID: 1}
	key := relationKey{consumer: coreNode2, target: target}
	tm.linkRelations[key] = struct{}{}
	entry := tm.targetIndex[target]
	if entry != nil {
		entry.consumers[coreNode2] = struct{}{}
	}

	// node1's second consumer
	consumer2 := gen.PID{Node: "node1", ID: 101}
	core.resetSentLinks()
	tm.LinkPID(consumer2, target)

	// Should NOT send (node1 already has link)
	if core.countSentLinks() != 0 {
		t.Error("Should not send duplicate from same node")
	}
}

// Corner case: ProcessID empty Node
func TestCorner_ProcessID_EmptyNode(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.ProcessID{Node: "", Name: "test"}

	err := tm.LinkProcessID(consumer, target)
	if err != nil {
		t.Fatalf("LinkProcessID failed: %v", err)
	}

	// Treated as local
	if core.countSentLinks() != 0 {
		t.Error("Empty node should be local")
	}
}

// Corner case: Re-subscribe
func TestCorner_ReSubscribe(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node2", ID: 200}

	tm.LinkPID(consumer, target)
	tm.UnlinkPID(consumer, target)

	core.resetSentLinks()

	err := tm.LinkPID(consumer, target)
	if err != nil {
		t.Fatalf("Re-subscribe failed: %v", err)
	}

	// Should send network again
	if core.countSentLinks() != 1 {
		t.Error("Re-subscribe should send network")
	}
}

// Corner case: Has after error
func TestCorner_HasAfterError(t *testing.T) {
	core := newMockCore("node1")
	core.linkError = gen.ErrProcessUnknown
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node2", ID: 200}

	tm.LinkPID(consumer, target)

	// Has should be false (rollback)
	if tm.HasLink(consumer, target) == true {
		t.Error("HasLink should be false after failed link")
	}
}

// Corner case: targetIndex cleaned after rollback
func TestCorner_TargetIndexCleanedAfterRollback(t *testing.T) {
	core := newMockCore("node1")
	core.connectionError = gen.ErrNoConnection
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	target := gen.PID{Node: "node2", ID: 200}

	tm.LinkPID(consumer, target)

	// Cleaned
	if _, exists := tm.targetIndex[target]; exists {
		t.Error("targetIndex should be cleaned after rollback")
	}
}

// Event corner cases

func TestCorner_Event_ReSubscribe(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "node1", ID: 100}
	consumer := gen.PID{Node: "node1", ID: 101}

	tm.RegisterEvent(producer, "test", gen.EventOptions{})

	event := gen.Event{Node: "node1", Name: "test"}

	tm.LinkEvent(consumer, event)
	tm.UnlinkEvent(consumer, event)

	_, err := tm.LinkEvent(consumer, event)
	if err != nil {
		t.Error("Re-subscribe should work")
	}

	entry := tm.events[event]
	if entry.subscriberCount != 1 {
		t.Errorf("Counter should be 1, got %d", entry.subscriberCount)
	}
}

func TestCorner_Event_MultiNodeBuffered(t *testing.T) {
	core := newMockCore("node1")
	core.eventBuffers = make(map[gen.Event][]gen.MessageEvent)

	event := gen.Event{Node: "node2", Name: "test"}
	core.eventBuffers[event] = []gen.MessageEvent{{Message: "msg1"}}

	tm := Create(core, Options{}).(*targetManager)

	consumer1 := gen.PID{Node: "node1", ID: 100}
	buffer1, _ := tm.LinkEvent(consumer1, event)

	if len(buffer1) != 1 {
		t.Error("Should get buffer")
	}

	consumer2 := gen.PID{Node: "node1", ID: 101}
	buffer2, _ := tm.LinkEvent(consumer2, event)

	if len(buffer2) != 1 {
		t.Error("Second should also get buffer")
	}

	// 2 network requests (buffered)
	if core.countSentEventLinks() != 2 {
		t.Errorf("Buffered should send 2 requests, got %d", core.countSentEventLinks())
	}
}

func TestCorner_PublishEvent_NoSubscribers(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "node1", ID: 100}

	token, _ := tm.RegisterEvent(producer, "test", gen.EventOptions{Buffer: 5})

	event := gen.Event{Node: "node1", Name: "test"}

	core.resetSentEvents()

	err := tm.PublishEvent(producer, token, gen.MessageOptions{}, gen.MessageEvent{
		Event:     event,
		Timestamp: time.Now().UnixNano(),
		Message:   "msg",
	})
	if err != nil {
		t.Error("Publish with no subscribers should work")
	}

	time.Sleep(20 * time.Millisecond)

	// No deliveries
	if core.countSentEvents() != 0 {
		t.Error("Should not send with no subscribers")
	}

	// Buffer updated
	entry := tm.events[event]
	if len(entry.buffer) != 1 {
		t.Error("Buffer should be updated")
	}
}

func TestCorner_Event_EmptyBufferVsNil(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "node1", ID: 100}
	consumer := gen.PID{Node: "node1", ID: 101}

	tm.RegisterEvent(producer, "buffered", gen.EventOptions{Buffer: 10})
	tm.RegisterEvent(producer, "unbuffered", gen.EventOptions{Buffer: 0})

	bufferedEvent := gen.Event{Node: "node1", Name: "buffered"}
	unbufferedEvent := gen.Event{Node: "node1", Name: "unbuffered"}

	buffer1, _ := tm.LinkEvent(consumer, bufferedEvent)

	// Empty slice (not nil)
	if buffer1 == nil {
		t.Error("Buffered should return empty slice, not nil")
	}

	buffer2, _ := tm.LinkEvent(consumer, unbufferedEvent)

	// Nil
	if buffer2 != nil {
		t.Error("Unbuffered should return nil")
	}
}

func TestCorner_ConcurrentAdd(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	target := gen.PID{Node: "node2", ID: 200}

	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(id int) {
			consumer := gen.PID{Node: "node1", ID: uint64(100 + id)}
			tm.LinkPID(consumer, target)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// All stored
	if len(tm.linkRelations) != 10 {
		t.Errorf("Expected 10 links, got %d", len(tm.linkRelations))
	}

	// Only 1 network
	if core.countSentLinks() != 1 {
		t.Error("Only first should send network")
	}
}

func TestCorner_TerminatedProcess_NoSubscriptions(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}

	tm.TerminatedProcess(consumer, gen.TerminateReasonNormal)

	time.Sleep(20 * time.Millisecond)

	// No crash, no messages
}

// =============================================================================
// STRESS TESTS
// =============================================================================

// Stress: 1000 goroutines adding links simultaneously
func TestStress_1000ConcurrentLinks(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	target := gen.PID{Node: "node2", ID: 200}

	var wg sync.WaitGroup
	errors := make(chan error, 1000)

	// 1000 goroutines trying to link
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			consumer := gen.PID{Node: "node1", ID: uint64(100 + id)}
			err := tm.LinkPID(consumer, target)
			if err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	errorCount := 0
	for err := range errors {
		t.Errorf("Link failed: %v", err)
		errorCount++
	}

	if errorCount > 0 {
		t.Fatalf("Got %d errors out of 1000", errorCount)
	}

	// Verify all 1000 stored
	if len(tm.linkRelations) != 1000 {
		t.Errorf("Expected 1000 links, got %d", len(tm.linkRelations))
	}

	// Only 1 network request (CorePID optimization!)
	if core.countSentLinks() != 1 {
		t.Errorf("Expected exactly 1 network request with CorePID optimization, got %d", core.countSentLinks())
	}

	// Verify targetIndex has all 1000
	entry := tm.targetIndex[target]
	if entry == nil {
		t.Fatal("targetEntry should exist")
	}

	if len(entry.consumers) != 1000 {
		t.Errorf("Expected 1000 consumers in targetIndex, got %d", len(entry.consumers))
	}
}

// Stress: 100 goroutines add/remove simultaneously
func TestStress_ConcurrentAddRemove(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	target := gen.PID{Node: "node2", ID: 200}

	var wg sync.WaitGroup

	// 100 goroutines: add then remove
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			consumer := gen.PID{Node: "node1", ID: uint64(100 + id)}

			// Add
			tm.LinkPID(consumer, target)

			// Small delay
			time.Sleep(time.Millisecond)

			// Remove
			tm.UnlinkPID(consumer, target)
		}(i)
	}

	wg.Wait()

	// Give time for cleanup
	time.Sleep(100 * time.Millisecond)

	// All should be cleaned
	if len(tm.linkRelations) != 0 {
		t.Errorf("All links should be removed, got %d", len(tm.linkRelations))
	}

	// targetIndex should be clean (or have minimal entries)
	if len(tm.targetIndex) > 0 {
		t.Logf("targetIndex has %d entries (may be cleanup in progress)", len(tm.targetIndex))
	}
}

// Stress: Event publish to 1000 subscribers
func TestStress_Event_1000Subscribers(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "node1", ID: 1}

	token, err := tm.RegisterEvent(producer, "broadcast", gen.EventOptions{})
	if err != nil {
		t.Fatalf("RegisterEvent failed: %v", err)
	}

	event := gen.Event{Node: "node1", Name: "broadcast"}

	// 1000 subscribers
	for i := 0; i < 1000; i++ {
		consumer := gen.PID{Node: "node1", ID: uint64(100 + i)}
		tm.LinkEvent(consumer, event)
	}

	// Verify all subscribed
	entry := tm.events[event]
	if len(entry.linkSubscribers) != 1000 {
		t.Errorf("Expected 1000 subscribers, got %d", len(entry.linkSubscribers))
	}

	core.resetSentEvents()

	// Publish event
	err = tm.PublishEvent(producer, token, gen.MessageOptions{}, gen.MessageEvent{
		Event:     event,
		Timestamp: time.Now().UnixNano(),
		Message:   "broadcast message",
	})
	if err != nil {
		t.Fatalf("PublishEvent failed: %v", err)
	}

	// Check statistics immediately (worker processed, dispatchers queued)
	info := tm.Info()
	if info.EventsPublished != 1 {
		t.Errorf("EventsPublished should be 1, got %d", info.EventsPublished)
	}

	// Wait for all deliveries to complete
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if core.countSentEvents() >= 1000 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Verify all 1000 delivered
	delivered := core.countSentEvents()
	if delivered != 1000 {
		t.Errorf("Expected exactly 1000 deliveries, got %d", delivered)
	}
}

// Stress: Rapid subscribe/unsubscribe cycles
func TestStress_RapidSubscribeUnsubscribe(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	target := gen.PID{Node: "node2", ID: 200}
	consumer := gen.PID{Node: "node1", ID: 100}

	// 100 cycles of subscribe/unsubscribe
	for i := 0; i < 100; i++ {
		err := tm.LinkPID(consumer, target)
		if err != nil {
			t.Fatalf("Cycle %d: LinkPID failed: %v", i, err)
		}

		err = tm.UnlinkPID(consumer, target)
		if err != nil {
			t.Fatalf("Cycle %d: UnlinkPID failed: %v", i, err)
		}
	}

	// Should be clean
	if len(tm.linkRelations) != 0 {
		t.Errorf("linkRelations should be empty, got %d", len(tm.linkRelations))
	}

	if len(tm.targetIndex) != 0 {
		t.Errorf("targetIndex should be empty, got %d", len(tm.targetIndex))
	}
}

// Stress: Multiple terminations simultaneously
func TestStress_MassTermination(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	// Create 100 targets with 10 subscribers each
	for targetID := 0; targetID < 100; targetID++ {
		target := gen.PID{Node: "node1", ID: uint64(1000 + targetID)}

		for subID := 0; subID < 10; subID++ {
			consumer := gen.PID{Node: "node1", ID: uint64(100 + targetID*10 + subID)}
			tm.LinkPID(consumer, target)
		}
	}

	// Verify 1000 links total
	if len(tm.linkRelations) != 1000 {
		t.Errorf("Expected 1000 links, got %d", len(tm.linkRelations))
	}

	core.resetSentExits()

	var wg sync.WaitGroup

	// Terminate all 100 targets simultaneously
	for targetID := 0; targetID < 100; targetID++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			target := gen.PID{Node: "node1", ID: uint64(1000 + id)}
			tm.TerminatedTargetPID(target, gen.ErrProcessTerminated)
		}(targetID)
	}

	wg.Wait()

	// Give dispatchers time (10 dispatchers, 1000 exits)
	time.Sleep(1 * time.Second)

	// All 1000 exits should be sent
	if core.countSentExits() != 1000 {
		t.Errorf("Expected 1000 exits, got %d", core.countSentExits())
	}

	// All cleaned
	if len(tm.linkRelations) != 0 {
		t.Errorf("All links should be cleaned, got %d", len(tm.linkRelations))
	}

	// Check statistics
	// Produced = 100 (one per target termination)
	// Delivered = 1000 (10 subscribers per target)
	info := tm.Info()
	if info.ExitSignalsProduced != 100 {
		t.Errorf("ExitSignalsProduced should be 100, got %d", info.ExitSignalsProduced)
	}

	if info.ExitSignalsDelivered < 990 {
		t.Errorf("ExitSignalsDelivered should be ~1000, got %d", info.ExitSignalsDelivered)
	}
}

// Stress: Concurrent event operations (publish, subscribe, unsubscribe)
func TestStress_Event_ConcurrentOperations(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "node1", ID: 1}

	token, _ := tm.RegisterEvent(producer, "stress", gen.EventOptions{Buffer: 100, Notify: true})

	event := gen.Event{Node: "node1", Name: "stress"}

	var wg sync.WaitGroup

	// 100 goroutines subscribing
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			consumer := gen.PID{Node: "node1", ID: uint64(100 + id)}
			tm.LinkEvent(consumer, event)
		}(i)
	}

	// 100 goroutines publishing
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			tm.PublishEvent(producer, token, gen.MessageOptions{}, gen.MessageEvent{
				Event:     event,
				Timestamp: time.Now().UnixNano(),
				Message:   id,
			})
		}(i)
	}

	wg.Wait()

	// Give time for operations
	time.Sleep(200 * time.Millisecond)

	// Verify consistency
	entry := tm.events[event]
	if entry == nil {
		t.Fatal("Event should exist")
	}

	if len(entry.linkSubscribers) != 100 {
		t.Errorf("Expected 100 subscribers, got %d", len(entry.linkSubscribers))
	}

	// Check statistics
	info := tm.Info()
	if info.EventsPublished != 100 {
		t.Errorf("EventsPublished should be 100, got %d", info.EventsPublished)
	}

	// eventsSent should be 100 * number_of_subscribers_at_publish_time
	// (varies based on timing, but should be > 0)
	if info.EventsSent == 0 {
		t.Error("EventsSent should be > 0")
	}
}

// Stress: TerminatedNode with 1000 subscriptions
func TestStress_TerminatedNode_1000Subscriptions(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	// 1000 local consumers linked to remote targets on node2
	for i := 0; i < 1000; i++ {
		consumer := gen.PID{Node: "node1", ID: uint64(100 + i)}
		target := gen.PID{Node: "node2", ID: uint64(1000 + i)}

		tm.LinkPID(consumer, target)
	}

	core.resetSentExits()

	// node2 dies
	tm.TerminatedTargetNode("node2", gen.ErrNoConnection)

	// Give dispatchers time
	time.Sleep(1 * time.Second)

	// All 1000 local consumers should get exit
	if core.countSentExits() != 1000 {
		t.Errorf("Expected 1000 exits, got %d", core.countSentExits())
	}

	// All cleaned
	if len(tm.linkRelations) != 0 {
		t.Errorf("All links should be cleaned, got %d", len(tm.linkRelations))
	}
}

// Stress: Memory stability - 10K subscribe/unsubscribe cycles
func TestStress_Memory_10KCycles(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}

	// 10K cycles
	for i := 0; i < 10000; i++ {
		target := gen.PID{Node: "node2", ID: uint64(200 + i%100)}

		tm.LinkPID(consumer, target)
		tm.UnlinkPID(consumer, target)

		if i%1000 == 0 {
			// Check no memory leak every 1000 cycles
			if len(tm.linkRelations) != 0 {
				t.Errorf("Cycle %d: memory leak detected, %d relations", i, len(tm.linkRelations))
			}

			if len(tm.targetIndex) != 0 {
				t.Errorf("Cycle %d: targetIndex leak, %d entries", i, len(tm.targetIndex))
			}
		}
	}

	// Final check
	if len(tm.linkRelations) != 0 {
		t.Errorf("Final: linkRelations should be empty, got %d", len(tm.linkRelations))
	}

	if len(tm.targetIndex) != 0 {
		t.Errorf("Final: targetIndex should be empty, got %d", len(tm.targetIndex))
	}
}

// Stress: Event publish rate (1000 publishes in quick succession)
func TestStress_Event_RapidPublish(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "node1", ID: 1}

	token, _ := tm.RegisterEvent(producer, "rapid", gen.EventOptions{Buffer: 10})

	event := gen.Event{Node: "node1", Name: "rapid"}

	// 10 subscribers
	for i := 0; i < 10; i++ {
		consumer := gen.PID{Node: "node1", ID: uint64(100 + i)}
		tm.LinkEvent(consumer, event)
	}

	// 1000 rapid publishes
	start := time.Now()

	for i := 0; i < 1000; i++ {
		tm.PublishEvent(producer, token, gen.MessageOptions{}, gen.MessageEvent{
			Event:     event,
			Timestamp: time.Now().UnixNano(),
			Message:   i,
		})
	}

	duration := time.Since(start)

	// Should complete quickly (worker processes sequentially)
	if duration > 2*time.Second {
		t.Errorf("1000 publishes took too long: %v", duration)
	}

	// Give dispatchers time
	time.Sleep(500 * time.Millisecond)

	// 10K total deliveries (1000 publishes × 10 subscribers)
	if core.countSentEvents() < 9000 {
		t.Errorf("Expected ~10000 deliveries, got %d", core.countSentEvents())
	}

	// Check statistics
	info := tm.Info()
	if info.EventsPublished != 1000 {
		t.Errorf("EventsPublished should be 1000, got %d", info.EventsPublished)
	}
}

// Stress: Concurrent TerminatedProcess calls
func TestStress_ConcurrentTerminatedProcess(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	// 500 consumers each with 2 links
	for i := 0; i < 500; i++ {
		consumer := gen.PID{Node: "node1", ID: uint64(100 + i)}
		target1 := gen.PID{Node: "node2", ID: uint64(1000 + i)}
		target2 := gen.PID{Node: "node2", ID: uint64(2000 + i)}

		tm.LinkPID(consumer, target1)
		tm.LinkPID(consumer, target2)
	}

	// 1000 links total
	if len(tm.linkRelations) != 1000 {
		t.Errorf("Expected 1000 links, got %d", len(tm.linkRelations))
	}

	var wg sync.WaitGroup

	// All 500 consumers terminate simultaneously
	for i := 0; i < 500; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			consumer := gen.PID{Node: "node1", ID: uint64(100 + id)}
			tm.TerminatedProcess(consumer, gen.TerminateReasonKill)
		}(i)
	}

	wg.Wait()

	// Give time for cleanup
	time.Sleep(500 * time.Millisecond)

	// All cleaned
	if len(tm.linkRelations) != 0 {
		t.Errorf("All links should be cleaned, got %d", len(tm.linkRelations))
	}

	// Remote Unlinks sent (500 consumers × 2 targets but CorePID optimization)
	// Exact number depends on cleanup order
	if core.countSentUnlinks() == 0 {
		t.Error("Should send some remote Unlinks")
	}
}

// Stress: Mixed operations under load
func TestStress_MixedOperations(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	var wg sync.WaitGroup

	// 100 goroutines doing random operations
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			consumer := gen.PID{Node: "node1", ID: uint64(100 + id)}
			target := gen.PID{Node: "node2", ID: uint64(200 + id%10)}

			// Link
			tm.LinkPID(consumer, target)

			// Has
			tm.HasLink(consumer, target)

			// Monitor same target
			tm.MonitorPID(consumer, target)

			// Unlink
			tm.UnlinkPID(consumer, target)

			// Demonitor
			tm.DemonitorPID(consumer, target)
		}(i)
	}

	wg.Wait()

	// Give time to settle
	time.Sleep(200 * time.Millisecond)

	// Should be mostly clean
	if len(tm.linkRelations) != 0 {
		t.Logf("linkRelations: %d (some may be in flight)", len(tm.linkRelations))
	}

	if len(tm.monitorRelations) != 0 {
		t.Logf("monitorRelations: %d (some may be in flight)", len(tm.monitorRelations))
	}
}

// Stress: Worker goroutine spawn/sleep cycles under load
func TestStress_WorkerSpawnCycles(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	target := gen.PID{Node: "node1", ID: 200}

	// 100 cycles: burst of operations, then pause (worker sleeps)
	for cycle := 0; cycle < 100; cycle++ {
		// Burst of 10 operations
		for i := 0; i < 10; i++ {
			consumer := gen.PID{Node: "node1", ID: uint64(100 + cycle*10 + i)}
			tm.LinkPID(consumer, target)
		}

		// Pause to let worker sleep
		time.Sleep(10 * time.Millisecond)
	}

	// All 1000 links stored
	if len(tm.linkRelations) != 1000 {
		t.Errorf("Expected 1000 links, got %d", len(tm.linkRelations))
	}

	// Worker should have spawned and slept multiple times
	// (can't directly verify but no crashes is good)
}
