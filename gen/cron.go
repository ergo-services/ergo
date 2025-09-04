package gen

import (
	"time"
)

type CronOptions struct {
	Jobs []CronJob
}

type Cron interface {
	// AddJob adds new job
	AddJob(job CronJob) error
	// RemoveJob removes new job
	RemoveJob(name Atom) error
	// EnableJob allows you to enable previously disabled job
	EnableJob(name Atom) error
	// DisableJob disables job
	DisableJob(name Atom) error

	// Info returns information about the jobs, spool of jobs for the next run
	Info() CronInfo
	// JobInfo returns information for the given job
	JobInfo(name Atom) (CronJobInfo, error)

	// Schedule returns a list of jobs planned to be run for the given period
	Schedule(since time.Time, duration time.Duration) []CronSchedule
	// JobSchedule returns a list of scheduled run times for the given job and period.
	JobSchedule(job Atom, since time.Time, duration time.Duration) ([]time.Time, error)
}

type CronJob struct {
	// Name job name
	Name Atom
	// Spec time spec in "crontab" format
	Spec string
	// Location defines timezone
	Location *time.Location
	// Action
	Action CronAction
	// Fallback
	Fallback ProcessFallback
}

type CronInfo struct {
	Next  time.Time
	Spool []Atom
	Jobs  []CronJobInfo
}

type CronSchedule struct {
	Time time.Time
	Jobs []Atom
}

type CronJobInfo struct {
	Disabled   bool
	Name       Atom
	Spec       string
	Location   string
	ActionInfo string
	LastRun    time.Time
	LastErr    string
	Fallback   ProcessFallback
}
