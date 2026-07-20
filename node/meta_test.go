package node

import (
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/mock"
)

func newTestMeta(core gen.Core) *meta {
	p := &process{
		pid:  gen.PID{Node: "n@localhost", ID: 100, Creation: 1},
		core: core,
	}
	m := &meta{p: p}
	m.state = int32(gen.MetaStateRunning)
	return m
}

// SendResponse must carry the request ref so the caller can correlate the reply.
func TestMetaSendResponseCarriesRef(t *testing.T) {
	core := mock.NewCoreT(t)
	var gotRef gen.Ref
	var gotMsg any
	core.OnRouteSendResponse(func(from, to gen.PID, opts gen.MessageOptions, message any) error {
		gotRef, gotMsg = opts.Ref, message
		return nil
	})

	m := newTestMeta(core)
	ref := gen.Ref{Node: "n@localhost", Creation: 1, ID: [3]uint64{7, 0, 0}}
	if err := m.SendResponse(gen.PID{Node: "n@localhost", ID: 5, Creation: 1}, ref, "result"); err != nil {
		t.Fatal(err)
	}

	check.Equal(t, ref, gotRef)
	check.Equal(t, "result", gotMsg)
}

// SendResponseError must route through the error path (not the success path) and
// carry the request ref.
func TestMetaSendResponseErrorUsesErrorRoute(t *testing.T) {
	core := mock.NewCoreT(t)
	errRoute := false
	okRoute := false
	var gotRef gen.Ref
	var gotErr error
	core.OnRouteSendResponseError(func(from, to gen.PID, opts gen.MessageOptions, err error) error {
		errRoute, gotRef, gotErr = true, opts.Ref, err
		return nil
	})
	core.OnRouteSendResponse(func(from, to gen.PID, opts gen.MessageOptions, message any) error {
		okRoute = true
		return nil
	})

	m := newTestMeta(core)
	ref := gen.Ref{Node: "n@localhost", Creation: 1, ID: [3]uint64{9, 0, 0}}
	if err := m.SendResponseError(gen.PID{Node: "n@localhost", ID: 5, Creation: 1}, ref, gen.ErrProcessUnknown); err != nil {
		t.Fatal(err)
	}

	check.True(t, errRoute)
	check.False(t, okRoute)
	check.Equal(t, ref, gotRef)
	check.ErrorIs(t, gotErr, gen.ErrProcessUnknown)
}
