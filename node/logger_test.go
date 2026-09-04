package node

import (
	"sync"
	"sync/atomic"
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

// Concurrent LoggerAdd of the same name must register exactly one logger: the
// existence check and the store are serialized, so a second racing Add sees
// ErrTaken instead of silently replacing (and leaking) the first.
func TestLoggerAddConcurrentSameName(t *testing.T) {
	n := &node{creation: 1, loggers: make(map[gen.LogLevel]*sync.Map)}
	for _, lvl := range []gen.LogLevel{
		gen.LogLevelSystem, gen.LogLevelTrace, gen.LogLevelDebug, gen.LogLevelInfo,
		gen.LogLevelWarning, gen.LogLevelError, gen.LogLevelPanic,
	} {
		n.loggers[lvl] = &sync.Map{}
	}
	n.log = createLog(gen.LogLevelInfo, func(gen.MessageLog, string) {})

	const goroutines = 50
	var success int64
	var start, done sync.WaitGroup
	start.Add(1)
	for i := 0; i < goroutines; i++ {
		done.Add(1)
		go func() {
			defer done.Done()
			start.Wait() // fire all at once to maximize the race window
			if err := n.LoggerAdd("dup", stubLogger{}); err == nil {
				atomic.AddInt64(&success, 1)
			}
		}()
	}
	start.Done()
	done.Wait()

	if success != 1 {
		t.Fatalf("concurrent LoggerAdd of the same name: %d succeeded, want exactly 1", success)
	}
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
