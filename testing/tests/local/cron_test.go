package local

import (
	"errors"
	"testing"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

func cronJobNames(jobs []gen.CronJobInfo) map[gen.Atom]gen.CronJobInfo {
	m := make(map[gen.Atom]gen.CronJobInfo, len(jobs))
	for _, j := range jobs {
		m[j.Name] = j
	}
	return m
}

// TestLocalCronManagement: the node cron scheduler's job lifecycle: AddJob (with
// ErrTaken on a duplicate), Info/JobInfo reflecting registered jobs, Disable/Enable
// toggling the Disabled flag (ErrUnknown for an unknown job), and RemoveJob
// (ErrUnknown afterwards).
func TestLocalCronManagement(t *testing.T) {
	s := stage.New(t)
	n := s.Node("n")
	cron := n.Native().Cron()

	action := gen.CreateCronActionMessage(gen.Atom("target"), gen.MessagePriorityNormal)
	check.NoError(t, cron.AddJob(gen.CronJob{Name: "j1", Spec: "*/5 * * * *", Action: action}))

	// duplicate name is rejected
	check.True(t, errors.Is(cron.AddJob(gen.CronJob{Name: "j1", Spec: "* * * * *", Action: action}), gen.ErrTaken))

	// Info / JobInfo reflect the registered job
	jobs := cronJobNames(cron.Info().Jobs)
	ji, ok := jobs["j1"]
	check.True(t, ok)
	check.Equal(t, "*/5 * * * *", ji.Spec)
	check.Equal(t, false, ji.Disabled)

	info, err := cron.JobInfo("j1")
	check.NoError(t, err)
	check.Equal(t, gen.Atom("j1"), info.Name)
	check.True(t, info.ActionInfo != "")

	// disable / enable toggles the flag
	check.NoError(t, cron.DisableJob("j1"))
	info, _ = cron.JobInfo("j1")
	check.Equal(t, true, info.Disabled)
	check.NoError(t, cron.EnableJob("j1"))
	info, _ = cron.JobInfo("j1")
	check.Equal(t, false, info.Disabled)

	// unknown-job operations return ErrUnknown
	check.True(t, errors.Is(cron.DisableJob("nope"), gen.ErrUnknown))
	check.True(t, errors.Is(cron.EnableJob("nope"), gen.ErrUnknown))
	check.True(t, errors.Is(cron.RemoveJob("nope"), gen.ErrUnknown))
	_, err = cron.JobInfo("nope")
	check.True(t, errors.Is(err, gen.ErrUnknown))

	// remove drops the job
	check.NoError(t, cron.RemoveJob("j1"))
	_, err = cron.JobInfo("j1")
	check.True(t, errors.Is(err, gen.ErrUnknown))
	_, present := cronJobNames(cron.Info().Jobs)["j1"]
	check.Equal(t, false, present)
}

// TestLocalCronSchedule: the scheduler computes the planned run times for a spec
// over a window deterministically, without waiting for live firing. A job at
// minute 30 of every hour, previewed over three hours from a fixed instant, yields
// exactly 00:30, 01:30, 02:30. JobSchedule on an unknown job is ErrUnknown.
func TestLocalCronSchedule(t *testing.T) {
	s := stage.New(t)
	n := s.Node("n")
	cron := n.Native().Cron()

	action := gen.CreateCronActionMessage(gen.Atom("target"), gen.MessagePriorityNormal)
	check.NoError(t, cron.AddJob(gen.CronJob{Name: "clock", Spec: "30 * * * *", Action: action}))

	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	times, err := cron.JobSchedule("clock", since, 3*time.Hour)
	check.NoError(t, err)
	check.Equal(t, 3, len(times))
	for i, want := range []time.Time{
		time.Date(2026, 1, 1, 0, 30, 0, 0, time.UTC),
		time.Date(2026, 1, 1, 1, 30, 0, 0, time.UTC),
		time.Date(2026, 1, 1, 2, 30, 0, 0, time.UTC),
	} {
		check.True(t, times[i].Equal(want))
	}

	// the grouped schedule includes the job within the window
	found := false
	for _, sc := range cron.Schedule(since, 3*time.Hour) {
		for _, j := range sc.Jobs {
			if j == "clock" {
				found = true
			}
		}
	}
	check.True(t, found)

	_, err = cron.JobSchedule("nope", since, time.Hour)
	check.True(t, errors.Is(err, gen.ErrUnknown))
}
