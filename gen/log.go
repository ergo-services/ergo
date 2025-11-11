package gen

import (
	"fmt"
)

// Log interface provides logging functionality for processes and nodes.
// Retrieved via process.Log() or node.Log().
// Supports structured logging with fields, log levels, and custom loggers.
//
// Logging Architecture (Fan-Out):
// - By default, log messages are sent to ALL registered loggers (fan-out)
// - Each logger receives messages based on its filter (log level)
// - Use SetLogger() to limit logging to a specific logger only (no fan-out)
//
// Example:
//   Default behavior: log.Info("msg") → sent to default logger + custom loggers
//   With SetLogger("file"): log.Info("msg") → sent ONLY to "file" logger
type Log interface {
	// Level returns the current logging level.
	Level() LogLevel

	// SetLevel sets the logging level.
	// Only messages at or above this level are logged.
	// Returns ErrIncorrect for invalid level.
	SetLevel(level LogLevel) error

	// Logger returns the name of the custom logger assigned to this source.
	// Empty string means fan-out to all loggers (default behavior).
	Logger() string

	// SetLogger assigns a specific logger to this log source.
	// Disables fan-out - messages go ONLY to this logger.
	// The named logger must be registered via node.LoggerAdd().
	// Empty string enables fan-out to all loggers (default).
	// Use to limit logging for specific processes (e.g., verbose process → file only).
	SetLogger(name string)

	// Fields returns the current structured logging fields.
	// Fields are included in all log messages from this source.
	Fields() []LogField

	// AddFields adds structured fields to all subsequent log messages.
	// Fields are key-value pairs (e.g., "user_id", "request_id").
	// Useful for correlation and filtering.
	AddFields(fields ...LogField)

	// DeleteFields removes specified fields by name.
	// Fields no longer included in log messages.
	DeleteFields(fields ...string)

	// PushFields saves the current field set and starts a new scope.
	// Returns the depth level. Use with PopFields() for scoped logging.
	// Example: add request fields, log operations, pop fields on completion.
	PushFields() int

	// PopFields restores the previous field set.
	// Returns the new depth level. Use with PushFields() for scoped logging.
	// If no pushed fields, does nothing.
	PopFields() int

	// Trace logs a trace-level message (most verbose).
	// Only logged if level <= LogLevelTrace.
	Trace(format string, args ...any)

	// Debug logs a debug-level message.
	// Only logged if level <= LogLevelDebug.
	Debug(format string, args ...any)

	// Info logs an info-level message.
	// Only logged if level <= LogLevelInfo.
	Info(format string, args ...any)

	// Warning logs a warning-level message.
	// Only logged if level <= LogLevelWarning.
	Warning(format string, args ...any)

	// Error logs an error-level message.
	// Only logged if level <= LogLevelError.
	Error(format string, args ...any)

	// Panic logs a panic-level message.
	// Always logged regardless of level setting.
	// Does NOT trigger actual panic - just logs with panic severity.
	Panic(format string, args ...any)
}

// LogField represents a structured logging field (key-value pair).
// Used with Log.AddFields() for structured logging and log correlation.
type LogField struct {
	// Name is the field key (e.g., "user_id", "request_id", "transaction").
	Name string

	// Value is the field value (can be any type, formatted via %v).
	Value any
}

func (lf LogField) String() string {
	return fmt.Sprintf("%s:%v", lf.Name, lf.Value)
}

// LoggerBehavior interface defines a custom logger implementation.
// Register via node.LoggerAdd() to receive log messages.
//
// Built-in default logger (gen.CreateDefaultLogger):
// - Automatically enabled unless NodeOptions.Log.DefaultLogger.Disable = true
// - Writes to os.Stdout (or custom io.Writer)
// - Plain text or JSON format
// - See DefaultLoggerOptions for configuration
//
// Official loggers from ergo.services/logger module:
// - colored: Terminal output with ANSI colors highlighting PIDs, Atoms, Refs
//   • Colorizes log levels and Ergo types for visual clarity
//   • Includes emoji indicators for log levels
//   • Performance overhead - not for intensive logging
//   • Disable default logger to avoid duplicate console output
//   • Module: ergo.services/logger/colored
//
// - rotate: File logger with automatic rotation and gzip compression
//   • Size-based or time-based rotation policies
//   • Configurable retention (delete old logs)
//   • Async writes with buffering for performance
//   • Module: ergo.services/logger/rotate
//
// Custom implementations: syslog, metrics aggregation, database logging, remote logging.
type LoggerBehavior interface {
	// Log processes a log message.
	// Called for each log message matching the logger's filter.
	// Should not block - queue messages if needed for async processing.
	// For high-volume logging, use buffering and async writes.
	Log(message MessageLog)

	// Terminate is called when logger is removed or node stops.
	// Use for cleanup: close files, flush buffers, stop goroutines.
	// Must complete quickly - node shutdown waits for logger termination.
	Terminate()
}
