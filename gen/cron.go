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
	JobInfo(name Atom) (CronJobInfo, error)

	Schedule(since time.Time, duration time.Duration) []CronSchedule
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
