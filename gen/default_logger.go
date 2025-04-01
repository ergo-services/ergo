package gen

import (
	"fmt"
	"io"
	"os"
)

// DefaultLoggerOptions
type DefaultLoggerOptions struct {
	// Disable makes node to disable default logger
	Disable bool
	// TimeFormat enables output time in the defined format. See https://pkg.go.dev/time#pkg-constants
	// Not defined format makes output time as a timestamp in nanoseconds.
	TimeFormat string
	// IncludeBehavior includes process/meta behavior to the log message
	IncludeBehavior bool
	// IncludeName includes registered process name to the log message
	IncludeName bool
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
	l.includeBehavior = options.IncludeBehavior
	l.includeName = options.IncludeName

	return &l
}

type defaultLogger struct {
	out             io.Writer
	format          string
	includeBehavior bool
	includeName     bool
}

func (l *defaultLogger) Log(m MessageLog) {
	var t string
	var source string
	var behavior string
	var name string

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
		if l.includeBehavior {
			behavior = " " + src.Behavior
		}
		if l.includeName && src.Name != "" {
			name = " " + src.Name.String()
		}
		source = src.PID.String()
	case MessageLogMeta:
		if l.includeBehavior {
			behavior = " " + src.Behavior
		}
		source = src.Meta.String()
	default:
		panic(fmt.Sprintf("unknown log source type: %#v", m.Source))
	}

	message := fmt.Sprintf(m.Format, m.Args...)
	_, err := fmt.Fprintf(l.out, "%s [%s] %s%s%s: %s\n",
		t, m.Level, source, name, behavior, message)
	if err != nil {
		fmt.Printf("(fallback) %s [%s] %s%s%s: %s\n",
			t, m.Level, source, name, behavior, message)
	}
}

func (l *defaultLogger) Terminate() {}
