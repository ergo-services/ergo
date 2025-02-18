package node

import (
	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
	"fmt"
	"runtime"
	"sync"
	"time"
)

const cronLogPrefix = "(cron) "

type cron struct {
	node gen.Node
	sync.RWMutex

	jobs  map[gen.Atom]*cronJob
	spool lib.QueueMPSC

	timer *time.Timer
	next  time.Time
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

	c.timer = time.AfterFunc(in, func() {
		if node.IsAlive() == false {
			// node terminated
			return
		}
		actionTime := time.Now().Truncate(time.Minute)
		for {

			item, ok := c.spool.Pop()
			if ok == false {
				// empty queue
				break
			}
			cj := item.(*cronJob)
			if cj.disable == true {
				continue
			}

			// check if actionTime is actually now:
			// - no time adjustment happened,
			// - no Day Light Saving happened
			nowInLocation := time.Now().In(cj.job.Location).Truncate(time.Minute)
			actionTimeInLocation := actionTime.In(cj.job.Location).Truncate(time.Minute)
			if nowInLocation != actionTimeInLocation {
				// do nothing
				c.node.Log().Debug(cronLogPrefix+"ignore job %s action time != now",
					cj.job.Name)
				return
			}

			// DO the job
			go func() {
				if lib.Recover() {
					defer func() {
						if r := recover(); r != nil {
							pc, fn, line, _ := runtime.Caller(2)
							c.node.Log().Panic("panic in cron action for job %s: %#v at %s[%s:%d]",
								cj.job.Name, r, runtime.FuncForPC(pc).Name(), fn, line)
						}
					}()
				}

				cj.last = actionTimeInLocation
				cj.lastErr = cj.job.Action.Do(cj.job.Name, c.node, actionTime)
				if cj.lastErr == nil {
					c.node.Log().Info(cronLogPrefix+"%s has completed (action time: %s)",
						cj.job.Name, cj.last)
					return
				}

				c.node.Log().Error(cronLogPrefix+"job %s has failed (action time: %s): %s", cj.job.Name, cj.last, cj.lastErr)

				if cj.job.Fallback.Enable == false {
					return
				}

				messageFallback := gen.MessageCronFallback{
					Job:  cj.job.Name,
					Tag:  cj.job.Fallback.Tag,
					Time: cj.last,
					Err:  cj.lastErr,
				}
				if err := c.node.Send(cj.job.Fallback.Name, messageFallback); err != nil {
					c.node.Log().Error(
						cronLogPrefix+"fallback process %s for %s is unreachable: %s",
						cj.job.Fallback.Name, cj.job.Name, err,
					)
				}
				c.node.Log().Info(cronLogPrefix+"sent fallback message to %s (job: %s)",
					cj.job.Fallback.Name, cj.job.Name)
			}()
		}

		now := time.Now()
		next := now.Add(time.Minute).Truncate(time.Minute)
		in := next.Sub(now)
		c.timer.Reset(in)
		c.schedule(next)
	})

	return c
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
	cj.disable = true
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
	cj.disable = false
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
	cj.disable = true
	return nil
}

func (c *cron) JobInfo(name gen.Atom) (gen.CronJobInfo, error) {
	var jobInfo gen.CronJobInfo
	c.RLock()
	defer c.RUnlock()
	v, found := c.jobs[name]
	if found == false {
		return jobInfo, gen.ErrUnknown
	}
	jobInfo.Name = v.job.Name
	jobInfo.Spec = v.job.Spec
	jobInfo.Location = v.job.Location.String()
	jobInfo.ActionInfo = v.job.Action.Info()
	jobInfo.Disabled = v.disable
	jobInfo.LastRun = v.last
	if v.lastErr != nil {
		jobInfo.LastErr = v.lastErr.Error()
	}
	jobInfo.Fallback = v.job.Fallback
	return jobInfo, nil
}

func (c *cron) Info() gen.CronInfo {
	var info gen.CronInfo
	c.RLock()
	defer c.RUnlock()

	info.Next = c.next
	info.Spool = []gen.Atom{}
	info.Jobs = []gen.CronJobInfo{}

	for item := c.spool.Item(); item != nil; item = item.Next() {
		cj := item.Value().(*cronJob)
		if cj.disable == true {
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
		jobInfo.Disabled = v.disable
		jobInfo.LastRun = v.last
		if v.lastErr != nil {
			jobInfo.LastErr = v.lastErr.Error()
		}
		jobInfo.Fallback = v.job.Fallback

		info.Jobs = append(info.Jobs, jobInfo)
	}

	return info
}

func (c *cron) terminate() {
	c.timer.Stop()
}

func (c *cron) schedule(next time.Time) {
	c.RLock()
	defer c.RUnlock()
	c.next = next
	for _, cj := range c.jobs {
		c.scheduleJob(cj)
	}
}

func (c *cron) scheduleJob(cj *cronJob) {
	// cron must be locked before invoking this func
	// to get rid of concurrent access to the c.next value

	next := c.next.In(cj.job.Location)
	if cj.disable == true {
		return
	}
	if cj.mask.IsRunAt(next) == false {
		return
	}
	c.spool.Push(cj)
}

// internal job

type cronJob struct {
	disable bool

	job gen.CronJob

	mask cronSpecMask

	last    time.Time
	lastErr error
}
