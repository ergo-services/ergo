package meta

import (
	"encoding/binary"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"ergo.services/ergo/gen"
)

// countReader adds the number of bytes read to a counter, so the shared chunk
// reader preserves the per-connection/per-port bytesIn accounting.
type countReader struct {
	r io.Reader
	n *uint64
}

func (c countReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	atomic.AddUint64(c.n, uint64(n))
	return n, err
}

type ChunkOptions struct {
	Enable                     bool
	FixedLength                int
	HeaderSize                 int
	HeaderLengthPosition       int // within the header
	HeaderLengthSize           int // 1, 2 or 4
	HeaderLengthIncludesHeader bool
	MaxLength                  int
}

func (co ChunkOptions) IsValid() error {
	if co.Enable == false {
		return nil
	}

	if co.FixedLength > 0 {
		return nil
	}

	// dynamic length
	if co.HeaderSize == 0 {
		return fmt.Errorf("chunk HeaderSize must be non-zero for dynamic chunk size")
	}

	hl := co.HeaderLengthSize + co.HeaderLengthPosition
	if hl > co.HeaderSize {
		return fmt.Errorf("chunk HeaderLengthPosition + ...LengthSize is out of HeaderSize bounds")
	}

	switch co.HeaderLengthSize {
	case 1, 2, 4:
	default:
		return fmt.Errorf("chunk HeaderLengthSize must be either: 1, 2, or 4 bytes")
	}

	return nil
}

// readChunks reads length-framed chunks from r according to opts and invokes
// onChunk for each complete chunk, in order. Framing is independent of how bytes
// are segmented by r: a chunk split across reads is reassembled, and several
// chunks arriving in one read are split out. It returns nil on EOF, gen.ErrTooLarge
// if a chunk would exceed opts.MaxLength, or the first error from r or onChunk.
//
// bufSize is the per-read buffer size. If pool is non-nil it supplies the
// accumulation buffers; a chunk passed to onChunk aliases its buffer, so the
// consumer owns recycling. This is the shared framing used by the TCP connection
// and the Port (binary) meta processes.
func readChunks(r io.Reader, opts ChunkOptions, bufSize int, pool *sync.Pool, onChunk func(chunk []byte) error) error {
	if bufSize < 1 {
		bufSize = defaultBufferSize
	}

	buf := make([]byte, bufSize)

	var chunk []byte
	if pool == nil {
		chunk = make([]byte, 0, bufSize)
	} else {
		chunk = pool.Get().([]byte)
		chunk = chunk[:0]
	}

	cl := opts.FixedLength // chunk length

	for {
		n, err := r.Read(buf)
		if err != nil {
			if n == 0 {
				// closed
				return nil
			}
			return err
		}

		if n == 0 {
			continue
		}

		chunk = append(chunk, buf[:n]...)

	next:

		// resolve the chunk length
		if cl == 0 {
			// wait until the whole header is buffered
			if len(chunk) < opts.HeaderSize {
				continue
			}

			pos := opts.HeaderLengthPosition
			switch opts.HeaderLengthSize {
			case 1:
				cl = int(chunk[pos])
			case 2:
				cl = int(binary.BigEndian.Uint16(chunk[pos : pos+2]))
			case 4:
				cl = int(binary.BigEndian.Uint32(chunk[pos : pos+4]))
			default:
				panic("bug")
			}

			if opts.HeaderLengthIncludesHeader == false {
				cl += opts.HeaderSize
			}

			if opts.MaxLength > 0 && cl > opts.MaxLength {
				return gen.ErrTooLarge
			}
		}

		if len(chunk) < cl {
			continue
		}

		if err := onChunk(chunk[:cl]); err != nil {
			return err
		}

		tail := chunk[cl:]

		// prepare the next chunk buffer
		if pool == nil {
			chunk = make([]byte, 0, opts.FixedLength)
		} else {
			chunk = pool.Get().([]byte)
			chunk = chunk[:0]
		}

		cl = opts.FixedLength

		if len(tail) > 0 {
			chunk = append(chunk, tail...)
			goto next
		}
	}
}
