---
description: Schedule tasks on a repetitive basis
---

# Cron

Applications often need tasks to run periodically. Generate a daily report at midnight. Clean up expired sessions every hour. Send weekly summary emails. Poll an external API every five minutes.

You could implement this yourself - spawn a process that sleeps, wakes up, performs the task, and sleeps again. But then you're managing wake times, handling timezone changes, accounting for daylight saving time transitions, and ensuring the scheduler itself stays alive. The scheduling logic becomes scattered across your application.

Cron provides scheduled task execution as a framework service. You declare what should run and when using the familiar crontab syntax. The framework handles timing, execution, and all the edge cases around time-based scheduling.

## How It Works

Every minute, the cron system wakes up and evaluates all job specifications against the current time. Jobs whose specifications match the current minute are queued for execution. Each queued job then runs in its own goroutine.

This design is stateless - no pre-calculated schedules, no complex data structures to maintain. When you add a job, it participates in the next evaluation. When you remove a job, it stops participating. Timezone and daylight saving time transitions are handled naturally because each evaluation uses current time rules.

The stateless approach has implications. Multiple executions of the same job can run concurrently if the job takes longer than its interval. A job scheduled every minute that takes two minutes to complete will have two instances running simultaneously. If your job can't handle concurrent execution, implement serialization in the action itself - for example, send a message to a named process that processes requests sequentially.

## Defining Jobs

A job specification declares what should run and when:

```go
job := gen.CronJob{
    Name:     "daily_report",
    Spec:     "0 0 * * *",
    Location: time.UTC,
    Action:   gen.CreateCronActionMessage(gen.Atom("reporter"), gen.MessagePriorityNormal),
}
```

`CreateCronActionMessage` is generic over `gen.Atom | gen.ProcessID | gen.PID | gen.Alias`, and a bare `"reporter"` infers as `string`, which is not in that set. Write the conversion out: `gen.Atom("reporter")`.

The `Name` identifies the job uniquely within the node. The `Spec` uses crontab format to define the schedule. The `Location` specifies which timezone to use when interpreting the schedule. The `Action` defines what happens when the schedule triggers.

Optionally, `Fallback` names a process to notify when the action returns an error, which gives scheduled tasks one place to handle failures:

```go
Fallback: gen.ProcessFallback{
    Enable: true,              // required: without it nothing is sent
    Name:   "cron_supervisor",
    Tag:    "daily_report",
},
```

`Enable` is the part that is easy to miss. The field is a `gen.ProcessFallback` borrowed from mailbox-overflow configuration, and the cron path returns early unless `Enable` is true - so filling in `Name` alone gives you silence, with the failure visible only as an error line in the log. The notified process receives `gen.MessageCronFallback{Job, Tag, Time, Err}`, and if it is unreachable that is logged too.

## Actions

Actions define what happens when a job runs.

The simplest action sends a message. The job triggers, the cron system sends `gen.MessageCron` to the specified process, and the process handles it through normal message processing. This integrates cleanly with the actor model - the scheduled work happens inside an actor's message handler.

```go
action := gen.CreateCronActionMessage(gen.Atom("worker"), gen.MessagePriorityNormal)
```

For work that needs isolation per execution, spawn a process. Each time the job triggers, a fresh process spawns, performs the work, and terminates. If one execution crashes, the next starts clean. The spawned process receives environment variables identifying which job spawned it and when (`gen.CronEnvNodeName`, `gen.CronEnvJobName`, `gen.CronEnvJobActionTime`).

```go
action := gen.CreateCronActionSpawn(createReportWorker, gen.CronActionSpawnOptions{})
```

For distributed systems, spawn on a remote node. A job on the coordinator can trigger work on data nodes. The remote node must have enabled spawn permissions for the process name. This pattern centralizes scheduling while distributing execution.

```go
action := gen.CreateCronActionRemoteSpawn("worker@datanode", "report_worker", gen.CronActionSpawnOptions{})
```

Custom actions implement the `gen.CronAction` interface. The `Do` method receives the job name, node reference, and execution time in the job's timezone. Return an error to trigger fallback handling.

## Crontab Format

Five fields, in the crontab order: minute, hour, day-of-month, month, day-of-week. Close to standard crontab, with two deliberate differences worth knowing before you write a spec.

**Day-of-week runs 1 to 7, and Sunday is 7.** A `0` is rejected - `"* * * * 0"` fails with `incorrect value: 0`. Matching remaps the runtime's Sunday to 7, so `7` is how you write it.

**Step syntax is not accepted in the day-of-week field.** `*/15 * * * *` is fine in the minute field; `0 0 * * */2` is not, and fails with `incorrect value: */2`. That field takes `*`, a number, a range `d-d`, `dL` for the last such weekday of the month, or `d#n` for the n-th.

Common patterns:
- `0 * * * *` - Every hour
- `0 0 * * *` - Every day at midnight
- `*/15 * * * *` - Every 15 minutes
- `0 9-17 * * 1-5` - Every hour from 9-5 on weekdays
- `0 0 1 * *` - First day of each month
- `0 0 * * 5#2` - Second Friday of each month
- `0 0 L * *` - Last day of each month
- `0 0 * * 7` - Every Sunday

Four macros are recognised, and they are **not** on the hour - they carry a deliberate offset so that unrelated jobs do not all fire on the same tick:

| Macro | Expands to | Fires |
|-------|------------|-------|
| `@hourly` | `1 * * * *` | at minute 1 of every hour |
| `@daily` | `10 3 * * *` | 03:10 every day |
| `@weekly` | `30 5 * * 1` | Monday 05:30 |
| `@monthly` | `20 4 1 * *` | the 1st at 04:20 |

There is no `@yearly`, `@annually`, `@midnight` or `@reboot`: those fail with `incorrect cron spec format`. Write the five fields out if you need midnight exactly.

## Managing Jobs

Jobs can be defined at node startup in `gen.NodeOptions.Cron.Jobs`, or managed dynamically through the `gen.Cron` interface.

Add jobs with `AddJob`. Remove them with `RemoveJob`. Temporarily disable with `DisableJob` (useful for maintenance windows), and resume with `EnableJob`. Query status with `Info` and `JobInfo`, which show execution history and errors.

The `Schedule` and `JobSchedule` methods preview upcoming executions. Since the implementation evaluates specifications on-demand rather than maintaining pre-calculated schedules, these methods perform the same evaluation logic for a future time range. Use them to verify your crontab specs are correct or to detect scheduling conflicts.

## Timezone Handling

Each job has its own timezone. A job with `Location: time.UTC` scheduled for midnight runs at UTC midnight. A job with a New York timezone runs at New York midnight. The physical location of the node doesn't matter - jobs run in their configured timezone.

This matters for distributed systems where jobs serve different regions. One node can run jobs for multiple timezones. A cleanup job for European users runs at European midnight. A report job for Asian users runs at Asian business hours. Same node, different timezones, correct local timing.

## Daylight Saving Time

Timezone transitions are handled carefully.

When clocks spring forward, an hour disappears. A job scheduled for 02:30 simply does not run on that date: the spec is evaluated against local time, and that local minute never occurs, so nothing matches. It is skipped rather than run an hour early or late.

Separately from the transitions, the scheduler checks that the minute it is firing for is really the current minute, and drops the job if the clock moved between scheduling and firing - an NTP step or a suspended process, rather than a timezone rule.

When clocks fall back, an hour repeats, and a job scheduled inside it matches on both passes - `30 1 * * *` in `America/New_York` matches at 01:30 EDT and again an hour later at 01:30 EST. It runs **once**: before running, the scheduler compares the action's local wall clock minute against the last one it ran, and a repeat is skipped. Comparing instants would not catch this, since the two passes are a genuine hour apart.

`Cron().JobSchedule(name, since, period)` previews the same decision, so a transition day reports one run rather than two, matching what the scheduler will actually do.

The action can still tell where it is in time: a message action receives `gen.MessageCron` with a `Time` field, and a spawn action gets the same instant in its environment as `gen.CronEnvJobActionTime`.

## Error Handling

If a job action returns an error and the job has a configured fallback, the system sends `gen.MessageCronFallback` to the fallback process. The message includes the job name, execution time, error, and an optional tag for identifying the job source.

This allows centralizing monitoring of failed scheduled tasks. A single fallback process can receive failures from all jobs, log them, send alerts, or take corrective action.

For complete crontab specification syntax and additional examples, refer to the `gen.Cron` interface documentation in the code.
