package meta

import (
	"ergo.services/ergo/gen"
	"sync"
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

type PortOptions struct {
	Cmd        string
	Args       []string
	Tag        string
	Process    gen.Atom
	BufferSize int
	BufferPool *sync.Pool
}
