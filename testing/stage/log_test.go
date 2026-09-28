package stage_test

import (
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

type logActor struct {
	act.Actor
}

func factoryLogActor() gen.ProcessBehavior { return &logActor{} }

func (a *logActor) Init(args ...any) error {
	a.Log().AddFields(gen.LogField{Name: "actor", Value: "log"})
	return nil
}

func (a *logActor) HandleMessage(from gen.PID, message any) error {
	switch message {
	case "plain":
		a.Log().Info("handled %d messages", 1)

	case "scope":
		a.Log().PushFields()
		a.Log().AddFields(gen.LogField{Name: "op", Value: "scope"})
		a.Log().Info("inside scope")
		a.Log().PopFields()
		a.Log().Info("outside scope")

	case "debug":
		a.Log().Debug("debug line")

	case "redirected":
		a.Log().SetLogger("nowhere")
		a.Log().Info("redirected line")
	}
	return nil
}

func (a *logActor) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	if request != "meta" {
		return nil, nil
	}
	return a.SpawnMeta(&logMeta{}, gen.MetaOptions{})
}

type logMeta struct {
	proc gen.MetaProcess
}

func (m *logMeta) Init(p gen.MetaProcess) error {
	m.proc = p
	p.Log().AddFields(gen.LogField{Name: "meta", Value: "log"})
	p.Log().Info("meta line")
	return nil
}
func (m *logMeta) Start() error                                  { return nil }
func (m *logMeta) HandleMessage(from gen.PID, message any) error { return nil }
func (m *logMeta) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, nil
}
func (m *logMeta) Terminate(reason error)                                       {}
func (m *logMeta) HandleInspect(from gen.PID, item ...string) map[string]string { return nil }

func TestStageRecordsLogFields(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("logs")
	pid := n.Spawn(factoryLogActor, gen.ProcessOptions{})

	n.Send(pid, "plain")
	n.ShouldLog().From(pid).Level(gen.LogLevelInfo).
		Format("handled %d messages").Containing("handled 1 messages").
		WithField("actor", "log").Once().Within(time.Second).Must()

	n.Send(pid, "scope")
	n.ShouldLog().Containing("inside scope").WithFields(
		gen.LogField{Name: "actor", Value: "log"},
		gen.LogField{Name: "op", Value: "scope"},
	).Once().Within(time.Second).Must()
	n.ShouldLog().Containing("outside scope").WithField("actor", "log").
		Once().Within(time.Second).Must()
	n.ShouldLog().Containing("outside scope").WithFieldName("op").
		None().Within(300 * time.Millisecond).Assert()
}

func TestStageRecordsMetaLogFields(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("logs")
	pid := n.Spawn(factoryLogActor, gen.ProcessOptions{})

	result, err := n.Call(pid, "meta")
	check.NoError(t, err)
	alias := result.(gen.Alias)

	n.ShouldLog().FromMeta(alias).From(pid).Containing("meta line").
		WithField("meta", "log").Once().Within(time.Second).Must()
	n.ShouldLog().FromMeta(alias).WithFieldName("actor").
		None().Within(300 * time.Millisecond).Assert()
}

func TestStageLogLevelOption(t *testing.T) {
	s := stage.New(t)

	quiet := s.StartNode("quiet")
	pid := quiet.Spawn(factoryLogActor, gen.ProcessOptions{})
	quiet.Send(pid, "debug")
	quiet.ShouldLog().Containing("debug line").None().Within(300 * time.Millisecond).Assert()

	verbose := s.StartNode("verbose", stage.NodeOptions{LogLevel: gen.LogLevelDebug})
	pid = verbose.Spawn(factoryLogActor, gen.ProcessOptions{})
	verbose.Send(pid, "debug")
	verbose.ShouldLog().Level(gen.LogLevelDebug).Containing("debug line").
		WithField("actor", "log").Once().Within(time.Second).Must()
}

func TestStageDoesNotRecordRedirectedLines(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("logs")
	pid := n.Spawn(factoryLogActor, gen.ProcessOptions{})

	n.Send(pid, "redirected")
	n.ShouldLog().Containing("redirected line").None().Within(300 * time.Millisecond).Assert()
}
