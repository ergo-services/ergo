package local

import (
	"bytes"
	"io"
	"net/http"
	"testing"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/meta"
	"ergo.services/ergo/testing/check"
	"ergo.services/ergo/testing/stage"
)

// webWorker is an act.WebWorker that answers GET with 202; POST is left to the
// default (501 Not Implemented).
type webWorker struct{ act.WebWorker }

func factoryWebWorker() gen.ProcessBehavior { return &webWorker{} }

func (w *webWorker) HandleGet(from gen.PID, writer http.ResponseWriter, request *http.Request) error {
	writer.WriteHeader(http.StatusAccepted)
	return nil
}

// webHost spawns the web meta processes: a root handler with no worker (so it
// forwards to the host, which answers 204), a "/test" handler bound to the
// "webworker" process, an unspawned handler on "/nometaprocess", and the web
// server on an ephemeral port. The bound address is reported on a Call.
type webHost struct {
	act.Actor
	server gen.Alias
}

func factoryWebHost() gen.ProcessBehavior { return &webHost{} }

func (h *webHost) Init(args ...any) error {
	mux := http.NewServeMux()

	h1 := meta.CreateWebHandler(meta.WebHandlerOptions{}) // no worker -> forwards to host
	if _, err := h.SpawnMeta(h1, gen.MetaOptions{}); err != nil {
		return err
	}
	mux.Handle("/", h1)

	h2 := meta.CreateWebHandler(meta.WebHandlerOptions{Worker: "webworker"})
	if _, err := h.SpawnMeta(h2, gen.MetaOptions{}); err != nil {
		return err
	}
	mux.Handle("/test", h2)

	// created but NOT spawned: its ServeHTTP answers 503
	h3 := meta.CreateWebHandler(meta.WebHandlerOptions{})
	mux.Handle("/nometaprocess", h3)

	ws, err := meta.CreateWebServer(meta.WebServerOptions{Host: "localhost", Port: 0, Handler: mux})
	if err != nil {
		return err
	}
	alias, err := h.SpawnMeta(ws, gen.MetaOptions{})
	if err != nil {
		return err
	}
	h.server = alias
	return nil
}

func (h *webHost) HandleMessage(from gen.PID, message any) error {
	if m, ok := message.(meta.MessageWebRequest); ok {
		defer m.Done()
		m.Response.WriteHeader(http.StatusNoContent)
	}
	return nil
}

func (h *webHost) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	if request == "addr" {
		insp, err := h.InspectMeta(h.server)
		if err != nil {
			return err, nil
		}
		return insp["listener"], nil
	}
	return "ok", nil
}

func httpStatus(t *testing.T, method, url string, body []byte) int {
	t.Helper()
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		t.Fatalf("%s %s: %s", method, url, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %s", method, url, err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

// TestLocalWeb: the web meta processes serve real HTTP. A handler with no worker
// forwards the request to its host process (204); a handler bound to a worker
// forwards to it (GET 202, unimplemented POST 501); an unspawned handler answers
// 503; and once the worker is gone the handler can no longer forward, so it
// answers 502.
func TestLocalWeb(t *testing.T) {
	s := stage.New(t)
	n := s.Node("n")

	host := n.Spawn(factoryWebHost, gen.ProcessOptions{})
	worker := n.SpawnRegister("webworker", factoryWebWorker, gen.ProcessOptions{})

	addrAny, err := n.Call(host, "addr")
	check.NoError(t, err)
	addr, ok := addrAny.(string)
	check.True(t, ok)
	base := "http://" + addr

	check.Equal(t, http.StatusNoContent, httpStatus(t, http.MethodGet, base+"/", nil))
	check.Equal(t, http.StatusAccepted, httpStatus(t, http.MethodGet, base+"/test", nil))
	check.Equal(t, http.StatusNotImplemented, httpStatus(t, http.MethodPost, base+"/test", []byte{1, 2, 3}))
	// the other verbs the worker does not implement also default to 501
	check.Equal(t, http.StatusNotImplemented, httpStatus(t, http.MethodPut, base+"/test", []byte{1}))
	check.Equal(t, http.StatusNotImplemented, httpStatus(t, http.MethodPatch, base+"/test", []byte{1}))
	check.Equal(t, http.StatusNotImplemented, httpStatus(t, http.MethodDelete, base+"/test", nil))
	check.Equal(t, http.StatusServiceUnavailable, httpStatus(t, http.MethodGet, base+"/nometaprocess", nil))

	// kill the worker, confirm it is gone, then the handler can no longer forward
	watcher := n.Spawn(factoryWatcher, gen.ProcessOptions{})
	n.Send(watcher, monitorCmd{Target: worker})
	n.ShouldMonitor().From(watcher).Target(worker).Once().Within(time.Second).Must()
	mk := n.Mark()
	check.NoError(t, n.Native().Kill(worker))
	n.ShouldReceiveDown().To(watcher).About(worker).Since(mk).Once().Within(time.Second).Must()

	check.Equal(t, http.StatusBadGateway, httpStatus(t, http.MethodGet, base+"/test", nil))
}
