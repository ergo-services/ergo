package meta

import (
	"bufio"
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
	Cmd             string
	Args            []string
	Env             map[gen.Env]string
	EnableEnvMeta   bool
	EnableEnvOS     bool
	Tag             string
	Process         gen.Atom
	SplitFuncStdout bufio.SplitFunc
	SplitFuncStderr bufio.SplitFunc
	Binary          PortBinaryOptions
}

type PortChunkOptions struct {
	Enable                     bool
	FixedLength                int
	HeaderSize                 int
	HeaderLengthPosition       int // within the header
	HeaderLengthSize           int // 1, 2 or 4
	HeaderLengthIncludesHeader bool
	MaxLength                  int
}

type PortBinaryOptions struct {
	Enable                     bool
	Chunk                      PortChunkOptions
	ReadBufferSize             int
	ReadBufferPool             *sync.Pool
	WriteBufferKeepAlive       []byte
	WriteBufferKeepAlivePeriod time.Duration
}
