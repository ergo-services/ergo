package act_test

import (
	"errors"
	"testing"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/unit"
)

var errActorBoom = errors.New("actor boom")

// acu records which callback handled each delivery and lets a test drive the
// call-result / termination paths via well-known requests.
type acu struct {
	act.Actor
	hits []string
}

func factoryAcu() gen.ProcessBehavior { return &acu{} }

func (a *acu) HandleMessage(from gen.PID, message any) error {
	if message == "die" {
		return errActorBoom
	}
	a.hits = append(a.hits, "msg")
	return nil
}
func (a *acu) HandleMessageName(name gen.Atom, from gen.PID, message any) error {
	a.hits = append(a.hits, "name:"+string(name))
	return nil
}
func (a *acu) HandleMessageAlias(alias gen.Alias, from gen.PID, message any) error {
	a.hits = append(a.hits, "alias")
	return nil
}
func (a *acu) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	switch request {
	case "async":
		return nil, nil // deferred response
	case "fail":
		return nil, errActorBoom
	case "normal-result":
		return "bye", gen.TerminateReasonNormal
	}
	return "pong", nil
}
func (a *acu) HandleCallName(name gen.Atom, from gen.PID, ref gen.Ref, request any) (any, error) {
	return "name-resp", nil
}
func (a *acu) HandleCallAlias(alias gen.Alias, from gen.PID, ref gen.Ref, request any) (any, error) {
	return "alias-resp", nil
}
func (a *acu) HandleEvent(message gen.MessageEvent) error {
	a.hits = append(a.hits, "event")
	return nil
}

func acb(s *unit.Subject) *acu { return s.Behavior().(*acu) }

//
// dispatch
//

func TestActorUnitHandleMessage(t *testing.T) {
	s, err := unit.Spawn(t, factoryAcu, gen.ProcessOptions{})
	check.NoError(t, err)
	s.SendMessage(gen.PID{}, "hi")
	check.Equal(t, []string{"msg"}, acb(s).hits)
}

func TestActorUnitHandleMessageError(t *testing.T) {
	s, _ := unit.Spawn(t, factoryAcu, gen.ProcessOptions{})
	s.SendMessage(gen.PID{}, "die")
	s.ShouldTerminate().Reason(errActorBoom).Once().Assert()
}

func TestActorUnitCallResult(t *testing.T) {
	s, _ := unit.Spawn(t, factoryAcu, gen.ProcessOptions{})
	resp, err := s.Call(gen.PID{}, "ping")
	check.NoError(t, err)
	check.Equal(t, "pong", resp)
}

func TestActorUnitCallAsync(t *testing.T) {
	s, _ := unit.Spawn(t, factoryAcu, gen.ProcessOptions{})
	resp, err := s.Call(gen.PID{}, "async") // nil result -> deferred, no response
	check.NoError(t, err)
	check.Nil(t, resp)
	s.ShouldSendResponse().None().Assert()
}

func TestActorUnitCallFail(t *testing.T) {
	s, _ := unit.Spawn(t, factoryAcu, gen.ProcessOptions{})
	_, err := s.Call(gen.PID{}, "fail")
	check.ErrorIs(t, err, errActorBoom)
	check.True(t, s.Terminated())
}

func TestActorUnitCallNormalResultThenTerminate(t *testing.T) {
	s, _ := unit.Spawn(t, factoryAcu, gen.ProcessOptions{})
	resp, err := s.Call(gen.PID{}, "normal-result")
	check.NoError(t, err) // result delivered before shutdown
	check.Equal(t, "bye", resp)
	check.True(t, s.Terminated())
}

func TestActorUnitHandleEvent(t *testing.T) {
	s, _ := unit.Spawn(t, factoryAcu, gen.ProcessOptions{})
	s.DeliverEvent(gen.Event{Name: "e"}, "m")
	check.Equal(t, []string{"event"}, acb(s).hits)
}

//
// split-handler dispatch (by Target type)
//

func TestActorUnitSplitMessageDispatch(t *testing.T) {
	s, _ := unit.Spawn(t, factoryAcu, gen.ProcessOptions{})
	acb(s).SetSplitHandle(true)
	check.True(t, acb(s).SplitHandle())

	s.SendMessage(gen.PID{}, "by-pid")                              // -> HandleMessage
	s.SendMessageName("svc", gen.PID{}, "by-name")                 // -> HandleMessageName
	s.SendMessageAlias(gen.Alias{Node: "unit@localhost"}, gen.PID{}, "by-alias") // -> HandleMessageAlias

	check.Equal(t, []string{"msg", "name:svc", "alias"}, acb(s).hits)
}

func TestActorUnitSplitCallDispatch(t *testing.T) {
	s, _ := unit.Spawn(t, factoryAcu, gen.ProcessOptions{})
	acb(s).SetSplitHandle(true)

	byName, err := s.CallName("svc", gen.PID{}, "q")
	check.NoError(t, err)
	check.Equal(t, "name-resp", byName)

	byAlias, err := s.CallAlias(gen.Alias{Node: "unit@localhost"}, gen.PID{}, "q")
	check.NoError(t, err)
	check.Equal(t, "alias-resp", byAlias)
}

//
// exit / trap-exit
//

func TestActorUnitExitTerminates(t *testing.T) {
	s, _ := unit.Spawn(t, factoryAcu, gen.ProcessOptions{})
	s.DeliverExit(gen.PID{Node: "x@y", ID: 5}, errors.New("crash"))
	check.True(t, s.Terminated())
}

func TestActorUnitTrapExit(t *testing.T) {
	s, _ := unit.Spawn(t, factoryAcu, gen.ProcessOptions{})
	acb(s).SetTrapExit(true)
	check.True(t, acb(s).TrapExit())

	// a non-parent exit is trapped: re-dispatched to HandleMessage, actor survives
	s.DeliverExit(gen.PID{Node: "x@y", ID: 5}, errors.New("crash"))
	check.False(t, s.Terminated())
	check.Equal(t, []string{"msg"}, acb(s).hits)
}

func exitVariants() []any {
	return []any{
		gen.MessageExitProcessID{ProcessID: gen.ProcessID{Name: "p", Node: "x@y"}, Reason: errors.New("boom")},
		gen.MessageExitAlias{Alias: gen.Alias{Node: "x@y"}, Reason: errors.New("boom")},
		gen.MessageExitEvent{Event: gen.Event{Name: "e"}, Reason: errors.New("boom")},
		gen.MessageExitNode{Name: "x@y"},
	}
}

// each non-PID exit signal variant terminates the actor (no trap).
func TestActorUnitExitVariantsTerminate(t *testing.T) {
	for _, ev := range exitVariants() {
		s, err := unit.Spawn(t, factoryAcu, gen.ProcessOptions{})
		check.NoError(t, err)
		s.DeliverExitMessage(ev)
		check.True(t, s.Terminated())
	}
}

// with trap enabled, every exit variant is re-dispatched to HandleMessage instead.
func TestActorUnitTrapExitVariants(t *testing.T) {
	for _, ev := range exitVariants() {
		s, _ := unit.Spawn(t, factoryAcu, gen.ProcessOptions{})
		acb(s).SetTrapExit(true)
		s.DeliverExitMessage(ev)
		check.False(t, s.Terminated())
		check.Equal(t, []string{"msg"}, acb(s).hits)
	}
}

// a panic in Init is recovered into a spawn error.
type acuInitPanic struct{ act.Actor }

func factoryAcuInitPanic() gen.ProcessBehavior { return &acuInitPanic{} }

func (a *acuInitPanic) Init(args ...any) error { panic("init boom") }

func TestActorUnitInitPanic(t *testing.T) {
	_, err := unit.Spawn(t, factoryAcuInitPanic, gen.ProcessOptions{})
	check.Error(t, err)
}

//
// inspect / kind
//

func TestActorUnitInspect(t *testing.T) {
	s, _ := unit.Spawn(t, factoryAcu, gen.ProcessOptions{})
	_, err := s.Inspect(gen.PID{}) // default HandleInspect returns nil
	check.NoError(t, err)
}

func TestActorUnitHandleLog(t *testing.T) {
	s, _ := unit.Spawn(t, factoryAcu, gen.ProcessOptions{})
	s.DeliverLog(gen.MessageLog{}) // default HandleLog (warn), actor survives
	check.False(t, s.Terminated())
}

func TestActorUnitHandleSpan(t *testing.T) {
	s, _ := unit.Spawn(t, factoryAcu, gen.ProcessOptions{})
	s.DeliverSpan(gen.TracingSpan{}) // default HandleSpan, actor survives
	check.False(t, s.Terminated())
}

func TestActorUnitKind(t *testing.T) {
	s, _ := unit.Spawn(t, factoryAcu, gen.ProcessOptions{})
	kind := s.Behavior().(interface{ ProcessKind() gen.ProcessKind }).ProcessKind()
	check.Equal(t, gen.ProcessKindActor, kind)
}

//
// base act.Actor default callbacks (no overrides)
//

type acuPlain struct{ act.Actor }

func factoryAcuPlain() gen.ProcessBehavior { return &acuPlain{} }

func TestActorUnitDefaultCallbacks(t *testing.T) {
	s, err := unit.Spawn(t, factoryAcuPlain, gen.ProcessOptions{})
	check.NoError(t, err)
	acb := s.Behavior().(*acuPlain)
	acb.SetSplitHandle(true)

	s.SendMessage(gen.PID{}, "m")                                  // default HandleMessage (warn)
	s.SendMessageName("n", gen.PID{}, "m")                        // default HandleMessageName (warn)
	s.SendMessageAlias(gen.Alias{Node: "unit@localhost"}, gen.PID{}, "m") // default HandleMessageAlias (warn)
	s.DeliverEvent(gen.Event{Name: "e"}, "m")                     // default HandleEvent

	resp, err := s.Call(gen.PID{}, "q") // default HandleCall (warn, nil)
	check.NoError(t, err)
	check.Nil(t, resp)

	nr, err := s.CallName("n", gen.PID{}, "q") // default HandleCallName (warn, nil)
	check.NoError(t, err)
	check.Nil(t, nr)
	ar, err := s.CallAlias(gen.Alias{Node: "unit@localhost"}, gen.PID{}, "q") // default HandleCallAlias
	check.NoError(t, err)
	check.Nil(t, ar)

	s.ShouldTerminate().None().Assert()
}
