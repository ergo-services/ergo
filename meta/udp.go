package meta

import (
	"errors"
	"net"
	"strconv"
	"sync"

	"ergo.services/ergo/gen"
)

//
// UDP Server meta process
//

const (
	defaultUDPBufferSize int = 65000
)

func CreateUDP(options UDPOptions) (gen.MetaBehavior, error) {
	hp := net.JoinHostPort(options.Host, strconv.Itoa(int(options.Port)))
	pc, err := net.ListenPacket("udp", hp)
	if err != nil {
		return nil, err
	}

	mb := &udp{
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

type UDPOptions struct {
	Host       string
	Port       uint16
	Process    gen.Atom
	BufferSize int
	BufferPool *sync.Pool
}

type MessageUDP struct {
	ID   gen.Alias
	Addr net.Addr
	Data []byte
}

type udp struct {
	gen.MetaProcess
	pc         net.PacketConn
	bufferSize int
	process    gen.Atom
	bufpool    *sync.Pool

	// TODO add to UDPOptions
	stopnoproc bool
}

func (u *udp) Init(process gen.MetaProcess) error {
	u.MetaProcess = process
	return nil
}

func (u *udp) Start() error {
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

			if err := u.Send(to, packet); err != nil {
				u.Log().Error("unable to send MessageUDP to %s: %s", to, err)
				if u.stopnoproc {
					return err
				}
			}
		}
		if err != nil {
			return err
		}
	}
}

func (u *udp) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case MessageUDP:
		_, err := u.pc.WriteTo(m.Data, m.Addr)
		if u.bufpool != nil {
			u.bufpool.Put(m.Data)
		}
		return err
	default:
		u.Log().Error("unsupported message from %s. ignored", from)
	}
	return nil
}

func (u *udp) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, nil
}

func (u *udp) Terminate(reason error) {
	u.pc.Close()
}

func (u *udp) HandleInspect(from gen.PID) map[string]string {
	return map[string]string{
		"listener": u.pc.LocalAddr().String(),
	}
}
