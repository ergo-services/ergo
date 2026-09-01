package gen

import (
	"errors"
	"fmt"
)

// Error carries a process exit reason with optional wrapped causes and an
// optional captured mailbox. Msg holds the message text; Errorf fills it with
// fmt.Errorf output, which means the causes' own text ends up inside it as
// well. Wrapped holds errors corresponding to %w substitutions, preserving
// identity through errors.Is and errors.As - not errors.Unwrap, see the
// Unwrap method below. Mailbox, when non-nil, holds the captured mailbox of a
// panicked process for replay on supervisor restart; it is excluded from EDF
// wire encoding.
//
// User code typically constructs *Error through gen.Errorf. The framework
// constructs it directly when capturing a mailbox or wrapping supervisor
// exit reasons.
type Error struct {
	Msg     string
	Wrapped []error
	Mailbox *ProcessMailbox `edf:"-"`
}

// Error returns Msg, or the first cause's text when Msg is empty.
func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Msg == "" && len(e.Wrapped) > 0 {
		return e.Wrapped[0].Error()
	}
	return e.Msg
}

// Unwrap returns the wrapped causes. This is the multi-error form, which the
// standard errors.Unwrap does not follow: it calls only Unwrap() error, so
// errors.Unwrap on *Error returns nil - with a single cause as well as with
// several. errors.Join behaves the same way. Use errors.Is to test identity
// and errors.As to extract a type; both traverse every cause depth-first.
// Read Wrapped directly when the cause objects themselves are needed, and
// check its length rather than indexing it: Errorf leaves it empty whenever a
// %w argument was nil or was not an error.
func (e *Error) Unwrap() []error {
	if e == nil {
		return nil
	}
	return e.Wrapped
}

// Errorf mirrors fmt.Errorf: formats according to format and args, and
// supports %w (single or multiple) for wrapping errors with preserved
// identity. The wrapped errors are stored in Wrapped so that errors.Is and
// errors.As traverse them, and so EDF wire encoding preserves the chain
// across the network when peer capability allows.
//
// Errorf composes a value at the point of failure. It is not a way to declare
// a sentinel: package-level markers must be errors.New values, because an
// *Error cannot be registered in the network error cache and is rebuilt as a
// new value on the receiving node, which leaves errors.Is false against the
// original while the text still looks correct.
func Errorf(format string, args ...any) error {
	w := fmt.Errorf(format, args...)
	e := &Error{Msg: w.Error()}
	switch u := w.(type) {
	case interface{ Unwrap() error }:
		if m := u.Unwrap(); m != nil {
			e.Wrapped = []error{m}
		}
	case interface{ Unwrap() []error }:
		e.Wrapped = u.Unwrap()
	}
	return e
}

var (
	ErrNameUnknown    = errors.New("unknown name")
	ErrParentUnknown  = errors.New("parent/leader is not set")
	ErrNodeTerminated = errors.New("node terminated")

	ErrProcessMailboxFull = errors.New("process mailbox is full")
	ErrProcessUnknown     = errors.New("unknown process")
	ErrProcessIncarnation = errors.New("process ID belongs to the previous incarnation")
	ErrProcessTerminated  = errors.New("process terminated")

	ErrMetaUnknown     = errors.New("unknown meta process")
	ErrMetaMailboxFull = errors.New("meta process mailbox is full")

	ErrApplicationUnknown   = errors.New("unknown application")
	ErrApplicationDepends   = errors.New("dependency fail")
	ErrApplicationState     = errors.New("application is in running/stopping state")
	ErrApplicationLoadPanic = errors.New("panic in application loading")
	ErrApplicationEmpty     = errors.New("application has no items")
	ErrApplicationName      = errors.New("application has no name")
	ErrApplicationStopping  = errors.New("application stopping is in progress")
	ErrApplicationRunning   = errors.New("application is still running")

	ErrTargetUnknown         = errors.New("unknown target")
	ErrTargetExist           = errors.New("target is already exist")
	ErrTargetManagerOverload = errors.New("target manager queue is full")

	ErrRegistrarTerminated = errors.New("registrar client terminated")

	ErrAliasUnknown = errors.New("unknown alias")
	ErrAliasOwner   = errors.New("not an owner")
	ErrEventUnknown = errors.New("unknown event")
	ErrEventOwner   = errors.New("not an owner")
	ErrTaken        = errors.New("resource is taken")

	ErrAtomTooLong = errors.New("too long Atom (max: 255)")

	ErrTimeout     = errors.New("timed out")
	ErrUnsupported = errors.New("not supported")
	ErrUnknown     = errors.New("unknown")
	ErrNotAllowed  = errors.New("not allowed")
	ErrDiscarded   = errors.New("discarded by recipient")
	ErrDisabled    = errors.New("target is disabled")
	ErrBusy        = errors.New("target is busy")
	ErrExceeded    = errors.New("limit exceeded")

	ErrIncorrect       = errors.New("incorrect value or argument")
	ErrMalformed       = errors.New("malformed value")
	ErrResponseIgnored = errors.New("response ignored")
	ErrUnregistered    = errors.New("unregistered")
	ErrTooLarge        = errors.New("too large")

	ErrNetworkStopped = errors.New("network stack is stopped")
	ErrNoConnection   = errors.New("no connection")
	ErrNoRoute        = errors.New("no route")

	ErrInternal = errors.New("internal error")
)
