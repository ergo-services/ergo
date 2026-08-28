package meta

import (
	"errors"
	"net/http"
	"time"

	"ergo.services/ergo/gen"
)

type WebServerOptions struct {
	Host        string
	Port        uint16
	CertManager gen.CertManager
	Handler     http.Handler
}
type WebHandlerOptions struct {
	Worker         gen.Atom
	RequestTimeout time.Duration

	// Refusal answers a request this handler could not pass to its worker. Nil answers with
	// plain text, which a caller speaking another protocol cannot read.
	Refusal RefusalHandler
}

// RefusalHandler answers a request a meta handler could not pass on. Nothing has been written
// yet, so it owns the whole response: headers, status and body. The status is the one the
// handler would have used, and the reason is one of the sentinels below.
type RefusalHandler func(writer http.ResponseWriter, request *http.Request,
	status int, reason error)

var (
	ErrHandlerNotInitialized = errors.New("handler is not initialized")
	ErrHandlerTerminated     = errors.New("handler terminated")
	ErrHandlerNotReady       = errors.New("handler is not ready")
	ErrWorkerUnreachable     = errors.New("worker is unreachable")
	ErrWorkerTimeout         = errors.New("worker did not answer in time")
)

type MessageWebRequest struct {
	Response http.ResponseWriter
	Request  *http.Request
	Done     func()
}
