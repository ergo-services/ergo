package act

import (
	"errors"
)

var (
	ErrSupervisorStrategyActive   = errors.New("supervisor strategy is active")
	ErrSupervisorChildUnknown     = errors.New("unknown child")
	ErrSupervisorChildRunning     = errors.New("child process is already running")
	ErrSupervisorChildDisabled    = errors.New("child is disabled")
	ErrSupervisorRestartsExceeded = errors.New("restart intensity exceeded")
	ErrSupervisorChildDuplicate   = errors.New("duplicate child spec Name")
	ErrSupervisorInvalidSpec      = errors.New("invalid supervisor spec")

	ErrPoolEmpty = errors.New("no worker process in the pool")
)
