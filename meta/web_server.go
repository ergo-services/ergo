package meta

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"ergo.services/ergo/gen"
)

//
// Web Server meta process
//

func CreateWebServer(options WebServerOptions) (gen.MetaBehavior, error) {
	hostPort := net.JoinHostPort(options.Host, strconv.Itoa(int(options.Port)))
	listener, err := net.Listen("tcp", hostPort)
	if err != nil {
		return nil, err
	}
	if options.CertManager != nil {
		config := &tls.Config{GetCertificate: options.CertManager.GetCertificateFunc()}
		listener = tls.NewListener(listener, config)
	}

	w := &webserver{
		listener: listener,
		tls:      options.CertManager != nil,
		handler:  fmt.Sprintf("%T", options.Handler),
	}

	w.server = http.Server{
		Handler:   options.Handler,
		ErrorLog:  log.New(w, "", 0),
		ConnState: w.connState,
	}
	return w, nil
}

type webserver struct {
	gen.MetaProcess
	server   http.Server
	listener net.Listener
	tls      bool
	handler  string
	started  time.Time

	// counted on the http server goroutines, read by HandleInspect on the mailbox one
	accepted    atomic.Int64
	requests    atomic.Int64
	hijacked    atomic.Int64
	closed      atomic.Int64
	errors      atomic.Int64
	lastRequest atomic.Int64 // unix nano
	lastError   atomic.Value // string
	stopped     atomic.Bool
}

func (w *webserver) Init(process gen.MetaProcess) error {
	w.MetaProcess = process
	w.started = time.Now()
	w.Log().Debug("web server started on %s", w.listener.Addr())
	return nil
}

func (w *webserver) Start() error {
	w.server.Serve(w.listener)
	w.stopped.Store(true)
	return nil
}

// connState is the only place the connection counters move: http.Server calls it on every
// transition, so nothing has to be tracked per connection.
func (w *webserver) connState(conn net.Conn, state http.ConnState) {
	switch state {
	case http.StateNew:
		w.accepted.Add(1)
	case http.StateActive:
		w.requests.Add(1)
		w.lastRequest.Store(time.Now().UnixNano())
	case http.StateHijacked:
		w.hijacked.Add(1)
	case http.StateClosed:
		w.closed.Add(1)
	}
}

func (w *webserver) HandleMessage(from gen.PID, message any) error {
	return nil
}

func (w *webserver) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, nil
}

func (w *webserver) Terminate(reason error) {
	w.listener.Close()
}

const webServerInspectHelp = "summary keys: state, listener, tls, handler, uptime, accepted, open, " +
	"requests, hijacked, closed, errors, last_request, last_error"

func (w *webserver) HandleInspect(from gen.PID, item ...string) map[string]string {
	if len(item) == 0 {
		return w.inspectSummary()
	}

	result := map[string]string{}
	for _, q := range item {
		if q == "help" {
			result["help"] = webServerInspectHelp
			continue
		}
		result[q] = "<unknown item>"
	}
	return result
}

func (w *webserver) inspectSummary() map[string]string {
	state := "serving"
	switch {
	case w.MetaProcess == nil:
		state = "not initialized"
	case w.stopped.Load():
		state = "stopped"
	}

	uptime := "never started"
	if w.started.IsZero() == false {
		uptime = time.Since(w.started).Round(time.Second).String()
	}

	last := "never"
	if at := w.lastRequest.Load(); at > 0 {
		last = time.Since(time.Unix(0, at)).Round(time.Second).String()
	}

	lastError := "none"
	if text, ok := w.lastError.Load().(string); ok {
		lastError = text
	}

	accepted := w.accepted.Load()
	return map[string]string{
		"state":        state,
		"listener":     w.listener.Addr().String(),
		"tls":          fmt.Sprintf("%t", w.tls),
		"handler":      w.handler,
		"uptime":       uptime,
		"accepted":     fmt.Sprintf("%d", accepted),
		"open":         fmt.Sprintf("%d", accepted-w.closed.Load()-w.hijacked.Load()),
		"requests":     fmt.Sprintf("%d", w.requests.Load()),
		"hijacked":     fmt.Sprintf("%d", w.hijacked.Load()),
		"closed":       fmt.Sprintf("%d", w.closed.Load()),
		"errors":       fmt.Sprintf("%d", w.errors.Load()),
		"last_request": last,
		"last_error":   lastError,
		"items":        "help",
	}
}

// webServerErrorKept is how much of an error line the inspect keeps.
const webServerErrorKept = 200

func (w *webserver) Write(log []byte) (int, error) {
	// http server adds '[\r]\n' at the end of the message. remove it before logging
	text := strings.TrimSpace(string(log))

	w.errors.Add(1)
	if len(text) > webServerErrorKept {
		text = text[:webServerErrorKept]
	}
	w.lastError.Store(text)

	w.Log().Error(text)
	return len(log), nil
}
