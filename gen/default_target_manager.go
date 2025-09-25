package gen

import "sync"

type relationKey struct {
	consumer PID
	target   any
	monitor  bool
}

type defaultTargetManager struct {
	sync.RWMutex
	relations   map[relationKey]struct{}         // Primary index for exact lookups
	targetIndex map[any]map[relationKey]struct{} // Secondary index by target
}

func CreateDefaultTargetManager() TargetManager {
	return &defaultTargetManager{
		relations:   make(map[relationKey]struct{}),
		targetIndex: make(map[any]map[relationKey]struct{}),
	}
}

func (tm *defaultTargetManager) AddLink(consumer PID, target any) error {
	tm.Lock()
	defer tm.Unlock()

	key := relationKey{consumer: consumer, target: target, monitor: false}
	if _, exists := tm.relations[key]; exists {
		return ErrTargetExist
	}

	tm.relations[key] = struct{}{}

	// Add to target index
	if tm.targetIndex[target] == nil {
		tm.targetIndex[target] = make(map[relationKey]struct{})
	}
	tm.targetIndex[target][key] = struct{}{}

	return nil
}

func (tm *defaultTargetManager) RemoveLink(consumer PID, target any) error {
	tm.Lock()
	defer tm.Unlock()

	key := relationKey{consumer: consumer, target: target, monitor: false}
	if _, exists := tm.relations[key]; !exists {
		return ErrTargetUnknown
	}

	delete(tm.relations, key)

	// Remove from target index
	if targetKeys := tm.targetIndex[target]; targetKeys != nil {
		delete(targetKeys, key)
		if len(targetKeys) == 0 {
			delete(tm.targetIndex, target)
		}
	}

	return nil
}

func (tm *defaultTargetManager) HasLink(consumer PID, target any) bool {
	tm.RLock()
	defer tm.RUnlock()

	key := relationKey{consumer: consumer, target: target, monitor: false}
	_, exists := tm.relations[key]
	return exists
}

func (tm *defaultTargetManager) AddMonitor(consumer PID, target any) error {
	tm.Lock()
	defer tm.Unlock()

	key := relationKey{consumer: consumer, target: target, monitor: true}
	if _, exists := tm.relations[key]; exists {
		return ErrTargetExist
	}

	tm.relations[key] = struct{}{}

	// Add to target index
	if tm.targetIndex[target] == nil {
		tm.targetIndex[target] = make(map[relationKey]struct{})
	}
	tm.targetIndex[target][key] = struct{}{}

	return nil
}

func (tm *defaultTargetManager) RemoveMonitor(consumer PID, target any) error {
	tm.Lock()
	defer tm.Unlock()

	key := relationKey{consumer: consumer, target: target, monitor: true}
	if _, exists := tm.relations[key]; !exists {
		return ErrTargetUnknown
	}

	delete(tm.relations, key)

	// Remove from target index
	if targetKeys := tm.targetIndex[target]; targetKeys != nil {
		delete(targetKeys, key)
		if len(targetKeys) == 0 {
			delete(tm.targetIndex, target)
		}
	}

	return nil
}

func (tm *defaultTargetManager) HasMonitor(consumer PID, target any) bool {
	tm.RLock()
	defer tm.RUnlock()

	key := relationKey{consumer: consumer, target: target, monitor: true}
	_, exists := tm.relations[key]
	return exists
}

func (tm *defaultTargetManager) CleanupConsumer(consumer PID) (linkTargets []any, monitorTargets []any) {
	tm.Lock()
	defer tm.Unlock()

	// Find and delete relations for this consumer
	for key := range tm.relations {
		if key.consumer == consumer {
			if key.monitor {
				monitorTargets = append(monitorTargets, key.target)
			} else {
				linkTargets = append(linkTargets, key.target)
			}
			delete(tm.relations, key)

			// Remove from target index
			if targetKeys := tm.targetIndex[key.target]; targetKeys != nil {
				delete(targetKeys, key)
				if len(targetKeys) == 0 {
					delete(tm.targetIndex, key.target)
				}
			}
		}
	}

	return linkTargets, monitorTargets
}

func (tm *defaultTargetManager) CleanupTarget(target any) (linkConsumers []PID, monitorConsumers []PID) {
	tm.Lock()
	defer tm.Unlock()

	// Use target index for efficient cleanup
	if targetKeys := tm.targetIndex[target]; targetKeys != nil {
		for key := range targetKeys {
			if key.monitor {
				monitorConsumers = append(monitorConsumers, key.consumer)
			} else {
				linkConsumers = append(linkConsumers, key.consumer)
			}
			delete(tm.relations, key)
		}
		// Remove entire target from index
		delete(tm.targetIndex, target)
	}

	return linkConsumers, monitorConsumers
}

func (tm *defaultTargetManager) CleanupNode(
	node Atom,
) (linkTargetsWithConsumers map[any][]PID, monitorTargetsWithConsumers map[any][]PID) {
	tm.Lock()
	defer tm.Unlock()

	linkTargetsWithConsumers = make(map[any][]PID)
	monitorTargetsWithConsumers = make(map[any][]PID)

	// Clean up relations involving the down node
	for key := range tm.relations {
		// If consumer is from terminated node, just delete and continue
		if key.consumer.Node == node {
			delete(tm.relations, key)

			// Remove from target index
			if targetKeys := tm.targetIndex[key.target]; targetKeys != nil {
				delete(targetKeys, key)
				if len(targetKeys) == 0 {
					delete(tm.targetIndex, key.target)
				}
			}
			continue
		}

		// Check if target belongs to the down node
		shouldDelete := false
		switch tt := key.target.(type) {
		case PID:
			if tt.Node == node {
				shouldDelete = true
			}
		case ProcessID:
			if tt.Node == node {
				shouldDelete = true
			}
		case Alias:
			if tt.Node == node {
				shouldDelete = true
			}
		case Event:
			if tt.Node == node {
				shouldDelete = true
			}
		case Atom:
			if tt == node {
				shouldDelete = true
			}
		}

		if shouldDelete {
			// Record which consumers were affected by this target going down
			if key.monitor {
				monitorTargetsWithConsumers[key.target] = append(monitorTargetsWithConsumers[key.target], key.consumer)
			} else {
				linkTargetsWithConsumers[key.target] = append(linkTargetsWithConsumers[key.target], key.consumer)
			}
			delete(tm.relations, key)

			// Remove from target index
			if targetKeys := tm.targetIndex[key.target]; targetKeys != nil {
				delete(targetKeys, key)
				if len(targetKeys) == 0 {
					delete(tm.targetIndex, key.target)
				}
			}
		}
	}

	return linkTargetsWithConsumers, monitorTargetsWithConsumers
}

func (tm *defaultTargetManager) GetTargetsForConsumer(consumer PID) (links []any, monitors []any) {
	tm.RLock()
	defer tm.RUnlock()

	for key := range tm.relations {
		if key.consumer == consumer {
			if key.monitor {
				monitors = append(monitors, key.target)
			} else {
				links = append(links, key.target)
			}
		}
	}

	return links, monitors
}

func (tm *defaultTargetManager) GetConsumersForTarget(target any) []PID {
	tm.RLock()
	defer tm.RUnlock()

	keySet := tm.targetIndex[target] // O(1) direct access
	if keySet == nil {
		return nil
	}

	consumers := make([]PID, 0, len(keySet))
	for key := range keySet { // O(K) where K = consumers for this target
		consumers = append(consumers, key.consumer)
	}

	return consumers
}

