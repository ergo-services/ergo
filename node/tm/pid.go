package tm

import "ergo.services/ergo/gen"

func (tm *targetManager) LinkPID(consumer gen.PID, target gen.PID) error {
	s := tm.shardFor(target)
	s.mutex.Lock()
	defer s.mutex.Unlock()

	key := relationKey{
		consumer: consumer,
		target:   target,
	}

	_, exists := s.linkRelations[key]
	if exists == true && consumer.Node != tm.core.Name() {
		return nil
	}
	if exists == true {
		return gen.ErrTargetExist
	}

	s.linkRelations[key] = struct{}{}

	entry := s.targetIndex[target]
	needsRemote := false

	if entry == nil {
		entry = &targetEntry{
			allowAlwaysFirst: true,
			consumers:        make(map[gen.PID]struct{}),
		}
		s.targetIndex[target] = entry
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
		delete(s.linkRelations, key)
		delete(entry.consumers, consumer)
		if len(entry.consumers) == 0 {
			delete(s.targetIndex, target)
		}
		return err
	}

	err = connection.LinkPID(tm.core.PID(), target)
	if err != nil {
		delete(s.linkRelations, key)
		delete(entry.consumers, consumer)
		if len(entry.consumers) == 0 {
			delete(s.targetIndex, target)
		}
		return err
	}

	entry.allowAlwaysFirst = false
	return nil
}

func (tm *targetManager) UnlinkPID(consumer gen.PID, target gen.PID) error {
	s := tm.shardFor(target)
	s.mutex.Lock()
	defer s.mutex.Unlock()

	key := relationKey{
		consumer: consumer,
		target:   target,
	}

	if _, exists := s.linkRelations[key]; exists == false {
		return nil
	}

	delete(s.linkRelations, key)

	entry := s.targetIndex[target]
	if entry == nil {
		return nil
	}

	delete(entry.consumers, consumer)

	isLast := (len(entry.consumers) == 0)
	if isLast {
		delete(s.targetIndex, target)
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

	connection.UnlinkPID(tm.core.PID(), target)
	return nil
}

func (tm *targetManager) MonitorPID(consumer gen.PID, target gen.PID) error {
	s := tm.shardFor(target)
	s.mutex.Lock()
	defer s.mutex.Unlock()

	key := relationKey{
		consumer: consumer,
		target:   target,
	}

	_, exists := s.monitorRelations[key]
	if exists == true && consumer.Node != tm.core.Name() {
		return nil
	}
	if exists == true {
		return gen.ErrTargetExist
	}

	s.monitorRelations[key] = struct{}{}

	entry := s.targetIndex[target]
	needsRemote := false

	if entry == nil {
		entry = &targetEntry{
			allowAlwaysFirst: true,
			consumers:        make(map[gen.PID]struct{}),
		}
		s.targetIndex[target] = entry
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
		delete(s.monitorRelations, key)
		delete(entry.consumers, consumer)
		if len(entry.consumers) == 0 {
			delete(s.targetIndex, target)
		}
		return err
	}

	err = connection.MonitorPID(tm.core.PID(), target)
	if err != nil {
		delete(s.monitorRelations, key)
		delete(entry.consumers, consumer)
		if len(entry.consumers) == 0 {
			delete(s.targetIndex, target)
		}
		return err
	}

	entry.allowAlwaysFirst = false
	return nil
}

func (tm *targetManager) DemonitorPID(consumer gen.PID, target gen.PID) error {
	s := tm.shardFor(target)
	s.mutex.Lock()
	defer s.mutex.Unlock()

	key := relationKey{
		consumer: consumer,
		target:   target,
	}

	if _, exists := s.monitorRelations[key]; exists == false {
		return nil
	}

	delete(s.monitorRelations, key)

	entry := s.targetIndex[target]
	if entry == nil {
		return nil
	}

	delete(entry.consumers, consumer)

	isLast := (len(entry.consumers) == 0)
	if isLast {
		delete(s.targetIndex, target)
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
