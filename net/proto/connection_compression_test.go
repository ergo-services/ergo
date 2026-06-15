package proto

import (
	"testing"

	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
	"ergo.services/ergo/testing/check"
)

// when compression is enabled and the payload clears the threshold, send wraps the
// frame as protoMessageZ: a type byte naming the algorithm followed by the
// compressed inner frame. Decompressing it must yield the original message frame.
func TestSendCompressed(t *testing.T) {
	cases := []struct {
		name       string
		ctype      gen.CompressionType
		decompress func(*lib.Buffer, uint, int) (*lib.Buffer, error)
	}{
		{"gzip", gen.CompressionTypeGZIP, lib.DecompressGZIP},
		{"zlib", gen.CompressionTypeZLIB, lib.DecompressZLIB},
		{"lzw", gen.CompressionTypeLZW, lib.DecompressLZW},
	}

	for _, tcase := range cases {
		t.Run(tcase.name, func(t *testing.T) {
			tc := newTestConn(t, gen.NetworkFlags{})
			from, to := localPID(5), peerPID(9)
			options := gen.MessageOptions{Compression: gen.Compression{Enable: true, Type: tcase.ctype}}

			err := tc.c.SendPID(from, to, options, "hello")
			check.NoError(t, err)

			_, mtype, body := tc.readFrame(t)
			check.Equal(t, protoMessageZ, mtype)
			check.Equal(t, tcase.ctype.ID(), body[0]) // algorithm id

			src := lib.TakeBuffer()
			src.B = append(src.B, body[1:]...)
			inner, err := tcase.decompress(src, 0, 1<<20)
			check.NoError(t, err)
			lib.ReleaseBuffer(src)

			check.Equal(t, protoMessagePID, inner.B[7]) // the inner frame is a plain PID message
			check.Equal(t, "hello", tc.decode(t, inner.B[33:]))
			lib.ReleaseBuffer(inner)
		})
	}
}
