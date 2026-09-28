package mock_test

import (
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/mock"
)

func TestMockLogFieldsRideAlongWithTheLine(t *testing.T) {
	l := mock.NewLogT(t)
	l.AddFields(gen.LogField{Name: "worker", Value: 3})
	l.Info("handled %d jobs", 7)

	l.ShouldLog().Level(gen.LogLevelInfo).Format("handled %d jobs").
		Containing("handled 7 jobs").WithField("worker", 3).Once().Assert()
}

func TestMockLogFieldScope(t *testing.T) {
	l := mock.NewLogT(t)
	l.AddFields(gen.LogField{Name: "base", Value: 1})

	check.Equal(t, 1, l.PushFields())
	l.AddFields(gen.LogField{Name: "scoped", Value: 2})
	l.Info("inside")
	check.Equal(t, 0, l.PopFields())
	l.Info("outside")

	check.Equal(t, []gen.LogField{{Name: "base", Value: 1}}, l.Fields())

	l.ShouldLog().Containing("inside").WithFields(
		gen.LogField{Name: "base", Value: 1},
		gen.LogField{Name: "scoped", Value: 2},
	).Once().Assert()
	l.ShouldLog().Containing("outside").WithFieldName("scoped").None().Assert()
}

func TestMockLogFieldsDeleted(t *testing.T) {
	l := mock.NewLogT(t)
	l.AddFields(gen.LogField{Name: "keep", Value: "a"}, gen.LogField{Name: "drop", Value: "b"})
	l.DeleteFields("drop")

	check.Equal(t, []gen.LogField{{Name: "keep", Value: "a"}}, l.Fields())

	l.DeleteFields("keep")
	check.Equal(t, 0, len(l.Fields()))
}

func TestMockLogFieldDeleteRefusedWhilePushed(t *testing.T) {
	l := mock.NewLogT(t)
	l.AddFields(gen.LogField{Name: "base", Value: 1})
	l.PushFields()
	l.DeleteFields("base")

	check.Equal(t, []gen.LogField{{Name: "base", Value: 1}}, l.Fields())
	l.ShouldLog().Level(gen.LogLevelError).
		Containing("cannot delete log field(s) while the field stack has 1 active frame(s)").
		Once().Assert()
}

func TestMockLogFieldsReturnsACopy(t *testing.T) {
	l := mock.NewLogT(t)
	l.AddFields(gen.LogField{Name: "base", Value: 1})

	fields := l.Fields()
	fields[0] = gen.LogField{Name: "hijacked", Value: true}

	check.Equal(t, []gen.LogField{{Name: "base", Value: 1}}, l.Fields())
}

func TestMockLogFieldOverridesStillWin(t *testing.T) {
	l := mock.NewLogT(t)
	deleted := []string{}
	l.OnDeleteFields(func(fields ...string) { deleted = append(deleted, fields...) })
	l.OnPushFields(func() int { return 42 })
	l.OnPopFields(func() int { return 41 })

	l.AddFields(gen.LogField{Name: "base", Value: 1})
	l.DeleteFields("base")

	check.Equal(t, []string{"base"}, deleted)
	check.Equal(t, []gen.LogField{{Name: "base", Value: 1}}, l.Fields())
	check.Equal(t, 42, l.PushFields())
	check.Equal(t, 41, l.PopFields())
}
