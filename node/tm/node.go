package tm

import "ergo.services/ergo/gen"

// Node operations (always local - connection monitoring)

func (tm *targetManager) LinkNode(consumer gen.PID, target gen.Atom) error {
	s := tm.shardFor(target)
	s.mutex.Lock()
	defer s.mutex.Unlock()

	key := relationKey{
		consumer: consumer,
		target:   target,
	}

	if _, exists := s.linkRelations[key]; exists {
		return gen.ErrTargetExist
	}

	s.linkRelations[key] = struct{}{}

	entry := s.targetIndex[target]
	if entry == nil {
		entry = &targetEntry{
			consumers: make(map[gen.PID]struct{}),
		}
		s.targetIndex[target] = entry
	}
	entry.consumers[consumer] = struct{}{}

	return nil
}

func (tm *targetManager) UnlinkNode(consumer gen.PID, target gen.Atom) error {
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
	if len(entry.consumers) == 0 {
		delete(s.targetIndex, target)
	}

	return nil
}

func (tm *targetManager) MonitorNode(consumer gen.PID, target gen.Atom) error {
	s := tm.shardFor(target)
	s.mutex.Lock()
	defer s.mutex.Unlock()

	key := relationKey{
		consumer: consumer,
		target:   target,
	}

	if _, exists := s.monitorRelations[key]; exists {
		return gen.ErrTargetExist
	}

	s.monitorRelations[key] = struct{}{}

	entry := s.targetIndex[target]
	if entry == nil {
		entry = &targetEntry{
			consumers: make(map[gen.PID]struct{}),
		}
		s.targetIndex[target] = entry
	}
	entry.consumers[consumer] = struct{}{}

	return nil
}

func (tm *targetManager) DemonitorNode(consumer gen.PID, target gen.Atom) error {
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
	if len(entry.consumers) == 0 {
		delete(s.targetIndex, target)
	}

	return nil
}
