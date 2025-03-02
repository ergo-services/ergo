package meta

import (
	"crypto/tls"
	"encoding/binary"
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

	if options.CertManager != nil {
		config := &tls.Config{
			GetCertificate:     options.CertManager.GetCertificateFunc(),
			InsecureSkipVerify: options.InsecureSkipVerify,
		}
		conn, err := tls.Dial("tcp", hp, config)
		if err != nil {
			return nil, err
		}
		c.conn = conn
		return c, nil
	}

	dialer := net.Dialer{
		KeepAlive: options.Advanced.KeepAlivePeriod,
	}
	conn, err := dialer.Dial("tcp", hp)
	if err != nil {
		return nil, err
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
	var buf []byte
	var chunk []byte

	id := t.ID()

	t.Log().Info("RRR DataChunk %v", t.options.ReadBufferSize)
	buf = make([]byte, t.options.ReadBufferSize)

	if t.options.ReadBufferPool == nil {
		chunk = make([]byte, 0, t.options.ReadBufferSize)
	} else {
		chunk = t.options.ReadBufferPool.Get().([]byte)
		chunk = chunk[:0]
	}

	cl := t.options.ReadChunk.FixedLength // chunk length

	for {

		n, err := t.conn.Read(buf)
		if err != nil {
			if n == 0 {
				// closed stdin
				return nil
			}

			t.Log().Error("unable to read from tcp socket: %s", err)
			return err
		}

		if n == 0 {
			continue
		}

		atomic.AddUint64(&t.bytesIn, uint64(n))

		chunk = append(chunk, buf[:n]...)

	next:

		// read length value for the chunk
		if cl == 0 {
			// check if we got the header
			if len(chunk) < t.options.ReadChunk.HeaderSize {
				continue
			}

			pos := t.options.ReadChunk.HeaderLengthPosition
			switch t.options.ReadChunk.HeaderLengthSize {
			case 1:
				cl = int(chunk[pos])
			case 2:
				cl = int(binary.BigEndian.Uint16(chunk[pos : pos+2]))
			case 4:
				cl = int(binary.BigEndian.Uint32(chunk[pos : pos+4]))
			default:
				// shouldn't reach this code
				panic("bug")
			}

			if t.options.ReadChunk.HeaderLengthIncludesHeader == false {
				cl += t.options.ReadChunk.HeaderSize
			}

			if t.options.ReadChunk.MaxLength > 0 {
				if cl > t.options.ReadChunk.MaxLength {
					t.Log().Error("chunk size %d is exceeded the limit (chunk MaxLenth: %d)", cl, t.options.ReadChunk.MaxLength)
					return gen.ErrTooLarge
				}
			}

		}

		if len(chunk) < cl {
			continue
		}

		// send chunk
		message := MessageTCP{
			ID:   id,
			Data: chunk[:cl],
		}

		if err := t.Send(to, message); err != nil {
			t.Log().Error("unable to send MessageTCP: %s", err)
			return err
		}

		tail := chunk[cl:]

		// prepare next chunk
		if t.options.ReadBufferPool == nil {
			chunk = make([]byte, 0, t.options.ReadChunk.FixedLength)
		} else {
			chunk = t.options.ReadBufferPool.Get().([]byte)
			chunk = chunk[:0]
		}

		cl = t.options.ReadChunk.FixedLength

		if len(tail) > 0 {
			chunk = append(chunk, tail...)
			goto next
		}

	}

}

func (t *tcpconnection) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case MessageTCP:
		t.Log().Info("MMMMM %v", m.Data)
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
