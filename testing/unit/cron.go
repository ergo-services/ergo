package unit

import (
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
)

// mockCron is the built-in gen.Cron behind Node().Cron(). AddJob/RemoveJob record the
// egress (check.AddCronJob / check.RemoveCronJob) and track jobs so Subject.FireCron can
// deliver the job's gen.MessageCron to the subject. There is no real scheduler: jobs fire
// only when the test calls FireCron.
type mockCron struct {
	node     *mockNode
	jobs     map[gen.Atom]gen.CronJob
	disabled map[gen.Atom]bool
}

func newMockCron(n *mockNode) *mockCron {
	return &mockCron{
		node:     n,
		jobs:     make(map[gen.Atom]gen.CronJob),
		disabled: make(map[gen.Atom]bool),
	}
}

var _ gen.Cron = (*mockCron)(nil)

func (c *mockCron) AddJob(job gen.CronJob) error {
	var err error
	if _, exists := c.jobs[job.Name]; exists {
		err = gen.ErrTaken
	} else {
		c.jobs[job.Name] = job
	}
	c.node.rec.Put(check.AddCronJob{From: c.node.subjectPID, Name: job.Name, Spec: job.Spec, Error: err})
	return err
}

func (c *mockCron) RemoveJob(name gen.Atom) error {
	var err error
	if _, exists := c.jobs[name]; exists == false {
		err = gen.ErrUnknown
	} else {
		delete(c.jobs, name)
		delete(c.disabled, name)
	}
	c.node.rec.Put(check.RemoveCronJob{From: c.node.subjectPID, Name: name, Error: err})
	return err
}

func (c *mockCron) EnableJob(name gen.Atom) error {
	if _, exists := c.jobs[name]; exists == false {
		return gen.ErrUnknown
	}
	delete(c.disabled, name)
	return nil
}

func (c *mockCron) DisableJob(name gen.Atom) error {
	if _, exists := c.jobs[name]; exists == false {
		return gen.ErrUnknown
	}
	c.disabled[name] = true
	return nil
}

func (c *mockCron) Info() gen.CronInfo {
	info := gen.CronInfo{}
	for name := range c.jobs {
		ji, _ := c.JobInfo(name)
		info.Jobs = append(info.Jobs, ji)
	}
	return info
}

func (c *mockCron) JobInfo(name gen.Atom) (gen.CronJobInfo, error) {
	job, exists := c.jobs[name]
	if exists == false {
		return gen.CronJobInfo{}, gen.ErrUnknown
	}
	info := gen.CronJobInfo{Name: name, Spec: job.Spec, Disabled: c.disabled[name]}
	if job.Action != nil {
		info.ActionInfo = job.Action.Info()
	}
	return info, nil
}

func (c *mockCron) Schedule(since time.Time, duration time.Duration) []gen.CronSchedule {
	return nil
}

func (c *mockCron) JobSchedule(name gen.Atom, since time.Time, duration time.Duration) ([]time.Time, error) {
	if _, exists := c.jobs[name]; exists == false {
		return nil, gen.ErrUnknown
	}
	return nil, nil
}

// fireable reports whether the job exists and is enabled (used by Subject.FireCron).
func (c *mockCron) fireable(name gen.Atom) bool {
	_, exists := c.jobs[name]
	return exists && c.disabled[name] == false
}
