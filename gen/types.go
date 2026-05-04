package gen

import (
	"fmt"
	"hash/crc32"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	crc32q     = crc32.MakeTable(0xD5828281)
	crc32cache sync.Map
)

// Atom is a special kind of string used for process names and node names.
// The max allowed length is 255 bytes. Exceed this limit causes disability
// to send it over the network. Keep this in mind.
type Atom string

func (a Atom) String() string {
	return "'" + string(a) + "'"
}

func (a Atom) Host() string {
	s := strings.Split(string(a), "@")
	if len(s) == 2 {
		return s[1]
	}
	return ""
}

func (a Atom) CRC32Sum() uint32 {
	return crc32.Checksum([]byte(a), crc32q)
}

func (a Atom) CRC32() string {
	if v, exist := crc32cache.Load(a); exist {
		return v.(string)
	}
	if a == "" {
		return ""
	}
	hash := fmt.Sprintf("%08X", crc32.Checksum([]byte(a), crc32q))
	crc32cache.Store(a, hash)
	return hash
}

// PID
type PID struct {
	Node     Atom
	ID       uint64
	Creation int64
}

// String
func (p PID) String() string {
	buf := make([]byte, 0, 32)
	buf = append(buf, '<')
	buf = append(buf, p.Node.CRC32()...)
	buf = append(buf, '.')
	buf = strconv.AppendInt(buf, int64(int32(p.ID>>32)), 10)
	buf = append(buf, '.')
	buf = strconv.AppendInt(buf, int64(int32(p.ID)), 10)
	buf = append(buf, '>')
	return string(buf)
}

func (p PID) MarshalJSON() ([]byte, error) {
	return []byte("\"" + p.String() + "\""), nil
}

// ProcessID long notation of registered process {process_name, node_name}
type ProcessID struct {
	Name Atom
	Node Atom
}

// String
func (p ProcessID) String() string {
	buf := make([]byte, 0, 64)
	buf = append(buf, '<')
	buf = append(buf, p.Node.CRC32()...)
	buf = append(buf, '.')
	buf = append(buf, p.Name...)
	buf = append(buf, '>')
	return string(buf)
}
func (p ProcessID) MarshalJSON() ([]byte, error) {
	return []byte("\"" + p.String() + "\""), nil
}

// Ref
type Ref struct {
	Node     Atom
	Creation int64
	ID       [3]uint64
}

// String
func (r Ref) String() string {
	buf := make([]byte, 0, 64)
	buf = append(buf, "Ref#<"...)
	buf = append(buf, r.Node.CRC32()...)
	buf = append(buf, '.')
	buf = strconv.AppendUint(buf, r.ID[0], 10)
	buf = append(buf, '.')
	buf = strconv.AppendUint(buf, r.ID[1], 10)
	buf = append(buf, '.')
	buf = strconv.AppendUint(buf, r.ID[2], 10)
	buf = append(buf, '>')
	return string(buf)
}

// Deadline returns the deadline timestamp (unix seconds) stored in ID[2].
// Returns 0 if no deadline was set.
func (r Ref) Deadline() uint64 {
	return r.ID[2]
}

// IsAlive checks if the reference is still valid (deadline not exceeded).
// Returns true if no deadline is set (ID[2] == 0) or false if current time is after deadline.
func (r Ref) IsAlive() bool {
	deadline := r.ID[2]
	if deadline == 0 {
		return true
	}
	now := uint64(time.Now().Unix())
	if now > deadline {
		return false
	}
	return true
}

func (r Ref) MarshalJSON() ([]byte, error) {
	return []byte("\"" + r.String() + "\""), nil
}

// Alias
type Alias Ref

// String
func (a Alias) String() string {
	buf := make([]byte, 0, 64)
	buf = append(buf, "Alias#<"...)
	buf = append(buf, a.Node.CRC32()...)
	buf = append(buf, '.')
	buf = strconv.AppendUint(buf, a.ID[0], 10)
	buf = append(buf, '.')
	buf = strconv.AppendUint(buf, a.ID[1], 10)
	buf = append(buf, '.')
	buf = strconv.AppendUint(buf, a.ID[2], 10)
	buf = append(buf, '>')
	return string(buf)
}

func (a Alias) MarshalJSON() ([]byte, error) {
	return []byte("\"" + a.String() + "\""), nil
}

// Event
type Event struct {
	Name Atom
	Node Atom
}

type EventOptions struct {
	// Notify controls whether the producer receives MessageEventStart and
	// MessageEventStop when the subscriber count crosses zero. Ignored for
	// node-level events (producer is corePID), since the node core does not
	// consume such messages.
	Notify bool

	// Buffer is the size of the ring buffer that stores the most recent
	// MessageEvent values. New subscribers receive the buffer contents on
	// Link or Monitor. Zero means no buffer.
	Buffer int

	// Open disables the token check on SendEvent. When true, any local process
	// can publish to this event by name, regardless of the token value. The
	// token returned by RegisterEvent is still generated but not required for
	// publishing. The owner check on UnregisterEvent is unaffected: only the
	// registering process (or the node, for node-level events) can unregister.
	// Default: false.
	Open bool
}

// String
func (e Event) String() string {
	buf := make([]byte, 0, 64)
	buf = append(buf, "Event#<"...)
	buf = append(buf, e.Node.CRC32()...)
	buf = append(buf, ':')
	buf = append(buf, e.Name...)
	buf = append(buf, '>')
	return string(buf)
}
func (e Event) MarshalJSON() ([]byte, error) {
	return []byte("\"" + e.String() + "\""), nil
}

// RegisteredTypeInfo describes a type registered with a wire-format proto.
type RegisteredTypeInfo struct {
	// ID is the per-proto registration order. Strictly monotonic; gaps possible.
	ID uint64
	// Name is the canonical wire-format name within the proto.
	Name string
	// Kind classifies the type for filters/badges (bool, int, struct, marshaler, ...).
	Kind string
	// Schema is a Go-syntax shape of the type.
	Schema string
	// Proto is the proto version owning this entry, set by the aggregator.
	Proto string
	// MinSize is the wire-format size in bytes of a zero-value of this type
	// when sent as a top-level message (including the type-tag prefix).
	// Real values may be larger if SizeVariable is true.
	MinSize uint32
	// SizeVariable is true if the type contains string/slice/map/pointer/
	// interface fields whose encoded size depends on the actual value.
	SizeVariable bool
}

// Env
type Env string

func (e Env) String() string {
	return strings.ToUpper(string(e))
}
func (e Env) MarshalJSON() ([]byte, error) {
	return []byte("\"" + e.String() + "\""), nil
}

// Version
type Version struct {
	Name    string
	Release string
	Commit  string
	License string
}

func (v Version) String() string {
	if v.Name == "" {
		return ""
	}
	if v.Commit == "" {
		return v.Str()
	}
	return fmt.Sprintf("%s:%s[%s]", v.Name, v.Release, v.Commit)
}

func (v Version) Str() string {
	if v.Name == "" {
		return ""
	}
	return fmt.Sprintf("%s:%s", v.Name, v.Release)
}

// LogLevel
type LogLevel int

func (l LogLevel) String() string {
	switch l {
	case LogLevelTrace:
		return "trace"
	case LogLevelDebug:
		return "debug"
	case LogLevelInfo:
		return "info"
	case LogLevelWarning:
		return "warning"
	case LogLevelError:
		return "error"
	case LogLevelPanic:
		return "panic"
	case LogLevelDisabled:
		return "disabled"

	case LogLevelSystem:
		return "system"
	}
	return "unknown log level"
}

func (l LogLevel) MarshalJSON() ([]byte, error) {
	return []byte("\"" + l.String() + "\""), nil
}

const (
	LogLevelSystem LogLevel = -100

	LogLevelTrace    LogLevel = -2
	LogLevelDebug    LogLevel = -1
	LogLevelDefault  LogLevel = 0 // inherite from node/app/parent process
	LogLevelInfo     LogLevel = 1
	LogLevelWarning  LogLevel = 2
	LogLevelError    LogLevel = 3
	LogLevelPanic    LogLevel = 4
	LogLevelDisabled LogLevel = 5
)
