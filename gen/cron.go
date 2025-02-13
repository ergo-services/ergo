package gen

import (
	"time"
)

type CronOptions struct {
	Jobs []CronJob
}

type Cron interface {
	AddJob(job CronJob) error
	RemoveJob(name Atom) error
	EnableJob(name Atom) error
	DisableJob(name Atom) error
	Info() CronInfo
}

type CronJob struct {
	// Name job name
	Name Atom
	// Spec time spec in "crontab" format
	Spec string
	// Location defines timezone
	Location *time.Location
	// Action can be either CronActionMessage, CronActionSpawn, CronActionRemoteSpawn
	Action any

	// Fallback
	// MessageCronFallback will be sent with the details if action has failed
	Fallback ProcessFallback
}

type CronActionMessage struct {
	// Process defines where to send MessageCron. Can be local or remote one.
	Process ProcessID

	Priority MessagePriority
}

type CronActionSpawn struct {
	// Register use registered name for the spawned process
	Register Atom
	// ProcessFactory
	ProcessFactory ProcessFactory
	// ProcessOptions
	ProcessOptions ProcessOptions
	// Args
	Args []any
}

type CronActionRemoteSpawn struct {
	// Node remote node name
	Node Atom
	// Name of the remote process factory
	Name Atom

	// Register use registered name for the spawned process
	Register       Atom
	ProcessOptions ProcessOptions
	Args           []any
}

type CronInfo struct {
	Next  time.Time
	Spool []Atom
	Jobs  []CronJobInfo
}

type CronJobInfo struct {
	Disabled bool
	Name     Atom
	Spec     string
	Action   any
	LastRun  time.Time
	LastErr  error
	Fallback ProcessFallback
}
