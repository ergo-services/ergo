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

func (w *webhandler) HandleInspect(from gen.PID, item ...string) map[string]string {
	if w.MetaProcess != nil {
		return nil
	}
	return map[string]string{
		"worker process": fmt.Sprintf("%s", w.to.Load()),
	}
}

//
// http.Handler implementation
//

func (w *webhandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if w.MetaProcess == nil {
		http.Error(writer, "Handler is not initialized", http.StatusServiceUnavailable)
		return
	}

	if w.terminated.Load() {
		http.Error(writer, "Handler terminated", http.StatusServiceUnavailable)
		return
	}

	to := w.to.Load()
	if to == nil {
		http.Error(writer, "Handler is not ready", http.StatusServiceUnavailable)
		return
	}

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
		w.Log().Error("can not handle HTTP request: %s", err)
		http.Error(writer, "Bad gateway", http.StatusBadGateway)
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
		w.Log().Error("handling HTTP-request timed out")
		rw.timeout()
	default:
		cancel()
		w.Log().Error("got context error: %s", err)
	}
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

// timeout writes the gateway-timeout response unless the worker already owns the writer.
func (w *webResponseWriter) timeout() {
	if w.state.CompareAndSwap(webWriterUnclaimed, webWriterTimeout) {
		http.Error(w.ResponseWriter, "Gateway timeout", http.StatusGatewayTimeout)
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
