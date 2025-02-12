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

type cronNetwork interface {
	GetNode(name gen.Atom) (gen.RemoteNode, error)
}

type cron struct {
	cronNode
	cronNetwork

	sync.RWMutex

	jobs  map[gen.Atom]*cronJob
	spool lib.QueueMPSC

	timer *time.Timer
	next  time.Time
}

func createCron(node cronNode, network cronNetwork) *cron {
	c := &cron{
		cronNode:    node,
		cronNetwork: network,
		jobs:        make(map[gen.Atom]*cronJob),
		spool:       lib.NewQueueMPSC(),
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

			// DO the job
			go cj.do(actionTime)
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
		job:         job,
		cronNode:    c.cronNode,
		cronNetwork: c.cronNetwork,
		mask:        mask,
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

func (c *cron) Info() gen.CronInfo {
	var info gen.CronInfo
	// TODO

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
	cronNode
	cronNetwork

	disable bool

	job  gen.CronJob
	mask cronSpecMask

	last    time.Time
	lastErr error
}

func (cj *cronJob) do(actionTime time.Time) {

	if cj.disable {
		return
	}

	// check if actionTime is actually now:
	// - no time adjustment happened,
	// - no Day Light Saving happened
	nowInLocation := time.Now().In(cj.job.Location).Truncate(time.Minute)
	actionTimeInLocation := actionTime.In(cj.job.Location).Truncate(time.Minute)
	if nowInLocation != actionTimeInLocation {
		// do nothing
		cj.Log().Debug("ignore job %s action time != now", cj.job.Name)
		return
	}

	switch action := cj.job.Action.(type) {
	case gen.CronActionMessage:
		message := gen.MessageCron{
			Node: cj.Name(),
			Job:  cj.job.Name,
			Time: actionTimeInLocation,
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
			Job:  cj.job.Name,
			Tag:  action.Fallback.Tag,
			Time: actionTimeInLocation,
			Err:  cj.lastErr,
		}
		cj.Send(action.Fallback.Name, messageFallback)

	case gen.CronActionSpawn:
		var err error
		var pid gen.PID

		if action.Register == "" {
			pid, err = cj.Spawn(action.ProcessFactory, action.ProcessOptions, action.Args...)
		} else {
			pid, err = cj.SpawnRegister(action.Register, action.ProcessFactory, action.ProcessOptions, action.Args...)
		}
		if err == nil {
			cj.lastErr = nil
			cj.Log().Info("(cron) %q has completed (spawned process %s)", cj.job.Name, pid)
			return
		}

		cj.lastErr = fmt.Errorf("unable to spawn process: %w", err)

		if action.Fallback.Enable == false {
			return
		}
		messageFallback := gen.MessageCronFallback{
			Job:  cj.job.Name,
			Tag:  action.Fallback.Tag,
			Time: actionTimeInLocation,
			Err:  cj.lastErr,
		}
		cj.Send(action.Fallback.Name, messageFallback)

	case gen.CronActionRemoteSpawn:

		remote, err := cj.GetNode(action.Node)
		if err == nil {
			var e error
			var pid gen.PID

			if action.Register == "" {
				pid, e = remote.Spawn(action.Name, action.ProcessOptions, action.Args...)
			} else {
				pid, e = remote.SpawnRegister(action.Register, action.Name, action.ProcessOptions, action.Args...)
			}

			if e == nil {
				cj.lastErr = nil
				cj.Log().Info("(cron) %q has completed (spawned remote process %s on %s)", cj.job.Name, action.Node, pid)
				return
			}
			err = e
		}

		cj.lastErr = fmt.Errorf("unable to spawn remote process %s on %s: %w", action.Name, action.Node, err)

		if action.Fallback.Enable == false {
			return
		}
		messageFallback := gen.MessageCronFallback{
			Job:  cj.job.Name,
			Tag:  action.Fallback.Tag,
			Time: actionTimeInLocation,
			Err:  cj.lastErr,
		}
		cj.Send(action.Fallback.Name, messageFallback)
	}
}
