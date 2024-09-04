package meta

import (
	"net"
	"sync"

	"ergo.services/ergo/gen"
)

type UDPServerOptions struct {
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
