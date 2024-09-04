package gen

import (
	"fmt"
	"hash/crc32"
	"strings"
	"sync"
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
	return fmt.Sprintf("<%s.%d.%d>", p.Node.CRC32(), int32(p.ID>>32), int32(p.ID))
}
func (p PID) MarshalJSON() ([]byte, error) {
	return []byte("\"" + p.String() + "\""), nil
}

// ProcessID long notation of registered process {process_name, node_name}
type ProcessID struct {
	Name Atom
	Node Atom
}

// String string representaion of ProcessID value
func (p ProcessID) String() string {
	return fmt.Sprintf("<%s.%s>", p.Node.CRC32(), p.Name)
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
	return fmt.Sprintf("Ref#<%s.%d.%d.%d>", r.Node.CRC32(), r.ID[0], r.ID[1], r.ID[2])
}
func (r Ref) MarshalJSON() ([]byte, error) {
	return []byte("\"" + r.String() + "\""), nil
}

// Alias
type Alias Ref

// String
func (a Alias) String() string {
	return fmt.Sprintf("Alias#<%s.%d.%d.%d>", a.Node.CRC32(), a.ID[0], a.ID[1], a.ID[2])
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
	Notify bool
	Buffer int
}

func (e Event) String() string {
	return fmt.Sprintf("Event#<%s:%s>", e.Node.CRC32(), e.Name)
}
func (e Event) MarshalJSON() ([]byte, error) {
	return []byte("\"" + e.String() + "\""), nil
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
