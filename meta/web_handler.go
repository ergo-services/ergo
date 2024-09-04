package meta

import (
	"context"
	"fmt"
	"net/http"
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
	to         any
	terminated bool
	ch         chan error
}

//
// gen.MetaBehavior implementation
//

func (w *webhandler) Init(process gen.MetaProcess) error {
	w.MetaProcess = process
	return nil
}

func (w *webhandler) Start() error {
	if w.options.Worker == "" {
		w.to = w.Parent()
	} else {
		w.to = w.options.Worker
	}
	return <-w.ch
}

func (w *webhandler) HandleMessage(from gen.PID, message any) error {
	return nil
}

func (w *webhandler) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return gen.ErrUnsupported, nil
}

func (w *webhandler) Terminate(reason error) {
	w.terminated = true
	w.ch <- reason
	close(w.ch)
}

func (w *webhandler) HandleInspect(from gen.PID, item ...string) map[string]string {
	if w.MetaProcess != nil {
		return nil
	}
	return map[string]string{
		"worker process": fmt.Sprintf("%s", w.to),
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

	if w.terminated {
		http.Error(writer, "Handler terminated", http.StatusServiceUnavailable)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), w.options.RequestTimeout)

	message := MessageWebRequest{
		Response: writer,
		Request:  request,
		Done:     cancel,
	}
	if err := w.Send(w.to, message); err != nil {
		w.Log().Error("can not handle HTTP request: %s", err)
		http.Error(writer, "Bad gateway", http.StatusBadGateway)
		cancel()
		return
	}

	<-ctx.Done()

	err := ctx.Err()
	switch err {
	case context.Canceled:
		return
	case context.DeadlineExceeded:
		w.Log().Error("handling HTTP-request timed out")
		http.Error(writer, "Gateway timeout", http.StatusGatewayTimeout)
	default:
		cancel()
		w.Log().Error("got context error: %s", err)
	}
}
