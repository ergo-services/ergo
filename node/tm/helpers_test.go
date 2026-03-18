package tm

import "ergo.services/ergo/gen"

// Test helpers: aggregate views across shards

func (tm *targetManager) totalLinks() int {
	count := 0
	for i := range tm.shards {
		count += len(tm.shards[i].linkRelations)
	}
	return count
}

func (tm *targetManager) totalMonitors() int {
	count := 0
	for i := range tm.shards {
		count += len(tm.shards[i].monitorRelations)
	}
	return count
}

func (tm *targetManager) totalEvents() int {
	count := 0
	for i := range tm.shards {
		count += len(tm.shards[i].events)
	}
	return count
}

func (tm *targetManager) totalTargetIndex() int {
	count := 0
	for i := range tm.shards {
		count += len(tm.shards[i].targetIndex)
	}
	return count
}

func (tm *targetManager) hasLinkRelation(consumer gen.PID, target any) bool {
	s := tm.shardFor(target)
	_, exists := s.linkRelations[relationKey{consumer: consumer, target: target}]
	return exists
}

func (tm *targetManager) hasMonitorRelation(consumer gen.PID, target any) bool {
	s := tm.shardFor(target)
	_, exists := s.monitorRelations[relationKey{consumer: consumer, target: target}]
	return exists
}

func (tm *targetManager) getTargetEntry(target any) *targetEntry {
	s := tm.shardFor(target)
	return s.targetIndex[target]
}

func (tm *targetManager) getEventEntry(event gen.Event) *eventEntry {
	s := tm.shardFor(event)
	return s.events[event]
}

func (tm *targetManager) getProducerEventsMap(producer gen.PID) map[gen.Event]struct{} {
	result := make(map[gen.Event]struct{})
	for i := range tm.shards {
		if pe := tm.shards[i].producerEvents[producer]; pe != nil {
			for event := range pe {
				result[event] = struct{}{}
			}
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
