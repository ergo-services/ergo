package gen

import (
	"sync"
)

// defaultTargetManager implements TargetManager using dual-map structure for 
// optimal O(1) consumer cleanup. This implementation solves the memory leak 
// issue (#195) by providing efficient bidirectional relationship management.
type defaultTargetManager struct {
	// Consumer -> Set of targets they link/monitor
	links    map[PID]map[any]struct{}
	monitors map[PID]map[any]struct{}
	
	// Reverse lookup: Target -> Set of consumers linking/monitoring it
	linkConsumers    map[any]map[PID]struct{}
	monitorConsumers map[any]map[PID]struct{}
	
	// Protects all operations
	mu sync.RWMutex
}

// CreateDefaultTargetManager creates a new default target manager implementation
func CreateDefaultTargetManager() TargetManager {
	return &defaultTargetManager{
		links:            make(map[PID]map[any]struct{}),
		monitors:         make(map[PID]map[any]struct{}),
		linkConsumers:    make(map[any]map[PID]struct{}),
		monitorConsumers: make(map[any]map[PID]struct{}),
	}
}

// Link operations

func (tm *defaultTargetManager) AddLink(consumer PID, target any) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Check if relationship already exists
	if targets, exists := tm.links[consumer]; exists {
		if _, hasTarget := targets[target]; hasTarget {
			return ErrTargetExist
		}
	}

	// Initialize consumer's target set if needed
	if tm.links[consumer] == nil {
		tm.links[consumer] = make(map[any]struct{})
	}

	// Initialize target's consumer set if needed
	if tm.linkConsumers[target] == nil {
		tm.linkConsumers[target] = make(map[PID]struct{})
	}

	// Add to both maps
	tm.links[consumer][target] = struct{}{}
	tm.linkConsumers[target][consumer] = struct{}{}

	return nil
}

func (tm *defaultTargetManager) RemoveLink(consumer PID, target any) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Check if relationship exists
	targets, consumerExists := tm.links[consumer]
	if !consumerExists {
		return ErrTargetUnknown
	}
	
	if _, hasTarget := targets[target]; !hasTarget {
		return ErrTargetUnknown
	}

	// Remove from both maps
	delete(tm.links[consumer], target)
	delete(tm.linkConsumers[target], consumer)

	// Clean up empty maps
	if len(tm.links[consumer]) == 0 {
		delete(tm.links, consumer)
	}
	if len(tm.linkConsumers[target]) == 0 {
		delete(tm.linkConsumers, target)
	}

	return nil
}

func (tm *defaultTargetManager) HasLink(consumer PID, target any) bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	targets, exists := tm.links[consumer]
	if !exists {
		return false
	}
	
	_, exists = targets[target]
	return exists
}

// Monitor operations

func (tm *defaultTargetManager) AddMonitor(consumer PID, target any) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Check if relationship already exists
	if targets, exists := tm.monitors[consumer]; exists {
		if _, hasTarget := targets[target]; hasTarget {
			return ErrTargetExist
		}
	}

	// Initialize consumer's target set if needed
	if tm.monitors[consumer] == nil {
		tm.monitors[consumer] = make(map[any]struct{})
	}

	// Initialize target's consumer set if needed
	if tm.monitorConsumers[target] == nil {
		tm.monitorConsumers[target] = make(map[PID]struct{})
	}

	// Add to both maps
	tm.monitors[consumer][target] = struct{}{}
	tm.monitorConsumers[target][consumer] = struct{}{}

	return nil
}

func (tm *defaultTargetManager) RemoveMonitor(consumer PID, target any) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Check if relationship exists
	targets, consumerExists := tm.monitors[consumer]
	if !consumerExists {
		return ErrTargetUnknown
	}
	
	if _, hasTarget := targets[target]; !hasTarget {
		return ErrTargetUnknown
	}

	// Remove from both maps
	delete(tm.monitors[consumer], target)
	delete(tm.monitorConsumers[target], consumer)

	// Clean up empty maps
	if len(tm.monitors[consumer]) == 0 {
		delete(tm.monitors, consumer)
	}
	if len(tm.monitorConsumers[target]) == 0 {
		delete(tm.monitorConsumers, target)
	}

	return nil
}

func (tm *defaultTargetManager) HasMonitor(consumer PID, target any) bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	targets, exists := tm.monitors[consumer]
	if !exists {
		return false
	}
	
	_, exists = targets[target]
	return exists
}

// Cleanup operations - CRITICAL for fixing issue #195

func (tm *defaultTargetManager) CleanupConsumer(consumer PID) (linkTargets []any, monitorTargets []any) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Clean up links - O(1) consumer lookup, O(k) where k = consumer's targets
	if targets, exists := tm.links[consumer]; exists {
		for target := range targets {
			linkTargets = append(linkTargets, target)
			// Remove from reverse mapping
			delete(tm.linkConsumers[target], consumer)
			if len(tm.linkConsumers[target]) == 0 {
				delete(tm.linkConsumers, target)
			}
		}
		delete(tm.links, consumer)
	}

	// Clean up monitors - O(1) consumer lookup, O(k) where k = consumer's targets
	if targets, exists := tm.monitors[consumer]; exists {
		for target := range targets {
			monitorTargets = append(monitorTargets, target)
			// Remove from reverse mapping
			delete(tm.monitorConsumers[target], consumer)
			if len(tm.monitorConsumers[target]) == 0 {
				delete(tm.monitorConsumers, target)
			}
		}
		delete(tm.monitors, consumer)
	}

	return linkTargets, monitorTargets
}

func (tm *defaultTargetManager) CleanupTarget(target any) (linkConsumers []PID, monitorConsumers []PID) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Clean up link consumers - O(1) target lookup, O(c) where c = target's consumers
	if consumers, exists := tm.linkConsumers[target]; exists {
		for consumer := range consumers {
			linkConsumers = append(linkConsumers, consumer)
			// Remove from forward mapping
			delete(tm.links[consumer], target)
			if len(tm.links[consumer]) == 0 {
				delete(tm.links, consumer)
			}
		}
		delete(tm.linkConsumers, target)
	}

	// Clean up monitor consumers - O(1) target lookup, O(c) where c = target's consumers  
	if consumers, exists := tm.monitorConsumers[target]; exists {
		for consumer := range consumers {
			monitorConsumers = append(monitorConsumers, consumer)
			// Remove from forward mapping
			delete(tm.monitors[consumer], target)
			if len(tm.monitors[consumer]) == 0 {
				delete(tm.monitors, consumer)
			}
		}
		delete(tm.monitorConsumers, target)
	}

	return linkConsumers, monitorConsumers
}

func (tm *defaultTargetManager) CleanupNode(node Atom) (linkTargets []any, monitorTargets []any) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Clean up link relationships involving the node
	for consumer, targets := range tm.links {
		shouldCleanupConsumer := consumer.Node == node
		targetsToRemove := make([]any, 0)
		
		for target := range targets {
			if shouldCleanupConsumer || tm.isTargetFromNode(target, node) {
				linkTargets = append(linkTargets, target)
				targetsToRemove = append(targetsToRemove, target)
			}
		}
		
		// Remove targets and clean up reverse mappings
		for _, target := range targetsToRemove {
			delete(tm.links[consumer], target)
			delete(tm.linkConsumers[target], consumer)
			if len(tm.linkConsumers[target]) == 0 {
				delete(tm.linkConsumers, target)
			}
		}
		
		// Remove consumer if no targets left
		if len(tm.links[consumer]) == 0 {
			delete(tm.links, consumer)
		}
	}

	// Clean up monitor relationships involving the node
	for consumer, targets := range tm.monitors {
		shouldCleanupConsumer := consumer.Node == node
		targetsToRemove := make([]any, 0)
		
		for target := range targets {
			if shouldCleanupConsumer || tm.isTargetFromNode(target, node) {
				monitorTargets = append(monitorTargets, target)
				targetsToRemove = append(targetsToRemove, target)
			}
		}
		
		// Remove targets and clean up reverse mappings
		for _, target := range targetsToRemove {
			delete(tm.monitors[consumer], target)
			delete(tm.monitorConsumers[target], consumer)
			if len(tm.monitorConsumers[target]) == 0 {
				delete(tm.monitorConsumers, target)
			}
		}
		
		// Remove consumer if no targets left
		if len(tm.monitors[consumer]) == 0 {
			delete(tm.monitors, consumer)
		}
	}

	return linkTargets, monitorTargets
}

// Inspection operations

func (tm *defaultTargetManager) GetTargetsForConsumer(consumer PID) (links []any, monitors []any) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	// Get all targets this consumer is linking to - O(1) lookup
	if targets, exists := tm.links[consumer]; exists {
		for target := range targets {
			links = append(links, target)
		}
	}

	// Get all targets this consumer is monitoring - O(1) lookup
	if targets, exists := tm.monitors[consumer]; exists {
		for target := range targets {
			monitors = append(monitors, target)
		}
	}

	return links, monitors
}

func (tm *defaultTargetManager) GetConsumersForTarget(target any) (linkConsumers []PID, monitorConsumers []PID) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	// Get all consumers linking to this target - O(1) lookup
	if consumers, exists := tm.linkConsumers[target]; exists {
		for consumer := range consumers {
			linkConsumers = append(linkConsumers, consumer)
		}
	}

	// Get all consumers monitoring this target - O(1) lookup
	if consumers, exists := tm.monitorConsumers[target]; exists {
		for consumer := range consumers {
			monitorConsumers = append(monitorConsumers, consumer)
		}
	}

	return linkConsumers, monitorConsumers
}

// Helper methods

func (tm *defaultTargetManager) isTargetFromNode(target any, node Atom) bool {
	switch t := target.(type) {
	case PID:
		return t.Node == node
	case ProcessID:
		return t.Node == node
	case Alias:
		return t.Node == node
	case Event:
		return t.Node == node
	case Atom:
		return t == node
	}
	return false
}