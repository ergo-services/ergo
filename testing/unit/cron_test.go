package unit

import (
	"testing"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

// TestCron_BasicJobManagement tests basic cron job operations
func TestCron_BasicJobManagement(t *testing.T) {
	actor, err := Spawn(t, factoryExampleActor)
	if err != nil {
		t.Fatal(err)
	}

	cron := actor.Node().Cron()

	// Test adding a cron job
	job := gen.CronJob{
		Name:   "test_job",
		Spec:   "0 * * * *", // Every hour
		Action: gen.CreateCronActionMessage(gen.Atom("worker"), gen.MessagePriorityNormal),
	}

	err = cron.AddJob(job)
	if err != nil {
		t.Fatalf("Failed to add cron job: %v", err)
	}

	// Verify job was added
	actor.ShouldAddCronJob().WithName("test_job").WithSpec("0 * * * *").Assert()

	// Test getting job info
	jobInfo, err := cron.JobInfo("test_job")
	if err != nil {
		t.Fatalf("Failed to get job info: %v", err)
	}
	if jobInfo.Name != "test_job" {
		t.Errorf("Expected job name 'test_job', got %s", jobInfo.Name)
	}

	// Test removing job
	err = cron.RemoveJob("test_job")
	if err != nil {
		t.Fatalf("Failed to remove cron job: %v", err)
	}

	actor.ShouldRemoveCronJob().WithName("test_job").Assert()
}

// cronTestActor for testing cron message handling
type cronTestActor struct {
	act.Actor
	messagesReceived []gen.MessageCron
}

func factoryCronTestActor() gen.ProcessBehavior {
	return &cronTestActor{}
}

func (cta *cronTestActor) Init(args ...any) error {
	cta.messagesReceived = []gen.MessageCron{}
	return nil
}

func (cta *cronTestActor) HandleMessage(from gen.PID, message any) error {
	switch msg := message.(type) {
	case gen.MessageCron:
		cta.messagesReceived = append(cta.messagesReceived, msg)
	}
	return nil
}

func (cta *cronTestActor) Terminate(reason error) {
	// cleanup if needed
}
