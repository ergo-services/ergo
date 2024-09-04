package meta

import (
	"crypto/tls"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"

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
	}

	w.server = http.Server{
		Handler:  options.Handler,
		ErrorLog: log.New(w, "", 0),
	}
	return w, nil
}

type webserver struct {
	gen.MetaProcess
	server   http.Server
	listener net.Listener
}

func (w *webserver) Init(process gen.MetaProcess) error {
	w.MetaProcess = process
	w.Log().Debug("web server started on %s", w.listener.Addr())
	return nil
}

func (w *webserver) Start() error {
	w.server.Serve(w.listener)
	return nil
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

func (w *webserver) HandleInspect(from gen.PID, item ...string) map[string]string {
	return map[string]string{
		"listener": w.listener.Addr().String(),
	}
}

func (w *webserver) Write(log []byte) (int, error) {
	// http server adds '[\r]\n' at the end of the message. remove it before logging
	w.Log().Error(strings.TrimSpace(string(log)))
	return len(log), nil
}
