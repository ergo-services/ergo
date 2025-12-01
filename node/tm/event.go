package tm

import (
	"time"

	"ergo.services/ergo/gen"
)

func (tm *targetManager) RegisterEvent(producer gen.PID, name gen.Atom, options gen.EventOptions) (gen.Ref, error) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	event := gen.Event{Node: tm.core.Name(), Name: name}

	// Check if already exists
	if _, exists := tm.events[event]; exists {
		return gen.Ref{}, gen.ErrTaken
	}

	// Generate unique token
	token := gen.Ref{
		Node:     tm.core.Name(),
		Creation: tm.core.PID().Creation,
		ID:       [3]uint64{uint64(time.Now().UnixNano()), 0, 0},
	}

	// Create event entry
	entry := &eventEntry{
		producer:                producer,
		token:                   token,
		notify:                  options.Notify,
		linkSubscribersIndex:    make(map[gen.PID]int),
		monitorSubscribersIndex: make(map[gen.PID]int),
		subscriberCount:         0,
	}

	// Create buffer if configured
	if options.Buffer > 0 {
		entry.buffer = make([]gen.MessageEvent, 0, options.Buffer)
		entry.bufferSize = options.Buffer
	}

	tm.events[event] = entry

	// Add to producerEvents index
	if tm.producerEvents[producer] == nil {
		tm.producerEvents[producer] = make(map[gen.Event]struct{})
	}
	tm.producerEvents[producer][event] = struct{}{}

	return token, nil
}

func (tm *targetManager) UnregisterEvent(producer gen.PID, name gen.Atom) error {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	event := gen.Event{Node: tm.core.Name(), Name: name}
	entry, exists := tm.events[event]
	if exists == false {
		return gen.ErrEventUnknown
	}

	if entry.producer != producer {
		return gen.ErrEventOwner
	}

	// Collect all subscribers (links + monitors)
	remoteNodes := make(map[gen.Atom]bool)
	var localExitConsumers []gen.PID
	var localDownConsumers []gen.PID

	for _, consumer := range entry.linkSubscribers {
		if consumer.Node == tm.core.Name() {
			localExitConsumers = append(localExitConsumers, consumer)
		} else {
			remoteNodes[consumer.Node] = true
		}
	}

	for _, consumer := range entry.monitorSubscribers {
		if consumer.Node == tm.core.Name() {
			localDownConsumers = append(localDownConsumers, consumer)
		} else {
			remoteNodes[consumer.Node] = true
		}
	}

	// Send exit messages to local link subscribers
	if len(localExitConsumers) > 0 {
		tm.core.RouteSendExitMessages(
			tm.core.PID(),
			localExitConsumers,
			gen.MessageExitEvent{Event: event, Reason: gen.ErrUnregistered},
		)
	}

	// Send down messages to local monitor subscribers
	for _, consumer := range localDownConsumers {
		tm.core.RouteSendPID(
			tm.core.PID(),
			consumer,
			gen.MessageOptions{Priority: gen.MessagePriorityHigh},
			gen.MessageDownEvent{Event: event, Reason: gen.ErrUnregistered},
		)
	}

	// Send to remote nodes
	for node := range remoteNodes {
		connection, err := tm.core.GetConnection(node)
		if err != nil {
			continue
		}
		connection.SendTerminateEvent(event, gen.ErrUnregistered)
	}

	// Cleanup event
	delete(tm.events, event)

	// Cleanup producerEvents index
	if events := tm.producerEvents[producer]; events != nil {
		delete(events, event)
		if len(events) == 0 {
			delete(tm.producerEvents, producer)
		}
	}

	// Cleanup relations
	for key := range tm.linkRelations {
		if key.target == event {
			delete(tm.linkRelations, key)
		}
	}

	for key := range tm.monitorRelations {
		if key.target == event {
			delete(tm.monitorRelations, key)
		}
	}

	// Cleanup targetIndex
	delete(tm.targetIndex, event)

	return nil
}

func (tm *targetManager) PublishEvent(
	from gen.PID,
	token gen.Ref,
	options gen.MessageOptions,
	message gen.MessageEvent,
) error {
	// Check if this is a LOCAL producer or REMOTE event
	if from.Node == tm.core.Name() {
		// LOCAL producer - needs Lock (writes to buffer)
		return tm.publishEventLocalProducer(from, token, options, message)
	}

	// REMOTE event - needs only RLock (read-only)
	tm.mutex.RLock()
	defer tm.mutex.RUnlock()
	return tm.publishEventRemoteProducer(from, options, message)
}

func (tm *targetManager) publishEventLocalProducer(
	from gen.PID,
	token gen.Ref,
	options gen.MessageOptions,
	message gen.MessageEvent,
) error {
	tm.mutex.Lock()

	entry, exists := tm.events[message.Event]
	if exists == false {
		tm.mutex.Unlock()
		return gen.ErrEventUnknown
	}

	// Validate token
	if entry.token != token {
		tm.mutex.Unlock()
		return gen.ErrEventOwner
	}

	// Store in buffer if configured (needs write lock)
	if entry.buffer != nil {
		// Re-check after lock upgrade
		entry, exists = tm.events[message.Event]
		if exists == false || entry.token != token {
			tm.mutex.Unlock()
			return gen.ErrEventOwner
		}
		if len(entry.buffer) < entry.bufferSize {
			entry.buffer = append(entry.buffer, message)
		} else {
			copy(entry.buffer, entry.buffer[1:])
			entry.buffer[entry.bufferSize-1] = message
		}
	}

	// Copy slices under lock (minimize lock hold time)
	linkSubs := entry.linkSubscribers
	monitorSubs := entry.monitorSubscribers
	tm.mutex.Unlock()

	// Increment published counter
	tm.eventsPublished.Add(1)

	// Collect local consumers and remote nodes (no lock needed)
	var localConsumers []gen.PID
	remoteNodes := make(map[gen.Atom]bool)

	// Fanout to link subscribers
	for _, consumer := range linkSubs {
		if consumer.Node != tm.core.Name() {
			remoteNodes[consumer.Node] = true
			continue
		}
		localConsumers = append(localConsumers, consumer)
	}

	// Fanout to monitor subscribers
	for _, consumer := range monitorSubs {
		if consumer.Node != tm.core.Name() {
			remoteNodes[consumer.Node] = true
			continue
		}
		localConsumers = append(localConsumers, consumer)
	}

	// Send to local consumers directly
	if len(localConsumers) > 0 {
		tm.core.RouteSendEventMessages(from, localConsumers, options, message)
		tm.eventsSent.Add(int64(len(localConsumers)))
	}

	// Send to remote nodes
	for node := range remoteNodes {
		connection, err := tm.core.GetConnection(node)
		if err != nil {
			continue
		}
		connection.SendEvent(from, options, message)
		tm.eventsSent.Add(1)
	}

	return nil
}

func (tm *targetManager) publishEventRemoteProducer(
	from gen.PID,
	options gen.MessageOptions,
	message gen.MessageEvent,
) error {
	// Remote event arrived - deliver to local subscribers only
	entry := tm.targetIndex[message.Event]
	if entry == nil {
		return nil
	}

	// Increment published counter
	tm.eventsPublished.Add(1)

	// Collect local consumers for batch delivery
	var localConsumers []gen.PID

	for consumer := range entry.consumers {
		if consumer.Node == tm.core.Name() {
			localConsumers = append(localConsumers, consumer)
		}
	}

	if len(localConsumers) > 0 {
		tm.core.RouteSendEventMessages(from, localConsumers, options, message)
		tm.eventsSent.Add(int64(len(localConsumers)))
	}

	return nil
}

func (tm *targetManager) LinkEvent(consumer gen.PID, event gen.Event) ([]gen.MessageEvent, error) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	// Check if local or remote event
	if event.Node == tm.core.Name() {
		// LOCAL event
		return tm.linkEventLocal(consumer, event)
	}

	// REMOTE event
	return tm.linkEventRemote(consumer, event)
}

func (tm *targetManager) linkEventLocal(consumer gen.PID, event gen.Event) ([]gen.MessageEvent, error) {
	entry, exists := tm.events[event]
	if exists == false {
		return nil, gen.ErrEventUnknown
	}

	key := relationKey{
		consumer: consumer,
		target:   event,
	}

	// Check duplicate
	if _, exists := tm.linkRelations[key]; exists {
		// For local event, duplicate from REMOTE CorePID is allowed
		if consumer.Node != tm.core.Name() {
			// Remote CorePID - return buffer anyway
			return tm.getEventBuffer(entry), nil
		}

		return nil, gen.ErrTargetExist
	}

	// Add subscription
	tm.linkRelations[key] = struct{}{}
	entry.linkSubscribersIndex[consumer] = len(entry.linkSubscribers)
	entry.linkSubscribers = append(entry.linkSubscribers, consumer)

	// Increment counter
	entry.subscriberCount++

	// Check if need to send EventStart
	if entry.subscriberCount == 1 && entry.notify {
		tm.core.RouteSendPID(
			tm.core.PID(),
			entry.producer,
			gen.MessageOptions{Priority: gen.MessagePriorityHigh},
			gen.MessageEventStart{Name: event.Name},
		)
	}

	// Return buffer
	return tm.getEventBuffer(entry), nil
}

func (tm *targetManager) linkEventRemote(consumer gen.PID, event gen.Event) ([]gen.MessageEvent, error) {
	key := relationKey{
		consumer: consumer,
		target:   event,
	}

	if _, exists := tm.linkRelations[key]; exists {
		return nil, gen.ErrTargetExist
	}

	// Add subscription locally
	tm.linkRelations[key] = struct{}{}

	// Check targetIndex for remote request decision
	entry := tm.targetIndex[event]
	needsRemote := false

	if entry == nil {
		// First subscriber
		entry = &targetEntry{
			allowAlwaysFirst: true, // Start with true for buffered events
			consumers:        make(map[gen.PID]struct{}),
		}
		tm.targetIndex[event] = entry
		needsRemote = true
	}

	// For events, allowAlwaysFirst stays true for buffered!
	if entry.allowAlwaysFirst == true {
		needsRemote = true
	}

	entry.consumers[consumer] = struct{}{}

	if needsRemote == false {
		// Unbuffered event, not first - no remote request
		return nil, nil
	}

	// Send remote LinkEvent
	connection, err := tm.core.GetConnection(event.Node)
	if err != nil {
		// Rollback
		delete(tm.linkRelations, key)
		delete(entry.consumers, consumer)

		if len(entry.consumers) == 0 {
			delete(tm.targetIndex, event)
		}

		return nil, err
	}

	buffer, err := connection.LinkEvent(tm.core.PID(), event)
	if err != nil {
		// Rollback
		delete(tm.linkRelations, key)
		delete(entry.consumers, consumer)

		if len(entry.consumers) == 0 {
			delete(tm.targetIndex, event)
		}

		return nil, err
	}

	// Success!
	// If buffer == nil: unbuffered, set allowAlwaysFirst=false
	// If buffer != nil: buffered, keep allowAlwaysFirst=true
	if buffer == nil {
		entry.allowAlwaysFirst = false
	}
	// else: buffered, keep allowAlwaysFirst=true for next subscribers!

	return buffer, nil
}

func (tm *targetManager) UnlinkEvent(consumer gen.PID, event gen.Event) error {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	// Check if local or remote
	if event.Node == tm.core.Name() {
		return tm.unlinkEventLocal(consumer, event)
	}

	return tm.unlinkEventRemote(consumer, event)
}

func (tm *targetManager) unlinkEventLocal(consumer gen.PID, event gen.Event) error {
	entry, exists := tm.events[event]
	if exists == false {
		return gen.ErrEventUnknown
	}

	key := relationKey{
		consumer: consumer,
		target:   event,
	}

	if _, exists := tm.linkRelations[key]; exists == false {
		return gen.ErrTargetUnknown
	}

	// Remove subscription
	delete(tm.linkRelations, key)

	// Swap-delete from slice
	idx := entry.linkSubscribersIndex[consumer]
	last := len(entry.linkSubscribers) - 1
	if idx != last {
		entry.linkSubscribers[idx] = entry.linkSubscribers[last]
		entry.linkSubscribersIndex[entry.linkSubscribers[idx]] = idx
	}
	entry.linkSubscribers = entry.linkSubscribers[:last]
	delete(entry.linkSubscribersIndex, consumer)

	// Decrement counter
	entry.subscriberCount--

	// Send EventStop if last subscriber
	if entry.subscriberCount == 0 && entry.notify {
		tm.core.RouteSendPID(
			tm.core.PID(),
			entry.producer,
			gen.MessageOptions{Priority: gen.MessagePriorityHigh},
			gen.MessageEventStop{Name: event.Name},
		)
	}

	return nil
}

func (tm *targetManager) unlinkEventRemote(consumer gen.PID, event gen.Event) error {
	key := relationKey{
		consumer: consumer,
		target:   event,
	}

	if _, exists := tm.linkRelations[key]; exists == false {
		return gen.ErrTargetUnknown
	}

	delete(tm.linkRelations, key)

	entry := tm.targetIndex[event]
	if entry == nil {
		return nil
	}

	delete(entry.consumers, consumer)

	isLast := (len(entry.consumers) == 0)

	if isLast {
		delete(tm.targetIndex, event)
	}

	// Check if need to send remote UnlinkEvent
	if isLast == false {
		hasLocal := false
		for pid := range entry.consumers {
			if pid.Node == tm.core.Name() && pid != tm.core.PID() {
				hasLocal = true
				break
			}
		}

		if hasLocal {
			return nil
		}
	}

	// Last local consumer - send remote UnlinkEvent
	connection, err := tm.core.GetConnection(event.Node)
	if err != nil {
		return nil
	}

	connection.UnlinkEvent(tm.core.PID(), event)

	return nil
}

func (tm *targetManager) MonitorEvent(consumer gen.PID, event gen.Event) ([]gen.MessageEvent, error) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	// Same logic as LinkEvent, but for monitors
	if event.Node == tm.core.Name() {
		return tm.monitorEventLocal(consumer, event)
	}

	return tm.monitorEventRemote(consumer, event)
}

func (tm *targetManager) monitorEventLocal(consumer gen.PID, event gen.Event) ([]gen.MessageEvent, error) {
	entry, exists := tm.events[event]
	if exists == false {
		return nil, gen.ErrEventUnknown
	}

	key := relationKey{
		consumer: consumer,
		target:   event,
	}

	if _, exists := tm.monitorRelations[key]; exists {
		if consumer.Node != tm.core.Name() {
			// Remote CorePID duplicate - return buffer
			return tm.getEventBuffer(entry), nil
		}

		return nil, gen.ErrTargetExist
	}

	// Add subscription
	tm.monitorRelations[key] = struct{}{}
	entry.monitorSubscribersIndex[consumer] = len(entry.monitorSubscribers)
	entry.monitorSubscribers = append(entry.monitorSubscribers, consumer)

	// Increment counter (shared with links!)
	entry.subscriberCount++

	// Send EventStart if first subscriber overall
	if entry.subscriberCount == 1 && entry.notify {
		tm.core.RouteSendPID(
			tm.core.PID(),
			entry.producer,
			gen.MessageOptions{Priority: gen.MessagePriorityHigh},
			gen.MessageEventStart{Name: event.Name},
		)
	}

	// Return buffer
	return tm.getEventBuffer(entry), nil
}

func (tm *targetManager) monitorEventRemote(consumer gen.PID, event gen.Event) ([]gen.MessageEvent, error) {
	key := relationKey{
		consumer: consumer,
		target:   event,
	}

	if _, exists := tm.monitorRelations[key]; exists {
		return nil, gen.ErrTargetExist
	}

	tm.monitorRelations[key] = struct{}{}

	entry := tm.targetIndex[event]
	needsRemote := false

	if entry == nil {
		entry = &targetEntry{
			allowAlwaysFirst: true,
			consumers:        make(map[gen.PID]struct{}),
		}
		tm.targetIndex[event] = entry
		needsRemote = true
	}

	if entry.allowAlwaysFirst == true {
		needsRemote = true
	}

	entry.consumers[consumer] = struct{}{}

	if needsRemote == false {
		return nil, nil
	}

	connection, err := tm.core.GetConnection(event.Node)
	if err != nil {
		delete(tm.monitorRelations, key)
		delete(entry.consumers, consumer)

		if len(entry.consumers) == 0 {
			delete(tm.targetIndex, event)
		}

		return nil, err
	}

	buffer, err := connection.MonitorEvent(tm.core.PID(), event)
	if err != nil {
		delete(tm.monitorRelations, key)
		delete(entry.consumers, consumer)

		if len(entry.consumers) == 0 {
			delete(tm.targetIndex, event)
		}

		return nil, err
	}

	// Buffered vs unbuffered
	if buffer == nil {
		entry.allowAlwaysFirst = false
	}

	return buffer, nil
}

func (tm *targetManager) DemonitorEvent(consumer gen.PID, event gen.Event) error {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	if event.Node == tm.core.Name() {
		return tm.demonitorEventLocal(consumer, event)
	}

	return tm.demonitorEventRemote(consumer, event)
}

func (tm *targetManager) demonitorEventLocal(consumer gen.PID, event gen.Event) error {
	entry, exists := tm.events[event]
	if exists == false {
		return gen.ErrEventUnknown
	}

	key := relationKey{
		consumer: consumer,
		target:   event,
	}

	if _, exists := tm.monitorRelations[key]; exists == false {
		return gen.ErrTargetUnknown
	}

	delete(tm.monitorRelations, key)

	// Swap-delete from slice
	idx := entry.monitorSubscribersIndex[consumer]
	last := len(entry.monitorSubscribers) - 1
	if idx != last {
		entry.monitorSubscribers[idx] = entry.monitorSubscribers[last]
		entry.monitorSubscribersIndex[entry.monitorSubscribers[idx]] = idx
	}
	entry.monitorSubscribers = entry.monitorSubscribers[:last]
	delete(entry.monitorSubscribersIndex, consumer)

	// Decrement counter (shared with links!)
	entry.subscriberCount--

	// Send EventStop if last
	if entry.subscriberCount == 0 && entry.notify {
		tm.core.RouteSendPID(
			tm.core.PID(),
			entry.producer,
			gen.MessageOptions{Priority: gen.MessagePriorityHigh},
			gen.MessageEventStop{Name: event.Name},
		)
	}

	return nil
}

func (tm *targetManager) demonitorEventRemote(consumer gen.PID, event gen.Event) error {
	key := relationKey{
		consumer: consumer,
		target:   event,
	}

	if _, exists := tm.monitorRelations[key]; exists == false {
		return gen.ErrTargetUnknown
	}

	delete(tm.monitorRelations, key)

	entry := tm.targetIndex[event]
	if entry == nil {
		return nil
	}

	delete(entry.consumers, consumer)

	isLast := (len(entry.consumers) == 0)

	if isLast {
		delete(tm.targetIndex, event)
	}

	if isLast == false {
		hasLocal := false
		for pid := range entry.consumers {
			if pid.Node == tm.core.Name() && pid != tm.core.PID() {
				hasLocal = true
				break
			}
		}

		if hasLocal {
			return nil
		}
	}

	connection, err := tm.core.GetConnection(event.Node)
	if err != nil {
		return nil
	}

	connection.DemonitorEvent(tm.core.PID(), event)

	return nil
}

func (tm *targetManager) EventInfo(event gen.Event) (gen.EventInfo, error) {
	tm.mutex.RLock()
	defer tm.mutex.RUnlock()

	entry, exists := tm.events[event]
	if exists == false {
		return gen.EventInfo{}, gen.ErrEventUnknown
	}

	// Build event info
	info := gen.EventInfo{
		Producer:      entry.producer,
		BufferSize:    entry.bufferSize,
		CurrentBuffer: len(entry.buffer),
		Notify:        entry.notify,
		Subscribers:   entry.subscriberCount,
	}

	return info, nil
}

// Helper: get event buffer
func (tm *targetManager) getEventBuffer(entry *eventEntry) []gen.MessageEvent {
	if entry.buffer == nil {
		return nil
	}

	// Return copy of buffer
	buffer := make([]gen.MessageEvent, len(entry.buffer))
	copy(buffer, entry.buffer)
	return buffer
}
