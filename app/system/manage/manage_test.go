package manage

import (
	"errors"
	"testing"
	"time"

	"ergo.services/ergo/gen"
)

func TestPoolSpawnsTheDefaultWorkerCount(t *testing.T) {
	node := manageNode(t)
	sub, err := node.Spawn(Factory, gen.ProcessOptions{})
	if err != nil {
		t.Fatalf("spawn pool: %s", err)
	}
	sub.ShouldSpawn().Times(int(DefaultPoolSize)).Assert()
}

func TestPoolTakesItsSizeFromTheEnvironment(t *testing.T) {
	node := manageNode(t)
	sub, err := node.Spawn(Factory, gen.ProcessOptions{Env: map[gen.Env]any{EnvPoolSize: 3}})
	if err != nil {
		t.Fatalf("spawn pool: %s", err)
	}
	sub.ShouldSpawn().Times(3).Assert()
}

func TestPoolKeepsTheDefaultOnAnUnusableEnvironmentValue(t *testing.T) {
	for _, value := range []any{"3", 0, -1} {
		node := manageNode(t)
		sub, err := node.Spawn(Factory, gen.ProcessOptions{Env: map[gen.Env]any{EnvPoolSize: value}})
		if err != nil {
			t.Fatalf("spawn pool with %#v: %s", value, err)
		}
		sub.ShouldSpawn().Times(int(DefaultPoolSize)).Assert()
	}
}

func TestHandleCallRefusesAnUnknownRequest(t *testing.T) {
	sub := spawnManage(t, manageNode(t))

	_, err := sub.Call(callerPID, "no such request")
	if errors.Is(err, gen.ErrUnsupported) == false {
		t.Fatalf("an unplanned request answered %v, not ErrUnsupported", err)
	}
}

func TestHandleCallDropsARequestPastItsDeadline(t *testing.T) {
	sub := spawnManage(t, manageNode(t))

	expired := time.Now().Unix() - 1
	response, err := sub.CallWithDeadline(callerPID, RequestDoSend{PID: targetPID, Message: "ping"}, expired)
	if err != nil {
		t.Fatalf("an expired request answered the error %s", err)
	}
	if response != nil {
		t.Fatalf("an expired request was still answered with %#v", response)
	}
	sub.ShouldSend().None().Assert()
}

func TestHandleCallAppliesARequestWithinItsDeadline(t *testing.T) {
	sub := spawnManage(t, manageNode(t))

	alive := time.Now().Unix() + 60
	response, err := sub.CallWithDeadline(callerPID, RequestDoSend{PID: targetPID, Message: "ping"}, alive)
	if err != nil {
		t.Fatalf("call: %s", err)
	}
	if r, ok := response.(ResponseDoSend); ok == false || r.Error != nil {
		t.Fatalf("a request within its deadline answered %#v", response)
	}
	sub.ShouldSend().To(targetPID).Once().Assert()
}

func TestHandleCallRollsBackWhenTheCallerStoppedWaiting(t *testing.T) {
	node := manageNode(t)
	node.OnProcessInfo(func(pid gen.PID) (gen.ProcessInfo, error) {
		return gen.ProcessInfo{PID: pid, LogLevel: gen.LogLevelInfo}, nil
	})
	levels := []gen.LogLevel{}
	node.OnSetProcessLogLevel(func(pid gen.PID, level gen.LogLevel) error {
		levels = append(levels, level)
		return nil
	})

	sub := spawnManage(t, node)
	sub.OnSendResponse(callerPID).Fail(gen.ErrResponseIgnored)

	request := RequestDoSetProcessLogLevel{PID: targetPID, Level: gen.LogLevelDebug}
	if _, err := sub.Call(callerPID, request); err != nil {
		t.Fatalf("call: %s", err)
	}

	want := []gen.LogLevel{gen.LogLevelDebug, gen.LogLevelInfo}
	if len(levels) != len(want) || levels[0] != want[0] || levels[1] != want[1] {
		t.Fatalf("the level moved through %v; an ignored response must restore %s", levels, want[1])
	}
}

func TestHandleCallLeavesASendInPlaceWhenTheCallerStoppedWaiting(t *testing.T) {
	sub := spawnManage(t, manageNode(t))
	sub.OnSendResponse(callerPID).Fail(gen.ErrResponseIgnored)

	if _, err := sub.Call(callerPID, RequestDoSend{PID: targetPID, Message: "ping"}); err != nil {
		t.Fatalf("call: %s", err)
	}

	sub.ShouldSend().To(targetPID).Once().Assert()
}

func TestHandleCallKeepsTheChangeWhenTheRollbackFails(t *testing.T) {
	node := manageNode(t)
	node.OnProcessInfo(func(pid gen.PID) (gen.ProcessInfo, error) {
		return gen.ProcessInfo{PID: pid, LogLevel: gen.LogLevelInfo}, nil
	})
	attempts := 0
	node.OnSetProcessLogLevel(func(pid gen.PID, level gen.LogLevel) error {
		attempts++
		if attempts == 1 {
			return nil
		}
		return gen.ErrProcessTerminated
	})

	sub := spawnManage(t, node)
	sub.OnSendResponse(callerPID).Fail(gen.ErrResponseIgnored)

	request := RequestDoSetProcessLogLevel{PID: targetPID, Level: gen.LogLevelDebug}
	if _, err := sub.Call(callerPID, request); err != nil {
		t.Fatalf("call: %s", err)
	}
	if attempts != 2 {
		t.Fatalf("the level was set %d times; the rollback must be attempted once after the change", attempts)
	}
}

func TestHandleCallKeepsTheChangeWhenTheResponseIsUnconfirmed(t *testing.T) {
	node := manageNode(t)
	node.OnProcessInfo(func(pid gen.PID) (gen.ProcessInfo, error) {
		return gen.ProcessInfo{PID: pid, LogLevel: gen.LogLevelInfo}, nil
	})
	levels := []gen.LogLevel{}
	node.OnSetProcessLogLevel(func(pid gen.PID, level gen.LogLevel) error {
		levels = append(levels, level)
		return nil
	})

	sub := spawnManage(t, node)
	sub.OnSendResponse(callerPID).Fail(gen.ErrNoConnection)

	request := RequestDoSetProcessLogLevel{PID: targetPID, Level: gen.LogLevelDebug}
	if _, err := sub.Call(callerPID, request); err != nil {
		t.Fatalf("call: %s", err)
	}

	if len(levels) != 1 || levels[0] != gen.LogLevelDebug {
		t.Fatalf("the level moved through %v; a response that may have arrived must not be undone", levels)
	}
}

func TestHandleInspectNamesThePlane(t *testing.T) {
	sub := spawnManage(t, manageNode(t))

	info, err := sub.Inspect(callerPID)
	if err != nil {
		t.Fatalf("inspect: %s", err)
	}
	if info["plane"] != "manage" {
		t.Fatalf("the worker reports the plane %q, not manage", info["plane"])
	}
}

func TestCapabilitiesCoverEveryPlannedOperation(t *testing.T) {
	caps := Capabilities()
	seen := map[string]bool{}
	for _, c := range caps {
		if seen[c] {
			t.Fatalf("%s is listed twice", c)
		}
		seen[c] = true
	}

	node := manageNode(t)
	node.OnMetaInfo(func(meta gen.Alias) (gen.MetaInfo, error) { return gen.MetaInfo{}, nil })
	node.OnTracingSampler(func() gen.TracingSampler { return gen.TracingSamplerDisable })
	worker := &manage{}

	if _, err := node.Spawn(func() gen.ProcessBehavior { return worker }, gen.ProcessOptions{}); err != nil {
		t.Fatalf("spawn: %s", err)
	}

	for _, request := range Types() {
		op, known := worker.plan(request)
		if known == false {
			continue
		}
		if seen[op.name] == false {
			t.Errorf("%#v plans %s, which Capabilities() does not list", request, op.name)
		}
		delete(seen, op.name)
	}
	for name := range seen {
		t.Errorf("%s is listed as a capability and no request plans it", name)
	}
}

func TestWorkerTerminatesOnAnExitSignal(t *testing.T) {
	sub := spawnManage(t, manageNode(t))

	sub.DeliverExit(sub.PID(), gen.TerminateReasonShutdown)
	if sub.Terminated() == false {
		t.Fatal("the worker survived an exit signal")
	}
	if errors.Is(sub.Reason(), gen.TerminateReasonShutdown) == false {
		t.Fatalf("the worker terminated with %v, not with shutdown", sub.Reason())
	}
}
