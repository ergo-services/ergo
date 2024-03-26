package act

import (
	"errors"
)

var (
	ErrSupervisorStrategyActive   = errors.New("supervisor strategy is active")
	ErrSupervisorChildUnknown     = errors.New("unknown child")
	ErrSupervisorChildRunning     = errors.New("child process is already running")
	ErrSupervisorChildDisabled    = errors.New("child is disabled")
	ErrSupervisorRestartsExceeded = errors.New("restart intensity is exceeded")
	ErrSupervisorChildDuplicate   = errors.New("duplicate child spec Name")

	ErrPoolEmpty = errors.New("no worker process in the pool")
)
