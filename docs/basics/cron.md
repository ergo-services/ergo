---
description: schedule tasks on a repetitive basis, such as daily, weekly, or monthly
---

# Cron

{% hint style="info" %}
Introduced in 3.1.0 (not yet released. available in `v310` branch)
{% endhint %}

Cron functionality is provided to enable periodic job execution in a node. Its implementation replicates the functionality of the [Cron service in Unix systems](https://en.wikipedia.org/wiki/Cron) and supports the [Crontab specification](cron.md#cron-specification) format.

To set up jobs at the node startup, use the `gen.NodeOptions.Cron` parameters. Each job is described using the `gen.CronJob`:

```go
type CronJob struct {
	// Name job name
	Name gen.Atom
	// Spec time spec in "crontab" format
	Spec string
	// Location defines timezone
	Location *time.Location
	// Action
	Action gen.CronAction
	// Fallback
	Fallback gen.ProcessFallback
}
```

Parameters:

* `Name` defines the name of the job. It must be unique among all cron jobs
* `Spec` sets the scheduling parameters for the job in [crontab format](cron.md#cron-specification)
* `Location` allows to set the time zone for the job's scheduling parameters. By default, the local time zone is used
* `Action` defines what needs to be run
* `Fallback` allows specifying a process to notify if the Action results in an error

The `Action` field has the interface type `gen.CronAction`. You can use the following ready-to-use implementations:

* **`gen.CreateCronActionMessage`**`(to any, priority gen.MessagePriority)` – creates an `Action` that sends a `gen.MessageCron` message to the specified process `to`. The `to` argument can be one of the following: `gen.Atom`, `gen.ProcessID`, `gen.PID`, or `gen.Alias`, referring to either a local process or a process on a remote node.
* **`gen.CreateCronActionSpawn`**`(factory gen.ProcessFactory, options gen.CronActionSpawnOptions)` – spawns a local process. The `options` argument specifies the startup parameters for the process.
* **`gen.CreateCronActionRemoteSpawn`**`(node gen.Atom, name gen.Atom, options gen.CronActionSpawnOptions)` – spawns a process on a remote node. The mechanism for spawning processes on a remote node is described in the [Remote Spawn Process](../networking/remote-spawn-process.md) section.

When using `CreateCronActionSpawn` or `CreateCronActionRemoteSpawn`, the following environment variables will be added to the spawned processes:

* `gen.CronEnvNodeName`
* `gen.CronEnvJobName`
* `gen.CronEnvJobActionTime`

### Example

In the example below, every day at 21:07 (Shanghai time), a `gen.MessageCron` message will be sent to the local process registered as `myweb1`. If the message cannot be delivered, a `gen.MessageCronFallback` message will be sent to the local process named `myweb2`:

```go
func main() {
	var options gen.NodeOptions
	// ...
	locationAsiaShanghai, _ := time.LoadLocation("Asia/Shanghai")
	options.Cron.Jobs = []gen.CronJob{
		gen.CronJob{Name: "job1",
			Spec:     "7 21 * * *",
			Action:   gen.CreateCronActionMessage(gen.Atom("myweb1"), 
							gen.MessagePriorityNormal),
			Location: locationAsiaShanghai,
			Fallback: gen.ProcessFallback{Enable: true, Name: "myweb2"},
		},
	}
	node, err := ergo.StartNode("cron@localhost", options)
	if err != nil {
		panic(err)
	}
	// ...
}
```

### `gen.Cron` interface

You can also manage jobs using the `gen.Cron` interface. It provides the following methods for this:

```go
type Cron interface {
	// AddJob adds a new job
	AddJob(job gen.CronJob) error
	// RemoveJob removes new job
	RemoveJob(name gen.Atom) error
	// EnableJob allows you to enable previously disabled job
	EnableJob(name gen.Atom) error
	// DisableJob disables job
	DisableJob(name gen.Atom) error

	// Info returns information about the jobs, spool of jobs for the next run
	Info() gen.CronInfo
	// JobInfo returns information for the given job
	JobInfo(name gen.Atom) (gen.CronJobInfo, error)

	// Schedule returns a list of jobs planned to be run for the given period
	Schedule(since time.Time, duration time.Duration) []gen.CronSchedule
	// JobSchedule returns a list of scheduled run times for the given job and period.
	JobSchedule(job Atom, since time.Time, duration time.Duration) ([]time.Time, error)
}

```

Access to the `gen.Cron` interface can be obtained using the `Cron` method of the `gen.Node` interface.

### Custom Action

You can also create your own Action. To do this, simply implement the `gen.CronAction` interface:

```go
type CronAction interface {
	Do(job gen.Atom, node gen.Node, action_time time.Time) error
	Info() string
}
```

It is worth noting that the `action_time` argument is passed in the time zone of the job, i.e., the one specified in the `gen.CronJob.Location` field when the job was created.

The `Info` method of the interface is used when calling `gen.Cron.Info()` to retrieve summary information about _Cron_ and its jobs.

### Cron specification

```
* * * * *
| | | | |                                                 allowed format
| | | | day-of-week (1–7) (Monday to Sunday)      *   d   d,d   d-d    dL    d#d
| | | month (1–12)                                *   d   d,d   d-d   */d
| | day-of-month (1–31)                           *   d   d,d   d-d   */d   d-d/d   L
| hour (0–23)                                     *   d   d,d   d-d   */d   d-d/d
minute (0–59)                                     *   d   d,d   d-d   */d   d-d/d
```

* `*`  represents "_all_". For example, using `* * * * *` will run every minute. Using `* * * * 1` will run every minute only on Monday.
* `-` allows specifying a range of values. For example, `15 23 * * 1-3` will trigger the job from Monday to Wednesday at 23:15
* `,` defines a sequence of values: `15,25,35 23 * * *` – this will trigger the job every day at 23:15, 23:25, and 23:35
* `/` used for step values: `*/5 3 * * *` this will trigger the job every 5 minutes during the hour starting from 3:00 (3:00, 3:05 ... 3:55). It can also be used with ranges: `21-37/5 17 * * *` - every day at 17:21, 17:26, 17:31 and 17:36
* `#` available for use only in the _day-of-week_ field in the format `day-of-week#occurrence`. It allows specifying constructs like `5#2`, which refers to the second Friday of the month
* `L` stands for "last". In the _day-of-month_ field, it specifies the last day of the month. In the _day-of-week_ field - allows specifying constructs such as "the last Friday" (`5L`) of a given month.&#x20;

You can also use the following macro definitions:

* `@hourly` - `1 * * * *` every hour
* `@daily` - `10 3 * * *` every day at 3:10
* `@monthly` - `20 4 1 * *` on day 1 of the month at 4:20
* `@weekly` - `30 5 * * 1` on Monday at 5:30

#### Examples

`1 19 * * 1#1,7L`   run at 19:01 every month on the first Monday and last Sunday&#x20;

`15 15 10-15/3 * *` run at 15:15 every month on the 10th and 13th

To view the schedule of your job, use the `JobSchedule` method of the `gen.Cron` interface. To view the schedule of all jobs, use the `Schedule` method of this interface.

### DST (daylight saving time) and Time-adjustment support

This implementation takes time changes into account – if a time adjustment occurs at the time of the job's scheduled execution and the new time does not match the job's schedule, the job will not be executed.

For example, if a job has the specification `0 2 * * *` (to run every day at 2:00), it will be skipped on March 30, 2025, because at 2:00 that day, the time will be moved forward by one hour (due to DST). After the end of DST on October 26, 2025, at 3:00, the time will be shifted back by one hour, but the job in the example will be executed only once.

