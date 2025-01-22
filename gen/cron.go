package gen

import (
	"time"
)

type CronOptions struct {
}

type Cron interface {
	AddJob(job Job) error
	RemoveJob(name Atom) error
	EnableJob(name Atom) error
	DisableJob(name Atom) error
	// Jobs() []Job
	// Job(name Atom) (Job, error)
	Info() CronInfo
}

type Job struct {
	// Name job name
	Name Atom
	// Spec time spec in "crontab" format
	Spec string
	// Action can be either JobActionMessage, JobActionSpawn, JobActionRemoteSpawn
	Action any
}

type JobActionMessage struct {
	// Process can be either gen.PID, gen.ProcessID, gen.Atom, gen.Alias. Local or remote.
	Process any

	// Fallback process name if Process isn't reachable ()
	Fallback ProcessFallback
}

type JobActionSpawn struct {
	// Name use registered name for the spawned process
	Name Atom
	// ProcessFactory
	ProcessFactory ProcessFactory
	// ProcessOptions
	ProcessOptions ProcessOptions
	// Args
	Args []any

	// Fallback process name if Process isn't reachable ()
	Fallback ProcessFallback
}
type JobActionRemoteSpawn struct {
	// Node remote node name
	Node Atom
	// Name use registered name for the spawned process
	Name           Atom
	ProcessOptions ProcessOptions
	Args           []any

	// Fallback process name if Process isn't reachable ()
	Fallback ProcessFallback
}

type JobSpawn struct {
	Enable bool
}

type CronInfo struct {
}
