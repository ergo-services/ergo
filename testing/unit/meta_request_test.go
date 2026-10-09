package unit_test

import (
	"testing"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/unit"
)

type reqMeta struct {
	mp      gen.MetaProcess
	refs    []gen.Ref
	refused []error
	answers []gen.MessageResponse
}

func (m *reqMeta) Init(process gen.MetaProcess) error { m.mp = process; return nil }
func (m *reqMeta) Start() error                       { return nil }

type askTo struct{ To any }

func (m *reqMeta) HandleMessage(from gen.PID, message any) error {
	if c, ok := message.(askTo); ok {
		_, err := m.mp.SendRequest(c.To, "q")
		m.refused = append(m.refused, err)
		return nil
	}
	switch message {
	case "ask":
		ref, err := m.mp.SendRequestWithLabel(gen.Atom("db"), "q", "orders")
		if err != nil {
			return err
		}
		m.refs = append(m.refs, ref)

	case "ask-parent":
		_, err := m.mp.SendRequest(m.mp.Parent(), "q")
		m.refused = append(m.refused, err)

	case "ask-self":
		_, err := m.mp.SendRequest(m.mp.ID(), "q")
		m.refused = append(m.refused, err)

	case "ask-and-cancel":
		ref, err := m.mp.SendRequest(gen.Atom("db"), "q")
		if err != nil {
			return err
		}
		return m.mp.CancelRequest(ref)
	}
	return nil
}

func (m *reqMeta) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, nil
}

func (m *reqMeta) HandleResponse(response gen.MessageResponse) error {
	m.answers = append(m.answers, response)
	return nil
}

func (m *reqMeta) HandleInspect(from gen.PID, item ...string) map[string]string { return nil }
func (m *reqMeta) Terminate(reason error)                                       {}

func TestMetaRequestIsRecordedAndAnswered(t *testing.T) {
	sub, err := unit.Spawn(t, factoryMetaParent, gen.ProcessOptions{})
	check.NoError(t, err)
	m, err := sub.SpawnMeta(&reqMeta{}, gen.MetaOptions{})
	check.NoError(t, err)

	m.DeliverMessage(sub.PID(), "ask")

	m.ShouldSendRequest().From(sub.PID()).To(gen.Atom("db")).Request("q").
		Label("orders").Timeout(gen.DefaultRequestTimeout).Once().Assert()

	meta := m.Behavior().(*reqMeta)
	m.DeliverResponse(gen.PID{Node: "unit@localhost", ID: 7}, meta.refs[0], "answer", nil)

	check.Equal(t, 1, len(meta.answers))
	check.Equal(t, "answer", meta.answers[0].Result)
	check.Equal(t, "orders", meta.answers[0].Label)
	check.NoError(t, meta.answers[0].Error)
	check.Equal(t, gen.MetaStateSleep, m.State())
}

func TestMetaRequestExpires(t *testing.T) {
	sub, err := unit.Spawn(t, factoryMetaParent, gen.ProcessOptions{})
	check.NoError(t, err)
	m, err := sub.SpawnMeta(&reqMeta{}, gen.MetaOptions{})
	check.NoError(t, err)

	m.DeliverMessage(sub.PID(), "ask")
	meta := m.Behavior().(*reqMeta)
	m.ExpireRequest(meta.refs[0])

	check.Equal(t, 1, len(meta.answers))
	check.ErrorIs(t, meta.answers[0].Error, gen.ErrTimeout)
	check.Equal(t, "orders", meta.answers[0].Label)
}

func TestMetaRequestToParentOrSelfIsRefused(t *testing.T) {
	sub, err := unit.Spawn(t, factoryMetaParent, gen.ProcessOptions{})
	check.NoError(t, err)
	m, err := sub.SpawnMeta(&reqMeta{}, gen.MetaOptions{})
	check.NoError(t, err)

	m.DeliverMessage(sub.PID(), "ask-parent")
	m.DeliverMessage(sub.PID(), "ask-self")

	meta := m.Behavior().(*reqMeta)
	check.Equal(t, 2, len(meta.refused))
	check.ErrorIs(t, meta.refused[0], gen.ErrNotAllowed)
	check.ErrorIs(t, meta.refused[1], gen.ErrNotAllowed)
	m.ShouldSendRequest().None().Assert()
}

func TestMetaRequestCancelled(t *testing.T) {
	sub, err := unit.Spawn(t, factoryMetaParent, gen.ProcessOptions{})
	check.NoError(t, err)
	m, err := sub.SpawnMeta(&reqMeta{}, gen.MetaOptions{})
	check.NoError(t, err)

	m.DeliverMessage(sub.PID(), "ask-and-cancel")
	m.ShouldSendRequest().Once().Assert()
	check.Equal(t, gen.MetaStateSleep, m.State())
}

type namedParent struct {
	act.Actor
	alias   gen.Alias
	refused []error
}

func factoryNamedParent() gen.ProcessBehavior { return &namedParent{} }

func (p *namedParent) Init(args ...any) error {
	if err := p.RegisterName("host"); err != nil {
		return err
	}
	alias, err := p.CreateAlias()
	p.alias = alias
	return err
}

func (p *namedParent) HandleMessage(from gen.PID, message any) error {
	if c, ok := message.(askTo); ok {
		_, err := p.SendRequest(c.To, "q")
		p.refused = append(p.refused, err)
	}
	return nil
}

func TestMetaRequestToParentByNameOrAliasIsRefused(t *testing.T) {
	sub, err := unit.Spawn(t, factoryNamedParent, gen.ProcessOptions{})
	check.NoError(t, err)
	m, err := sub.SpawnMeta(&reqMeta{}, gen.MetaOptions{})
	check.NoError(t, err)
	parent := sub.Behavior().(*namedParent)

	targets := []any{
		gen.Atom("host"),
		gen.ProcessID{Name: "host"},
		gen.ProcessID{Name: "host", Node: sub.PID().Node},
		parent.alias,
	}
	for _, to := range targets {
		m.DeliverMessage(sub.PID(), askTo{To: to})
	}

	meta := m.Behavior().(*reqMeta)
	check.Equal(t, len(targets), len(meta.refused))
	for _, err := range meta.refused {
		check.ErrorIs(t, err, gen.ErrNotAllowed)
	}
	m.ShouldSendRequest().None().Assert()
}

func TestRequestToSelfByNameOrAliasIsRefused(t *testing.T) {
	sub, err := unit.Spawn(t, factoryNamedParent, gen.ProcessOptions{})
	check.NoError(t, err)
	parent := sub.Behavior().(*namedParent)

	targets := []any{
		sub.PID(),
		gen.Atom("host"),
		gen.ProcessID{Name: "host", Node: sub.PID().Node},
		parent.alias,
	}
	for _, to := range targets {
		sub.SendMessage(sub.PID(), askTo{To: to})
	}

	check.Equal(t, len(targets), len(parent.refused))
	for _, err := range parent.refused {
		check.ErrorIs(t, err, gen.ErrNotAllowed)
	}
	sub.ShouldSendRequest().None().Assert()
}
