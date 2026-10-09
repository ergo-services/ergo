package unit_test

import (
	"testing"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/unit"
)

type lfActor struct {
	act.Actor

	pushed   int
	popped   int
	restored []gen.LogField
	copied   []gen.LogField
}

func factoryLFActor() gen.ProcessBehavior { return &lfActor{} }

func (a *lfActor) Init(args ...any) error {
	a.Log().AddFields(gen.LogField{Name: "actor", Value: "lf"})
	a.Log().Info("started with %d fields", len(a.Log().Fields()))
	return nil
}

func (a *lfActor) HandleMessage(from gen.PID, message any) error {
	switch message {
	case "scope":
		a.pushed = a.Log().PushFields()
		a.Log().AddFields(gen.LogField{Name: "op", Value: "scope"})
		a.Log().Info("inside scope")
		a.popped = a.Log().PopFields()
		a.Log().Info("outside scope")
		a.restored = a.Log().Fields()

	case "delete":
		a.Log().AddFields(gen.LogField{Name: "tmp", Value: 1})
		a.Log().DeleteFields("tmp")
		a.Log().Info("after delete")

	case "delete-pushed":
		a.Log().PushFields()
		a.Log().DeleteFields("actor")
		a.Log().Info("after refused delete")

	case "mutate":
		a.copied = a.Log().Fields()
		a.copied[0] = gen.LogField{Name: "hijacked", Value: true}
		a.Log().Info("after mutating the returned slice")
	}
	return nil
}

func TestLogFieldsRideAlongWithTheLine(t *testing.T) {
	sub, err := unit.Spawn(t, factoryLFActor, gen.ProcessOptions{})
	check.NoError(t, err)

	sub.ShouldLog().From(sub.PID()).Level(gen.LogLevelInfo).
		Format("started with %d fields").Containing("started with 1 fields").
		WithField("actor", "lf").Once().Assert()
}

func TestLogFieldScopeIsRestored(t *testing.T) {
	sub, err := unit.Spawn(t, factoryLFActor, gen.ProcessOptions{})
	check.NoError(t, err)

	sub.SendMessage(sub.PID(), "scope")

	a := sub.Behavior().(*lfActor)
	check.Equal(t, 1, a.pushed)
	check.Equal(t, 0, a.popped)
	check.Equal(t, []gen.LogField{{Name: "actor", Value: "lf"}}, a.restored)

	sub.ShouldLog().Containing("inside scope").WithFields(
		gen.LogField{Name: "actor", Value: "lf"},
		gen.LogField{Name: "op", Value: "scope"},
	).Once().Assert()

	sub.ShouldLog().Containing("outside scope").WithField("actor", "lf").Once().Assert()
	sub.ShouldLog().Containing("outside scope").WithFieldName("op").None().Assert()
}

func TestLogFieldsDeleted(t *testing.T) {
	sub, err := unit.Spawn(t, factoryLFActor, gen.ProcessOptions{})
	check.NoError(t, err)

	sub.SendMessage(sub.PID(), "delete")

	sub.ShouldLog().Containing("after delete").WithField("actor", "lf").Once().Assert()
	sub.ShouldLog().Containing("after delete").WithFieldName("tmp").None().Assert()
}

func TestLogFieldDeleteRefusedWhilePushed(t *testing.T) {
	sub, err := unit.Spawn(t, factoryLFActor, gen.ProcessOptions{})
	check.NoError(t, err)

	sub.SendMessage(sub.PID(), "delete-pushed")

	sub.ShouldLog().Level(gen.LogLevelError).
		Containing("cannot delete log field(s) while the field stack has 1 active frame(s)").
		Once().Assert()
	sub.ShouldLog().Containing("after refused delete").WithField("actor", "lf").Once().Assert()
}

func TestLogFieldsReturnsACopy(t *testing.T) {
	sub, err := unit.Spawn(t, factoryLFActor, gen.ProcessOptions{})
	check.NoError(t, err)

	sub.SendMessage(sub.PID(), "mutate")

	sub.ShouldLog().Containing("after mutating the returned slice").
		WithField("actor", "lf").Once().Assert()
	sub.ShouldLog().Containing("after mutating the returned slice").
		WithFieldName("hijacked").None().Assert()
}

type lfMeta struct {
	proc gen.MetaProcess
}

func (m *lfMeta) Init(p gen.MetaProcess) error {
	m.proc = p
	p.Log().AddFields(gen.LogField{Name: "meta", Value: "lf"})
	p.Log().Info("meta started")
	return nil
}
func (m *lfMeta) Start() error                                  { return nil }
func (m *lfMeta) HandleMessage(from gen.PID, message any) error { return nil }
func (m *lfMeta) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, nil
}

func (m *lfMeta) HandleResponse(response gen.MessageResponse) error {
	return nil
}
func (m *lfMeta) Terminate(reason error)                                       {}
func (m *lfMeta) HandleInspect(from gen.PID, item ...string) map[string]string { return nil }

func TestLogFieldsFromMeta(t *testing.T) {
	sub, err := unit.Spawn(t, factoryLFActor, gen.ProcessOptions{})
	check.NoError(t, err)

	ms, err := sub.SpawnMeta(&lfMeta{}, gen.MetaOptions{})
	check.NoError(t, err)

	sub.ShouldLog().FromMeta(ms.ID()).Containing("meta started").
		WithField("meta", "lf").Once().Assert()
	sub.ShouldLog().FromMeta(ms.ID()).Containing("started with").None().Assert()
}
