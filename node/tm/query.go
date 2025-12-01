package tm

import "ergo.services/ergo/gen"

func (tm *targetManager) HasLink(consumer gen.PID, target any) bool {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	key := relationKey{
		consumer: consumer,
		target:   target,
	}

	_, exists := tm.linkRelations[key]
	return exists
}

func (tm *targetManager) HasMonitor(consumer gen.PID, target any) bool {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	key := relationKey{
		consumer: consumer,
		target:   target,
	}

	_, exists := tm.monitorRelations[key]
	return exists
}

func (tm *targetManager) LinksFor(consumer gen.PID) []any {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	var targets []any

	for key := range tm.linkRelations {
		if key.consumer == consumer {
			targets = append(targets, key.target)
		}
	}

	return targets
}

func (tm *targetManager) MonitorsFor(consumer gen.PID) []any {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	var targets []any

	for key := range tm.monitorRelations {
		if key.consumer == consumer {
			targets = append(targets, key.target)
		}
	}

	return targets
}

func (tm *targetManager) EventsFor(producer gen.PID) []gen.Event {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	// Use producerEvents index for O(1) lookup
	eventSet := tm.producerEvents[producer]
	if eventSet == nil {
		return nil
	}

	events := make([]gen.Event, 0, len(eventSet))
	for event := range eventSet {
		events = append(events, event)
	}

	return events
}
