package tm

import "ergo.services/ergo/gen"

func (tm *targetManager) LinkPID(consumer gen.PID, target gen.PID) error {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	key := relationKey{
		consumer: consumer,
		target:   target,
	}

	// Check if already exists
	if _, exists := tm.linkRelations[key]; exists {
		// Special case: remote PID linking to local target (concurrent CorePID)
		if consumer.Node != tm.core.Name() {
			// Remote consumer - ignore duplicate (CorePID optimization)
			return nil
		}

		return gen.ErrTargetExist
	}

	// Add to linkRelations
	tm.linkRelations[key] = struct{}{}

	// Check targetIndex for remote request decision
	entry := tm.targetIndex[target]
	needsRemote := false

	if entry == nil {
		// First subscriber overall - create entry
		entry = &targetEntry{
			allowAlwaysFirst: true,
			consumers:        make(map[gen.PID]struct{}),
		}
		tm.targetIndex[target] = entry
		needsRemote = true
	}

	// Check if allowAlwaysFirst permits remote request
	if entry.allowAlwaysFirst == true {
		needsRemote = true
	}

	// Add consumer to entry
	entry.consumers[consumer] = struct{}{}

	// Check if target is local
	if target.Node == tm.core.Name() {
		// Local target - no remote request needed
		return nil
	}

	// Remote target
	if needsRemote == false {
		// Not allowed to send remote (someone already succeeded)
		return nil
	}

	// Send remote LinkPID with CorePID
	connection, err := tm.core.GetConnection(target.Node)
	if err != nil {
		// Network error - rollback
		delete(tm.linkRelations, key)
		delete(entry.consumers, consumer)

		if len(entry.consumers) == 0 {
			delete(tm.targetIndex, target)
		}

		return err
	}

	err = connection.LinkPID(tm.core.PID(), target)
	if err != nil {
		// Remote LinkPID failed - rollback
		delete(tm.linkRelations, key)
		delete(entry.consumers, consumer)

		if len(entry.consumers) == 0 {
			delete(tm.targetIndex, target)
		}

		return err
	}

	// Success! Set allowAlwaysFirst=false to prevent future remote requests
	entry.allowAlwaysFirst = false

	return nil
}

func (tm *targetManager) UnlinkPID(consumer gen.PID, target gen.PID) error {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	key := relationKey{
		consumer: consumer,
		target:   target,
	}

	// Check if exists
	if _, exists := tm.linkRelations[key]; exists == false {
		// Idempotent - not an error if already removed
		return nil
	}

	// Remove from linkRelations
	delete(tm.linkRelations, key)

	// Remove from targetIndex
	entry := tm.targetIndex[target]
	if entry == nil {
		return nil
	}

	delete(entry.consumers, consumer)

	// Check if last consumer
	isLast := (len(entry.consumers) == 0)

	if isLast {
		delete(tm.targetIndex, target)
	}

	// Check if target is local
	if target.Node == tm.core.Name() {
		// Local target - no remote request needed
		return nil
	}

	// Remote target - check if last LOCAL consumer
	if isLast == false {
		// Other consumers still exist - check if any are local
		hasLocal := false
		for pid := range entry.consumers {
			if pid.Node == tm.core.Name() && pid != tm.core.PID() {
				hasLocal = true
				break
			}
		}

		if hasLocal {
			// Other local consumers exist - don't send UnlinkPID
			return nil
		}
	}

	// Last local consumer (or isLast overall) - send remote UnlinkPID
	connection, err := tm.core.GetConnection(target.Node)
	if err != nil {
		// Connection lost - not an error, remote will cleanup via RouteNodeDown
		return nil
	}

	// Send UnlinkPID with CorePID (ignore errors - best effort)
	connection.UnlinkPID(tm.core.PID(), target)

	return nil
}

func (tm *targetManager) MonitorPID(consumer gen.PID, target gen.PID) error {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	key := relationKey{
		consumer: consumer,
		target:   target,
	}

	// Check if already exists
	if _, exists := tm.monitorRelations[key]; exists {
		// Special case: remote PID monitoring local target
		if consumer.Node != tm.core.Name() {
			// Remote consumer - ignore duplicate
			return nil
		}

		return gen.ErrTargetExist
	}

	// Add to monitorRelations
	tm.monitorRelations[key] = struct{}{}

	// Check targetIndex for remote request decision
	entry := tm.targetIndex[target]
	needsRemote := false

	if entry == nil {
		entry = &targetEntry{
			allowAlwaysFirst: true,
			consumers:        make(map[gen.PID]struct{}),
		}
		tm.targetIndex[target] = entry
		needsRemote = true
	}

	if entry.allowAlwaysFirst == true {
		needsRemote = true
	}

	entry.consumers[consumer] = struct{}{}

	// Check if target is local
	if target.Node == tm.core.Name() {
		return nil
	}

	// Remote target
	if needsRemote == false {
		return nil
	}

	// Send remote MonitorPID
	connection, err := tm.core.GetConnection(target.Node)
	if err != nil {
		// Rollback
		delete(tm.monitorRelations, key)
		delete(entry.consumers, consumer)

		if len(entry.consumers) == 0 {
			delete(tm.targetIndex, target)
		}

		return err
	}

	err = connection.MonitorPID(tm.core.PID(), target)
	if err != nil {
		// Rollback
		delete(tm.monitorRelations, key)
		delete(entry.consumers, consumer)

		if len(entry.consumers) == 0 {
			delete(tm.targetIndex, target)
		}

		return err
	}

	// Success
	entry.allowAlwaysFirst = false

	return nil
}

func (tm *targetManager) DemonitorPID(consumer gen.PID, target gen.PID) error {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	key := relationKey{
		consumer: consumer,
		target:   target,
	}

	if _, exists := tm.monitorRelations[key]; exists == false {
		return nil
	}

	delete(tm.monitorRelations, key)

	entry := tm.targetIndex[target]
	if entry == nil {
		return nil
	}

	delete(entry.consumers, consumer)

	isLast := (len(entry.consumers) == 0)

	if isLast {
		delete(tm.targetIndex, target)
	}

	if target.Node == tm.core.Name() {
		return nil
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

	connection, err := tm.core.GetConnection(target.Node)
	if err != nil {
		return nil
	}

	connection.DemonitorPID(tm.core.PID(), target)

	return nil
}
