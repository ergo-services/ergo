package meta

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"

	"ergo.services/ergo/gen"
)

// scriptReader returns one predefined segment per Read (then io.EOF), so a test
// controls exactly how the byte stream is segmented across reads.
type scriptReader struct {
	segments [][]byte
	i        int
}

func (s *scriptReader) Read(p []byte) (int, error) {
	if s.i >= len(s.segments) {
		return 0, io.EOF
	}
	seg := s.segments[s.i]
	s.i++
	return copy(p, seg), nil
}

// collect runs readChunks over the segments and returns the delivered chunks
// (copied, since the reader aliases its buffer) and the terminal error.
func collect(opts ChunkOptions, bufSize int, segments ...[]byte) ([][]byte, error) {
	var got [][]byte
	err := readChunks(&scriptReader{segments: segments}, opts, bufSize, nil, func(c []byte) error {
		cp := make([]byte, len(c))
		copy(cp, c)
		got = append(got, cp)
		return nil
	})
	return got, err
}

// buildFrame builds one length-prefixed frame for the given dynamic-header opts.
func buildFrame(opts ChunkOptions, payload []byte) []byte {
	hdr := make([]byte, opts.HeaderSize)
	lv := len(payload)
	if opts.HeaderLengthIncludesHeader {
		lv += opts.HeaderSize
	}
	switch opts.HeaderLengthSize {
	case 1:
		hdr[opts.HeaderLengthPosition] = byte(lv)
	case 2:
		binary.BigEndian.PutUint16(hdr[opts.HeaderLengthPosition:], uint16(lv))
	case 4:
		binary.BigEndian.PutUint32(hdr[opts.HeaderLengthPosition:], uint32(lv))
	}
	return append(hdr, payload...)
}

func wantChunks(t *testing.T, got [][]byte, want ...[]byte) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d chunks, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if bytes.Equal(got[i], want[i]) == false {
			t.Fatalf("chunk %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestReadChunksFixedLength(t *testing.T) {
	opts := ChunkOptions{Enable: true, FixedLength: 3}

	// a chunk split across reads, with a tail carried across the read boundary;
	// the trailing two bytes never complete a chunk and are dropped at EOF
	got, err := collect(opts, 64,
		[]byte{1, 2},             // partial chunk1
		[]byte{3, 4, 5, 6, 7, 8}, // completes chunk1, chunk2, tail {7,8}
		[]byte{9, 10, 11},        // completes chunk3 {7,8,9}, tail {10,11} dropped
	)
	if err != nil {
		t.Fatal(err)
	}
	wantChunks(t, got,
		[]byte{1, 2, 3},
		[]byte{4, 5, 6},
		[]byte{7, 8, 9},
	)
}

func TestReadChunksHeader1(t *testing.T) {
	opts := ChunkOptions{Enable: true, HeaderSize: 1, HeaderLengthSize: 1, HeaderLengthPosition: 0}
	f1 := buildFrame(opts, []byte("AB"))
	f2 := buildFrame(opts, []byte("CDE"))
	// whole stream in one read -> two frames
	got, err := collect(opts, 64, append(append([]byte{}, f1...), f2...))
	if err != nil {
		t.Fatal(err)
	}
	wantChunks(t, got, f1, f2)
}

func TestReadChunksHeader2(t *testing.T) {
	opts := ChunkOptions{Enable: true, HeaderSize: 2, HeaderLengthSize: 2, HeaderLengthPosition: 0}
	f := buildFrame(opts, bytes.Repeat([]byte{0xAA}, 300)) // length needs 2 bytes
	// split: header in one read, payload across two
	got, err := collect(opts, 512, f[:1], f[1:5], f[5:])
	if err != nil {
		t.Fatal(err)
	}
	wantChunks(t, got, f)
}

func TestReadChunksHeader4(t *testing.T) {
	opts := ChunkOptions{Enable: true, HeaderSize: 4, HeaderLengthSize: 4, HeaderLengthPosition: 0}
	f1 := buildFrame(opts, []byte("hello"))
	f2 := buildFrame(opts, []byte("world!!"))
	// two frames coalesced in a single read -> split out in order
	got, err := collect(opts, 64, append(append([]byte{}, f1...), f2...))
	if err != nil {
		t.Fatal(err)
	}
	wantChunks(t, got, f1, f2)
}

func TestReadChunksHeaderOffset(t *testing.T) {
	// the length field sits after a 2-byte tag inside a 6-byte header
	opts := ChunkOptions{Enable: true, HeaderSize: 6, HeaderLengthSize: 4, HeaderLengthPosition: 2}
	f := buildFrame(opts, []byte("payload"))
	got, err := collect(opts, 64, f)
	if err != nil {
		t.Fatal(err)
	}
	wantChunks(t, got, f)
}

func TestReadChunksHeaderIncludesHeader(t *testing.T) {
	// the length field counts the header bytes too
	opts := ChunkOptions{Enable: true, HeaderSize: 4, HeaderLengthSize: 4, HeaderLengthIncludesHeader: true}
	f := buildFrame(opts, []byte("data"))
	got, err := collect(opts, 64, f)
	if err != nil {
		t.Fatal(err)
	}
	wantChunks(t, got, f)
}

func TestReadChunksMaxLengthExceeded(t *testing.T) {
	opts := ChunkOptions{Enable: true, HeaderSize: 4, HeaderLengthSize: 4, MaxLength: 8}
	f := buildFrame(opts, bytes.Repeat([]byte{1}, 100)) // chunk len 104 > 8
	_, err := collect(opts, 256, f)
	if err != gen.ErrTooLarge {
		t.Fatalf("got %v, want gen.ErrTooLarge", err)
	}
}

func TestReadChunksIncompleteAtEOF(t *testing.T) {
	opts := ChunkOptions{Enable: true, HeaderSize: 4, HeaderLengthSize: 4}
	f := buildFrame(opts, []byte("complete"))
	partial := buildFrame(opts, []byte("never finishes"))
	// one complete frame, then a truncated frame, then EOF: only the complete one
	got, err := collect(opts, 256, f, partial[:6])
	if err != nil {
		t.Fatal(err)
	}
	wantChunks(t, got, f)
}

// a dynamic-length header declaring a length below the header size (0 in the
// worst case) must error instead of spinning forever on the same buffer.
func TestReadChunksRejectsUndersizedLength(t *testing.T) {
	opts := ChunkOptions{
		Enable:                     true,
		HeaderSize:                 4,
		HeaderLengthSize:           4,
		HeaderLengthIncludesHeader: true,
	}
	if _, err := collect(opts, 64, []byte{0, 0, 0, 0}); err == nil {
		t.Fatal("expected an error for a zero chunk length, got nil")
	}
	if _, err := collect(opts, 64, []byte{0, 0, 0, 2}); err == nil {
		t.Fatal("expected an error for a length below header size, got nil")
	}
}
