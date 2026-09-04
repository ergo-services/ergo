package manage

import (
	"errors"
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/unit"
)

var (
	callerPID = gen.PID{Node: "manage@localhost", ID: 1}
	targetPID = gen.PID{Node: "manage@localhost", ID: 42}
	targetMet = gen.Alias{Node: "manage@localhost", ID: [3]uint64{7, 0, 0}}
)

func manageNode(t *testing.T) *unit.MockNode {
	t.Helper()
	return unit.StartNode(t, "manage@localhost", gen.NodeOptions{})
}

func settingNode(t *testing.T) *unit.MockNode {
	t.Helper()
	node := manageNode(t)
	node.OnProcessInfo(func(pid gen.PID) (gen.ProcessInfo, error) {
		return gen.ProcessInfo{
			PID:               pid,
			LogLevel:          gen.LogLevelInfo,
			MessagePriority:   gen.MessagePriorityNormal,
			KeepNetworkOrder:  true,
			ImportantDelivery: false,
			Compression: gen.Compression{
				Enable:    false,
				Type:      gen.CompressionTypeGZIP,
				Level:     gen.CompressionDefault,
				Threshold: 1024,
			},
		}, nil
	})
	return node
}

func spawnManage(t *testing.T, node *unit.MockNode) *unit.Subject {
	t.Helper()
	sub, err := node.Spawn(workerFactory, gen.ProcessOptions{})
	if err != nil {
		t.Fatalf("spawn: %s", err)
	}
	return sub
}

func callManage(t *testing.T, sub *unit.Subject, request any) any {
	t.Helper()
	response, err := sub.Call(callerPID, request)
	if err != nil {
		t.Fatalf("%#v answered the error %s", request, err)
	}
	if response == nil {
		t.Fatalf("%#v was not answered at all", request)
	}
	return response
}

func setErrorOf(t *testing.T, response any) error {
	t.Helper()
	r, ok := response.(ResponseDoSet)
	if ok == false {
		t.Fatalf("answered %#v, which is not a ResponseDoSet", response)
	}
	return r.Error
}

func TestPlanSend(t *testing.T) {
	sub := spawnManage(t, manageNode(t))

	response := callManage(t, sub, RequestDoSend{PID: targetPID, Priority: gen.MessagePriorityHigh, Message: "ping"})
	if r := response.(ResponseDoSend); r.Error != nil {
		t.Fatalf("send answered %s", r.Error)
	}

	sub.ShouldSend().To(targetPID).Message("ping").Priority(gen.MessagePriorityHigh).Once().Assert()
}

func TestPlanSendReportsFailure(t *testing.T) {
	sub := spawnManage(t, manageNode(t))
	sub.OnSend(targetPID).Fail(gen.ErrProcessUnknown)

	response := callManage(t, sub, RequestDoSend{PID: targetPID, Message: "ping"})
	if r := response.(ResponseDoSend); errors.Is(r.Error, gen.ErrProcessUnknown) == false {
		t.Fatalf("a send to an unknown process answered %v", r.Error)
	}
}

func TestPlanSendMeta(t *testing.T) {
	sub := spawnManage(t, manageNode(t))

	response := callManage(t, sub, RequestDoSendMeta{Meta: targetMet, Message: "ping"})
	if r := response.(ResponseDoSendMeta); r.Error != nil {
		t.Fatalf("send meta answered %s", r.Error)
	}

	sub.ShouldSend().To(targetMet).Message("ping").Once().Assert()
}

func TestPlanSendExit(t *testing.T) {
	sub := spawnManage(t, manageNode(t))

	response := callManage(t, sub, RequestDoSendExit{PID: targetPID, Reason: gen.TerminateReasonShutdown})
	if r := response.(ResponseDoSendExit); r.Error != nil {
		t.Fatalf("send exit answered %s", r.Error)
	}

	sub.ShouldSendExit().To(targetPID).Reason(gen.TerminateReasonShutdown).Once().Assert()
}

func TestPlanSendExitMeta(t *testing.T) {
	sub := spawnManage(t, manageNode(t))

	response := callManage(t, sub, RequestDoSendExitMeta{Meta: targetMet, Reason: gen.TerminateReasonShutdown})
	if r := response.(ResponseDoSendExitMeta); r.Error != nil {
		t.Fatalf("send exit meta answered %s", r.Error)
	}

	sub.ShouldSendExitMeta().Meta(targetMet).Reason(gen.TerminateReasonShutdown).Once().Assert()
}

func TestPlanKill(t *testing.T) {
	node := manageNode(t)
	killed := []gen.PID{}
	node.OnKill(func(pid gen.PID) error {
		killed = append(killed, pid)
		return nil
	})

	response := callManage(t, spawnManage(t, node), RequestDoKill{PID: targetPID})
	if r := response.(ResponseDoKill); r.Error != nil {
		t.Fatalf("kill answered %s", r.Error)
	}
	if len(killed) != 1 || killed[0] != targetPID {
		t.Fatalf("kill reached %v instead of %s alone", killed, targetPID)
	}
}

func TestPlanSetLogLevel(t *testing.T) {
	node := manageNode(t)
	sub := spawnManage(t, node)

	response := callManage(t, sub, RequestDoSetLogLevel{Level: gen.LogLevelError})
	r, ok := response.(ResponseDoSetLogLevel)
	if ok == false {
		t.Fatalf("answered %#v, which is not a ResponseDoSetLogLevel", response)
	}
	if r.Error != nil {
		t.Fatalf("set log level answered %s", r.Error)
	}
	if got := node.Log().Level(); got != gen.LogLevelError {
		t.Fatalf("the node log level is %s, not %s", got, gen.LogLevelError)
	}
}

func TestPlanSetLogLevelRejected(t *testing.T) {
	node := manageNode(t)
	before := node.Log().Level()

	response := callManage(t, spawnManage(t, node), RequestDoSetLogLevel{Level: gen.LogLevelTrace})
	if r := response.(ResponseDoSetLogLevel); r.Error == nil {
		t.Fatal("the trace level was accepted at runtime, which the node refuses")
	}
	if got := node.Log().Level(); got != before {
		t.Fatalf("a refused level still moved the node log level to %s", got)
	}
}

func TestPlanSetProcessLogLevel(t *testing.T) {
	node := settingNode(t)
	levels := []gen.LogLevel{}
	node.OnSetProcessLogLevel(func(pid gen.PID, level gen.LogLevel) error {
		if pid != targetPID {
			t.Errorf("the level was set on %s, not on %s", pid, targetPID)
		}
		levels = append(levels, level)
		return nil
	})

	response := callManage(t, spawnManage(t, node), RequestDoSetProcessLogLevel{PID: targetPID, Level: gen.LogLevelDebug})
	if r := response.(ResponseDoSetLogLevel); r.Error != nil {
		t.Fatalf("set process log level answered %s", r.Error)
	}
	if len(levels) != 1 || levels[0] != gen.LogLevelDebug {
		t.Fatalf("the process log level moved through %v instead of debug alone", levels)
	}
}

func TestPlanSetMetaLogLevel(t *testing.T) {
	node := manageNode(t)
	node.OnMetaInfo(func(meta gen.Alias) (gen.MetaInfo, error) {
		return gen.MetaInfo{LogLevel: gen.LogLevelInfo}, nil
	})
	levels := []gen.LogLevel{}
	node.OnSetMetaLogLevel(func(meta gen.Alias, level gen.LogLevel) error {
		if meta != targetMet {
			t.Errorf("the level was set on %s, not on %s", meta, targetMet)
		}
		levels = append(levels, level)
		return nil
	})

	response := callManage(t, spawnManage(t, node), RequestDoSetMetaLogLevel{Meta: targetMet, Level: gen.LogLevelWarning})
	if r := response.(ResponseDoSetLogLevel); r.Error != nil {
		t.Fatalf("set meta log level answered %s", r.Error)
	}
	if len(levels) != 1 || levels[0] != gen.LogLevelWarning {
		t.Fatalf("the meta log level moved through %v instead of warning alone", levels)
	}
}

func TestPlanSetNodeTracingSampler(t *testing.T) {
	node := manageNode(t)
	node.OnTracingSampler(func() gen.TracingSampler { return gen.TracingSamplerDisable })
	applied := []gen.TracingSampler{}
	node.OnSetTracingSampler(func(sampler gen.TracingSampler) error {
		applied = append(applied, sampler)
		return nil
	})

	request := RequestDoSetNodeTracingSampler{Type: "ratio", Rate: 0.5}
	if err := setErrorOf(t, callManage(t, spawnManage(t, node), request)); err != nil {
		t.Fatalf("set node sampler answered %s", err)
	}
	if len(applied) != 1 || applied[0].String() != "ratio(0.5)" {
		t.Fatalf("the node sampler was set to %v, not to ratio(0.5)", applied)
	}
}

func TestPlanSetProcessTracingSampler(t *testing.T) {
	node := manageNode(t)
	applied := map[gen.PID]string{}
	node.OnSetProcessTracingSampler(func(pid gen.PID, sampler gen.TracingSampler) error {
		applied[pid] = sampler.String()
		return nil
	})

	request := RequestDoSetProcessTracingSampler{PID: targetPID, Type: "rate_limit", Limit: 10}
	if err := setErrorOf(t, callManage(t, spawnManage(t, node), request)); err != nil {
		t.Fatalf("set process sampler answered %s", err)
	}
	if applied[targetPID] != "rate_limit(10/s)" {
		t.Fatalf("the sampler of %s is %q, not rate_limit(10/s)", targetPID, applied[targetPID])
	}
}

func TestPlanSetProcessSendPriority(t *testing.T) {
	node := settingNode(t)
	got := []gen.MessagePriority{}
	node.OnSetProcessSendPriority(func(pid gen.PID, priority gen.MessagePriority) error {
		got = append(got, priority)
		return nil
	})

	request := RequestDoSetProcessSendPriority{PID: targetPID, Priority: gen.MessagePriorityMax}
	if err := setErrorOf(t, callManage(t, spawnManage(t, node), request)); err != nil {
		t.Fatalf("set send priority answered %s", err)
	}
	if len(got) != 1 || got[0] != gen.MessagePriorityMax {
		t.Fatalf("the send priority moved through %v instead of max alone", got)
	}
}

func TestPlanSetProcessCompression(t *testing.T) {
	node := settingNode(t)
	got := []bool{}
	node.OnSetProcessCompression(func(pid gen.PID, enabled bool) error {
		got = append(got, enabled)
		return nil
	})

	request := RequestDoSetProcessCompression{PID: targetPID, Enabled: true}
	if err := setErrorOf(t, callManage(t, spawnManage(t, node), request)); err != nil {
		t.Fatalf("set compression answered %s", err)
	}
	if len(got) != 1 || got[0] == false {
		t.Fatalf("compression moved through %v instead of being enabled once", got)
	}
}

func TestPlanSetProcessCompressionType(t *testing.T) {
	node := settingNode(t)
	got := []gen.CompressionType{}
	node.OnSetProcessCompressionType(func(pid gen.PID, ctype gen.CompressionType) error {
		got = append(got, ctype)
		return nil
	})

	request := RequestDoSetProcessCompressionType{PID: targetPID, Type: gen.CompressionTypeLZW}
	if err := setErrorOf(t, callManage(t, spawnManage(t, node), request)); err != nil {
		t.Fatalf("set compression type answered %s", err)
	}
	if len(got) != 1 || got[0] != gen.CompressionTypeLZW {
		t.Fatalf("the compression type moved through %v instead of lzw alone", got)
	}
}

func TestPlanSetProcessCompressionLevel(t *testing.T) {
	node := settingNode(t)
	got := []gen.CompressionLevel{}
	node.OnSetProcessCompressionLevel(func(pid gen.PID, level gen.CompressionLevel) error {
		got = append(got, level)
		return nil
	})

	request := RequestDoSetProcessCompressionLevel{PID: targetPID, Level: gen.CompressionBestSize}
	if err := setErrorOf(t, callManage(t, spawnManage(t, node), request)); err != nil {
		t.Fatalf("set compression level answered %s", err)
	}
	if len(got) != 1 || got[0] != gen.CompressionBestSize {
		t.Fatalf("the compression level moved through %v instead of best size alone", got)
	}
}

func TestPlanSetProcessCompressionThreshold(t *testing.T) {
	node := settingNode(t)
	got := []int{}
	node.OnSetProcessCompressionThreshold(func(pid gen.PID, threshold int) error {
		got = append(got, threshold)
		return nil
	})

	request := RequestDoSetProcessCompressionThreshold{PID: targetPID, Threshold: 4096}
	if err := setErrorOf(t, callManage(t, spawnManage(t, node), request)); err != nil {
		t.Fatalf("set compression threshold answered %s", err)
	}
	if len(got) != 1 || got[0] != 4096 {
		t.Fatalf("the compression threshold moved through %v instead of 4096 alone", got)
	}
}

func TestPlanSetProcessKeepNetworkOrder(t *testing.T) {
	node := settingNode(t)
	got := []bool{}
	node.OnSetProcessKeepNetworkOrder(func(pid gen.PID, order bool) error {
		got = append(got, order)
		return nil
	})

	request := RequestDoSetProcessKeepNetworkOrder{PID: targetPID, Order: false}
	if err := setErrorOf(t, callManage(t, spawnManage(t, node), request)); err != nil {
		t.Fatalf("set keep network order answered %s", err)
	}
	if len(got) != 1 || got[0] == true {
		t.Fatalf("keep network order moved through %v instead of being cleared once", got)
	}
}

func TestPlanSetProcessImportantDelivery(t *testing.T) {
	node := settingNode(t)
	got := []bool{}
	node.OnSetProcessImportantDelivery(func(pid gen.PID, important bool) error {
		got = append(got, important)
		return nil
	})

	request := RequestDoSetProcessImportantDelivery{PID: targetPID, Important: true}
	if err := setErrorOf(t, callManage(t, spawnManage(t, node), request)); err != nil {
		t.Fatalf("set important delivery answered %s", err)
	}
	if len(got) != 1 || got[0] == false {
		t.Fatalf("important delivery moved through %v instead of being set once", got)
	}
}

func TestPlanSetMetaSendPriority(t *testing.T) {
	node := manageNode(t)
	node.OnMetaInfo(func(meta gen.Alias) (gen.MetaInfo, error) {
		return gen.MetaInfo{MessagePriority: gen.MessagePriorityNormal}, nil
	})
	got := []gen.MessagePriority{}
	node.OnSetMetaSendPriority(func(meta gen.Alias, priority gen.MessagePriority) error {
		if meta != targetMet {
			t.Errorf("the priority was set on %s, not on %s", meta, targetMet)
		}
		got = append(got, priority)
		return nil
	})

	request := RequestDoSetMetaSendPriority{Meta: targetMet, Priority: gen.MessagePriorityHigh}
	if err := setErrorOf(t, callManage(t, spawnManage(t, node), request)); err != nil {
		t.Fatalf("set meta send priority answered %s", err)
	}
	if len(got) != 1 || got[0] != gen.MessagePriorityHigh {
		t.Fatalf("the meta send priority moved through %v instead of high alone", got)
	}
}

func TestPlanSetReportsFailure(t *testing.T) {
	node := manageNode(t)
	node.OnSetProcessSendPriority(func(pid gen.PID, priority gen.MessagePriority) error {
		return gen.ErrProcessUnknown
	})

	request := RequestDoSetProcessSendPriority{PID: targetPID, Priority: gen.MessagePriorityMax}
	err := setErrorOf(t, callManage(t, spawnManage(t, node), request))
	if errors.Is(err, gen.ErrProcessUnknown) == false {
		t.Fatalf("a setting on an unknown process answered %v", err)
	}
}

func TestPlanAppStartPerMode(t *testing.T) {
	for _, tc := range []struct {
		mode gen.ApplicationMode
		want string
	}{
		{gen.ApplicationModeTemporary, "temporary"},
		{gen.ApplicationModeTransient, "transient"},
		{gen.ApplicationModePermanent, "permanent"},
		{gen.ApplicationMode(200), "default"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			node := manageNode(t)
			started := []string{}
			node.OnApplicationStart(func(name gen.Atom, options gen.ApplicationOptions) error {
				started = append(started, "default")
				return nil
			})
			node.OnApplicationStartTemporary(func(name gen.Atom, options gen.ApplicationOptions) error {
				started = append(started, "temporary")
				return nil
			})
			node.OnApplicationStartTransient(func(name gen.Atom, options gen.ApplicationOptions) error {
				started = append(started, "transient")
				return nil
			})
			node.OnApplicationStartPermanent(func(name gen.Atom, options gen.ApplicationOptions) error {
				started = append(started, "permanent")
				return nil
			})

			request := RequestDoAppStart{Name: "worker_app", Mode: tc.mode}
			response := callManage(t, spawnManage(t, node), request)
			if r := response.(ResponseDoAppStart); r.Error != nil {
				t.Fatalf("app start answered %s", r.Error)
			}
			if len(started) != 1 || started[0] != tc.want {
				t.Fatalf("the application was started as %v, not as %s", started, tc.want)
			}
		})
	}
}

func TestPlanAppStartReportsFailure(t *testing.T) {
	node := manageNode(t)
	node.OnApplicationStartPermanent(func(name gen.Atom, options gen.ApplicationOptions) error {
		return gen.ErrApplicationUnknown
	})

	request := RequestDoAppStart{Name: "worker_app", Mode: gen.ApplicationModePermanent}
	response := callManage(t, spawnManage(t, node), request)
	if r := response.(ResponseDoAppStart); errors.Is(r.Error, gen.ErrApplicationUnknown) == false {
		t.Fatalf("starting an unknown application answered %v", r.Error)
	}
}

func TestPlanAppStop(t *testing.T) {
	node := manageNode(t)
	stopped := []gen.Atom{}
	forced := []gen.Atom{}
	node.OnApplicationStop(func(name gen.Atom) error {
		stopped = append(stopped, name)
		return nil
	})
	node.OnApplicationStopForce(func(name gen.Atom) error {
		forced = append(forced, name)
		return nil
	})

	response := callManage(t, spawnManage(t, node), RequestDoAppStop{Name: "worker_app"})
	if r := response.(ResponseDoAppStop); r.Error != nil {
		t.Fatalf("app stop answered %s", r.Error)
	}
	if len(stopped) != 1 || stopped[0] != "worker_app" || len(forced) != 0 {
		t.Fatalf("a plain stop reached stop=%v force=%v", stopped, forced)
	}
}

func TestPlanAppStopForce(t *testing.T) {
	node := manageNode(t)
	stopped := []gen.Atom{}
	forced := []gen.Atom{}
	node.OnApplicationStop(func(name gen.Atom) error {
		stopped = append(stopped, name)
		return nil
	})
	node.OnApplicationStopForce(func(name gen.Atom) error {
		forced = append(forced, name)
		return nil
	})

	response := callManage(t, spawnManage(t, node), RequestDoAppStop{Name: "worker_app", Force: true})
	if r := response.(ResponseDoAppStop); r.Error != nil {
		t.Fatalf("forced app stop answered %s", r.Error)
	}
	if len(forced) != 1 || forced[0] != "worker_app" || len(stopped) != 0 {
		t.Fatalf("a forced stop reached stop=%v force=%v", stopped, forced)
	}
}

func TestPlanAppUnload(t *testing.T) {
	node := manageNode(t)
	unloaded := []gen.Atom{}
	node.OnApplicationUnload(func(name gen.Atom) error {
		unloaded = append(unloaded, name)
		return gen.ErrApplicationRunning
	})

	response := callManage(t, spawnManage(t, node), RequestDoAppUnload{Name: "worker_app"})
	r := response.(ResponseDoAppUnload)
	if errors.Is(r.Error, gen.ErrApplicationRunning) == false {
		t.Fatalf("unloading a running application answered %v", r.Error)
	}
	if len(unloaded) != 1 || unloaded[0] != "worker_app" {
		t.Fatalf("unload reached %v instead of worker_app alone", unloaded)
	}
}

func TestMakeSampler(t *testing.T) {
	for _, tc := range []struct {
		typ   string
		rate  float64
		limit int
		want  string
	}{
		{"always", 0, 0, "always"},
		{"ratio", 0.25, 0, "ratio(0.25)"},
		{"rate_limit", 0, 5, "rate_limit(5/s)"},
		{"disable", 0, 0, "disable"},
		{"", 0, 0, "disable"},
		{"nonsense", 0, 0, "disable"},
	} {
		t.Run(tc.typ+"/"+tc.want, func(t *testing.T) {
			if got := makeSampler(tc.typ, tc.rate, tc.limit).String(); got != tc.want {
				t.Fatalf("%q built the sampler %s, not %s", tc.typ, got, tc.want)
			}
		})
	}
}
