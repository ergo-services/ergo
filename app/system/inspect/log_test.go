package inspect

import (
	"fmt"
	"testing"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/unit"
)

func spawnLogInspector(t *testing.T, pattern string, exclude bool) *unit.Subject {
	t.Helper()

	node := unit.StartNode(t, "inspect@localhost", gen.NodeOptions{})
	node.OnLoggerAddPID(func(gen.PID, string, ...gen.LogLevel) error { return nil })
	node.OnLoggerDeletePID(func(gen.PID) {})

	sub, err := node.SpawnRegister(gen.Atom(inspectLog), factory_log, gen.ProcessOptions{},
		[]gen.LogLevel{gen.LogLevelInfo}, 10, pattern, exclude)
	if err != nil {
		t.Fatalf("spawn: %s", err)
	}
	return sub
}

func logEvent(sub *unit.Subject) gen.Atom {
	return gen.Atom(fmt.Sprintf("%s_%s", inspectLog, sub.PID()))
}

func logMessage(text string) gen.MessageLog {
	return gen.MessageLog{
		Time:   time.Now(),
		Level:  gen.LogLevelInfo,
		Format: text,
		Source: gen.MessageLogNode{Node: "inspect@localhost", Creation: 1},
	}
}

func TestLogInspectorRegistersItsEventOnInit(t *testing.T) {
	sub := spawnLogInspector(t, "", false)

	sub.ShouldRegisterEvent().Name(logEvent(sub)).Once().Assert()
}

func TestLogInspectorBecomesALoggerOnlyWhileWatched(t *testing.T) {
	added := 0
	deleted := 0

	node := unit.StartNode(t, "inspect@localhost", gen.NodeOptions{})
	node.OnLoggerAddPID(func(gen.PID, string, ...gen.LogLevel) error { added++; return nil })
	node.OnLoggerDeletePID(func(gen.PID) { deleted++ })
	sub, err := node.SpawnRegister(gen.Atom(inspectLog), factory_log, gen.ProcessOptions{},
		[]gen.LogLevel{gen.LogLevelInfo}, 10, "", false)
	if err != nil {
		t.Fatalf("spawn: %s", err)
	}

	if added != 0 {
		t.Fatal("the inspector attached itself as a logger before anyone subscribed")
	}

	sub.SendMessage(gen.PID{}, gen.MessageEventStart{Name: logEvent(sub)})
	if added != 1 {
		t.Fatalf("attached %d times on the first subscriber, want once", added)
	}

	sub.SendMessage(gen.PID{}, gen.MessageEventStop{Name: logEvent(sub)})
	if deleted != 1 {
		t.Fatalf("detached %d times when the last subscriber left, want once", deleted)
	}
}

func TestLogInspectorPublishesWhatItCollected(t *testing.T) {
	sub := spawnLogInspector(t, "", false)

	sub.SendMessage(gen.PID{}, gen.MessageEventStart{Name: logEvent(sub)})
	sub.DeliverLog(logMessage("disk is full"))
	sub.FireTimers()
	sub.Drain()

	sub.ShouldSendEvent().Name(logEvent(sub)).AtLeast(1).Assert()
}

func TestLogInspectorPublishesNothingWithoutEntries(t *testing.T) {
	sub := spawnLogInspector(t, "", false)

	sub.SendMessage(gen.PID{}, gen.MessageEventStart{Name: logEvent(sub)})
	mark := sub.Mark()
	sub.FireTimers()
	sub.Drain()

	sub.ShouldSendEvent().Since(mark).None().Assert()
}

func TestLogInspectorKeepsOnlyWhatMatchesThePattern(t *testing.T) {
	sub := spawnLogInspector(t, "timeout", false)

	sub.SendMessage(gen.PID{}, gen.MessageEventStart{Name: logEvent(sub)})
	sub.DeliverLog(logMessage("disk is full"))
	mark := sub.Mark()
	sub.FireTimers()
	sub.Drain()

	sub.ShouldSendEvent().Since(mark).None().Assert()
}

func TestLogInspectorDropsWhatMatchesAnExcludingPattern(t *testing.T) {
	sub := spawnLogInspector(t, "noise", true)

	sub.SendMessage(gen.PID{}, gen.MessageEventStart{Name: logEvent(sub)})
	sub.DeliverLog(logMessage("noise noise noise"))
	mark := sub.Mark()
	sub.FireTimers()
	sub.Drain()

	sub.ShouldSendEvent().Since(mark).None().Assert()
}

func TestLogInspectorAnswersAnInspectRequest(t *testing.T) {
	sub := spawnLogInspector(t, "", false)

	client := gen.PID{Node: "inspect@localhost", ID: 100}
	sub.SendMessage(gen.PID{}, requestInspect{pid: client, ref: gen.Ref{}})

	sub.ShouldSendResponse().To(client).Once().Assert()
}

func TestLogInspectorShutsDownWhenNobodyEverSubscribed(t *testing.T) {
	sub := spawnLogInspector(t, "", false)

	sub.SendMessage(gen.PID{}, shutdown{})

	if sub.Terminated() == false {
		t.Fatal("an inspector nobody subscribed to stayed alive, holding its event forever")
	}
}

func TestLogInspectorIgnoresShutdownWhileWatched(t *testing.T) {
	sub := spawnLogInspector(t, "", false)

	sub.SendMessage(gen.PID{}, gen.MessageEventStart{Name: logEvent(sub)})
	sub.SendMessage(gen.PID{}, shutdown{})

	if sub.Terminated() {
		t.Fatal("the inspector shut down under a live subscriber")
	}
}
