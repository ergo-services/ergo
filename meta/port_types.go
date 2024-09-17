package meta

import (
	"sync"
	"time"

	"ergo.services/ergo/gen"
)

type MessagePortStarted struct {
	ID  gen.Alias
	Tag string
}

type MessagePortTerminated struct {
	ID  gen.Alias
	Tag string
}

type MessagePort struct {
	ID   gen.Alias
	Tag  string
	Data []byte
}

type MessagePortError struct {
	ID   gen.Alias
	Tag  string
	Data []byte
}

type PortOptions struct {
	Cmd            string
	Args           []string
	Tag            string
	Process        gen.Atom
	ReadBufferSize int
	ReadBufferPool *sync.Pool

	// WriteBuffer enables buffering for outgoing data. It improves performance
	// in case of writing a lot of small data chunks
	WriteBuffer                bool
	WriteBufferKeepAlive       []byte
	WriteBufferKeepAlivePeriod time.Duration
}
