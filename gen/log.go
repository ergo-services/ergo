package gen

import (
	"fmt"
)

type Log interface {
	Level() LogLevel
	SetLevel(level LogLevel) error

	Logger() string
	SetLogger(name string)

	Fields() []LogField
	AddFields(fields ...LogField)
	DeleteFields(fields ...string)
	PushFields()
	PopFields()

	Trace(format string, args ...any)
	Debug(format string, args ...any)
	Info(format string, args ...any)
	Warning(format string, args ...any)
	Error(format string, args ...any)
	Panic(format string, args ...any)
}

type LogField struct {
	Name  string
	Value any
}

func (lf LogField) String() string {
	return fmt.Sprintf("%s:%v", lf.Name, lf.Value)
}

type LoggerBehavior interface {
	Log(message MessageLog)
	Terminate()
}
