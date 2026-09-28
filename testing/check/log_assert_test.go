package check_test

import (
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
)

func TestLogAssertFilters(t *testing.T) {
	rec := check.NewRecorder()
	a := check.NewAsserter(t, rec)

	pid := gen.PID{Node: "n@localhost", ID: 1000, Creation: 7}
	alias := gen.Alias{Node: "n@localhost", Creation: 7, ID: [3]uint64{5, 0, 0}}

	rec.Put(check.Log{
		From:    pid,
		Level:   gen.LogLevelInfo,
		Message: "order 7 placed",
		Format:  "order %d placed",
		Args:    []any{7},
		Fields:  []gen.LogField{{Name: "order", Value: 7}, {Name: "user", Value: "bob"}},
		Source:  gen.MessageLogProcess{Node: "n@localhost", PID: pid, Name: "trader"},
	})
	rec.Put(check.Log{
		From:    pid,
		Level:   gen.LogLevelError,
		Message: "order 7 rejected",
		Format:  "order %d rejected",
		Args:    []any{7},
		Fields:  []gen.LogField{{Name: "order", Value: 7}},
		Source:  gen.MessageLogMeta{Node: "n@localhost", Parent: pid, Meta: alias},
	})
	rec.Put(check.Log{
		Level:   gen.LogLevelWarning,
		Message: "node is busy",
		Format:  "node is busy",
		Source:  gen.MessageLogNode{Node: "n@localhost", Creation: 7},
	})

	a.ShouldLog().Times(3).Assert()

	a.ShouldLog().WithField("order", 7).Times(2).Assert()
	a.ShouldLog().WithField("order", "7").None().Assert()
	a.ShouldLog().WithFieldName("user").Once().Assert()
	a.ShouldLog().WithFields(
		gen.LogField{Name: "order", Value: 7},
		gen.LogField{Name: "user", Value: "bob"},
	).Once().Assert()
	a.ShouldLog().WithFields(
		gen.LogField{Name: "order", Value: 7},
		gen.LogField{Name: "missing", Value: 1},
	).None().Assert()

	a.ShouldLog().Format("order %d placed").Once().Assert()
	a.ShouldLog().Format("order 7 placed").None().Assert()

	a.ShouldLog().FromMeta(alias).Once().Assert()
	a.ShouldLog().FromMeta(gen.Alias{}).None().Assert()
	a.ShouldLog().FromNode().Once().Assert()

	a.ShouldLog().From(pid).Level(gen.LogLevelError).FromMeta(alias).
		WithField("order", 7).Once().Assert()
}

func TestLogRecordString(t *testing.T) {
	plain := check.Log{Level: gen.LogLevelInfo, Message: "hello"}
	check.Equal(t, false, containsFields(plain.String()))

	withFields := check.Log{
		Level:   gen.LogLevelInfo,
		Message: "hello",
		Fields:  []gen.LogField{{Name: "a", Value: 1}},
	}
	check.Equal(t, true, containsFields(withFields.String()))
	check.Equal(t, "logged", check.Log{}.Kind())
}

func containsFields(s string) bool {
	for i := 0; i+len("fields=") <= len(s); i++ {
		if s[i:i+len("fields=")] == "fields=" {
			return true
		}
	}
	return false
}
