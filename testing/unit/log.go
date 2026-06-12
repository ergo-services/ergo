package unit

import (
	"fmt"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
)

// mockLog implements gen.Log, recording every log line as a check.Logged on the
// node recorder. Both the mock process and the mock node use it (each with its
// own from PID).
type mockLog struct {
	node   *mockNode
	from   gen.PID
	level  gen.LogLevel
	logger string
	fields []gen.LogField
}

func newMockLog(node *mockNode, from gen.PID, level gen.LogLevel) *mockLog {
	return &mockLog{node: node, from: from, level: level}
}

func (l *mockLog) Level() gen.LogLevel { return l.level }

// SetLevel mirrors the real logger: the level must be within [Debug, Disabled]
// (note this rejects Trace, matching the framework's current behavior).
func (l *mockLog) SetLevel(level gen.LogLevel) error {
	if level < gen.LogLevelDebug || level > gen.LogLevelDisabled {
		return gen.ErrIncorrect
	}
	l.level = level
	return nil
}
func (l *mockLog) Logger() string                   { return l.logger }
func (l *mockLog) SetLogger(name string)            { l.logger = name }
func (l *mockLog) Fields() []gen.LogField           { return l.fields }
func (l *mockLog) AddFields(fields ...gen.LogField) { l.fields = append(l.fields, fields...) }
func (l *mockLog) DeleteFields(fields ...string)    {}
func (l *mockLog) PushFields() int                  { return 0 }
func (l *mockLog) PopFields() int                   { return 0 }

func (l *mockLog) emit(level gen.LogLevel, format string, args ...any) {
	// mirror the real logger's gate: drop lines below the configured level
	if l.level > level {
		return
	}
	l.node.rec.Put(check.Logged{From: l.from, Level: level, Message: fmt.Sprintf(format, args...)})
}

func (l *mockLog) Trace(format string, args ...any)   { l.emit(gen.LogLevelTrace, format, args...) }
func (l *mockLog) Debug(format string, args ...any)   { l.emit(gen.LogLevelDebug, format, args...) }
func (l *mockLog) Info(format string, args ...any)    { l.emit(gen.LogLevelInfo, format, args...) }
func (l *mockLog) Warning(format string, args ...any) { l.emit(gen.LogLevelWarning, format, args...) }
func (l *mockLog) Error(format string, args ...any)   { l.emit(gen.LogLevelError, format, args...) }
func (l *mockLog) Panic(format string, args ...any)   { l.emit(gen.LogLevelPanic, format, args...) }
