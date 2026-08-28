package meta

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"ergo.services/ergo/gen"
)

//
// Web Handler meta process
//

func CreateWebHandler(options WebHandlerOptions) WebHandler {
	if options.RequestTimeout == 0 {
		options.RequestTimeout = 5 * time.Second
	}

	return &webhandler{
		options: options,
		ch:      make(chan error),
	}
}

type WebHandler interface {
	http.Handler
	gen.MetaBehavior
}

type webhandler struct {
	gen.MetaProcess
	options    WebHandlerOptions
	to         atomic.Value // target (PID/ProcessID/Alias); set once in Init
	terminated atomic.Bool
	ch         chan error

	// counted on the http server goroutines, read by HandleInspect on the mailbox one
	requests    atomic.Int64
	inFlight    atomic.Int64
	timeouts    atomic.Int64
	sendFailed  atomic.Int64
	unavailable atomic.Int64
	lastRequest atomic.Int64 // unix nano
}

//
// gen.MetaBehavior implementation
//

func (w *webhandler) Init(process gen.MetaProcess) error {
	w.MetaProcess = process
	if w.options.Worker == "" {
		w.to.Store(process.Parent())
	} else {
		w.to.Store(w.options.Worker)
	}
	return nil
}

func (w *webhandler) Start() error {
	return <-w.ch
}

func (w *webhandler) HandleMessage(from gen.PID, message any) error {
	return nil
}

func (w *webhandler) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return gen.ErrUnsupported, nil
}

func (w *webhandler) Terminate(reason error) {
	w.terminated.Store(true)
	w.ch <- reason
	close(w.ch)
}

const webHandlerInspectHelp = "summary keys: state, worker, request_timeout, requests, in_flight, " +
	"timeouts, send_failed, unavailable, last_request"

func (w *webhandler) HandleInspect(from gen.PID, item ...string) map[string]string {
	if len(item) == 0 {
		return w.inspectSummary()
	}

	result := map[string]string{}
	for _, q := range item {
		if q == "help" {
			result["help"] = webHandlerInspectHelp
			continue
		}
		result[q] = "<unknown item>"
	}
	return result
}

func (w *webhandler) inspectSummary() map[string]string {
	state := "running"
	switch {
	case w.MetaProcess == nil:
		state = "not initialized"
	case w.terminated.Load():
		state = "terminated"
	}

	worker := "not set"
	if to := w.to.Load(); to != nil {
		worker = fmt.Sprintf("%s", to)
	}

	last := "never"
	if at := w.lastRequest.Load(); at > 0 {
		last = time.Since(time.Unix(0, at)).Round(time.Second).String()
	}

	return map[string]string{
		"state":           state,
		"worker":          worker,
		"request_timeout": w.options.RequestTimeout.String(),
		"requests":        fmt.Sprintf("%d", w.requests.Load()),
		"in_flight":       fmt.Sprintf("%d", w.inFlight.Load()),
		"timeouts":        fmt.Sprintf("%d", w.timeouts.Load()),
		"send_failed":     fmt.Sprintf("%d", w.sendFailed.Load()),
		"unavailable":     fmt.Sprintf("%d", w.unavailable.Load()),
		"last_request":    last,
		"items":           "help",
	}
}

//
// http.Handler implementation
//

func (w *webhandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if w.MetaProcess == nil {
		w.unavailable.Add(1)
		w.refuse(writer, request, http.StatusServiceUnavailable, ErrHandlerNotInitialized)
		return
	}

	if w.terminated.Load() {
		w.unavailable.Add(1)
		w.refuse(writer, request, http.StatusServiceUnavailable, ErrHandlerTerminated)
		return
	}

	to := w.to.Load()
	if to == nil {
		w.unavailable.Add(1)
		w.refuse(writer, request, http.StatusServiceUnavailable, ErrHandlerNotReady)
		return
	}

	w.requests.Add(1)
	w.lastRequest.Store(time.Now().UnixNano())
	w.inFlight.Add(1)
	defer w.inFlight.Add(-1)

	ctx, cancel := context.WithTimeout(context.Background(), w.options.RequestTimeout)

	// The worker runs on another goroutine and may outlive the request deadline.
	// It writes through rw, which forwards to the real writer only while ctx is
	// alive; once the deadline passes its writes are dropped so a late worker
	// cannot corrupt a response the handler already gave up on.
	rw := &webResponseWriter{ResponseWriter: writer, ctx: ctx}
	message := MessageWebRequest{
		Response: rw,
		Request:  request,
		Done:     cancel,
	}
	if err := w.Send(to, message); err != nil {
		w.sendFailed.Add(1)
		w.Log().Error("can not handle HTTP request: %s", err)
		w.refuse(writer, request, http.StatusBadGateway, ErrWorkerUnreachable)
		cancel()
		return
	}

	<-ctx.Done()

	err := ctx.Err()
	switch err {
	case context.Canceled:
		rw.commitHeader()
		return
	case context.DeadlineExceeded:
		w.timeouts.Add(1)
		w.Log().Error("handling HTTP-request timed out")
		rw.timeout(func() {
			w.refuse(writer, request, http.StatusGatewayTimeout, ErrWorkerTimeout)
		})
	default:
		cancel()
		w.Log().Error("got context error: %s", err)
	}
}

func (w *webhandler) refuse(writer http.ResponseWriter, request *http.Request,
	status int, reason error) {

	if w.options.Refusal == nil {
		http.Error(writer, reason.Error(), status)
		return
	}
	w.options.Refusal(writer, request, status, reason)
}

const (
	webWriterUnclaimed int32 = iota
	webWriterWorker
	webWriterTimeout
)

// webResponseWriter grants the underlying writer to a single owner: the worker or the deadline 504.
type webResponseWriter struct {
	http.ResponseWriter
	ctx             context.Context
	state           atomic.Int32
	header          http.Header
	headerCommitted bool
}

// claim grants the worker the sole right to write to the underlying writer.
func (w *webResponseWriter) claim() bool {
	if w.ctx.Err() != nil {
		return false
	}
	if w.state.CompareAndSwap(webWriterUnclaimed, webWriterWorker) {
		return true
	}
	return w.state.Load() == webWriterWorker
}

// timeout answers with refuse unless the worker already owns the writer.
func (w *webResponseWriter) timeout(refuse func()) {
	if w.state.CompareAndSwap(webWriterUnclaimed, webWriterTimeout) {
		refuse()
	}
}

// Header returns the worker's private header map, applied to the underlying writer on the first write.
func (w *webResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

// commitHeader applies the worker's headers to the underlying writer once, before its first write.
func (w *webResponseWriter) commitHeader() {
	if w.headerCommitted {
		return
	}
	w.headerCommitted = true
	dst := w.ResponseWriter.Header()
	for k, vv := range w.header {
		dst[k] = vv
	}
}

func (w *webResponseWriter) WriteHeader(status int) {
	if w.claim() == false {
		return
	}
	w.commitHeader()
	w.ResponseWriter.WriteHeader(status)
}

func (w *webResponseWriter) Write(b []byte) (int, error) {
	if w.claim() == false {
		return 0, w.ctx.Err()
	}
	w.commitHeader()
	return w.ResponseWriter.Write(b)
}

func (w *webResponseWriter) Flush() {
	if w.ctx.Err() != nil {
		return
	}
	if w.state.Load() != webWriterWorker {
		return
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
