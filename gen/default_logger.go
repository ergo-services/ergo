package gen

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
)

// Buffer pool for reusing strings.Builder instances
var builderPool = sync.Pool{
	New: func() interface{} {
		return &strings.Builder{}
	},
}

// Pre-escaped common log levels to avoid repeated escaping
var escapedLevels = map[LogLevel]string{
	LogLevelTrace:    `"trace"`,
	LogLevelDebug:    `"debug"`,
	LogLevelInfo:     `"info"`,
	LogLevelWarning:  `"warning"`,
	LogLevelError:    `"error"`,
	LogLevelPanic:    `"panic"`,
	LogLevelDisabled: `"disabled"`,
	LogLevelSystem:   `"system"`,
}

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
	// IncludeFields includes associated fields to the log message
	IncludeFields bool
	// Filter enables filtering log messages.
	Filter []LogLevel
	// Output defines output for the log messages. By default it uses os.Stdout
	Output io.Writer
	// EnableJSON enables JSON output format
	EnableJSON bool
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
	l.includeFields = options.IncludeFields
	l.enableJSON = options.EnableJSON

	return &l
}

type defaultLogger struct {
	out             io.Writer
	format          string
	includeBehavior bool
	includeName     bool
	includeFields   bool
	enableJSON      bool
}

func (l *defaultLogger) Log(m MessageLog) {
	if l.enableJSON {
		l.logJSON(m)
	} else {
		l.logPlainText(m)
	}
}

func (l *defaultLogger) logJSON(m MessageLog) {
	// Get a buffer from the pool
	buf := builderPool.Get().(*strings.Builder)
	defer func() {
		buf.Reset()
		builderPool.Put(buf)
	}()

	// Pre-allocate buffer size estimate to reduce reallocations
	estimatedSize := 256
	if l.includeFields {
		estimatedSize += len(m.Fields) * 32
	}
	buf.Grow(estimatedSize)

	// Build JSON directly in buffer
	buf.WriteString(`{"time":`)

	// Optimized time handling
	if l.format == "" {
		buf.WriteString(strconv.FormatInt(m.Time.UnixNano(), 10))
	} else {
		buf.WriteByte('"')
		l.writeEscapedStringDirect(buf, m.Time.Format(l.format))
		buf.WriteByte('"')
	}

	// Use pre-escaped log levels
	buf.WriteString(`,"level":`)
	if escaped, exists := escapedLevels[m.Level]; exists {
		buf.WriteString(escaped)
	} else {
		buf.WriteByte('"')
		l.writeEscapedStringDirect(buf, m.Level.String())
		buf.WriteByte('"')
	}

	// Optimized source object building
	buf.WriteString(`,"source":`)
	l.writeSourceObjectDirect(buf, m.Source)

	// Optimized message formatting
	buf.WriteString(`,"message":"`)
	if len(m.Args) == 0 {
		l.writeEscapedStringDirect(buf, m.Format)
	} else {
		// Format message and escape directly
		formattedMsg := fmt.Sprintf(m.Format, m.Args...)
		l.writeEscapedStringDirect(buf, formattedMsg)
	}
	buf.WriteByte('"')

	// Optimized fields handling with type-specific formatting
	if l.includeFields && len(m.Fields) > 0 {
		buf.WriteString(`,"fields":{`)
		for i, field := range m.Fields {
			if i > 0 {
				buf.WriteByte(',')
			}
			buf.WriteByte('"')
			l.writeEscapedStringDirect(buf, field.Name)
			buf.WriteString(`":"`)
			l.writeFieldValueDirect(buf, field.Value)
			buf.WriteByte('"')
		}
		buf.WriteByte('}')
	}

	buf.WriteString("}\n")

	// Write directly to output
	l.out.Write([]byte(buf.String()))
}

// Optimized source object writing directly to buffer
func (l *defaultLogger) writeSourceObjectDirect(buf *strings.Builder, source any) {
	switch src := source.(type) {
	case MessageLogNode:
		buf.WriteString(`{"type":"node","node":"`)
		l.writeEscapedStringDirect(buf, src.Node.CRC32())
		buf.WriteString(`"}`)
	case MessageLogNetwork:
		buf.WriteString(`{"type":"network","node":"`)
		l.writeEscapedStringDirect(buf, src.Node.CRC32())
		buf.WriteString(`","peer":"`)
		l.writeEscapedStringDirect(buf, src.Peer.CRC32())
		buf.WriteString(`"}`)
	case MessageLogProcess:
		buf.WriteString(`{"type":"process","pid":"`)
		l.writeEscapedStringDirect(buf, src.PID.String())
		buf.WriteByte('"')

		if l.includeName && src.Name != "" {
			buf.WriteString(`,"name":"`)
			l.writeEscapedStringDirect(buf, src.Name.String())
			buf.WriteByte('"')
		}

		if l.includeBehavior {
			buf.WriteString(`,"behavior":"`)
			l.writeEscapedStringDirect(buf, src.Behavior)
			buf.WriteByte('"')
		}
		buf.WriteByte('}')
	case MessageLogMeta:
		buf.WriteString(`{"type":"meta","meta":"`)
		l.writeEscapedStringDirect(buf, src.Meta.String())
		buf.WriteByte('"')

		if l.includeBehavior {
			buf.WriteString(`,"behavior":"`)
			l.writeEscapedStringDirect(buf, src.Behavior)
			buf.WriteByte('"')
		}
		buf.WriteByte('}')
	default:
		buf.WriteString(`{"type":"unknown","raw":"`)
		l.writeEscapedStringDirect(buf, fmt.Sprintf("%#v", source))
		buf.WriteString(`"}`)
	}
}

// Type-specific field value writing for better performance
func (l *defaultLogger) writeFieldValueDirect(buf *strings.Builder, value any) {
	switch v := value.(type) {
	case string:
		l.writeEscapedStringDirect(buf, v)
	case int:
		l.writeEscapedStringDirect(buf, strconv.Itoa(v))
	case int64:
		l.writeEscapedStringDirect(buf, strconv.FormatInt(v, 10))
	case int32:
		l.writeEscapedStringDirect(buf, strconv.FormatInt(int64(v), 10))
	case float64:
		l.writeEscapedStringDirect(buf, strconv.FormatFloat(v, 'f', -1, 64))
	case float32:
		l.writeEscapedStringDirect(buf, strconv.FormatFloat(float64(v), 'f', -1, 32))
	case bool:
		if v {
			l.writeEscapedStringDirect(buf, "true")
		} else {
			l.writeEscapedStringDirect(buf, "false")
		}
	default:
		// Fallback to string representation for unknown types
		l.writeEscapedStringDirect(buf, fmt.Sprintf("%v", v))
	}
}

// Optimized escaping that writes directly to builder
func (l *defaultLogger) writeEscapedStringDirect(buf *strings.Builder, s string) {
	for _, r := range s {
		switch r {
		case '"':
			buf.WriteString(`\"`)
		case '\\':
			buf.WriteString(`\\`)
		case '\b':
			buf.WriteString(`\b`)
		case '\f':
			buf.WriteString(`\f`)
		case '\n':
			buf.WriteString(`\n`)
		case '\r':
			buf.WriteString(`\r`)
		case '\t':
			buf.WriteString(`\t`)
		default:
			if r < 0x20 {
				buf.WriteString(`\u`)
				// Manually write hex digits to avoid fmt.Sprintf allocation
				hex := "0123456789abcdef"
				buf.WriteByte(hex[(r>>12)&0xF])
				buf.WriteByte(hex[(r>>8)&0xF])
				buf.WriteByte(hex[(r>>4)&0xF])
				buf.WriteByte(hex[r&0xF])
			} else {
				buf.WriteRune(r)
			}
		}
	}
}

func (l *defaultLogger) logPlainText(m MessageLog) {
	// Get a buffer from the pool
	buf := builderPool.Get().(*strings.Builder)
	defer func() {
		buf.Reset()
		builderPool.Put(buf)
	}()

	// Pre-allocate buffer size estimate
	estimatedSize := 128
	if l.includeFields {
		estimatedSize += len(m.Fields) * 16
	}
	buf.Grow(estimatedSize)

	// Write time directly to buffer
	if l.format == "" {
		buf.WriteString(strconv.FormatInt(m.Time.UnixNano(), 10))
	} else {
		buf.WriteString(m.Time.Format(l.format))
	}

	// Write level
	buf.WriteString(" [")
	buf.WriteString(m.Level.String())
	buf.WriteString("] ")

	// Write source directly to buffer
	l.writeSourceDirect(buf, m.Source)

	// Write name if included
	if l.includeName {
		switch src := m.Source.(type) {
		case MessageLogProcess:
			if src.Name != "" {
				buf.WriteByte(' ')
				buf.WriteString(src.Name.String())
			}
		}
	}

	// Write behavior if included
	if l.includeBehavior {
		switch src := m.Source.(type) {
		case MessageLogProcess:
			buf.WriteByte(' ')
			buf.WriteString(src.Behavior)
		case MessageLogMeta:
			buf.WriteByte(' ')
			buf.WriteString(src.Behavior)
		}
	}

	buf.WriteString(": ")

	// Write message directly
	if len(m.Args) == 0 {
		buf.WriteString(m.Format)
	} else {
		// Format message efficiently
		formattedMsg := fmt.Sprintf(m.Format, m.Args...)
		buf.WriteString(formattedMsg)
	}

	// Write fields if included
	if l.includeFields && len(m.Fields) > 0 {
		// Calculate spacing for alignment
		timeLen := len(strconv.FormatInt(m.Time.UnixNano(), 10))
		if l.format != "" {
			timeLen = len(m.Time.Format(l.format))
		}

		buf.WriteByte('\n')
		// Add spacing for alignment
		if timeLen > 6 {
			buf.WriteString(strings.Repeat(" ", timeLen-6))
		}
		buf.WriteString("fields ")

		// Write fields efficiently
		l.writeFieldsPlainText(buf, m.Fields)
	}

	buf.WriteByte('\n')

	// Write directly to output
	_, err := l.out.Write([]byte(buf.String()))
	if err != nil {
		// Fallback - print directly
		fmt.Printf("(fallback) %s", buf.String())
	}
}

// Optimized source writing for plain text
func (l *defaultLogger) writeSourceDirect(buf *strings.Builder, source any) {
	switch src := source.(type) {
	case MessageLogNode:
		buf.WriteString(src.Node.CRC32())
	case MessageLogNetwork:
		buf.WriteString(src.Node.CRC32())
		buf.WriteByte('-')
		buf.WriteString(src.Peer.CRC32())
	case MessageLogProcess:
		buf.WriteString(src.PID.String())
	case MessageLogMeta:
		buf.WriteString(src.Meta.String())
	default:
		buf.WriteString(fmt.Sprintf("%#v", source))
	}
}

// Optimized field writing for plain text
func (l *defaultLogger) writeFieldsPlainText(buf *strings.Builder, fields []LogField) {
	for i, field := range fields {
		if i > 0 {
			buf.WriteByte(' ')
		}
		buf.WriteString(field.Name)
		buf.WriteByte(':')
		l.writeFieldValuePlainText(buf, field.Value)
	}
}

// Type-specific field value writing for plain text
func (l *defaultLogger) writeFieldValuePlainText(buf *strings.Builder, value any) {
	switch v := value.(type) {
	case string:
		buf.WriteString(v)
	case int:
		buf.WriteString(strconv.Itoa(v))
	case int64:
		buf.WriteString(strconv.FormatInt(v, 10))
	case int32:
		buf.WriteString(strconv.FormatInt(int64(v), 10))
	case float64:
		buf.WriteString(strconv.FormatFloat(v, 'f', -1, 64))
	case float32:
		buf.WriteString(strconv.FormatFloat(float64(v), 'f', -1, 32))
	case bool:
		if v {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	default:
		// Fallback to string representation for unknown types
		buf.WriteString(fmt.Sprintf("%v", v))
	}
}

func (l *defaultLogger) Terminate() {}
