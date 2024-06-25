package meta

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"strconv"
	"sync"

	"ergo.services/ergo/gen"
)

//
// TCP Server meta process
//

func CreateTCPServer(options TCPServerOptions) (gen.MetaBehavior, error) {
	lc := net.ListenConfig{
		KeepAlive: -1, // disabled
	}
	if options.KeepAlivePeriod > 0 {
		lc.KeepAlive = options.KeepAlivePeriod
	}
	if options.BufferSize < 1 {
		options.BufferSize = gen.DefaultTCPBufferSize
	}

	// check sync.Pool
	if options.BufferPool != nil {
		b := options.BufferPool.Get()
		if _, ok := b.([]byte); ok == false {
			return nil, errors.New("options.BufferPool must be pool of []byte values")
		}
		// get it back to the pool
		options.BufferPool.Put(b)
	}

	hp := net.JoinHostPort(options.Host, strconv.Itoa(int(options.Port)))
	listener, err := lc.Listen(context.Background(), "tcp", hp)
	if err != nil {
		return nil, err
	}

	if options.CertManager != nil {
		config := &tls.Config{
			GetCertificate:     options.CertManager.GetCertificateFunc(),
			InsecureSkipVerify: options.InsecureSkipVerify,
		}
		listener = tls.NewListener(listener, config)
	}

	s := &tcpserver{
		bufpool:    options.BufferPool,
		listener:   listener,
		procpool:   options.ProcessPool,
		bufferSize: options.BufferSize,
	}

	return s, nil
}

type tcpserver struct {
	gen.MetaProcess
	procpool   []gen.Atom
	bufpool    *sync.Pool
	listener   net.Listener
	bufferSize int
}

func (t *tcpserver) Init(process gen.MetaProcess) error {
	t.MetaProcess = process
	return nil
}

func (t *tcpserver) Start() error {
	i := 0

	for {
		conn, err := t.listener.Accept()
		if err != nil {
			return err
		}

		c := &tcpconnection{
			conn:       conn,
			bufpool:    t.bufpool,
			bufferSize: t.bufferSize,
		}
		if len(t.procpool) > 0 {
			l := len(t.procpool)
			c.process = t.procpool[i%l]
		}

		if _, err := t.Spawn(c, gen.MetaOptions{}); err != nil {
			conn.Close()
			t.Log().Error("unable to spawn meta process: %s", err)
		}
		i++

	}
}
func (t *tcpserver) HandleMessage(from gen.PID, message any) error {
	if t.MetaProcess != nil {
		t.Log().Error("ignored message from %s", from)
		return nil
	}
	return nil
}

func (t *tcpserver) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	if t.MetaProcess != nil {
		t.Log().Error("ignored request from %s", from)
	}
	return gen.ErrUnsupported, nil
}

func (t *tcpserver) Terminate(reason error) {
	defer t.listener.Close()

	if reason == nil || reason == gen.TerminateReasonNormal {
		return
	}
	t.Log().Error("terminated abnormaly: %s", reason)
}

func (t *tcpserver) HandleInspect(from gen.PID, item ...string) map[string]string {
	return map[string]string{
		"listener": t.listener.Addr().String(),
	}
}
