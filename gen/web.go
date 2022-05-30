package gen

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"reflect"
	"strconv"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/lib"
)

type WebBehavior interface {
	InitWeb(process *WebProcess, args ...etf.Term) (WebOptions, error)

	HandleWebCall(process *WebProcess, from ServerFrom, message etf.Term) (etf.Term, ServerStatus)
	HandleWebCast(process *WebProcess, message etf.Term) ServerStatus
	HandleWebInfo(process *WebProcess, message etf.Term) ServerStatus

	HandleWebTerminate(process *WebProcess, reason string)
}

type WebStatus error

var (
	WebStatusOK   WebStatus // nil
	WebStatusStop WebStatus = fmt.Errorf("stop")

	// internals
	defaultWebPort    = uint16(8080)
	defaultWebTLSPort = uint16(8443)
)

type Web struct {
	Server
}

type WebOptions struct {
	Host    string
	Port    uint16 // default port 8080, for TLS - 8443
	Cert    tls.Certificate
	Handler http.Handler
}

type WebProcess struct {
	ServerProcess
	options  WebOptions
	behavior WebBehavior
}

type WebMiddlewareFunc func(http.Handler) http.Handler

type WebRoute struct {
	EndPoint     string
	HandlerGroup string
	Middleware   WebMiddlewareFunc
}

type WebRouteGroup struct {
	Name       string
	Middleware WebMiddlewareFunc
}

type webMessageTest struct{}

type WebMessageRequest struct {
	Canceled int32
	Request  *http.Request
	Response http.ResponseWriter
}

type defaultHandler struct {
}

func (dh *defaultHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	fmt.Fprintf(w, "Handler is not initialized\n")
}

//
// WebProcess API
//

func (wp *WebProcess) StartWebHandler(web WebHandlerBehavior, options WebHandlerOptions) http.Handler {
	handler, err := web.initHandler(wp, web, options)
	if err != nil {
		name := reflect.ValueOf(web).Elem().Type().Name()
		lib.Warning("[%s] can not initialaze WebHandler (%s): %s", wp.Self(), name, err)

		return &defaultHandler{}
	}
	return handler
}

//
// Server callbacks
//

func (web *Web) Init(process *ServerProcess, args ...etf.Term) error {

	behavior, ok := process.Behavior().(WebBehavior)
	if !ok {
		return fmt.Errorf("Web: not a WebBehavior")
	}

	webProcess := &WebProcess{
		ServerProcess: *process,
		behavior:      behavior,
	}
	// do not inherit parent State
	webProcess.State = nil

	options, err := behavior.InitWeb(webProcess, args...)
	if err != nil {
		return err
	}

	tlsEnabled := options.Cert.Certificate != nil

	if options.Port == 0 {
		if tlsEnabled {
			options.Port = defaultWebTLSPort
		} else {
			options.Port = defaultWebPort
		}
	}

	lc := net.ListenConfig{}
	ctx := process.Context()
	hostPort := net.JoinHostPort(options.Host, strconv.Itoa(int(options.Port)))
	listener, err := lc.Listen(ctx, "tcp", hostPort)
	if err != nil {
		return err
	}

	if tlsEnabled {
		config := tls.Config{
			Certificates: []tls.Certificate{options.Cert},
		}
		listener = tls.NewListener(listener, &config)
	}

	httpServer := http.Server{
		Handler: options.Handler,
	}

	// start acceptor
	go func() {
		err := httpServer.Serve(listener)
		process.Exit(err.Error())
	}()

	// Golang's listener is weird. It takes the context in the Listen method
	// but doesn't use it at all. HTTP server has the same issue.
	// So making a little workaround to handle process context cancelation.
	// Maybe one day they fix it.
	go func() {
		// this goroutine will be alive until the process context is canceled.
		select {
		case <-ctx.Done():
			httpServer.Close()
		}
	}()

	webProcess.options = options
	process.State = webProcess

	return nil
}

// HandleCall
func (web *Web) HandleCall(process *ServerProcess, from ServerFrom, message etf.Term) (etf.Term, ServerStatus) {
	webp := process.State.(*WebProcess)
	return webp.behavior.HandleWebCall(webp, from, message)
}

// HandleDirect
func (web *Web) HandleDirect(process *ServerProcess, ref etf.Ref, message interface{}) (interface{}, DirectStatus) {
	return nil, DirectStatusOK
}

// HandleCast
func (web *Web) HandleCast(process *ServerProcess, message etf.Term) ServerStatus {
	webp := process.State.(*WebProcess)
	status := webp.behavior.HandleWebCast(webp, message)

	switch status {
	case WebStatusOK:
		return ServerStatusOK
	case WebStatusStop:
		return ServerStatusStop
	default:
		return ServerStatus(status)
	}
}

// HandleInfo
func (web *Web) HandleInfo(process *ServerProcess, message etf.Term) ServerStatus {
	webp := process.State.(*WebProcess)
	status := webp.behavior.HandleWebInfo(webp, message)

	switch status {
	case WebStatusOK:
		return ServerStatusOK
	case WebStatusStop:
		return ServerStatusStop
	default:
		return ServerStatus(status)
	}
}

//
// default Web callbacks
//

// HandleWebCall
func (web *Web) HandleWebCall(process *WebProcess, from ServerFrom, message etf.Term) (etf.Term, ServerStatus) {
	lib.Warning("HandleWebCall: unhandled message (from %#v) %#v", from, message)
	return etf.Atom("ok"), ServerStatusOK
}

// HandleWebCast
func (web *Web) HandleWebCast(process *WebProcess, message etf.Term) ServerStatus {
	lib.Warning("HandleWebCast: unhandled message %#v", message)
	return ServerStatusOK
}

// HandleWebInfo
func (web *Web) HandleWebInfo(process *WebProcess, message etf.Term) ServerStatus {
	lib.Warning("HandleWebInfo: unhandled message %#v", message)
	return ServerStatusOK
}

// HandleWebTerminate
func (w *Web) HandleWebTerminate(process *WebProcess, reason string) {
	return
}

//
// WebProcess
//
