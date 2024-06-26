package meta

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"sync/atomic"

	"ergo.services/ergo/gen"
)

//
// UDP Server meta process
//

const (
	defaultUDPBufferSize int = 65000
)

func CreateUDPServer(options UDPServerOptions) (gen.MetaBehavior, error) {
	hp := net.JoinHostPort(options.Host, strconv.Itoa(int(options.Port)))
	pc, err := net.ListenPacket("udp", hp)
	if err != nil {
		return nil, err
	}

	mb := &udpserver{
		pc: pc,
	}
	if options.BufferSize < 1 {
		options.BufferSize = defaultUDPBufferSize
	}

	mb.bufferSize = options.BufferSize

	// check sync.Pool
	if options.BufferPool != nil {
		b := options.BufferPool.Get()
		if _, ok := b.([]byte); ok == false {
			return nil, errors.New("options.BufferPool must be pool of []byte values")
		}
		// get it back to the pool
		options.BufferPool.Put(b)
	}

	mb.bufpool = options.BufferPool
	mb.process = options.Process

	return mb, nil
}

type udpserver struct {
	gen.MetaProcess
	pc         net.PacketConn
	bufferSize int
	process    gen.Atom
	bufpool    *sync.Pool

	bytesIn  uint64
	bytesOut uint64
}

func (u *udpserver) Init(process gen.MetaProcess) error {
	u.MetaProcess = process
	return nil
}

func (u *udpserver) Start() error {
	var buf []byte
	var to any

	if u.process == "" {
		to = u.Parent()
	} else {
		to = u.process
	}

	id := u.ID()

	for {
		if u.bufpool == nil {
			buf = make([]byte, u.bufferSize)
		} else {
			b := u.bufpool.Get()
			buf = b.([]byte)
		}
		n, addr, err := u.pc.ReadFrom(buf)
		if n > 0 {
			packet := MessageUDP{
				ID:   id,
				Data: buf[:n],
				Addr: addr,
			}
			atomic.AddUint64(&u.bytesIn, uint64(n))

			if err := u.Send(to, packet); err != nil {
				u.Log().Error("unable to send MessageUDP to %s: %s", to, err)
			}
		}
		if err != nil {
			return err
		}
	}
}

func (u *udpserver) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case MessageUDP:
		n, err := u.pc.WriteTo(m.Data, m.Addr)
		if u.bufpool != nil {
			u.bufpool.Put(m.Data)
		}
		atomic.AddUint64(&u.bytesOut, uint64(n))
		return err
	default:
		u.Log().Error("unsupported message from %s. ignored", from)
	}
	return nil
}

func (u *udpserver) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, nil
}

func (u *udpserver) Terminate(reason error) {
	u.pc.Close()
}

func (u *udpserver) HandleInspect(from gen.PID, item ...string) map[string]string {
	var to any
	bytesIn := atomic.LoadUint64(&u.bytesIn)
	bytesOut := atomic.LoadUint64(&u.bytesOut)

	if u.process == "" {
		to = u.Parent()
	} else {
		to = u.process
	}
	return map[string]string{
		"listener":  u.pc.LocalAddr().String(),
		"process":   fmt.Sprintf("%s", to),
		"bytes in":  fmt.Sprintf("%d", bytesIn),
		"bytes out": fmt.Sprintf("%d", bytesOut),
	}
}
