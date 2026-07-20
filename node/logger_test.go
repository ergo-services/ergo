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
