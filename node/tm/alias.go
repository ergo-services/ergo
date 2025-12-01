package tm

import "ergo.services/ergo/gen"

func (tm *targetManager) LinkAlias(consumer gen.PID, target gen.Alias) error {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	key := relationKey{
		consumer: consumer,
		target:   target,
	}

	if _, exists := tm.linkRelations[key]; exists {
		if consumer.Node != tm.core.Name() {
			return nil
		}

		return gen.ErrTargetExist
	}

	tm.linkRelations[key] = struct{}{}

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

	if target.Node == tm.core.Name() {
		return nil
	}

	if needsRemote == false {
		return nil
	}

	connection, err := tm.core.GetConnection(target.Node)
	if err != nil {
		delete(tm.linkRelations, key)
		delete(entry.consumers, consumer)

		if len(entry.consumers) == 0 {
			delete(tm.targetIndex, target)
		}

		return err
	}

	err = connection.LinkAlias(tm.core.PID(), target)
	if err != nil {
		delete(tm.linkRelations, key)
		delete(entry.consumers, consumer)

		if len(entry.consumers) == 0 {
			delete(tm.targetIndex, target)
		}

		return err
	}

	entry.allowAlwaysFirst = false

	return nil
}

func (tm *targetManager) UnlinkAlias(consumer gen.PID, target gen.Alias) error {
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

	connection.UnlinkAlias(tm.core.PID(), target)

	return nil
}

func (tm *targetManager) MonitorAlias(consumer gen.PID, target gen.Alias) error {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	key := relationKey{
		consumer: consumer,
		target:   target,
	}

	if _, exists := tm.monitorRelations[key]; exists {
		if consumer.Node != tm.core.Name() {
			return nil
		}

		return gen.ErrTargetExist
	}

	tm.monitorRelations[key] = struct{}{}

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

	if target.Node == tm.core.Name() {
		return nil
	}

	if needsRemote == false {
		return nil
	}

	connection, err := tm.core.GetConnection(target.Node)
	if err != nil {
		delete(tm.monitorRelations, key)
		delete(entry.consumers, consumer)

		if len(entry.consumers) == 0 {
			delete(tm.targetIndex, target)
		}

		return err
	}

	err = connection.MonitorAlias(tm.core.PID(), target)
	if err != nil {
		delete(tm.monitorRelations, key)
		delete(entry.consumers, consumer)

		if len(entry.consumers) == 0 {
			delete(tm.targetIndex, target)
		}

		return err
	}

	entry.allowAlwaysFirst = false

	return nil
}

func (tm *targetManager) DemonitorAlias(consumer gen.PID, target gen.Alias) error {
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

	connection.DemonitorAlias(tm.core.PID(), target)

	return nil
}
