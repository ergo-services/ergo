package local

import (
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

type askMeta struct {
	gen.MetaProcess
	collector gen.PID
	startAsk  *metaAsk
	stop      chan struct{}
}

type metaAsk struct {
	To        any
	Request   any
	Label     any
	Timeout   int
	Important bool
}

type metaAskCancel struct {
	To      any
	Request any
}

type metaAnswer struct {
	From   gen.PID
	Label  any
	Result any
	Err    string
}

type metaAskResult struct {
	Label any
	Err   string
}

func (m *askMeta) Init(meta gen.MetaProcess) error {
	m.MetaProcess = meta
	return nil
}

func (m *askMeta) Start() error {
	if m.startAsk != nil {
		m.ask(*m.startAsk)
	}
	<-m.stop
	return nil
}

func (m *askMeta) HandleMessage(from gen.PID, message any) error {
	switch c := message.(type) {
	case metaAsk:
		m.ask(c)
	case metaAskCancel:
		ref, err := m.SendRequest(c.To, c.Request)
		if err == nil {
			err = m.CancelRequest(ref)
		}
		m.Send(m.collector, metaAskResult{Label: "cancel", Err: errText(err)})
	}
	return nil
}

func (m *askMeta) ask(c metaAsk) {
	_, err := m.SendRequestWithOptions(c.To, c.Request, gen.RequestOptions{
		Label:     c.Label,
		Timeout:   c.Timeout,
		Important: c.Important,
	})
	if err != nil {
		m.Send(m.collector, metaAskResult{Label: c.Label, Err: errText(err)})
	}
}

func (m *askMeta) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return request, nil
}

func (m *askMeta) HandleResponse(response gen.MessageResponse) error {
	return m.Send(m.collector, metaAnswer{
		From:   response.From,
		Label:  response.Label,
		Result: response.Result,
		Err:    errText(response.Error),
	})
}

func (m *askMeta) HandleInspect(from gen.PID, item ...string) map[string]string { return nil }

func (m *askMeta) Terminate(reason error) { close(m.stop) }

type askHost struct {
	act.Actor
	alias gen.Alias
}

func factoryAskHost() gen.ProcessBehavior { return &askHost{} }

type spawnAskMeta struct {
	Collector gen.PID
	StartAsk  *metaAsk
}
type hostAliasCmd struct{}
type exitAskMeta struct{ Alias gen.Alias }

func (h *askHost) Init(args ...any) error {
	alias, err := h.CreateAlias()
	h.alias = alias
	return err
}

func (h *askHost) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	switch c := request.(type) {
	case spawnAskMeta:
		meta := &askMeta{collector: c.Collector, startAsk: c.StartAsk, stop: make(chan struct{})}
		id, err := h.SpawnMeta(meta, gen.MetaOptions{})
		if err != nil {
			return err, nil
		}
		return id, nil
	case hostAliasCmd:
		return h.alias, nil
	case exitAskMeta:
		return errText(h.SendExitMeta(c.Alias, gen.TerminateReasonShutdown)), nil
	}
	return "host", nil
}

type holdServer struct {
	act.Actor
	reporter gen.PID
	from     gen.PID
	ref      gen.Ref
}

type holdReleased struct{ Err string }

func factoryHoldServer() gen.ProcessBehavior { return &holdServer{} }

func (s *holdServer) Init(args ...any) error {
	s.reporter = args[0].(gen.PID)
	return nil
}

func (s *holdServer) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	s.from, s.ref = from, ref
	s.Send(s.reporter, "held")
	return nil, nil
}

func (s *holdServer) HandleMessage(from gen.PID, message any) error {
	if message == "release" {
		err := s.SendResponse(s.from, s.ref, "late")
		s.Send(s.reporter, holdReleased{Err: errText(err)})
	}
	return nil
}

type importantServer struct {
	act.Actor
	reporter gen.PID
}

type importantSent struct{ Err string }

func factoryImportantServer() gen.ProcessBehavior { return &importantServer{} }

func (s *importantServer) Init(args ...any) error {
	s.reporter = args[0].(gen.PID)
	return nil
}

func (s *importantServer) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	err := s.SendResponseImportant(from, ref, request)
	s.Send(s.reporter, importantSent{Err: errText(err)})
	return nil, nil
}

func waitMetaGone(t *testing.T, n *stage.Node, meta gen.Alias) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := n.Native().MetaInfo(meta); err != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("meta %s is still alive", meta)
}

func spawnAskMetaOn(t *testing.T, n *stage.Node, host gen.PID, collector gen.PID, startAsk *metaAsk) gen.Alias {
	t.Helper()
	v, err := n.Call(host, spawnAskMeta{Collector: collector, StartAsk: startAsk})
	check.NoError(t, err)
	alias, ok := v.(gen.Alias)
	check.True(t, ok)
	return alias
}

// TestMetaRequest: a meta process makes a request without blocking and gets the
// answer in HandleResponse. The request is made with the voice of its parent;
// the response comes to the parent PID and the reply-to in the ref brings it to
// the meta, never to the parent.
func TestMetaRequest(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("metarequest")

	host := n.SpawnRegister("askhost", factoryAskHost, gen.ProcessOptions{})
	collector := n.Spawn(factoryEcho, gen.ProcessOptions{})
	server := n.SpawnRegister("server", factorySlowServer, gen.ProcessOptions{})

	t.Run("Answered", func(t *testing.T) {
		meta := spawnAskMetaOn(t, n, host, collector, nil)
		mk := n.Mark()
		n.Send(meta, metaAsk{To: gen.Atom("server"), Request: "ping", Label: "first"})
		n.ShouldDeliver().To(collector).
			Message(metaAnswer{From: server, Label: "first", Result: "pong"}).
			Since(mk).Once().Within(2 * time.Second).Must()
		n.ShouldDeliver().To(host).Message("pong").Since(mk).None().Assert()
	})

	t.Run("AnsweredWithError", func(t *testing.T) {
		meta := spawnAskMetaOn(t, n, host, collector, nil)
		mk := n.Mark()
		n.Send(meta, metaAsk{To: server, Request: "fail", Label: 42})
		n.ShouldDeliver().To(collector).
			Message(metaAnswer{From: server, Label: 42, Err: errText(gen.ErrUnsupported)}).
			Since(mk).Once().Within(2 * time.Second).Must()
	})

	t.Run("TimedOut", func(t *testing.T) {
		meta := spawnAskMetaOn(t, n, host, collector, nil)
		mk := n.Mark()
		n.Send(meta, metaAsk{To: server, Request: "silence", Label: "late", Timeout: 1})
		n.ShouldDeliver().To(collector).
			Message(metaAnswer{Label: "late", Err: errText(gen.ErrTimeout)}).
			Since(mk).Once().Within(3 * time.Second).Must()
	})

	t.Run("Cancelled", func(t *testing.T) {
		meta := spawnAskMetaOn(t, n, host, collector, nil)
		mk := n.Mark()
		n.Send(meta, metaAskCancel{To: server, Request: "ping"})
		n.ShouldDeliver().To(collector).Message(metaAskResult{Label: "cancel"}).
			Since(mk).Once().Within(2 * time.Second).Must()
		n.ShouldDeliver().To(collector).
			Where(func(r check.Delivered) bool { _, ok := r.Message.(metaAnswer); return ok }).
			Since(mk).None().Within(time.Second).Assert()
	})

	t.Run("FromStart", func(t *testing.T) {
		mk := n.Mark()
		spawnAskMetaOn(t, n, host, collector, &metaAsk{To: server, Request: "ping", Label: "reader"})
		n.ShouldDeliver().To(collector).
			Message(metaAnswer{From: server, Label: "reader", Result: "pong"}).
			Since(mk).Once().Within(2 * time.Second).Must()
	})

	t.Run("ToSiblingMeta", func(t *testing.T) {
		meta := spawnAskMetaOn(t, n, host, collector, nil)
		sibling := spawnAskMetaOn(t, n, host, collector, nil)
		mk := n.Mark()
		n.Send(meta, metaAsk{To: sibling, Request: "hello", Label: "sibling"})
		n.ShouldDeliver().To(collector).
			Message(metaAnswer{From: host, Label: "sibling", Result: "hello"}).
			Since(mk).Once().Within(2 * time.Second).Must()
	})

	t.Run("NotAllowed", func(t *testing.T) {
		start := n.Mark()
		meta := spawnAskMetaOn(t, n, host, collector, nil)
		v, err := n.Call(host, hostAliasCmd{})
		check.NoError(t, err)
		hostAlias := v.(gen.Alias)

		targets := map[string]any{
			"parent-pid":   host,
			"parent-name":  gen.Atom("askhost"),
			"parent-pname": gen.ProcessID{Name: "askhost", Node: n.Name()},
			"parent-alias": hostAlias,
			"own-alias":    meta,
		}
		for label, to := range targets {
			mk := n.Mark()
			n.Send(meta, metaAsk{To: to, Request: "anything", Label: label})
			n.ShouldDeliver().To(collector).
				Message(metaAskResult{Label: label, Err: errText(gen.ErrNotAllowed)}).
				Since(mk).Once().Within(2 * time.Second).Must()
		}
		n.ShouldDeliver().To(collector).
			Where(func(r check.Delivered) bool { _, ok := r.Message.(metaAnswer); return ok }).
			Since(start).None().Within(300 * time.Millisecond).Assert()
	})

	t.Run("ImportantResponse", func(t *testing.T) {
		meta := spawnAskMetaOn(t, n, host, collector, nil)
		important := n.Spawn(factoryImportantServer, gen.ProcessOptions{}, collector)
		mk := n.Mark()
		n.Send(meta, metaAsk{To: important, Request: "ack-me", Label: "important"})
		n.ShouldDeliver().To(collector).
			Message(metaAnswer{From: important, Label: "important", Result: "ack-me"}).
			Since(mk).Once().Within(2 * time.Second).Must()
		n.ShouldDeliver().To(collector).Message(importantSent{}).
			Since(mk).Once().Within(2 * time.Second).Must()
	})

	t.Run("AnswerAfterTermination", func(t *testing.T) {
		meta := spawnAskMetaOn(t, n, host, collector, nil)
		hold := n.Spawn(factoryHoldServer, gen.ProcessOptions{}, collector)
		mk := n.Mark()
		n.Send(meta, metaAsk{To: hold, Request: "keep", Label: "orphan", Timeout: 30})
		n.ShouldDeliver().To(collector).Message("held").Since(mk).Once().Within(2 * time.Second).Must()

		v, err := n.Call(host, exitAskMeta{Alias: meta})
		check.NoError(t, err)
		check.Equal(t, "", v)
		waitMetaGone(t, n, meta)

		n.Send(hold, "release")
		n.ShouldDeliver().To(collector).
			Message(holdReleased{Err: errText(gen.ErrProcessUnknown)}).
			Since(mk).Once().Within(2 * time.Second).Must()
		n.ShouldDeliver().To(collector).
			Where(func(r check.Delivered) bool { _, ok := r.Message.(metaAnswer); return ok }).
			Since(mk).None().Within(300 * time.Millisecond).Assert()
	})
}

type selfCaller struct {
	act.Actor
	alias gen.Alias
}

type callSelf struct{ By string }

func factorySelfCaller() gen.ProcessBehavior { return &selfCaller{} }

func (c *selfCaller) Init(args ...any) error {
	alias, err := c.CreateAlias()
	c.alias = alias
	return err
}

func (c *selfCaller) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	cmd, ok := request.(callSelf)
	if ok == false {
		return request, nil
	}
	var err error
	switch cmd.By {
	case "name":
		_, err = c.Call(gen.Atom("selfcaller"), "hello")
	case "alias":
		_, err = c.Call(c.alias, "hello")
	case "request-name":
		_, err = c.SendRequest(gen.Atom("selfcaller"), "hello")
	}
	return errText(err), nil
}

// TestCallSelf: a process calling itself by its own name or alias is turned down
// at once, the way a call to its own PID always was, instead of waiting out the
// timeout on a request it can never handle.
func TestCallSelf(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("callself")
	caller := n.SpawnRegister("selfcaller", factorySelfCaller, gen.ProcessOptions{})

	for _, by := range []string{"name", "alias", "request-name"} {
		started := time.Now()
		v, err := n.Call(caller, callSelf{By: by})
		check.NoError(t, err)
		check.Equal(t, errText(gen.ErrNotAllowed), v)
		check.True(t, time.Since(started) < time.Second)
	}
}
