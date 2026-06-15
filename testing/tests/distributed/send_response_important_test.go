package distributed

import (
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/net/edf"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// respWrapped carries (From, Ref, Msg) across nodes so a final responder can reply
// to the original caller. EDF keeps the embedded Ref's Node atom intact, so the ref
// reaches the final node exactly as the caller generated it.
type respWrapped struct {
	From gen.PID
	Ref  gen.Ref
	Msg  int
}

func init() {
	if err := edf.RegisterTypeOf(respWrapped{}); err != nil && err != gen.ErrTaken {
		panic(err)
	}
}

// respImportant replies via SendResponseImportant from inside HandleCall and reports
// that call's return value (as text) to a reporter, then defers the reply.
type respImportant struct {
	act.Actor
	reporter gen.PID
}

func factoryRespImportant() gen.ProcessBehavior { return &respImportant{} }

func (p *respImportant) Init(args ...any) error {
	p.reporter = args[0].(gen.PID)
	return nil
}

func (p *respImportant) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	err := p.SendResponseImportant(from, ref, request)
	p.Send(p.reporter, errText(err))
	if err != nil {
		return nil, err
	}
	return nil, nil
}

// respErrImportant replies via SendResponseErrorImportant.
type respErrImportant struct {
	act.Actor
	reporter gen.PID
}

func factoryRespErrImportant() gen.ProcessBehavior { return &respErrImportant{} }

func (p *respErrImportant) Init(args ...any) error {
	p.reporter = args[0].(gen.PID)
	return nil
}

func (p *respErrImportant) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	err := p.SendResponseErrorImportant(from, ref, gen.ErrTaken)
	p.Send(p.reporter, errText(err))
	if err != nil {
		return nil, err
	}
	return nil, nil
}

// respForwarder captures the call into respWrapped and forwards it to a final
// responder on a third node, deferring the reply.
type respForwarder struct {
	act.Actor
	to gen.PID
}

func factoryRespForwarder() gen.ProcessBehavior { return &respForwarder{} }

func (f *respForwarder) Init(args ...any) error {
	f.to = args[0].(gen.PID)
	return nil
}

func (f *respForwarder) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	f.Send(f.to, respWrapped{From: from, Ref: ref, Msg: request.(int)})
	return nil, nil
}

// respFinal replies to the original caller carried in the wrapper.
type respFinal struct {
	act.Actor
	reporter gen.PID
}

func factoryRespFinal() gen.ProcessBehavior { return &respFinal{} }

func (r *respFinal) Init(args ...any) error {
	r.reporter = args[0].(gen.PID)
	return nil
}

func (r *respFinal) HandleMessage(from gen.PID, message any) error {
	w := message.(respWrapped)
	err := r.SendResponseImportant(w.From, w.Ref, w.Msg)
	r.Send(r.reporter, errText(err))
	return err
}

// TestDistSendResponseImportant: a deferred important reply must have its ack
// matched on the responder's side. The caller's Call returns as soon as the
// response arrives, so a broken ack match is invisible to it; the responder, which
// waits for the ack, is the one that would block until ErrTimeout. Each responder
// reports the return value of SendResponseImportant: an empty status text means the
// ack matched immediately. Covers a direct response, an error response, and a reply
// deferred across a third node (the ref is preserved through EDF, never
// reconstructed on the request path at the final node).
func TestDistSendResponseImportant(t *testing.T) {
	s := stage.New(t)
	n1 := s.Node("n1")
	n2 := s.Node("n2")
	n3 := s.Node("n3")
	s.Connect(n1, n2)
	s.Connect(n1, n3)
	s.Connect(n2, n3)

	reporter := n1.Spawn(factorySpawnable, gen.ProcessOptions{})

	// the responder's SendResponseImportant returned nil (ack matched) within 2s;
	// with the ack-match bug the status text would be a timeout arriving after ~5s
	expectOK := func(mk int) {
		n1.ShouldDeliver().To(reporter).Message("").Since(mk).Once().Within(2 * time.Second).Must()
	}

	t.Run("Direct", func(t *testing.T) {
		pong := n2.Spawn(factoryRespImportant, gen.ProcessOptions{}, reporter)
		mk := n1.Mark()
		res, err := n1.Call(pong, 123)
		check.NoError(t, err)
		check.Equal(t, 123, res)
		expectOK(mk)
	})

	t.Run("Error", func(t *testing.T) {
		pong := n2.Spawn(factoryRespErrImportant, gen.ProcessOptions{}, reporter)
		mk := n1.Mark()
		_, err := n1.Call(pong, 123)
		check.True(t, err == gen.ErrTaken)
		expectOK(mk)
	})

	t.Run("DeferredReply", func(t *testing.T) {
		final := n3.Spawn(factoryRespFinal, gen.ProcessOptions{}, reporter)
		forwarder := n2.Spawn(factoryRespForwarder, gen.ProcessOptions{}, final)
		mk := n1.Mark()
		res, err := n1.Call(forwarder, 456)
		check.NoError(t, err)
		check.Equal(t, 456, res)
		expectOK(mk)
	})
}
