package meta

import (
	"context"
	"crypto/tls"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ergo.services/ergo/gen"
)

//
// Web Server meta process
//

func CreateWeb(options WebOptions) (gen.MetaBehavior, error) {
	hostPort := net.JoinHostPort(options.Host, strconv.Itoa(int(options.Port)))
	listener, err := net.Listen("tcp", hostPort)
	if err != nil {
		return nil, err
	}
	if options.CertManager != nil {
		config := &tls.Config{GetCertificate: options.CertManager.GetCertificateFunc()}
		listener = tls.NewListener(listener, config)
	}

	w := &web{
		listener: listener,
	}

	w.server = http.Server{
		Handler:  options.Handler,
		ErrorLog: log.New(w, "", 0),
	}
	return w, nil
}

type WebOptions struct {
	Host        string
	Port        uint16
	CertManager gen.CertManager
	Handler     http.Handler
}

type web struct {
	gen.MetaProcess
	server   http.Server
	listener net.Listener
}

func (w *web) Init(process gen.MetaProcess) error {
	w.MetaProcess = process
	w.Log().Debug("web server started on %s", w.listener.Addr())
	return nil
}

func (w *web) Start() error {
	w.server.Serve(w.listener)
	return nil
}

func (w *web) HandleMessage(from gen.PID, message any) error {
	return nil
}

func (w *web) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, nil
}

func (w *web) Terminate(reason error) {
	w.listener.Close()
}

func (w *web) HandleInspect(from gen.PID) map[string]string {
	return map[string]string{
		"listener": w.listener.Addr().String(),
	}
}

func (w *web) Write(log []byte) (int, error) {
	// http server adds '[\r]\n' at the end of the message. remove it before logging
	w.Log().Error(strings.TrimSpace(string(log)))
	return len(log), nil
}

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

type WebHandlerOptions struct {
	Process        gen.Atom
	RequestTimeout time.Duration
}

type MessageWebRequest struct {
	Response http.ResponseWriter
	Request  *http.Request
	Done     func()
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
	if w.options.Process == "" {
		w.to = w.Parent()
	} else {
		w.to = w.options.Process
	}
	return <-w.ch
}

func (w *webhandler) HandleMessage(from gen.PID, message any) error {
	if w.MetaProcess != nil {
		w.Log().Error("ignored message from %s", from)
		return nil
	}
	return nil
}

func (w *webhandler) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	if w.MetaProcess != nil {
		w.Log().Error("ignored request from %s", from)
	}
	return gen.ErrUnsupported, nil
}

func (w *webhandler) Terminate(reason error) {
	w.terminated = true
	w.ch <- reason
	close(w.ch)
}

func (w *webhandler) HandleInspect(from gen.PID) map[string]string {
	if w.MetaProcess != nil {
		w.Log().Error("ignored inspect request from %s", from)
	}
	return nil
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
