package gen

import (
	"errors"
)

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

	ErrTargetUnknown = errors.New("unknown target")
	ErrTargetExist   = errors.New("target is already exist")

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
