package tm

import "ergo.services/ergo/gen"

func (tm *targetManager) TerminatedTargetPID(pid gen.PID, reason error) {
	s := tm.shardFor(pid)
	s.mutex.Lock()

	remoteNodesLinks := make(map[gen.Atom]bool)
	remoteNodesMonitors := make(map[gen.Atom]bool)
	var localExitConsumers []gen.PID
	var localDownConsumers []gen.PID

	for key := range s.linkRelations {
		if key.target != pid {
			continue
		}

		delete(s.linkRelations, key)

		if key.consumer.Node != tm.core.Name() {
			remoteNodesLinks[key.consumer.Node] = true
			continue
		}

		localExitConsumers = append(localExitConsumers, key.consumer)
	}

	for key := range s.monitorRelations {
		if key.target != pid {
			continue
		}

		delete(s.monitorRelations, key)

		if key.consumer.Node != tm.core.Name() {
			remoteNodesMonitors[key.consumer.Node] = true
			continue
		}

		localDownConsumers = append(localDownConsumers, key.consumer)
	}

	delete(s.targetIndex, pid)
	s.mutex.Unlock()

	if len(localExitConsumers) > 0 {
		tm.exitSignalsProduced.Add(1)
		tm.core.RouteSendExitMessages(pid, localExitConsumers, gen.MessageExitPID{PID: pid, Reason: reason})
		tm.exitSignalsDelivered.Add(int64(len(localExitConsumers)))
	}

	if len(localDownConsumers) > 0 {
		tm.downMessagesProduced.Add(1)
		for _, consumer := range localDownConsumers {
			tm.core.RouteSendPID(pid, consumer, gen.MessageOptions{Priority: gen.MessagePriorityHigh}, gen.MessageDownPID{PID: pid, Reason: reason})
		}
		tm.downMessagesDelivered.Add(int64(len(localDownConsumers)))
	}

	for node := range remoteNodesLinks {
		connection, err := tm.core.GetConnection(node)
		if err != nil {
			continue
		}
		connection.SendTerminatePID(pid, reason)
	}

	for node := range remoteNodesMonitors {
		connection, err := tm.core.GetConnection(node)
		if err != nil {
			continue
		}
		connection.SendTerminatePID(pid, reason)
	}
}

func (tm *targetManager) TerminatedTargetProcessID(processID gen.ProcessID, reason error) {
	s := tm.shardFor(processID)
	s.mutex.Lock()

	remoteNodes := make(map[gen.Atom]bool)
	var localExitConsumers []gen.PID
	var localDownConsumers []gen.PID

	for key := range s.linkRelations {
		if key.target != processID {
			continue
		}

		delete(s.linkRelations, key)

		if key.consumer.Node != tm.core.Name() {
			remoteNodes[key.consumer.Node] = true
			continue
		}

		localExitConsumers = append(localExitConsumers, key.consumer)
	}

	for key := range s.monitorRelations {
		if key.target != processID {
			continue
		}

		delete(s.monitorRelations, key)

		if key.consumer.Node != tm.core.Name() {
			remoteNodes[key.consumer.Node] = true
			continue
		}

		localDownConsumers = append(localDownConsumers, key.consumer)
	}

	delete(s.targetIndex, processID)
	s.mutex.Unlock()

	if len(localExitConsumers) > 0 {
		tm.exitSignalsProduced.Add(1)
		tm.core.RouteSendExitMessages(tm.core.PID(), localExitConsumers, gen.MessageExitProcessID{ProcessID: processID, Reason: reason})
		tm.exitSignalsDelivered.Add(int64(len(localExitConsumers)))
	}

	if len(localDownConsumers) > 0 {
		tm.downMessagesProduced.Add(1)
		for _, consumer := range localDownConsumers {
			tm.core.RouteSendPID(tm.core.PID(), consumer, gen.MessageOptions{Priority: gen.MessagePriorityHigh}, gen.MessageDownProcessID{ProcessID: processID, Reason: reason})
		}
		tm.downMessagesDelivered.Add(int64(len(localDownConsumers)))
	}

	for node := range remoteNodes {
		connection, err := tm.core.GetConnection(node)
		if err != nil {
			continue
		}
		connection.SendTerminateProcessID(processID, reason)
	}
}

func (tm *targetManager) TerminatedTargetAlias(alias gen.Alias, reason error) {
	s := tm.shardFor(alias)
	s.mutex.Lock()

	remoteNodes := make(map[gen.Atom]bool)
	var localExitConsumers []gen.PID
	var localDownConsumers []gen.PID

	for key := range s.linkRelations {
		if key.target != alias {
			continue
		}

		delete(s.linkRelations, key)

		if key.consumer.Node != tm.core.Name() {
			remoteNodes[key.consumer.Node] = true
			continue
		}

		localExitConsumers = append(localExitConsumers, key.consumer)
	}

	for key := range s.monitorRelations {
		if key.target != alias {
			continue
		}

		delete(s.monitorRelations, key)

		if key.consumer.Node != tm.core.Name() {
			remoteNodes[key.consumer.Node] = true
			continue
		}

		localDownConsumers = append(localDownConsumers, key.consumer)
	}

	delete(s.targetIndex, alias)
	s.mutex.Unlock()

	if len(localExitConsumers) > 0 {
		tm.exitSignalsProduced.Add(1)
		tm.core.RouteSendExitMessages(tm.core.PID(), localExitConsumers, gen.MessageExitAlias{Alias: alias, Reason: reason})
		tm.exitSignalsDelivered.Add(int64(len(localExitConsumers)))
	}

	if len(localDownConsumers) > 0 {
		tm.downMessagesProduced.Add(1)
		for _, consumer := range localDownConsumers {
			tm.core.RouteSendPID(tm.core.PID(), consumer, gen.MessageOptions{Priority: gen.MessagePriorityHigh}, gen.MessageDownAlias{Alias: alias, Reason: reason})
		}
		tm.downMessagesDelivered.Add(int64(len(localDownConsumers)))
	}

	for node := range remoteNodes {
		connection, err := tm.core.GetConnection(node)
		if err != nil {
			continue
		}
		connection.SendTerminateAlias(alias, reason)
	}
}

func (tm *targetManager) TerminatedTargetEvent(event gen.Event, reason error) {
	s := tm.shardFor(event)
	s.mutex.Lock()

	remoteNodes := make(map[gen.Atom]bool)
	var localExitConsumers []gen.PID
	var localDownConsumers []gen.PID

	for key := range s.linkRelations {
		if key.target != event {
			continue
		}

		delete(s.linkRelations, key)

		if key.consumer.Node != tm.core.Name() {
			remoteNodes[key.consumer.Node] = true
			continue
		}

		localExitConsumers = append(localExitConsumers, key.consumer)
	}

	for key := range s.monitorRelations {
		if key.target != event {
			continue
		}

		delete(s.monitorRelations, key)

		if key.consumer.Node != tm.core.Name() {
			remoteNodes[key.consumer.Node] = true
			continue
		}

		localDownConsumers = append(localDownConsumers, key.consumer)
	}

	if entry, exists := s.events[event]; exists {
		tm.eventIndex.Delete(entry.id)
	}
	delete(s.events, event)
	delete(s.targetIndex, event)
	s.mutex.Unlock()

	if len(localExitConsumers) > 0 {
		tm.exitSignalsProduced.Add(1)
		tm.core.RouteSendExitMessages(tm.core.PID(), localExitConsumers, gen.MessageExitEvent{Event: event, Reason: reason})
		tm.exitSignalsDelivered.Add(int64(len(localExitConsumers)))
	}

	if len(localDownConsumers) > 0 {
		tm.downMessagesProduced.Add(1)
		for _, consumer := range localDownConsumers {
			tm.core.RouteSendPID(tm.core.PID(), consumer, gen.MessageOptions{Priority: gen.MessagePriorityHigh}, gen.MessageDownEvent{Event: event, Reason: reason})
		}
		tm.downMessagesDelivered.Add(int64(len(localDownConsumers)))
	}

	for node := range remoteNodes {
		connection, err := tm.core.GetConnection(node)
		if err != nil {
			continue
		}
		connection.SendTerminateEvent(event, reason)
	}
}

func (tm *targetManager) TerminatedTargetNode(node gen.Atom, reason error) {
	for i := range tm.shards {
		tm.terminateNodeInShard(&tm.shards[i], node, reason)
	}
}

func (tm *targetManager) terminateNodeInShard(s *shard, node gen.Atom, reason error) {
	s.mutex.Lock()

	exitPID := make(map[gen.PID][]gen.PID)
	exitProcessID := make(map[gen.ProcessID][]gen.PID)
	exitAlias := make(map[gen.Alias][]gen.PID)
	exitEvent := make(map[gen.Event][]gen.PID)
	exitNode := make(map[gen.Atom][]gen.PID)

	for key := range s.linkRelations {
		shouldRemove := false

		if key.consumer.Node == node {
			shouldRemove = true
		}

		if shouldRemove == false {
			switch t := key.target.(type) {
			case gen.PID:
				if t.Node != node {
					break
				}
				shouldRemove = true
				if key.consumer.Node == tm.core.Name() {
					exitPID[t] = append(exitPID[t], key.consumer)
				}
			case gen.ProcessID:
				if t.Node != node {
					break
				}
				shouldRemove = true
				if key.consumer.Node == tm.core.Name() {
					exitProcessID[t] = append(exitProcessID[t], key.consumer)
				}
			case gen.Alias:
				if t.Node != node {
					break
				}
				shouldRemove = true
				if key.consumer.Node == tm.core.Name() {
					exitAlias[t] = append(exitAlias[t], key.consumer)
				}
			case gen.Event:
				if t.Node != node {
					break
				}
				shouldRemove = true
				if key.consumer.Node == tm.core.Name() {
					exitEvent[t] = append(exitEvent[t], key.consumer)
				}
			case gen.Atom:
				if t != node {
					break
				}
				shouldRemove = true
				if key.consumer.Node == tm.core.Name() {
					exitNode[t] = append(exitNode[t], key.consumer)
				}
			}
		}

		if shouldRemove == false {
			continue
		}

		delete(s.linkRelations, key)

		entry := s.targetIndex[key.target]
		if entry == nil {
			continue
		}

		delete(entry.consumers, key.consumer)
		if len(entry.consumers) == 0 {
			delete(s.targetIndex, key.target)
		}
	}

	downPID := make(map[gen.PID][]gen.PID)
	downProcessID := make(map[gen.ProcessID][]gen.PID)
	downAlias := make(map[gen.Alias][]gen.PID)
	downEvent := make(map[gen.Event][]gen.PID)
	downNode := make(map[gen.Atom][]gen.PID)

	for key := range s.monitorRelations {
		shouldRemove := false

		if key.consumer.Node == node {
			shouldRemove = true
		}

		if shouldRemove == false {
			switch t := key.target.(type) {
			case gen.PID:
				if t.Node != node {
					break
				}
				shouldRemove = true
				if key.consumer.Node == tm.core.Name() {
					downPID[t] = append(downPID[t], key.consumer)
				}
			case gen.ProcessID:
				if t.Node != node {
					break
				}
				shouldRemove = true
				if key.consumer.Node == tm.core.Name() {
					downProcessID[t] = append(downProcessID[t], key.consumer)
				}
			case gen.Alias:
				if t.Node != node {
					break
				}
				shouldRemove = true
				if key.consumer.Node == tm.core.Name() {
					downAlias[t] = append(downAlias[t], key.consumer)
				}
			case gen.Event:
				if t.Node != node {
					break
				}
				shouldRemove = true
				if key.consumer.Node == tm.core.Name() {
					downEvent[t] = append(downEvent[t], key.consumer)
				}
			case gen.Atom:
				if t != node {
					break
				}
				shouldRemove = true
				if key.consumer.Node == tm.core.Name() {
					downNode[t] = append(downNode[t], key.consumer)
				}
			}
		}

		if shouldRemove == false {
			continue
		}

		delete(s.monitorRelations, key)

		entry := s.targetIndex[key.target]
		if entry == nil {
			continue
		}

		delete(entry.consumers, key.consumer)
		if len(entry.consumers) == 0 {
			delete(s.targetIndex, key.target)
		}
	}

	// Cleanup events from terminated node
	for event, entry := range s.events {
		if event.Node == node {
			tm.eventIndex.Delete(entry.id)
			delete(s.events, event)
		}
	}

	s.mutex.Unlock()

	// Dispatch exit messages
	for target, consumers := range exitPID {
		tm.exitSignalsProduced.Add(1)
		tm.core.RouteSendExitMessages(tm.core.PID(), consumers, gen.MessageExitPID{PID: target, Reason: gen.ErrNoConnection})
		tm.exitSignalsDelivered.Add(int64(len(consumers)))
	}
	for target, consumers := range exitProcessID {
		tm.exitSignalsProduced.Add(1)
		tm.core.RouteSendExitMessages(tm.core.PID(), consumers, gen.MessageExitProcessID{ProcessID: target, Reason: gen.ErrNoConnection})
		tm.exitSignalsDelivered.Add(int64(len(consumers)))
	}
	for target, consumers := range exitAlias {
		tm.exitSignalsProduced.Add(1)
		tm.core.RouteSendExitMessages(tm.core.PID(), consumers, gen.MessageExitAlias{Alias: target, Reason: gen.ErrNoConnection})
		tm.exitSignalsDelivered.Add(int64(len(consumers)))
	}
	for target, consumers := range exitEvent {
		tm.exitSignalsProduced.Add(1)
		tm.core.RouteSendExitMessages(tm.core.PID(), consumers, gen.MessageExitEvent{Event: target, Reason: gen.ErrNoConnection})
		tm.exitSignalsDelivered.Add(int64(len(consumers)))
	}
	for target, consumers := range exitNode {
		tm.exitSignalsProduced.Add(1)
		tm.core.RouteSendExitMessages(tm.core.PID(), consumers, gen.MessageExitNode{Name: target})
		tm.exitSignalsDelivered.Add(int64(len(consumers)))
	}

	// Dispatch down messages
	for target, consumers := range downPID {
		tm.downMessagesProduced.Add(1)
		for _, consumer := range consumers {
			tm.core.RouteSendPID(tm.core.PID(), consumer, gen.MessageOptions{Priority: gen.MessagePriorityHigh}, gen.MessageDownPID{PID: target, Reason: gen.ErrNoConnection})
		}
		tm.downMessagesDelivered.Add(int64(len(consumers)))
	}
	for target, consumers := range downProcessID {
		tm.downMessagesProduced.Add(1)
		for _, consumer := range consumers {
			tm.core.RouteSendPID(tm.core.PID(), consumer, gen.MessageOptions{Priority: gen.MessagePriorityHigh}, gen.MessageDownProcessID{ProcessID: target, Reason: gen.ErrNoConnection})
		}
		tm.downMessagesDelivered.Add(int64(len(consumers)))
	}
	for target, consumers := range downAlias {
		tm.downMessagesProduced.Add(1)
		for _, consumer := range consumers {
			tm.core.RouteSendPID(tm.core.PID(), consumer, gen.MessageOptions{Priority: gen.MessagePriorityHigh}, gen.MessageDownAlias{Alias: target, Reason: gen.ErrNoConnection})
		}
		tm.downMessagesDelivered.Add(int64(len(consumers)))
	}
	for target, consumers := range downEvent {
		tm.downMessagesProduced.Add(1)
		for _, consumer := range consumers {
			tm.core.RouteSendPID(tm.core.PID(), consumer, gen.MessageOptions{Priority: gen.MessagePriorityHigh}, gen.MessageDownEvent{Event: target, Reason: gen.ErrNoConnection})
		}
		tm.downMessagesDelivered.Add(int64(len(consumers)))
	}
	for target, consumers := range downNode {
		tm.downMessagesProduced.Add(1)
		for _, consumer := range consumers {
			tm.core.RouteSendPID(tm.core.PID(), consumer, gen.MessageOptions{Priority: gen.MessagePriorityHigh}, gen.MessageDownNode{Name: target})
		}
		tm.downMessagesDelivered.Add(int64(len(consumers)))
	}
}

func (tm *targetManager) TerminatedProcess(pid gen.PID, reason error) {
	for i := range tm.shards {
		tm.terminateProcessInShard(&tm.shards[i], pid, reason)
	}
}

func (tm *targetManager) terminateProcessInShard(s *shard, pid gen.PID, reason error) {
	s.mutex.Lock()

	for key := range s.linkRelations {
		if key.consumer != pid {
			continue
		}

		delete(s.linkRelations, key)

		event, ok := key.target.(gen.Event)
		if ok == true && event.Node == tm.core.Name() {
			entry := s.events[event]
			if entry != nil {
				idx, exists := entry.linkSubscribersIndex[pid]
				if exists == true {
					last := len(entry.linkSubscribers) - 1
					if idx != last {
						entry.linkSubscribers[idx] = entry.linkSubscribers[last]
						entry.linkSubscribersIndex[entry.linkSubscribers[idx]] = idx
					}
					entry.linkSubscribers = entry.linkSubscribers[:last]
					delete(entry.linkSubscribersIndex, pid)
				}
				entry.subscriberCount--
				if entry.subscriberCount == 0 && entry.notify {
					tm.core.RouteSendPID(tm.core.PID(), entry.producer, gen.MessageOptions{Priority: gen.MessagePriorityHigh}, gen.MessageEventStop{Name: event.Name})
				}
			}
		}

		tm.cleanupTargetEntry(s, pid, key.target, true)
	}

	for key := range s.monitorRelations {
		if key.consumer != pid {
			continue
		}

		delete(s.monitorRelations, key)

		event, ok := key.target.(gen.Event)
		if ok == true && event.Node == tm.core.Name() {
			entry := s.events[event]
			if entry != nil {
				idx, exists := entry.monitorSubscribersIndex[pid]
				if exists == true {
					last := len(entry.monitorSubscribers) - 1
					if idx != last {
						entry.monitorSubscribers[idx] = entry.monitorSubscribers[last]
						entry.monitorSubscribersIndex[entry.monitorSubscribers[idx]] = idx
					}
					entry.monitorSubscribers = entry.monitorSubscribers[:last]
					delete(entry.monitorSubscribersIndex, pid)
				}
				entry.subscriberCount--
				if entry.subscriberCount == 0 && entry.notify {
					tm.core.RouteSendPID(tm.core.PID(), entry.producer, gen.MessageOptions{Priority: gen.MessagePriorityHigh}, gen.MessageEventStop{Name: event.Name})
				}
			}
		}

		tm.cleanupTargetEntry(s, pid, key.target, false)
	}

	// Producer cleanup
	tm.cleanupProducerInShard(s, pid, reason)

	s.mutex.Unlock()
}

// cleanupTargetEntry handles targetIndex update and remote unlink/demonitor.
func (tm *targetManager) cleanupTargetEntry(s *shard, consumer gen.PID, target any, isLink bool) {
	entry := s.targetIndex[target]
	if entry == nil {
		return
	}

	delete(entry.consumers, consumer)
	isLast := (len(entry.consumers) == 0)
	if isLast {
		delete(s.targetIndex, target)
	}

	isRemote := false
	var targetNode gen.Atom

	switch t := target.(type) {
	case gen.PID:
		targetNode = t.Node
		isRemote = (t.Node != tm.core.Name())
	case gen.ProcessID:
		if t.Node == "" {
			targetNode = tm.core.Name()
		} else {
			targetNode = t.Node
		}
		isRemote = (targetNode != tm.core.Name())
	case gen.Alias:
		targetNode = t.Node
		isRemote = (t.Node != tm.core.Name())
	case gen.Event:
		targetNode = t.Node
		isRemote = (t.Node != tm.core.Name())
	case gen.Atom:
		isRemote = false
	}

	if isRemote == false {
		return
	}

	if isLast == false {
		hasLocal := false
		for p := range entry.consumers {
			if p.Node == tm.core.Name() && p != tm.core.PID() {
				hasLocal = true
				break
			}
		}
		if hasLocal {
			return
		}
	}

	connection, err := tm.core.GetConnection(targetNode)
	if err != nil {
		return
	}

	if isLink {
		switch t := target.(type) {
		case gen.PID:
			connection.UnlinkPID(tm.core.PID(), t)
		case gen.ProcessID:
			connection.UnlinkProcessID(tm.core.PID(), t)
		case gen.Alias:
			connection.UnlinkAlias(tm.core.PID(), t)
		case gen.Event:
			connection.UnlinkEvent(tm.core.PID(), t)
		}
	} else {
		switch t := target.(type) {
		case gen.PID:
			connection.DemonitorPID(tm.core.PID(), t)
		case gen.ProcessID:
			connection.DemonitorProcessID(tm.core.PID(), t)
		case gen.Alias:
			connection.DemonitorAlias(tm.core.PID(), t)
		case gen.Event:
			connection.DemonitorEvent(tm.core.PID(), t)
		}
	}
}

// cleanupProducerInShard handles events owned by terminated process within a shard.
func (tm *targetManager) cleanupProducerInShard(s *shard, pid gen.PID, reason error) {
	events := s.producerEvents[pid]
	if events == nil {
		return
	}

	remoteEvents := make(map[gen.Atom][]gen.Event)

	for event := range events {
		entry := s.events[event]
		if entry == nil {
			continue
		}

		// Send exit to link subscribers
		var localExitConsumers []gen.PID
		for _, consumer := range entry.linkSubscribers {
			if consumer.Node != tm.core.Name() {
				remoteEvents[consumer.Node] = append(remoteEvents[consumer.Node], event)
				continue
			}
			localExitConsumers = append(localExitConsumers, consumer)
		}
		if len(localExitConsumers) > 0 {
			tm.exitSignalsProduced.Add(1)
			tm.core.RouteSendExitMessages(tm.core.PID(), localExitConsumers, gen.MessageExitEvent{Event: event, Reason: reason})
			tm.exitSignalsDelivered.Add(int64(len(localExitConsumers)))
		}

		// Send down to monitor subscribers
		var localDownConsumers []gen.PID
		for _, consumer := range entry.monitorSubscribers {
			if consumer.Node != tm.core.Name() {
				remoteEvents[consumer.Node] = append(remoteEvents[consumer.Node], event)
				continue
			}
			localDownConsumers = append(localDownConsumers, consumer)
		}
		if len(localDownConsumers) > 0 {
			tm.downMessagesProduced.Add(1)
			for _, consumer := range localDownConsumers {
				tm.core.RouteSendPID(tm.core.PID(), consumer, gen.MessageOptions{Priority: gen.MessagePriorityHigh}, gen.MessageDownEvent{Event: event, Reason: reason})
			}
			tm.downMessagesDelivered.Add(int64(len(localDownConsumers)))
		}

		// Cleanup relations for this event
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
		delete(s.targetIndex, event)
		delete(s.events, event)
	}

	// Send to remote nodes
	for remoteNode, nodeEvents := range remoteEvents {
		connection, err := tm.core.GetConnection(remoteNode)
		if err != nil {
			continue
		}
		for _, event := range nodeEvents {
			connection.SendTerminateEvent(event, reason)
		}
	}

	delete(s.producerEvents, pid)
}
