package act_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/meta"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/unit"
)

// plain web worker: every callback is the act.WebWorker default.
type wwu struct{ act.WebWorker }

func factoryWwu() gen.ProcessBehavior { return &wwu{} }

// webRequest builds a MessageWebRequest for the method and returns it with the
// response recorder.
func webRequest(method string) (meta.MessageWebRequest, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, "/", nil)
	return meta.MessageWebRequest{Response: rec, Request: req, Done: func() {}}, rec
}

// each HTTP method is dispatched to the matching handler; the defaults answer 501.
func TestWebWorkerUnitMethodDispatch(t *testing.T) {
	for _, method := range []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"} {
		s, err := unit.Spawn(t, factoryWwu, gen.ProcessOptions{})
		check.NoError(t, err)
		msg, rec := webRequest(method)
		s.SendMessage(gen.PID{}, msg)
		check.Equal(t, http.StatusNotImplemented, rec.Code) // default handler
		check.False(t, s.Terminated())
	}
}

// an unknown method is answered 501 by the dispatcher itself.
func TestWebWorkerUnitUnknownMethod(t *testing.T) {
	s, _ := unit.Spawn(t, factoryWwu, gen.ProcessOptions{})
	msg, rec := webRequest("TRACE")
	s.SendMessage(gen.PID{}, msg)
	check.Equal(t, http.StatusNotImplemented, rec.Code)
}

// a panicking handler must still call r.Done() (releasing the HTTP goroutine)
// before the worker terminates on the panic.
type wwuPanic struct{ act.WebWorker }

func factoryWwuPanic() gen.ProcessBehavior { return &wwuPanic{} }

func (w *wwuPanic) HandleGet(from gen.PID, writer http.ResponseWriter, request *http.Request) error {
	panic("boom")
}

func TestWebWorkerUnitHandlerPanicCallsDone(t *testing.T) {
	s, err := unit.Spawn(t, factoryWwuPanic, gen.ProcessOptions{})
	check.NoError(t, err)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	done := false
	msg := meta.MessageWebRequest{Response: rec, Request: req, Done: func() { done = true }}
	s.SendMessage(gen.PID{}, msg)

	check.True(t, done)
	check.True(t, s.Terminated())
}

// a custom worker can answer a request itself.
type wwuGet struct {
	act.WebWorker
	failGet bool
}

func factoryWwuGet() gen.ProcessBehavior { return &wwuGet{} }

func (w *wwuGet) HandleGet(from gen.PID, writer http.ResponseWriter, request *http.Request) error {
	if w.failGet {
		return errActorBoom
	}
	writer.WriteHeader(http.StatusOK)
	writer.Write([]byte("ok"))
	return nil
}
func (w *wwuGet) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	switch request {
	case "ping":
		return "pong", nil
	case "fail":
		return nil, errActorBoom
	case "normal":
		return "bye", gen.TerminateReasonNormal
	}
	return nil, nil
}

func TestWebWorkerUnitCallResult(t *testing.T) {
	s, _ := unit.Spawn(t, factoryWwuGet, gen.ProcessOptions{})
	resp, err := s.Call(gen.PID{}, "ping")
	check.NoError(t, err)
	check.Equal(t, "pong", resp)
}

func TestWebWorkerUnitCallError(t *testing.T) {
	s, _ := unit.Spawn(t, factoryWwuGet, gen.ProcessOptions{})
	_, err := s.Call(gen.PID{}, "fail")
	check.ErrorIs(t, err, errActorBoom)
	check.True(t, s.Terminated())
}

func TestWebWorkerUnitCallNormalResult(t *testing.T) {
	s, _ := unit.Spawn(t, factoryWwuGet, gen.ProcessOptions{})
	resp, err := s.Call(gen.PID{}, "normal")
	check.NoError(t, err)
	check.Equal(t, "bye", resp)
	check.True(t, s.Terminated())
}

func TestWebWorkerUnitCustomGet(t *testing.T) {
	s, _ := unit.Spawn(t, factoryWwuGet, gen.ProcessOptions{})
	msg, rec := webRequest("GET")
	s.SendMessage(gen.PID{}, msg)
	check.Equal(t, http.StatusOK, rec.Code)
	check.Equal(t, "ok", rec.Body.String())
}

// a handler returning an error terminates the worker.
func TestWebWorkerUnitGetError(t *testing.T) {
	s, _ := unit.Spawn(t, factoryWwuGet, gen.ProcessOptions{})
	s.Behavior().(*wwuGet).failGet = true
	msg, _ := webRequest("GET")
	s.SendMessage(gen.PID{}, msg)
	s.ShouldTerminate().Reason(errActorBoom).Once().Assert()
}

//
// non-web traffic + lifecycle
//

func TestWebWorkerUnitHandleMessage(t *testing.T) {
	s, _ := unit.Spawn(t, factoryWwu, gen.ProcessOptions{})
	s.SendMessage(gen.PID{}, "plain") // default HandleMessage (warn), survives
	check.False(t, s.Terminated())
}

func TestWebWorkerUnitHandleCall(t *testing.T) {
	s, _ := unit.Spawn(t, factoryWwu, gen.ProcessOptions{})
	resp, err := s.Call(gen.PID{}, "q") // default HandleCall (nil, nil)
	check.NoError(t, err)
	check.Nil(t, resp)
}

func TestWebWorkerUnitHandleEvent(t *testing.T) {
	s, _ := unit.Spawn(t, factoryWwu, gen.ProcessOptions{})
	s.DeliverEvent(gen.Event{Name: "e"}, "m")
	check.False(t, s.Terminated())
}

func TestWebWorkerUnitInspect(t *testing.T) {
	s, _ := unit.Spawn(t, factoryWwu, gen.ProcessOptions{})
	_, err := s.Inspect(gen.PID{}) // default HandleInspect returns nil
	check.NoError(t, err)
}

func TestWebWorkerUnitExitTerminates(t *testing.T) {
	s, _ := unit.Spawn(t, factoryWwu, gen.ProcessOptions{})
	s.DeliverExit(gen.PID{Node: "x@y", ID: 5}, errActorBoom)
	check.True(t, s.Terminated())
}

func TestWebWorkerUnitExitVariants(t *testing.T) {
	for _, ev := range exitVariants() {
		s, _ := unit.Spawn(t, factoryWwu, gen.ProcessOptions{})
		s.DeliverExitMessage(ev)
		check.True(t, s.Terminated())
	}
}

func TestWebWorkerUnitKind(t *testing.T) {
	s, _ := unit.Spawn(t, factoryWwu, gen.ProcessOptions{})
	kind := s.Behavior().(interface{ ProcessKind() gen.ProcessKind }).ProcessKind()
	check.Equal(t, gen.ProcessKindWeb, kind)
}

type wwuInitPanic struct{ act.WebWorker }

func factoryWwuInitPanic() gen.ProcessBehavior { return &wwuInitPanic{} }

func (w *wwuInitPanic) Init(args ...any) error { panic("web init boom") }

func TestWebWorkerUnitInitPanic(t *testing.T) {
	_, err := unit.Spawn(t, factoryWwuInitPanic, gen.ProcessOptions{})
	check.Error(t, err)
}
