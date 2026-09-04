package node

import (
	"testing"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
	"ergo.services/ergo/testing/mock"
)

type dstAction struct{}

func (dstAction) Do(job gen.Atom, node gen.Node, actionTime time.Time) error { return nil }
func (dstAction) Info() string                                               { return "dst" }

func dstCron(t *testing.T, spec string, loc *time.Location) *cron {
	t.Helper()
	node := mock.NewNode()
	node.OnIsAlive(func() bool { return false })
	c := createCron(node)
	t.Cleanup(c.terminate)

	if err := c.AddJob(gen.CronJob{
		Name:     "job",
		Spec:     spec,
		Location: loc,
		Action:   dstAction{},
	}); err != nil {
		t.Fatal(err)
	}
	return c
}

// TestCronFallBackRunsOnce covers the daylight saving fall-back, where a local
// hour repeats: "30 1 * * *" matches at 01:30 -0400 and again an hour later at
// 01:30 -0500. The job must run once for the day.
func TestCronFallBackRunsOnce(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("no tzdata")
	}

	first := time.Date(2026, 11, 1, 5, 30, 0, 0, time.UTC)  // 01:30 EDT
	second := time.Date(2026, 11, 1, 6, 30, 0, 0, time.UTC) // 01:30 EST

	c := dstCron(t, "30 1 * * *", ny)
	cj := c.jobs["job"]

	// first pass: due, and nothing has run yet
	c.spool = lib.NewQueueMPSC()
	c.schedule(first)
	if c.spool.Len() != 1 {
		t.Fatalf("first pass: expected the job due, spool holds %d", c.spool.Len())
	}
	// the tick stores the action time as the last run
	cj.last.Store(&cronLast{time: first.In(ny)})

	// second pass: the spec matches again, but the local minute already ran
	c.spool = lib.NewQueueMPSC()
	c.schedule(second)
	if c.spool.Len() != 1 {
		t.Fatalf("second pass: expected the spec to match again, spool holds %d", c.spool.Len())
	}
	if l := cj.last.Load(); sameWallMinute(l.time, second.In(ny)) == false {
		t.Fatalf("second pass should be the same wall clock minute as the first: %s vs %s",
			l.time, second.In(ny))
	}
	// tick drains it and must not run it: same wall clock minute as the last run
	c.tick(second)
	if l := cj.last.Load(); l.time.Equal(first.In(ny)) == false {
		t.Errorf("the second pass ran: last run moved to %s", l.time)
	}
}

// TestCronSpringForwardSkips covers the other side: the local hour disappears, so
// a job inside it never matches and simply does not run that day.
func TestCronSpringForwardSkips(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("no tzdata")
	}

	c := dstCron(t, "30 2 * * *", ny)

	// 2026-03-08: 02:00 EST jumps straight to 03:00 EDT, so 02:30 does not exist
	list, err := c.JobSchedule("job", time.Date(2026, 3, 8, 0, 0, 0, 0, ny), 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	for _, at := range list {
		if h := at.In(ny).Hour(); h == 2 {
			t.Errorf("02:30 local does not exist on the spring-forward date, got %s", at.In(ny))
		}
	}
	t.Logf("spring-forward day scheduled %d run(s) for '30 2 * * *'", len(list))
}

// TestCronJobScheduleMatchesTick checks the preview agrees with what the tick will
// do across the fall-back: one entry, not two.
func TestCronJobScheduleMatchesTick(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("no tzdata")
	}

	c := dstCron(t, "30 1 * * *", ny)

	from := time.Date(2026, 11, 1, 0, 0, 0, 0, ny)
	list, err := c.JobSchedule("job", from, 8*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	for _, at := range list {
		t.Logf("scheduled %s (UTC %s)", at.In(ny).Format("15:04 -0700"), at.UTC().Format("15:04"))
	}
	if len(list) != 1 {
		t.Errorf("expected one run across the fall-back, got %d", len(list))
	}
}

// TestCronAdjustmentGuard checks the guard the fall-back fix relies on: an action
// time that disagrees with the wall clock is dropped rather than run at the wrong
// moment.
func TestCronAdjustmentGuard(t *testing.T) {
	c := dstCron(t, "* * * * *", time.UTC)
	cj := c.jobs["job"]

	stale := time.Now().UTC().Add(-90 * time.Minute).Truncate(time.Minute)
	c.schedule(stale)
	c.tick(stale)
	if l := cj.last.Load(); l != nil {
		t.Errorf("a stale action time was run anyway: %s", l.time)
	}

	now := time.Now().UTC().Truncate(time.Minute)
	c.schedule(now)
	c.tick(now)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && cj.last.Load() == nil {
		time.Sleep(5 * time.Millisecond)
	}
	if cj.last.Load() == nil {
		t.Error("a current action time was not run")
	}
}
