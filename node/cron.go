package node

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

const cronLogPrefix = "(cron) "

type cron struct {
	node gen.Node
	sync.Mutex

	jobs  map[gen.Atom]*cronJob
	spool lib.QueueMPSC

	timer *time.Timer
	next  time.Time
}

// internal job

type cronJob struct {
	disable atomic.Bool

	job gen.CronJob

	mask cronSpecMask

	last atomic.Pointer[cronLast]
}

// last completed (or in-progress) action of a job, stored as one atomic value so
// readers always see a consistent time/error pair without taking the cron lock.
type cronLast struct {
	time time.Time
	err  error
}

func createCron(node gen.Node) *cron {
	c := &cron{
		node:  node,
		jobs:  make(map[gen.Atom]*cronJob),
		spool: lib.NewQueueMPSC(),
	}

	// run every minute
	now := time.Now()
	next := now.Add(time.Minute).Truncate(time.Minute)
	in := next.Sub(now)

	// assign under the lock: time.AfterFunc starts the timer immediately, so if it
	// fires before the assignment completes the callback would race it. The
	// callback takes the same lock before touching c.timer, so it cannot proceed
	// until the assignment is done (same pattern as lib/flusher).
	c.Lock()
	c.next = next
	c.timer = time.AfterFunc(in, func() {
		if node.IsAlive() == false {
			// node terminated
			return
		}
		c.tick(time.Now().Truncate(time.Minute))

		now := time.Now()
		next := now.Add(time.Minute).Truncate(time.Minute)
		in := next.Sub(now)
		c.Lock()
		c.timer.Reset(in)
		c.Unlock()
		c.schedule(next)
	})
	c.Unlock()

	return c
}

// tick drains the spool for actionTime and runs each due job's action in its own goroutine.
func (c *cron) tick(actionTime time.Time) {
	for {
		item, ok := c.spool.Pop()
		if ok == false {
			// empty queue
			break
		}
		cj := item.(*cronJob)
		if cj.disable.Load() {
			continue
		}

		// check if actionTime is actually now:
		// - no time adjustment happened,
		// - no Day Light Saving happened
		nowInLocation := time.Now().In(cj.job.Location).Truncate(time.Minute)
		actionTimeInLocation := actionTime.In(cj.job.Location).Truncate(time.Minute)
		if nowInLocation != actionTimeInLocation {
			c.node.Log().Debug(cronLogPrefix+"ignore job %s action time != now",
				cj.job.Name)
			continue
		}

		// DO the job
		go func() {
			if cj.disable.Load() {
				// disabled between the spool pop and the action launch
				return
			}
			if lib.Recover() {
				defer func() {
					if r := recover(); r != nil {
						pc, fn, line, _ := runtime.Caller(2)
						c.node.Log().Panic("panic in cron action for job %s: %#v at %s[%s:%d]",
							cj.job.Name, r, runtime.FuncForPC(pc).Name(), fn, line)
					}
				}()
			}

			cj.last.Store(&cronLast{time: actionTimeInLocation})
			err := cj.job.Action.Do(cj.job.Name, c.node, actionTime)
			cj.last.Store(&cronLast{time: actionTimeInLocation, err: err})
			if err == nil {
				c.node.Log().Info(cronLogPrefix+"%s has completed (action time: %s)",
					cj.job.Name, actionTimeInLocation)
				return
			}

			c.node.Log().Error(cronLogPrefix+"job %s has failed (action time: %s): %s", cj.job.Name, actionTimeInLocation, err)

			if cj.job.Fallback.Enable == false {
				return
			}

			messageFallback := gen.MessageCronFallback{
				Job:  cj.job.Name,
				Tag:  cj.job.Fallback.Tag,
				Time: actionTimeInLocation,
				Err:  err,
			}
			if sendErr := c.node.Send(cj.job.Fallback.Name, messageFallback); sendErr != nil {
				c.node.Log().Error(
					cronLogPrefix+"fallback process %s for %s is unreachable: %s",
					cj.job.Fallback.Name, cj.job.Name, sendErr,
				)
				return
			}

			c.node.Log().Info(cronLogPrefix+"sent fallback message to %s (job: %s)",
				cj.job.Fallback.Name, cj.job.Name)
		}()
	}
}

func (c *cron) AddJob(job gen.CronJob) error {
	if job.Name == "" {
		return fmt.Errorf("empty job name")
	}

	if job.Action == nil {
		return fmt.Errorf("empty action")
	}

	if job.Location == nil {
		job.Location = time.Local
	}

	mask, err := cronParseSpec(job)
	if err != nil {
		return err
	}

	cj := &cronJob{
		job:  job,
		mask: mask,
	}

	c.Lock()
	if _, exist := c.jobs[job.Name]; exist {
		c.Unlock()
		return gen.ErrTaken
	}

	c.jobs[job.Name] = cj
	c.scheduleJob(cj)
	c.Unlock()

	return nil
}

func (c *cron) RemoveJob(name gen.Atom) error {
	c.Lock()
	defer c.Unlock()
	cj, exist := c.jobs[name]
	if exist == false {
		return gen.ErrUnknown
	}
	cj.disable.Store(true)
	delete(c.jobs, name)
	return nil
}

func (c *cron) EnableJob(name gen.Atom) error {
	c.Lock()
	defer c.Unlock()
	cj, exist := c.jobs[name]
	if exist == false {
		return gen.ErrUnknown
	}
	cj.disable.Store(false)
	c.scheduleJob(cj)
	return nil
}

func (c *cron) DisableJob(name gen.Atom) error {
	c.Lock()
	defer c.Unlock()
	cj, exist := c.jobs[name]
	if exist == false {
		return gen.ErrUnknown
	}
	cj.disable.Store(true)
	return nil
}

func (c *cron) JobInfo(name gen.Atom) (gen.CronJobInfo, error) {
	var jobInfo gen.CronJobInfo
	c.Lock()
	defer c.Unlock()
	v, found := c.jobs[name]
	if found == false {
		return jobInfo, gen.ErrUnknown
	}
	jobInfo.Name = v.job.Name
	jobInfo.Spec = v.job.Spec
	jobInfo.Location = v.job.Location.String()
	jobInfo.ActionInfo = v.job.Action.Info()
	jobInfo.Disabled = v.disable.Load()
	if l := v.last.Load(); l != nil {
		jobInfo.LastRun = l.time
		if l.err != nil {
			jobInfo.LastErr = l.err.Error()
		}
	}
	jobInfo.Fallback = v.job.Fallback
	return jobInfo, nil
}

func (c *cron) Info() gen.CronInfo {
	var info gen.CronInfo

	c.Lock()
	defer c.Unlock()

	info.Next = c.next
	info.Spool = []gen.Atom{}
	info.Jobs = []gen.CronJobInfo{}

	for _, cj := range c.jobs {
		if cj.disable.Load() {
			continue
		}
		if cj.mask.IsRunAt(c.next.In(cj.job.Location)) == false {
			continue
		}
		info.Spool = append(info.Spool, cj.job.Name)
	}

	for _, v := range c.jobs {
		var jobInfo gen.CronJobInfo
		jobInfo.Name = v.job.Name
		jobInfo.Spec = v.job.Spec
		jobInfo.Location = v.job.Location.String()
		jobInfo.ActionInfo = v.job.Action.Info()
		jobInfo.Disabled = v.disable.Load()
		if l := v.last.Load(); l != nil {
			jobInfo.LastRun = l.time
			if l.err != nil {
				jobInfo.LastErr = l.err.Error()
			}
		}
		jobInfo.Fallback = v.job.Fallback

		info.Jobs = append(info.Jobs, jobInfo)
	}

	return info
}

func (c *cron) Schedule(since time.Time, period time.Duration) []gen.CronSchedule {
	// snapshot the jobs (name, immutable mask, location) under the lock, then run
	// the O(period) computation lock-free so a large period cannot block the tick.
	type entry struct {
		name gen.Atom
		mask cronSpecMask
		loc  *time.Location
	}
	c.Lock()
	snapshot := make([]entry, 0, len(c.jobs))
	for _, v := range c.jobs {
		snapshot = append(snapshot, entry{name: v.job.Name, mask: v.mask, loc: v.job.Location})
	}
	c.Unlock()

	var schedule []gen.CronSchedule
	start := since.Truncate(time.Minute)
	end := start.Add(period)

	for now := start; now.Before(end); now = now.Add(time.Minute) {
		cronSchedule := gen.CronSchedule{
			Time: now,
		}
		for _, e := range snapshot {
			if e.mask.IsRunAt(now.In(e.loc)) == false {
				continue
			}
			cronSchedule.Jobs = append(cronSchedule.Jobs, e.name)
		}

		if len(cronSchedule.Jobs) > 0 {
			schedule = append(schedule, cronSchedule)
		}
	}

	return schedule
}

func (c *cron) JobSchedule(job gen.Atom, since time.Time, period time.Duration) ([]time.Time, error) {
	c.Lock()
	v, found := c.jobs[job]
	if found == false {
		c.Unlock()
		return nil, gen.ErrUnknown
	}
	mask := v.mask
	loc := v.job.Location
	c.Unlock()

	var schedule []time.Time
	start := since.Truncate(time.Minute)
	end := start.Add(period)

	for now := start; now.Before(end); now = now.Add(time.Minute) {
		if mask.IsRunAt(now.In(loc)) == false {
			continue
		}
		schedule = append(schedule, now)
	}

	return schedule, nil
}

func (c *cron) terminate() {
	c.Lock()
	defer c.Unlock()
	if c.timer == nil {
		return
	}
	c.timer.Stop()
}

func (c *cron) schedule(next time.Time) {
	c.Lock()
	defer c.Unlock()
	c.next = next
	for _, cj := range c.jobs {
		c.scheduleJob(cj)
	}
}

func (c *cron) scheduleJob(cj *cronJob) {
	// cron must be locked before invoking this func
	// to get rid of concurrent access to the c.next value

	next := c.next.In(cj.job.Location)
	if cj.disable.Load() {
		return
	}
	if cj.mask.IsRunAt(next) == false {
		return
	}
	c.spool.Push(cj)
}
