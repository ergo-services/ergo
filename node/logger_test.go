package node

import (
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
)

type stubLogger struct{}

func (stubLogger) Log(gen.MessageLog) {}
func (stubLogger) Terminate()         {}

// LoggerAdd must reject an empty name (which would later panic at logger[0]).
func TestLoggerAddRejectsEmptyName(t *testing.T) {
	n := &node{creation: 1}
	err := n.LoggerAdd("", stubLogger{})
	check.ErrorIs(t, err, gen.ErrIncorrect)
}

// LoggerDeletePID restores the process's prior log level (captured at
// LoggerAddPID time) instead of a hardcoded Info, and clears the logger name.
func TestLoggerDeletePIDRestoresLevel(t *testing.T) {
	n := &node{creation: 1}
	n.log = createLog(gen.LogLevelInfo, func(gen.MessageLog, string) {})

	pid := gen.PID{Node: "n@localhost", ID: 1, Creation: 1}
	p := &process{
		log:         createLog(gen.LogLevelWarning, func(gen.MessageLog, string) {}),
		loggername:  "mylogger",
		loggerlevel: gen.LogLevelDebug, // captured when registered as a logger
	}
	p.log.SetLevel(gen.LogLevelDisabled) // the registered-as-logger state
	n.processes.Store(pid, p)

	n.LoggerDeletePID(pid)

	if p.log.Level() != gen.LogLevelDebug {
		t.Fatalf("level = %v, want restored LogLevelDebug (not hardcoded Info)", p.log.Level())
	}
	if p.loggername != "" {
		t.Fatalf("loggername must be cleared, got %q", p.loggername)
	}
}
