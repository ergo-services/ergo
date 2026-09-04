package local

import (
	"testing"
	"time"

	"ergo.services/ergo/app/system/inspect"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/stage"
)

func cronCall(t *testing.T, n *stage.Node, request any) any {
	t.Helper()

	target := gen.ProcessID{Name: inspect.Name, Node: n.Name()}
	result, err := n.Native().Call(target, request)
	if err != nil {
		t.Fatalf("cron request %T: %s", request, err)
	}
	return result
}

func startCronNode(t *testing.T) *stage.Node {
	t.Helper()

	s := stage.New(t)
	n := s.StartNode("n", stage.NodeOptions{EnableSystemApp: true})

	action := gen.CreateCronActionMessage(gen.Atom("target"), gen.MessagePriorityNormal)
	cron := n.Native().Cron()
	if err := cron.AddJob(gen.CronJob{Name: "every5", Spec: "*/5 * * * *", Action: action}); err != nil {
		t.Fatalf("add job: %s", err)
	}
	if err := cron.AddJob(gen.CronJob{Name: "hourly", Spec: "0 * * * *", Action: action}); err != nil {
		t.Fatalf("add job: %s", err)
	}
	return n
}

// TestSystemCronInfo: the scheduler and a single job are readable over the wire,
// so a caller after one job does not have to pull the whole node snapshot.
func TestSystemCronInfo(t *testing.T) {
	n := startCronNode(t)

	all := cronCall(t, n, inspect.RequestGetCronInfo{}).(inspect.ResponseGetCronInfo)
	if all.Error != nil {
		t.Fatalf("cron info: %s", all.Error)
	}
	if len(all.Jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(all.Jobs))
	}
	if all.Next.IsZero() {
		t.Error("the scheduler reports no next firing while two jobs are registered")
	}

	one := cronCall(t, n, inspect.RequestGetCronInfo{Job: "hourly"}).(inspect.ResponseGetCronInfo)
	if one.Error != nil {
		t.Fatalf("job info: %s", one.Error)
	}
	if len(one.Jobs) != 1 || one.Jobs[0].Name != "hourly" {
		t.Fatalf("expected only the hourly job, got %v", one.Jobs)
	}
	if one.Jobs[0].Spec != "0 * * * *" {
		t.Errorf("spec came back as %q", one.Jobs[0].Spec)
	}

	unknown := cronCall(t, n, inspect.RequestGetCronInfo{Job: "nobody"}).(inspect.ResponseGetCronInfo)
	if unknown.Error == nil {
		t.Error("an unknown job answered without an error")
	}
}

// TestSystemCronSchedule: the preview is computed by the node, because only it
// knows its clock and each job's timezone. One job and the whole scheduler come
// back in the same shape.
func TestSystemCronSchedule(t *testing.T) {
	n := startCronNode(t)
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)

	all := cronCall(t, n, inspect.RequestGetCronSchedule{
		Since:    since,
		Duration: time.Hour,
	}).(inspect.ResponseGetCronSchedule)
	if all.Error != nil {
		t.Fatalf("schedule: %s", all.Error)
	}
	if len(all.Schedule) == 0 {
		t.Fatal("an hour window over a */5 job returned nothing")
	}

	one := cronCall(t, n, inspect.RequestGetCronSchedule{
		Job:      "hourly",
		Since:    since,
		Duration: 3 * time.Hour,
	}).(inspect.ResponseGetCronSchedule)
	if one.Error != nil {
		t.Fatalf("job schedule: %s", one.Error)
	}
	if len(one.Schedule) == 0 {
		t.Fatal("three hours over an hourly job returned nothing")
	}
	for _, entry := range one.Schedule {
		if len(entry.Jobs) != 1 || entry.Jobs[0] != "hourly" {
			t.Fatalf("a single-job preview carries %v", entry.Jobs)
		}
	}

	// the cap must bite instead of answering with thousands of timestamps
	capped := cronCall(t, n, inspect.RequestGetCronSchedule{
		Since:    since,
		Duration: 24 * time.Hour,
		Limit:    3,
	}).(inspect.ResponseGetCronSchedule)
	if len(capped.Schedule) != 3 {
		t.Errorf("limit 3 returned %d entries", len(capped.Schedule))
	}
	if capped.Truncated == false {
		t.Error("the answer was capped but not marked truncated")
	}

	unknown := cronCall(t, n, inspect.RequestGetCronSchedule{Job: "nobody"}).(inspect.ResponseGetCronSchedule)
	if unknown.Error == nil {
		t.Error("a preview for an unknown job answered without an error")
	}
}
