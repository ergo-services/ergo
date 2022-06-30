package gen

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync/atomic"
	"time"
	"unsafe"

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

	defaultDeadlineTimeout int = 3
	defaultDirectTimeout   int = 5
)

type TCP struct {
	Server
}

type TCPOptions struct {
	Host            string
	Port            uint16
	Cert            tls.Certificate
	KeepAlivePeriod int
	Handler         TCPHandlerBehavior
	// QueueLength defines how many parallel requests can be directed to this process. Default value is 10.
	QueueLength int
	// NumHandlers defines how many handlers will be started. Default 1
	NumHandlers int
	// IdleTimeout defines how long (in seconds) keeps the started handler alive with no packets. Zero value makes the handler non-stop.
	IdleTimeout     int
	DeadlineTimeout int
	MaxPacketSize   int
}

type TCPProcess struct {
	ServerProcess
	options  TCPOptions
	behavior TCPBehavior

	pool    []*Process
	counter uint64
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
	if options.Handler == nil {
		return fmt.Errorf("TCP handler must be defined")
	}
	tcpProcess.options = options

	if err := tcpProcess.initHandlers(); err != nil {
		return err
	}

	tlsEnabled := options.Cert.Certificate != nil

	if options.Port == 0 {
		return fmt.Errorf("TCP port must be defined")
	}

	if options.DeadlineTimeout < 1 {
		// we need to check the context if it was canceled to stop
		// reading and close the connection socket
		options.DeadlineTimeout = defaultDeadlineTimeout
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
		var err error
		var c net.Conn
		defer func() {
			if err == nil {
				process.Exit("normal")
				return
			}
			process.Exit(err.Error())
		}()

		for {
			c, err = listener.Accept()
			if err != nil {
				if ctx.Err() == nil {
					continue
				}
				return
			}
			go tcpProcess.serve(ctx, c)
		}
	}()

	// Golang's listener is weird. It takes the context in the Listen method
	// but doesn't use it at all.
	// So make a little workaround to handle process context cancelation.
	// Maybe one day they fix it.
	go func() {
		// this goroutine will be alive until the process context is canceled.
		select {
		case <-ctx.Done():
			listener.Close()
		}
	}()

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

func (tcpp *TCPProcess) serve(ctx context.Context, c net.Conn) error {
	var handlerProcess Process
	var packet interface{}
	var disconnect bool
	var disconnectError error
	var expectingBytes int = 1

	defer c.Close()

	deadlineTimeout := time.Second * time.Duration(tcpp.options.DeadlineTimeout)

	tcpConnection := TCPConnection{
		Addr:   c.RemoteAddr(),
		Socket: c,
	}

	l := uint64(tcpp.options.NumHandlers)
	// make round robin using the counter value
	cnt := atomic.AddUint64(&tcpp.counter, 1)
	i := cnt % l
	handlerProcess = *(*Process)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&tcpp.pool[i]))))

	b := lib.TakeBuffer()

nextPacket:
	for {
		if ctx.Err() != nil {
			return nil
		}

		if packet == nil {
			// just connected
			packet = messageTCPHandlerConnect{
				connection: tcpConnection,
			}
			break
		}

		if b.Len() < expectingBytes {
			deadline := false
			if err := c.SetReadDeadline(time.Now().Add(deadlineTimeout)); err == nil {
				deadline = true
			}

			n, e := b.ReadDataFrom(c, tcpp.options.MaxPacketSize)
			if n == 0 {
				if err, ok := e.(net.Error); deadline && ok && err.Timeout() {
					packet = messageTCPHandlerTimeout{
						connection: tcpConnection,
					}
					break
				}
				packet = messageTCPHandlerDisconnect{
					connection: tcpConnection,
				}
				// closed connection
				disconnect = true
				break
			}

			if e != nil && e != io.EOF {
				// something went wrong
				packet = messageTCPHandlerDisconnect{
					connection: tcpConnection,
				}
				disconnect = true
				disconnectError = e
				break
			}

			// check onemore time if we should read more data
			continue
		}
		// FIXME take it from the pool
		packet = &messageTCPHandlerPacket{
			connection: tcpConnection,
		}
		break
	}

	for a := uint64(0); a < l; a++ {
		if ctx.Err() != nil {
			return nil
		}

		_, err := handlerProcess.DirectWithTimeout(packet, defaultDirectTimeout)
		switch err {
		case TCPHandlerStatusOK:
			b.Reset()
			goto nextPacket
		case TCPHandlerStatusMore:
			goto nextPacket
		case TCPHandlerStatusLeft:

		case TCPHandlerStatusClose:
			disconnect = true
		case lib.ErrProcessTerminated:
			handlerProcess = tcpp.startHandler(int(i), tcpp.options.IdleTimeout)
			atomic.SwapPointer((*unsafe.Pointer)(unsafe.Pointer(&tcpp.pool[i])), unsafe.Pointer(&handlerProcess))
			continue
		case lib.ErrProcessBusy:
		case lib.ErrTimeout:
		}

		if disconnect {
			return disconnectError
		}
		expectingBytes = 1
		goto nextPacket
	}

	lib.Warning("[TCP] all handler are busy. closing connection with %q", c.RemoteAddr())
	return nil
}

func (tcpp *TCPProcess) initHandlers() error {
	if tcpp.options.NumHandlers < 1 {
		tcpp.options.NumHandlers = 1
	}
	if tcpp.options.IdleTimeout < 0 {
		tcpp.options.IdleTimeout = 0
	}

	if tcpp.options.QueueLength < 1 {
		tcpp.options.QueueLength = defaultQueueLength
	}

	c := atomic.AddUint64(&tcpp.counter, 1)
	if c > 1 {
		return fmt.Errorf("you can not use the same object more than once")
	}

	for i := 0; i < tcpp.options.NumHandlers; i++ {
		p := tcpp.startHandler(i, tcpp.options.IdleTimeout)
		if p == nil {
			return fmt.Errorf("can not initialize handlers")
		}
		tcpp.pool = append(tcpp.pool, &p)
	}
	return nil
}

func (tcpp *TCPProcess) startHandler(id int, idleTimeout int) Process {
	opts := ProcessOptions{
		Context:       tcpp.Context(),
		DirectboxSize: uint16(tcpp.options.QueueLength),
	}

	optsHandler := optsTCPHandler{id: id, idleTimeout: idleTimeout}
	p, err := tcpp.Spawn("", opts, tcpp.options.Handler, optsHandler)
	if err != nil {
		lib.Warning("can not start TCPHandler: %s", err)
		return nil
	}
	return p
}
