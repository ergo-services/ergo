package distributed

import (
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

type distMeta struct {
	gen.MetaProcess
	collector gen.PID
	stop      chan struct{}
}

type distMetaAsk struct {
	To    any
	Label any
}

type distMetaAnswer struct {
	From   gen.PID
	Label  any
	Result any
	Err    string
}

func (m *distMeta) Init(meta gen.MetaProcess) error {
	m.MetaProcess = meta
	return nil
}

func (m *distMeta) Start() error {
	<-m.stop
	return nil
}

func (m *distMeta) HandleMessage(from gen.PID, message any) error {
	if c, ok := message.(distMetaAsk); ok {
		if _, err := m.SendRequestWithLabel(c.To, "ping", c.Label); err != nil {
			m.Send(m.collector, distMetaAnswer{Label: c.Label, Err: errText(err)})
		}
	}
	return nil
}

func (m *distMeta) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, nil
}

func (m *distMeta) HandleResponse(response gen.MessageResponse) error {
	return m.Send(m.collector, distMetaAnswer{
		From:   response.From,
		Label:  response.Label,
		Result: response.Result,
		Err:    errText(response.Error),
	})
}

func (m *distMeta) HandleInspect(from gen.PID, item ...string) map[string]string { return nil }

func (m *distMeta) Terminate(reason error) { close(m.stop) }

type distMetaHost struct{ act.Actor }

func factoryDistMetaHost() gen.ProcessBehavior { return &distMetaHost{} }

func (h *distMetaHost) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	collector := request.(gen.PID)
	id, err := h.SpawnMeta(&distMeta{collector: collector, stop: make(chan struct{})}, gen.MetaOptions{})
	if err != nil {
		return err, nil
	}
	return id, nil
}

// TestDistMetaRequest: a meta process asks a process on another node. The
// request crosses the wire with the parent PID as the requester, the response
// comes back to that PID, and the reply-to in the ref brings it to the meta. An
// important response is acknowledged by the meta's node: with the ack frame when
// the responder's node accepts it, as a response error when it does not, and
// either way the responder gets its ack and the meta gets one answer only.
func TestDistMetaRequest(t *testing.T) {
	legacy := gen.DefaultNetworkFlags
	legacy.EnableAck = false

	cases := []struct {
		name      string
		requester gen.NetworkFlags
		responder gen.NetworkFlags
	}{
		{"Ack", gen.DefaultNetworkFlags, gen.DefaultNetworkFlags},
		{"LegacyResponder", gen.DefaultNetworkFlags, legacy},
		{"LegacyRequester", legacy, gen.DefaultNetworkFlags},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := stage.New(t)
			n1 := s.StartNode("n1", stage.NodeOptions{NetworkFlags: c.requester})
			n2 := s.StartNode("n2", stage.NodeOptions{NetworkFlags: c.responder})
			s.Connect(n1, n2)

			remote, err := n1.Native().Network().Node(n2.Name())
			check.NoError(t, err)
			check.Equal(t, c.responder.EnableAck, remote.Info().NetworkFlags.EnableAck)

			collector := n1.Spawn(factorySpawnable, gen.ProcessOptions{})
			host := n1.Spawn(factoryDistMetaHost, gen.ProcessOptions{})
			v, err := n1.Call(host, collector)
			check.NoError(t, err)
			meta := v.(gen.Alias)

			pong := n2.Spawn(factoryCallPong, gen.ProcessOptions{})
			mk := n1.Mark()
			n1.Send(meta, distMetaAsk{To: pong, Label: "plain"})
			n1.ShouldDeliver().To(collector).
				Message(distMetaAnswer{From: pong, Label: "plain", Result: "ping"}).
				Since(mk).Once().Within(2 * time.Second).Must()

			important := n2.Spawn(factoryRespImportant, gen.ProcessOptions{}, collector)
			mk = n1.Mark()
			n1.Send(meta, distMetaAsk{To: important, Label: "important"})
			n1.ShouldDeliver().To(collector).
				Message(distMetaAnswer{From: important, Label: "important", Result: "ping"}).
				Since(mk).Once().Within(2 * time.Second).Must()
			n1.ShouldDeliver().To(collector).Message("").Since(mk).Once().Within(2 * time.Second).Must()
			n1.ShouldDeliver().To(collector).
				Where(func(r check.Delivered) bool {
					a, ok := r.Message.(distMetaAnswer)
					return ok && a.Label == "important"
				}).
				Since(mk).Once().Within(300 * time.Millisecond).Assert()

			snd := n1.Spawn(factorySender, gen.ProcessOptions{})
			res, err := n1.Call(snd, sendCmd{Kind: "important", To: pong, Msg: "imp"})
			check.NoError(t, err)
			check.Equal(t, "", res)

			back := n1.Spawn(factoryRespImportant, gen.ProcessOptions{}, collector)
			mk = n1.Mark()
			res, err = n2.Native().CallImportant(back, 7)
			check.NoError(t, err)
			check.Equal(t, 7, res)
			n1.ShouldDeliver().To(collector).Message("").Since(mk).Once().Within(2 * time.Second).Must()
		})
	}
}
