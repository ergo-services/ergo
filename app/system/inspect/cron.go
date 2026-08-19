package inspect

import (
	"time"

	"ergo.services/ergo/gen"
)

const (
	cronScheduleDuration = 24 * time.Hour
	cronScheduleLimit    = 1000
)

// responseCronSchedule previews the upcoming firings. A single job is normalised
// into the same shape as the whole scheduler, so the caller reads one form.
func (i *inspect) responseCronSchedule(request RequestGetCronSchedule) ResponseGetCronSchedule {
	since := request.Since
	if since.IsZero() {
		since = time.Now()
	}
	duration := request.Duration
	if duration <= 0 {
		duration = cronScheduleDuration
	}
	limit := request.Limit
	if limit < 1 {
		limit = cronScheduleLimit
	}

	cron := i.Node().Cron()

	var schedule []gen.CronSchedule
	if request.Job == "" {
		schedule = cron.Schedule(since, duration)
	} else {
		times, err := cron.JobSchedule(request.Job, since, duration)
		if err != nil {
			return ResponseGetCronSchedule{Error: err}
		}
		for _, at := range times {
			schedule = append(schedule, gen.CronSchedule{
				Time: at,
				Jobs: []gen.Atom{request.Job},
			})
		}
	}

	if len(schedule) > limit {
		return ResponseGetCronSchedule{Schedule: schedule[:limit], Truncated: true}
	}
	return ResponseGetCronSchedule{Schedule: schedule}
}

func (i *inspect) responseCronInfo(request RequestGetCronInfo) ResponseGetCronInfo {
	cron := i.Node().Cron()

	if request.Job != "" {
		job, err := cron.JobInfo(request.Job)
		if err != nil {
			return ResponseGetCronInfo{Error: err}
		}
		return ResponseGetCronInfo{Jobs: []gen.CronJobInfo{job}}
	}

	info := cron.Info()
	return ResponseGetCronInfo{
		Next:  info.Next,
		Spool: info.Spool,
		Jobs:  info.Jobs,
	}
}
