package node

import (
	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
	"fmt"
	"sync"
	"time"
)

type cronNode interface {
	Name() gen.Atom
	Log() gen.Log
	IsAlive() bool

	Send(to any, message any) error
	Spawn(factory gen.ProcessFactory, options gen.ProcessOptions, args ...any) (gen.PID, error)
	SpawnRegister(register gen.Atom, factory gen.ProcessFactory, options gen.ProcessOptions, args ...any) (gen.PID, error)
}

type cron struct {
	cronNode

	sync.RWMutex

	jobs  map[gen.Atom]*cronJob
	spool lib.QueueMPSC

	timer *time.Timer
}

func createCron(node cronNode) *cron {
	c := &cron{
		cronNode: node,
		jobs:     make(map[gen.Atom]*cronJob),
		spool:    lib.NewQueueMPSC(),
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
				break
			}
			cj := item.(*cronJob)
			if cj.disable == true {
				continue
			}

			// DO the job
			go cj.do(actionTime)
		}

		now := time.Now()
		next := now.Add(time.Minute).Truncate(time.Minute)
		in := next.Sub(now)
		c.timer.Reset(in)
		c.schedule()
	})

	return c
}

func (c *cron) AddJob(job gen.CronJob) error {
	if job.Name == "" {
		return fmt.Errorf("empty job name")
	}

	switch ac := job.Action.(type) {
	case gen.CronActionMessage:
		if ac.Process.Name == "" {
			return fmt.Errorf("incorrect value in gen.CronActionMessage.Process")
		}

	case gen.CronActionSpawn:
		if ac.ProcessFactory == nil {
			return fmt.Errorf("nil value in gen.CronActionSpawn.ProcessFactory")
		}
	case gen.CronActionRemoteSpawn:
		if ac.Node == "" {
			return fmt.Errorf("empty Node value in gen.CronActionRemoteSpawn")
		}
		if ac.Name == "" {
			return fmt.Errorf("empty Name value in gen.CronActionRemoteSpawn")
		}
	default:
		return fmt.Errorf("unknown action type %T", job.Action)
	}

	if job.Location == nil {
		job.Location = time.Local
	}

	mask, err := cronParseSpec(job)
	if err != nil {
		return err
	}

	cj := &cronJob{
		job:      job,
		cronNode: c.cronNode,
		mask:     mask,
	}

	c.Lock()
	if _, exist := c.jobs[job.Name]; exist {
		c.Unlock()
		return gen.ErrTaken
	}

	c.jobs[job.Name] = cj
	c.Unlock()

	c.scheduleJob(cj)
	return nil
}

func (c *cron) RemoveJob(name gen.Atom) error {

	return nil
}

func (c *cron) EnableJob(name gen.Atom) error {

	return nil
}

func (c *cron) DisableJob(name gen.Atom) error {
	return nil
}

func (c *cron) Info() gen.CronInfo {
	var info gen.CronInfo

	return info
}

func (c *cron) terminate() {
	c.timer.Stop()
}

func (c *cron) schedule() {
	c.RLock()
	defer c.RUnlock()
	for _, cj := range c.jobs {
		c.scheduleJob(cj)
	}
}

func (c *cron) scheduleJob(cj *cronJob) {
	if cj.disable == true {
		return
	}
	now := time.Now()
	// spoolNextRun := now.Add(time.Minute).Truncate(time.Minute)
	jobNextRun := now

	for _, cm := range cj.mask {
		if cm.IsRunAt(now) == false {
			continue
		}

		c.spool.Push(cj)
		break
	}
	cj.next = jobNextRun
}

// internal job

type cronJob struct {
	cronNode

	disable bool

	job  gen.CronJob
	mask []cronMask

	next    time.Time
	last    time.Time
	lastErr error
}

func (cj *cronJob) do(actionTime time.Time) {
	if cj.disable {
		return
	}

	// check if actionTime is actually now
	// no time adjustment happened,
	// no Day Light Saving happened
	now := time.Now().In(cj.job.Location).Truncate(time.Minute)
	if now != actionTime {
		// do nothing
		cj.Log().Debug("ignore job %s action time != now", cj.job.Name)
		return
	}

	switch action := cj.job.Action.(type) {
	case gen.CronActionMessage:
		message := gen.MessageCron{
			Node: cj.Name(),
			Job:  cj.job.Name,
			Time: actionTime,
		}

		err := cj.Send(action.Process, message)
		cj.last = actionTime
		if err == nil {
			cj.lastErr = nil
			cj.Log().Info("(cron) %q has completed (sent message to: %s)",
				message.Job, action.Process)
			return
		}
		cj.lastErr = fmt.Errorf("unable to send cron message: %w", err)

		if action.Fallback.Enable == false {
			return
		}
		messageFallback := gen.MessageCronFallback{
			Job:  message.Job,
			Tag:  action.Fallback.Tag,
			Time: message.Time,
			Err:  cj.lastErr,
		}
		cj.Send(action.Fallback.Name, messageFallback)
		return

	case gen.CronActionSpawn:
		// TODO
	case gen.CronActionRemoteSpawn:
		// TODO
	}
}
