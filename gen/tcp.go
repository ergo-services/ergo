package gen

import (
	"crypto/tls"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/lib"
)

type TCPBehavior interface {
	InitTCP(process *TCPProcess, args ...etf.Term) (TCPOptions, error)

	HandleTCPCall(process *TCPProcess, from ServerFrom, message etf.Term) (etf.Term, ServerStatus)
	HandleTCPCast(process *TCPProcess, message etf.Term) ServerStatus
	HandleTCPInfo(process *TCPProcess, message etf.Term) ServerStatus

	HandleTCPTerminate(process *TCPProcess, reason string)
}

type TCPStatus error

var (
	TCPStatusOK   TCPStatus
	TCPStatusStop TCPStatus = fmt.Errorf("stop")
)

type TCP struct {
	Server
}

type TCPOptions struct {
	Host            string
	Port            uint16
	Cert            tls.Certificate
	KeepAlivePeriod int
}

type TCPProcess struct {
	ServerProcess
	options  TCPOptions
	behavior TCPBehavior
}

//
// Server callbacks
//
func (tcp *TCP) Init(process *ServerProcess, args ...etf.Term) error {

	behavior, ok := process.Behavior().(TCPBehavior)
	if !ok {
		return fmt.Errorf("Web: not a TCPBehavior")
	}

	tcpProcess := &TCPProcess{
		ServerProcess: *process,
		behavior:      behavior,
	}
	// do not inherit parent State
	tcpProcess.State = nil

	options, err := behavior.InitTCP(tcpProcess, args...)
	if err != nil {
		return err
	}

	tlsEnabled := options.Cert.Certificate != nil

	if options.Port == 0 {
		return fmt.Errorf("TCP port must be defined")
	}

	lc := net.ListenConfig{}

	if options.KeepAlivePeriod > 0 {
		lc.KeepAlive = time.Duration(options.KeepAlivePeriod) * time.Second
	}
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

	// start acceptor
	go func() {
		err := tcp.serve(listener)
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
			listener.Close()
		}
	}()

	tcpProcess.options = options
	process.State = tcpProcess

	return nil
}

//
// default TCP callbacks
//

// HandleWebCall
func (tcp *TCP) HandleTCPCall(process *WebProcess, from ServerFrom, message etf.Term) (etf.Term, ServerStatus) {
	lib.Warning("HandleTCPCall: unhandled message (from %#v) %#v", from, message)
	return etf.Atom("ok"), ServerStatusOK
}

// HandleWebCast
func (tcp *TCP) HandleTCPCast(process *WebProcess, message etf.Term) ServerStatus {
	lib.Warning("HandleTCPCast: unhandled message %#v", message)
	return ServerStatusOK
}

// HandleWebInfo
func (tcp *TCP) HandleTCPInfo(process *WebProcess, message etf.Term) ServerStatus {
	lib.Warning("HandleTCPInfo: unhandled message %#v", message)
	return ServerStatusOK
}

// internal

func (tcp *TCP) serve(listener net.Listener) error {

	return nil
}
