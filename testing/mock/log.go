package mock

import (
	"fmt"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
)

// Log is a standalone gen.Log mock. Every method has an On<Method> override; unset,
// the line emitters (Trace/Debug/Info/Warning/Error/Panic) record a check.Log gated by
// the level and the rest return safe defaults.
type Log struct {
	recorder
	level  gen.LogLevel
	logger string
	fields []gen.LogField
	ov     logOverrides
}

type logOverrides struct {
	level        func() gen.LogLevel
	setLevel     func(level gen.LogLevel) error
	logger       func() string
	setLogger    func(name string)
	fields       func() []gen.LogField
	addFields    func(fields ...gen.LogField)
	deleteFields func(fields ...string)
	pushFields   func() int
	popFields    func() int
	trace        func(format string, args ...any)
	debug        func(format string, args ...any)
	info         func(format string, args ...any)
	warning      func(format string, args ...any)
	err          func(format string, args ...any)
	panic        func(format string, args ...any)
}

var _ gen.Log = (*Log)(nil)

// NewLog returns a dumb gen.Log mock (no recording; use NewLogT for Should*).
func NewLog() *Log { return newLog(recorder{}) }

// NewLogT returns a gen.Log mock that records every emitted line as a check.Log and
// asserts through t.
func NewLogT(t check.T) *Log { return newLog(newRecorder(t)) }

func newLog(r recorder) *Log { return &Log{recorder: r, level: gen.LogLevelInfo} }

// On<Method> overrides

func (l *Log) OnLevel(fn func() gen.LogLevel)                { l.ov.level = fn }
func (l *Log) OnSetLevel(fn func(level gen.LogLevel) error)  { l.ov.setLevel = fn }
func (l *Log) OnLogger(fn func() string)                     { l.ov.logger = fn }
func (l *Log) OnSetLogger(fn func(name string))              { l.ov.setLogger = fn }
func (l *Log) OnFields(fn func() []gen.LogField)             { l.ov.fields = fn }
func (l *Log) OnAddFields(fn func(fields ...gen.LogField))   { l.ov.addFields = fn }
func (l *Log) OnDeleteFields(fn func(fields ...string))      { l.ov.deleteFields = fn }
func (l *Log) OnPushFields(fn func() int)                    { l.ov.pushFields = fn }
func (l *Log) OnPopFields(fn func() int)                     { l.ov.popFields = fn }
func (l *Log) OnTrace(fn func(format string, args ...any))   { l.ov.trace = fn }
func (l *Log) OnDebug(fn func(format string, args ...any))   { l.ov.debug = fn }
func (l *Log) OnInfo(fn func(format string, args ...any))    { l.ov.info = fn }
func (l *Log) OnWarning(fn func(format string, args ...any)) { l.ov.warning = fn }
func (l *Log) OnError(fn func(format string, args ...any))   { l.ov.err = fn }
func (l *Log) OnPanic(fn func(format string, args ...any))   { l.ov.panic = fn }

// gen.Log

func (l *Log) Level() gen.LogLevel {
	if l.ov.level != nil {
		return l.ov.level()
	}
	return l.level
}

func (l *Log) SetLevel(level gen.LogLevel) error {
	if l.ov.setLevel != nil {
		return l.ov.setLevel(level)
	}
	if level < gen.LogLevelDebug || level > gen.LogLevelDisabled {
		return gen.ErrIncorrect
	}
	l.level = level
	return nil
}

func (l *Log) Logger() string {
	if l.ov.logger != nil {
		return l.ov.logger()
	}
	return l.logger
}

func (l *Log) SetLogger(name string) {
	if l.ov.setLogger != nil {
		l.ov.setLogger(name)
		return
	}
	l.logger = name
}

func (l *Log) Fields() []gen.LogField {
	if l.ov.fields != nil {
		return l.ov.fields()
	}
	return l.fields
}

func (l *Log) AddFields(fields ...gen.LogField) {
	if l.ov.addFields != nil {
		l.ov.addFields(fields...)
		return
	}
	l.fields = append(l.fields, fields...)
}

func (l *Log) DeleteFields(fields ...string) {
	if l.ov.deleteFields != nil {
		l.ov.deleteFields(fields...)
	}
}

func (l *Log) PushFields() int {
	if l.ov.pushFields != nil {
		return l.ov.pushFields()
	}
	return 0
}

func (l *Log) PopFields() int {
	if l.ov.popFields != nil {
		return l.ov.popFields()
	}
	return 0
}

func (l *Log) emit(level gen.LogLevel, format string, args ...any) {
	if l.level > level {
		return
	}
	l.put(check.Log{Level: level, Message: fmt.Sprintf(format, args...)})
}

func (l *Log) Trace(format string, args ...any) {
	if l.ov.trace != nil {
		l.ov.trace(format, args...)
	}
	l.emit(gen.LogLevelTrace, format, args...)
}

func (l *Log) Debug(format string, args ...any) {
	if l.ov.debug != nil {
		l.ov.debug(format, args...)
	}
	l.emit(gen.LogLevelDebug, format, args...)
}

func (l *Log) Info(format string, args ...any) {
	if l.ov.info != nil {
		l.ov.info(format, args...)
	}
	l.emit(gen.LogLevelInfo, format, args...)
}

func (l *Log) Warning(format string, args ...any) {
	if l.ov.warning != nil {
		l.ov.warning(format, args...)
	}
	l.emit(gen.LogLevelWarning, format, args...)
}

func (l *Log) Error(format string, args ...any) {
	if l.ov.err != nil {
		l.ov.err(format, args...)
	}
	l.emit(gen.LogLevelError, format, args...)
}

func (l *Log) Panic(format string, args ...any) {
	if l.ov.panic != nil {
		l.ov.panic(format, args...)
	}
	l.emit(gen.LogLevelPanic, format, args...)
}
