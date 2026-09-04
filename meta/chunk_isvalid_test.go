package meta

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"ergo.services/ergo/gen"
)

func TestChunkOptionsIsValid(t *testing.T) {
	cases := []struct {
		name string
		opts ChunkOptions
		want string // substring of the error, empty means valid
	}{
		{"disabled", ChunkOptions{Enable: false}, ""},
		{"fixed", ChunkOptions{Enable: true, FixedLength: 4}, ""},
		{"dynamic-ok", ChunkOptions{Enable: true, HeaderSize: 4, HeaderLengthSize: 4}, ""},
		{"zero-header", ChunkOptions{Enable: true}, "HeaderSize must be non-zero"},
		{"out-of-bounds", ChunkOptions{Enable: true, HeaderSize: 2, HeaderLengthSize: 4}, "out of HeaderSize bounds"},
		{"bad-size", ChunkOptions{Enable: true, HeaderSize: 8, HeaderLengthSize: 3}, "must be either: 1, 2, or 4"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.opts.IsValid()
			if c.want == "" {
				if err != nil {
					t.Fatalf("IsValid = %v, want nil", err)
				}
				return
			}
			if err == nil || strings.Contains(err.Error(), c.want) == false {
				t.Fatalf("IsValid = %v, want to contain %q", err, c.want)
			}
		})
	}
}

func TestReadChunksWithPool(t *testing.T) {
	pool := &sync.Pool{New: func() any { return make([]byte, 0, 64) }}
	opts := ChunkOptions{Enable: true, FixedLength: 3}
	var got [][]byte
	err := readChunks(&scriptReader{segments: [][]byte{{1, 2, 3, 4, 5, 6}}}, opts, 64, pool, func(c []byte) error {
		cp := make([]byte, len(c))
		copy(cp, c)
		got = append(got, cp)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	wantChunks(t, got, []byte{1, 2, 3}, []byte{4, 5, 6})
}

func TestReadChunksOnChunkError(t *testing.T) {
	boom := errors.New("boom")
	opts := ChunkOptions{Enable: true, FixedLength: 2}
	err := readChunks(&scriptReader{segments: [][]byte{{1, 2, 3, 4}}}, opts, 64, nil, func(c []byte) error {
		return boom
	})
	if errors.Is(err, boom) == false {
		t.Fatalf("got %v, want boom", err)
	}
}

func TestReadChunksReadError(t *testing.T) {
	want := gen.ErrMalformed
	opts := ChunkOptions{Enable: true, FixedLength: 2}
	err := readChunks(&errReader{err: want}, opts, 64, nil, func(c []byte) error { return nil })
	if errors.Is(err, want) == false {
		t.Fatalf("got %v, want %v", err, want)
	}
}

// errReader returns some bytes once together with a non-EOF error, so readChunks
// surfaces a read error (n>0 with err set) rather than treating it as a clean close.
type errReader struct {
	err  error
	done bool
}

func (e *errReader) Read(p []byte) (int, error) {
	if e.done {
		return 0, e.err
	}
	e.done = true
	p[0] = 1
	return 1, e.err
}
