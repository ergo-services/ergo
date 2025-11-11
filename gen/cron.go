package gen

import (
	"time"
)

// CronOptions configures the cron scheduler on node start.
// Part of NodeOptions. Defines jobs to be scheduled automatically.
type CronOptions struct {
	// Jobs is the list of cron jobs to register on node start.
	// Jobs are scheduled according to their Spec (crontab format).
	Jobs []CronJob
}

// Cron interface provides time-based task scheduling using crontab syntax.
// Retrieved via node.Cron(). Manages periodic jobs with various actions.
type Cron interface {
	// AddJob adds a new cron job to the scheduler.
	// Job is scheduled according to its Spec (crontab format).
	// Returns ErrTaken if job name already exists.
	AddJob(job CronJob) error

	// RemoveJob removes a job from the scheduler.
	// Returns ErrUnknown if job doesn't exist.
	RemoveJob(name Atom) error

	// EnableJob enables a previously disabled job.
	// Job will resume executing on schedule.
	// Returns ErrUnknown if job doesn't exist.
	EnableJob(name Atom) error

	// DisableJob temporarily disables a job.
	// Job remains registered but won't execute until enabled.
	// Returns ErrUnknown if job doesn't exist.
	DisableJob(name Atom) error

	// Info returns comprehensive cron scheduler information.
	// Includes next run time, queued jobs, and all job details.
	Info() CronInfo

	// JobInfo returns detailed information for a specific job.
	// Includes spec, location, last run, and error information.
	// Returns ErrUnknown if job doesn't exist.
	JobInfo(name Atom) (CronJobInfo, error)

	// Schedule returns all jobs planned to run within the given time period.
	// Groups jobs by their scheduled execution time.
	// Useful for previewing upcoming job executions.
	Schedule(since time.Time, duration time.Duration) []CronSchedule

	// JobSchedule returns scheduled run times for a specific job within the period.
	// Returns a list of times when this job will execute.
	// Returns ErrUnknown if job doesn't exist.
	JobSchedule(job Atom, since time.Time, duration time.Duration) ([]time.Time, error)
}

// CronJob defines a scheduled task with execution time and action.
type CronJob struct {
	// Name is the unique job identifier.
	Name Atom

	// Spec defines the execution schedule in crontab format.
	// Examples:
	//   "* * * * *" - every minute
	//   "0 * * * *" - every hour
	//   "0 0 * * *" - every day at midnight
	//   "*/5 * * * *" - every 5 minutes
	Spec string

	// Location specifies the timezone for schedule interpretation.
	// If nil, uses UTC. Example: time.LoadLocation("America/New_York").
	Location *time.Location

	// Action defines what to do when the job runs.
	// Can be: Send message, Spawn process, or custom CronAction.
	Action CronAction

	// Fallback handles action failures.
	// If action fails, error is forwarded to the fallback process.
	Fallback ProcessFallback
}

// CronInfo contains comprehensive cron scheduler status.
// Retrieved via cron.Info(). Shows current state and all jobs.
type CronInfo struct {
	// Next is the time of the next scheduled job execution.
	Next time.Time

	// Spool lists job names queued for immediate execution.
	// Jobs waiting to run (e.g., accumulated during downtime).
	Spool []Atom

	// Jobs lists all registered jobs with their details.
	Jobs []CronJobInfo
}

// CronSchedule groups jobs by their scheduled execution time.
// Part of cron.Schedule() result. Shows which jobs run at each time.
type CronSchedule struct {
	// Time is the scheduled execution time.
	Time time.Time

	// Jobs lists job names scheduled to run at this time.
	Jobs []Atom
}

// CronJobInfo contains detailed information about a cron job.
// Retrieved via cron.JobInfo() or as part of CronInfo.
type CronJobInfo struct {
	// Disabled indicates if the job is currently disabled.
	// Disabled jobs don't execute but remain registered.
	Disabled bool

	// Name is the job identifier.
	Name Atom

	// Spec is the crontab schedule specification.
	Spec string

	// Location is the timezone name (e.g., "UTC", "America/New_York").
	Location string

	// ActionInfo describes the action (e.g., "send message to process_name").
	ActionInfo string

	// LastRun is the timestamp of the last execution.
	// Zero if job hasn't run yet.
	LastRun time.Time

	// LastErr contains the error message from last failed execution.
	// Empty if last execution succeeded.
	LastErr string

	// Fallback is the process that receives action failure notifications.
	Fallback ProcessFallback
}
