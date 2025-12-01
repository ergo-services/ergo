package tm

import "ergo.services/ergo/gen"

// Node operations (always local - connection monitoring)

func (tm *targetManager) LinkNode(consumer gen.PID, target gen.Atom) error {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	key := relationKey{
		consumer: consumer,
		target:   target,
	}

	if _, exists := tm.linkRelations[key]; exists {
		return gen.ErrTargetExist
	}

	// Add to linkRelations
	tm.linkRelations[key] = struct{}{}

	// Add to targetIndex
	entry := tm.targetIndex[target]
	if entry == nil {
		entry = &targetEntry{
			consumers: make(map[gen.PID]struct{}),
		}
		tm.targetIndex[target] = entry
	}
	entry.consumers[consumer] = struct{}{}

	return nil
}

func (tm *targetManager) UnlinkNode(consumer gen.PID, target gen.Atom) error {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	key := relationKey{
		consumer: consumer,
		target:   target,
	}

	if _, exists := tm.linkRelations[key]; exists == false {
		return nil
	}

	delete(tm.linkRelations, key)

	entry := tm.targetIndex[target]
	if entry != nil {
		delete(entry.consumers, consumer)

		if len(entry.consumers) == 0 {
			delete(tm.targetIndex, target)
		}
	}

	return nil
}

func (tm *targetManager) MonitorNode(consumer gen.PID, target gen.Atom) error {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	key := relationKey{
		consumer: consumer,
		target:   target,
	}

	if _, exists := tm.monitorRelations[key]; exists {
		return gen.ErrTargetExist
	}

	// Add to monitorRelations (always local - no remote for Node targets)
	tm.monitorRelations[key] = struct{}{}

	// Add to targetIndex
	entry := tm.targetIndex[target]
	if entry == nil {
		entry = &targetEntry{
			consumers: make(map[gen.PID]struct{}),
		}
		tm.targetIndex[target] = entry
	}
	entry.consumers[consumer] = struct{}{}

	return nil
}

func (tm *targetManager) DemonitorNode(consumer gen.PID, target gen.Atom) error {
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
	if entry != nil {
		delete(entry.consumers, consumer)

		if len(entry.consumers) == 0 {
			delete(tm.targetIndex, target)
		}
	}

	return nil
}
