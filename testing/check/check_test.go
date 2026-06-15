package check_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

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

// growSource reveals its records only after revealAt polls, so Within's polling can
// be exercised deterministically (by poll count, not wall-clock data races).
type growSource struct {
	recs     []check.Record
	calls    int
	revealAt int
}

func (s *growSource) Records() []check.Record {
	s.calls++
	if s.calls >= s.revealAt {
		return s.recs
	}
	return nil
}

func isA(m msgRec) bool    { return m.from == "a" }
func isPing(m msgRec) bool { return m.body == "ping" }

// Within waits for a record that appears after several polls.
func TestCheckWithinFindsLateRecord(t *testing.T) {
	src := &growSource{recs: snapshot{msgRec{"a", "ping"}}, revealAt: 3}
	check.For[msgRec](t, src).Where(isPing).Within(2 * time.Second).Once().Assert()
}

// Within + exact cardinality fails when the count overshoots n between polls (documented).
func TestCheckWithinExactOvershootFails(t *testing.T) {
	src := &growSource{recs: snapshot{msgRec{"a", "ping"}, msgRec{"a", "ping"}}, revealAt: 2}
	ft := &fakeT{}
	check.For[msgRec](ft, src).Where(isPing).Within(200 * time.Millisecond).Times(1).Assert()
	if ft.failed == false {
		t.Fatal("expected Times(1) to fail when the count overshoots to 2")
	}
}

// Capture waits (up to Within) and returns the first match.
func TestCheckCaptureWithin(t *testing.T) {
	src := &growSource{recs: snapshot{msgRec{"a", "ping"}}, revealAt: 3}
	r, ok := check.For[msgRec](t, src).Where(isPing).Within(2 * time.Second).Capture()
	if ok == false || r.body != "ping" {
		t.Fatalf("Capture: got (%v, %v)", r, ok)
	}
}

// Collect returns every match in observation order.
func TestCheckCollectOrder(t *testing.T) {
	src := snapshot{msgRec{"a", "1"}, msgRec{"b", "x"}, msgRec{"a", "2"}}
	got := check.For[msgRec](t, src).Where(isA).Collect()
	if len(got) != 2 || got[0].body != "1" || got[1].body != "2" {
		t.Fatalf("Collect order: %v", got)
	}
}

// Since scopes the assertion to records observed after a mark.
func TestCheckSinceScopes(t *testing.T) {
	full := snapshot{msgRec{"a", "old"}, msgRec{"a", "new"}}
	check.For[msgRec](t, full).Where(isA).Since(1).Times(1).Assert() // only "new"
}

// None().Within passes when nothing matches in the window, fails when a match exists.
func TestCheckNoneWithin(t *testing.T) {
	src := snapshot{msgRec{"a", "ping"}}
	check.For[msgRec](t, src).
		Where(func(m msgRec) bool { return m.body == "nope" }).
		None().Within(50 * time.Millisecond).Assert()

	ft := &fakeT{}
	check.For[msgRec](ft, src).Where(isPing).None().Within(50 * time.Millisecond).Assert()
	if ft.failed == false {
		t.Fatal("expected None to fail when a match exists")
	}
}
