package node

import (
	"fmt"
	"runtime"
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
	level            gen.LogLevel
	logger           string
	source           any
	fields           []gen.LogField
	stacktrace       bool
	stacktraceLevels map[gen.LogLevel]bool
	stacktraceDepth  int
	dolog            func(gen.MessageLog, string)
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

func (l *log) EnableStackTrace(depth int, levels ...gen.LogLevel) {
	l.stacktrace = true
	if len(levels) == 0 {
		levels = []gen.LogLevel{gen.LogLevelPanic}
	}
	if depth < 1 {
		depth = 10
	}

	l.stacktraceLevels = make(map[gen.LogLevel]bool)
	for _, lev := range levels {
		l.stacktraceLevels[lev] = true
	}
}

func (l *log) DisableStackTrace() {
	l.stacktrace = false
	l.stacktraceLevels = nil
}

func (l *log) Fields() []gen.LogField {
	f := make([]gen.LogField, len(l.fields))
	copy(f, l.fields)
	return f
}

func (l *log) AddFields(fields ...gen.LogField) {
	l.fields = append(l.fields, fields...)
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

	if l.stacktrace == false {
		l.dolog(m, l.logger)
		return
	}

	if _, found := l.stacktraceLevels[level]; found == false {
		l.dolog(m, l.logger)
		return
	}

	stack := make([]uintptr, l.stacktraceDepth)
	n := runtime.Callers(2, stack)
	nstack := stack[:n]
	frames := runtime.CallersFrames(nstack)

	for {
		frame, more := frames.Next()
		line := fmt.Sprintf("%s %s:%d", frame.Function, frame.File, frame.Line)
		m.StackTrace = append(m.StackTrace, line)
		if more == false {
			break
		}
	}

	l.dolog(m, l.logger)
}
