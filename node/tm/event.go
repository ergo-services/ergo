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
		producer:           producer,
		token:              token,
		notify:             options.Notify,
		linkSubscribers:    make(map[gen.PID]struct{}),
		monitorSubscribers: make(map[gen.PID]struct{}),
		subscriberCount:    0,
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
	dispatcherIdx := 0
	remoteNodes := make(map[gen.Atom]bool)

	for consumer := range entry.linkSubscribers {
		if consumer.Node == tm.core.Name() {
			// LOCAL consumer
			tm.dispatchers[dispatcherIdx].push(&dispatchTask{
				from:    tm.core.PID(),
				to:      consumer,
				options: gen.MessageOptions{Priority: gen.MessagePriorityHigh},
				message: gen.MessageExitEvent{Event: event, Reason: gen.ErrUnregistered},
			})
			dispatcherIdx = (dispatcherIdx + 1) % len(tm.dispatchers)
		} else {
			// REMOTE consumer - collect node for network send
			remoteNodes[consumer.Node] = true
		}
	}

	for consumer := range entry.monitorSubscribers {
		if consumer.Node == tm.core.Name() {
			// LOCAL consumer
			tm.dispatchers[dispatcherIdx].push(&dispatchTask{
				from:    tm.core.PID(),
				to:      consumer,
				options: gen.MessageOptions{Priority: gen.MessagePriorityHigh},
				message: gen.MessageDownEvent{Event: event, Reason: gen.ErrUnregistered},
			})
			dispatcherIdx = (dispatcherIdx + 1) % len(tm.dispatchers)
		} else {
			// REMOTE consumer - collect node for network send
			remoteNodes[consumer.Node] = true
		}
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

func (tm *targetManager) PublishEvent(from gen.PID, token gen.Ref, options gen.MessageOptions, message gen.MessageEvent) error {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	// Check if this is a LOCAL producer or REMOTE event
	if from.Node == tm.core.Name() {
		// LOCAL producer - validate token and deliver to all subscribers
		return tm.publishEventLocalProducer(from, token, options, message)
	}

	// REMOTE event - deliver to local subscribers only
	return tm.publishEventRemoteProducer(from, options, message)
}

func (tm *targetManager) publishEventLocalProducer(from gen.PID, token gen.Ref, options gen.MessageOptions, message gen.MessageEvent) error {
	entry, exists := tm.events[message.Event]
	if exists == false {
		return gen.ErrEventUnknown
	}

	// Validate token
	if entry.token != token {
		return gen.ErrEventOwner
	}

	// Store in buffer (protected by mutex)
	if entry.buffer != nil {
		if len(entry.buffer) < entry.bufferSize {
			// Buffer not full
			entry.buffer = append(entry.buffer, message)
		} else {
			// Buffer full - shift and append (flush oldest)
			copy(entry.buffer, entry.buffer[1:])
			entry.buffer[entry.bufferSize-1] = message
		}
	}

	// Increment published counter
	tm.eventsPublished.Add(1)

	// Collect local tasks for batch dispatch
	var localTasks []*dispatchTask
	remoteNodes := make(map[gen.Atom]bool)
	dispatcherIdx := 0

	// Fanout to link subscribers
	for consumer := range entry.linkSubscribers {
		if consumer.Node != tm.core.Name() {
			// REMOTE consumer - dispatch immediately per connection
			if remoteNodes[consumer.Node] {
				continue
			}
			remoteNodes[consumer.Node] = true
			tm.dispatchers[dispatcherIdx].pushRemoteEvent(&dispatchRemoteEvent{
				node:    consumer.Node,
				from:    from,
				options: options,
				message: message,
			})
			dispatcherIdx = (dispatcherIdx + 1) % len(tm.dispatchers)
			continue
		}

		// LOCAL consumer
		localTasks = append(localTasks, &dispatchTask{
			from:    from,
			to:      consumer,
			options: options,
			message: message,
		})
	}

	// Fanout to monitor subscribers
	for consumer := range entry.monitorSubscribers {
		if consumer.Node != tm.core.Name() {
			// REMOTE consumer - dispatch immediately per connection
			if remoteNodes[consumer.Node] {
				continue
			}
			remoteNodes[consumer.Node] = true
			tm.dispatchers[dispatcherIdx].pushRemoteEvent(&dispatchRemoteEvent{
				node:    consumer.Node,
				from:    from,
				options: options,
				message: message,
			})
			dispatcherIdx = (dispatcherIdx + 1) % len(tm.dispatchers)
			continue
		}

		// LOCAL consumer
		localTasks = append(localTasks, &dispatchTask{
			from:    from,
			to:      consumer,
			options: options,
			message: message,
		})
	}

	// Batch dispatch local tasks
	if len(localTasks) > 0 {
		tm.dispatchers[0].pushBatch(&dispatchBatch{tasks: localTasks})
	}

	return nil
}

func (tm *targetManager) publishEventRemoteProducer(from gen.PID, options gen.MessageOptions, message gen.MessageEvent) error {
	// Remote event arrived - deliver to local subscribers only
	entry := tm.targetIndex[message.Event]
	if entry == nil {
		return nil
	}

	// Increment published counter
	tm.eventsPublished.Add(1)

	// Collect local tasks for batch dispatch
	var localTasks []*dispatchTask

	for consumer := range entry.consumers {
		if consumer.Node == tm.core.Name() {
			localTasks = append(localTasks, &dispatchTask{
				from:    from,
				to:      consumer,
				options: options,
				message: message,
			})
		}
		// Remote consumer here would be a bug - skip silently
	}

	if len(localTasks) > 0 {
		tm.dispatchers[0].pushBatch(&dispatchBatch{tasks: localTasks})
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
	entry.linkSubscribers[consumer] = struct{}{}

	// Increment counter
	entry.subscriberCount++

	// Check if need to send EventStart
	if entry.subscriberCount == 1 && entry.notify {
		// First subscriber - send EventStart via dispatcher!
		tm.dispatchers[0].push(&dispatchTask{
			from:    tm.core.PID(),
			to:      entry.producer,
			options: gen.MessageOptions{Priority: gen.MessagePriorityHigh},
			message: gen.MessageEventStart{Name: event.Name},
		})
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
	delete(entry.linkSubscribers, consumer)

	// Decrement counter
	entry.subscriberCount--

	// Send EventStop if last subscriber
	if entry.subscriberCount == 0 && entry.notify {
		tm.dispatchers[0].push(&dispatchTask{
			from:    tm.core.PID(),
			to:      entry.producer,
			options: gen.MessageOptions{Priority: gen.MessagePriorityHigh},
			message: gen.MessageEventStop{Name: event.Name},
		})
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
	entry.monitorSubscribers[consumer] = struct{}{}

	// Increment counter (shared with links!)
	entry.subscriberCount++

	// Send EventStart if first subscriber overall
	if entry.subscriberCount == 1 && entry.notify {
		tm.dispatchers[0].push(&dispatchTask{
			from:    tm.core.PID(),
			to:      entry.producer,
			options: gen.MessageOptions{Priority: gen.MessagePriorityHigh},
			message: gen.MessageEventStart{Name: event.Name},
		})
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
	delete(entry.monitorSubscribers, consumer)

	// Decrement counter (shared with links!)
	entry.subscriberCount--

	// Send EventStop if last
	if entry.subscriberCount == 0 && entry.notify {
		tm.dispatchers[0].push(&dispatchTask{
			from:    tm.core.PID(),
			to:      entry.producer,
			options: gen.MessageOptions{Priority: gen.MessagePriorityHigh},
			message: gen.MessageEventStop{Name: event.Name},
		})
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
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

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
