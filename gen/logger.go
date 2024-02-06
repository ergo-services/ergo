package gen

import (
	"fmt"
	"io"
	"os"
)

type Log interface {
	Level() LogLevel
	SetLevel(level LogLevel) error

	Logger() string
	SetLogger(name string)

	Trace(format string, args ...any)
	Debug(format string, args ...any)
	Info(format string, args ...any)
	Warning(format string, args ...any)
	Error(format string, args ...any)
	Panic(format string, args ...any)
}

type LoggerBehavior interface {
	Log(message MessageLog)
	Terminate()
}

// DefaultLoggerOptions
type DefaultLoggerOptions struct {
	// Disable makes node to disable default logger
	Disable bool
	// TimeFormat enables output time in the defined format. See https://pkg.go.dev/time#pkg-constants
	// Not defined format makes output time as a timestamp in nanoseconds.
	TimeFormat string
	// Filter enables filtering log messages.
	Filter []LogLevel
	// Output defines output for the log messages. By default it uses os.Stdout
	Output io.Writer
}

//
// default logger for the Ergo Framework. It uses stdout as an output by default, but can be used
// any io.Writer.
//

func CreateDefaultLogger(options DefaultLoggerOptions) LoggerBehavior {
	var l defaultLogger

	l.out = options.Output
	if l.out == nil {
		l.out = os.Stdout
	}

	l.format = options.TimeFormat

	return &l
}

type defaultLogger struct {
	out    io.Writer
	format string
}

func (l *defaultLogger) Log(m MessageLog) {
	var t string
	var source string

	if l.format == "" {
		t = fmt.Sprintf("%d", m.Time.UnixNano())
	} else {
		t = m.Time.Format(l.format)
	}

	switch src := m.Source.(type) {
	case MessageLogNode:
		source = src.Node.CRC32()
	case MessageLogNetwork:
		source = fmt.Sprintf("%s-%s", src.Node.CRC32(), src.Peer.CRC32())
	case MessageLogProcess:
		source = src.PID.String()
	case MessageLogMeta:
		source = src.Meta.String()
	default:
		panic(fmt.Sprintf("unknown log source type: %#v", m.Source))
	}

	message := fmt.Sprintf(m.Format, m.Args...)
	_, err := fmt.Fprintf(l.out, "%s [%s] %s: %s\n", t, m.Level, source, message)
	if err != nil {
		fmt.Printf("(fallback) %s [%s] %s: %s\n", t, m.Level, source, message)
	}
}

func (l *defaultLogger) Terminate() {}
