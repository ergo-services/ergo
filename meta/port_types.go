package meta

import (
	"sync"
	"time"

	"ergo.services/ergo/gen"
)

type MessagePortStart struct {
	ID  gen.Alias
	Tag string
}

type MessagePortTerminate struct {
	ID  gen.Alias
	Tag string
}

type MessagePortText struct {
	ID   gen.Alias
	Tag  string
	Text string
}

type MessagePortData struct {
	ID   gen.Alias
	Tag  string
	Data []byte
}

type MessagePortError struct {
	ID    gen.Alias
	Tag   string
	Error error
}

type PortOptions struct {
	Cmd     string
	Args    []string
	Tag     string
	Process gen.Atom
	Binary  PortBinaryOptions
}

type PortBinaryOptions struct {
	Enable bool

	EnableAutoChunk bool

	ChunkFixedLength int

	ChunkHeaderSize                 int
	ChunkHeaderLengthPosition       int // within the header
	ChunkHeaderLengthSize           int // 1, 2 or 4
	ChunkHeaderLengthIncludesHeader bool
	ChunkMaxLength                  int

	ReadBufferSize int
	ReadBufferPool *sync.Pool

	// EnableWriteBuffer enables buffering for outgoing data. It improves performance
	// in case of writing a lot of small data chunks
	EnableWriteBuffer          bool
	WriteBufferKeepAlive       []byte
	WriteBufferKeepAlivePeriod time.Duration
}
