package unit

import (
	"fmt"
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

// TestLog implements gen.Log for testing
type TestLog struct {
	t      testing.TB
	events lib.QueueMPSC
	level  gen.LogLevel
	logger string
	fields []gen.LogField
	stack  [][]gen.LogField
}

// NewTestLog creates a new test log instance
func NewTestLog(t testing.TB, events lib.QueueMPSC, level gen.LogLevel) *TestLog {
	return &TestLog{
		t:      t,
		events: events,
		level:  level,
		fields: make([]gen.LogField, 0),
		stack:  make([][]gen.LogField, 0),
	}
}

// gen.Log interface implementation

func (tl *TestLog) Level() gen.LogLevel {
	return tl.level
}

func (tl *TestLog) SetLevel(level gen.LogLevel) error {
	tl.level = level
	return nil
}

func (tl *TestLog) Logger() string {
	return tl.logger
}

func (tl *TestLog) SetLogger(name string) {
	tl.logger = name
}

func (tl *TestLog) Fields() []gen.LogField {
	result := make([]gen.LogField, len(tl.fields))
	copy(result, tl.fields)
	return result
}

func (tl *TestLog) AddFields(fields ...gen.LogField) {
	tl.fields = append(tl.fields, fields...)
}

func (tl *TestLog) DeleteFields(fields ...string) {
	if len(fields) == 0 {
		return
	}

	filter := make(map[string]bool)
	for _, f := range fields {
		filter[f] = true
	}

	newFields := make([]gen.LogField, 0)
	for _, f := range tl.fields {
		if _, found := filter[f.Name]; !found {
			newFields = append(newFields, f)
		}
	}

	tl.fields = newFields
}

func (tl *TestLog) PushFields() int {
	// Save current fields to stack
	fieldsCopy := make([]gen.LogField, len(tl.fields))
	copy(fieldsCopy, tl.fields)
	tl.stack = append(tl.stack, fieldsCopy)
	return len(tl.stack)
}

func (tl *TestLog) PopFields() int {
	if len(tl.stack) == 0 {
		return 0
	}

	// Restore fields from stack
	last := len(tl.stack) - 1
	tl.fields = tl.stack[last]
	tl.stack = tl.stack[:last]
	return len(tl.stack)
}

func (tl *TestLog) Trace(format string, args ...any) {
	tl.log(gen.LogLevelTrace, format, args...)
}

func (tl *TestLog) Debug(format string, args ...any) {
	tl.log(gen.LogLevelDebug, format, args...)
}

func (tl *TestLog) Info(format string, args ...any) {
	tl.log(gen.LogLevelInfo, format, args...)
}

func (tl *TestLog) Warning(format string, args ...any) {
	tl.log(gen.LogLevelWarning, format, args...)
}

func (tl *TestLog) Error(format string, args ...any) {
	tl.log(gen.LogLevelError, format, args...)
}

func (tl *TestLog) Panic(format string, args ...any) {
	tl.log(gen.LogLevelPanic, format, args...)
}

func (tl *TestLog) log(level gen.LogLevel, format string, args ...any) {
	if tl.level > level {
		return
	}

	message := fmt.Sprintf(format, args...)
	tl.events.Push(LogEvent{
		Level:   level,
		Message: message,
		Args:    args,
	})
}
