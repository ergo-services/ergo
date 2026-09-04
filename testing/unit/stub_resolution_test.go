package unit_test

import (
	"errors"
	"testing"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/unit"
)

var errStub = errors.New("stubbed failure")

type egressOp struct {
	Kind   string
	Target any
	Value  any
}

type egressResult struct {
	Err   error
	Value any
}

type egress struct{ act.Actor }

func factoryEgress() gen.ProcessBehavior { return &egress{} }

func (e *egress) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	op, ok := request.(egressOp)
	if ok == false {
		return egressResult{}, nil
	}

	switch op.Kind {
	case "send":
		return egressResult{Err: e.Send(op.Target, op.Value)}, nil
	case "call":
		v, err := e.Call(op.Target, op.Value)
		return egressResult{Err: err, Value: v}, nil
	case "link":
		return egressResult{Err: e.Link(op.Target)}, nil
	case "spawn":
		pid, err := e.Spawn(factoryEgress, gen.ProcessOptions{})
		return egressResult{Err: err, Value: pid}, nil
	case "alias":
		alias, err := e.CreateAlias()
		return egressResult{Err: err, Value: alias}, nil
	case "event":
		token, err := e.RegisterEvent(op.Target.(gen.Atom), gen.EventOptions{})
		return egressResult{Err: err, Value: token}, nil
	}
	return egressResult{}, nil
}

func spawnEgress(t *testing.T) *unit.Subject {
	t.Helper()
	sub, err := unit.StartNode(t, "unit@localhost", gen.NodeOptions{}).Spawn(factoryEgress, gen.ProcessOptions{})
	if err != nil {
		t.Fatalf("spawn: %s", err)
	}
	return sub
}

func doEgress(t *testing.T, sub *unit.Subject, op egressOp) egressResult {
	t.Helper()
	result, err := sub.Call(gen.PID{}, op)
	if err != nil {
		t.Fatalf("call: %s", err)
	}
	return result.(egressResult)
}

func TestStubMatchesItsTargetOnly(t *testing.T) {
	sub := spawnEgress(t)
	sub.OnSend(gen.Atom("blocked")).Fail(errStub)

	if err := doEgress(t, sub, egressOp{Kind: "send", Target: gen.Atom("blocked"), Value: "x"}).Err; errors.Is(err, errStub) == false {
		t.Fatalf("the stubbed target answered %v", err)
	}
	if err := doEgress(t, sub, egressOp{Kind: "send", Target: gen.Atom("open"), Value: "x"}).Err; err != nil {
		t.Fatalf("an unstubbed target answered %v", err)
	}
}

func TestStubWithoutTargetMatchesAnything(t *testing.T) {
	sub := spawnEgress(t)
	sub.OnSend(nil).Fail(errStub)

	for _, target := range []gen.Atom{"one", "two"} {
		if err := doEgress(t, sub, egressOp{Kind: "send", Target: target, Value: "x"}).Err; errors.Is(err, errStub) == false {
			t.Fatalf("sending to %s answered %v", target, err)
		}
	}
}

func TestTheLastStubRegisteredWins(t *testing.T) {
	sub := spawnEgress(t)
	first := errors.New("first")
	sub.OnSend(gen.Atom("target")).Fail(first)
	sub.OnSend(gen.Atom("target")).Fail(errStub)

	if err := doEgress(t, sub, egressOp{Kind: "send", Target: gen.Atom("target"), Value: "x"}).Err; errors.Is(err, errStub) == false {
		t.Fatalf("the later stub did not win: %v", err)
	}
}

func TestFailFuncIsConsultedPerCall(t *testing.T) {
	sub := spawnEgress(t)
	calls := 0
	sub.OnSend(nil).FailFunc(func() error {
		calls++
		if calls == 1 {
			return errStub
		}
		return nil
	})

	if err := doEgress(t, sub, egressOp{Kind: "send", Target: gen.Atom("t"), Value: "x"}).Err; errors.Is(err, errStub) == false {
		t.Fatalf("the first send answered %v", err)
	}
	if err := doEgress(t, sub, egressOp{Kind: "send", Target: gen.Atom("t"), Value: "x"}).Err; err != nil {
		t.Fatalf("the second send answered %v", err)
	}
}

func TestCallStubRespondsAndFails(t *testing.T) {
	sub := spawnEgress(t)
	sub.OnCall(gen.Atom("svc")).Respond("pong")
	sub.OnCall(gen.Atom("dead")).Fail(errStub)

	r := doEgress(t, sub, egressOp{Kind: "call", Target: gen.Atom("svc"), Value: "ping"})
	check.NoError(t, r.Err)
	check.Equal(t, "pong", r.Value)

	r = doEgress(t, sub, egressOp{Kind: "call", Target: gen.Atom("dead"), Value: "ping"})
	if errors.Is(r.Err, errStub) == false {
		t.Fatalf("the failing call answered %v", r.Err)
	}
}

func TestCallStubSeesTheRequest(t *testing.T) {
	sub := spawnEgress(t)
	sub.OnCall(gen.Atom("svc")).RespondWith(func(request any) (any, error) {
		return request.(string) + "!", nil
	})

	r := doEgress(t, sub, egressOp{Kind: "call", Target: gen.Atom("svc"), Value: "ping"})
	check.NoError(t, r.Err)
	check.Equal(t, "ping!", r.Value)
}

func TestCallStubNarrowedByWhere(t *testing.T) {
	sub := spawnEgress(t)
	sub.OnCall(gen.Atom("svc")).Respond("general")
	sub.OnCall(gen.Atom("svc")).Where(check.Equals("special")).Respond("narrow")

	r := doEgress(t, sub, egressOp{Kind: "call", Target: gen.Atom("svc"), Value: "special"})
	check.NoError(t, r.Err)
	check.Equal(t, "narrow", r.Value)

	r = doEgress(t, sub, egressOp{Kind: "call", Target: gen.Atom("svc"), Value: "other"})
	check.NoError(t, r.Err)
	check.Equal(t, "general", r.Value)
}

func TestSpawnStubReturnsAndFails(t *testing.T) {
	sub := spawnEgress(t)
	want := gen.PID{Node: "unit@localhost", ID: 4242}
	sub.OnSpawn(factoryEgress).Return(want)

	r := doEgress(t, sub, egressOp{Kind: "spawn"})
	check.NoError(t, r.Err)
	check.Equal(t, want, r.Value)

	sub.OnSpawn(factoryEgress).Fail(errStub)
	if r := doEgress(t, sub, egressOp{Kind: "spawn"}); errors.Is(r.Err, errStub) == false {
		t.Fatalf("the failing spawn answered %v", r.Err)
	}
}

func TestSpawnStubReturnFuncIsConsultedPerCall(t *testing.T) {
	sub := spawnEgress(t)
	n := uint64(0)
	sub.OnSpawn(factoryEgress).ReturnFunc(func() (gen.PID, error) {
		n++
		return gen.PID{Node: "unit@localhost", ID: n}, nil
	})

	first := doEgress(t, sub, egressOp{Kind: "spawn"})
	second := doEgress(t, sub, egressOp{Kind: "spawn"})
	check.NoError(t, first.Err)
	check.NoError(t, second.Err)
	if first.Value == second.Value {
		t.Fatalf("both spawns answered %v", first.Value)
	}
}

func TestCreateAliasAndRegisterEventStubs(t *testing.T) {
	sub := spawnEgress(t)
	sub.OnCreateAlias().Fail(errStub)
	sub.OnRegisterEvent("ev").Fail(errStub)

	if r := doEgress(t, sub, egressOp{Kind: "alias"}); errors.Is(r.Err, errStub) == false {
		t.Fatalf("the failing alias answered %v", r.Err)
	}
	if r := doEgress(t, sub, egressOp{Kind: "event", Target: gen.Atom("ev")}); errors.Is(r.Err, errStub) == false {
		t.Fatalf("the failing event answered %v", r.Err)
	}

	other := spawnEgress(t)
	wantAlias := gen.Alias{Node: "unit@localhost", ID: [3]uint64{7, 0, 0}}
	wantToken := gen.Ref{Node: "unit@localhost", ID: [3]uint64{9, 0, 0}}
	other.OnCreateAlias().Return(wantAlias)
	other.OnRegisterEvent("ev").Return(wantToken)

	r := doEgress(t, other, egressOp{Kind: "alias"})
	check.NoError(t, r.Err)
	check.Equal(t, wantAlias, r.Value)

	r = doEgress(t, other, egressOp{Kind: "event", Target: gen.Atom("ev")})
	check.NoError(t, r.Err)
	check.Equal(t, wantToken, r.Value)
}

func TestLinkStubFails(t *testing.T) {
	sub := spawnEgress(t)
	target := gen.PID{Node: "unit@localhost", ID: 77}
	sub.OnLink(target).Fail(errStub)

	if r := doEgress(t, sub, egressOp{Kind: "link", Target: target}); errors.Is(r.Err, errStub) == false {
		t.Fatalf("the failing link answered %v", r.Err)
	}
}
