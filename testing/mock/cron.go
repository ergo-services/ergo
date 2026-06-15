package mock

import (
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
)

// Cron is a standalone gen.Cron mock. Every method has an On<Method> override; unset,
// AddJob/RemoveJob record a check.AddCronJob/check.RemoveCronJob and the rest return
// safe defaults.
type Cron struct {
	recorder
	ov cronOverrides
}

type cronOverrides struct {
	addJob      func(job gen.CronJob) error
	removeJob   func(name gen.Atom) error
	enableJob   func(name gen.Atom) error
	disableJob  func(name gen.Atom) error
	info        func() gen.CronInfo
	jobInfo     func(name gen.Atom) (gen.CronJobInfo, error)
	schedule    func(since time.Time, duration time.Duration) []gen.CronSchedule
	jobSchedule func(job gen.Atom, since time.Time, duration time.Duration) ([]time.Time, error)
}

var _ gen.Cron = (*Cron)(nil)

// NewCron returns a dumb gen.Cron mock (no recording; use NewCronT for Should*).
func NewCron() *Cron { return newCron(recorder{}) }

// NewCronT returns a gen.Cron mock that records AddJob/RemoveJob and asserts through t.
func NewCronT(t check.T) *Cron { return newCron(newRecorder(t)) }

func newCron(r recorder) *Cron { return &Cron{recorder: r} }

// On<Method> overrides

func (c *Cron) OnAddJob(fn func(job gen.CronJob) error)   { c.ov.addJob = fn }
func (c *Cron) OnRemoveJob(fn func(name gen.Atom) error)  { c.ov.removeJob = fn }
func (c *Cron) OnEnableJob(fn func(name gen.Atom) error)  { c.ov.enableJob = fn }
func (c *Cron) OnDisableJob(fn func(name gen.Atom) error) { c.ov.disableJob = fn }
func (c *Cron) OnInfo(fn func() gen.CronInfo)             { c.ov.info = fn }
func (c *Cron) OnJobInfo(fn func(name gen.Atom) (gen.CronJobInfo, error)) {
	c.ov.jobInfo = fn
}
func (c *Cron) OnSchedule(fn func(since time.Time, duration time.Duration) []gen.CronSchedule) {
	c.ov.schedule = fn
}
func (c *Cron) OnJobSchedule(fn func(job gen.Atom, since time.Time, duration time.Duration) ([]time.Time, error)) {
	c.ov.jobSchedule = fn
}

// gen.Cron

func (c *Cron) AddJob(job gen.CronJob) error {
	var err error
	if c.ov.addJob != nil {
		err = c.ov.addJob(job)
	}
	c.put(check.AddCronJob{Name: job.Name, Spec: job.Spec, Error: err})
	return err
}

func (c *Cron) RemoveJob(name gen.Atom) error {
	var err error
	if c.ov.removeJob != nil {
		err = c.ov.removeJob(name)
	}
	c.put(check.RemoveCronJob{Name: name, Error: err})
	return err
}

func (c *Cron) EnableJob(name gen.Atom) error {
	if c.ov.enableJob != nil {
		return c.ov.enableJob(name)
	}
	return nil
}

func (c *Cron) DisableJob(name gen.Atom) error {
	if c.ov.disableJob != nil {
		return c.ov.disableJob(name)
	}
	return nil
}

func (c *Cron) Info() gen.CronInfo {
	if c.ov.info != nil {
		return c.ov.info()
	}
	return gen.CronInfo{}
}

func (c *Cron) JobInfo(name gen.Atom) (gen.CronJobInfo, error) {
	if c.ov.jobInfo != nil {
		return c.ov.jobInfo(name)
	}
	return gen.CronJobInfo{}, nil
}

func (c *Cron) Schedule(since time.Time, duration time.Duration) []gen.CronSchedule {
	if c.ov.schedule != nil {
		return c.ov.schedule(since, duration)
	}
	return nil
}

func (c *Cron) JobSchedule(job gen.Atom, since time.Time, duration time.Duration) ([]time.Time, error) {
	if c.ov.jobSchedule != nil {
		return c.ov.jobSchedule(job, since, duration)
	}
	return nil, nil
}
