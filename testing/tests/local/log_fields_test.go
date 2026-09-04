package local

import (
	"testing"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

type logQuery struct {
	Kind   string
	Fields []gen.LogField
	Names  []string
	Logger string
}

type logProbe struct{ act.Actor }

func factoryLogProbe() gen.ProcessBehavior { return &logProbe{} }

func (l *logProbe) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	q, ok := request.(logQuery)
	if ok == false {
		return probeResult{}, nil
	}

	switch q.Kind {
	case "fields":
		return probeResult{Value: l.Log().Fields()}, nil
	case "add":
		l.Log().AddFields(q.Fields...)
		return probeResult{Value: l.Log().Fields()}, nil
	case "delete":
		l.Log().DeleteFields(q.Names...)
		return probeResult{Value: l.Log().Fields()}, nil
	case "push":
		return probeResult{Value: l.Log().PushFields()}, nil
	case "pop":
		return probeResult{Value: l.Log().PopFields()}, nil
	case "setlogger":
		l.Log().SetLogger(q.Logger)
		return probeResult{Value: l.Log().Logger()}, nil
	}
	return probeResult{}, nil
}

func askLog(t *testing.T, n *stage.Node, pid gen.PID, q logQuery) any {
	t.Helper()
	result, err := n.Call(pid, q)
	check.NoError(t, err)
	return result.(probeResult).Value
}

func fieldNames(v any) []string {
	fields, _ := v.([]gen.LogField)
	names := make([]string, 0, len(fields))
	for _, f := range fields {
		names = append(names, f.Name)
	}
	return names
}

func TestLogFields(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	pid := n.Spawn(factoryLogProbe, gen.ProcessOptions{})

	check.Equal(t, 0, len(fieldNames(askLog(t, n, pid, logQuery{Kind: "fields"}))))

	added := askLog(t, n, pid, logQuery{Kind: "add", Fields: []gen.LogField{
		{Name: "order", Value: 1},
		{Name: "user", Value: "bob"},
	}})
	check.Equal(t, []string{"order", "user"}, fieldNames(added))

	kept := askLog(t, n, pid, logQuery{Kind: "delete", Names: []string{"order"}})
	check.Equal(t, []string{"user"}, fieldNames(kept))

	check.Equal(t, []string{"user"}, fieldNames(askLog(t, n, pid, logQuery{Kind: "delete"})))

	check.Equal(t, 0, len(fieldNames(askLog(t, n, pid, logQuery{Kind: "delete", Names: []string{"user"}}))))
}

func TestLogFieldStack(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	pid := n.Spawn(factoryLogProbe, gen.ProcessOptions{})

	check.Equal(t, 0, askLog(t, n, pid, logQuery{Kind: "pop"}))

	askLog(t, n, pid, logQuery{Kind: "add", Fields: []gen.LogField{{Name: "base", Value: 1}}})
	check.Equal(t, 1, askLog(t, n, pid, logQuery{Kind: "push"}))

	askLog(t, n, pid, logQuery{Kind: "add", Fields: []gen.LogField{{Name: "scoped", Value: 2}}})
	check.Equal(t, []string{"base", "scoped"}, fieldNames(askLog(t, n, pid, logQuery{Kind: "fields"})))

	check.Equal(t, []string{"base", "scoped"}, fieldNames(askLog(t, n, pid, logQuery{Kind: "delete", Names: []string{"scoped"}})))

	check.Equal(t, 0, askLog(t, n, pid, logQuery{Kind: "pop"}))
	check.Equal(t, []string{"base"}, fieldNames(askLog(t, n, pid, logQuery{Kind: "fields"})))
}

func TestLogLoggerName(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	pid := n.Spawn(factoryLogProbe, gen.ProcessOptions{})

	check.Equal(t, "custom", askLog(t, n, pid, logQuery{Kind: "setlogger", Logger: "custom"}))
}

func TestAcceptorInfo(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("n")
	net := n.Native().Network()

	acceptors, err := net.Acceptors()
	check.NoError(t, err)
	if len(acceptors) == 0 {
		t.Fatal("a node with the network enabled has no acceptor")
	}
	a := acceptors[0]

	check.Equal(t, net.MaxMessageSize(), a.MaxMessageSize())
	check.True(t, a.NetworkFlags().Enable)

	info := a.Info()
	check.Equal(t, a.MaxMessageSize(), info.MaxMessageSize)
	check.Equal(t, a.NetworkFlags(), info.Flags)
	check.True(t, info.Interface != "")
	check.True(t, info.HandshakeVersion.Name != "")
	check.True(t, info.ProtoVersion.Name != "")
	check.True(t, info.TLS == false)
}
