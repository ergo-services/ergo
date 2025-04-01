package meta

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"strconv"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

//
// TCP Server meta process
//

func CreateTCPServer(options TCPServerOptions) (gen.MetaBehavior, error) {

	if err := options.ReadChunk.IsValid(); err != nil {
		return nil, err
	}

	lc := net.ListenConfig{
		KeepAlive: -1, // disabled
	}
	lc.KeepAlive = options.Advanced.KeepAlivePeriod

	// check sync.Pool
	if options.ReadBufferPool != nil {
		b := options.ReadBufferPool.Get()
		if _, ok := b.([]byte); ok == false {
			return nil, errors.New("options.ReadBufferPool must be pool of []byte values")
		}
		// get it back to the pool
		options.ReadBufferPool.Put(b)
	}

	if options.ReadBufferSize < 1 {
		options.ReadBufferSize = gen.DefaultTCPBufferSize
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
		listener: listener,
		options:  options,
	}

	return s, nil
}

type tcpserver struct {
	gen.MetaProcess
	options  TCPServerOptions
	listener net.Listener
}

func (t *tcpserver) Init(process gen.MetaProcess) error {
	t.MetaProcess = process
	return nil
}

func (t *tcpserver) Start() error {
	options := TCPConnectionOptions{
		ReadChunk:                  t.options.ReadChunk,
		ReadBufferSize:             t.options.ReadBufferSize,
		ReadBufferPool:             t.options.ReadBufferPool,
		WriteBufferKeepAlive:       t.options.WriteBufferKeepAlive,
		WriteBufferKeepAlivePeriod: t.options.WriteBufferKeepAlivePeriod,
		Advanced:                   t.options.Advanced,
	}

	for i := 0; ; i++ {
		conn, err := t.listener.Accept()
		if err != nil {
			return err
		}

		c := &tcpconnection{
			conn:    conn,
			options: options,
		}

		if len(c.options.WriteBufferKeepAlive) > 0 {
			// keepalive enabled
			c.connWriter = lib.NewFlusherWithKeepAlive(conn,
				c.options.WriteBufferKeepAlive,
				c.options.WriteBufferKeepAlivePeriod)
		} else {
			c.connWriter = lib.NewFlusher(conn)
		}

		if l := len(t.options.ProcessPool); l > 0 {
			c.options.Process = t.options.ProcessPool[i%l]
		}

		if _, err := t.Spawn(c, gen.MetaOptions{}); err != nil {
			conn.Close()
			t.Log().Error("unable to spawn meta process: %s", err)
		}
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

	if reason == nil {
		return
	}

	if reason == gen.TerminateReasonShutdown || reason == gen.TerminateReasonNormal {
		return
	}

	t.Log().Error("terminated abnormaly: %s", reason)
}

func (t *tcpserver) HandleInspect(from gen.PID, item ...string) map[string]string {
	return map[string]string{
		"listener": t.listener.Addr().String(),
	}
}
