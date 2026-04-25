package tm

import (
	"time"

	"ergo.services/ergo/gen"
)

func (tm *targetManager) RegisterEvent(producer gen.PID, name gen.Atom, options gen.EventOptions) (gen.Ref, error) {
	event := gen.Event{Node: tm.core.Name(), Name: name}
	s := tm.shardFor(event)
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if _, exists := s.events[event]; exists {
		return gen.Ref{}, gen.ErrTaken
	}

	token := tm.generateToken()

	// Node-level events (producer is corePID) do not consume MessageEventStart
	// or MessageEventStop, so Notify is silently ignored for them.
	notify := options.Notify
	if producer == tm.core.PID() {
		notify = false
	}

	id := tm.eventSeq.Add(1)
	entry := &eventEntry{
		id:                      id,
		createdAt:               time.Now().UnixNano(),
		event:                   event,
		producer:                producer,
		token:                   token,
		notify:                  notify,
		open:                    options.Open,
		linkSubscribersIndex:    make(map[gen.PID]int),
		monitorSubscribersIndex: make(map[gen.PID]int),
		subscriberCount:         0,
	}

	if options.Buffer > 0 {
		entry.buffer = &eventRingBuffer{
			data: make([]gen.MessageEvent, options.Buffer),
			size: options.Buffer,
		}
	}

	s.events[event] = entry

	if s.producerEvents[producer] == nil {
		s.producerEvents[producer] = make(map[gen.Event]struct{})
	}
	s.producerEvents[producer][event] = struct{}{}

	tm.eventIndex.Store(id, entry)

	return token, nil
}

func (tm *targetManager) UnregisterEvent(producer gen.PID, name gen.Atom) error {
	event := gen.Event{Node: tm.core.Name(), Name: name}
	s := tm.shardFor(event)
	s.mutex.Lock()

	entry, exists := s.events[event]
	if exists == false {
		s.mutex.Unlock()
		return gen.ErrEventUnknown
	}

	if entry.producer != producer {
		s.mutex.Unlock()
		return gen.ErrEventOwner
	}

	remoteNodes := make(map[gen.Atom]bool)
	var localExitConsumers []gen.PID
	var localDownConsumers []gen.PID

	for _, consumer := range entry.linkSubscribers {
		if consumer.Node != tm.core.Name() {
			remoteNodes[consumer.Node] = true
			continue
		}
		localExitConsumers = append(localExitConsumers, consumer)
	}

	for _, consumer := range entry.monitorSubscribers {
		if consumer.Node != tm.core.Name() {
			remoteNodes[consumer.Node] = true
			continue
		}
		localDownConsumers = append(localDownConsumers, consumer)
	}

	// Cleanup relations
	for key := range s.linkRelations {
		if key.target == event {
			delete(s.linkRelations, key)
		}
	}

	for key := range s.monitorRelations {
		if key.target == event {
			delete(s.monitorRelations, key)
		}
	}

	tm.eventIndex.Delete(entry.id)
	delete(s.events, event)
	delete(s.targetIndex, event)

	events := s.producerEvents[producer]
	if events != nil {
		delete(events, event)
		if len(events) == 0 {
			delete(s.producerEvents, producer)
		}
	}

	s.mutex.Unlock()

	// Dispatch without lock
	if len(localExitConsumers) > 0 {
		tm.core.RouteSendExitMessages(
			tm.core.PID(),
			localExitConsumers,
			gen.MessageExitEvent{Event: event, Reason: gen.ErrUnregistered},
		)
	}

	for _, consumer := range localDownConsumers {
		tm.core.RouteSendPID(
			tm.core.PID(),
			consumer,
			gen.MessageOptions{Priority: gen.MessagePriorityHigh},
			gen.MessageDownEvent{Event: event, Reason: gen.ErrUnregistered},
		)
	}

	for node := range remoteNodes {
		connection, err := tm.core.GetConnection(node)
		if err != nil {
			continue
		}
		connection.SendTerminateEvent(event, gen.ErrUnregistered)
	}

	return nil
}

func (tm *targetManager) PublishEvent(
	from gen.PID,
	token gen.Ref,
	options gen.MessageOptions,
	message gen.MessageEvent,
) error {
	if from.Node == tm.core.Name() {
		return tm.publishEventLocalProducer(from, token, options, message)
	}

	s := tm.shardFor(message.Event)
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return tm.publishEventRemoteProducer(s, from, options, message)
}

func (tm *targetManager) publishEventLocalProducer(
	from gen.PID,
	token gen.Ref,
	options gen.MessageOptions,
	message gen.MessageEvent,
) error {
	s := tm.shardFor(message.Event)
	s.mutex.RLock()

	entry, exists := s.events[message.Event]
	if exists == false {
		s.mutex.RUnlock()
		return gen.ErrEventUnknown
	}

	if entry.open == false && entry.token != token {
		s.mutex.RUnlock()
		return gen.ErrEventOwner
	}

	if entry.buffer != nil {
		entry.bufferMutex.Lock()
		entry.buffer.push(message)
		entry.bufferMutex.Unlock()
	}

	entry.messagesPublished.Add(1)

	// Snapshot slices under RLock (safe: slices only modified under write lock)
	linkSubs := entry.linkSubscribers
	monitorSubs := entry.monitorSubscribers
	s.mutex.RUnlock()

	tm.eventsPublished.Add(1)

	var localConsumers []gen.PID
	remoteNodes := make(map[gen.Atom]bool)

	for _, consumer := range linkSubs {
		if consumer.Node != tm.core.Name() {
			remoteNodes[consumer.Node] = true
			continue
		}
		localConsumers = append(localConsumers, consumer)
	}

	for _, consumer := range monitorSubs {
		if consumer.Node != tm.core.Name() {
			remoteNodes[consumer.Node] = true
			continue
		}
		localConsumers = append(localConsumers, consumer)
	}

	if len(localConsumers) > 0 {
		tm.core.RouteSendEventMessages(from, localConsumers, options, message)
		n := int64(len(localConsumers))
		entry.messagesLocalSent.Add(n)
		tm.eventsLocalSent.Add(n)
	}

	for node := range remoteNodes {
		connection, err := tm.core.GetConnection(node)
		if err != nil {
			continue
		}
		connection.SendEvent(from, options, message)
		entry.messagesRemoteSent.Add(1)
		tm.eventsRemoteSent.Add(1)
	}

	return nil
}

func (tm *targetManager) publishEventRemoteProducer(
	s *shard,
	from gen.PID,
	options gen.MessageOptions,
	message gen.MessageEvent,
) error {
	entry := s.targetIndex[message.Event]
	if entry == nil {
		return nil
	}

	tm.eventsReceived.Add(1)

	var localConsumers []gen.PID
	for consumer := range entry.consumers {
		if consumer.Node != tm.core.Name() {
			continue
		}
		localConsumers = append(localConsumers, consumer)
	}

	if len(localConsumers) > 0 {
		tm.core.RouteSendEventMessages(from, localConsumers, options, message)
		tm.eventsLocalSent.Add(int64(len(localConsumers)))
	}

	return nil
}

func (tm *targetManager) LinkEvent(consumer gen.PID, event gen.Event) ([]gen.MessageEvent, error) {
	s := tm.shardFor(event)
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if event.Node == tm.core.Name() {
		return tm.linkEventLocal(s, consumer, event)
	}

	return tm.linkEventRemote(s, consumer, event)
}

func (tm *targetManager) linkEventLocal(s *shard, consumer gen.PID, event gen.Event) ([]gen.MessageEvent, error) {
	entry, exists := s.events[event]
	if exists == false {
		return nil, gen.ErrEventUnknown
	}

	key := relationKey{
		consumer: consumer,
		target:   event,
	}

	_, dup := s.linkRelations[key]
	if dup == true && consumer.Node != tm.core.Name() {
		return getEventBuffer(entry), nil
	}
	if dup == true {
		return nil, gen.ErrTargetExist
	}

	s.linkRelations[key] = struct{}{}
	entry.linkSubscribersIndex[consumer] = len(entry.linkSubscribers)
	entry.linkSubscribers = append(entry.linkSubscribers, consumer)

	entry.subscriberCount++

	if entry.subscriberCount == 1 && entry.notify {
		tm.core.RouteSendPID(
			tm.core.PID(),
			entry.producer,
			gen.MessageOptions{Priority: gen.MessagePriorityHigh},
			gen.MessageEventStart{Name: event.Name},
		)
	}

	return getEventBuffer(entry), nil
}

func (tm *targetManager) linkEventRemote(s *shard, consumer gen.PID, event gen.Event) ([]gen.MessageEvent, error) {
	key := relationKey{
		consumer: consumer,
		target:   event,
	}

	if _, exists := s.linkRelations[key]; exists {
		return nil, gen.ErrTargetExist
	}

	s.linkRelations[key] = struct{}{}

	entry := s.targetIndex[event]
	needsRemote := false

	if entry == nil {
		entry = &targetEntry{
			allowAlwaysFirst: true,
			consumers:        make(map[gen.PID]struct{}),
		}
		s.targetIndex[event] = entry
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
		delete(s.linkRelations, key)
		delete(entry.consumers, consumer)
		if len(entry.consumers) == 0 {
			delete(s.targetIndex, event)
		}
		return nil, err
	}

	buffer, err := connection.LinkEvent(tm.core.PID(), event)
	if err != nil {
		delete(s.linkRelations, key)
		delete(entry.consumers, consumer)
		if len(entry.consumers) == 0 {
			delete(s.targetIndex, event)
		}
		return nil, err
	}

	if buffer == nil {
		entry.allowAlwaysFirst = false
	}

	return buffer, nil
}

func (tm *targetManager) UnlinkEvent(consumer gen.PID, event gen.Event) error {
	s := tm.shardFor(event)
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if event.Node == tm.core.Name() {
		return tm.unlinkEventLocal(s, consumer, event)
	}

	return tm.unlinkEventRemote(s, consumer, event)
}

func (tm *targetManager) unlinkEventLocal(s *shard, consumer gen.PID, event gen.Event) error {
	entry, exists := s.events[event]
	if exists == false {
		return gen.ErrEventUnknown
	}

	key := relationKey{
		consumer: consumer,
		target:   event,
	}

	if _, exists := s.linkRelations[key]; exists == false {
		return gen.ErrTargetUnknown
	}

	delete(s.linkRelations, key)

	// Swap-delete from slice
	idx := entry.linkSubscribersIndex[consumer]
	last := len(entry.linkSubscribers) - 1
	if idx != last {
		entry.linkSubscribers[idx] = entry.linkSubscribers[last]
		entry.linkSubscribersIndex[entry.linkSubscribers[idx]] = idx
	}
	entry.linkSubscribers = entry.linkSubscribers[:last]
	delete(entry.linkSubscribersIndex, consumer)

	entry.subscriberCount--

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

func (tm *targetManager) unlinkEventRemote(s *shard, consumer gen.PID, event gen.Event) error {
	key := relationKey{
		consumer: consumer,
		target:   event,
	}

	if _, exists := s.linkRelations[key]; exists == false {
		return gen.ErrTargetUnknown
	}

	delete(s.linkRelations, key)

	entry := s.targetIndex[event]
	if entry == nil {
		return nil
	}

	delete(entry.consumers, consumer)

	isLast := (len(entry.consumers) == 0)
	if isLast {
		delete(s.targetIndex, event)
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

	connection.UnlinkEvent(tm.core.PID(), event)
	return nil
}

func (tm *targetManager) MonitorEvent(consumer gen.PID, event gen.Event) ([]gen.MessageEvent, error) {
	s := tm.shardFor(event)
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if event.Node == tm.core.Name() {
		return tm.monitorEventLocal(s, consumer, event)
	}

	return tm.monitorEventRemote(s, consumer, event)
}

func (tm *targetManager) monitorEventLocal(s *shard, consumer gen.PID, event gen.Event) ([]gen.MessageEvent, error) {
	entry, exists := s.events[event]
	if exists == false {
		return nil, gen.ErrEventUnknown
	}

	key := relationKey{
		consumer: consumer,
		target:   event,
	}

	_, dup := s.monitorRelations[key]
	if dup == true && consumer.Node != tm.core.Name() {
		return getEventBuffer(entry), nil
	}
	if dup == true {
		return nil, gen.ErrTargetExist
	}

	s.monitorRelations[key] = struct{}{}
	entry.monitorSubscribersIndex[consumer] = len(entry.monitorSubscribers)
	entry.monitorSubscribers = append(entry.monitorSubscribers, consumer)

	entry.subscriberCount++

	if entry.subscriberCount == 1 && entry.notify {
		tm.core.RouteSendPID(
			tm.core.PID(),
			entry.producer,
			gen.MessageOptions{Priority: gen.MessagePriorityHigh},
			gen.MessageEventStart{Name: event.Name},
		)
	}

	return getEventBuffer(entry), nil
}

func (tm *targetManager) monitorEventRemote(s *shard, consumer gen.PID, event gen.Event) ([]gen.MessageEvent, error) {
	key := relationKey{
		consumer: consumer,
		target:   event,
	}

	if _, exists := s.monitorRelations[key]; exists {
		return nil, gen.ErrTargetExist
	}

	s.monitorRelations[key] = struct{}{}

	entry := s.targetIndex[event]
	needsRemote := false

	if entry == nil {
		entry = &targetEntry{
			allowAlwaysFirst: true,
			consumers:        make(map[gen.PID]struct{}),
		}
		s.targetIndex[event] = entry
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
		delete(s.monitorRelations, key)
		delete(entry.consumers, consumer)
		if len(entry.consumers) == 0 {
			delete(s.targetIndex, event)
		}
		return nil, err
	}

	buffer, err := connection.MonitorEvent(tm.core.PID(), event)
	if err != nil {
		delete(s.monitorRelations, key)
		delete(entry.consumers, consumer)
		if len(entry.consumers) == 0 {
			delete(s.targetIndex, event)
		}
		return nil, err
	}

	if buffer == nil {
		entry.allowAlwaysFirst = false
	}

	return buffer, nil
}

func (tm *targetManager) DemonitorEvent(consumer gen.PID, event gen.Event) error {
	s := tm.shardFor(event)
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if event.Node == tm.core.Name() {
		return tm.demonitorEventLocal(s, consumer, event)
	}

	return tm.demonitorEventRemote(s, consumer, event)
}

func (tm *targetManager) demonitorEventLocal(s *shard, consumer gen.PID, event gen.Event) error {
	entry, exists := s.events[event]
	if exists == false {
		return gen.ErrEventUnknown
	}

	key := relationKey{
		consumer: consumer,
		target:   event,
	}

	if _, exists := s.monitorRelations[key]; exists == false {
		return gen.ErrTargetUnknown
	}

	delete(s.monitorRelations, key)

	// Swap-delete from slice
	idx := entry.monitorSubscribersIndex[consumer]
	last := len(entry.monitorSubscribers) - 1
	if idx != last {
		entry.monitorSubscribers[idx] = entry.monitorSubscribers[last]
		entry.monitorSubscribersIndex[entry.monitorSubscribers[idx]] = idx
	}
	entry.monitorSubscribers = entry.monitorSubscribers[:last]
	delete(entry.monitorSubscribersIndex, consumer)

	entry.subscriberCount--

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

func (tm *targetManager) demonitorEventRemote(s *shard, consumer gen.PID, event gen.Event) error {
	key := relationKey{
		consumer: consumer,
		target:   event,
	}

	if _, exists := s.monitorRelations[key]; exists == false {
		return gen.ErrTargetUnknown
	}

	delete(s.monitorRelations, key)

	entry := s.targetIndex[event]
	if entry == nil {
		return nil
	}

	delete(entry.consumers, consumer)

	isLast := (len(entry.consumers) == 0)
	if isLast {
		delete(s.targetIndex, event)
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
	s := tm.shardFor(event)
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	entry, exists := s.events[event]
	if exists == false {
		return gen.EventInfo{}, gen.ErrEventUnknown
	}

	var bufSize, bufLen int
	if entry.buffer != nil {
		bufSize = entry.buffer.size
		bufLen = entry.buffer.len
	}

	return gen.EventInfo{
		CreatedAt:          entry.createdAt,
		Event:              event,
		Producer:           entry.producer,
		BufferSize:         bufSize,
		CurrentBuffer:      bufLen,
		Notify:             entry.notify,
		Open:               entry.open,
		Subscribers:        entry.subscriberCount,
		MessagesPublished:  entry.messagesPublished.Load(),
		MessagesLocalSent:  entry.messagesLocalSent.Load(),
		MessagesRemoteSent: entry.messagesRemoteSent.Load(),
	}, nil
}

func (tm *targetManager) EventRangeInfo(fn func(gen.EventInfo) bool) error {
	var infos []gen.EventInfo

	for i := range tm.shards {
		s := &tm.shards[i]
		s.mutex.RLock()
		for event, entry := range s.events {
			var bufSize, bufLen int
			if entry.buffer != nil {
				bufSize = entry.buffer.size
				bufLen = entry.buffer.len
			}
			infos = append(infos, gen.EventInfo{
				CreatedAt:          entry.createdAt,
				Event:              event,
				Producer:           entry.producer,
				BufferSize:         bufSize,
				CurrentBuffer:      bufLen,
				Notify:             entry.notify,
				Open:               entry.open,
				Subscribers:        entry.subscriberCount,
				MessagesPublished:  entry.messagesPublished.Load(),
				MessagesLocalSent:  entry.messagesLocalSent.Load(),
				MessagesRemoteSent: entry.messagesRemoteSent.Load(),
			})
		}
		s.mutex.RUnlock()
	}

	for _, info := range infos {
		if fn(info) == false {
			break
		}
	}

	return nil
}

func (tm *targetManager) EventListInfo(timestamp int64, limit int, filter ...func(gen.EventInfo) bool) ([]gen.EventInfo, error) {
	maxID := tm.eventSeq.Load()
	if maxID == 0 {
		return nil, nil
	}

	absLimit := limit
	if absLimit < 0 {
		absLimit = -absLimit
	}
	if absLimit == 0 {
		return nil, nil
	}

	var fn func(gen.EventInfo) bool
	if len(filter) > 0 {
		fn = filter[0]
	}

	result := make([]gen.EventInfo, 0, absLimit)

	buildInfo := func(entry *eventEntry) gen.EventInfo {
		var bufSize, bufLen int
		if entry.buffer != nil {
			bufSize = entry.buffer.size
			bufLen = entry.buffer.len
		}
		return gen.EventInfo{
			CreatedAt:          entry.createdAt,
			Event:              entry.event,
			Producer:           entry.producer,
			BufferSize:         bufSize,
			CurrentBuffer:      bufLen,
			Notify:             entry.notify,
			Open:               entry.open,
			Subscribers:        entry.subscriberCount,
			MessagesPublished:  entry.messagesPublished.Load(),
			MessagesLocalSent:  entry.messagesLocalSent.Load(),
			MessagesRemoteSent: entry.messagesRemoteSent.Load(),
		}
	}

	accept := func(entry *eventEntry) bool {
		if timestamp > 0 {
			if limit >= 0 && entry.createdAt < timestamp {
				return false
			}
			if limit < 0 && entry.createdAt > timestamp {
				return false
			}
		}
		if fn != nil {
			return fn(buildInfo(entry))
		}
		return true
	}

	if timestamp == -1 || limit < 0 {
		// backward: from newest
		for id := maxID; id >= 1 && len(result) < absLimit; id-- {
			v, ok := tm.eventIndex.Load(id)
			if ok == false {
				continue
			}
			entry := v.(*eventEntry)
			if accept(entry) {
				result = append(result, buildInfo(entry))
			}
		}
	} else {
		// forward: from oldest
		for id := uint64(1); id <= maxID && len(result) < absLimit; id++ {
			v, ok := tm.eventIndex.Load(id)
			if ok == false {
				continue
			}
			entry := v.(*eventEntry)
			if accept(entry) {
				result = append(result, buildInfo(entry))
			}
		}
	}

	return result, nil
}
