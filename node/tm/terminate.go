package tm

import "ergo.services/ergo/gen"

func (tm *targetManager) TerminatedTargetPID(pid gen.PID, reason error) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	dispatcherIdx := 0
	remoteNodesLinks := make(map[gen.Atom]bool)
	remoteNodesMonitors := make(map[gen.Atom]bool)

	// Process link consumers
	for key := range tm.linkRelations {
		if key.target != pid {
			continue
		}

		delete(tm.linkRelations, key)

		if key.consumer.Node != tm.core.Name() {
			// REMOTE consumer
			remoteNodesLinks[key.consumer.Node] = true
			continue
		}

		// LOCAL consumer
		tm.exitSignalsProduced.Add(1)
		tm.dispatchers[dispatcherIdx].push(&dispatchTask{
			from:    pid,
			to:      key.consumer,
			options: gen.MessageOptions{Priority: gen.MessagePriorityHigh},
			message: gen.MessageExitPID{PID: pid, Reason: reason},
		})
		dispatcherIdx = (dispatcherIdx + 1) % len(tm.dispatchers)
	}

	// Process monitor consumers
	for key := range tm.monitorRelations {
		if key.target != pid {
			continue
		}

		delete(tm.monitorRelations, key)

		if key.consumer.Node != tm.core.Name() {
			// REMOTE consumer
			remoteNodesMonitors[key.consumer.Node] = true
			continue
		}

		// LOCAL consumer
		tm.downMessagesProduced.Add(1)
		tm.dispatchers[dispatcherIdx].push(&dispatchTask{
			from:    pid,
			to:      key.consumer,
			options: gen.MessageOptions{Priority: gen.MessagePriorityHigh},
			message: gen.MessageDownPID{PID: pid, Reason: reason},
		})
		dispatcherIdx = (dispatcherIdx + 1) % len(tm.dispatchers)
	}

	// Send to remote nodes (they will fanout locally)
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

	dispatcherIdx := 0
	remoteNodes := make(map[gen.Atom]bool)

	// Link consumers
	for key := range tm.linkRelations {
		if key.target != processID {
			continue
		}

		delete(tm.linkRelations, key)

		if key.consumer.Node != tm.core.Name() {
			// REMOTE
			remoteNodes[key.consumer.Node] = true
			continue
		}

		// LOCAL
		tm.exitSignalsProduced.Add(1)
		tm.dispatchers[dispatcherIdx].push(&dispatchTask{
			from:    tm.core.PID(),
			to:      key.consumer,
			options: gen.MessageOptions{Priority: gen.MessagePriorityHigh},
			message: gen.MessageExitProcessID{ProcessID: processID, Reason: reason},
		})
		dispatcherIdx = (dispatcherIdx + 1) % len(tm.dispatchers)
	}

	// Monitor consumers
	for key := range tm.monitorRelations {
		if key.target != processID {
			continue
		}

		delete(tm.monitorRelations, key)

		if key.consumer.Node != tm.core.Name() {
			// REMOTE
			remoteNodes[key.consumer.Node] = true
			continue
		}

		// LOCAL
		tm.downMessagesProduced.Add(1)
		tm.dispatchers[dispatcherIdx].push(&dispatchTask{
			from:    tm.core.PID(),
			to:      key.consumer,
			options: gen.MessageOptions{Priority: gen.MessagePriorityHigh},
			message: gen.MessageDownProcessID{ProcessID: processID, Reason: reason},
		})
		dispatcherIdx = (dispatcherIdx + 1) % len(tm.dispatchers)
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

	dispatcherIdx := 0
	remoteNodes := make(map[gen.Atom]bool)

	// Link consumers
	for key := range tm.linkRelations {
		if key.target != alias {
			continue
		}

		delete(tm.linkRelations, key)

		if key.consumer.Node != tm.core.Name() {
			// REMOTE
			remoteNodes[key.consumer.Node] = true
			continue
		}

		// LOCAL
		tm.exitSignalsProduced.Add(1)
		tm.dispatchers[dispatcherIdx].push(&dispatchTask{
			from:    tm.core.PID(),
			to:      key.consumer,
			options: gen.MessageOptions{Priority: gen.MessagePriorityHigh},
			message: gen.MessageExitAlias{Alias: alias, Reason: reason},
		})
		dispatcherIdx = (dispatcherIdx + 1) % len(tm.dispatchers)
	}

	// Monitor consumers
	for key := range tm.monitorRelations {
		if key.target != alias {
			continue
		}

		delete(tm.monitorRelations, key)

		if key.consumer.Node != tm.core.Name() {
			// REMOTE
			remoteNodes[key.consumer.Node] = true
			continue
		}

		// LOCAL
		tm.downMessagesProduced.Add(1)
		tm.dispatchers[dispatcherIdx].push(&dispatchTask{
			from:    tm.core.PID(),
			to:      key.consumer,
			options: gen.MessageOptions{Priority: gen.MessagePriorityHigh},
			message: gen.MessageDownAlias{Alias: alias, Reason: reason},
		})
		dispatcherIdx = (dispatcherIdx + 1) % len(tm.dispatchers)
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

	dispatcherIdx := 0
	remoteNodes := make(map[gen.Atom]bool)

	// Link consumers
	for key := range tm.linkRelations {
		if key.target != event {
			continue
		}

		delete(tm.linkRelations, key)

		if key.consumer.Node != tm.core.Name() {
			// REMOTE
			remoteNodes[key.consumer.Node] = true
			continue
		}

		// LOCAL
		tm.exitSignalsProduced.Add(1)
		tm.dispatchers[dispatcherIdx].push(&dispatchTask{
			from:    tm.core.PID(),
			to:      key.consumer,
			options: gen.MessageOptions{Priority: gen.MessagePriorityHigh},
			message: gen.MessageExitEvent{Event: event, Reason: reason},
		})
		dispatcherIdx = (dispatcherIdx + 1) % len(tm.dispatchers)
	}

	// Monitor consumers
	for key := range tm.monitorRelations {
		if key.target != event {
			continue
		}

		delete(tm.monitorRelations, key)

		if key.consumer.Node != tm.core.Name() {
			// REMOTE
			remoteNodes[key.consumer.Node] = true
			continue
		}

		// LOCAL
		tm.downMessagesProduced.Add(1)
		tm.dispatchers[dispatcherIdx].push(&dispatchTask{
			from:    tm.core.PID(),
			to:      key.consumer,
			options: gen.MessageOptions{Priority: gen.MessagePriorityHigh},
			message: gen.MessageDownEvent{Event: event, Reason: reason},
		})
		dispatcherIdx = (dispatcherIdx + 1) % len(tm.dispatchers)
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

	dispatcherIdx := 0

	// Cleanup linkRelations
	for key := range tm.linkRelations {
		shouldRemove := false

		// Consumer on terminated node
		if key.consumer.Node == node {
			shouldRemove = true
			// Don't send exit - consumer is dead
		}

		// Target on terminated node
		if shouldRemove == false {
			switch t := key.target.(type) {
			case gen.PID:
				if t.Node == node {
					shouldRemove = true

					// Send exit to LOCAL consumers
					if key.consumer.Node == tm.core.Name() {
						tm.dispatchers[dispatcherIdx].push(&dispatchTask{
							from:    tm.core.PID(),
							to:      key.consumer,
							options: gen.MessageOptions{Priority: gen.MessagePriorityHigh},
							message: gen.MessageExitPID{PID: t, Reason: gen.ErrNoConnection},
						})

						dispatcherIdx = (dispatcherIdx + 1) % len(tm.dispatchers)
					}
				}

			case gen.ProcessID:
				if t.Node == node {
					shouldRemove = true

					if key.consumer.Node == tm.core.Name() {
						tm.dispatchers[dispatcherIdx].push(&dispatchTask{
							from:    tm.core.PID(),
							to:      key.consumer,
							options: gen.MessageOptions{Priority: gen.MessagePriorityHigh},
							message: gen.MessageExitProcessID{ProcessID: t, Reason: gen.ErrNoConnection},
						})

						dispatcherIdx = (dispatcherIdx + 1) % len(tm.dispatchers)
					}
				}

			case gen.Alias:
				if t.Node == node {
					shouldRemove = true

					if key.consumer.Node == tm.core.Name() {
						tm.dispatchers[dispatcherIdx].push(&dispatchTask{
							from:    tm.core.PID(),
							to:      key.consumer,
							options: gen.MessageOptions{Priority: gen.MessagePriorityHigh},
							message: gen.MessageExitAlias{Alias: t, Reason: gen.ErrNoConnection},
						})

						dispatcherIdx = (dispatcherIdx + 1) % len(tm.dispatchers)
					}
				}

			case gen.Event:
				if t.Node == node {
					shouldRemove = true

					if key.consumer.Node == tm.core.Name() {
						tm.dispatchers[dispatcherIdx].push(&dispatchTask{
							from:    tm.core.PID(),
							to:      key.consumer,
							options: gen.MessageOptions{Priority: gen.MessagePriorityHigh},
							message: gen.MessageExitEvent{Event: t, Reason: gen.ErrNoConnection},
						})

						dispatcherIdx = (dispatcherIdx + 1) % len(tm.dispatchers)
					}
				}

			case gen.Atom:
				if t == node {
					shouldRemove = true

					if key.consumer.Node == tm.core.Name() {
						tm.dispatchers[dispatcherIdx].push(&dispatchTask{
							from:    tm.core.PID(),
							to:      key.consumer,
							options: gen.MessageOptions{Priority: gen.MessagePriorityHigh},
							message: gen.MessageExitNode{Name: t},
						})

						dispatcherIdx = (dispatcherIdx + 1) % len(tm.dispatchers)
					}
				}
			}
		}

		if shouldRemove == false {
			continue
		}

		delete(tm.linkRelations, key)

		// Remove from targetIndex
		entry := tm.targetIndex[key.target]
		if entry == nil {
			continue
		}

		delete(entry.consumers, key.consumer)

		if len(entry.consumers) == 0 {
			delete(tm.targetIndex, key.target)
		}
	}

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
						tm.dispatchers[dispatcherIdx].push(&dispatchTask{
							from:    tm.core.PID(),
							to:      key.consumer,
							options: gen.MessageOptions{Priority: gen.MessagePriorityHigh},
							message: gen.MessageDownPID{PID: t, Reason: gen.ErrNoConnection},
						})

						dispatcherIdx = (dispatcherIdx + 1) % len(tm.dispatchers)
					}
				}

			case gen.ProcessID:
				if t.Node == node {
					shouldRemove = true

					if key.consumer.Node == tm.core.Name() {
						tm.dispatchers[dispatcherIdx].push(&dispatchTask{
							from:    tm.core.PID(),
							to:      key.consumer,
							options: gen.MessageOptions{Priority: gen.MessagePriorityHigh},
							message: gen.MessageDownProcessID{ProcessID: t, Reason: gen.ErrNoConnection},
						})

						dispatcherIdx = (dispatcherIdx + 1) % len(tm.dispatchers)
					}
				}

			case gen.Alias:
				if t.Node == node {
					shouldRemove = true

					if key.consumer.Node == tm.core.Name() {
						tm.dispatchers[dispatcherIdx].push(&dispatchTask{
							from:    tm.core.PID(),
							to:      key.consumer,
							options: gen.MessageOptions{Priority: gen.MessagePriorityHigh},
							message: gen.MessageDownAlias{Alias: t, Reason: gen.ErrNoConnection},
						})

						dispatcherIdx = (dispatcherIdx + 1) % len(tm.dispatchers)
					}
				}

			case gen.Event:
				if t.Node == node {
					shouldRemove = true

					if key.consumer.Node == tm.core.Name() {
						tm.dispatchers[dispatcherIdx].push(&dispatchTask{
							from:    tm.core.PID(),
							to:      key.consumer,
							options: gen.MessageOptions{Priority: gen.MessagePriorityHigh},
							message: gen.MessageDownEvent{Event: t, Reason: gen.ErrNoConnection},
						})

						dispatcherIdx = (dispatcherIdx + 1) % len(tm.dispatchers)
					}
				}

			case gen.Atom:
				if t == node {
					shouldRemove = true

					if key.consumer.Node == tm.core.Name() {
						tm.dispatchers[dispatcherIdx].push(&dispatchTask{
							from:    tm.core.PID(),
							to:      key.consumer,
							options: gen.MessageOptions{Priority: gen.MessagePriorityHigh},
							message: gen.MessageDownNode{Name: t},
						})

						dispatcherIdx = (dispatcherIdx + 1) % len(tm.dispatchers)
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

		// Remove from linkRelations
		delete(tm.linkRelations, key)

		// Handle events separately (need to decrement counter)
		if event, ok := key.target.(gen.Event); ok {
			if event.Node == tm.core.Name() {
				// LOCAL event - decrement counter
				entry := tm.events[event]
				if entry != nil {
					delete(entry.linkSubscribers, pid)
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
				}
			}
		}

		// Remove from targetIndex
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
			// Node links are always local
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

	// Process monitorRelations (identical logic)
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
					delete(entry.monitorSubscribers, pid)
					entry.subscriberCount--

					if entry.subscriberCount == 0 && entry.notify {
						tm.dispatchers[0].push(&dispatchTask{
							from:    tm.core.PID(),
							to:      entry.producer,
							options: gen.MessageOptions{Priority: gen.MessagePriorityHigh},
							message: gen.MessageEventStop{Name: event.Name},
						})
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
	// Use producerEvents index for O(1) lookup instead of O(n) iteration
	if events := tm.producerEvents[pid]; events != nil {
		// Collect all local tasks and remote events per node
		var localTasks []*dispatchTask
		remoteEvents := make(map[gen.Atom][]gen.Event) // node -> events

		for event := range events {
			entry := tm.events[event]
			if entry == nil {
				continue
			}

			// Collect link subscribers
			for consumer := range entry.linkSubscribers {
				if consumer.Node != tm.core.Name() {
					remoteEvents[consumer.Node] = append(remoteEvents[consumer.Node], event)
					continue
				}
				localTasks = append(localTasks, &dispatchTask{
					from:    tm.core.PID(),
					to:      consumer,
					options: gen.MessageOptions{Priority: gen.MessagePriorityHigh},
					message: gen.MessageExitEvent{Event: event, Reason: reason},
				})
			}

			// Collect monitor subscribers
			for consumer := range entry.monitorSubscribers {
				if consumer.Node != tm.core.Name() {
					remoteEvents[consumer.Node] = append(remoteEvents[consumer.Node], event)
					continue
				}
				localTasks = append(localTasks, &dispatchTask{
					from:    tm.core.PID(),
					to:      consumer,
					options: gen.MessageOptions{Priority: gen.MessagePriorityHigh},
					message: gen.MessageDownEvent{Event: event, Reason: reason},
				})
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

			// Cleanup targetIndex and event
			delete(tm.targetIndex, event)
			delete(tm.events, event)
		}

		// Send local batch
		if len(localTasks) > 0 {
			tm.dispatchers[0].pushBatch(&dispatchBatch{tasks: localTasks})
		}

		// Send to remote nodes
		for node, nodeEvents := range remoteEvents {
			connection, err := tm.core.GetConnection(node)
			if err != nil {
				continue
			}
			// Send termination for each event to this node
			for _, event := range nodeEvents {
				connection.SendTerminateEvent(event, reason)
			}
		}

		// Cleanup producerEvents index
		delete(tm.producerEvents, pid)
	}
}
