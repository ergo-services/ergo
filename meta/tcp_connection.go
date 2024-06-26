package meta

import (
	"crypto/tls"
	"fmt"
	"net"
	"strconv"
	"sync"
	"sync/atomic"

	"ergo.services/ergo/gen"
)

func CreateTCPConnection(options TCPConnectionOptions) (gen.MetaBehavior, error) {
	var conn net.Conn

	hp := net.JoinHostPort(options.Host, strconv.Itoa(int(options.Port)))
	if options.CertManager != nil {
		config := &tls.Config{
			GetCertificate:     options.CertManager.GetCertificateFunc(),
			InsecureSkipVerify: options.InsecureSkipVerify,
		}
		c, err := tls.Dial("tcp", hp, config)
		if err != nil {
			return nil, err
		}
		conn = c
	} else {
		dialer := net.Dialer{}
		if options.KeepAlivePeriod > 0 {
			dialer.KeepAlive = options.KeepAlivePeriod
		}
		c, err := net.Dial("tcp", hp)
		if err != nil {
			return nil, err
		}
		conn = c
	}

	if options.BufferSize < 1 {
		options.BufferSize = gen.DefaultTCPBufferSize
	}

	c := &tcpconnection{
		conn:       conn,
		bufpool:    options.BufferPool,
		bufferSize: options.BufferSize,
	}

	if options.Process == "" {
		return c, nil
	}

	c.process = options.Process
	return c, nil
}

//
// Connection gen.MetaBehavior implementation
//

type tcpconnection struct {
	gen.MetaProcess
	process    gen.Atom
	conn       net.Conn
	bufpool    *sync.Pool
	bufferSize int
	bytesIn    uint64
	bytesOut   uint64
}

func (t *tcpconnection) Init(process gen.MetaProcess) error {
	t.MetaProcess = process
	return nil
}

func (t *tcpconnection) Start() error {
	var to any
	var buf []byte

	id := t.ID()

	if t.process == "" {
		to = t.Parent()
	} else {
		to = t.process
	}

	defer func() {
		t.conn.Close()
		message := MessageTCPDisconnect{
			ID: id,
		}
		if err := t.Send(to, message); err != nil {
			t.Log().Error("unable to send MessageTCPDisconnect to %s: %s", to, err)
			return
		}
	}()

	message := MessageTCPConnect{
		ID:         id,
		RemoteAddr: t.conn.RemoteAddr(),
		LocalAddr:  t.conn.LocalAddr(),
	}
	if err := t.Send(to, message); err != nil {
		t.Log().Error("unable to send MessageTCPConnect to %v: %s", to, err)
		return err
	}

	for {
		if t.bufpool == nil {
			buf = make([]byte, t.bufferSize)
		} else {
			buf = t.bufpool.Get().([]byte)
		}

	retry:
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
			goto retry // use goto to get rid of buffer reallocation
		}
		message := MessageTCP{
			ID:   id,
			Data: buf[:n],
		}
		atomic.AddUint64(&t.bytesIn, uint64(n))
		if err := t.Send(to, message); err != nil {
			t.Log().Error("unable to send MessageTCP: %s", err)
			return err
		}
	}
}

func (t *tcpconnection) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case MessageTCP:
		l := len(m.Data)
		lenD := l
		for {
			n, e := t.conn.Write(m.Data[lenD-l:])
			if e != nil {
				return e
			}
			// check if something left
			l -= n
			if l == 0 {
				break
			}
		}
		atomic.AddUint64(&t.bytesOut, uint64(l))
		if t.bufpool != nil {
			t.bufpool.Put(m.Data)
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
	if reason == nil || reason == gen.TerminateMetaNormal {
		return
	}
	t.Log().Error("terminated abnormaly: %s", reason)
}

func (t *tcpconnection) HandleInspect(from gen.PID, item ...string) map[string]string {
	bytesIn := atomic.LoadUint64(&t.bytesIn)
	bytesOut := atomic.LoadUint64(&t.bytesOut)
	return map[string]string{
		"local":     t.conn.LocalAddr().String(),
		"remote":    t.conn.RemoteAddr().String(),
		"process":   t.process.String(),
		"bytes in":  fmt.Sprintf("%d", bytesIn),
		"bytes out": fmt.Sprintf("%d", bytesOut),
	}
}
