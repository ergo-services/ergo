package meta

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"strconv"
	"sync"
	"time"

	"ergo.services/ergo/gen"
)

//
// TCP Server meta process
//

func CreateTCP(options TCPOptions) (gen.MetaBehavior, error) {
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

	s := &tcps{
		bufpool:    options.BufferPool,
		listener:   listener,
		procpool:   options.ProcessPool,
		bufferSize: options.BufferSize,
	}

	return s, nil
}

type TCPOptions struct {
	Host               string
	Port               uint16
	ProcessPool        []gen.Atom
	CertManager        gen.CertManager
	BufferSize         int
	BufferPool         *sync.Pool
	KeepAlivePeriod    time.Duration
	InsecureSkipVerify bool
}

type TCPClientOptions struct {
	Host               string
	Port               uint16
	Process            gen.Atom
	CertManager        gen.CertManager
	BufferSize         int
	BufferPool         *sync.Pool
	KeepAlivePeriod    time.Duration
	InsecureSkipVerify bool
}

type MessageTCPConnect struct {
	ID         gen.Alias
	RemoteAddr net.Addr
	LocalAddr  net.Addr
}

type MessageTCPDisconnect struct {
	ID gen.Alias
}

type MessageTCP struct {
	ID   gen.Alias
	Data []byte
}

func CreateTCPClient(options TCPClientOptions) (gen.MetaBehavior, error) {
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

	c := &tcpc{
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

type tcps struct {
	gen.MetaProcess
	procpool   []gen.Atom
	bufpool    *sync.Pool
	listener   net.Listener
	bufferSize int
}

func (t *tcps) Init(process gen.MetaProcess) error {
	t.MetaProcess = process
	return nil
}

func (t *tcps) Start() error {
	i := 0

	for {
		conn, err := t.listener.Accept()
		if err != nil {
			return err
		}

		c := &tcpc{
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
func (t *tcps) HandleMessage(from gen.PID, message any) error {
	if t.MetaProcess != nil {
		t.Log().Error("ignored message from %s", from)
		return nil
	}
	return nil
}

func (t *tcps) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	if t.MetaProcess != nil {
		t.Log().Error("ignored request from %s", from)
	}
	return gen.ErrUnsupported, nil
}

func (t *tcps) Terminate(reason error) {
	defer t.listener.Close()

	if reason == nil || reason == gen.TerminateReasonNormal {
		return
	}
	t.Log().Error("terminated abnormaly: %s", reason)
}

func (t *tcps) HandleInspect(from gen.PID) map[string]string {
	return map[string]string{
		"listener": t.listener.Addr().String(),
	}
}

//
// Connection gen.MetaBehavior implementation
//

type tcpc struct {
	gen.MetaProcess
	process    gen.Atom
	conn       net.Conn
	bufpool    *sync.Pool
	bufferSize int
}

func (t *tcpc) Init(process gen.MetaProcess) error {
	t.MetaProcess = process
	return nil
}

func (t *tcpc) Start() error {
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
		if err := t.Send(to, message); err != nil {
			t.Log().Error("unable to send MessageTCP: %s", err)
			return err
		}
	}
}

func (t *tcpc) HandleMessage(from gen.PID, message any) error {
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
		if t.bufpool != nil {
			t.bufpool.Put(m.Data)
		}
	default:
		t.Log().Error("unsupported message from %s. ignored", from)
	}
	return nil
}

func (t *tcpc) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, nil
}

func (t *tcpc) Terminate(reason error) {
	defer t.conn.Close()
	if reason == nil || reason == gen.TerminateReasonNormal {
		return
	}
	t.Log().Error("terminated abnormaly: %s", reason)
}

func (t *tcpc) HandleInspect(from gen.PID) map[string]string {
	return map[string]string{
		"local":  t.conn.LocalAddr().String(),
		"remote": t.conn.RemoteAddr().String(),
	}
}
