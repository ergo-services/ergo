package meta

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/testing/mock"
)

func TestWebServerLifecycle(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	mb, err := CreateWebServer(WebServerOptions{Host: "127.0.0.1", Port: 0, Handler: handler})
	if err != nil {
		t.Fatalf("CreateWebServer: %v", err)
	}
	ws := mb.(*webserver)
	ws.Init(mock.NewMeta())

	go ws.Start()
	defer ws.Terminate(nil)

	resp, err := http.Get("http://" + ws.listener.Addr().String() + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "ok" {
		t.Fatalf("body = %q, want ok", body)
	}

	if err := ws.HandleMessage(gen.PID{}, "x"); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.HandleCall(gen.PID{}, gen.Ref{}, "x"); err != nil {
		t.Fatal(err)
	}
	insp := ws.HandleInspect(gen.PID{})
	if insp["listener"] == "" {
		t.Fatalf("inspect missing listener: %v", insp)
	}
	if insp["state"] != "serving" || insp["tls"] != "false" || insp["handler"] == "" {
		t.Fatalf("inspect = %v", insp)
	}
	if insp["accepted"] == "0" || insp["requests"] == "0" || insp["last_request"] == "never" {
		t.Fatalf("the served request was not counted: %v", insp)
	}
	if insp["errors"] != "0" || insp["last_error"] != "none" {
		t.Fatalf("a healthy server reports errors: %v", insp)
	}

	// the http.Server ErrorLog adapter trims the trailing CRLF and logs
	if n, err := ws.Write([]byte("boom\r\n")); err != nil || n != 6 {
		t.Fatalf("Write = (%d, %v), want (6, nil)", n, err)
	}
	insp = ws.HandleInspect(gen.PID{})
	if insp["errors"] != "1" || insp["last_error"] != "boom" {
		t.Fatalf("the error line was not kept: %v", insp)
	}

	queried := ws.HandleInspect(gen.PID{}, "help", "nonsense")
	if queried["help"] == "" || queried["nonsense"] != "<unknown item>" {
		t.Fatalf("queries came out as %v", queried)
	}
}

func TestWebHandlerNotInitialized(t *testing.T) {
	h := CreateWebHandler(WebHandlerOptions{}).(*webhandler)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code = %d, want 503", rec.Code)
	}

	// before Init the inspect says so and counts the refusal, message/call are inert
	insp := h.HandleInspect(gen.PID{})
	if insp["state"] != "not initialized" {
		t.Fatalf("inspect before Init = %v", insp)
	}
	if insp["worker"] != "not set" || insp["unavailable"] != "1" {
		t.Fatalf("inspect before Init = %v", insp)
	}
	if err := h.HandleMessage(gen.PID{}, "x"); err != nil {
		t.Fatal(err)
	}
	if res, err := h.HandleCall(gen.PID{}, gen.Ref{}, "x"); err != nil || res != gen.ErrUnsupported {
		t.Fatalf("HandleCall = (%v, %v), want (ErrUnsupported, nil)", res, err)
	}
}

func TestWebHandlerServeForwardsRequest(t *testing.T) {
	mp := mock.NewMeta()
	mp.OnSend(func(to any, message any) error {
		req := message.(MessageWebRequest)
		req.Done() // cancel the request context so ServeHTTP returns at once
		return nil
	})

	h := CreateWebHandler(WebHandlerOptions{RequestTimeout: time.Second}).(*webhandler)
	h.Init(mp)

	if insp := h.HandleInspect(gen.PID{}); insp["state"] != "running" || insp["requests"] != "0" {
		t.Fatalf("inspect after Init = %v", insp)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}

	// the request is counted, none of it is left in flight, and help names the keys
	insp := h.HandleInspect(gen.PID{})
	if insp["requests"] != "1" || insp["in_flight"] != "0" || insp["last_request"] == "never" {
		t.Fatalf("inspect after a request = %v", insp)
	}
	if insp["timeouts"] != "0" || insp["send_failed"] != "0" {
		t.Fatalf("a served request was counted as a failure: %v", insp)
	}
	queried := h.HandleInspect(gen.PID{}, "help", "nonsense")
	if queried["help"] == "" || queried["nonsense"] != "<unknown item>" {
		t.Fatalf("queries came out as %v", queried)
	}
}

func TestWebHandlerTimeout(t *testing.T) {
	mp := mock.NewMeta()
	mp.OnSend(func(to any, message any) error { return nil }) // never cancels

	h := CreateWebHandler(WebHandlerOptions{RequestTimeout: 10 * time.Millisecond}).(*webhandler)
	h.Init(mp)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("code = %d, want 504", rec.Code)
	}
	if insp := h.HandleInspect(gen.PID{}); insp["timeouts"] != "1" {
		t.Errorf("the timeout was not counted: %v", insp)
	}
}

// a write from a worker that runs past the deadline must not reach the real
// writer once the request context is done.
func TestWebResponseWriterDropsAfterDeadline(t *testing.T) {
	rec := httptest.NewRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	rw := &webResponseWriter{ResponseWriter: rec, ctx: ctx}

	rw.WriteHeader(http.StatusOK)
	if _, err := rw.Write([]byte("live")); err != nil {
		t.Fatal(err)
	}

	cancel() // request deadline passed

	if n, err := rw.Write([]byte("late")); err == nil || n != 0 {
		t.Fatalf("write after deadline must fail: n=%d err=%v", n, err)
	}
	rw.WriteHeader(http.StatusInternalServerError) // must be a no-op

	if rec.Body.String() != "live" {
		t.Fatalf("late write leaked into response: %q", rec.Body.String())
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("late WriteHeader changed status: %d", rec.Code)
	}
}

func TestWebHandlerSendError(t *testing.T) {
	mp := mock.NewMeta()
	mp.OnSend(func(to any, message any) error { return gen.ErrProcessUnknown })

	h := CreateWebHandler(WebHandlerOptions{RequestTimeout: time.Second}).(*webhandler)
	h.Init(mp)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("code = %d, want 502", rec.Code)
	}
}

func TestWebHandlerStartTerminate(t *testing.T) {
	h := CreateWebHandler(WebHandlerOptions{RequestTimeout: time.Second}).(*webhandler)
	h.Init(mock.NewMeta())

	done := make(chan error, 1)
	go func() { done <- h.Start() }()

	h.Terminate(nil) // unblocks Start by sending the reason on the channel
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Start did not return after Terminate")
	}

	// a terminated handler refuses further requests
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code = %d, want 503", rec.Code)
	}
}
