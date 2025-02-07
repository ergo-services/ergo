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
	// Jobs() []Job
	// Job(name Atom) (Job, error)
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
}

type CronActionMessage struct {
	// Process defines where to send MessageCron. Can be local or remote one.
	Process ProcessID

	// Fallback process name if Process isn't reachable.
	// MessageCronFallback will be sent with the details.
	Fallback ProcessFallback
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

	// Fallback process name if the spawning process has failed.
	// MessageCronFallback will be sent with the details.
	Fallback ProcessFallback
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

	// Fallback process name if the spawning process has failed.
	// MessageCronFallback will be sent with the details.
	Fallback ProcessFallback
}

type CronInfo struct {
	Jobs []CronJobInfo
}

type CronJobInfo struct {
}
