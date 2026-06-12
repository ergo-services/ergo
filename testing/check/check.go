// Package check is the shared assertion core for the Ergo test layers: the
// in-process unit harness (testing/unit) and the live multi-node system harness
// (testing/stage). Both observe a stream of Records and assert over it with one
// fluent grammar; the only difference is sync (snapshot) vs async (Within).
package check

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"
)

// T is the minimal testing sink. *testing.T satisfies it.
type T interface {
	Errorf(format string, args ...any)
	FailNow()
	Helper()
}

// Record is one observed fact: an intercepted action (unit) or an observed
// happening (stage). Kind/String are for diagnostics; matching is by Go type.
type Record interface {
	Kind() string
	String() string
}

// Source yields the records observed so far (a snapshot). unit returns its
// captured events; stage drains its recorder buffer.
type Source interface {
	Records() []Record
}

// Matcher is a value predicate.
type Matcher func(any) bool

// Anything matches any value.
func Anything() Matcher { return func(any) bool { return true } }

// MatchedBy adapts a predicate to a Matcher.
func MatchedBy(f func(any) bool) Matcher { return f }

// Equals matches a value deeply equal to want.
func Equals(want any) Matcher { return func(got any) bool { return reflect.DeepEqual(got, want) } }

// IsType matches a value whose dynamic type is V.
func IsType[V any]() Matcher {
	want := reflect.TypeOf(new(V)).Elem()
	return func(got any) bool { return got != nil && reflect.TypeOf(got) == want }
}

type cardinality int

const (
	cardAtLeast cardinality = iota
	cardExactly
	cardNone
)

const defaultTick = 10 * time.Millisecond

// Assertion accumulates filters and a cardinality over records of type R.
type Assertion[R Record] struct {
	t      T
	src    Source
	preds  []func(R) bool
	card   cardinality
	n      int
	since  int
	within time.Duration
	tick   time.Duration
}

// For starts an assertion over records of type R taken from src.
func For[R Record](t T, src Source) *Assertion[R] {
	return &Assertion[R]{t: t, src: src, card: cardAtLeast, n: 1, tick: defaultTick}
}

// Where adds a typed filter predicate.
func (a *Assertion[R]) Where(pred func(R) bool) *Assertion[R] { a.preds = append(a.preds, pred); return a }

// Once expects exactly one match.
func (a *Assertion[R]) Once() *Assertion[R] { a.card = cardExactly; a.n = 1; return a }

// Times expects exactly n matches.
func (a *Assertion[R]) Times(n int) *Assertion[R] { a.card = cardExactly; a.n = n; return a }

// AtLeast expects at least n matches.
func (a *Assertion[R]) AtLeast(n int) *Assertion[R] { a.card = cardAtLeast; a.n = n; return a }

// None expects no matches.
func (a *Assertion[R]) None() *Assertion[R] { a.card = cardNone; a.n = 0; return a }

// Since restricts matching to records observed after the given source position
// (a mark obtained earlier, e.g. len(src.Records())). Used to scope an assertion
// to a phase: counting only what happened after the mark, ignoring earlier
// identical records. Composes with every cardinality and Within.
func (a *Assertion[R]) Since(mark int) *Assertion[R] { a.since = mark; return a }

// Within turns the terminal into an async wait of at most d (poll every tick).
//
// With an exact cardinality (Once/Times), the wait is satisfied at the first poll
// where the count equals n. A count that overshoots n between two polls is observed
// as "> n" and never re-equals n, so the assertion then fails at the deadline. This
// is correct for a stable exact count (the common case); for a monotonically growing
// count where you only need a lower bound, use AtLeast instead of Times.
func (a *Assertion[R]) Within(d time.Duration) *Assertion[R] { a.within = d; return a }

// records returns the source records scoped to [since:].
func (a *Assertion[R]) records() []Record {
	recs := a.src.Records()
	if a.since <= 0 {
		return recs
	}
	if a.since >= len(recs) {
		return nil
	}
	return recs[a.since:]
}

// Assert evaluates the assertion (non-fatal).
func (a *Assertion[R]) Assert() { a.t.Helper(); if a.evaluate() == false { a.fail(false) } }

// Must evaluates the assertion and stops the test on failure (fatal).
func (a *Assertion[R]) Must() { a.t.Helper(); if a.evaluate() == false { a.fail(true) } }

// Capture returns the first matching record (waiting up to Within for async).
func (a *Assertion[R]) Capture() (R, bool) {
	if a.within <= 0 {
		return a.first()
	}
	deadline := time.Now().Add(a.within)
	for {
		if r, ok := a.first(); ok {
			return r, true
		}
		if time.Now().Before(deadline) == false {
			return a.first()
		}
		time.Sleep(a.tick)
	}
}

// Collect waits (up to Within) until at least n matches are observed, then
// returns every matching record of type R in observation order, scoped by
// Since/Where. Without Within it returns the current matches immediately. Use it
// to assert on the order of a sequence (e.g. round-robin forward distribution),
// which the count-based terminals cannot express.
func (a *Assertion[R]) Collect() []R {
	if a.within > 0 {
		deadline := time.Now().Add(a.within)
		for a.count() < a.n && time.Now().Before(deadline) {
			time.Sleep(a.tick)
		}
	}
	var out []R
	for _, rec := range a.records() {
		if r, ok := a.matches(rec); ok {
			out = append(out, r)
		}
	}
	return out
}

func (a *Assertion[R]) matches(rec Record) (R, bool) {
	r, ok := any(rec).(R)
	if ok == false {
		var zero R
		return zero, false
	}
	for _, p := range a.preds {
		if p(r) == false {
			var zero R
			return zero, false
		}
	}
	return r, true
}

func (a *Assertion[R]) count() int {
	c := 0
	for _, rec := range a.records() {
		if _, ok := a.matches(rec); ok {
			c++
		}
	}
	return c
}

func (a *Assertion[R]) first() (R, bool) {
	for _, rec := range a.records() {
		if r, ok := a.matches(rec); ok {
			return r, true
		}
	}
	var zero R
	return zero, false
}

func (a *Assertion[R]) satisfied(c int) bool {
	switch a.card {
	case cardExactly:
		return c == a.n
	case cardNone:
		return c == 0
	default:
		return c >= a.n
	}
}

func (a *Assertion[R]) evaluate() bool {
	if a.within <= 0 {
		return a.satisfied(a.count())
	}
	deadline := time.Now().Add(a.within)
	for {
		c := a.count()
		if a.card == cardNone {
			if c > 0 {
				return false
			}
		} else if a.satisfied(c) {
			return true
		}
		if time.Now().Before(deadline) == false {
			break
		}
		time.Sleep(a.tick)
	}
	if a.card == cardNone {
		return true
	}
	return a.satisfied(a.count())
}

func (a *Assertion[R]) fail(fatal bool) {
	a.t.Helper()
	tname := reflect.TypeOf(new(R)).Elem().String()
	want := ""
	switch a.card {
	case cardExactly:
		want = fmt.Sprintf("exactly %d", a.n)
	case cardNone:
		want = "no"
	default:
		want = fmt.Sprintf("at least %d", a.n)
	}
	var seen []string
	for _, rec := range a.records() {
		if _, ok := any(rec).(R); ok {
			seen = append(seen, rec.String())
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "check: expected %s %s, got %d match(es)", want, tname, a.count())
	if len(seen) == 0 {
		b.WriteString("\n  (no records of this kind observed)")
	} else {
		b.WriteString("\n  observed:")
		for _, s := range seen {
			fmt.Fprintf(&b, "\n    - %s", s)
		}
	}
	a.t.Errorf("%s", b.String())
	if fatal {
		a.t.FailNow()
	}
}

// NoError fails if err is non-nil.
func NoError(t T, err error, msg ...any) {
	t.Helper()
	if err != nil {
		t.Errorf("check: expected no error, got: %v%s", err, suffix(msg))
	}
}

// ErrorIs fails unless errors.Is(err, target).
func ErrorIs(t T, err, target error, msg ...any) {
	t.Helper()
	if errors.Is(err, target) == false {
		t.Errorf("check: expected error %v, got: %v%s", target, err, suffix(msg))
	}
}

// ErrorContains fails unless err is non-nil and its message contains sub.
func ErrorContains(t T, err error, sub string, msg ...any) {
	t.Helper()
	if err == nil || strings.Contains(err.Error(), sub) == false {
		t.Errorf("check: expected error containing %q, got: %v%s", sub, err, suffix(msg))
	}
}

// Equal fails unless want and got are deeply equal.
func Equal(t T, want, got any, msg ...any) {
	t.Helper()
	if reflect.DeepEqual(want, got) == false {
		t.Errorf("check: expected %#v, got %#v%s", want, got, suffix(msg))
	}
}

// True fails unless cond is true.
func True(t T, cond bool, msg ...any) {
	t.Helper()
	if cond == false {
		t.Errorf("check: expected true%s", suffix(msg))
	}
}

func suffix(msg []any) string {
	if len(msg) == 0 {
		return ""
	}
	if format, ok := msg[0].(string); ok {
		return ": " + fmt.Sprintf(format, msg[1:]...)
	}
	return ": " + fmt.Sprint(msg...)
}
