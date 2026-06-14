package check_test

import (
	"errors"
	"fmt"
	"testing"

	"ergo.services/ergo/testing/check"
)

// msgRec is a minimal Record for the sync pilot.
type msgRec struct {
	from string
	body any
}

func (msgRec) Kind() string     { return "msg" }
func (r msgRec) String() string { return fmt.Sprintf("msg(from=%s, body=%v)", r.from, r.body) }

// snapshot is a trivial synchronous check.Source.
type snapshot []check.Record

func (s snapshot) Records() []check.Record { return s }

var errSentinel = errors.New("sentinel")

func TestCheckSyncPilot(t *testing.T) {
	src := snapshot{
		msgRec{"a", "ping"},
		msgRec{"b", "pong"},
		msgRec{"a", "ping"},
	}

	// at least one "ping" from a
	check.For[msgRec](t, src).
		Where(func(m msgRec) bool { return m.from == "a" && m.body == "ping" }).
		AtLeast(1).Assert()

	// exactly two "ping" from a
	check.For[msgRec](t, src).
		Where(func(m msgRec) bool { return m.from == "a" && m.body == "ping" }).
		Times(2).Assert()

	// negative: none with body "nope"
	check.For[msgRec](t, src).
		Where(func(m msgRec) bool { return m.body == "nope" }).
		None().Assert()

	// error vocabulary
	check.NoError(t, nil)
	check.ErrorIs(t, fmt.Errorf("wrap: %w", errSentinel), errSentinel)
	check.ErrorContains(t, errSentinel, "sentin")
}

// fakeT records failures so we can verify the engine actually fails.
type fakeT struct {
	failed bool
	msgs   []string
}

func (f *fakeT) Errorf(format string, args ...any) {
	f.failed = true
	f.msgs = append(f.msgs, fmt.Sprintf(format, args...))
}
func (f *fakeT) FailNow() { f.failed = true }
func (f *fakeT) Helper()  {}

func TestCheckDetectsFailure(t *testing.T) {
	src := snapshot{msgRec{"a", "ping"}}

	ft := &fakeT{}
	check.For[msgRec](ft, src).
		Where(func(m msgRec) bool { return m.body == "nope" }).
		AtLeast(1).Assert()
	if ft.failed == false {
		t.Fatal("expected failure for a missing record")
	}

	ftNone := &fakeT{}
	check.For[msgRec](ftNone, src).
		Where(func(m msgRec) bool { return m.body == "ping" }).
		None().Assert()
	if ftNone.failed == false {
		t.Fatal("expected None to fail when a match exists")
	}

	ftErr := &fakeT{}
	check.NoError(ftErr, errSentinel)
	if ftErr.failed == false {
		t.Fatal("expected NoError to fail on a non-nil error")
	}
}

// IsType matches concrete types exactly and interface types by assignability (the
// latter regressed when the matcher compared reflect types with ==).
func TestCheckIsTypeMatcher(t *testing.T) {
	isString := check.IsType[string]()
	if isString("hello") == false {
		t.Fatal("IsType[string] should match a string")
	}
	if isString(42) {
		t.Fatal("IsType[string] should not match an int")
	}
	if isString(nil) {
		t.Fatal("IsType[string] should not match nil")
	}

	isError := check.IsType[error]()
	if isError(errors.New("boom")) == false {
		t.Fatal("IsType[error] should match a value implementing error")
	}
	if isError("not an error") {
		t.Fatal("IsType[error] should not match a non-error value")
	}
}
