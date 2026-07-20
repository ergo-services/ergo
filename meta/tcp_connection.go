package meta

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync/atomic"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
)

func CreateTCPConnection(options TCPConnectionOptions) (gen.MetaBehavior, error) {

	if err := options.ReadChunk.IsValid(); err != nil {
		return nil, err
	}

	if options.ReadBufferSize < 1 {
		options.ReadBufferSize = gen.DefaultTCPBufferSize
	}

	hp := net.JoinHostPort(options.Host, strconv.Itoa(int(options.Port)))
	c := &tcpconnection{
		options: options,
	}

	var conn net.Conn
	if options.CertManager != nil {
		config := &tls.Config{
			GetCertificate:     options.CertManager.GetCertificateFunc(),
			InsecureSkipVerify: options.InsecureSkipVerify,
		}
		tlsConn, err := tls.Dial("tcp", hp, config)
		if err != nil {
			return nil, err
		}
		conn = tlsConn
	} else {
		dialer := net.Dialer{
			KeepAlive: options.Advanced.KeepAlivePeriod,
		}
		tcpConn, err := dialer.Dial("tcp", hp)
		if err != nil {
			return nil, err
		}
		conn = tcpConn
	}
	c.conn = conn

	if len(c.options.WriteBufferKeepAlive) > 0 {
		// keepalive enabled
		c.connWriter = lib.NewFlusherWithKeepAlive(conn,
			c.options.WriteBufferKeepAlive,
			c.options.WriteBufferKeepAlivePeriod)
	} else {
		c.connWriter = lib.NewFlusher(conn)
	}

	return c, nil
}

//
// Connection gen.MetaBehavior implementation
//

type tcpconnection struct {
	gen.MetaProcess
	conn       net.Conn
	connWriter io.Writer
	options    TCPConnectionOptions
	bytesIn    uint64
	bytesOut   uint64
}

func (t *tcpconnection) Init(process gen.MetaProcess) error {
	t.MetaProcess = process
	return nil
}

func (t *tcpconnection) Start() error {
	var to any

	if t.options.Process == "" {
		to = t.Parent()
	} else {
		to = t.options.Process
	}

	defer func() {
		t.conn.Close()
		message := MessageTCPDisconnect{
			ID: t.ID(),
		}
		if err := t.Send(to, message); err != nil {
			t.Log().Error("unable to send MessageTCPDisconnect to %s: %s", to, err)
			return
		}
	}()

	message := MessageTCPConnect{
		ID:         t.ID(),
		RemoteAddr: t.conn.RemoteAddr(),
		LocalAddr:  t.conn.LocalAddr(),
	}
	if err := t.Send(to, message); err != nil {
		t.Log().Error("unable to send MessageTCPConnect to %v: %s", to, err)
		return err
	}

	if t.options.ReadChunk.Enable == false {
		return t.readData(to)
	}

	return t.readDataChunk(to)

}

func (t *tcpconnection) readData(to any) error {
	var buf []byte

	id := t.ID()

	for {
		if t.options.ReadBufferPool == nil {
			buf = make([]byte, t.options.ReadBufferSize)
		} else {
			buf = t.options.ReadBufferPool.Get().([]byte)
			if len(buf) == 0 {
				if cap(buf) == 0 {
					buf = make([]byte, t.options.ReadBufferSize)
				} else {
					buf = buf[0:cap(buf)]
				}
			}
		}

	next:
		n, err := t.conn.Read(buf)
		if err != nil {
			if n == 0 {
				// closed connection
				return nil
			}

			t.Log().Error("unable to read from tcp socket: %s", err)
			return err
		}

		if n == 0 {
			// keepalive
			goto next // use goto to get rid of buffer reallocation
		}
		atomic.AddUint64(&t.bytesIn, uint64(n))

		message := MessageTCP{
			ID:   id,
			Data: buf[:n],
		}
		if err := t.Send(to, message); err != nil {
			t.Log().Error("unable to send MessageTCP: %s", err)
			return err
		}
	}
}

func (t *tcpconnection) readDataChunk(to any) error {
	id := t.ID()
	err := readChunks(countReader{t.conn, &t.bytesIn}, t.options.ReadChunk,
		t.options.ReadBufferSize, t.options.ReadBufferPool,
		func(chunk []byte) error {
			return t.Send(to, MessageTCP{ID: id, Data: chunk})
		})
	if err != nil {
		t.Log().Error("unable to read chunk from tcp socket: %s", err)
	}
	return err
}

func (t *tcpconnection) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case MessageTCP:
		l, err := t.connWriter.Write(m.Data)
		if err != nil {
			return err
		}
		atomic.AddUint64(&t.bytesOut, uint64(l))
		if t.options.ReadBufferPool != nil {
			t.options.ReadBufferPool.Put(m.Data)
		}
	default:
		t.Log().Error("unsupported message from %s. ignored", from)
	}
	return nil
}

func (t *tcpconnection) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, nil
}

func (t *tcpconnection) Terminate(reason error) {
	defer t.conn.Close()

	if reason == nil {
		return
	}

	if reason == gen.TerminateReasonShutdown || reason == gen.TerminateReasonNormal {
		return
	}

	t.Log().Error("terminated abnormaly: %s", reason)
}

func (t *tcpconnection) HandleInspect(from gen.PID, item ...string) map[string]string {
	var to any
	bytesIn := atomic.LoadUint64(&t.bytesIn)
	bytesOut := atomic.LoadUint64(&t.bytesOut)
	if t.options.Process == "" {
		to = t.Parent()
	} else {
		to = t.options.Process
	}
	return map[string]string{
		"local":     t.conn.LocalAddr().String(),
		"remote":    t.conn.RemoteAddr().String(),
		"process":   fmt.Sprintf("%s", to),
		"bytes in":  fmt.Sprintf("%d", bytesIn),
		"bytes out": fmt.Sprintf("%d", bytesOut),
	}
}
