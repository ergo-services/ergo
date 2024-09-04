package meta

import (
	"ergo.services/ergo/gen"
	"net"
	"sync"
	"time"
)

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

type TCPConnectionOptions struct {
	Host               string
	Port               uint16
	Process            gen.Atom
	CertManager        gen.CertManager
	BufferSize         int
	BufferPool         *sync.Pool
	KeepAlivePeriod    time.Duration
	InsecureSkipVerify bool
}
type TCPServerOptions struct {
	Host               string
	Port               uint16
	ProcessPool        []gen.Atom
	CertManager        gen.CertManager
	BufferSize         int
	BufferPool         *sync.Pool
	KeepAlivePeriod    time.Duration
	InsecureSkipVerify bool
}
