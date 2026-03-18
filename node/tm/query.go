package tm

import "ergo.services/ergo/gen"

func (tm *targetManager) HasLink(consumer gen.PID, target any) bool {
	s := tm.shardFor(target)
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	key := relationKey{
		consumer: consumer,
		target:   target,
	}

	_, exists := s.linkRelations[key]
	return exists
}

func (tm *targetManager) HasMonitor(consumer gen.PID, target any) bool {
	s := tm.shardFor(target)
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	key := relationKey{
		consumer: consumer,
		target:   target,
	}

	_, exists := s.monitorRelations[key]
	return exists
}

func (tm *targetManager) LinksFor(consumer gen.PID) []any {
	var targets []any

	for i := range tm.shards {
		s := &tm.shards[i]
		s.mutex.RLock()
		for key := range s.linkRelations {
			if key.consumer == consumer {
				targets = append(targets, key.target)
			}
		}
		s.mutex.RUnlock()
	}

	return targets
}

func (tm *targetManager) MonitorsFor(consumer gen.PID) []any {
	var targets []any

	for i := range tm.shards {
		s := &tm.shards[i]
		s.mutex.RLock()
		for key := range s.monitorRelations {
			if key.consumer == consumer {
				targets = append(targets, key.target)
			}
		}
		s.mutex.RUnlock()
	}

	return targets
}

func (tm *targetManager) EventsFor(producer gen.PID) []gen.Event {
	var events []gen.Event

	for i := range tm.shards {
		s := &tm.shards[i]
		s.mutex.RLock()
		pe := s.producerEvents[producer]
		if pe != nil {
			for event := range pe {
				events = append(events, event)
			}
		}
		s.mutex.RUnlock()
	}

	if len(events) == 0 {
		return nil
	}
	return events
}
