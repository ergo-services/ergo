package lib

import (
	"io"
	"time"
)

type NetReadWriter interface {
	NetReader
	NetWriter
}

type NetReader interface {
	io.Reader
	SetReadDeadline(t time.Time) error
}

type NetWriter interface {
	io.Writer
	SetWriteDeadline(t time.Time) error
}
