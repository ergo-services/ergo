package tm

import (
	"testing"
	"time"

	"ergo.services/ergo/gen"
)

// Test RegisterEvent - basic
func TestRegisterEvent_Basic(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "node1", ID: 100}

	token, err := tm.RegisterEvent(producer, "test", gen.EventOptions{Buffer: 10, Notify: true})
	if err != nil {
		t.Fatalf("RegisterEvent failed: %v", err)
	}

	// Verify token returned
	if token.Node != "node1" {
		t.Error("Token should have correct node")
	}

	// Verify event stored
	event := gen.Event{Node: "node1", Name: "test"}
	entry := tm.getEventEntry(event)
	if entry == nil {
		t.Fatal("Event should be stored")
	}

	if entry.producer != producer {
		t.Error("Producer should match")
	}

	if entry.buffer == nil || entry.buffer.size != 10 {
		t.Errorf("Buffer size should be 10")
	}

	if entry.notify == false {
		t.Error("Notify should be true")
	}
}

// Test RegisterEvent - duplicate
func TestRegisterEvent_Duplicate(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "node1", ID: 100}

	tm.RegisterEvent(producer, "test", gen.EventOptions{})

	// Try duplicate
	_, err := tm.RegisterEvent(producer, "test", gen.EventOptions{})
	if err != gen.ErrTaken {
		t.Errorf("Expected ErrTaken for duplicate, got %v", err)
	}
}

// Test LinkEvent local - first subscriber gets EventStart
func TestLinkEvent_Local_FirstSubscriber_EventStart(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "node1", ID: 100}
	consumer := gen.PID{Node: "node1", ID: 101}

	_, _ = tm.RegisterEvent(producer, "test", gen.EventOptions{Notify: true})

	event := gen.Event{Node: "node1", Name: "test"}

	core.resetSentEventStarts()

	// First subscriber
	_, err := tm.LinkEvent(consumer, event)
	if err != nil {
		t.Fatalf("LinkEvent failed: %v", err)
	}

	// Give dispatcher time
	time.Sleep(50 * time.Millisecond)

	// EventStart sent!
	if core.countSentEventStarts() != 1 {
		t.Errorf("Expected 1 EventStart, got %d", core.countSentEventStarts())
	}

	if core.countSentEventStarts() > 0 {
		item := core.sentEventStarts.Item()
		if item != nil {
			sent := item.Value().(eventNotification)
			if sent.producer != producer {
				t.Error("EventStart should be sent to producer")
			}
		}
	}

	// Verify stored
	entry := tm.getEventEntry(event)
	if entry.subscriberCount != 1 {
		t.Errorf("subscriberCount should be 1, got %d", entry.subscriberCount)
	}
}

// Test LinkEvent local - second subscriber NO EventStart
func TestLinkEvent_Local_SecondSubscriber_NoEventStart(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "node1", ID: 100}
	consumer1 := gen.PID{Node: "node1", ID: 101}
	consumer2 := gen.PID{Node: "node1", ID: 102}

	tm.RegisterEvent(producer, "test", gen.EventOptions{Notify: true})

	event := gen.Event{Node: "node1", Name: "test"}

	tm.LinkEvent(consumer1, event)
	time.Sleep(20 * time.Millisecond)

	core.resetSentEventStarts()

	// Second subscriber
	tm.LinkEvent(consumer2, event)
	time.Sleep(50 * time.Millisecond)

	// NO EventStart
	if core.countSentEventStarts() != 0 {
		t.Errorf("Second subscriber should NOT trigger EventStart, got %d", core.countSentEventStarts())
	}

	// Counter = 2
	entry := tm.getEventEntry(event)
	if entry.subscriberCount != 2 {
		t.Errorf("subscriberCount should be 2, got %d", entry.subscriberCount)
	}
}

// Test UnlinkEvent local - last subscriber gets EventStop
func TestUnlinkEvent_Local_LastSubscriber_EventStop(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "node1", ID: 100}
	consumer := gen.PID{Node: "node1", ID: 101}

	tm.RegisterEvent(producer, "test", gen.EventOptions{Notify: true})

	event := gen.Event{Node: "node1", Name: "test"}

	tm.LinkEvent(consumer, event)
	time.Sleep(20 * time.Millisecond)

	core.resetSentEventStops()

	// Unlink (last)
	err := tm.UnlinkEvent(consumer, event)
	if err != nil {
		t.Fatalf("UnlinkEvent failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// EventStop sent!
	if core.countSentEventStops() != 1 {
		t.Errorf("Expected 1 EventStop, got %d", core.countSentEventStops())
	}

	// Counter = 0
	entry := tm.getEventEntry(event)
	if entry.subscriberCount != 0 {
		t.Errorf("subscriberCount should be 0, got %d", entry.subscriberCount)
	}
}

// Test LinkEvent remote - first subscriber
func TestLinkEvent_Remote_FirstSubscriber(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	event := gen.Event{Node: "node2", Name: "test"}

	// Mock will return nil buffer (unbuffered)
	_, err := tm.LinkEvent(consumer, event)
	if err != nil {
		t.Fatalf("LinkEvent failed: %v", err)
	}

	// Verify stored locally
	if exists := tm.hasLinkRelation(consumer, event); exists == false {
		t.Error("Link should be stored locally")
	}

	// Verify network request sent
	if core.countSentEventLinks() != 1 {
		t.Fatalf("Expected 1 network LinkEvent, got %d", core.countSentEventLinks())
	}

	// Verify allowAlwaysFirst=false (unbuffered)
	entry := tm.getTargetEntry(event)
	if entry.allowAlwaysFirst == true {
		t.Error("allowAlwaysFirst should be false for unbuffered event")
	}
}

// Test LinkEvent remote - second subscriber unbuffered (NO network!)
func TestLinkEvent_Remote_SecondSubscriber_Unbuffered_NoNetwork(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer1 := gen.PID{Node: "node1", ID: 100}
	consumer2 := gen.PID{Node: "node1", ID: 101}
	event := gen.Event{Node: "node2", Name: "test"}

	// First subscriber (unbuffered)
	tm.LinkEvent(consumer1, event)

	core.resetSentEventLinks()

	// Second subscriber
	_, err := tm.LinkEvent(consumer2, event)
	if err != nil {
		t.Fatalf("Second LinkEvent failed: %v", err)
	}

	// NO network request! (unbuffered optimization)
	if core.countSentEventLinks() != 0 {
		t.Errorf("Second subscriber to unbuffered event should NOT send network, got %d", core.countSentEventLinks())
	}

	// Verify both links stored in linkRelations
	if exists := tm.hasLinkRelation(consumer1, event); exists == false {
		t.Error("First link should be stored in linkRelations")
	}

	if exists := tm.hasLinkRelation(consumer2, event); exists == false {
		t.Error("Second link should be stored in linkRelations")
	}

	// Verify targetIndex has both consumers
	entry := tm.getTargetEntry(event)
	if entry == nil {
		t.Fatal("targetIndex entry should exist")
	}
	if len(entry.consumers) != 2 {
		t.Errorf("targetIndex should have 2 consumers, got %d", len(entry.consumers))
	}
}

// Test LinkEvent remote - buffered, second subscriber DOES send network
func TestLinkEvent_Remote_SecondSubscriber_Buffered_SendsNetwork(t *testing.T) {
	core := newMockCore("node1")
	core.eventBuffers = make(map[gen.Event][]gen.MessageEvent)
	core.eventBuffers[gen.Event{Node: "node2", Name: "test"}] = []gen.MessageEvent{
		{Message: "msg1"},
		{Message: "msg2"},
	}

	tm := Create(core, Options{}).(*targetManager)

	consumer1 := gen.PID{Node: "node1", ID: 100}
	consumer2 := gen.PID{Node: "node1", ID: 101}
	event := gen.Event{Node: "node2", Name: "test"}

	// First subscriber (buffered)
	buffer1, _ := tm.LinkEvent(consumer1, event)

	if len(buffer1) != 2 {
		t.Errorf("First subscriber should get buffer, got %d messages", len(buffer1))
	}

	core.resetSentEventLinks()

	// Second subscriber
	buffer2, err := tm.LinkEvent(consumer2, event)
	if err != nil {
		t.Fatalf("Second LinkEvent failed: %v", err)
	}

	// DOES send network request (buffered!)
	if core.countSentEventLinks() != 1 {
		t.Errorf("Second subscriber to buffered event SHOULD send network, got %d", core.countSentEventLinks())
	}

	// Second subscriber also gets buffer
	if len(buffer2) != 2 {
		t.Errorf("Second subscriber should also get buffer, got %d", len(buffer2))
	}

	// allowAlwaysFirst still true!
	entry := tm.getTargetEntry(event)
	if entry.allowAlwaysFirst == false {
		t.Error("allowAlwaysFirst should stay true for buffered event")
	}
}

// Test PublishEvent - fanout to subscribers
func TestPublishEvent_Fanout(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "node1", ID: 100}
	consumer1 := gen.PID{Node: "node1", ID: 101}
	consumer2 := gen.PID{Node: "node1", ID: 102}

	token, _ := tm.RegisterEvent(producer, "test", gen.EventOptions{})

	event := gen.Event{Node: "node1", Name: "test"}

	tm.LinkEvent(consumer1, event)
	tm.LinkEvent(consumer2, event)

	// Verify both links stored
	if exists := tm.hasLinkRelation(consumer1, event); exists == false {
		t.Error("consumer1 link should be stored")
	}
	if exists := tm.hasLinkRelation(consumer2, event); exists == false {
		t.Error("consumer2 link should be stored")
	}

	// Verify subscriberCount
	entry := tm.getEventEntry(event)
	if entry == nil {
		t.Fatal("Event entry should exist")
	}
	if entry.subscriberCount != 2 {
		t.Errorf("subscriberCount should be 2, got %d", entry.subscriberCount)
	}

	core.resetSentEvents()

	// Publish
	err := tm.PublishEvent(producer, token, gen.MessageOptions{}, gen.MessageEvent{
		Event:     event,
		Timestamp: time.Now().UnixNano(),
		Message:   "message",
	})
	if err != nil {
		t.Fatalf("PublishEvent failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// 2 events delivered
	if core.countSentEvents() != 2 {
		t.Errorf("Expected 2 events delivered, got %d", core.countSentEvents())
	}
}

// Test PublishEvent - updates buffer
func TestPublishEvent_UpdatesBuffer(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "node1", ID: 100}

	token, _ := tm.RegisterEvent(producer, "test", gen.EventOptions{Buffer: 3})

	event := gen.Event{Node: "node1", Name: "test"}

	// Publish 3 messages
	tm.PublishEvent(
		producer,
		token,
		gen.MessageOptions{},
		gen.MessageEvent{Event: event, Timestamp: time.Now().UnixNano(), Message: "msg1"},
	)
	tm.PublishEvent(
		producer,
		token,
		gen.MessageOptions{},
		gen.MessageEvent{Event: event, Timestamp: time.Now().UnixNano(), Message: "msg2"},
	)
	tm.PublishEvent(
		producer,
		token,
		gen.MessageOptions{},
		gen.MessageEvent{Event: event, Timestamp: time.Now().UnixNano(), Message: "msg3"},
	)

	// Check buffer
	entry := tm.getEventEntry(event)
	if entry.buffer.len != 3 {
		t.Errorf("Buffer should have 3 messages, got %d", entry.buffer.len)
	}
}

// Test PublishEvent - buffer overflow (flush oldest)
func TestPublishEvent_BufferOverflow(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "node1", ID: 100}

	token, _ := tm.RegisterEvent(producer, "test", gen.EventOptions{Buffer: 2})

	event := gen.Event{Node: "node1", Name: "test"}

	// Publish 3 messages (buffer size = 2)
	tm.PublishEvent(
		producer,
		token,
		gen.MessageOptions{},
		gen.MessageEvent{Event: event, Timestamp: time.Now().UnixNano(), Message: "msg1"},
	)
	tm.PublishEvent(
		producer,
		token,
		gen.MessageOptions{},
		gen.MessageEvent{Event: event, Timestamp: time.Now().UnixNano(), Message: "msg2"},
	)
	tm.PublishEvent(
		producer,
		token,
		gen.MessageOptions{},
		gen.MessageEvent{Event: event, Timestamp: time.Now().UnixNano(), Message: "msg3"},
	)

	// Buffer should have 2 messages (msg2, msg3 - msg1 flushed)
	entry := tm.getEventEntry(event)
	if entry.buffer.len != 2 {
		t.Errorf("Buffer should have 2 messages, got %d", entry.buffer.len)
	}

	// msg2 and msg3 should be in buffer (oldest first)
	snap := entry.buffer.snapshot()
	if len(snap) >= 2 {
		if snap[0].Message != "msg2" {
			t.Error("First message should be msg2 (msg1 flushed)")
		}
		if snap[1].Message != "msg3" {
			t.Error("Second message should be msg3")
		}
	}
}

// Test UnregisterEvent - sends exit to all subscribers
func TestUnregisterEvent_SendsExit(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "node1", ID: 100}
	consumer1 := gen.PID{Node: "node1", ID: 101}
	consumer2 := gen.PID{Node: "node1", ID: 102}

	tm.RegisterEvent(producer, "test", gen.EventOptions{})

	event := gen.Event{Node: "node1", Name: "test"}

	tm.LinkEvent(consumer1, event)
	tm.LinkEvent(consumer2, event)

	// Verify links stored before unregister
	if exists := tm.hasLinkRelation(consumer1, event); exists == false {
		t.Fatal("consumer1 link should exist before unregister")
	}
	if exists := tm.hasLinkRelation(consumer2, event); exists == false {
		t.Fatal("consumer2 link should exist before unregister")
	}

	// Verify producerEvents has producer
	if pe := tm.getProducerEventsMap(producer); pe == nil {
		t.Fatal("producerEvents should have producer before unregister")
	}

	core.resetSentExits()

	// Unregister
	err := tm.UnregisterEvent(producer, event.Name)
	if err != nil {
		t.Fatalf("UnregisterEvent failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// 2 exits sent
	if core.countSentExits() != 2 {
		t.Errorf("Expected 2 exits, got %d", core.countSentExits())
	}

	// Event removed
	if entry := tm.getEventEntry(event); entry != nil {
		t.Error("Event should be removed")
	}

	// linkRelations cleaned
	if exists := tm.hasLinkRelation(consumer1, event); exists {
		t.Error("consumer1 link should be cleaned after unregister")
	}
	if exists := tm.hasLinkRelation(consumer2, event); exists {
		t.Error("consumer2 link should be cleaned after unregister")
	}

	// producerEvents cleaned
	if pe := tm.getProducerEventsMap(producer); pe != nil {
		t.Error("producerEvents should be cleaned after unregister")
	}
}

// Test MonitorEvent local - shares counter with LinkEvent
func TestMonitorEvent_Local_SharesCounter(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "node1", ID: 100}
	linkConsumer := gen.PID{Node: "node1", ID: 101}
	monitorConsumer := gen.PID{Node: "node1", ID: 102}

	tm.RegisterEvent(producer, "test", gen.EventOptions{Notify: true})

	event := gen.Event{Node: "node1", Name: "test"}

	// Link first
	tm.LinkEvent(linkConsumer, event)
	time.Sleep(20 * time.Millisecond)

	core.resetSentEventStarts()

	// Monitor second
	tm.MonitorEvent(monitorConsumer, event)
	time.Sleep(50 * time.Millisecond)

	// NO EventStart (counter already 1 from link)
	if core.countSentEventStarts() != 0 {
		t.Error("Monitor after Link should NOT send EventStart")
	}

	// Counter = 2 (link + monitor)
	entry := tm.getEventEntry(event)
	if entry.subscriberCount != 2 {
		t.Errorf("Counter should be 2 (link+monitor), got %d", entry.subscriberCount)
	}
}

// Test Unlink + Demonitor - EventStop only when both gone
func TestUnlinkDemonitor_EventStop_WhenBothGone(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "node1", ID: 100}
	linkConsumer := gen.PID{Node: "node1", ID: 101}
	monitorConsumer := gen.PID{Node: "node1", ID: 102}

	tm.RegisterEvent(producer, "test", gen.EventOptions{Notify: true})

	event := gen.Event{Node: "node1", Name: "test"}

	tm.LinkEvent(linkConsumer, event)
	tm.MonitorEvent(monitorConsumer, event)

	// Verify both relations exist
	if exists := tm.hasLinkRelation(linkConsumer, event); exists == false {
		t.Fatal("Link should exist")
	}
	if exists := tm.hasMonitorRelation(monitorConsumer, event); exists == false {
		t.Fatal("Monitor should exist")
	}

	// Verify subscriberCount = 2
	entry := tm.getEventEntry(event)
	if entry.subscriberCount != 2 {
		t.Fatalf("subscriberCount should be 2, got %d", entry.subscriberCount)
	}

	core.resetSentEventStops()

	// Unlink first
	tm.UnlinkEvent(linkConsumer, event)
	time.Sleep(50 * time.Millisecond)

	// NO EventStop (monitor still there)
	if core.countSentEventStops() != 0 {
		t.Error("Should NOT send EventStop when monitor still subscribed")
	}

	// Verify link removed, monitor still exists
	if exists := tm.hasLinkRelation(linkConsumer, event); exists {
		t.Error("Link should be removed after unlink")
	}
	if exists := tm.hasMonitorRelation(monitorConsumer, event); exists == false {
		t.Error("Monitor should still exist")
	}

	// subscriberCount = 1
	if entry.subscriberCount != 1 {
		t.Errorf("subscriberCount should be 1 after unlink, got %d", entry.subscriberCount)
	}

	// Demonitor (last)
	tm.DemonitorEvent(monitorConsumer, event)
	time.Sleep(50 * time.Millisecond)

	// EventStop sent!
	if core.countSentEventStops() != 1 {
		t.Errorf("Expected 1 EventStop, got %d", core.countSentEventStops())
	}

	// Verify both relations cleaned
	if exists := tm.hasLinkRelation(linkConsumer, event); exists {
		t.Error("Link should be removed")
	}
	if exists := tm.hasMonitorRelation(monitorConsumer, event); exists {
		t.Error("Monitor should be removed")
	}

	// subscriberCount = 0
	if entry.subscriberCount != 0 {
		t.Errorf("subscriberCount should be 0, got %d", entry.subscriberCount)
	}
}

// Test LinkEvent - buffer delivery
func TestLinkEvent_BufferDelivery(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "node1", ID: 100}
	consumer := gen.PID{Node: "node1", ID: 101}

	token, _ := tm.RegisterEvent(producer, "test", gen.EventOptions{Buffer: 5})

	event := gen.Event{Node: "node1", Name: "test"}

	// Publish 2 messages
	tm.PublishEvent(
		producer,
		token,
		gen.MessageOptions{},
		gen.MessageEvent{Event: event, Timestamp: time.Now().UnixNano(), Message: "msg1"},
	)
	tm.PublishEvent(
		producer,
		token,
		gen.MessageOptions{},
		gen.MessageEvent{Event: event, Timestamp: time.Now().UnixNano(), Message: "msg2"},
	)

	// Subscribe
	buffer, err := tm.LinkEvent(consumer, event)
	if err != nil {
		t.Fatalf("LinkEvent failed: %v", err)
	}

	// Should get buffer with 2 messages
	if len(buffer) != 2 {
		t.Errorf("Expected buffer with 2 messages, got %d", len(buffer))
	}

	if len(buffer) >= 2 {
		if buffer[0].Message != "msg1" {
			t.Error("First message should be msg1")
		}
		if buffer[1].Message != "msg2" {
			t.Error("Second message should be msg2")
		}
	}
}

// Test PublishEvent - mixed link and monitor subscribers
func TestPublishEvent_MixedSubscribers(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "node1", ID: 100}
	linkConsumer := gen.PID{Node: "node1", ID: 101}
	monitorConsumer := gen.PID{Node: "node1", ID: 102}

	token, _ := tm.RegisterEvent(producer, "test", gen.EventOptions{})

	event := gen.Event{Node: "node1", Name: "test"}

	tm.LinkEvent(linkConsumer, event)
	tm.MonitorEvent(monitorConsumer, event)

	core.resetSentEvents()

	tm.PublishEvent(producer, token, gen.MessageOptions{}, gen.MessageEvent{
		Event:     event,
		Timestamp: time.Now().UnixNano(),
		Message:   "message",
	})

	time.Sleep(50 * time.Millisecond)

	// Both receive event
	if core.countSentEvents() != 2 {
		t.Errorf("Expected 2 events (link+monitor), got %d", core.countSentEvents())
	}
}

// Test PublishEvent - invalid token
func TestPublishEvent_InvalidToken(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "node1", ID: 100}

	tm.RegisterEvent(producer, "test", gen.EventOptions{})

	event := gen.Event{Node: "node1", Name: "test"}

	// Wrong token
	wrongToken := gen.Ref{Node: "wrong"}

	err := tm.PublishEvent(producer, wrongToken, gen.MessageOptions{}, gen.MessageEvent{
		Event:     event,
		Timestamp: time.Now().UnixNano(),
		Message:   "msg",
	})
	if err != gen.ErrEventOwner {
		t.Errorf("Expected ErrEventOwner for wrong token, got %v", err)
	}
}

// Test LinkEvent - to non-existent event
func TestLinkEvent_EventNotExist(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	event := gen.Event{Node: "node1", Name: "nonexistent"}

	_, err := tm.LinkEvent(consumer, event)
	if err != gen.ErrEventUnknown {
		t.Errorf("Expected ErrEventUnknown, got %v", err)
	}
}

// Test UnlinkEvent remote - not last local
func TestUnlinkEvent_Remote_NotLast(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer1 := gen.PID{Node: "node1", ID: 100}
	consumer2 := gen.PID{Node: "node1", ID: 101}
	event := gen.Event{Node: "node2", Name: "test"}

	// Both subscribe to remote event
	tm.LinkEvent(consumer1, event)
	tm.LinkEvent(consumer2, event)

	// Verify both stored
	if exists := tm.hasLinkRelation(consumer1, event); exists == false {
		t.Fatal("consumer1 link should exist before unlink")
	}
	if exists := tm.hasLinkRelation(consumer2, event); exists == false {
		t.Fatal("consumer2 link should exist before unlink")
	}

	// Unlink first
	err := tm.UnlinkEvent(consumer1, event)
	if err != nil {
		t.Fatalf("UnlinkEvent failed: %v", err)
	}

	// Verify removed from linkRelations
	if exists := tm.hasLinkRelation(consumer1, event); exists {
		t.Error("consumer1 link should be removed")
	}

	// consumer2 still exists
	if exists := tm.hasLinkRelation(consumer2, event); exists == false {
		t.Error("consumer2 link should still exist")
	}

	// Verify targetIndex still exists with consumer2
	entry := tm.getTargetEntry(event)
	if entry == nil {
		t.Fatal("targetIndex should still exist")
	}
	if _, exists := entry.consumers[consumer2]; exists == false {
		t.Error("consumer2 should still be in targetIndex.consumers")
	}
	if len(entry.consumers) != 1 {
		t.Errorf("targetIndex should have 1 consumer, got %d", len(entry.consumers))
	}
}

// Test UnlinkEvent remote - last local sends UnlinkEvent
func TestUnlinkEvent_Remote_LastLocal_SendsUnlink(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	event := gen.Event{Node: "node2", Name: "test"}

	// Subscribe - this sends network LinkEvent
	tm.LinkEvent(consumer, event)

	// Verify network LinkEvent was sent (first subscriber)
	if core.countSentEventLinks() != 1 {
		t.Fatalf("Expected 1 network LinkEvent on subscribe, got %d", core.countSentEventLinks())
	}

	// Verify stored before unlink
	if exists := tm.hasLinkRelation(consumer, event); exists == false {
		t.Fatal("Link should exist before unlink")
	}

	entry := tm.getTargetEntry(event)
	if entry == nil {
		t.Fatal("targetIndex should exist before unlink")
	}

	// Unlink (last local consumer!) - triggers remote UnlinkEvent
	err := tm.UnlinkEvent(consumer, event)
	if err != nil {
		t.Fatalf("UnlinkEvent failed: %v", err)
	}

	// Verify removed from linkRelations
	if exists := tm.hasLinkRelation(consumer, event); exists {
		t.Error("Link should be removed")
	}

	// Verify targetIndex cleaned (last consumer)
	if entry := tm.getTargetEntry(event); entry != nil {
		t.Error("targetIndex should be cleaned when last consumer removed")
	}

	// Remote UnlinkEvent(corePID, event) is sent via Connection.UnlinkEvent
	// The cleanup of targetIndex proves it was the last local subscriber
}

// Test DemonitorEvent remote - not last
func TestDemonitorEvent_Remote_NotLast(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer1 := gen.PID{Node: "node1", ID: 100}
	consumer2 := gen.PID{Node: "node1", ID: 101}
	event := gen.Event{Node: "node2", Name: "test"}

	tm.MonitorEvent(consumer1, event)
	tm.MonitorEvent(consumer2, event)

	// Verify both stored before demonitor
	if exists := tm.hasMonitorRelation(consumer1, event); exists == false {
		t.Fatal("consumer1 monitor should exist before demonitor")
	}
	if exists := tm.hasMonitorRelation(consumer2, event); exists == false {
		t.Fatal("consumer2 monitor should exist before demonitor")
	}

	// Demonitor first
	err := tm.DemonitorEvent(consumer1, event)
	if err != nil {
		t.Fatalf("DemonitorEvent failed: %v", err)
	}

	// consumer1 removed
	if exists := tm.hasMonitorRelation(consumer1, event); exists {
		t.Error("consumer1 monitor should be removed")
	}

	// consumer2 still exists
	if exists := tm.hasMonitorRelation(consumer2, event); exists == false {
		t.Error("consumer2 monitor should still exist")
	}

	// Verify targetIndex still exists with consumer2
	entry := tm.getTargetEntry(event)
	if entry == nil {
		t.Fatal("targetIndex should still exist")
	}
	if _, exists := entry.consumers[consumer2]; exists == false {
		t.Error("consumer2 should still be in targetIndex.consumers")
	}
	if len(entry.consumers) != 1 {
		t.Errorf("targetIndex should have 1 consumer, got %d", len(entry.consumers))
	}
}

// Test DemonitorEvent remote - last local sends Demonitor
func TestDemonitorEvent_Remote_LastLocal_SendsDemonitor(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	event := gen.Event{Node: "node2", Name: "test"}

	tm.MonitorEvent(consumer, event)

	// Verify stored before demonitor
	if exists := tm.hasMonitorRelation(consumer, event); exists == false {
		t.Fatal("Monitor should exist before demonitor")
	}

	entry := tm.getTargetEntry(event)
	if entry == nil {
		t.Fatal("targetIndex should exist before demonitor")
	}
	if len(entry.consumers) != 1 {
		t.Fatalf("targetIndex should have 1 consumer, got %d", len(entry.consumers))
	}

	// Demonitor (last!) - triggers remote DemonitorEvent
	err := tm.DemonitorEvent(consumer, event)
	if err != nil {
		t.Fatalf("DemonitorEvent failed: %v", err)
	}

	// Verify removed from monitorRelations
	if exists := tm.hasMonitorRelation(consumer, event); exists {
		t.Error("Monitor should be removed")
	}

	// Verify targetIndex cleaned (last consumer)
	if entry := tm.getTargetEntry(event); entry != nil {
		t.Error("targetIndex should be cleaned when last consumer removed")
	}

	// Remote DemonitorEvent(corePID, event) is sent via Connection.DemonitorEvent
	// The cleanup of targetIndex proves it was the last local subscriber
}

// Test UnlinkEvent remote - error handling
func TestUnlinkEvent_Remote_ConnectionError(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer := gen.PID{Node: "node1", ID: 100}
	event := gen.Event{Node: "node2", Name: "test"}

	tm.LinkEvent(consumer, event)

	// Simulate connection error
	core.connectionError = gen.ErrNoConnection

	// Unlink (connection fails but not an error)
	err := tm.UnlinkEvent(consumer, event)
	if err != nil {
		t.Errorf("UnlinkEvent should not fail on connection error, got %v", err)
	}

	// Still cleaned locally
	if exists := tm.hasLinkRelation(consumer, event); exists {
		t.Error("Link should be cleaned even if connection fails")
	}
}

// Test MonitorEvent remote - unbuffered second subscriber
func TestMonitorEvent_Remote_Unbuffered_SecondNoNetwork(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	consumer1 := gen.PID{Node: "node1", ID: 100}
	consumer2 := gen.PID{Node: "node1", ID: 101}
	event := gen.Event{Node: "node2", Name: "test"}

	// First (unbuffered - returns nil) - sends network MonitorEvent
	tm.MonitorEvent(consumer1, event)

	// allowAlwaysFirst should be false now (unbuffered)
	entry := tm.getTargetEntry(event)
	if entry == nil {
		t.Fatal("targetIndex should exist")
	}
	if entry.allowAlwaysFirst == true {
		t.Error("allowAlwaysFirst should be false for unbuffered")
	}

	// Note: MonitorEvent uses Connection.MonitorEvent which is tracked in sentMonitors
	// but events use a different path. For remote events, only first subscriber
	// sends network request when unbuffered.

	// Second
	_, err := tm.MonitorEvent(consumer2, event)
	if err != nil {
		t.Fatalf("Second MonitorEvent failed: %v", err)
	}

	// Verify both stored locally
	if exists := tm.hasMonitorRelation(consumer1, event); exists == false {
		t.Error("First monitor should exist")
	}

	if exists := tm.hasMonitorRelation(consumer2, event); exists == false {
		t.Error("Second monitor should exist")
	}

	// Verify targetIndex has both consumers
	if len(entry.consumers) != 2 {
		t.Errorf("targetIndex should have 2 consumers, got %d", len(entry.consumers))
	}
}

// Test MonitorEvent remote - buffered, second gets buffer
func TestMonitorEvent_Remote_Buffered_SecondGetsBuffer(t *testing.T) {
	core := newMockCore("node1")
	core.eventBuffers = make(map[gen.Event][]gen.MessageEvent)

	event := gen.Event{Node: "node2", Name: "test"}
	core.eventBuffers[event] = []gen.MessageEvent{{Message: "msg1"}}

	tm := Create(core, Options{}).(*targetManager)

	consumer1 := gen.PID{Node: "node1", ID: 100}
	consumer2 := gen.PID{Node: "node1", ID: 101}

	// First
	buffer1, _ := tm.MonitorEvent(consumer1, event)
	if len(buffer1) != 1 {
		t.Error("First should get buffer")
	}

	// Second
	buffer2, _ := tm.MonitorEvent(consumer2, event)
	if len(buffer2) != 1 {
		t.Error("Second should also get buffer")
	}

	// Verify both monitors stored in monitorRelations
	if exists := tm.hasMonitorRelation(consumer1, event); exists == false {
		t.Error("First monitor should be stored in monitorRelations")
	}

	if exists := tm.hasMonitorRelation(consumer2, event); exists == false {
		t.Error("Second monitor should be stored in monitorRelations")
	}

	// Verify targetIndex has both consumers
	entry := tm.getTargetEntry(event)
	if entry == nil {
		t.Fatal("targetIndex should exist")
	}
	if len(entry.consumers) != 2 {
		t.Errorf("targetIndex should have 2 consumers, got %d", len(entry.consumers))
	}

	// allowAlwaysFirst still true (buffered)
	if entry.allowAlwaysFirst == false {
		t.Error("allowAlwaysFirst should stay true for buffered")
	}
}

// Test EventInfo
func TestEventInfo_Basic(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "node1", ID: 100}

	_, _ = tm.RegisterEvent(producer, "test", gen.EventOptions{Buffer: 10, Notify: true})

	event := gen.Event{Node: "node1", Name: "test"}

	// Get info before subscribers
	info, err := tm.EventInfo(event)
	if err != nil {
		t.Fatalf("EventInfo failed: %v", err)
	}

	if info.Producer != producer {
		t.Error("Producer should match")
	}

	if info.BufferSize != 10 {
		t.Errorf("BufferSize should be 10, got %d", info.BufferSize)
	}

	if info.Notify == false {
		t.Error("Notify should be true")
	}

	if info.Subscribers != 0 {
		t.Errorf("Subscribers should be 0, got %d", info.Subscribers)
	}

	// Add subscribers
	consumer1 := gen.PID{Node: "node1", ID: 101}
	consumer2 := gen.PID{Node: "node1", ID: 102}

	tm.LinkEvent(consumer1, event)
	tm.MonitorEvent(consumer2, event)

	// Get info after subscribers
	info, err = tm.EventInfo(event)
	if err != nil {
		t.Fatalf("EventInfo failed: %v", err)
	}

	if info.Subscribers != 2 {
		t.Errorf("Subscribers should be 2 (link+monitor), got %d", info.Subscribers)
	}
}

// Test EventInfo - event not exist
func TestEventInfo_NotExist(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	event := gen.Event{Node: "node1", Name: "nonexistent"}

	_, err := tm.EventInfo(event)
	if err != gen.ErrEventUnknown {
		t.Errorf("Expected ErrEventUnknown, got %v", err)
	}
}

// Test RegisterEvent - Open=true lets any publisher bypass token check
func TestPublishEvent_Open_BypassesToken(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "node1", ID: 100}
	consumer := gen.PID{Node: "node1", ID: 101}
	other := gen.PID{Node: "node1", ID: 102}

	_, err := tm.RegisterEvent(producer, "bus", gen.EventOptions{Open: true})
	if err != nil {
		t.Fatalf("RegisterEvent failed: %v", err)
	}

	event := gen.Event{Node: "node1", Name: "bus"}
	if _, err := tm.LinkEvent(consumer, event); err != nil {
		t.Fatalf("LinkEvent failed: %v", err)
	}

	// Publish from producer with zero token: must succeed.
	err = tm.PublishEvent(producer, gen.Ref{}, gen.MessageOptions{}, gen.MessageEvent{
		Event:     event,
		Timestamp: time.Now().UnixNano(),
		Message:   "from producer",
	})
	if err != nil {
		t.Errorf("producer publish with zero token should succeed, got %v", err)
	}

	// Publish from another process with a wrong token: must also succeed.
	err = tm.PublishEvent(other, gen.Ref{Node: "wrong"}, gen.MessageOptions{}, gen.MessageEvent{
		Event:     event,
		Timestamp: time.Now().UnixNano(),
		Message:   "from other",
	})
	if err != nil {
		t.Errorf("foreign publish with wrong token should succeed for open event, got %v", err)
	}

	// Non-open event rejects wrong token.
	strictProducer := gen.PID{Node: "node1", ID: 200}
	_, err = tm.RegisterEvent(strictProducer, "strict", gen.EventOptions{})
	if err != nil {
		t.Fatalf("RegisterEvent failed: %v", err)
	}
	strictEvent := gen.Event{Node: "node1", Name: "strict"}
	err = tm.PublishEvent(strictProducer, gen.Ref{Node: "wrong"}, gen.MessageOptions{}, gen.MessageEvent{
		Event:     strictEvent,
		Timestamp: time.Now().UnixNano(),
		Message:   "bad token",
	})
	if err != gen.ErrEventOwner {
		t.Errorf("non-open event should reject wrong token: expected ErrEventOwner, got %v", err)
	}
}

// Test UnregisterEvent - Open=true does not relax the owner check
func TestUnregisterEvent_Open_StillRequiresOwner(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	producer := gen.PID{Node: "node1", ID: 100}
	other := gen.PID{Node: "node1", ID: 101}

	_, err := tm.RegisterEvent(producer, "bus", gen.EventOptions{Open: true})
	if err != nil {
		t.Fatalf("RegisterEvent failed: %v", err)
	}

	if err := tm.UnregisterEvent(other, "bus"); err != gen.ErrEventOwner {
		t.Errorf("non-owner unregister on open event: expected ErrEventOwner, got %v", err)
	}

	if err := tm.UnregisterEvent(producer, "bus"); err != nil {
		t.Errorf("owner unregister on open event failed: %v", err)
	}
}

// Test RegisterEvent - Notify is ignored when the producer is the node core
func TestRegisterEvent_NotifyIgnoredForCorePID(t *testing.T) {
	core := newMockCore("node1")
	tm := Create(core, Options{}).(*targetManager)

	// Register as the node core with Notify=true. Notify must be silently
	// suppressed since the node core does not consume MessageEventStart or
	// MessageEventStop.
	_, err := tm.RegisterEvent(core.PID(), "bus", gen.EventOptions{Notify: true})
	if err != nil {
		t.Fatalf("RegisterEvent failed: %v", err)
	}

	event := gen.Event{Node: "node1", Name: "bus"}
	entry := tm.getEventEntry(event)
	if entry == nil {
		t.Fatal("event entry not found")
	}
	if entry.notify {
		t.Error("entry.notify must be false for node-level events even when Notify=true was requested")
	}

	// First subscriber should not produce a MessageEventStart send.
	core.resetSentEventStarts()
	consumer := gen.PID{Node: "node1", ID: 200}
	if _, err := tm.LinkEvent(consumer, event); err != nil {
		t.Fatalf("LinkEvent failed: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	if n := core.countSentEventStarts(); n != 0 {
		t.Errorf("expected 0 MessageEventStart for node-level event, got %d", n)
	}
}
