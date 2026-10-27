package meta

import (
	"net/netip"
	"sync"
	"time"

	"ergo.services/ergo/gen"
)

type MessageTCPConnect struct {
	ID         gen.Alias
	RemoteAddr netip.AddrPort
	LocalAddr  netip.AddrPort
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
	InsecureSkipVerify bool
	// HandshakeTimeout ends the connection when its TLS handshake takes longer
	// (10 seconds for zero)
	HandshakeTimeout time.Duration

	ReadBufferSize             int
	ReadBufferPool             *sync.Pool
	ReadChunk                  ChunkOptions
	WriteBufferKeepAlive       []byte
	WriteBufferKeepAlivePeriod time.Duration

	Advanced TCPAdvancedOptions
}
type TCPServerOptions struct {
	Host               string
	Port               uint16
	ProcessPool        []gen.Atom
	CertManager        gen.CertManager
	InsecureSkipVerify bool
	// HandshakeTimeout ends an accepted connection when its TLS handshake takes
	// longer (10 seconds for zero): a client that says nothing holds no meta process
	HandshakeTimeout time.Duration

	ReadBufferSize             int
	ReadBufferPool             *sync.Pool
	ReadChunk                  ChunkOptions
	WriteBufferKeepAlive       []byte
	WriteBufferKeepAlivePeriod time.Duration

	Advanced TCPAdvancedOptions
}

type TCPAdvancedOptions struct {
	KeepAlivePeriod time.Duration
}
