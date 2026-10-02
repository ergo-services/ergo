package local

import (
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

type requestActor struct {
	act.Actor

	answers chan gen.MessageResponse
}

func factoryRequestActor() gen.ProcessBehavior { return &requestActor{} }

func (a *requestActor) Init(args ...any) error {
	a.answers = args[0].(chan gen.MessageResponse)
	return nil
}

func (a *requestActor) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case askOnce:
		_, err := a.SendRequestWithOptions(m.to, m.request, gen.RequestOptions{
			Label:   m.label,
			Timeout: m.timeout,
		})
		return err
	case askAndCancel:
		ref, err := a.SendRequest(m.to, m.request)
		if err != nil {
			return err
		}
		return a.CancelRequest(ref)
	}
	return nil
}

func (a *requestActor) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	if request == "alive" {
		return "yes", nil
	}
	return nil, nil
}

func (a *requestActor) HandleResponse(response gen.MessageResponse) error {
	a.answers <- response
	return nil
}

type askOnce struct {
	to      any
	request any
	label   any
	timeout int
}

type askAndCancel struct {
	to      any
	request any
}

type slowServer struct {
	act.Actor
}

func factorySlowServer() gen.ProcessBehavior { return &slowServer{} }

func (s *slowServer) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	switch request {
	case "ping":
		return "pong", nil
	case "fail":
		return nil, s.SendResponseError(from, ref, gen.ErrUnsupported)
	}
	return nil, nil
}

func waitAnswer(t *testing.T, answers chan gen.MessageResponse) gen.MessageResponse {
	t.Helper()
	select {
	case response := <-answers:
		return response
	case <-time.After(5 * time.Second):
		t.Fatal("no response delivered")
	}
	return gen.MessageResponse{}
}

func TestRequestAnswered(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("request")

	answers := make(chan gen.MessageResponse, 1)
	caller := n.Spawn(factoryRequestActor, gen.ProcessOptions{}, answers)
	server := n.SpawnRegister("server", factorySlowServer, gen.ProcessOptions{})

	n.Send(caller, askOnce{to: gen.Atom("server"), request: "ping", label: "first"})

	response := waitAnswer(t, answers)
	check.NoError(t, response.Error)
	check.Equal(t, "pong", response.Result)
	check.Equal(t, "first", response.Label)
	check.Equal(t, server, response.From)

	n.ShouldSendRequest().From(caller).To(gen.Atom("server")).Request("ping").
		Label("first").Once().Within(time.Second).Must()
}

func TestRequestAnsweredWithError(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("request")

	answers := make(chan gen.MessageResponse, 1)
	caller := n.Spawn(factoryRequestActor, gen.ProcessOptions{}, answers)
	n.SpawnRegister("server", factorySlowServer, gen.ProcessOptions{})

	n.Send(caller, askOnce{to: gen.Atom("server"), request: "fail", label: 42})

	response := waitAnswer(t, answers)
	check.ErrorIs(t, response.Error, gen.ErrUnsupported)
	check.Equal(t, 42, response.Label)
}

func TestRequestTimedOut(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("request")

	answers := make(chan gen.MessageResponse, 1)
	caller := n.Spawn(factoryRequestActor, gen.ProcessOptions{}, answers)
	n.SpawnRegister("server", factorySlowServer, gen.ProcessOptions{})

	n.Send(caller, askOnce{to: gen.Atom("server"), request: "silence", label: "late", timeout: 1})

	response := waitAnswer(t, answers)
	check.ErrorIs(t, response.Error, gen.ErrTimeout)
	check.Equal(t, "late", response.Label)
	check.Equal(t, nil, response.Result)
}

func TestRequestToUnknownProcess(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("request")

	answers := make(chan gen.MessageResponse, 1)
	caller := n.Spawn(factoryRequestActor, gen.ProcessOptions{}, answers)

	n.Send(caller, askOnce{to: gen.Atom("nobody"), request: "ping", label: nil})

	n.ShouldSendRequest().From(caller).To(gen.Atom("nobody")).
		ErrorIs(gen.ErrProcessUnknown).Once().Within(time.Second).Must()

	select {
	case response := <-answers:
		t.Fatalf("a request that was never sent got an answer: %#v", response)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestRequestCancelled(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("request")

	answers := make(chan gen.MessageResponse, 1)
	caller := n.Spawn(factoryRequestActor, gen.ProcessOptions{}, answers)
	n.SpawnRegister("server", factorySlowServer, gen.ProcessOptions{})

	n.Send(caller, askAndCancel{to: gen.Atom("server"), request: "ping"})

	select {
	case response := <-answers:
		t.Fatalf("a cancelled request got an answer: %#v", response)
	case <-time.After(time.Second):
	}
}

func TestRequestKeepsTheCallerResponsive(t *testing.T) {
	s := stage.New(t)
	n := s.StartNode("request")

	answers := make(chan gen.MessageResponse, 1)
	caller := n.Spawn(factoryRequestActor, gen.ProcessOptions{}, answers)
	n.SpawnRegister("server", factorySlowServer, gen.ProcessOptions{})

	n.Send(caller, askOnce{to: gen.Atom("server"), request: "silence", label: "pending", timeout: 1})

	result, err := n.Call(caller, "alive")
	check.NoError(t, err)
	check.Equal(t, "yes", result)

	response := waitAnswer(t, answers)
	check.ErrorIs(t, response.Error, gen.ErrTimeout)
}
