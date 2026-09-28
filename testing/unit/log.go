package unit

import (
	"fmt"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
)

// mockLog implements gen.Log, recording every log line as a check.Log on the
// node recorder. Both the mock process and the mock node use it (each with its
// own from PID).
type mockLog struct {
	node   *mockNode
	from   gen.PID
	level  gen.LogLevel
	logger string
	fields []gen.LogField
	stack  [][]gen.LogField
	source any
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
func (l *mockLog) Logger() string        { return l.logger }
func (l *mockLog) SetLogger(name string) { l.logger = name }

func (l *mockLog) setSource(source any) { l.source = source }

func (l *mockLog) Fields() []gen.LogField {
	f := make([]gen.LogField, len(l.fields))
	copy(f, l.fields)
	return f
}

func (l *mockLog) AddFields(fields ...gen.LogField) { l.fields = append(l.fields, fields...) }

func (l *mockLog) DeleteFields(fields ...string) {
	if len(fields) == 0 {
		return
	}

	if ls := len(l.stack); ls > 0 {
		l.Error("cannot delete log field(s) while the field stack has %d active frame(s); use the PopFields method instead", ls)
		return
	}

	filter := make(map[string]bool)
	for _, f := range fields {
		filter[f] = true
	}

	newFields := []gen.LogField{}
	for _, f := range l.fields {
		if _, found := filter[f.Name]; found {
			continue
		}
		newFields = append(newFields, f)
	}

	if len(newFields) > 0 {
		l.fields = newFields
		return
	}

	l.fields = nil
}

func (l *mockLog) PushFields() int {
	l.stack = append(l.stack, l.fields)
	return len(l.stack)
}

func (l *mockLog) PopFields() int {
	last := len(l.stack) - 1
	if last < 0 {
		return 0
	}
	l.fields = l.stack[last]
	l.stack = l.stack[:last]
	return len(l.stack)
}

func (l *mockLog) emit(level gen.LogLevel, format string, args ...any) {
	// mirror the real logger's gate: drop lines below the configured level
	if l.level > level {
		return
	}
	var fields []gen.LogField
	if len(l.fields) > 0 {
		fields = make([]gen.LogField, len(l.fields))
		copy(fields, l.fields)
	}
	l.node.rec.Put(check.Log{
		From:    l.from,
		Level:   level,
		Message: fmt.Sprintf(format, args...),
		Format:  format,
		Args:    args,
		Fields:  fields,
		Source:  l.source,
	})
}

func (l *mockLog) Trace(format string, args ...any)   { l.emit(gen.LogLevelTrace, format, args...) }
func (l *mockLog) Debug(format string, args ...any)   { l.emit(gen.LogLevelDebug, format, args...) }
func (l *mockLog) Info(format string, args ...any)    { l.emit(gen.LogLevelInfo, format, args...) }
func (l *mockLog) Warning(format string, args ...any) { l.emit(gen.LogLevelWarning, format, args...) }
func (l *mockLog) Error(format string, args ...any)   { l.emit(gen.LogLevelError, format, args...) }
func (l *mockLog) Panic(format string, args ...any)   { l.emit(gen.LogLevelPanic, format, args...) }
