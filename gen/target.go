package gen

// TargetManager manages all link/monitor relationships between processes.
// This interface provides a clean abstraction for target management,
// ensuring proper cleanup and preventing memory leaks.
type TargetManager interface {
	// Link operations
	// AddLink creates a link relationship between consumer and target.
	// Returns error if relationship already exists or if operation fails.
	AddLink(consumer PID, target any) error

	// RemoveLink removes a link relationship between consumer and target.
	// Returns error if relationship doesn't exist or if operation fails.
	RemoveLink(consumer PID, target any) error

	// HasLink checks if a link relationship exists between consumer and target.
	HasLink(consumer PID, target any) bool

	// Monitor operations
	// AddMonitor creates a monitor relationship between consumer and target.
	// Returns error if relationship already exists or if operation fails.
	AddMonitor(consumer PID, target any) error

	// RemoveMonitor removes a monitor relationship between consumer and target.
	// Returns error if relationship doesn't exist or if operation fails.
	RemoveMonitor(consumer PID, target any) error

	// HasMonitor checks if a monitor relationship exists between consumer and target.
	HasMonitor(consumer PID, target any) bool

	// Cleanup operations - CRITICAL for fixing issue #195
	// CleanupConsumer removes all relationships where the given PID is a consumer.
	// This is called when a process terminates to prevent memory leaks.
	// Returns all targets that were being linked/monitored by the consumer.
	CleanupConsumer(consumer PID) (linkTargets []any, monitorTargets []any)

	// CleanupTarget removes all relationships involving the given target.
	// This is called when a target terminates.
	// Returns all consumers that were linking/monitoring the target.
	CleanupTarget(target any) (linkConsumers []PID, monitorConsumers []PID)

	// CleanupNode removes all relationships involving processes from the given node.
	// This is called when a remote node goes down.
	// Returns all targets that were being linked/monitored by processes from that node.
	CleanupNode(node Atom) (linkTargets []any, monitorTargets []any)

	// Inspection operations (for debugging and process info)
	// GetTargetsForConsumer returns all targets that a consumer is linking/monitoring.
	GetTargetsForConsumer(consumer PID) (links []any, monitors []any)

	// GetConsumersForTarget returns all consumers that are linking/monitoring a target.
	GetConsumersForTarget(target any) (linkConsumers []PID, monitorConsumers []PID)
}

// CreateTargetManager creates a new default target manager implementation.
func CreateTargetManager() TargetManager {
	return CreateDefaultTargetManager()
}