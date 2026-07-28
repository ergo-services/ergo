package node

import (
	"sync"
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

// SetSendPriority is Running-only per its godoc: it stores the priority while
// running and returns ErrNotAllowed once terminated (leaving it unchanged).
func TestMetaSetSendPriorityRunningOnly(t *testing.T) {
	m := newTestMeta(mock.NewCore())

	if err := m.SetSendPriority(gen.MessagePriorityHigh); err != nil {
		t.Fatalf("running: %v", err)
	}
	if got := gen.MessagePriority(m.priority.Load()); got != gen.MessagePriorityHigh {
		t.Fatalf("priority = %v, want High", got)
	}

	m.state = int32(gen.MetaStateTerminated)
	if err := m.SetSendPriority(gen.MessagePriorityMax); err != gen.ErrNotAllowed {
		t.Fatalf("terminated: expected ErrNotAllowed, got %v", err)
	}
	if got := gen.MessagePriority(m.priority.Load()); got != gen.MessagePriorityHigh {
		t.Fatalf("priority mutated in terminated state: got %v", got)
	}
}

// SetCompression is Running-only per its godoc, and Compression reflects the stored value.
func TestMetaSetCompressionRunningOnly(t *testing.T) {
	m := newTestMeta(mock.NewCore())

	if m.Compression() == true {
		t.Fatal("compression should default to false")
	}
	if err := m.SetCompression(true); err != nil {
		t.Fatalf("running: %v", err)
	}
	if m.Compression() == false {
		t.Fatal("compression not stored")
	}

	m.state = int32(gen.MetaStateTerminated)
	if err := m.SetCompression(false); err != gen.ErrNotAllowed {
		t.Fatalf("terminated: expected ErrNotAllowed, got %v", err)
	}
	if m.Compression() == false {
		t.Fatal("compression mutated in terminated state")
	}
}

// SetCompression from an external goroutine must not race a concurrent sender reading it.
// Run with -race; the field is atomic.Bool.
func TestMetaCompressionConcurrent(t *testing.T) {
	core := mock.NewCore()
	core.OnRouteSendResponse(func(from, to gen.PID, opts gen.MessageOptions, message any) error {
		return nil
	})
	m := newTestMeta(core)
	ref := gen.Ref{Node: "n@localhost", Creation: 1, ID: [3]uint64{1, 0, 0}}
	to := gen.PID{Node: "n@localhost", ID: 5, Creation: 1}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 10000; i++ {
			m.SetCompression(i%2 == 0)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 10000; i++ {
			m.SendResponse(to, ref, "x")
		}
	}()
	wg.Wait()
}
