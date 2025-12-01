package tm

import "ergo.services/ergo/gen"

func (tm *targetManager) TerminatedTargetPID(pid gen.PID, reason error) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	remoteNodesLinks := make(map[gen.Atom]bool)
	remoteNodesMonitors := make(map[gen.Atom]bool)
	var localExitConsumers []gen.PID
	var localDownConsumers []gen.PID

	// Process link consumers
	for key := range tm.linkRelations {
		if key.target != pid {
			continue
		}

		delete(tm.linkRelations, key)

		if key.consumer.Node != tm.core.Name() {
			remoteNodesLinks[key.consumer.Node] = true
			continue
		}

		localExitConsumers = append(localExitConsumers, key.consumer)
	}

	// Process monitor consumers
	for key := range tm.monitorRelations {
		if key.target != pid {
			continue
		}

		delete(tm.monitorRelations, key)

		if key.consumer.Node != tm.core.Name() {
			remoteNodesMonitors[key.consumer.Node] = true
			continue
		}

		localDownConsumers = append(localDownConsumers, key.consumer)
	}

	// Send exit messages to local consumers
	if len(localExitConsumers) > 0 {
		tm.exitSignalsProduced.Add(1)
		tm.core.RouteSendExitMessages(pid, localExitConsumers, gen.MessageExitPID{PID: pid, Reason: reason})
		tm.exitSignalsDelivered.Add(int64(len(localExitConsumers)))
	}

	// Send down messages to local consumers
	if len(localDownConsumers) > 0 {
		tm.downMessagesProduced.Add(1)
		for _, consumer := range localDownConsumers {
			tm.core.RouteSendPID(pid, consumer, gen.MessageOptions{Priority: gen.MessagePriorityHigh}, gen.MessageDownPID{PID: pid, Reason: reason})
		}
		tm.downMessagesDelivered.Add(int64(len(localDownConsumers)))
	}

	// Send to remote nodes
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

	delete(tm.targetIndex, pid)
}

func (tm *targetManager) TerminatedTargetProcessID(processID gen.ProcessID, reason error) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	remoteNodes := make(map[gen.Atom]bool)
	var localExitConsumers []gen.PID
	var localDownConsumers []gen.PID

	// Link consumers
	for key := range tm.linkRelations {
		if key.target != processID {
			continue
		}

		delete(tm.linkRelations, key)

		if key.consumer.Node != tm.core.Name() {
			remoteNodes[key.consumer.Node] = true
			continue
		}

		localExitConsumers = append(localExitConsumers, key.consumer)
	}

	// Monitor consumers
	for key := range tm.monitorRelations {
		if key.target != processID {
			continue
		}

		delete(tm.monitorRelations, key)

		if key.consumer.Node != tm.core.Name() {
			remoteNodes[key.consumer.Node] = true
			continue
		}

		localDownConsumers = append(localDownConsumers, key.consumer)
	}

	// Send exit messages
	if len(localExitConsumers) > 0 {
		tm.exitSignalsProduced.Add(1)
		tm.core.RouteSendExitMessages(tm.core.PID(), localExitConsumers, gen.MessageExitProcessID{ProcessID: processID, Reason: reason})
		tm.exitSignalsDelivered.Add(int64(len(localExitConsumers)))
	}

	// Send down messages
	if len(localDownConsumers) > 0 {
		tm.downMessagesProduced.Add(1)
		for _, consumer := range localDownConsumers {
			tm.core.RouteSendPID(tm.core.PID(), consumer, gen.MessageOptions{Priority: gen.MessagePriorityHigh}, gen.MessageDownProcessID{ProcessID: processID, Reason: reason})
		}
		tm.downMessagesDelivered.Add(int64(len(localDownConsumers)))
	}

	// Send to remote nodes
	for node := range remoteNodes {
		connection, err := tm.core.GetConnection(node)
		if err != nil {
			continue
		}
		connection.SendTerminateProcessID(processID, reason)
	}

	delete(tm.targetIndex, processID)
}

func (tm *targetManager) TerminatedTargetAlias(alias gen.Alias, reason error) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	remoteNodes := make(map[gen.Atom]bool)
	var localExitConsumers []gen.PID
	var localDownConsumers []gen.PID

	// Link consumers
	for key := range tm.linkRelations {
		if key.target != alias {
			continue
		}

		delete(tm.linkRelations, key)

		if key.consumer.Node != tm.core.Name() {
			remoteNodes[key.consumer.Node] = true
			continue
		}

		localExitConsumers = append(localExitConsumers, key.consumer)
	}

	// Monitor consumers
	for key := range tm.monitorRelations {
		if key.target != alias {
			continue
		}

		delete(tm.monitorRelations, key)

		if key.consumer.Node != tm.core.Name() {
			remoteNodes[key.consumer.Node] = true
			continue
		}

		localDownConsumers = append(localDownConsumers, key.consumer)
	}

	// Send exit messages
	if len(localExitConsumers) > 0 {
		tm.exitSignalsProduced.Add(1)
		tm.core.RouteSendExitMessages(tm.core.PID(), localExitConsumers, gen.MessageExitAlias{Alias: alias, Reason: reason})
		tm.exitSignalsDelivered.Add(int64(len(localExitConsumers)))
	}

	// Send down messages
	if len(localDownConsumers) > 0 {
		tm.downMessagesProduced.Add(1)
		for _, consumer := range localDownConsumers {
			tm.core.RouteSendPID(tm.core.PID(), consumer, gen.MessageOptions{Priority: gen.MessagePriorityHigh}, gen.MessageDownAlias{Alias: alias, Reason: reason})
		}
		tm.downMessagesDelivered.Add(int64(len(localDownConsumers)))
	}

	// Send to remote nodes
	for node := range remoteNodes {
		connection, err := tm.core.GetConnection(node)
		if err != nil {
			continue
		}
		connection.SendTerminateAlias(alias, reason)
	}

	delete(tm.targetIndex, alias)
}

func (tm *targetManager) TerminatedTargetEvent(event gen.Event, reason error) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	remoteNodes := make(map[gen.Atom]bool)
	var localExitConsumers []gen.PID
	var localDownConsumers []gen.PID

	// Link consumers
	for key := range tm.linkRelations {
		if key.target != event {
			continue
		}

		delete(tm.linkRelations, key)

		if key.consumer.Node != tm.core.Name() {
			remoteNodes[key.consumer.Node] = true
			continue
		}

		localExitConsumers = append(localExitConsumers, key.consumer)
	}

	// Monitor consumers
	for key := range tm.monitorRelations {
		if key.target != event {
			continue
		}

		delete(tm.monitorRelations, key)

		if key.consumer.Node != tm.core.Name() {
			remoteNodes[key.consumer.Node] = true
			continue
		}

		localDownConsumers = append(localDownConsumers, key.consumer)
	}

	// Send exit messages
	if len(localExitConsumers) > 0 {
		tm.exitSignalsProduced.Add(1)
		tm.core.RouteSendExitMessages(tm.core.PID(), localExitConsumers, gen.MessageExitEvent{Event: event, Reason: reason})
		tm.exitSignalsDelivered.Add(int64(len(localExitConsumers)))
	}

	// Send down messages
	if len(localDownConsumers) > 0 {
		tm.downMessagesProduced.Add(1)
		for _, consumer := range localDownConsumers {
			tm.core.RouteSendPID(tm.core.PID(), consumer, gen.MessageOptions{Priority: gen.MessagePriorityHigh}, gen.MessageDownEvent{Event: event, Reason: reason})
		}
		tm.downMessagesDelivered.Add(int64(len(localDownConsumers)))
	}

	// Send to remote nodes
	for node := range remoteNodes {
		connection, err := tm.core.GetConnection(node)
		if err != nil {
			continue
		}
		connection.SendTerminateEvent(event, reason)
	}

	// Cleanup event from events map
	delete(tm.events, event)
	delete(tm.targetIndex, event)
}

func (tm *targetManager) TerminatedTargetNode(node gen.Atom, reason error) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	// Collect exit messages by type
	exitPID := make(map[gen.PID][]gen.PID)           // target -> consumers
	exitProcessID := make(map[gen.ProcessID][]gen.PID)
	exitAlias := make(map[gen.Alias][]gen.PID)
	exitEvent := make(map[gen.Event][]gen.PID)
	exitNode := make(map[gen.Atom][]gen.PID)

	// Cleanup linkRelations
	for key := range tm.linkRelations {
		shouldRemove := false

		// Consumer on terminated node
		if key.consumer.Node == node {
			shouldRemove = true
		}

		// Target on terminated node
		if shouldRemove == false {
			switch t := key.target.(type) {
			case gen.PID:
				if t.Node == node {
					shouldRemove = true
					if key.consumer.Node == tm.core.Name() {
						exitPID[t] = append(exitPID[t], key.consumer)
					}
				}

			case gen.ProcessID:
				if t.Node == node {
					shouldRemove = true
					if key.consumer.Node == tm.core.Name() {
						exitProcessID[t] = append(exitProcessID[t], key.consumer)
					}
				}

			case gen.Alias:
				if t.Node == node {
					shouldRemove = true
					if key.consumer.Node == tm.core.Name() {
						exitAlias[t] = append(exitAlias[t], key.consumer)
					}
				}

			case gen.Event:
				if t.Node == node {
					shouldRemove = true
					if key.consumer.Node == tm.core.Name() {
						exitEvent[t] = append(exitEvent[t], key.consumer)
					}
				}

			case gen.Atom:
				if t == node {
					shouldRemove = true
					if key.consumer.Node == tm.core.Name() {
						exitNode[t] = append(exitNode[t], key.consumer)
					}
				}
			}
		}

		if shouldRemove == false {
			continue
		}

		delete(tm.linkRelations, key)

		entry := tm.targetIndex[key.target]
		if entry == nil {
			continue
		}

		delete(entry.consumers, key.consumer)

		if len(entry.consumers) == 0 {
			delete(tm.targetIndex, key.target)
		}
	}

	// Collect down messages by type
	downPID := make(map[gen.PID][]gen.PID)
	downProcessID := make(map[gen.ProcessID][]gen.PID)
	downAlias := make(map[gen.Alias][]gen.PID)
	downEvent := make(map[gen.Event][]gen.PID)
	downNode := make(map[gen.Atom][]gen.PID)

	// Cleanup monitorRelations
	for key := range tm.monitorRelations {
		shouldRemove := false

		if key.consumer.Node == node {
			shouldRemove = true
		}

		if shouldRemove == false {
			switch t := key.target.(type) {
			case gen.PID:
				if t.Node == node {
					shouldRemove = true
					if key.consumer.Node == tm.core.Name() {
						downPID[t] = append(downPID[t], key.consumer)
					}
				}

			case gen.ProcessID:
				if t.Node == node {
					shouldRemove = true
					if key.consumer.Node == tm.core.Name() {
						downProcessID[t] = append(downProcessID[t], key.consumer)
					}
				}

			case gen.Alias:
				if t.Node == node {
					shouldRemove = true
					if key.consumer.Node == tm.core.Name() {
						downAlias[t] = append(downAlias[t], key.consumer)
					}
				}

			case gen.Event:
				if t.Node == node {
					shouldRemove = true
					if key.consumer.Node == tm.core.Name() {
						downEvent[t] = append(downEvent[t], key.consumer)
					}
				}

			case gen.Atom:
				if t == node {
					shouldRemove = true
					if key.consumer.Node == tm.core.Name() {
						downNode[t] = append(downNode[t], key.consumer)
					}
				}
			}
		}

		if shouldRemove == false {
			continue
		}

		delete(tm.monitorRelations, key)

		entry := tm.targetIndex[key.target]
		if entry == nil {
			continue
		}

		delete(entry.consumers, key.consumer)

		if len(entry.consumers) == 0 {
			delete(tm.targetIndex, key.target)
		}
	}

	// Send exit messages (batch per target)
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

	// Send down messages
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

	// Cleanup events from terminated node
	for event := range tm.events {
		if event.Node == node {
			delete(tm.events, event)
		}
	}
}

func (tm *targetManager) TerminatedProcess(pid gen.PID, reason error) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	// CleanupConsumer - cleanup all subscriptions this process had

	// Process linkRelations
	for key := range tm.linkRelations {
		if key.consumer != pid {
			continue
		}

		delete(tm.linkRelations, key)

		// Handle events separately (need to decrement counter)
		if event, ok := key.target.(gen.Event); ok {
			if event.Node == tm.core.Name() {
				entry := tm.events[event]
				if entry != nil {
					// Swap-delete from linkSubscribers
					if idx, exists := entry.linkSubscribersIndex[pid]; exists {
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
		}

		entry := tm.targetIndex[key.target]
		if entry == nil {
			continue
		}

		delete(entry.consumers, key.consumer)

		isLast := (len(entry.consumers) == 0)

		if isLast {
			delete(tm.targetIndex, key.target)
		}

		// Check if target is remote and need to send Unlink
		isRemote := false
		var targetNode gen.Atom

		switch t := key.target.(type) {
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
			continue
		}

		// Remote target - check if last local consumer
		if isLast == false {
			hasLocal := false
			for p := range entry.consumers {
				if p.Node == tm.core.Name() && p != tm.core.PID() {
					hasLocal = true
					break
				}
			}

			if hasLocal {
				continue
			}
		}

		// Last local consumer - send remote Unlink
		connection, err := tm.core.GetConnection(targetNode)
		if err != nil {
			continue
		}

		switch t := key.target.(type) {
		case gen.PID:
			connection.UnlinkPID(tm.core.PID(), t)

		case gen.ProcessID:
			connection.UnlinkProcessID(tm.core.PID(), t)

		case gen.Alias:
			connection.UnlinkAlias(tm.core.PID(), t)

		case gen.Event:
			connection.UnlinkEvent(tm.core.PID(), t)
		}
	}

	// Process monitorRelations
	for key := range tm.monitorRelations {
		if key.consumer != pid {
			continue
		}

		delete(tm.monitorRelations, key)

		// Handle events
		if event, ok := key.target.(gen.Event); ok {
			if event.Node == tm.core.Name() {
				entry := tm.events[event]
				if entry != nil {
					// Swap-delete from monitorSubscribers
					if idx, exists := entry.monitorSubscribersIndex[pid]; exists {
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
		}

		entry := tm.targetIndex[key.target]
		if entry == nil {
			continue
		}

		delete(entry.consumers, key.consumer)

		isLast := (len(entry.consumers) == 0)

		if isLast {
			delete(tm.targetIndex, key.target)
		}

		// Check remote and send Demonitor
		isRemote := false
		var targetNode gen.Atom

		switch t := key.target.(type) {
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
			continue
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
				continue
			}
		}

		// Last local consumer - send remote Demonitor
		connection, err := tm.core.GetConnection(targetNode)
		if err != nil {
			continue
		}

		switch t := key.target.(type) {
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

	// Cleanup events owned by terminated process (PRODUCER cleanup)
	if events := tm.producerEvents[pid]; events != nil {
		remoteEvents := make(map[gen.Atom][]gen.Event)

		for event := range events {
			entry := tm.events[event]
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

			delete(tm.targetIndex, event)
			delete(tm.events, event)
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

		delete(tm.producerEvents, pid)
	}
}
