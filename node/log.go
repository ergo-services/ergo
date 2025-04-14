package node

import (
	"time"

	"ergo.services/ergo/gen"
)

// gen.Log interface implementation

func createLog(level gen.LogLevel, dolog func(gen.MessageLog, string)) *log {
	return &log{
		level: level,
		dolog: dolog,
	}
}

type log struct {
	level  gen.LogLevel
	logger string
	source any
	fields []gen.LogField
	dolog  func(gen.MessageLog, string)
}

func (l *log) Level() gen.LogLevel {
	return l.level
}

func (l *log) SetLevel(level gen.LogLevel) error {
	if level < gen.LogLevelDebug {
		return gen.ErrIncorrect
	}
	if level > gen.LogLevelDisabled {
		return gen.ErrIncorrect
	}
	l.level = level
	return nil
}

func (l *log) Logger() string {
	return l.logger
}

func (l *log) SetLogger(name string) {
	l.logger = name
}

func (l *log) Fields() []gen.LogField {
	f := make([]gen.LogField, len(l.fields))
	copy(f, l.fields)
	return f
}

func (l *log) AddFields(fields ...gen.LogField) {
	l.fields = append(l.fields, fields...)
}

func (l *log) DeleteFields(fields ...string) {
	if len(fields) == 0 {
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

func (l *log) Trace(format string, args ...any) {
	l.write(gen.LogLevelTrace, format, args)
}

func (l *log) Debug(format string, args ...any) {
	l.write(gen.LogLevelDebug, format, args)
}

func (l *log) Info(format string, args ...any) {
	l.write(gen.LogLevelInfo, format, args)
}

func (l *log) Warning(format string, args ...any) {
	l.write(gen.LogLevelWarning, format, args)
}

func (l *log) Error(format string, args ...any) {
	l.write(gen.LogLevelError, format, args)
}

func (l *log) Panic(format string, args ...any) {
	l.write(gen.LogLevelPanic, format, args)
}

func (l *log) setSource(source any) {
	switch source.(type) {
	case gen.MessageLogProcess, gen.MessageLogMeta, gen.MessageLogNode, gen.MessageLogNetwork:
	default:
		panic("unknown source type for log interface")
	}
	l.source = source
}

func (l *log) write(level gen.LogLevel, format string, args []any) {
	if l.level > level {
		return
	}

	m := gen.MessageLog{
		Time:   time.Now(),
		Level:  level,
		Source: l.source,
		Format: format,
		Args:   args,
		Fields: l.fields,
	}

	l.dolog(m, l.logger)
}
